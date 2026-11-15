// Package update finds, downloads and installs CLI releases from the download
// site (nexmap.net/releases, see site/ in the repository). It only uses the
// standard library.
//
// The site's layout, written by site/Dockerfile and its entrypoint hook:
//
//	<base>/latest.json                         the newest final release
//	<base>/<version>/release.json              {"version": …, "api_level": …}
//	<base>/<version>/SHA256SUMS                sha256sum format, one line per archive
//	<base>/<version>/nexmap_<version>_<os>_<arch>.tar.gz   (.zip for windows)
package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DefaultBaseURL is where releases are published.
const DefaultBaseURL = "https://nexmap.net/releases"

// Size limits for what the site returns, so a broken or hostile response
// cannot exhaust memory.
const (
	maxMetadata = 64 << 10
	maxArchive  = 100 << 20
	maxBinary   = 200 << 20
)

// ErrNotFound is returned when the requested release does not exist.
var ErrNotFound = errors.New("release not found")

// Release describes a published release.
type Release struct {
	Version string `json:"version"`
	// APILevel is the API level the release requires of the server
	// (buildinfo.APILevel of that build).
	APILevel int `json:"api_level"`
}

// Client talks to the download site.
type Client struct {
	Base      string // base URL, without trailing slash; empty means DefaultBaseURL
	UserAgent string
	HTTP      *http.Client // nil means http.DefaultClient
}

func (c *Client) base() string {
	if c.Base == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(c.Base, "/")
}

// Latest returns the newest final release.
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	return c.release(ctx, c.base()+"/latest.json", "")
}

// Release returns the release with the given version.
func (c *Client) Release(ctx context.Context, version string) (*Release, error) {
	if !Valid(version) {
		return nil, fmt.Errorf("%q is not a release version like v1.2.3", version)
	}
	r, err := c.release(ctx, c.base()+"/"+version+"/release.json", version)
	if errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("%s: %w", version, ErrNotFound)
	}
	return r, err
}

func (c *Client) release(ctx context.Context, url, want string) (*Release, error) {
	body, err := c.get(ctx, url, maxMetadata)
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	if !Valid(r.Version) || (want != "" && r.Version != want) {
		return nil, fmt.Errorf("read %s: unexpected version %q", url, r.Version)
	}
	return &r, nil
}

// ArchiveName is the file name of the release archive for a platform.
func ArchiveName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("nexmap_%s_%s_%s%s", version, goos, goarch, ext)
}

// BinaryName is the executable's name inside the archive.
func BinaryName(goos string) string {
	if goos == "windows" {
		return "nexmap.exe"
	}
	return "nexmap"
}

// Download fetches the archive of version for goos/goarch, checks it against
// the release's SHA256SUMS and returns the executable it contains.
func (c *Client) Download(ctx context.Context, version, goos, goarch string) ([]byte, error) {
	if !Valid(version) {
		return nil, fmt.Errorf("%q is not a release version like v1.2.3", version)
	}
	dir := c.base() + "/" + version + "/"
	name := ArchiveName(version, goos, goarch)

	sums, err := c.get(ctx, dir+"SHA256SUMS", maxMetadata)
	if err != nil {
		return nil, err
	}
	want, ok := lookupSum(sums, name)
	if !ok {
		return nil, fmt.Errorf("%s has no build for %s/%s", version, goos, goarch)
	}
	archive, err := c.get(ctx, dir+name, maxArchive)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("%s: checksum mismatch; the download is damaged or was tampered with", name)
	}
	return Extract(name, archive, BinaryName(goos))
}

// lookupSum finds name in sha256sum output ("<hex>  <name>", or "<hex> *<name>").
func lookupSum(sums []byte, name string) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name && len(fields[0]) == 64 {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

// Extract returns the file called binary from a .tar.gz or .zip archive.
func Extract(archiveName string, data []byte, binary string) ([]byte, error) {
	switch {
	case strings.HasSuffix(archiveName, ".zip"):
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", archiveName, err)
		}
		for _, f := range zr.File {
			if path.Base(f.Name) != binary || f.FileInfo().IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("%s: %w", archiveName, err)
			}
			defer rc.Close()
			return readLimited(rc, maxBinary, archiveName)
		}
	case strings.HasSuffix(archiveName, ".tar.gz"):
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", archiveName, err)
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", archiveName, err)
			}
			if h.Typeflag == tar.TypeReg && path.Base(h.Name) == binary {
				return readLimited(tr, maxBinary, archiveName)
			}
		}
	default:
		return nil, fmt.Errorf("%s: unknown archive format", archiveName)
	}
	return nil, fmt.Errorf("%s does not contain %s", archiveName, binary)
}

func readLimited(r io.Reader, limit int64, what string) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: larger than %d bytes", what, limit)
	}
	return b, nil
}

func (c *Client) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("GET %s: %w", url, ErrNotFound)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return readLimited(resp.Body, limit, url)
}

// Replace installs bin as the executable at exe. The new file is written next
// to exe and renamed over it, so exe is never half-written. Windows cannot
// overwrite a running executable but can rename it, so there the old one is
// moved to exe+".old" first (see CleanupOld).
func Replace(exe string, bin []byte) error {
	mode := os.FileMode(0o755)
	if fi, err := os.Stat(exe); err == nil {
		mode = fi.Mode().Perm() | 0o100
	}
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".nexmap-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		return os.Rename(tmpName, exe)
	}
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(tmpName, exe); err != nil {
		_ = os.Rename(old, exe)
		return err
	}
	return nil
}

// CleanupOld removes the previous executable Replace left behind on Windows.
// It fails silently while that file is still in use.
func CleanupOld(exe string) {
	_ = os.Remove(exe + ".old")
}

// State is what the background update check remembers between runs.
type State struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest,omitempty"`
}

// LoadState reads the state file; a missing or unreadable file yields the
// zero State, which means "never checked".
func LoadState(file string) State {
	var s State
	if b, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// SaveState writes the state file atomically.
func SaveState(file string, s State) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".update-check-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}
