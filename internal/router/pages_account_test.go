package router

import (
	"context"
	"database/sql"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/auth/builtin"
	"nexmap.net/internal/config"
	"nexmap.net/internal/crypto"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

// accountEnv is a server on a real database with one administrator and one
// product, for driving the public account pages over HTTP.
type accountEnv struct {
	t       *testing.T
	srv     *httptest.Server
	svc     *service.Services
	st      *store.Store
	admin   *model.User
	product *model.Product
}

func newAccountEnv(t *testing.T, driver string, db *sql.DB) *accountEnv {
	t.Helper()
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
	st := store.New(db, store.Dialect(driver), keys)
	svc := service.New(st, builtin.New(st.User, cfg.PBKDF2Iterations), cfg, blobs)

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	admin := &model.User{
		ID: uuid.New().String(), Username: "root", Email: "root@localhost", DisplayName: "Root",
		PasswordHash: "x", IsAdmin: true, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := st.User.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	product, err := svc.Product.Create(ctx, admin, service.CreateProductInput{Name: "Aviary"})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	srv := httptest.NewServer(New(svc, false, false, 1<<20))
	t.Cleanup(srv.Close)
	return &accountEnv{t: t, srv: srv, svc: svc, st: st, admin: admin, product: product}
}

func (e *accountEnv) setting(key, value string) {
	e.t.Helper()
	if err := e.st.Setting.Set(context.Background(), key, value); err != nil {
		e.t.Fatalf("set %s: %v", key, err)
	}
}

// get and post do not follow redirects, so tests see where they point.
func (e *accountEnv) client() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (e *accountEnv) get(path string) (*http.Response, string) {
	e.t.Helper()
	resp, err := e.client().Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	return resp, readBody(e.t, resp)
}

func (e *accountEnv) post(path string, form url.Values) (*http.Response, string) {
	e.t.Helper()
	resp, err := e.client().PostForm(e.srv.URL+path, form)
	if err != nil {
		e.t.Fatal(err)
	}
	return resp, readBody(e.t, resp)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hasSession(resp *http.Response) bool {
	for _, c := range resp.Cookies() {
		if c.Name == "nexmap_session" && c.Value != "" {
			return true
		}
	}
	return false
}

func accountForm(username, email, password string) url.Values {
	return url.Values{
		"username":         {username},
		"email":            {email},
		"password":         {password},
		"password_confirm": {password},
	}
}

const goodPassword = "Correct-Horse-9"

func TestAcceptInvitePage(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		inv, err := e.svc.Invitation.Create(ctx, e.admin, "dana@example.com", e.product.ID, model.RoleTester)
		if err != nil {
			t.Fatalf("create invitation: %v", err)
		}
		path := "/accept-invite/" + inv.RawToken

		resp, body := e.get(path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET status = %d, body: %s", resp.StatusCode, body)
		}
		for _, want := range []string{"Aviary", "tester", "dana@example.com"} {
			if !strings.Contains(body, want) {
				t.Errorf("page does not mention %q", want)
			}
		}

		// Mistyped confirmation: form again, nothing created.
		form := accountForm("dana", "", goodPassword)
		form.Set("password_confirm", "something else")
		resp, body = e.post(path, form)
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "do not match") {
			t.Fatalf("mismatch: status = %d, body: %s", resp.StatusCode, body)
		}

		// The email field is disabled in the form, so it is not submitted.
		resp, body = e.post(path, accountForm("dana", "", goodPassword))
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
			t.Fatalf("accept: status = %d, location %q, body: %s", resp.StatusCode, resp.Header.Get("Location"), body)
		}
		if !hasSession(resp) {
			t.Fatal("accepting did not sign the new user in")
		}

		user, err := e.st.User.GetByUsername(ctx, "dana")
		if err != nil {
			t.Fatalf("new user: %v", err)
		}
		if user.Email != "dana@example.com" || user.Status != model.UserStatusActive {
			t.Fatalf("user = %+v, want active with the invited address", user)
		}
		role, err := e.st.ProductRole.GetByUserAndProduct(ctx, user.ID, e.product.ID)
		if err != nil || role.Role != model.RoleTester {
			t.Fatalf("product role = %+v, %v; want tester", role, err)
		}

		// The link works once.
		resp, body = e.get(path)
		if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "already been used") {
			t.Fatalf("reuse: status = %d, body: %s", resp.StatusCode, body)
		}
		resp, _ = e.get("/accept-invite/not-a-token")
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("unknown token: status = %d", resp.StatusCode)
		}
	})
}

