package router

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// TestAPIParityEndpoints covers the REST endpoints added so that everything
// the web UI changes can be changed through the API as well (FR-901):
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

		// Product owners invite with a role (FR-305).
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

		// A draft campaign's scope can be edited (FR-321).
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
