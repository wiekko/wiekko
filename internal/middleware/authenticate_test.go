package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
)

type mockSessionValidator struct {
	fn func(ctx context.Context, sessionID string) (*model.User, string, error)
}

func (m *mockSessionValidator) ValidateSession(ctx context.Context, sessionID string) (*model.User, string, error) {
	return m.fn(ctx, sessionID)
}

type mockPATValidator struct {
	fn func(ctx context.Context, rawToken string) (*model.User, error)
}

func (m *mockPATValidator) ValidatePAT(ctx context.Context, rawToken string) (*model.User, error) {
	return m.fn(ctx, rawToken)
}

func TestAuthenticate_SessionCookie(t *testing.T) {
	user := &model.User{ID: "u1", Username: "alice"}
	sessions := &mockSessionValidator{
		fn: func(_ context.Context, id string) (*model.User, string, error) {
			if id == "valid-session" {
				return user, "csrf-tok", nil
			}
			return nil, "", errors.New("invalid")
		},
	}
	pats := &mockPATValidator{
		fn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("invalid")
		},
	}

	var ctxUser *model.User
	var ctxMethod string
	var ctxCSRF string
	handler := middleware.Authenticate(sessions, pats)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxUser = middleware.UserFromContext(r.Context())
		ctxMethod = middleware.AuthMethodFromContext(r.Context())
		ctxCSRF = middleware.CSRFTokenFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "nexmap_session", Value: "valid-session"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if ctxUser == nil || ctxUser.ID != "u1" {
		t.Fatalf("user = %v, want u1", ctxUser)
	}
	if ctxMethod != "session" {
		t.Fatalf("method = %q, want %q", ctxMethod, "session")
	}
	if ctxCSRF != "csrf-tok" {
		t.Fatalf("csrf = %q, want %q", ctxCSRF, "csrf-tok")
	}
}

func TestAuthenticate_BearerToken(t *testing.T) {
	user := &model.User{ID: "u2", Username: "bob"}
	sessions := &mockSessionValidator{
		fn: func(_ context.Context, _ string) (*model.User, string, error) {
			return nil, "", errors.New("invalid")
		},
	}
	pats := &mockPATValidator{
		fn: func(_ context.Context, tok string) (*model.User, error) {
			if tok == "nxm_valid" {
				return user, nil
			}
			return nil, errors.New("invalid")
		},
	}

	var ctxUser *model.User
	var ctxMethod string
	handler := middleware.Authenticate(sessions, pats)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxUser = middleware.UserFromContext(r.Context())
		ctxMethod = middleware.AuthMethodFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer nxm_valid")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if ctxUser == nil || ctxUser.ID != "u2" {
		t.Fatalf("user = %v, want u2", ctxUser)
	}
	if ctxMethod != "pat" {
		t.Fatalf("method = %q, want %q", ctxMethod, "pat")
	}
}

func TestAuthenticate_CookiePrecedence(t *testing.T) {
	sessionUser := &model.User{ID: "session-user"}
	sessions := &mockSessionValidator{
		fn: func(_ context.Context, _ string) (*model.User, string, error) {
			return sessionUser, "csrf", nil
		},
	}
	pats := &mockPATValidator{
		fn: func(_ context.Context, _ string) (*model.User, error) {
			return &model.User{ID: "pat-user"}, nil
		},
	}

	var ctxUser *model.User
	handler := middleware.Authenticate(sessions, pats)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxUser = middleware.UserFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "nexmap_session", Value: "sess"})
	r.Header.Set("Authorization", "Bearer nxm_tok")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if ctxUser == nil || ctxUser.ID != "session-user" {
		t.Fatalf("user = %v, want session-user (cookie should take precedence)", ctxUser)
	}
}

func TestAuthenticate_Unauthenticated(t *testing.T) {
	sessions := &mockSessionValidator{
		fn: func(_ context.Context, _ string) (*model.User, string, error) {
			return nil, "", errors.New("invalid")
		},
	}
	pats := &mockPATValidator{
		fn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("invalid")
		},
	}

	var ctxUser *model.User
	handler := middleware.Authenticate(sessions, pats)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxUser = middleware.UserFromContext(r.Context())
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if ctxUser != nil {
		t.Fatalf("user = %v, want nil (unauthenticated)", ctxUser)
	}
}
