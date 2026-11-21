// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"

	"nexmap.net/web/templates/patterns"
)

// About handles GET /about: the About dialog of the user menu, swapped into
// the page's modal container (UR-172). It is a fragment, so a browser that
// opens the address directly goes to the dashboard instead.
func About(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = patterns.AboutModal().Render(r.Context(), w)
}
