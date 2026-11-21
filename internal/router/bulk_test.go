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

// TestBulkOperationsPages drives the bulk operations of the requirements
// table (FR-130, FR-131): who may select, the confirmation naming what will
// and will not change (SC-92, UC-91), and applying it.
func TestBulkOperationsPages(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		pid := e.product.ID
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		statuses, err := e.svc.Milestone.ListStatuses(ctx, e.admin, pid, model.RequirementKindRequirement)
		must(err)
		var closed *model.Status
		for _, s := range statuses {
			if s.IsClosed {
				closed = s
			}
		}
		if closed == nil {
			t.Fatal("no closed status")
		}
		open, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{ProductID: pid, Title: "Still open"})
		must(err)
		done, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{ProductID: pid, Title: "All done"})
		must(err)
		_, err = e.svc.Backlog.UpdateRequirement(ctx, e.admin, pid, done.ID, service.UpdateRequirementInput{StatusID: &closed.ID})
		must(err)

		resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
		var admin *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "wiekko_session" && c.Value != "" {
				admin = c
			}
		}
		if admin == nil {
			t.Fatal("no session cookie")
		}
		page := "/products/" + pid + "/requirements"
		get := func(cookie *http.Cookie) string {
			t.Helper()
			r, err := http.NewRequest(http.MethodGet, e.srv.URL+page, nil)
			must(err)
			r.AddCookie(cookie)
			resp, err := e.client().Do(r)
			must(err)
			return readBody(t, resp)
		}
		post := func(cookie *http.Cookie, step string, form url.Values) (*http.Response, string) {
			t.Helper()
			r, err := http.NewRequest(http.MethodPost, e.srv.URL+page+"/bulk/"+step, strings.NewReader(form.Encode()))
			must(err)
			r.AddCookie(cookie)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
			r.Header.Set("HX-Request", "true")
			resp, err := e.client().Do(r)
			must(err)
			return resp, readBody(t, resp)
		}

		body := get(admin)
		for _, want := range []string{`data-bulk-toggle="requirements-table"`, `data-bulk-bar="requirements-table"`, `data-bulk-id="` + open.ID + `"`, `data-bulk-action="priority"`, `data-bulk-action="delete"`} {
			if !strings.Contains(body, want) {
				t.Errorf("owner's table lacks %s", want)
			}
		}

		// Archiving needs a closed status: the confirmation offers the done
		// item and names the open one with the reason.
		ids := open.ID + "," + done.ID
		_, body = post(admin, "preview", url.Values{"action": {"archive"}, "ids": {ids}})
		if !strings.Contains(body, "Archive 1") || !strings.Contains(body, "1 can&#39;t be changed:") || !strings.Contains(body, "closed status") {
			t.Errorf("archive preview does not tell what can and cannot change:\n%s", body)
		}
		if got, _ := e.svc.Backlog.GetRequirement(ctx, e.admin, pid, done.ID); got.Archived() {
			t.Fatal("the preview archived an item")
		}

		resp, body = post(admin, "apply", url.Values{"action": {"archive"}, "ids": {ids}, "state": {""}})
		if resp.StatusCode != http.StatusOK || resp.Header.Get("HX-Trigger") != "bulkApplied" {
			t.Fatalf("apply: %d %q", resp.StatusCode, resp.Header.Get("HX-Trigger"))
		}
		if !strings.Contains(body, "Archived 1 requirement.") || !strings.Contains(body, "1 requirement could not be changed") || !strings.Contains(body, `data-bulk-bar="requirements-table"`) {
			t.Errorf("apply answer lacks the summary or the table:\n%s", body)
		}
		if got, _ := e.svc.Backlog.GetRequirement(ctx, e.admin, pid, done.ID); !got.Archived() {
			t.Error("the done item was not archived")
		}

		// All matching: the confirmation resolves the filter (archived items
		// hidden by default) and freezes the IDs it found.
		_, body = post(admin, "preview", url.Values{"action": {"status"}, "scope": {"matching"}, "state": {""}})
		if !strings.Contains(body, `name="ids" value="`+open.ID+`"`) || !strings.Contains(body, "Change 1") {
			t.Errorf("matching preview does not freeze the open item alone:\n%s", body)
		}

		_, body = post(admin, "preview", url.Values{"action": {"priority"}, "ids": {open.ID}})
		if !strings.Contains(body, "Set the priority of 1 requirement to “P1 (highest)”") || !strings.Contains(body, `value="0"`) {
			t.Errorf("priority preview does not default to P1 or lacks “No priority”:\n%s", body)
		}

		tooMany := strings.Repeat(open.ID+",", service.BulkMaxItems+1)
		_, body = post(admin, "preview", url.Values{"action": {"milestone"}, "ids": {tooMany}})
		if !strings.Contains(body, "at most") {
			t.Error("an oversized selection is not refused")
		}

		// Developers may only change statuses; viewers cannot select.
		dev := e.invite("dev", model.RoleDeveloper)
		body = get(dev)
		if !strings.Contains(body, `data-bulk-action="status"`) || !strings.Contains(body, `data-bulk-action="assign"`) || strings.Contains(body, `data-bulk-action="archive"`) || strings.Contains(body, `data-bulk-action="priority"`) {
			t.Error("a developer's bar does not offer exactly the status change")
		}
		resp, _ = post(dev, "apply", url.Values{"action": {"delete"}, "ids": {open.ID}, "state": {""}})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("developer delete apply: %d", resp.StatusCode)
		}
		if _, err := e.svc.Backlog.GetRequirement(ctx, e.admin, pid, open.ID); err != nil {
			t.Error("a developer deleted an item in bulk")
		}
		viewer := e.invite("viewer", model.RoleViewer)
		if body = get(viewer); strings.Contains(body, "data-bulk-toggle") {
			t.Error("a viewer is offered selection")
		}
	})
}

