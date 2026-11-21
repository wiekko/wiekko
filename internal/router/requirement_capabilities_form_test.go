// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/model"
	"wiekko.net/internal/service"
	"wiekko.net/internal/store"
)

// TestRequirementFormLinksCapabilities checks the Capabilities field of the
// New Requirement form (FR-156): a compact chips field whose dropdown lists
// the capabilities by component, suggests the parent's capabilities, and
// links the new requirement to the chosen ones.
func TestRequirementFormLinksCapabilities(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		admin, pid := e.admin, e.product.ID
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		p := "/products/" + pid

		resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
		var cookie *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "wiekko_session" && c.Value != "" {
				cookie = c
			}
		}
		if cookie == nil {
			t.Fatal("no session cookie")
		}
		getAs := func(c *http.Cookie, path string) (int, string) {
			t.Helper()
			r, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
			must(err)
			r.AddCookie(c)
			resp, err := e.client().Do(r)
			must(err)
			return resp.StatusCode, readBody(t, resp)
		}
		get := func(path string) string {
			t.Helper()
			status, body := getAs(cookie, path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status %d", path, status)
			}
			return body
		}
		inOrder := func(what, body string, parts ...string) {
			t.Helper()
			last := -1
			for _, s := range parts {
				i := strings.Index(body, s)
				if i < 0 || i < last {
					t.Errorf("%s: %q missing or out of place (want %v)", what, s, parts)
					return
				}
				last = i
			}
		}

		// Without capabilities there is nothing to pick from.
		if body := get(p + "/requirements/new"); strings.Contains(body, "capability-field") {
			t.Error("Capabilities field shown although the product has no capabilities")
		}

		server, err := e.svc.Component.CreateComponent(ctx, admin, service.CreateComponentInput{ProductID: pid, Name: "Server"})
		must(err)
		web, err := e.svc.Component.CreateComponent(ctx, admin, service.CreateComponentInput{ProductID: pid, Name: "Web UI"})
		must(err)
		backlog, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, ComponentID: server.ID, Name: "Backlog"})
		must(err)
		forms, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, ComponentID: web.ID, Name: "Forms"})
		must(err)
		loose, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, Name: "Reporting"})
		must(err)

		// The form shows a one-line field; the capabilities are only in the
		// dropdown and in the fallback for browsers without JavaScript.
		body := get(p + "/requirements/new")
		if !strings.Contains(body, `id="capability-search"`) || !strings.Contains(body, `id="capability-chips"`) {
			t.Fatal("no Capabilities field on the New Requirement form")
		}
		if before, _, _ := strings.Cut(body, "<noscript>"); strings.Contains(before, backlog.ID) {
			t.Error("capabilities listed in the form outside the dropdown")
		}

		// The dropdown: by component in name order, capabilities without one
		// last, filtered by name or component, without the selected ones.
		opts := p + "/requirements/new/capability-options"
		inOrder("dropdown", get(opts), "Server", backlog.ID, "Web UI", forms.ID, "No component", loose.ID)
		if body := get(opts + "?capability_q=web"); !strings.Contains(body, forms.ID) || strings.Contains(body, backlog.ID) {
			t.Error("search by component name: want Forms only")
		}
		if body := get(opts + "?capability_q=back"); !strings.Contains(body, backlog.ID) || strings.Contains(body, forms.ID) {
			t.Error("search by capability name: want Backlog only")
		}
		if body := get(opts + "?capability_ids=" + backlog.ID); strings.Contains(body, backlog.ID) || strings.Contains(body, ">Server<") {
			t.Error("selected capability (or its emptied heading) still offered")
		}
		if body := get(opts + "?capability_q=nothing-like-this"); !strings.Contains(body, "No capabilities match.") {
			t.Error("no note for a search without matches")
		}

		// A parent with capabilities: they are offered first, under its
		// identifier, and suggested as chips.
		parent, err := e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{
			ProductID: pid, Title: "Parent", CapabilityIDs: []string{forms.ID, loose.ID},
		})
		must(err)
		prefixes, err := e.svc.Category.ListCategories(ctx, admin, pid, model.RequirementKindRequirement)
		must(err)
		ident := ""
		for _, cat := range prefixes {
			if cat.ID == parent.CategoryID {
				ident = model.EntityIdentifier(cat.Prefix, parent.SequenceNumber)
			}
		}
		inOrder("dropdown with a parent", get(opts+"?parent_id="+parent.ID), "From "+ident, forms.ID, loose.ID, "Server", backlog.ID)
		if body := get(opts + "?parent_id=" + parent.ID); strings.Count(body, `data-capability-id="`+forms.ID+`"`) != 1 {
			t.Error("a parent's capability is offered twice")
		}

		chipsURL := p + "/requirements/new/capability-chips"
		// No selection yet: the parent's capabilities become a suggestion.
		body = get(chipsURL + "?parent_id=" + parent.ID)
		if !strings.Contains(body, `value="`+forms.ID+`"`) || !strings.Contains(body, `value="`+loose.ID+`"`) ||
			!strings.Contains(body, `name="capability_suggested"`) || !strings.Contains(body, "Suggested from "+ident) {
			t.Errorf("parent suggestion: %s", body)
		}
		// The user's own selection stays as it is.
		body = get(chipsURL + "?parent_id=" + parent.ID + "&capability_ids=" + backlog.ID)
		if !strings.Contains(body, `value="`+backlog.ID+`"`) || strings.Contains(body, forms.ID) || strings.Contains(body, "capability_suggested") {
			t.Errorf("own selection replaced: %s", body)
		}
		// An untouched suggestion follows the parent, also back to none.
		body = get(chipsURL + "?parent_id=&capability_suggested=1&capability_ids=" + forms.ID)
		if strings.Contains(body, `name="capability_ids"`) || strings.Contains(body, "Suggested from") {
			t.Errorf("suggestion kept after the parent was cleared: %s", body)
		}

		// The fragments are for owners, like the form.
		viewer := e.invite("vera", model.RoleViewer)
		for _, path := range []string{opts, chipsURL} {
			if status, _ := getAs(viewer, path); status != http.StatusForbidden {
				t.Errorf("viewer GET %s: status %d, want 403", path, status)
			}
		}

		// The edit form leaves links to the Capabilities tab.
		if body := get(p + "/requirements/" + parent.ID + "/edit"); strings.Contains(body, "capability-field") {
			t.Error("Capabilities field shown on the edit form")
		}

		post := func(form url.Values) string {
			t.Helper()
			form.Set("_csrf", csrfToken(t, e, cookie))
			r, err := http.NewRequest(http.MethodPost, e.srv.URL+p+"/requirements", strings.NewReader(form.Encode()))
			must(err)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.AddCookie(cookie)
			resp, err := e.client().Do(r)
			must(err)
			return readBody(t, resp)
		}

		// A failed create keeps the chips, and an untouched suggestion stays one.
		body = post(url.Values{"title": {""}, "parent_id": {parent.ID}, "capability_ids": {forms.ID}, "capability_suggested": {"1"}})
		chips, _, _ := strings.Cut(body, "<noscript>")
		if !strings.Contains(chips, `name="capability_ids" value="`+forms.ID+`"`) || !strings.Contains(chips, "Suggested from "+ident) {
			t.Error("chips or suggestion lost after a validation error")
		}

		post(url.Values{"title": {"Sort me in"}, "capability_ids": {backlog.ID, loose.ID}})
		items, err := e.svc.Backlog.ListRequirements(ctx, admin, pid, store.RequirementFilter{})
		must(err)
		var id string
		for _, r := range items {
			if r.Title == "Sort me in" {
				id = r.ID
			}
		}
		if id == "" {
			t.Fatal("requirement not created")
		}
		caps, err := e.svc.Component.ListLinkedCapabilities(ctx, admin, pid, id)
		must(err)
		got := map[string]bool{}
		for _, c := range caps {
			got[c.ID] = true
		}
		if len(caps) != 2 || !got[backlog.ID] || !got[loose.ID] {
			t.Errorf("linked capabilities: %v, want Backlog and Reporting", got)
		}
	})
}
