// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"wiekko.net/internal/middleware"
	"wiekko.net/internal/model"
)

const tooLargeText = "That is too large to send. The limit is 1 MiB."

func oversizedForm() string {
	return url.Values{"_csrf": {"csrf-secret"}, "body": {strings.Repeat("a", middleware.DefaultMaxBody)}}.Encode()
}

// TestLimitBody_DeclaredLengthOverCap rejects a body by its Content-Length,
// before anything reads it, in the form each kind of client can show.
func TestLimitBody_DeclaredLengthOverCap(t *testing.T) {
	for _, tc := range []struct {
		name, path, hx, contentType, want string
	}{
		{"api", "/api/v1/products", "", "application/json", `"code":"TOO_LARGE"`},
		{"htmx", "/products/p/comments", "true", "text/plain", tooLargeText},
		{"form post", "/products/p/documents", "", "text/html", tooLargeText},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := middleware.LimitBody(10 << 20)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("handler called for an oversized body")
			}))
			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(oversizedForm()))
			if tc.hx != "" {
				r.Header.Set("HX-Request", tc.hx)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status = %d, want 413", w.Code)
			}
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.contentType) {
				t.Errorf("Content-Type = %q, want %s", ct, tc.contentType)
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("body %q does not contain %q", w.Body.String(), tc.want)
			}
		})
	}
}

func TestLimitBody_UnderCapPasses(t *testing.T) {
	called := false
	h := middleware.LimitBody(10 << 20)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		if b, err := io.ReadAll(r.Body); err != nil || string(b) != "hello" {
			t.Errorf("body = %q, %v", b, err)
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/products/p/documents", strings.NewReader("hello")))
	if !called {
		t.Fatal("handler not called")
	}
}

// TestLimitBody_ChunkedOverCap: without a Content-Length the cap trips while
// reading.
func TestLimitBody_ChunkedOverCap(t *testing.T) {
	h := middleware.LimitBody(10 << 20)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); !middleware.IsTooLarge(err) {
			t.Errorf("read error = %v, want *http.MaxBytesError", err)
		}
	}))
	r := httptest.NewRequest(http.MethodPost, "/products/p/documents", strings.NewReader(oversizedForm()))
	r.ContentLength = -1
	h.ServeHTTP(httptest.NewRecorder(), r)
}

// TestLimitBody_WebUploadsLeftToRoute: the web attachment routes cap the body
// themselves.
func TestLimitBody_WebUploadsLeftToRoute(t *testing.T) {
	called := false
	h := middleware.LimitBody(10 << 20)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/products/p/milestones/m/attachments", strings.NewReader(oversizedForm())))
	if !called {
		t.Fatal("handler not called for a web upload route")
	}
}

func sessionPost(path, contentType, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	return r.WithContext(middleware.WithUser(r.Context(), &model.User{ID: "u1"}, "session", "csrf-secret"))
}

// TestCSRF_BodyOverCap: a body that only turns out too large while CSRF reads
// the token is reported as 413, not as a missing token.
func TestCSRF_BodyOverCap(t *testing.T) {
	h := middleware.LimitBody(10 << 20)(middleware.CSRF(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler called for an oversized body")
	})))
	for _, tc := range []struct{ name, contentType, body string }{
		{"urlencoded", "application/x-www-form-urlencoded", oversizedForm()},
		{"multipart", "multipart/form-data; boundary=b", "--b\r\nContent-Disposition: form-data; name=\"body\"\r\n\r\n" + strings.Repeat("a", middleware.DefaultMaxBody) + "\r\n--b--\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := sessionPost("/products/p/documents", tc.contentType, tc.body)
			r.ContentLength = -1
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), tooLargeText) {
				t.Fatalf("status = %d, body %q; want 413 naming the limit", w.Code, w.Body.String())
			}
		})
	}
}

func TestCSRF_UnreadableBody(t *testing.T) {
	h := middleware.CSRF(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler called for an unreadable body")
	}))
	r := sessionPost("/products/p/documents", "multipart/form-data; boundary=b", "--b\r\nContent-Disposition: form-data; name=\"_csrf\"\r\n\r\ncsrf-se")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCSRF_FormToken(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		want                    int
	}{
		{"urlencoded valid", "application/x-www-form-urlencoded", "_csrf=csrf-secret", http.StatusOK},
		{"urlencoded wrong", "application/x-www-form-urlencoded", "_csrf=nope", http.StatusForbidden},
		{"multipart valid", "multipart/form-data; boundary=b", "--b\r\nContent-Disposition: form-data; name=\"_csrf\"\r\n\r\ncsrf-secret\r\n--b--\r\n", http.StatusOK},
		{"no body", "", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := middleware.CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, sessionPost("/products/p/documents", tc.contentType, tc.body))
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}
