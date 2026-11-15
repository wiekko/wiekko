package router

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// invite creates an account from an invitation (to the product with a role,
// or to the installation only if role is "") and returns its session cookie.
func (e *accountEnv) invite(username, role string) *http.Cookie {
	e.t.Helper()
	ctx := context.Background()
	productID := ""
	if role != "" {
		productID = e.product.ID
	}
	inv, err := e.svc.Invitation.Create(ctx, e.admin, username+"@example.com", productID, role)
	if err != nil {
		e.t.Fatalf("invite %s: %v", username, err)
	}
	if _, err := e.svc.User.AcceptInvitation(ctx, inv.RawToken, service.RegisterInput{
		Username: username, Password: goodPassword,
	}); err != nil {
		e.t.Fatalf("accept invitation for %s: %v", username, err)
	}
	resp, _ := e.post("/login", url.Values{"username": {username}, "password": {goodPassword}})
	for _, c := range resp.Cookies() {
		if c.Name == "nexmap_session" && c.Value != "" {
			return c
		}
	}
	e.t.Fatalf("no session for %s, status %d", username, resp.StatusCode)
	return nil
}

// TestUserSearchNeedsOwner checks that the user search behind "add member"
// does not hand the instance's user list to everyone who is signed in.
func TestUserSearchNeedsOwner(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		owner := e.invite("olga", model.RoleOwner)
		developer := e.invite("dev", model.RoleDeveloper)
		outsider := e.invite("mallory", "")

		search := func(cookie *http.Cookie, productID string) (*http.Response, string) {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, e.srv.URL+"/products/"+productID+"/team/search-users?q=ma", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(cookie)
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp, readBody(t, resp)
		}

		resp, body := search(owner, e.product.ID)
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, "mallory") {
			t.Fatalf("owner: status %d, body: %s", resp.StatusCode, body)
		}
		for name, cookie := range map[string]*http.Cookie{"developer": developer, "outsider": outsider} {
			for _, productID := range []string{e.product.ID, "00000000-0000-0000-0000-000000000000"} {
				resp, body := search(cookie, productID)
				if resp.StatusCode != http.StatusForbidden || strings.Contains(body, "mallory") {
					t.Errorf("%s on %s: status %d, body: %s", name, productID, resp.StatusCode, body)
				}
			}
		}
	})
}

// TestHardeningOverHTTP drives the request limits through the whole stack.
func TestHardeningOverHTTP(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)

		t.Run("oversized JSON body", func(t *testing.T) {
			body := `{"username":"x","password":"` + strings.Repeat("a", middleware.DefaultMaxBody) + `"}`
			resp, err := e.client().Post(e.srv.URL+"/api/v1/auth/login", "application/json", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			if got := readBody(t, resp); resp.StatusCode != http.StatusRequestEntityTooLarge {
				t.Fatalf("status = %d, body: %s", resp.StatusCode, got)
			}
		})

		// A web form over the cap gets a page saying so, not a CSRF error:
		// rejected on its Content-Length, or — sent chunked — when CSRF reads
		// the token.
		t.Run("oversized web form", func(t *testing.T) {
			resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
			var cookie *http.Cookie
			for _, c := range resp.Cookies() {
				if c.Name == "nexmap_session" && c.Value != "" {
					cookie = c
				}
			}
			if cookie == nil {
				t.Fatalf("no session cookie, status %d", resp.StatusCode)
			}
			form := url.Values{"_csrf": {csrfToken(t, e, cookie)}, "title": {"Big"}, "body": {strings.Repeat("a", middleware.DefaultMaxBody)}}.Encode()
			for _, chunked := range []bool{false, true} {
				req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/products/"+e.product.ID+"/documents", strings.NewReader(form))
				if err != nil {
					t.Fatal(err)
				}
				if chunked {
					req.ContentLength = -1
				}
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.AddCookie(cookie)
				resp, err := e.client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body := readBody(t, resp)
				if resp.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(body, "The limit is 1 MiB.") || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
					t.Errorf("chunked=%v: status %d, Content-Type %q, body %.200s", chunked, resp.StatusCode, resp.Header.Get("Content-Type"), body)
				}
			}

			resp, body := e.post("/login", url.Values{"username": {"root"}, "password": {strings.Repeat("a", middleware.DefaultMaxBody)}})
			if resp.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(body, "The limit is 1 MiB.") {
				t.Errorf("oversized login: status %d, body %.200s", resp.StatusCode, body)
			}
		})

		t.Run("security headers", func(t *testing.T) {
			resp, _ := e.get("/login")
			if resp.Header.Get("Content-Security-Policy") == "" {
				t.Error("no Content-Security-Policy on the login page")
			}
		})

		t.Run("login limit ignores a forged address", func(t *testing.T) {
			// The test client reaches the server from loopback, which the env
			// trusts as a proxy; a client behind it is told apart by the last
			// X-Forwarded-For entry, not by what it wrote in front.
			attempt := func(xff string) int {
				t.Helper()
				req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/login",
					strings.NewReader(url.Values{"username": {"ghost"}, "password": {"wrong"}}.Encode()))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("X-Forwarded-For", xff)
				resp, err := e.client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				_ = readBody(t, resp)
				return resp.StatusCode
			}
			for i := 0; i < 5; i++ {
				if got := attempt(fmt.Sprintf("198.51.100.%d, 203.0.113.9", i)); got != http.StatusUnprocessableEntity {
					t.Fatalf("attempt %d: status = %d, want 422", i+1, got)
				}
			}
			if got := attempt("198.51.100.99, 203.0.113.9"); got != http.StatusTooManyRequests {
				t.Fatalf("after 5 failures: status = %d, want 429", got)
			}
			if got := attempt("203.0.113.10"); got != http.StatusUnprocessableEntity {
				t.Fatalf("another client: status = %d, want 422", got)
			}
		})
	})
}
