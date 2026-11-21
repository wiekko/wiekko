// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware

import (
	"context"

	"wiekko.net/internal/model"
)

var (
	userKey       = &contextKey{"user"}
	authMethodKey = &contextKey{"authMethod"}
	csrfTokenKey  = &contextKey{"csrfToken"}
)

// WithUser stores the authenticated user in the context.
func WithUser(ctx context.Context, user *model.User, method string, csrfToken string) context.Context {
	ctx = context.WithValue(ctx, userKey, user)
	ctx = context.WithValue(ctx, authMethodKey, method)
	ctx = context.WithValue(ctx, csrfTokenKey, csrfToken)
	return ctx
}

// UserFromContext returns the authenticated user, or nil.
func UserFromContext(ctx context.Context) *model.User {
	u, _ := ctx.Value(userKey).(*model.User)
	return u
}

// AuthMethodFromContext returns "session", "pat", or "".
func AuthMethodFromContext(ctx context.Context) string {
	m, _ := ctx.Value(authMethodKey).(string)
	return m
}

// CSRFTokenFromContext returns the session CSRF token, or "".
func CSRFTokenFromContext(ctx context.Context) string {
	t, _ := ctx.Value(csrfTokenKey).(string)
	return t
}
