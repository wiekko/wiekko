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

// TestMilestoneSprintsTabFollowsWorkflowMode checks that a milestone page
// shows the Sprints tab only when the product uses Scrum (UR-157).
func TestMilestoneSprintsTabFollowsWorkflowMode(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		m, err := e.svc.Milestone.CreateMilestone(ctx, e.admin, service.CreateMilestoneInput{ProductID: e.product.ID, Name: "v2"})
		if err != nil {
			t.Fatal(err)
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
		page := func() string {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, e.srv.URL+"/products/"+e.product.ID+"/milestones/"+m.ID, nil)
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
				t.Fatalf("milestone page: status %d", resp.StatusCode)
			}
			return body
		}
		const tab, panel = `/tabs/sprints"`, `id="tab-panel-sprints"`

		// New products use Kanban.
		body := page()
		if strings.Contains(body, tab) || strings.Contains(body, panel) {
			t.Error("Kanban product: milestone page shows the Sprints tab")
		}
		if !strings.Contains(body, `/tabs/defects"`) {
			t.Error("Kanban product: milestone page lost its other tabs")
		}

		scrum := model.WorkflowModeScrum
		if _, err := e.svc.Product.Update(ctx, e.admin, e.product.ID, service.UpdateProductInput{WorkflowMode: &scrum}); err != nil {
			t.Fatal(err)
		}
		body = page()
		if !strings.Contains(body, tab) || !strings.Contains(body, panel) {
			t.Error("Scrum product: milestone page has no Sprints tab")
		}
		if !strings.Contains(body, `id="tab-count-sprints"`) {
			t.Error("Scrum product: Sprints tab has no count")
		}
	})
}
