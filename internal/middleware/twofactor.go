package middleware

import (
	"context"
	"net/http"
	"strings"

	"nexmap.net/internal/httputil"
)

// TwoFactorPolicy reports whether the instance requires two-factor authentication.
type TwoFactorPolicy interface {
	Enforced(ctx context.Context) bool
}

// TwoFactorSetupPath is where page requests are sent while setup is outstanding.
const TwoFactorSetupPath = "/user/2fa/setup"

// RequireTwoFactorEnrollment keeps session users who have not set up TOTP
// away from everything but the setup flow while the instance enforces 2FA
// (SC-107). Page requests are redirected to the setup page; API requests get
// 403 TOTP_SETUP_REQUIRED. PAT-authenticated requests are not affected: a
// token is a separate credential, and the setting cannot invalidate tokens
// issued before it was turned on.
func RequireTwoFactorEnrollment(policy TwoFactorPolicy) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil || user.TOTPEnabled || AuthMethodFromContext(r.Context()) != "session" ||
				twoFactorExempt(r) || !policy.Enforced(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}

			if strings.HasPrefix(r.URL.Path, "/api/") {
				httputil.Error(w, http.StatusForbidden, "TOTP_SETUP_REQUIRED",
					"Two-factor authentication is required; set it up via /api/v1/user/me/totp")
				return
			}
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", TwoFactorSetupPath)
				w.WriteHeader(http.StatusOK)
				return
			}
			http.Redirect(w, r, TwoFactorSetupPath, http.StatusSeeOther)
		})
	}
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
		p == "/api/v1/user/me" && r.Method == http.MethodGet:
		return true
	}
	return false
}
