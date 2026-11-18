package router

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// TestScopedTwoFactorEnforcement drives the product and role rules of SC-34
// over HTTP: a product that requires 2FA keeps members without it out of
// itself only, tokens are exempt, and a role rule forces enrollment.
func TestScopedTwoFactorEnforcement(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()

		open, err := e.svc.Product.Create(ctx, e.admin, service.CreateProductInput{Name: "Open"})
		if err != nil {
			t.Fatal(err)
		}

		cookie := e.invite("vic", model.RoleViewer) // member of the strict product
		vic, err := e.st.User.GetByUsername(ctx, "vic")
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.Product.AddMember(ctx, e.admin, open.ID, vic.ID, model.RoleViewer); err != nil {
			t.Fatal(err)
		}

		// Set directly: the administrator has no 2FA, so could neither turn
		// the requirement on nor invite to the product afterwards.
		strict := e.product
		strict.TwoFactorRequired = true
		if err := e.st.Product.Update(ctx, strict); err != nil {
			t.Fatal(err)
		}

		do := func(path, bearer string) (*http.Response, string) {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if bearer != "" {
				req.Header.Set("Authorization", "Bearer "+bearer)
			} else {
				req.AddCookie(cookie)
			}
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp, readBody(t, resp)
		}

		setup := "/user/2fa/setup?product=" + strict.ID
		if resp, _ := do("/products/"+strict.ID+"/requirements", ""); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != setup {
			t.Errorf("strict product page: %d -> %q, want a redirect to %q", resp.StatusCode, resp.Header.Get("Location"), setup)
		}
		if resp, _ := do("/products/"+open.ID, ""); resp.StatusCode != http.StatusOK {
			t.Errorf("open product page: status %d", resp.StatusCode)
		}
		if resp, body := do("/api/v1/products/"+strict.ID, ""); resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "TOTP_SETUP_REQUIRED") {
			t.Errorf("strict product API: %d %s", resp.StatusCode, body)
		}
		if resp, body := do(setup, ""); resp.StatusCode != http.StatusOK || !strings.Contains(body, "The product Aviary requires two-factor authentication") {
			t.Errorf("setup page for the product: %d", resp.StatusCode)
		}
		if resp, body := do("/", ""); resp.StatusCode != http.StatusOK || !strings.Contains(body, "Requires two-factor authentication") {
			t.Errorf("dashboard does not mark the locked product: %d", resp.StatusCode)
		}
		// The status API explains why.
		if resp, body := do("/api/v1/user/me/totp", ""); resp.StatusCode != http.StatusOK ||
			!strings.Contains(body, `"enforced":true`) || !strings.Contains(body, strict.ID) {
			t.Errorf("TOTP status: %d %s", resp.StatusCode, body)
		}

		// Tokens are exempt.
		pat, err := e.svc.PAT.Create(ctx, vic, service.CreatePATInput{Name: "ci"})
		if err != nil {
			t.Fatal(err)
		}
		if resp, body := do("/api/v1/products/"+strict.ID, pat.RawToken); resp.StatusCode != http.StatusOK {
			t.Errorf("strict product with a token: %d %s", resp.StatusCode, body)
		}

		// A role rule sends the viewer to setup from everywhere.
		e.setting("2fa_enforced_viewer", "true")
		if resp, _ := do("/products/"+open.ID, ""); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/user/2fa/setup" {
			t.Errorf("viewer rule: %d -> %q", resp.StatusCode, resp.Header.Get("Location"))
		}
		if resp, body := do("/user/2fa/setup", ""); !strings.Contains(body, "required for viewers") {
			t.Errorf("setup page does not name the viewer rule: %d", resp.StatusCode)
		}
	})
}
