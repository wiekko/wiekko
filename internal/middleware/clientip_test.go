// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"wiekko.net/internal/middleware"
)

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("127.0.0.0/8")}

	tests := []struct {
		name   string
		remote string
		xff    []string // one entry per header line
		want   string
	}{
		{"direct, no header", "203.0.113.9:4711", nil, "203.0.113.9"},
		{"direct client cannot claim an address", "203.0.113.9:4711", []string{"198.51.100.1"}, "203.0.113.9"},
		{"proxy without header", "10.0.0.2:80", nil, "10.0.0.2"},
		{"proxy passes the client on", "10.0.0.2:80", []string{"203.0.113.9"}, "203.0.113.9"},
		{"client-supplied entries are ignored", "10.0.0.2:80", []string{"1.2.3.4, 203.0.113.9"}, "203.0.113.9"},
		{"proxy appends its own header line", "10.0.0.2:80", []string{"1.2.3.4", "203.0.113.9"}, "203.0.113.9"},
		{"chain of trusted proxies", "10.0.0.2:80", []string{"203.0.113.9, 10.0.0.7"}, "203.0.113.9"},
		{"all hops trusted: the first one", "127.0.0.1:80", []string{"10.0.0.9, 10.0.0.7"}, "10.0.0.9"},
		{"garbage ends the walk", "10.0.0.2:80", []string{"203.0.113.9, nonsense, 10.0.0.7"}, "10.0.0.7"},
		{"IPv6 peer", "[2001:db8::1]:443", []string{"198.51.100.1"}, "2001:db8::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			h := middleware.ClientIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = middleware.ClientIPFromContext(r.Context())
			}))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			for _, line := range tc.xff {
				r.Header.Add("X-Forwarded-For", line)
			}
			h.ServeHTTP(httptest.NewRecorder(), r)
			if got != tc.want {
				t.Errorf("client address = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLimitBody(t *testing.T) {
	const maxUpload = 4 << 20
	tests := []struct {
		method, path string
		size         int
		tooBig       bool
	}{
		{http.MethodPost, "/api/v1/auth/login", middleware.DefaultMaxBody, false},
		{http.MethodPost, "/api/v1/auth/login", middleware.DefaultMaxBody + 1, true},
		{http.MethodPost, "/login", middleware.DefaultMaxBody + 1, true},
		{http.MethodPatch, "/api/v1/products/p/attachments", middleware.DefaultMaxBody + 1, true},
		{http.MethodPost, "/api/v1/products/p/attachments", maxUpload, false},
		{http.MethodPost, "/api/v1/products/p/attachments", maxUpload + (1 << 20) + 1, true},
		{http.MethodPost, "/api/v1/products/p/test-runs", middleware.DefaultMaxBody + 1, false},
		{http.MethodPost, "/api/v1/products/p/test-runs", middleware.MaxTestRunBody + 1, true},
		// The web UI's upload routes cap the body themselves.
		{http.MethodPost, "/products/p/defects/d/attachments", maxUpload + (2 << 20), false},
	}
	for _, tc := range tests {
		var readErr error
		h := middleware.LimitBody(maxUpload)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_, readErr = io.Copy(io.Discard, r.Body)
		}))

		// Without a Content-Length, the cap trips while reading.
		readErr = nil
		r := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(make([]byte, tc.size)))
		r.ContentLength = -1
		h.ServeHTTP(httptest.NewRecorder(), r)
		var tooBig *http.MaxBytesError
		if got := errors.As(readErr, &tooBig); got != tc.tooBig {
			t.Errorf("chunked %s %s with %d bytes: too big = %v (err %v), want %v", tc.method, tc.path, tc.size, got, readErr, tc.tooBig)
		}

		// With one, the request is answered with 413 before it is read.
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, bytes.NewReader(make([]byte, tc.size))))
		if got := w.Code == http.StatusRequestEntityTooLarge; got != tc.tooBig {
			t.Errorf("%s %s with Content-Length %d: status %d, want 413: %v", tc.method, tc.path, tc.size, w.Code, tc.tooBig)
		}
	}
}

func TestSecurityHeadersPolicy(t *testing.T) {
	serve := func(https bool) http.Header {
		w := httptest.NewRecorder()
		middleware.SecurityHeaders(https)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
			ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		return w.Header()
	}

	csp := serve(true).Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'", "form-action 'self'", "object-src 'none'", "base-uri 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("Content-Security-Policy %q lacks %q", csp, want)
		}
	}
	// Plain HTTP (development): HSTS would be ignored by browsers and wrong to send.
	if got := serve(false).Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security over HTTP = %q, want none", got)
	}
}