// TestBulkTestCasesPage drives the bulk operations of the test cases table
// (FR-169): developers select and link; testers cannot select.
func TestBulkTestCasesPage(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		pid := e.product.ID
		tc, err := e.svc.Testing.CreateTestCase(ctx, e.admin, service.CreateTestCaseInput{ProductID: pid, Title: "Parses sightings", ExecutionMode: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{ProductID: pid, Title: "Record a sighting"})
		if err != nil {
			t.Fatal(err)
		}
		page := "/products/" + pid + "/test-cases"
		do := func(cookie *http.Cookie, method, path string, form url.Values) string {
			t.Helper()
			var body *strings.Reader
			if form != nil {
				body = strings.NewReader(form.Encode())
			} else {
				body = strings.NewReader("")
			}
			req, err := http.NewRequest(method, e.srv.URL+path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(cookie)
			if method == http.MethodPost {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
				req.Header.Set("HX-Request", "true")
			}
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return readBody(t, resp)
		}

		dev := e.invite("dev", model.RoleDeveloper)
		body := do(dev, http.MethodGet, page, nil)
		for _, want := range []string{`data-bulk-toggle="test-cases-table"`, `data-bulk-id="` + tc.ID + `"`, `data-bulk-action="link_requirement"`, `data-bulk-action="delete"`} {
			if !strings.Contains(body, want) {
				t.Errorf("developer's test cases table lacks %s", want)
			}
		}
		body = do(dev, http.MethodPost, page+"/bulk/preview", url.Values{"action": {"link_requirement"}, "ids": {tc.ID}, "value": {r.ID}})
		if !strings.Contains(body, "Link 1 test case to") || !strings.Contains(body, tc.ID[:8]) {
			t.Errorf("link preview:\n%s", body)
		}
		body = do(dev, http.MethodPost, page+"/bulk/apply", url.Values{"action": {"link_requirement"}, "ids": {tc.ID}, "value": {r.ID}, "state": {""}})
		if !strings.Contains(body, "Linked 1 test case.") {
			t.Errorf("link apply:\n%s", body)
		}
		links, _ := e.svc.Testing.ListTestCaseRequirements(ctx, e.admin, pid, tc.ID)
		if len(links) != 1 {
			t.Errorf("%d links, want 1", len(links))
		}

		tester := e.invite("tess", model.RoleTester)
		if body = do(tester, http.MethodGet, page, nil); strings.Contains(body, "data-bulk-toggle") {
			t.Error("a tester is offered selection")
		}
	})
}

// TestBulkConstraintsPage drives the bulk operations of the constraints table
// (FR-170): owners select and set compliance; developers cannot select.
func TestBulkConstraintsPage(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		pid := e.product.ID
		cats, err := e.svc.Category.ListCategories(ctx, e.admin, pid, "constraint")
		if err != nil || len(cats) == 0 {
			t.Fatalf("constraint categories: %v", err)
		}
		con, err := e.svc.Constraint.CreateConstraint(ctx, e.admin, service.CreateConstraintInput{ProductID: pid, CategoryID: cats[0].ID, Title: "Runs offline"})
		if err != nil {
			t.Fatal(err)
		}
		page := "/products/" + pid + "/constraints"
		get := adminPageGetter(t, e)
		body := get(page, nil)
		for _, want := range []string{`data-bulk-toggle="constraints-table"`, `data-bulk-bar="constraints-table"`, `data-bulk-id="` + con.ID + `"`, `data-bulk-action="compliance"`} {
			if !strings.Contains(body, want) {
				t.Errorf("owner's constraints table lacks %s", want)
			}
		}

		resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
		var admin *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "wiekko_session" && c.Value != "" {
				admin = c
			}
		}
		post := func(step string, form url.Values) string {
			t.Helper()
			r, err := http.NewRequest(http.MethodPost, e.srv.URL+page+"/bulk/"+step, strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			r.AddCookie(admin)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("X-CSRF-Token", csrfToken(t, e, admin))
			r.Header.Set("HX-Request", "true")
			resp, err := e.client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			return readBody(t, resp)
		}
		// All matching, with the table's search narrowing it to the one.
		body = post("preview", url.Values{"action": {"compliance"}, "scope": {"matching"}, "state": {"q=offline"}, "value": {model.ComplianceViolated}})
		if !strings.Contains(body, "Set the compliance of 1 constraint to “Violated”") || !strings.Contains(body, `name="ids" value="`+con.ID+`"`) {
			t.Errorf("compliance preview:\n%s", body)
		}
		body = post("apply", url.Values{"action": {"compliance"}, "ids": {con.ID}, "value": {model.ComplianceViolated}, "state": {""}})
		if !strings.Contains(body, "Changed 1 constraint.") || !strings.Contains(body, "Violated") {
			t.Errorf("compliance apply:\n%s", body)
		}
		if got, _ := e.svc.Constraint.GetConstraint(ctx, e.admin, pid, con.ID); got.ComplianceState != model.ComplianceViolated {
			t.Errorf("compliance %q", got.ComplianceState)
		}

		dev := e.invite("dev", model.RoleDeveloper)
		r, err := http.NewRequest(http.MethodGet, e.srv.URL+page, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.AddCookie(dev)
		dresp, err := e.client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		if body := readBody(t, dresp); strings.Contains(body, "data-bulk-toggle") {
			t.Error("a developer is offered selection on constraints")
		}
	})
}
