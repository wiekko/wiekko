package cli_test

// End-to-end tests: the CLI runs in-process against the real router, service
// and store stack on a throwaway SQLite database. Server packages are imported
// here only; the CLI itself never links them (see cmd/nexmap/deps_test.go).

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/auth/builtin"
	"nexmap.net/internal/buildinfo"
	"nexmap.net/internal/cli"
	cliconfig "nexmap.net/internal/cli/config"
	"nexmap.net/internal/config"
	"nexmap.net/internal/crypto"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/devseed"
	"nexmap.net/internal/model"
	"nexmap.net/internal/router"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

type e2e struct {
	t         *testing.T
	serverURL string
	token     string
	products  []*model.Product
	configDir string
	repoDir   string
	env       map[string]string
}

func newE2E(t *testing.T) *e2e { return newEnv(t, false) }

// newSeededE2E populates the database with the dev-mode demo products
// (Aviary, Finch Tracker) instead of two empty ones.
func newSeededE2E(t *testing.T) *e2e { return newEnv(t, true) }

func newEnv(t *testing.T, seed bool) *e2e {
	t.Helper()
	db := dbtest.Open(t, dbtest.SQLite)

	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	keys, err := crypto.NewKeyRing(`[{"id":"test","key":"` + base64.StdEncoding.EncodeToString(raw) + `"}]`)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := storage.NewFS(filepath.Join(t.TempDir(), "uploads"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{SessionTTL: time.Hour, PBKDF2Iterations: 1}
	st := store.New(db, store.Dialect(dbtest.SQLite), keys)
	provider := builtin.New(st.User, cfg.PBKDF2Iterations)
	svc := service.New(st, provider, cfg, blobs)

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	admin := &model.User{
		ID: uuid.New().String(), Username: "root", Email: "root@localhost", DisplayName: "Root",
		PasswordHash: "x", IsAdmin: true, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := st.User.Create(ctx, admin); err != nil {
		t.Fatalf("create user: %v", err)
	}
	var products []*model.Product
	if seed {
		if err := devseed.Seed(ctx, svc, st, provider, admin); err != nil {
			t.Fatalf("seed: %v", err)
		}
		list, err := svc.Product.List(ctx, admin)
		if err != nil {
			t.Fatalf("list products: %v", err)
		}
		products = list
	} else {
		for _, name := range []string{"Aviary", "Finch"} {
			p, err := svc.Product.Create(ctx, admin, service.CreateProductInput{Name: name})
			if err != nil {
				t.Fatalf("create product: %v", err)
			}
			products = append(products, p)
		}
	}
	pat, err := svc.PAT.Create(ctx, admin.ID, "cli e2e", nil)
	if err != nil {
		t.Fatalf("create PAT: %v", err)
	}

	srv := httptest.NewServer(router.New(svc, false, false, 1<<20))
	t.Cleanup(srv.Close)

	repoDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	configDir := t.TempDir()
	return &e2e{
		t: t, serverURL: srv.URL, token: pat.RawToken, products: products,
		configDir: configDir, repoDir: repoDir,
		env: map[string]string{cliconfig.EnvConfigDir: configDir},
	}
}

type result struct {
	code           int
	stdout, stderr string
}

// run executes the CLI in dir (default: the repo dir) with stdin.
func (e *e2e) run(stdin, dir string, args ...string) result {
	e.t.Helper()
	if dir == "" {
		dir = e.repoDir
	}
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.IO{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out,
		Stderr: &errOut,
		Getenv: func(k string) string { return e.env[k] },
		Cwd:    dir,
	})
	return result{code: code, stdout: out.String(), stderr: errOut.String()}
}

func (e *e2e) mustRun(stdin string, args ...string) result {
	e.t.Helper()
	r := e.run(stdin, "", args...)
	if r.code != cli.ExitOK {
		e.t.Fatalf("nexmap %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), r.code, r.stdout, r.stderr)
	}
	return r
}

func decode[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return v
}

func TestE2EVersionReportsServer(t *testing.T) {
	e := newE2E(t)
	r := e.mustRun("", "version", "--server", e.serverURL, "-o", "json")
	v := decode[struct {
		Client struct {
			APILevel int `json:"api_level"`
		} `json:"client"`
		Server struct {
			URL        string `json:"url"`
			APILevel   int    `json:"api_level"`
			Compatible bool   `json:"compatible"`
		} `json:"server"`
	}](t, r.stdout)
	if v.Server.URL != e.serverURL || v.Server.APILevel != buildinfo.APILevel || !v.Server.Compatible {
		t.Fatalf("version = %+v", v)
	}
}

func TestE2ELoginWhoamiLogout(t *testing.T) {
	e := newE2E(t)

	r := e.run("not-a-token\n", "", "login", "--server", e.serverURL)
	if r.code != cli.ExitAPI || !strings.Contains(r.stderr, "rejected") {
		t.Fatalf("bad token: exit %d, stderr %q", r.code, r.stderr)
	}

	r = e.run("", "", "whoami", "--server", e.serverURL)
	if r.code != cli.ExitUsage || !strings.Contains(r.stderr, "not logged in") {
		t.Fatalf("whoami without token: exit %d, stderr %q", r.code, r.stderr)
	}

	e.mustRun(e.token+"\n", "login", "--server", e.serverURL+"/api/v1/")
	stored, err := os.ReadFile(filepath.Join(e.configDir, "config.json"))
	if err != nil || !strings.Contains(string(stored), e.token) {
		t.Fatalf("token not stored: %v %s", err, stored)
	}

	r = e.mustRun("", "whoami", "--server", e.serverURL, "-o", "json")
	who := decode[struct {
		Server string `json:"server"`
		User   struct {
			Username string `json:"username"`
		} `json:"user"`
	}](t, r.stdout)
	if who.User.Username != "root" || who.Server != e.serverURL {
		t.Fatalf("whoami = %+v", who)
	}

	e.mustRun("", "logout", "--server", e.serverURL)
	r = e.run("", "", "whoami", "--server", e.serverURL)
	if r.code != cli.ExitUsage {
		t.Fatalf("whoami after logout: exit %d", r.code)
	}
}

func TestE2ETokenFromEnvironment(t *testing.T) {
	e := newE2E(t)
	e.env[cliconfig.EnvToken] = e.token
	e.env[cliconfig.EnvServer] = e.serverURL
	r := e.mustRun("", "whoami")
	if !strings.Contains(r.stdout, "root") || !strings.Contains(r.stdout, cliconfig.EnvToken) {
		t.Fatalf("whoami = %q", r.stdout)
	}
}

func TestE2EInit(t *testing.T) {
	e := newE2E(t)
	e.env[cliconfig.EnvToken] = e.token
	sub := filepath.Join(e.repoDir, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Two products and no terminal: the user must choose with --product.
	r := e.run("", sub, "init", "--server", e.serverURL)
	if r.code != cli.ExitUsage || !strings.Contains(r.stderr, "Aviary") || !strings.Contains(r.stderr, "Finch") {
		t.Fatalf("init without product: exit %d, stderr %q", r.code, r.stderr)
	}

	finch := e.products[1]
	r = e.run("", sub, "init", "--server", e.serverURL, "--product", finch.ID)
	if r.code != cli.ExitOK {
		t.Fatalf("init: exit %d, stderr %q", r.code, r.stderr)
	}
	path := filepath.Join(e.repoDir, cliconfig.RepoFileName)
	repo := decode[cliconfig.Repo](t, mustRead(t, path))
	if repo.Server != e.serverURL || repo.Product != finch.ID {
		t.Fatalf(".nexmap.json = %+v", repo)
	}
	if strings.Contains(mustRead(t, path), e.token) {
		t.Fatal(".nexmap.json must never contain the token")
	}

	r = e.run("", sub, "init", "--server", e.serverURL, "--product", finch.ID)
	if r.code != cli.ExitUsage || !strings.Contains(r.stderr, "already exists") {
		t.Fatalf("second init: exit %d, stderr %q", r.code, r.stderr)
	}

	// Commands in a subdirectory pick the repo config up.
	r = e.run("", sub, "whoami", "-o", "json")
	who := decode[struct {
		Product struct {
			Name string `json:"name"`
		} `json:"product"`
	}](t, r.stdout)
	if r.code != cli.ExitOK || who.Product.Name != "Finch" {
		t.Fatalf("whoami in subdir: exit %d, %+v, stderr %q", r.code, who, r.stderr)
	}
}

func TestE2EAPI(t *testing.T) {
	e := newE2E(t)
	e.env[cliconfig.EnvToken] = e.token
	e.env[cliconfig.EnvServer] = e.serverURL
	aviary := e.products[0]

	r := e.mustRun("", "api", "/products/{product}", "--product", aviary.ID)
	got := decode[struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}](t, r.stdout)
	if got.Data.Name != "Aviary" {
		t.Fatalf("api GET = %q", r.stdout)
	}

	r = e.mustRun(`{"name":"Created by CLI"}`, "api", "POST", "/products", "--data", "@-")
	if !strings.Contains(r.stdout, "Created by CLI") {
		t.Fatalf("api POST = %q", r.stdout)
	}

	r = e.run("", "", "api", "GET", "/products/"+uuid.New().String())
	if r.code != cli.ExitNotFound || !strings.Contains(r.stdout, `"error"`) {
		t.Fatalf("api 404: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}

	r = e.run("", "", "api", "/products/{product}")
	if r.code != cli.ExitUsage || !strings.Contains(r.stderr, "no product configured") {
		t.Fatalf("api without product: exit %d, stderr %q", r.code, r.stderr)
	}

	r = e.run("", "", "api", "POST", "/products", "--data", "{nope")
	if r.code != cli.ExitUsage {
		t.Fatalf("api invalid JSON: exit %d", r.code)
	}
}

func TestE2EJSONErrors(t *testing.T) {
	e := newE2E(t)
	e.env[cliconfig.EnvToken] = e.token
	r := e.run("", "", "whoami", "--server", e.serverURL, "--product", uuid.New().String(), "-o", "json")
	// An unknown product is reported in the output, not as a failure.
	if r.code != cli.ExitOK || !strings.Contains(r.stdout, `"product": null`) {
		t.Fatalf("whoami unknown product: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}

	e.env[cliconfig.EnvToken] = "revoked"
	r = e.run("", "", "whoami", "--server", e.serverURL, "-o", "json")
	if r.code != cli.ExitAPI {
		t.Fatalf("whoami bad token: exit %d", r.code)
	}
	body := decode[struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}](t, r.stderr)
	if body.Error.Code == "" {
		t.Fatalf("stderr = %q, want JSON error with code", r.stderr)
	}
}

func TestE2EUsage(t *testing.T) {
	e := newE2E(t)
	if r := e.run("", "", "frobnicate"); r.code != cli.ExitUsage {
		t.Fatalf("unknown command: exit %d", r.code)
	}
	if r := e.run("", "", "help", "login"); r.code != cli.ExitOK || !strings.Contains(r.stdout, "Usage: nexmap login") {
		t.Fatalf("help login: exit %d, %q", r.code, r.stdout)
	}
	if r := e.run("", "", "whoami", "-o", "yaml"); r.code != cli.ExitUsage {
		t.Fatalf("bad format: exit %d", r.code)
	}
	if r := e.run("", "", "whoami", "--server", "http://nexmap.example.com"); r.code != cli.ExitUsage || !strings.Contains(r.stderr, "--allow-http") {
		t.Fatalf("plain http: exit %d, stderr %q", r.code, r.stderr)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
