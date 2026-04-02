package middleware

import (
	"context"
	"net/http"
	"strings"

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

// PATValidator validates a personal access token and returns the user.
type PATValidator interface {
	ValidatePAT(ctx context.Context, rawToken string) (*model.User, error)
}

// Authenticate extracts identity from a session cookie or Bearer token
// and stores it in the request context. Unauthenticated requests pass through.
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

			// Try Bearer token.
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, bearerPrefix) {
				rawToken := strings.TrimPrefix(authHeader, bearerPrefix)
				if user, err := pats.ValidatePAT(ctx, rawToken); err == nil {
					ctx = WithUser(ctx, user, "pat", "")
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Unauthenticated — pass through.
			next.ServeHTTP(w, r)
		})
	}
}
