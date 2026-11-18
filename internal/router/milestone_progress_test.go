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

// TestMilestoneOverviewShowsProgress checks that a milestone's Overview tab
// shows how many of its requirements, tasks and defects are done, overall
// and per kind, counting items in a closed status (UR-158).
func TestMilestoneOverviewShowsProgress(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		pid := e.product.ID
		m, err := e.svc.Milestone.CreateMilestone(ctx, e.admin, service.CreateMilestoneInput{ProductID: pid, Name: "v2"})
		if err != nil {
			t.Fatal(err)
		}
		other, err := e.svc.Milestone.CreateMilestone(ctx, e.admin, service.CreateMilestoneInput{ProductID: pid, Name: "v3"})
		if err != nil {
			t.Fatal(err)
		}

		statusIDs := func(entityType string) (open, closed string) {
			t.Helper()
			statuses, err := e.svc.Milestone.ListStatuses(ctx, e.admin, pid, entityType)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range statuses {
				if s.IsClosed && closed == "" {
					closed = s.ID
				}
				if !s.IsClosed && open == "" {
					open = s.ID
				}
			}
			if open == "" || closed == "" {
				t.Fatalf("%s statuses: need an open and a closed one", entityType)
			}
			return open, closed
		}
		reqOpen, reqClosed := statusIDs("requirement")
		defOpen, defClosed := statusIDs("defect")

		newItem := func(kind, title, statusID, milestoneID string) {
			t.Helper()
			if _, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{
				ProductID: pid, Kind: kind, Title: title, StatusID: statusID, MilestoneID: milestoneID,
			}); err != nil {
				t.Fatal(err)
			}
		}
		newDefect := func(title, statusID, milestoneID string) {
			t.Helper()
			if _, err := e.svc.Backlog.CreateDefect(ctx, e.admin, service.CreateDefectInput{
				ProductID: pid, Title: title, Criticality: "low", StatusID: statusID, MilestoneID: milestoneID,
			}); err != nil {
				t.Fatal(err)
			}
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
		get := func(path string) string {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(cookie)
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status %d", path, resp.StatusCode)
			}
			return body
		}
		pagePath := "/products/" + pid + "/milestones/" + m.ID
		tabPath := pagePath + "/tabs/overview"

		// Nothing assigned yet.
		if body := get(tabPath); !strings.Contains(body, "No requirements, tasks or defects are assigned") {
			t.Error("empty milestone: no hint that nothing is assigned")
		}

		newItem(model.RequirementKindRequirement, "Record a sighting", reqClosed, m.ID)
		newItem(model.RequirementKindRequirement, "Export sightings", reqOpen, m.ID)
		newItem(model.RequirementKindRequirement, "Share sightings", reqOpen, m.ID)
		newItem(model.RequirementKindTask, "Write the export", reqClosed, m.ID)
		// A kind with nothing assigned gets no bar.
		if body := get(tabPath); strings.Contains(body, `data-progress="defects"`) || !strings.Contains(body, `data-progress="tasks"`) {
			t.Error("milestone without defects: wrong per-kind bars")
		}
		newDefect("Sightings lose their date", defOpen, m.ID)
		// Items elsewhere don't count.
		newItem(model.RequirementKindRequirement, "Later work", reqClosed, other.ID)
		newDefect("Unplanned defect", defClosed, "")

		for _, path := range []string{pagePath, tabPath} {
			body := get(path)
			for _, want := range []string{
				`id="milestone-progress"`,
				"5 items", "2 items", // assigned, completed
				"40%",                       // 2 of 5, rounded down as on sprints
				"Requirements (1 of 3)", "33%", "bg-blue-500",
				"Tasks (1 of 1)", "100%", "bg-emerald-500",
				"Defects (0 of 1)", "bg-red-500",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("GET %s: progress lacks %q", path, want)
				}
			}
		}
	})
}
