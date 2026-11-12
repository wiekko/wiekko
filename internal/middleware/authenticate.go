package middleware

import (
	"context"
	"net/http"
	"strings"

	"nexmap.net/internal/access"
	"nexmap.net/internal/model"
)

const (
	sessionCookieName = "nexmap_session"
	bearerPrefix      = "Bearer "
)

// SessionValidator validates a session ID and returns the user and CSRF token.
type SessionValidator interface {
	ValidateSession(ctx context.Context, sessionID string) (*model.User, string, error)
}

// PATValidator validates a personal access token and returns the user and the
// token's restriction.
type PATValidator interface {
	ValidatePAT(ctx context.Context, rawToken string) (*model.User, *access.Restriction, error)
}

// apiPrefix is where personal access tokens are accepted. Page routes only
// accept sessions: the web UI is not covered by token scopes, and its CSRF
// protection exempts non-session requests.
const apiPrefix = "/api/"

// Authenticate extracts identity from a session cookie or Bearer token
// and stores it in the request context. Bearer tokens are honoured only for
// API routes. Unauthenticated requests pass through.
func Authenticate(sessions SessionValidator, pats PATValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Try session cookie first.
			if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
				if user, csrfToken, err := sessions.ValidateSession(ctx, cookie.Value); err == nil {
					ctx = WithUser(ctx, user, "session", csrfToken)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Try Bearer token (API routes only).
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, bearerPrefix) &&
				strings.HasPrefix(r.URL.Path, apiPrefix) {
				rawToken := strings.TrimPrefix(authHeader, bearerPrefix)
				if user, restriction, err := pats.ValidatePAT(ctx, rawToken); err == nil {
					ctx = WithUser(ctx, user, "pat", "")
					ctx = access.WithRestriction(ctx, restriction)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Unauthenticated — pass through.
			next.ServeHTTP(w, r)
		})
	}
}
