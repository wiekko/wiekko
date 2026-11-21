// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindRepoWalksUp(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, RepoFileName), `{"server":"https://n.example","product":"p1","future":true}`)
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	path, repo, err := FindRepo(deep)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	if path != filepath.Join(root, RepoFileName) {
		t.Fatalf("path = %q", path)
	}
	if repo.Server != "https://n.example" || repo.Product != "p1" {
		t.Fatalf("repo = %+v", repo)
	}
}

func TestFindRepoInvalidJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, RepoFileName), `{nope`)
	if _, _, err := FindRepo(root); err == nil {
		t.Fatal("want error for invalid JSON")
	}
}

func TestFindRepoRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "x", "y")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := FindRepoRoot(deep); got != root {
		t.Fatalf("FindRepoRoot = %q, want %q", got, root)
	}
}

func TestUserConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	u, err := LoadUser(path)
	if err != nil {
		t.Fatalf("LoadUser(missing): %v", err)
	}
	if err := u.SetToken("https://N.example/api/v1/", "nxp_1", "ada"); err != nil {
		t.Fatal(err)
	}
	if err := SaveUser(path, u); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("perm = %o, want 600", perm)
		}
	}

	loaded, err := LoadUser(path)
	if err != nil {
		t.Fatalf("LoadUser: %v", err)
	}
	if got := loaded.Token("https://n.example"); got != "nxp_1" {
		t.Fatalf("Token = %q", got)
	}
	removed, err := loaded.RemoveToken("https://n.example/")
	if err != nil || !removed {
		t.Fatalf("RemoveToken = %v, %v", removed, err)
	}
	if loaded.Token("https://n.example") != "" {
		t.Fatal("token still present")
	}
}

func TestResolvePrecedence(t *testing.T) {
	cfgDir := t.TempDir()
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, RepoFileName), `{"server":"https://repo.example","product":"repo-product"}`)

	u := &User{Output: "md"}
	_ = u.SetToken("https://repo.example", "tok-repo", "")
	_ = u.SetToken("https://env.example", "tok-env-server", "")
	_ = u.SetToken("https://flag.example", "tok-flag-server", "")
	if err := SaveUser(filepath.Join(cfgDir, "config.json"), u); err != nil {
		t.Fatal(err)
	}

	base := map[string]string{EnvConfigDir: cfgDir}

	t.Run("repo config", func(t *testing.T) {
		s, err := Resolve(repoDir, envFunc(base), Overrides{})
		if err != nil {
			t.Fatal(err)
		}
		if s.Server != "https://repo.example" || s.Product != "repo-product" || s.Token != "tok-repo" || s.Output != "md" {
			t.Fatalf("settings = %+v", s)
		}
		if s.ServerSource != filepath.Join(repoDir, RepoFileName) {
			t.Fatalf("ServerSource = %q", s.ServerSource)
		}
	})

	t.Run("env beats repo, token follows server", func(t *testing.T) {
		env := map[string]string{EnvConfigDir: cfgDir, EnvServer: "https://env.example", EnvProduct: "env-product", EnvOutput: "json"}
		s, err := Resolve(repoDir, envFunc(env), Overrides{})
		if err != nil {
			t.Fatal(err)
		}
		if s.Server != "https://env.example" || s.Product != "env-product" || s.Token != "tok-env-server" || s.Output != "json" {
			t.Fatalf("settings = %+v", s)
		}
	})

	t.Run("flags beat env", func(t *testing.T) {
		env := map[string]string{EnvConfigDir: cfgDir, EnvServer: "https://env.example", EnvProduct: "env-product"}
		s, err := Resolve(repoDir, envFunc(env), Overrides{Server: "https://flag.example", Product: "flag-product", Output: "table"})
		if err != nil {
			t.Fatal(err)
		}
		if s.Server != "https://flag.example" || s.Product != "flag-product" || s.Token != "tok-flag-server" || s.Output != "table" {
			t.Fatalf("settings = %+v", s)
		}
		if s.ServerSource != "--server" || s.ProductSource != "--product" {
			t.Fatalf("sources = %q, %q", s.ServerSource, s.ProductSource)
		}
	})

	t.Run("WIEKKO_TOKEN beats stored token", func(t *testing.T) {
		env := map[string]string{EnvConfigDir: cfgDir, EnvToken: "tok-from-env", EnvAllowHTTP: "1"}
		s, err := Resolve(repoDir, envFunc(env), Overrides{})
		if err != nil {
			t.Fatal(err)
		}
		if s.Token != "tok-from-env" || s.TokenSource != EnvToken || !s.AllowHTTP {
			t.Fatalf("settings = %+v", s)
		}
	})
}
