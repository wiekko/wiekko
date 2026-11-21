// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"wiekko.net/internal/buildinfo"
	"wiekko.net/internal/cli"
	cliconfig "wiekko.net/internal/cli/config"
	"wiekko.net/internal/cli/update"
)

// releaseSite serves latest (with api level) as the newest release, built for
// the running platform; its binary is "binary of <version>".
type releaseSite struct {
	url      string
	requests atomic.Int32
}

func newReleaseSite(t *testing.T, latest string, apiLevel int) *releaseSite {
	t.Helper()
	bin := []byte("binary of " + latest)
	name := update.ArchiveName(latest, runtime.GOOS, runtime.GOARCH)
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create(update.BinaryName(runtime.GOOS))
		w.Write(bin)
		zw.Close()
	} else {
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: "wiekko", Mode: 0o755, Size: int64(len(bin)), Typeflag: tar.TypeReg})
		tw.Write(bin)
		tw.Close()
		gz.Close()
	}
	archive := buf.Bytes()
	sum := sha256.Sum256(archive)
	meta := fmt.Sprintf(`{"version":%q,"api_level":%d}`, latest, apiLevel)

	s := &releaseSite{}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest.json", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, meta) })
	mux.HandleFunc("/"+latest+"/release.json", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, meta) })
	mux.HandleFunc("/"+latest+"/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	})
	mux.HandleFunc("/"+latest+"/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

// asVersion makes the CLI report version v for the duration of the test.
func asVersion(t *testing.T, v string) {
	t.Helper()
	old := buildinfo.Version
	buildinfo.Version = v
	t.Cleanup(func() { buildinfo.Version = old })
}

type updateEnv struct {
	env    map[string]string
	exe    string
	stderr bool // StderrIsTTY
}

func newUpdateEnv(t *testing.T, site *releaseSite) *updateEnv {
	t.Helper()
	exe := filepath.Join(t.TempDir(), update.BinaryName(runtime.GOOS))
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &updateEnv{
		env: map[string]string{
			cliconfig.EnvConfigDir:   t.TempDir(),
			cliconfig.EnvReleasesURL: site.url,
		},
		exe: exe,
	}
}

func (u *updateEnv) run(args ...string) result {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.IO{
		Stdin:       strings.NewReader(""),
		Stdout:      &out,
		Stderr:      &errOut,
		Getenv:      func(k string) string { return u.env[k] },
		Cwd:         filepath.Dir(u.exe),
		StderrIsTTY: u.stderr,
		Executable:  func() (string, error) { return u.exe, nil },
	})
	return result{code: code, stdout: out.String(), stderr: errOut.String()}
}

func TestUpdateCheck(t *testing.T) {
	asVersion(t, "v1.0.0")
	u := newUpdateEnv(t, newReleaseSite(t, "v1.1.0", buildinfo.APILevel))

	r := u.run("update", "--check", "-o", "json")
	if r.code != cli.ExitOK {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	got := decode[struct {
		Current         string `json:"current"`
		Latest          string `json:"latest"`
		UpdateAvailable bool   `json:"update_available"`
	}](t, r.stdout)
	if got.Current != "v1.0.0" || got.Latest != "v1.1.0" || !got.UpdateAvailable {
		t.Fatalf("check = %+v", got)
	}
	if b, _ := os.ReadFile(u.exe); string(b) != "old binary" {
		t.Fatal("--check must not replace the binary")
	}
}

func TestUpdateReplacesBinary(t *testing.T) {
	asVersion(t, "v1.0.0")
	u := newUpdateEnv(t, newReleaseSite(t, "v1.1.0", buildinfo.APILevel))

	r := u.run("update", "-o", "json")
	if r.code != cli.ExitOK {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if b, _ := os.ReadFile(u.exe); string(b) != "binary of v1.1.0" {
		t.Fatalf("binary = %q", b)
	}
	got := decode[struct{ Previous, Version string }](t, r.stdout)
	if got.Previous != "v1.0.0" || got.Version != "v1.1.0" {
		t.Fatalf("result = %+v", got)
	}

	// Asking for a version that does not exist fails without touching it.
	r = u.run("update", "v7.0.0")
	if r.code != cli.ExitAPI || !strings.Contains(r.stderr, "not found") {
		t.Fatalf("missing version: exit %d, %s", r.code, r.stderr)
	}
}

func TestUpdateUpToDate(t *testing.T) {
	asVersion(t, "v1.1.0")
	u := newUpdateEnv(t, newReleaseSite(t, "v1.1.0", buildinfo.APILevel))
	r := u.run("update")
	if r.code != cli.ExitOK || !strings.Contains(r.stderr, "up to date") {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if b, _ := os.ReadFile(u.exe); string(b) != "old binary" {
		t.Fatal("an up-to-date binary must stay")
	}
	if r := u.run("update", "--force"); r.code != cli.ExitOK {
		t.Fatalf("--force: exit %d: %s", r.code, r.stderr)
	}
	if b, _ := os.ReadFile(u.exe); string(b) != "binary of v1.1.0" {
		t.Fatal("--force must reinstall")
	}
}

func TestUpdateRefusesDevBuildWithoutVersion(t *testing.T) {
	asVersion(t, "v1.0.0+dev.abc")
	u := newUpdateEnv(t, newReleaseSite(t, "v1.1.0", buildinfo.APILevel))
	if r := u.run("update"); r.code != cli.ExitUsage || !strings.Contains(r.stderr, "development build") {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if r := u.run("update", "v1.1.0"); r.code != cli.ExitOK {
		t.Fatalf("explicit version: exit %d: %s", r.code, r.stderr)
	}
}

func TestUpdateRefusesUnsupportedServer(t *testing.T) {
	asVersion(t, "v1.0.0")
	e := newE2E(t)
	u := newUpdateEnv(t, newReleaseSite(t, "v2.0.0", buildinfo.APILevel+1))
	u.env[cliconfig.EnvServer] = e.serverURL

	r := u.run("update")
	if r.code != cli.ExitUsage || !strings.Contains(r.stderr, "API level") {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if b, _ := os.ReadFile(u.exe); string(b) != "old binary" {
		t.Fatal("the binary must stay")
	}
	if r := u.run("update", "--force"); r.code != cli.ExitOK {
		t.Fatalf("--force: exit %d: %s", r.code, r.stderr)
	}
}

func TestUpdateNotice(t *testing.T) {
	asVersion(t, "v1.0.0")
	site := newReleaseSite(t, "v1.1.0", buildinfo.APILevel)
	u := newUpdateEnv(t, site)

	// Not on a terminal: no check at all.
	u.run("help")
	u.run("version")
	if n := site.requests.Load(); n != 0 {
		t.Fatalf("checked without a terminal (%d requests)", n)
	}

	u.stderr = true
	u.env["CI"] = "true"
	u.run("version")
	if n := site.requests.Load(); n != 0 {
		t.Fatal("checked in CI")
	}
	delete(u.env, "CI")
	u.env[cliconfig.EnvNoUpdateCheck] = "1"
	u.run("version")
	if n := site.requests.Load(); n != 0 {
		t.Fatal("checked although turned off")
	}
	delete(u.env, cliconfig.EnvNoUpdateCheck)

	r := u.run("version")
	if !strings.Contains(r.stderr, "v1.1.0 is available") {
		t.Fatalf("no notice: %q", r.stderr)
	}
	// Once a day only.
	r = u.run("version")
	if strings.Contains(r.stderr, "available") || site.requests.Load() != 1 {
		t.Fatalf("checked twice: %q, %d requests", r.stderr, site.requests.Load())
	}
	state := update.LoadState(filepath.Join(u.env[cliconfig.EnvConfigDir], "update-check.json"))
	if state.Latest != "v1.1.0" || time.Since(state.CheckedAt) > time.Minute {
		t.Fatalf("state = %+v", state)
	}
}

func TestUpdateNoticeSkipsDevBuilds(t *testing.T) {
	site := newReleaseSite(t, "v1.1.0", buildinfo.APILevel)
	u := newUpdateEnv(t, site)
	u.stderr = true

	asVersion(t, "v1.0.0+dev.abc")
	u.run("version")
	if site.requests.Load() != 0 {
		t.Fatal("dev builds must not check")
	}
}
