// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package patterns

import "strings"

// isAdminSection reports whether section belongs to the instance administration
// area. Admin pages all set an "admin-" prefixed ActiveSection.
func isAdminSection(section string) bool {
	return strings.HasPrefix(section, "admin-")
}

// isAccountSection reports whether section belongs to the signed-in user's own
// account pages, which set a "user-" prefixed ActiveSection.
//
// The prefix rather than the empty string is deliberate: "" is the zero value of
// ActiveSection, so matching on it would hand the account navigation to any page
// that merely forgot to set a section.
func isAccountSection(section string) bool {
	return strings.HasPrefix(section, "user-")
}

// HasSidebar reports whether SidebarNav would render any navigation for this
// context. Callers must omit the sidebar entirely where it returns false rather
// than render an empty panel — that is the case on the dashboard and the
// product-create page, which belong to no product.
func HasSidebar(activeSection, productID string) bool {
	return productID != "" || isAdminSection(activeSection) || isAccountSection(activeSection)
}
