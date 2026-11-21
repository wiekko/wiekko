// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware

import (
	"fmt"
	"net/http"

	"nexmap.net/internal/access"
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

// RequireAdminPage redirects non-admin page requests.
// Unauthenticated users go to /login; authenticated non-admins go to /.
func RequireAdminPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !user.IsAdmin {
			http.Redirect(w, r, "/", http.StatusSeeOther)
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

// RequireScopes rejects token-authenticated requests whose token lacks one of
// the given scopes (SC-54) with 403 TOKEN_SCOPE. Session requests are not
// restricted and pass through.
func RequireScopes(need ...access.Need) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if n, missing := access.FromContext(r.Context()).Missing(need); missing {
				httputil.Error(w, http.StatusForbidden, "TOKEN_SCOPE",
					fmt.Sprintf("This token lacks the %s scope (%s)", n.Scope, n.Level))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
