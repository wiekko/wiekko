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

// TestMilestoneTasksTab checks that a milestone lists its tasks on a Tasks
// tab of their own, with their own prefix and links to the task pages, and
// that the Requirements tab keeps to requirements (DF-4).
func TestMilestoneTasksTab(t *testing.T) {
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
		firstCategory := func(entityType string) *model.Category {
			t.Helper()
			cats, err := e.svc.Category.ListCategories(ctx, e.admin, pid, entityType)
			must(err)
			if len(cats) == 0 {
				t.Fatalf("no %s categories", entityType)
			}
			return cats[0]
		}
		reqCat := firstCategory(model.RequirementKindRequirement)
		taskCat := firstCategory(model.RequirementKindTask)

		m, err := e.svc.Milestone.CreateMilestone(ctx, e.admin, service.CreateMilestoneInput{ProductID: pid, Name: "v2"})
		must(err)
		newItem := func(kind, categoryID, title, milestoneID string) *model.Requirement {
			t.Helper()
			item, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{
				ProductID: pid, Kind: kind, CategoryID: categoryID, Title: title, MilestoneID: milestoneID,
			})
			must(err)
			return item
		}
		req := newItem(model.RequirementKindRequirement, reqCat.ID, "Record a sighting", m.ID)
		task := newItem(model.RequirementKindTask, taskCat.ID, "Write the sighting form", m.ID)
		spare := newItem(model.RequirementKindTask, taskCat.ID, "Write the export", "")
		spareReq := newItem(model.RequirementKindRequirement, reqCat.ID, "Export sightings", "")

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
		do := func(method, path string, form url.Values) (int, string) {
			t.Helper()
			r, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(form.Encode()))
			must(err)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.AddCookie(cookie)
			if method != http.MethodGet {
				r.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
				r.Header.Set("HX-Request", "true")
			}
			resp, err := e.client().Do(r)
			must(err)
			return resp.StatusCode, readBody(t, resp)
		}
		get := func(path string) string {
			t.Helper()
			status, body := do(http.MethodGet, path, nil)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status %d", path, status)
			}
			return body
		}
		base := "/products/" + pid + "/milestones/" + m.ID
		taskID := model.EntityIdentifier(taskCat.Prefix, task.SequenceNumber)
		reqID := model.EntityIdentifier(reqCat.Prefix, req.SequenceNumber)

		page := get(base)
		for _, want := range []string{`/tabs/tasks"`, `id="tab-panel-tasks"`, `id="tab-count-tasks"`} {
			if !strings.Contains(page, want) {
				t.Errorf("milestone page lacks %s", want)
			}
		}

		reqs := get(base + "/tabs/requirements")
		if !strings.Contains(reqs, reqID) || !strings.Contains(reqs, "/requirements/"+req.ID) {
			t.Error("Requirements tab lost the requirement")
		}
		if strings.Contains(reqs, task.Title) {
			t.Error("Requirements tab lists a task")
		}

		tasks := get(base + "/tabs/tasks")
		if !strings.Contains(tasks, taskID) {
			t.Errorf("Tasks tab: no %s", taskID)
		}
		if strings.Contains(tasks, "?-") {
			t.Error("Tasks tab: identifier without its prefix")
		}
		if !strings.Contains(tasks, `href="/products/`+pid+`/tasks/`+task.ID+`"`) {
			t.Error("Tasks tab does not link to the task page")
		}
		if strings.Contains(tasks, req.Title) {
			t.Error("Tasks tab lists a requirement")
		}

		// Linking offers and accepts tasks only.
		search := get(base + "/tasks/link-search")
		if !strings.Contains(search, spare.Title) || strings.Contains(search, spareReq.Title) {
			t.Error("task link search: wrong candidates")
		}
		if status, _ := do(http.MethodPost, base+"/tasks/link", url.Values{"requirement_id": {spareReq.ID}}); status != http.StatusNotFound {
			t.Errorf("linking a requirement as a task: status %d, want 404", status)
		}
		if status, _ := do(http.MethodPost, base+"/requirements/link", url.Values{"requirement_id": {spare.ID}}); status != http.StatusNotFound {
			t.Errorf("linking a task as a requirement: status %d, want 404", status)
		}
		status, body := do(http.MethodPost, base+"/tasks/link", url.Values{"requirement_id": {spare.ID}})
		if status != http.StatusOK || !strings.Contains(body, spare.Title) {
			t.Fatalf("linking a task: status %d", status)
		}
		got, err := e.svc.Backlog.GetRequirement(ctx, e.admin, pid, spare.ID)
		must(err)
		if got.MilestoneID != m.ID {
			t.Error("linked task has no milestone")
		}

		// Unlinking clears the milestone and leaves the other tab alone.
		if status, _ := do(http.MethodDelete, base+"/tasks/"+req.ID, nil); status != http.StatusNotFound {
			t.Errorf("unlinking a requirement on the Tasks tab: status %d, want 404", status)
		}
		status, body = do(http.MethodDelete, base+"/tasks/"+spare.ID, nil)
		if status != http.StatusOK || strings.Contains(body, spare.Title) {
			t.Fatalf("unlinking a task: status %d", status)
		}
		got, err = e.svc.Backlog.GetRequirement(ctx, e.admin, pid, spare.ID)
		must(err)
		if got.MilestoneID != "" {
			t.Error("unlinked task keeps its milestone")
		}
	})
}
