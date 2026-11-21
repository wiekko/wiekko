// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"wiekko.net/internal/middleware"
)

func TestChain(t *testing.T) {
	t.Run("empty chain", func(t *testing.T) {
		called := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})
		chain := middleware.Chain()
		w := httptest.NewRecorder()
		chain(handler).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if !called {
			t.Fatal("handler was not called")
		}
	})

	t.Run("execution order", func(t *testing.T) {
		var order []string
		mw := func(name string) func(http.Handler) http.Handler {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					order = append(order, name)
					next.ServeHTTP(w, r)
				})
			}
		}
		handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		})

		chain := middleware.Chain(mw("first"), mw("second"), mw("third"))
		w := httptest.NewRecorder()
		chain(handler).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		expected := []string{"first", "second", "third", "handler"}
		if len(order) != len(expected) {
			t.Fatalf("order = %v, want %v", order, expected)
		}
		for i, v := range expected {
			if order[i] != v {
				t.Fatalf("order[%d] = %q, want %q", i, order[i], v)
			}
		}
	})
}
