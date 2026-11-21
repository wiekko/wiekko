// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Package config resolves the CLI's settings from the user config file, the
// repository config file, the environment and command line flags, in that
// order of increasing precedence.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"nexmap.net/internal/client"
)

// RepoFileName is the repository config file, committed alongside the code.
// It never holds credentials.
const RepoFileName = ".nexmap.json"

// Environment variables the CLI reads.
const (
	EnvServer    = "NEXMAP_SERVER"
	EnvProduct   = "NEXMAP_PRODUCT"
	EnvToken     = "NEXMAP_TOKEN"
	EnvOutput    = "NEXMAP_OUTPUT"
	EnvAllowHTTP = "NEXMAP_ALLOW_HTTP"
	// EnvConfigDir overrides the user config directory (default:
	// os.UserConfigDir()/nexmap).
	EnvConfigDir = "NEXMAP_CONFIG_DIR"
	// EnvNoUpdateCheck turns off the daily check for a newer release.
	EnvNoUpdateCheck = "NEXMAP_NO_UPDATE_CHECK"
	// EnvReleasesURL overrides where releases are looked up and downloaded
	// (default: update.DefaultBaseURL).
	EnvReleasesURL = "NEXMAP_RELEASES_URL"
)

// Repo is the content of .nexmap.json. Unknown fields are ignored so an older
// CLI can read a file written for a newer one.
type Repo struct {
	Server  string `json:"server,omitempty"`
	Product string `json:"product,omitempty"`
}

// FindRepo looks for .nexmap.json in dir and its parents. It returns the path
// and content of the first one found, or "" and nil when there is none.
func FindRepo(dir string) (string, *Repo, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", nil, err
	}
	for {
		p := filepath.Join(dir, RepoFileName)
		data, err := os.ReadFile(p)
		switch {
		case err == nil:
			var r Repo
			if err := json.Unmarshal(data, &r); err != nil {
				return p, nil, fmt.Errorf("%s: %w", p, err)
			}
			return p, &r, nil
		case !errors.Is(err, fs.ErrNotExist):
			return p, nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil, nil
		}
		dir = parent
	}
}

// WriteRepo writes r to path as indented JSON.
func WriteRepo(path string, r *Repo) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// FindRepoRoot returns the closest directory at or above dir containing a .git
// entry, or dir itself when there is none.
func FindRepoRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for d := abs; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return abs
		}
		d = parent
	}
}

// User is the per-user config file holding credentials. It lives outside any
// repository and is written with owner-only permissions.
type User struct {
	// Servers maps a normalized server URL to its credentials.
	Servers map[string]ServerCredentials `json:"servers,omitempty"`
	// Output is the default output format.
	Output string `json:"output,omitempty"`
}

// ServerCredentials is what the CLI stores for one server.
type ServerCredentials struct {
	Token    string `json:"token"`
	Username string `json:"username,omitempty"` // informational
}

// UserDir returns the directory holding the user config file.
func UserDir(getenv func(string) string) (string, error) {
	if d := getenv(EnvConfigDir); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w (set %s)", err, EnvConfigDir)
	}
	return filepath.Join(base, "nexmap"), nil
}

// UserFile returns the path of the user config file.
func UserFile(getenv func(string) string) (string, error) {
	dir, err := UserDir(getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadUser reads the user config file. A missing file yields an empty config.
func LoadUser(path string) (*User, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &User{}, nil
	}
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &u, nil
}

// SaveUser writes the user config file with owner-only permissions, replacing
// it atomically so a crash never leaves a truncated credentials file.
func SaveUser(path string, u *User) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// SetToken stores the token for server (normalized first).
func (u *User) SetToken(server, token, username string) error {
	key, err := client.NormalizeServer(server)
	if err != nil {
		return err
	}
	if u.Servers == nil {
		u.Servers = map[string]ServerCredentials{}
	}
	u.Servers[key] = ServerCredentials{Token: token, Username: username}
	return nil
}

// RemoveToken deletes the stored token for server and reports whether there
// was one.
func (u *User) RemoveToken(server string) (bool, error) {
	key, err := client.NormalizeServer(server)
	if err != nil {
		return false, err
	}
	if _, ok := u.Servers[key]; !ok {
		return false, nil
	}
	delete(u.Servers, key)
	return true, nil
}

// Token returns the stored token for server, if any.
func (u *User) Token(server string) string {
	key, err := client.NormalizeServer(server)
	if err != nil {
		return ""
	}
	return u.Servers[key].Token
}
