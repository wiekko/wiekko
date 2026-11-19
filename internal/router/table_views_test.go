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
		get := func(path string, headers map[string]string) string {
			t.Helper()
			r, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
			must(err)
			r.AddCookie(cookie)
			for k, v := range headers {
				r.Header.Set(k, v)
			}
			resp, err := e.client().Do(r)
			must(err)
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: %d", path, resp.StatusCode)
			}
			return body
		}
		swap := func(table string) map[string]string {
			return map[string]string{"HX-Request": "true", "HX-Target": table}
		}

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
