// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware

import "testing"

func TestLoggablePathRedactsTokens(t *testing.T) {
	tests := map[string]string{
		"/accept-invite/3107d8dbc3de88f6": "/accept-invite/[redacted]",
		"/accept-invite/":                 "/accept-invite/",
		"/reset-password/97fca2c249f6":    "/reset-password/[redacted]",
		"/products/abc/defects":           "/products/abc/defects",
		"/login":                          "/login",
	}
	for in, want := range tests {
		if got := loggablePath(in); got != want {
			t.Errorf("loggablePath(%q) = %q, want %q", in, got, want)
		}
	}
}
