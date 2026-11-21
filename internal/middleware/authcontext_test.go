// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware_test

import (
	"context"
	"testing"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
)

func TestWithUserRoundTrip(t *testing.T) {
	user := &model.User{ID: "u1", Username: "alice", IsAdmin: true}
	ctx := middleware.WithUser(context.Background(), user, "session", "csrf123")

	got := middleware.UserFromContext(ctx)
	if got == nil || got.ID != "u1" {
		t.Fatalf("UserFromContext = %v, want user u1", got)
	}
	if m := middleware.AuthMethodFromContext(ctx); m != "session" {
		t.Fatalf("AuthMethodFromContext = %q, want %q", m, "session")
	}
	if tok := middleware.CSRFTokenFromContext(ctx); tok != "csrf123" {
		t.Fatalf("CSRFTokenFromContext = %q, want %q", tok, "csrf123")
	}
}

func TestEmptyContext(t *testing.T) {
	ctx := context.Background()

	if u := middleware.UserFromContext(ctx); u != nil {
		t.Fatalf("UserFromContext on empty ctx = %v, want nil", u)
	}
	if m := middleware.AuthMethodFromContext(ctx); m != "" {
		t.Fatalf("AuthMethodFromContext on empty ctx = %q, want empty", m)
	}
	if tok := middleware.CSRFTokenFromContext(ctx); tok != "" {
		t.Fatalf("CSRFTokenFromContext on empty ctx = %q, want empty", tok)
	}
}
