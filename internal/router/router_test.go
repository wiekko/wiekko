// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"testing"

	"wiekko.net/internal/service"
)

// TestNewRegistersRoutes guards against duplicate or overlapping route
// patterns: http.ServeMux panics on conflict at registration time, which would
// otherwise only surface as a crash on startup.
func TestNewRegistersRoutes(t *testing.T) {
	// The handlers are never invoked, so an empty service set is enough.
	if h, err := New(&service.Services{}, Options{CookieSecure: true, DevMode: true, MaxUploadBytes: 1 << 20}); err != nil || h == nil {
		t.Fatal("New returned nil")
	}
}
