package middleware

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/model"
)

// TwoFactorPolicy decides who has to use two-factor authentication (SC-107).
type TwoFactorPolicy interface {
	// RequiredFor reports whether an instance-level rule requires the user
	// to set up 2FA before using Nexmap at all.
	RequiredFor(ctx context.Context, user *model.User) bool
	// ProductLocked reports whether the product requires 2FA of its members
	// and keeps the user, a member without 2FA, out.
	ProductLocked(ctx context.Context, user *model.User, productID string) bool
}

// TwoFactorSetupPath is where page requests are sent while setup is outstanding.
const TwoFactorSetupPath = "/user/2fa/setup"

// RequireTwoFactorEnrollment keeps session users who have not set up TOTP
// away from everything but the setup flow while an instance-level rule
// requires 2FA of them (SC-107). Page requests are redirected to the setup
// page; API requests get 403 TOTP_SETUP_REQUIRED.
//
// Pages of a product that requires 2FA are redirected to the setup page as
// well, naming the product; other products stay reachable. (API requests to
// such a product are refused by the service layer, see
// service.ErrTwoFactorRequired.)
//
// PAT-authenticated requests are not affected: a token is a separate
// credential, and the settings cannot invalidate tokens issued before they
// were turned on.
func RequireTwoFactorEnrollment(policy TwoFactorPolicy) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil || user.TOTPEnabled || AuthMethodFromContext(r.Context()) != "session" ||
				twoFactorExempt(r) {
				next.ServeHTTP(w, r)
				return
			}

			if policy.RequiredFor(r.Context(), user) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					httputil.Error(w, http.StatusForbidden, "TOTP_SETUP_REQUIRED",
						"Two-factor authentication is required; set it up via /api/v1/user/me/totp")
					return
				}
				redirectToSetup(w, r, TwoFactorSetupPath)
				return
			}

			if id := productPageID(r.URL.Path); id != "" && policy.ProductLocked(r.Context(), user, id) {
				redirectToSetup(w, r, TwoFactorSetupPath+"?product="+url.QueryEscape(id))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func redirectToSetup(w http.ResponseWriter, r *http.Request, target string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// productPageID returns the product ID of a product page path
// (/products/{id} or /products/{id}/...), or "" for any other path.
func productPageID(p string) string {
	rest, ok := strings.CutPrefix(p, "/products/")
	if !ok {
		return ""
	}
	id, _, _ := strings.Cut(rest, "/")
	switch id {
	case "", "new", "switcher":
		return ""
	}
	return id
}

// twoFactorExempt lists what a user must still reach to complete setup or leave.
func twoFactorExempt(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/static/"),
		strings.HasPrefix(p, "/user/2fa/"),
		strings.HasPrefix(p, "/api/v1/user/me/totp"),
		p == "/logout",
		p == "/login",
		p == "/api/v1/auth/logout",
		p == "/api/v1/health",
		p == "/api/v1/version",
		p == "/api/v1/user/me" && r.Method == http.MethodGet:
		return true
	}
	return false
}
