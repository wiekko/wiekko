// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"database/sql"
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"nexmap.net/internal/buildinfo"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/web/templates/patterns"
)

// TestVersionAndCLI covers UR-172: the user menu links to the documentation
// of the running version and opens an About dialog naming the version, and
// the API Tokens page tells how to install the matching CLI and sign it in.
func TestVersionAndCLI(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
		var session *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "nexmap_session" && c.Value != "" {
				session = c
			}
		}
		if session == nil {
			t.Fatal("no session cookie")
		}
		get := func(path string, htmx bool) (int, string) {
			t.Helper()
			r, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			r.AddCookie(session)
			if htmx {
				r.Header.Set("HX-Request", "true")
			}
			resp, err := e.client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			return resp.StatusCode, html.UnescapeString(readBody(t, resp))
		}

		_, page := get("/", false)
		if !strings.Contains(page, `href="`+patterns.DocsURL("")+`"`) || !strings.Contains(page, `hx-get="/about"`) {
			t.Error("the user menu lacks Help or About")
		}

		status, about := get("/about", true)
		if status != http.StatusOK {
			t.Fatalf("about: %d", status)
		}
		for _, want := range []string{buildinfo.String(), "EUPL 1.2", patterns.Copyright, `id="` + patterns.AboutModalID + `"`} {
			if !strings.Contains(about, want) {
				t.Errorf("the About dialog lacks %q", want)
			}
		}
		if status, _ = get("/about", false); status != http.StatusSeeOther {
			t.Errorf("about opened directly: %d, want a redirect", status)
		}

		_, tokens := get("/user/tokens", false)
		for _, c := range patterns.InstallCommands(buildinfo.Release()) {
			if !strings.Contains(tokens, c.Command) {
				t.Errorf("the tokens page lacks %q", c.Command)
			}
		}
		if want := "nexmap login --server " + e.srv.URL; !strings.Contains(tokens, want) {
			t.Errorf("the tokens page lacks %q", want)
		}
		if !strings.Contains(tokens, patterns.DocsURL("cli/setup.html")) {
			t.Error("the tokens page does not link the CLI setup guide")
		}
	})
}
