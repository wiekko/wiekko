package router

import (
	"testing"

	"nexmap.net/internal/service"
)

// TestNewRegistersRoutes guards against duplicate or overlapping route
// patterns: http.ServeMux panics on conflict at registration time, which would
// otherwise only surface as a crash on startup.
func TestNewRegistersRoutes(t *testing.T) {
	// The handlers are never invoked, so an empty service set is enough.
	if h := New(&service.Services{}, true, true, 1<<20); h == nil {
		t.Fatal("New returned nil")
	}
}
