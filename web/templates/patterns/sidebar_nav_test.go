// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package patterns

import "testing"

func TestHasSidebar(t *testing.T) {
	tests := []struct {
		name      string
		section   string
		productID string
		want      bool
	}{
		// A product supplies the context, whatever the section says.
		{"product overview", "overview", "p1", true},
		{"product backlog", "requirements", "p1", true},
		{"product setup", "settings", "p1", true},

		// The administration area is its own context, with no product.
		{"admin users", "admin-users", "", true},
		{"admin settings", "admin-settings", "", true},
		{"admin invitations", "admin-invitations", "", true},

		// So are the signed-in user's own account pages.
		{"account profile", "user-profile", "", true},
		{"account tokens", "user-tokens", "", true},

		// No context: these pages must render no sidebar at all.
		{"dashboard", "dashboard", "", false},
		{"product create", "dashboard", "", false},

		// The zero value must not be mistaken for the account context, so that a
		// page which forgets to set a section gets no sidebar rather than the
		// wrong one.
		{"unset section", "", "", false},
		{"unrecognised section", "somewhere-else", "", false},

		// Prefix matching must not fire on a section that merely contains the
		// marker further in.
		{"admin marker not a prefix", "product-admin-users", "", false},
		{"user marker not a prefix", "product-user-profile", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasSidebar(tt.section, tt.productID); got != tt.want {
				t.Errorf("HasSidebar(%q, %q) = %v, want %v", tt.section, tt.productID, got, tt.want)
			}
		})
	}
}
