package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthURL(t *testing.T) {
	for _, tc := range []struct{ host, want string }{
		{"0.0.0.0", "http://127.0.0.1:8080/api/v1/health"},
		{"", "http://127.0.0.1:8080/api/v1/health"},
		{"::", "http://[::1]:8080/api/v1/health"},
		{"localhost", "http://localhost:8080/api/v1/health"},
		{"10.0.0.5", "http://10.0.0.5:8080/api/v1/health"},
	} {
		if got := healthURL(tc.host, "8080"); got != tc.want {
			t.Errorf("healthURL(%q) = %s, want %s", tc.host, got, tc.want)
		}
	}
}

func TestHealthcheck(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-host", host, "-port", port}

	if code := healthcheck(args); code != 0 {
		t.Errorf("healthy server: exit code %d, want 0", code)
	}
	status = http.StatusServiceUnavailable
	if code := healthcheck(args); code != 1 {
		t.Errorf("unhealthy server: exit code %d, want 1", code)
	}
	srv.Close()
	if code := healthcheck(args); code != 1 {
		t.Errorf("stopped server: exit code %d, want 1", code)
	}
}
