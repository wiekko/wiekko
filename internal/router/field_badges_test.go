package router

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// hasBadge reports whether body renders label as a badge pill rather than
// as plain text.
func hasBadge(body, label string) bool {
	re := regexp.MustCompile(`<span class="inline-flex items-center rounded-full[^"]*"[^>]*>\s*` + regexp.QuoteMeta(label) + `\s*</span>`)
	return re.MatchString(body)
}

// TestFieldBadgesAndStatusColumns checks that the tables on detail pages show
// fixed-value fields as the same badges as the list pages (UC-89), and that
// every table of requirements, tasks or defects has a Status column (UC-90).
func TestFieldBadgesAndStatusColumns(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		admin := e.admin
		pid := e.product.ID
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		// A status of each workflow that is not the default, so the badge
		// shows a name that appears nowhere else on the tab.
		pickStatus := func(entityType string) *model.Status {
			t.Helper()
			statuses, err := e.svc.Milestone.ListStatuses(ctx, admin, pid, entityType)
			must(err)
			for _, s := range statuses {
				if !s.IsDefault {
					return s
				}
			}
			t.Fatalf("no non-default %s status", entityType)
			return nil
		}
		reqStatus := pickStatus("requirement")
		defStatus := pickStatus("defect")

		m, err := e.svc.Milestone.CreateMilestone(ctx, admin, service.CreateMilestoneInput{ProductID: pid, Name: "v2"})
		must(err)
		comp, err := e.svc.Component.CreateComponent(ctx, admin, service.CreateComponentInput{ProductID: pid, Name: "Server"})
		must(err)
		capab, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, ComponentID: comp.ID, Name: "Backlog"})
		must(err)
		req, err := e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{
			ProductID: pid, Kind: model.RequirementKindRequirement, Title: "Record a sighting", StatusID: reqStatus.ID, MilestoneID: m.ID,
		})
		must(err)
		task, err := e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{
			ProductID: pid, Kind: model.RequirementKindTask, Title: "Write the form", StatusID: reqStatus.ID, MilestoneID: m.ID, ParentID: req.ID,
		})
		must(err)
		must(e.svc.Component.LinkRequirementToCapability(ctx, admin, pid, capab.ID, req.ID))
		def, err := e.svc.Backlog.CreateDefect(ctx, admin, service.CreateDefectInput{
			ProductID: pid, Title: "Sightings lose their date", Criticality: "high", StatusID: defStatus.ID, MilestoneID: m.ID,
		})
		must(err)
		must(e.svc.Backlog.AddDefectLink(ctx, admin, pid, def.ID, model.DefectLinkTargetRequirement, req.ID))
		must(e.svc.Backlog.AddDefectLink(ctx, admin, pid, def.ID, model.DefectLinkTargetComponent, comp.ID))
		capDef, err := e.svc.Backlog.CreateDefect(ctx, admin, service.CreateDefectInput{
			ProductID: pid, Title: "Backlog loses order", Criticality: "medium", StatusID: defStatus.ID,
		})
		must(err)
		must(e.svc.Backlog.AddDefectLink(ctx, admin, pid, capDef.ID, model.DefectLinkTargetCapability, capab.ID))
		tc, err := e.svc.Testing.CreateTestCase(ctx, admin, service.CreateTestCaseInput{
			ProductID: pid, ComponentID: comp.ID, CapabilityID: capab.ID, Title: "Sighting round trip", ExecutionMode: "automatic",
		})
		must(err)
		must(e.svc.Testing.LinkRequirement(ctx, admin, pid, tc.ID, req.ID, "requirement"))
		must(e.svc.Backlog.AddDefectLink(ctx, admin, pid, def.ID, model.DefectLinkTargetTestCase, tc.ID))

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
		get := func(path string) string {
			t.Helper()
			r, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
			must(err)
			r.AddCookie(cookie)
			resp, err := e.client().Do(r)
			must(err)
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status %d", path, resp.StatusCode)
			}
			return body
		}
		p := "/products/" + pid

		// Tables of requirements and tasks: a Status column with the badge.
		for _, path := range []string{
			p + "/milestones/" + m.ID + "/tabs/requirements",
			p + "/milestones/" + m.ID + "/tabs/tasks",
			p + "/requirements/" + req.ID + "/tabs/children",
			p + "/components/" + comp.ID + "/tabs/requirements",
			p + "/capabilities/" + capab.ID + "/tabs/requirements",
			p + "/defects/" + def.ID + "/tabs/requirements",
			p + "/test-cases/" + tc.ID + "/tabs/requirements",
		} {
			body := get(path)
			if !strings.Contains(body, "Status") || !hasBadge(body, reqStatus.Name) {
				t.Errorf("%s: no Status column with the %q badge", path, reqStatus.Name)
			}
		}
		// The child is a task: its category badge uses the task colour.
		if body := get(p + "/requirements/" + req.ID + "/tabs/children"); !strings.Contains(body, "bg-emerald-") || !strings.Contains(body, task.Title) {
			t.Error("children tab: task category badge is not emerald")
		}

		// Tables of defects: criticality and status as badges.
		for _, c := range []struct{ path, criticality string }{
			{p + "/milestones/" + m.ID + "/tabs/defects", "High"},
			{p + "/requirements/" + req.ID + "/tabs/defects", "High"},
			{p + "/components/" + comp.ID + "/tabs/defects", "High"},   // direct link
			{p + "/components/" + comp.ID + "/tabs/defects", "Medium"}, // via the capability
			{p + "/capabilities/" + capab.ID + "/tabs/defects", "Medium"},
		} {
			body := get(c.path)
			if !hasBadge(body, c.criticality) {
				t.Errorf("%s: criticality %q is not a badge", c.path, c.criticality)
			}
			if !strings.Contains(body, "Status") || !hasBadge(body, defStatus.Name) {
				t.Errorf("%s: no Status column with the %q badge", c.path, defStatus.Name)
			}
		}

		// Tables of test cases: execution mode as a badge.
		for _, path := range []string{
			p + "/requirements/" + req.ID + "/tabs/test-cases",
			p + "/components/" + comp.ID + "/tabs/test-cases",
			p + "/capabilities/" + capab.ID + "/tabs/test-cases",
			p + "/defects/" + def.ID + "/tabs/test-cases",
		} {
			if body := get(path); !hasBadge(body, "Automatic") {
				t.Errorf("%s: execution mode is not a badge", path)
			}
		}

		// Detail fields.
		if body := get(p + "/defects/" + def.ID); !hasBadge(body, "High") {
			t.Error("defect page: criticality is not a badge")
		}
		if body := get(p + "/test-cases/" + tc.ID); !hasBadge(body, "Automatic") {
			t.Error("test case page: execution mode is not a badge")
		}
	})
}
