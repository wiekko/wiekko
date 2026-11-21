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
)

// TestFormAccessRedirectsWithNotice opens every create and edit form as a
// user whose role cannot save it. Each must redirect to what the user can
// read — the list for a create form, the detail page for an edit form — with
// a notice naming the role required (ui-structure.md, design principle 6).
func TestFormAccessRedirectsWithNotice(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		scrum := model.WorkflowModeScrum
		if _, err := e.svc.Product.Update(ctx, e.admin, e.product.ID, service.UpdateProductInput{WorkflowMode: &scrum}); err != nil {
			t.Fatalf("switch to scrum: %v", err)
		}

		signIn := func(username, password string) *http.Cookie {
			t.Helper()
			resp, _ := e.post("/login", url.Values{"username": {username}, "password": {password}})
			for _, c := range resp.Cookies() {
				if c.Name == "wiekko_session" && c.Value != "" {
					return c
				}
			}
			t.Fatalf("%s: no session cookie, status %d", username, resp.StatusCode)
			return nil
		}
		member := func(username, role string) *http.Cookie {
			t.Helper()
			inv, err := e.svc.Invitation.Create(ctx, e.admin, username+"@example.com", e.product.ID, role)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.svc.User.AcceptInvitation(ctx, inv.RawToken, service.RegisterInput{Username: username, Password: goodPassword}); err != nil {
				t.Fatalf("accept invitation for %s: %v", username, err)
			}
			return signIn(username, goodPassword)
		}
		admin := signIn("root", goodPassword)
		viewer := member("vic", model.RoleViewer)
		tester := member("tess", model.RoleTester)
		developer := member("dev", model.RoleDeveloper)

		get := func(cookie *http.Cookie, path string, header ...string) (*http.Response, string) {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(cookie)
			for i := 0; i+1 < len(header); i += 2 {
				req.Header.Set(header[i], header[i+1])
			}
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp, readBody(t, resp)
		}

		p := "/products/" + e.product.ID
		// Edit forms check the role before loading the item, so any ID will do.
		for _, tc := range []struct {
			who    string
			cookie *http.Cookie
			form   string
			target string
			code   string
		}{
			{"viewer", viewer, p + "/documents/new", p + "/documents", "needs-owner"},
			{"viewer", viewer, p + "/documents/x/edit", p + "/documents/x", "needs-owner"},
			{"viewer", viewer, p + "/constraints/new", p + "/constraints", "needs-owner"},
			{"viewer", viewer, p + "/constraints/x/edit", p + "/constraints/x", "needs-owner"},
			{"viewer", viewer, p + "/requirements/new", p + "/requirements", "needs-owner"},
			{"viewer", viewer, p + "/requirements/x/edit", p + "/requirements/x", "needs-owner"},
			{"viewer", viewer, p + "/tasks/new", p + "/tasks", "needs-owner"},
			{"viewer", viewer, p + "/tasks/x/edit", p + "/tasks/x", "needs-owner"},
			{"viewer", viewer, p + "/test-cases/new", p + "/test-cases", "needs-developer"},
			{"tester", tester, p + "/test-cases/new", p + "/test-cases", "needs-developer"},
			{"tester", tester, p + "/test-cases/x/edit", p + "/test-cases/x", "needs-developer"},
			{"viewer", viewer, p + "/defects/new", p + "/defects", "needs-tester"},
			{"developer", developer, p + "/defects/x/edit", p + "/defects/x", "needs-owner"},
			{"developer", developer, p + "/campaigns/new", p + "/campaigns", "needs-owner"},
			{"developer", developer, p + "/campaigns/x/edit", p + "/campaigns/x", "needs-owner"},
			{"viewer", viewer, p + "/milestones/new", p + "/milestones", "needs-owner"},
			{"viewer", viewer, p + "/milestones/x/edit", p + "/milestones/x", "needs-owner"},
			{"viewer", viewer, p + "/components/new", p + "/components", "needs-owner"},
			{"viewer", viewer, p + "/components/x/edit", p + "/components/x", "needs-owner"},
			{"viewer", viewer, p + "/capabilities/new", p + "/capabilities", "needs-owner"},
			{"viewer", viewer, p + "/capabilities/x/edit", p + "/capabilities/x", "needs-owner"},
			{"viewer", viewer, p + "/sprints/new", p + "/sprints", "needs-owner"},
			{"viewer", viewer, p + "/sprints/x/edit", p + "/sprints/x", "needs-owner"},
		} {
			resp, _ := get(tc.cookie, tc.form)
			want := tc.target + "?notice=" + tc.code
			if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
				t.Errorf("%s GET %s: status %d, Location %q, want 303 to %q", tc.who, tc.form, resp.StatusCode, resp.Header.Get("Location"), want)
			}
		}

		// Those who may save still get the forms.
		for _, tc := range []struct {
			who    string
			cookie *http.Cookie
			form   string
		}{
			{"admin", admin, p + "/documents/new"},
			{"admin", admin, p + "/constraints/new"},
			{"admin", admin, p + "/requirements/new"},
			{"admin", admin, p + "/tasks/new"},
			{"admin", admin, p + "/campaigns/new"},
			{"admin", admin, p + "/milestones/new"},
			{"admin", admin, p + "/components/new"},
			{"admin", admin, p + "/capabilities/new"},
			{"admin", admin, p + "/sprints/new"},
			{"developer", developer, p + "/test-cases/new"},
			{"tester", tester, p + "/defects/new"},
		} {
			if resp, _ := get(tc.cookie, tc.form); resp.StatusCode != http.StatusOK {
				t.Errorf("%s GET %s: status %d, want 200", tc.who, tc.form, resp.StatusCode)
			}
		}

		// Campaign pages for a missing product or campaign redirect like every
		// other page: to the dashboard, or to the campaign list.
		missing := "/products/00000000-0000-0000-0000-000000000000"
		for _, tc := range []struct{ path, target string }{
			{missing + "/campaigns", "/"},
			{missing + "/campaigns/new", "/"},
			{missing + "/campaigns/x", "/"},
			{missing + "/campaigns/x/edit", "/"},
			{p + "/campaigns/x", p + "/campaigns"},
			{p + "/campaigns/x/edit", p + "/campaigns"},
		} {
			if resp, _ := get(admin, tc.path); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != tc.target {
				t.Errorf("GET %s: status %d, Location %q, want 303 to %q", tc.path, resp.StatusCode, resp.Header.Get("Location"), tc.target)
			}
		}

		// The product form, with product creation turned off.
		e.setting("product_creation_enabled", "false")
		if resp, _ := get(viewer, "/products/new"); resp.Header.Get("Location") != "/?notice=product-creation-off" {
			t.Errorf("product form with creation off: status %d, Location %q", resp.StatusCode, resp.Header.Get("Location"))
		}

		// The page redirected to shows the notice, and only a known one.
		const ownerNotice = "You need the owner role in this product to create or edit that."
		resp, page := get(viewer, p+"/documents?notice=needs-owner")
		if resp.StatusCode != http.StatusOK || !strings.Contains(page, ownerNotice) || !strings.Contains(page, "history.replaceState") {
			t.Errorf("documents with notice: status %d, notice shown: %v", resp.StatusCode, strings.Contains(page, ownerNotice))
		}
		if _, page := get(viewer, "/?notice=product-creation-off"); !strings.Contains(page, "Creating products is turned off. Ask an administrator.") {
			t.Error("dashboard does not show the product creation notice")
		}
		for _, path := range []string{p + "/documents", p + "/documents?notice=bogus", p + "/documents?notice=" + url.QueryEscape(ownerNotice)} {
			if _, page := get(viewer, path); strings.Contains(page, "You need") || strings.Contains(page, "history.replaceState") {
				t.Errorf("GET %s shows a notice", path)
			}
		}
		if _, page := get(viewer, p+"/documents?notice=needs-owner", "HX-Request", "true"); strings.Contains(page, ownerNotice) {
			t.Error("an htmx request rendered the notice")
		}
	})
}
