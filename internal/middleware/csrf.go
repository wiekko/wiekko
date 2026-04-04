package middleware

import (
	"crypto/subtle"
	"net/http"

	"nexmap.net/internal/httputil"
)

const csrfHeader = "X-CSRF-Token"

// CSRF validates the CSRF token for session-authenticated state-changing requests.
// PAT-authenticated requests are exempt (bearer tokens are inherently CSRF-safe).
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isStateChanging(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// PAT-authenticated requests are exempt.
		if AuthMethodFromContext(r.Context()) != "session" {
			next.ServeHTTP(w, r)
			return
		}

		expected := CSRFTokenFromContext(r.Context())
		provided := r.Header.Get(csrfHeader)
		if provided == "" {
			provided = r.FormValue("_csrf")
		}
		if expected == "" || provided == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
			httputil.Error(w, http.StatusForbidden, "CSRF_INVALID", "Invalid or missing CSRF token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
