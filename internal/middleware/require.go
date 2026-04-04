package middleware

import (
	"net/http"

	"nexmap.net/internal/httputil"
)

// RequireAuth rejects unauthenticated requests with 401.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFromContext(r.Context()) == nil {
			httputil.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuthPage redirects unauthenticated requests to the login page.
// Used for page routes (as opposed to RequireAuth which returns JSON 401).
func RequireAuthPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFromContext(r.Context()) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin rejects non-admin requests with 403.
// Must be used after RequireAuth.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			httputil.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
			return
		}
		if !user.IsAdmin {
			httputil.Error(w, http.StatusForbidden, "FORBIDDEN", "Administrator access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
