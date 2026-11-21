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

	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// TestTableViewPages drives the requirements, tasks (UR-144, UR-145) and test
// cases (UR-146) lists over HTTP: the URL carries the search, facets and sort,
// a swap by the table's own controls gets the table fragment, and any other
// request for the same URL gets the whole page.
func TestTableViewPages(t *testing.T) {
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

		for _, title := range []string{"Sync sightings", "Bird gallery", "Export sightings"} {
			_, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{ProductID: pid, Title: title})
			must(err)
		}
		_, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{
			ProductID: pid, Title: "Upgrade toolchain", Kind: model.RequirementKindTask,
		})
		must(err)
		for _, in := range []service.CreateTestCaseInput{
			{ProductID: pid, Title: "Parses sightings", ExecutionMode: model.ExecutionModeAutomatic, TestIdentifier: "TestParse"},
			{ProductID: pid, Title: "Gallery scrolls", ExecutionMode: model.ExecutionModeManual},
		} {
			_, err := e.svc.Testing.CreateTestCase(ctx, e.admin, in)
			must(err)
		}

		get := adminPageGetter(t, e)
		base := "/products/" + pid
		body := get(base+"/requirements?q=sightings&sort=title", nil)
		if !strings.Contains(body, "<html") || !strings.Contains(body, "2 of 3 items") {
			t.Error("a searched requirements page does not render in full with both counts")
		}
		if strings.Contains(body, "Bird gallery") || strings.Contains(body, "Upgrade toolchain") {
			t.Error("the search does not narrow the requirements list, or tasks leak into it")
		}
		if i, j := strings.Index(body, "Export sightings"), strings.Index(body, "Sync sightings"); i < 0 || j < 0 || i > j {
			t.Error("the requirements list is not sorted by title")
		}

		frag := get(base+"/requirements?q=gallery", swap("requirements-table"))
		if strings.Contains(frag, "<html") || !strings.Contains(frag, "Bird gallery") || !strings.Contains(frag, `hx-swap-oob="true"`) {
			t.Error("a swap of the requirements table does not get the fragment with its count")
		}
		// A history restore names no table: it needs the whole page.
		if page := get(base+"/requirements?q=gallery", map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"}); !strings.Contains(page, "<html") {
			t.Error("an htmx request that is not a table swap got a fragment")
		}

		if body := get(base+"/tasks?q=nothing-like-this", nil); !strings.Contains(body, "No matching tasks") {
			t.Error("a filtered-empty tasks list does not say nothing matches")
		}

		body = get(base+"/test-cases?mode=automatic", nil)
		if !strings.Contains(body, "Parses sightings") || strings.Contains(body, "Gallery scrolls") || !strings.Contains(body, "1 of 2 items") {
			t.Error("the mode facet does not filter the test cases")
		}
		frag = get(base+"/test-cases?q=testparse", swap("test-cases-table"))
		if strings.Contains(frag, "<html") || !strings.Contains(frag, "Parses sightings") {
			t.Error("searching test cases by test identifier does not swap in the match")
		}
		// A page past the end lands on the last page instead of an empty one.
		if body := get(base+"/test-cases?page=9", nil); !strings.Contains(body, "Gallery scrolls") {
			t.Error("a page beyond the end renders empty")
		}
	})
}

// swap is the headers of a request by a table's own controls.
func swap(table string) map[string]string {
	return map[string]string{"HX-Request": "true", "HX-Target": table}
}

// adminPageGetter logs in as the administrator and returns a GET of a page
// that must answer 200.
func adminPageGetter(t *testing.T, e *accountEnv) func(path string, headers map[string]string) string {
	t.Helper()
	resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "nexmap_session" && c.Value != "" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	return func(path string, headers map[string]string) string {
		t.Helper()
		r, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.AddCookie(cookie)
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		resp, err := e.client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d", path, resp.StatusCode)
		}
		return body
	}
}

// TestInMemoryTableViewPages drives the remaining tables of UR-145 over HTTP:
// their rows are sorted, searched, filtered and paged in memory, behind the
// same URL contract as the others.
func TestInMemoryTableViewPages(t *testing.T) {
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

		m, err := e.svc.Milestone.CreateMilestone(ctx, e.admin, service.CreateMilestoneInput{ProductID: pid, Name: "Launch"})
		must(err)
		_, err = e.svc.Component.CreateComponent(ctx, e.admin, service.CreateComponentInput{ProductID: pid, Name: "Server", MilestoneID: m.ID})
		must(err)
		_, err = e.svc.Component.CreateComponent(ctx, e.admin, service.CreateComponentInput{ProductID: pid, Name: "App"})
		must(err)

		get := adminPageGetter(t, e)
		base := "/products/" + pid

		body := get(base+"/components?milestone_id=none", nil)
		if !strings.Contains(body, ">App<") || strings.Contains(body, ">Server<") || !strings.Contains(body, "1 of 2 items") {
			t.Error("the milestone facet does not select the components without a milestone")
		}
		frag := get(base+"/components?q=serv", swap("components-table"))
		if strings.Contains(frag, "<html") || !strings.Contains(frag, ">Server<") || strings.Contains(frag, ">App<") {
			t.Error("searching components does not swap in the match alone")
		}
		if body := get(base+"/components?sort=name&dir=desc", nil); strings.Index(body, ">Server<") > strings.Index(body, ">App<") {
			t.Error("components do not sort by name descending")
		}

		// The roadmap shows every milestone whatever the table is filtered to.
		if body := get(base+"/milestones?view=timeline&q=nothing", nil); !strings.Contains(body, "Launch") {
			t.Error("the table search leaks into the roadmap")
		}
		if body := get(base+"/milestones?q=nothing", nil); !strings.Contains(body, "No matching milestones") {
			t.Error("a filtered-empty milestones table does not say nothing matches")
		}

		if frag := get(base+"/team?role=owner", swap("team-table")); !strings.Contains(frag, `id="member-table-body"`) || !strings.Contains(frag, `hx-swap-oob="true"`) {
			t.Error("the team table does not swap with its count")
		}

		// The status tabs of the users page linked to ?status=…; the facet
		// keeps those links working.
		body = get("/admin/users?status=locked", nil)
		if !strings.Contains(body, "No matching users") {
			t.Error("the status filter of the former tabs is not applied")
		}
		if body := get("/admin/users?q=root", nil); !strings.Contains(body, "root") || !strings.Contains(body, `id="user-search"`) {
			t.Error("the users table cannot be searched")
		}
		// Search and facets over nothing at all are left out.
		if body := get("/admin/invitations", nil); !strings.Contains(body, "No invitations") || strings.Contains(body, `id="invitation-search"`) {
			t.Error("an empty invitations list renders table controls")
		}
		_, err = e.svc.Invitation.Create(ctx, e.admin, "birder@example.com", "", "")
		must(err)
		body = get("/admin/invitations?status=accepted", nil)
		if !strings.Contains(body, `id="invitation-filter-status"`) || !strings.Contains(body, "No matching invitations") {
			t.Error("the status facet of the invitations table does not filter")
		}
	})
}
