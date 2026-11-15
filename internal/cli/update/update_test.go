package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	// Each version ranks below the next (SemVer §11 example, plus our own).
	ordered := []string{
		"v0.9.0",
		"v1.0.0-alpha",
		"v1.0.0-alpha.1",
		"v1.0.0-alpha.beta",
		"v1.0.0-beta",
		"v1.0.0-beta.2",
		"v1.0.0-beta.11",
		"v1.0.0-rc.1",
		"v1.0.0",
		"v1.0.1",
		"v1.2.0",
		"v1.10.0",
		"v2.0.0",
	}
	for i := range ordered {
		for j := range ordered {
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := Compare(ordered[i], ordered[j]); got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
	if Compare("v1.0.0+dev.abc", "v1.0.0") != 0 {
		t.Error("build metadata must not affect precedence")
	}
	if Compare("garbage", "v0.0.1") != -1 || Compare("v0.0.1", "garbage") != 1 {
		t.Error("invalid versions must sort first")
	}
}

func TestValid(t *testing.T) {
	for v, want := range map[string]bool{
		"v1.2.3":           true,
		"v1.2.3-rc.1":      true,
		"v1.2.3+dev":       false,
		"v1.2.3+dev.abcde": false,
		"1.2.3":            false,
		"v1.2":             false,
		"v01.2.3":          false,
		"v1.2.3-":          false,
		"v1.2.3-a..b":      false,
		"v1.2.3/../x":      false,
		"":                 false,
	} {
		if Valid(v) != want {
			t.Errorf("Valid(%q) = %v, want %v", v, !want, want)
		}
	}
	if !Final("v1.2.3") || Final("v1.2.3-rc.1") || Final("v1.2.3+dev") {
		t.Error("Final")
	}
}

func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write(body)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(body)
	}
	zw.Close()
	return buf.Bytes()
}

func TestExtract(t *testing.T) {
	bin := []byte("binary")
	files := map[string][]byte{"LICENSE": []byte("MIT"), "nexmap": bin, "nexmap.exe": bin}
	for name, data := range map[string][]byte{"a.tar.gz": tarGz(t, files), "a.zip": zipOf(t, files)} {
		got, err := Extract(name, data, "nexmap")
		if err != nil || !bytes.Equal(got, bin) {
			t.Errorf("%s: got %q, %v", name, got, err)
		}
		if _, err := Extract(name, data, "missing"); err == nil {
			t.Errorf("%s: extracting a missing file must fail", name)
		}
	}
	if _, err := Extract("a.rar", nil, "nexmap"); err == nil {
		t.Error("unknown formats must fail")
	}
}

// releaseSite serves one release, v1.2.0, for the running platform.
func releaseSite(t *testing.T, tamper bool) (*httptest.Server, []byte) {
	t.Helper()
	bin := []byte("new nexmap " + runtime.GOOS)
	name := ArchiveName("v1.2.0", runtime.GOOS, runtime.GOARCH)
	files := map[string][]byte{BinaryName(runtime.GOOS): bin}
	archive := tarGz(t, files)
	if runtime.GOOS == "windows" {
		archive = zipOf(t, files)
	}
	sum := sha256.Sum256(archive)
	if tamper {
		archive = append(archive, 0)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":"v1.2.0","api_level":3}`)
	})
	mux.HandleFunc("/v1.2.0/release.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":"v1.2.0","api_level":3}`)
	})
	mux.HandleFunc("/v1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n%s  other.tar.gz\n", hex.EncodeToString(sum[:]), name, strings.Repeat("0", 64))
	})
	mux.HandleFunc("/v1.2.0/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, bin
}

func TestClient(t *testing.T) {
	srv, bin := releaseSite(t, false)
	c := &Client{Base: srv.URL + "/"}
	ctx := context.Background()

	r, err := c.Latest(ctx)
	if err != nil || r.Version != "v1.2.0" || r.APILevel != 3 {
		t.Fatalf("Latest = %+v, %v", r, err)
	}
	if r, err := c.Release(ctx, "v1.2.0"); err != nil || r.Version != "v1.2.0" {
		t.Fatalf("Release = %+v, %v", r, err)
	}
	if _, err := c.Release(ctx, "v9.9.9"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing release: %v", err)
	}
	got, err := c.Download(ctx, "v1.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil || !bytes.Equal(got, bin) {
		t.Fatalf("Download = %q, %v", got, err)
	}
	if _, err := c.Download(ctx, "v1.2.0", "plan9", "mips"); err == nil || !strings.Contains(err.Error(), "no build") {
		t.Fatalf("unknown platform: %v", err)
	}
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	srv, _ := releaseSite(t, true)
	c := &Client{Base: srv.URL}
	_, err := c.Download(context.Background(), "v1.2.0", runtime.GOOS, runtime.GOARCH)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
}

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, BinaryName(runtime.GOOS))
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Fatalf("exe = %q", b)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(exe); fi.Mode().Perm()&0o100 == 0 {
			t.Fatalf("mode = %v, want executable", fi.Mode())
		}
	}
	CleanupOld(exe)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestState(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sub", "state.json")
	if s := LoadState(file); !s.CheckedAt.IsZero() {
		t.Fatalf("missing file: %+v", s)
	}
	now := time.Now().Truncate(time.Second)
	if err := SaveState(file, State{CheckedAt: now, Latest: "v1.2.0"}); err != nil {
		t.Fatal(err)
	}
	if s := LoadState(file); !s.CheckedAt.Equal(now) || s.Latest != "v1.2.0" {
		t.Fatalf("state = %+v", s)
	}
}
