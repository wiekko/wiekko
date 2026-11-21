// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import "testing"

// TestFormBody checks that saving the document form unchanged never rewrites
// a body: browsers submit textarea line breaks as CRLF.
func TestFormBody(t *testing.T) {
	cases := []struct {
		submitted, existing, want string
	}{
		{"a\r\nb\r\n", "", "a\nb\n"},               // new body: LF
		{"a\r\nb\r\n", "a\nb\n", "a\nb\n"},         // LF body saved unchanged
		{"a\r\nb\r\n", "a\r\nb\r\n", "a\r\nb\r\n"}, // CRLF body saved unchanged
		{"a\r\nc\r\n", "a\r\nb\r\n", "a\nc\n"},     // CRLF body edited: LF
		{"", "a\n", ""},                            // cleared
	}
	for _, c := range cases {
		if got := formBody(c.submitted, c.existing); got != c.want {
			t.Errorf("formBody(%q, %q) = %q, want %q", c.submitted, c.existing, got, c.want)
		}
	}
}