func TestRegisterPage(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)

		t.Run("closed by default", func(t *testing.T) {
			resp, body := e.get("/register")
			if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "Registration is closed") {
				t.Fatalf("status = %d, body: %s", resp.StatusCode, body)
			}
			resp, _ = e.post("/register", accountForm("eve", "eve@example.com", goodPassword))
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("POST status = %d", resp.StatusCode)
			}
		})

		e.setting("self_registration_enabled", "true")

		t.Run("open", func(t *testing.T) {
			resp, body := e.get("/register")
			if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Create your account") {
				t.Fatalf("status = %d, body: %s", resp.StatusCode, body)
			}
			resp, _ = e.post("/register", accountForm("eve", "eve@example.com", goodPassword))
			if resp.StatusCode != http.StatusSeeOther || !hasSession(resp) {
				t.Fatalf("status = %d, session %v", resp.StatusCode, hasSession(resp))
			}
		})

		t.Run("taken username", func(t *testing.T) {
			resp, body := e.post("/register", accountForm("eve", "other@example.com", goodPassword))
			if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "already taken") {
				t.Fatalf("status = %d, body: %s", resp.StatusCode, body)
			}
		})

		t.Run("weak password", func(t *testing.T) {
			resp, body := e.post("/register", accountForm("frank", "frank@example.com", "short"))
			if resp.StatusCode != http.StatusUnprocessableEntity || strings.Contains(body, "could not be created") {
				t.Fatalf("want the policy message, got status = %d, body: %s", resp.StatusCode, body)
			}
		})

		t.Run("moderated", func(t *testing.T) {
			e.setting("moderation_enabled", "true")
			resp, body := e.post("/register", accountForm("gina", "gina@example.com", goodPassword))
			if resp.StatusCode != http.StatusOK || !strings.Contains(body, "approve") || hasSession(resp) {
				t.Fatalf("status = %d, session %v, body: %s", resp.StatusCode, hasSession(resp), body)
			}
		})
	})
}

func TestLoginRateLimit(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		wrong := url.Values{"username": {"ghost"}, "password": {"wrong"}}

		for i := 0; i < 5; i++ {
			resp, _ := e.post("/login", wrong)
			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("attempt %d: status = %d, want 422", i+1, resp.StatusCode)
			}
		}

		resp, body := e.post("/login", wrong)
		if resp.StatusCode != http.StatusTooManyRequests || !strings.Contains(body, "Too many failed sign-in attempts") {
			t.Fatalf("web: status = %d, body: %s", resp.StatusCode, body)
		}
		if resp.Header.Get("Retry-After") == "" {
			t.Error("web: no Retry-After header")
		}

		// The API shares the counters.
		apiResp, err := e.client().Post(e.srv.URL+"/api/v1/auth/login", "application/json",
			strings.NewReader(`{"username":"ghost","password":"wrong"}`))
		if err != nil {
			t.Fatal(err)
		}
		apiBody := readBody(t, apiResp)
		if apiResp.StatusCode != http.StatusTooManyRequests || !strings.Contains(apiBody, "TOO_MANY_ATTEMPTS") {
			t.Fatalf("api: status = %d, body: %s", apiResp.StatusCode, apiBody)
		}
	})
}
