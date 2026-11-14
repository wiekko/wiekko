package router

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/auth/builtin"
	"nexmap.net/internal/config"
	"nexmap.net/internal/crypto"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/mail"
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
	return newAccountEnvWithMail(t, driver, db, nil)
}

// newAccountEnvWithMail is newAccountEnv sending email through sender (nil:
// none), with links to accountEnvURL.
func newAccountEnvWithMail(t *testing.T, driver string, db *sql.DB, sender mail.Sender) *accountEnv {
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
	if sender != nil {
		cfg.PublicURL = accountEnvURL
	}
	st := store.New(db, store.Dialect(driver), keys)
	svc := service.NewWithMail(st, builtin.New(st.User, cfg.PBKDF2Iterations), cfg, blobs, sender)

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	adminHash, err := builtin.New(st.User, cfg.PBKDF2Iterations).HashPassword(goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	admin := &model.User{
		ID: uuid.New().String(), Username: "root", Email: "root@localhost", DisplayName: "Root",
		PasswordHash: adminHash, IsAdmin: true, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := st.User.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	product, err := svc.Product.Create(ctx, admin, service.CreateProductInput{Name: "Aviary"})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	handler, err := New(svc, Options{
		MaxUploadBytes: 1 << 20,
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
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

// TestPasswordChangeEndsOtherSessions signs in twice, changes the password
// from the first session and checks that only the second one was ended.
func TestPasswordChangeEndsOtherSessions(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		inv, err := e.svc.Invitation.Create(ctx, e.admin, "pat@example.com", e.product.ID, model.RoleTester)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.User.AcceptInvitation(ctx, inv.RawToken, service.RegisterInput{
			Username: "pat", Password: goodPassword,
		}); err != nil {
			t.Fatalf("accept invitation: %v", err)
		}

		signIn := func() *http.Cookie {
			t.Helper()
			resp, _ := e.post("/login", url.Values{"username": {"pat"}, "password": {goodPassword}})
			for _, c := range resp.Cookies() {
				if c.Name == "nexmap_session" && c.Value != "" {
					return c
				}
			}
			t.Fatalf("no session cookie, status %d", resp.StatusCode)
			return nil
		}
		first, second := signIn(), signIn()

		do := func(method, path string, cookie *http.Cookie, form url.Values) (*http.Response, string) {
			t.Helper()
			req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(cookie)
			if method != http.MethodGet {
				req.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
			}
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp, readBody(t, resp)
		}

		newPassword := "Battery-Staple-7"
		resp, body := do(http.MethodPost, "/user/profile/password", first, url.Values{
			"old_password": {goodPassword}, "new_password": {newPassword}, "confirm_password": {newPassword},
		})
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, "signed out on all other devices") {
			t.Fatalf("change password: status %d, body: %s", resp.StatusCode, body)
		}

		if resp, _ := do(http.MethodGet, "/user/profile", first, nil); resp.StatusCode != http.StatusOK {
			t.Errorf("current session: status %d, want 200", resp.StatusCode)
		}
		if resp, _ := do(http.MethodGet, "/user/profile", second, nil); resp.StatusCode != http.StatusSeeOther {
			t.Errorf("other session: status %d, want a redirect to sign in", resp.StatusCode)
		}
	})
}

var csrfTokenPattern = regexp.MustCompile(`X-CSRF-Token(?:&#34;|&quot;|")\s*:\s*(?:&#34;|&quot;|")([^&"]+)`)

// csrfToken reads the CSRF token of the session from a page, where the app
// layout hands it to htmx.
func csrfToken(t *testing.T, e *accountEnv, cookie *http.Cookie) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.srv.URL+"/user/profile", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	resp, err := e.client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	m := csrfTokenPattern.FindStringSubmatch(readBody(t, resp))
	if m == nil {
		t.Fatal("no CSRF token on /user/profile")
	}
	return m[1]
}

var pageRoutePattern = regexp.MustCompile(`mux\.Handle\("(POST|PUT|PATCH|DELETE) ([^"]+)",\s*(\w+)\(`)

// TestPageRoutesRequireCSRF sends every state-changing page route a request
// from a signed-in administrator without a CSRF token, and expects the CSRF
// check to refuse it. Routes are read from pages.go, so a new route that
// skips the check fails here. (The public routes — sign-in, registration —
// are registered with HandleFunc and have no session to protect.)
func TestPageRoutesRequireCSRF(t *testing.T) {
	src, err := os.ReadFile("pages.go")
	if err != nil {
		t.Fatal(err)
	}
	routes := pageRoutePattern.FindAllStringSubmatch(string(src), -1)
	if len(routes) < 100 {
		t.Fatalf("found only %d routes in pages.go; is the pattern still right?", len(routes))
	}
	placeholder := regexp.MustCompile(`\{[^}]+\}`)

	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		hash, err := builtin.New(e.st.User, 1).HashPassword(goodPassword)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if err := e.st.User.Create(ctx, &model.User{
			ID: uuid.New().String(), Username: "ops", Email: "ops@example.com", DisplayName: "Ops",
			PasswordHash: hash, IsAdmin: true, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		resp, _ := e.post("/login", url.Values{"username": {"ops"}, "password": {goodPassword}})
		var session *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "nexmap_session" {
				session = c
			}
		}
		if session == nil {
			t.Fatalf("sign-in failed: status %d", resp.StatusCode)
		}

		// Upload routes read the multipart body before the CSRF check (see
		// pageUpload), so they get one; anything else is refused before that.
		send := func(method, path string, form url.Values, header map[string]string, multi bool) (*http.Response, string) {
			t.Helper()
			var body bytes.Buffer
			contentType := "application/x-www-form-urlencoded"
			if multi {
				mw := multipart.NewWriter(&body)
				for k, vs := range form {
					for _, v := range vs {
						_ = mw.WriteField(k, v)
					}
				}
				_ = mw.Close()
				contentType = mw.FormDataContentType()
			} else {
				body.WriteString(form.Encode())
			}
			req, err := http.NewRequest(method, e.srv.URL+path, &body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", contentType)
			for k, v := range header {
				req.Header.Set(k, v)
			}
			req.AddCookie(session)
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp, readBody(t, resp)
		}

		for _, m := range routes {
			method, path, multi := m[1], placeholder.ReplaceAllString(m[2], "x"), m[3] == "upload"
			resp, body := send(method, path, url.Values{"name": {"x"}}, nil, multi)
			if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "CSRF_INVALID") {
				t.Errorf("%s %s without token: status %d, want 403 CSRF_INVALID", m[1], m[2], resp.StatusCode)
			}
			resp, body = send(method, path, url.Values{"_csrf": {"wrong"}}, map[string]string{"X-CSRF-Token": "wrong"}, multi)
			if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "CSRF_INVALID") {
				t.Errorf("%s %s with a wrong token: status %d, want 403 CSRF_INVALID", m[1], m[2], resp.StatusCode)
			}
		}

		// The real token in a plain form's _csrf field passes (sign-out form).
		token := csrfToken(t, e, session)
		resp, _ = send(http.MethodPost, "/logout", url.Values{"_csrf": {token}}, nil, false)
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
			t.Errorf("logout with token: status %d, location %q", resp.StatusCode, resp.Header.Get("Location"))
		}
	})
}
