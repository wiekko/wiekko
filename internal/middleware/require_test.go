package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
)

func TestRequireAuth_Authenticated(t *testing.T) {
	called := false
	handler := middleware.RequireAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	user := &model.User{ID: "u1"}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := middleware.WithUser(r.Context(), user, "session", "")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("handler was not called")
	}
}

func TestRequireAuth_Unauthenticated(t *testing.T) {
	handler := middleware.RequireAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	var envelope struct {
		Error struct{ Code string } `json:"error"`
	}
	json.NewDecoder(w.Body).Decode(&envelope)
	if envelope.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("code = %q, want %q", envelope.Error.Code, "UNAUTHORIZED")
	}
}

func TestRequireAdmin_AdminUser(t *testing.T) {
	called := false
	handler := middleware.RequireAdmin(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	user := &model.User{ID: "u1", IsAdmin: true}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := middleware.WithUser(r.Context(), user, "session", "")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("handler was not called")
	}
}

func TestRequireAdmin_NonAdmin(t *testing.T) {
	handler := middleware.RequireAdmin(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	user := &model.User{ID: "u1", IsAdmin: false}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := middleware.WithUser(r.Context(), user, "session", "")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireAdmin_Unauthenticated(t *testing.T) {
	handler := middleware.RequireAdmin(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
