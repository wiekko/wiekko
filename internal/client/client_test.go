package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func noBackoff(int) time.Duration { return 0 }

func newTestClient(t *testing.T, h http.HandlerFunc, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	opts = append([]Option{WithBackoff(noBackoff)}, opts...)
	c, err := New(srv.URL, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNormalizeServer(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "https://Nexmap.Example.com/", want: "https://nexmap.example.com"},
		{in: "https://nexmap.example.com/api/v1", want: "https://nexmap.example.com"},
		{in: "https://nexmap.example.com/api/v1/", want: "https://nexmap.example.com"},
		{in: "https://example.com/nexmap/?x=1#y", want: "https://example.com/nexmap"},
		{in: "http://localhost:8080", want: "http://localhost:8080"},
		{in: "nexmap.example.com", wantErr: true},
		{in: "ftp://nexmap.example.com", wantErr: true},
		{in: "", wantErr: true},
		{in: "https://", wantErr: true},
	}
	for _, tt := range tests {
		got, err := NormalizeServer(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("NormalizeServer(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeServer(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewRefusesPlainHTTPToRemoteHost(t *testing.T) {
	if _, err := New("http://nexmap.example.com"); !errors.Is(err, ErrInsecureServer) {
		t.Fatalf("err = %v, want ErrInsecureServer", err)
	}
	for _, s := range []string{"http://localhost:8080", "http://127.0.0.1:8080", "http://[::1]:8080", "http://app.localhost"} {
		if _, err := New(s); err != nil {
			t.Errorf("New(%q) = %v, want nil", s, err)
		}
	}
	if _, err := New("http://nexmap.example.com", AllowInsecure()); err != nil {
		t.Fatalf("AllowInsecure: %v", err)
	}
}

func TestDoSendsTokenAndDecodesEnvelope(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/me" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer nxp_secret" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "nexmap/test" {
			t.Errorf("User-Agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"u1","username":"ada","is_admin":true}}`)
	}, WithToken("nxp_secret"), WithUserAgent("nexmap/test"))

	u, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if u.ID != "u1" || u.Username != "ada" || !u.IsAdmin {
		t.Fatalf("user = %+v", u)
	}
}

func TestDoSendsJSONBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"name":"x"}` {
			t.Errorf("body = %s", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"p1","name":"x"}}`)
	})
	var p Product
	if err := c.Do(context.Background(), http.MethodPost, "/products", map[string]string{"name": "x"}, &p); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if p.ID != "p1" {
		t.Fatalf("product = %+v", p)
	}
}

func TestDoReturnsAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":"NOT_FOUND","message":"Product not found"}}`)
	})
	_, err := c.Product(context.Background(), "nope")
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %T %v, want *APIError", err, err)
	}
	if ae.Code != "NOT_FOUND" || ae.Message != "Product not found" || !IsNotFound(err) {
		t.Fatalf("APIError = %+v", ae)
	}
	if !strings.Contains(err.Error(), "Product not found") {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestDoReturnsAPIErrorForNonEnvelopeBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "<html>bad gateway</html>", http.StatusUnauthorized)
	})
	_, err := c.Me(context.Background())
	if !IsUnauthorized(err) {
		t.Fatalf("err = %v, want 401", err)
	}
	if strings.Contains(err.Error(), "<html>") {
		t.Fatalf("Error() leaks HTML: %q", err.Error())
	}
}

func TestGetIsRetriedOnTransientStatus(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"version":"v1","api_level":1}}`)
	})
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.APILevel != 1 || calls.Load() != 3 {
		t.Fatalf("api_level = %d, calls = %d", v.APILevel, calls.Load())
	}
}

func TestGetRetriesAreBounded(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}, WithMaxRetries(2))
	if _, err := c.Version(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3 (1 + 2 retries)", calls.Load())
	}
}

func TestPostIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	err := c.Do(context.Background(), http.MethodPost, "/products", map[string]string{}, nil)
	if err == nil || calls.Load() != 1 {
		t.Fatalf("err = %v, calls = %d, want error after 1 call", err, calls.Load())
	}
}

func TestRawKeepsErrorBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "kind=task" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":"FORBIDDEN","message":"no"}}`)
	})
	resp, err := c.Raw(context.Background(), "get", "products/p1/requirements?kind=task", nil, "")
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(resp.Body), "FORBIDDEN") {
		t.Fatalf("resp = %d %s", resp.StatusCode, resp.Body)
	}
}

func TestServerSubpathIsKept(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"data":{"version":"v","api_level":1}}`)
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL + "/nexmap/")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Version(context.Background()); err != nil {
		t.Fatalf("Version: %v", err)
	}
	if gotPath != "/nexmap/api/v1/version" {
		t.Fatalf("path = %q", gotPath)
	}
}
