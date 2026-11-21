// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nexmap.net/internal/middleware"
)

func TestRequestID_Generated(t *testing.T) {
	var ctxID string
	handler := middleware.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxID = middleware.RequestIDFromContext(r.Context())
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if ctxID == "" {
		t.Fatal("context request ID is empty")
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("response X-Request-ID header is empty")
	}
	if ctxID != w.Header().Get("X-Request-ID") {
		t.Fatalf("context ID %q != response header %q", ctxID, w.Header().Get("X-Request-ID"))
	}
}

func TestRequestID_Propagated(t *testing.T) {
	var ctxID string
	handler := middleware.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxID = middleware.RequestIDFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Request-ID", "my-request-id")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if ctxID != "my-request-id" {
		t.Fatalf("context ID = %q, want %q", ctxID, "my-request-id")
	}
	if w.Header().Get("X-Request-ID") != "my-request-id" {
		t.Fatalf("response header = %q, want %q", w.Header().Get("X-Request-ID"), "my-request-id")
	}
}
