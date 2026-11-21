// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"wiekko.net/internal/access"
	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/model"
	"wiekko.net/internal/service"
	"wiekko.net/internal/store"
)

// TestAPIParityEndpoints covers the REST endpoints added so that everything
// the web UI changes can be changed through the API as well (FR-133):
// product invitations, splitting statuses and a draft campaign's scope.
func TestAPIParityEndpoints(t *testing.T) {
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
		res, err := e.svc.PAT.Create(ctx, admin, service.CreatePATInput{Name: "t", Grants: access.FullAccess()})
		must(err)
		call := func(method, path, body string) (int, string) {
			t.Helper()
			req, err := http.NewRequest(method, e.srv.URL+"/api/v1/products/"+pid+path, strings.NewReader(body))
			must(err)
			req.Header.Set("Authorization", "Bearer "+res.RawToken)
			if body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := e.client().Do(req)
			must(err)
			return resp.StatusCode, readBody(t, resp)
		}

		// Product owners invite with a role (FR-77).
		status, body := call(http.MethodPost, "/invitations", `{"email":"new@example.com","role":"tester"}`)
		if status != http.StatusCreated || !strings.Contains(body, `"url"`) || !strings.Contains(body, `"role":"tester"`) {
			t.Fatalf("invite: %d %s", status, body)
		}
		if status, _ := call(http.MethodPost, "/invitations", `{"email":"x@example.com","role":"overlord"}`); status < 400 {
			t.Errorf("invite with an unknown role: status %d", status)
		}

		// Splitting the statuses gives defects a set of their own.
		reqStatuses, err := e.svc.Milestone.ListStatuses(ctx, admin, pid, model.RequirementKindRequirement)
		must(err)
		if status, body := call(http.MethodPost, "/statuses/split", ""); status != http.StatusOK {
			t.Fatalf("split statuses: %d %s", status, body)
		}
		p, err := e.svc.Product.GetByID(ctx, admin, pid)
		must(err)
		if p.StatusSync {
			t.Error("statuses still synced after the split")
		}
		if after, _ := e.svc.Milestone.ListStatuses(ctx, admin, pid, model.RequirementKindRequirement); len(after) != len(reqStatuses) {
			t.Errorf("requirement statuses changed by the split: %d -> %d", len(reqStatuses), len(after))
		}

		// An item moves below another one and back to the top level, with the
		// web UI's checks (FR-152, AC-86).
		newItem := func(kind, title string) *model.Requirement {
			t.Helper()
			r, err := e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{ProductID: pid, Kind: kind, Title: title})
			must(err)
			return r
		}
		upper := newItem(model.RequirementKindRequirement, "Upper")
		lower := newItem(model.RequirementKindRequirement, "Lower")
		task := newItem(model.RequirementKindTask, "Some task")
		move := func(item *model.Requirement, parentID string) (int, string) {
			t.Helper()
			return call(http.MethodPatch, "/requirements/"+item.ID, `{"parent_id":"`+parentID+`"}`)
		}
		if status, body := move(lower, upper.ID); status != http.StatusOK || !strings.Contains(body, `"parent_id":"`+upper.ID+`"`) {
			t.Fatalf("move below: %d %s", status, body)
		}
		for _, bad := range []struct {
			what       string
			item       *model.Requirement
			parentID   string
			wantStatus int
		}{
			{"own parent", upper, upper.ID, http.StatusBadRequest},
			{"cycle", upper, lower.ID, http.StatusBadRequest},
			{"requirement below a task", lower, task.ID, http.StatusBadRequest},
			{"unknown parent", lower, "no-such-item", http.StatusNotFound},
		} {
			if status, body := move(bad.item, bad.parentID); status != bad.wantStatus {
				t.Errorf("%s: %d %s, want %d", bad.what, status, body, bad.wantStatus)
			}
		}
		if status, body := move(task, lower.ID); status != http.StatusOK {
			t.Errorf("task below a requirement: %d %s", status, body)
		}
		if status, body := move(lower, ""); status != http.StatusOK || !strings.Contains(body, `"parent_id":""`) {
			t.Errorf("move to the top level: %d %s", status, body)
		}

		// A new requirement is linked to capabilities as it is created, as in
		// the New Requirement form (FR-156, AC-86).
		comp, err := e.svc.Component.CreateComponent(ctx, admin, service.CreateComponentInput{ProductID: pid, Name: "Server"})
		must(err)
		capA, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, ComponentID: comp.ID, Name: "Backlog"})
		must(err)
		capB, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, Name: "Reporting"})
		must(err)
		status, body = call(http.MethodPost, "/requirements", `{"title":"Linked on create","capability_ids":["`+capA.ID+`","`+capB.ID+`","`+capA.ID+`"]}`)
		if status != http.StatusCreated {
			t.Fatalf("create with capabilities: %d %s", status, body)
		}
		created, err := e.svc.Backlog.ListRequirements(ctx, admin, pid, store.RequirementFilter{})
		must(err)
		var linked *model.Requirement
		for _, r := range created {
			if r.Title == "Linked on create" {
				linked = r
			}
		}
		if linked == nil {
			t.Fatal("created requirement not found")
		}
		caps, err := e.svc.Component.ListLinkedCapabilities(ctx, admin, pid, linked.ID)
		must(err)
		if len(caps) != 2 {
			t.Errorf("linked capabilities: got %d, want 2", len(caps))
		}
		before := len(created)
		for _, bad := range []struct {
			what, body string
			wantStatus int
		}{
			{"unknown capability", `{"title":"Nope","capability_ids":["no-such-capability"]}`, http.StatusNotFound},
			{"task with capabilities", `{"kind":"task","title":"Nope","capability_ids":["` + capA.ID + `"]}`, http.StatusBadRequest},
		} {
			if status, body := call(http.MethodPost, "/requirements", bad.body); status != bad.wantStatus {
				t.Errorf("%s: %d %s, want %d", bad.what, status, body, bad.wantStatus)
			}
		}
		if after, _ := e.svc.Backlog.ListRequirements(ctx, admin, pid, store.RequirementFilter{}); len(after) != before {
			t.Errorf("a refused create left an item behind: %d -> %d", before, len(after))
		}

		// A draft campaign's scope can be edited (FR-94).
		tc1, err := e.svc.Testing.CreateTestCase(ctx, admin, service.CreateTestCaseInput{ProductID: pid, Title: "one", ExecutionMode: "manual"})
		must(err)
		tc2, err := e.svc.Testing.CreateTestCase(ctx, admin, service.CreateTestCaseInput{ProductID: pid, Title: "two", ExecutionMode: "manual"})
		must(err)
		camp, err := e.svc.Testing.CreateCampaign(ctx, admin, service.CreateCampaignInput{ProductID: pid, Name: "R1", TestCaseIDs: []string{tc1.ID}})
		must(err)
		base := "/campaigns/" + camp.ID
		if status, body := call(http.MethodPost, base+"/test-cases", `{"test_case_ids":["`+tc2.ID+`"]}`); status != http.StatusOK {
			t.Fatalf("add to scope: %d %s", status, body)
		}
		if _, body := call(http.MethodGet, base+"/results", ""); !strings.Contains(body, tc2.ID) {
			t.Errorf("added test case not in the campaign: %s", body)
		}
		if status, body := call(http.MethodDelete, base+"/test-cases/"+tc1.ID, ""); status != http.StatusOK {
			t.Fatalf("remove from scope: %d %s", status, body)
		}
		if _, body := call(http.MethodGet, base+"/results", ""); strings.Contains(body, tc1.ID) {
			t.Errorf("removed test case still in the campaign: %s", body)
		}
		if status, _ := call(http.MethodPost, base+"/test-cases", `{}`); status != http.StatusBadRequest {
			t.Errorf("empty scope addition: status %d, want 400", status)
		}
		// Once active, the scope is locked.
		_, err = e.svc.Testing.ActivateCampaign(ctx, admin, pid, camp.ID)
		must(err)
		if status, _ := call(http.MethodDelete, base+"/test-cases/"+tc2.ID, ""); status < 400 {
			t.Errorf("scope of an active campaign changed: status %d", status)
		}
	})
}
