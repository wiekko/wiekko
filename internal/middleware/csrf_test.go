// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
)

func TestCSRF_SafeMethods(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			called := false
			handler := middleware.CSRF(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				called = true
			}))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(method, "/", nil))
			if !called {
				t.Fatalf("handler not called for safe method %s", method)
			}
		})
	}
}

func TestCSRF_SessionAuth_ValidToken(t *testing.T) {
	called := false
	handler := middleware.CSRF(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := middleware.WithUser(r.Context(), &model.User{ID: "u1"}, "session", "csrf-secret")
	r = r.WithContext(ctx)
	r.Header.Set("X-CSRF-Token", "csrf-secret")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("handler was not called with valid CSRF token")
	}
}

func TestCSRF_SessionAuth_MissingToken(t *testing.T) {
	handler := middleware.CSRF(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := middleware.WithUser(r.Context(), &model.User{ID: "u1"}, "session", "csrf-secret")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestCSRF_SessionAuth_WrongToken(t *testing.T) {
	handler := middleware.CSRF(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := middleware.WithUser(r.Context(), &model.User{ID: "u1"}, "session", "csrf-secret")
	r = r.WithContext(ctx)
	r.Header.Set("X-CSRF-Token", "wrong-token")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestCSRF_PATAuth_Exempted(t *testing.T) {
	called := false
	handler := middleware.CSRF(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	ctx := middleware.WithUser(r.Context(), &model.User{ID: "u1"}, "pat", "")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("PAT-authenticated request should be CSRF-exempt")
	}
}

func TestCSRF_SessionAuth_ValidFormToken(t *testing.T) {
	called := false
	handler := middleware.CSRF(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	r := httptest.NewRequest(http.MethodPost, "/?_csrf=csrf-secret", nil)
	ctx := middleware.WithUser(r.Context(), &model.User{ID: "u1"}, "session", "csrf-secret")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("handler was not called with valid CSRF form token")
	}
}

func TestCSRF_StateChangingMethods(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			handler := middleware.CSRF(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				t.Fatal("handler should not be called without CSRF token")
			}))

			r := httptest.NewRequest(method, "/", nil)
			ctx := middleware.WithUser(r.Context(), &model.User{ID: "u1"}, "session", "csrf-secret")
			r = r.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d for method %s", w.Code, http.StatusForbidden, method)
			}
		})
	}
}
