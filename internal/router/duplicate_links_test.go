// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// TestDuplicateLinksAPI posts every link and assignment twice (FR-167): the
// second request succeeds and changes nothing, instead of failing on the
// database's unique constraint with a 500.
func TestDuplicateLinksAPI(t *testing.T) {
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
		req, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{ProductID: pid, Title: "Linked twice"})
		must(err)
		def, err := e.svc.Backlog.CreateDefect(ctx, e.admin, service.CreateDefectInput{ProductID: pid, Title: "Reported twice"})
		must(err)
		tc, err := e.svc.Testing.CreateTestCase(ctx, e.admin, service.CreateTestCaseInput{ProductID: pid, Title: "Run twice", ExecutionMode: "manual"})
		must(err)
		env, err := e.svc.Milestone.CreateRuntimeEnvironment(ctx, e.admin, pid, "linux")
		must(err)

		res, err := e.svc.PAT.Create(ctx, e.admin, service.CreatePATInput{Name: "t", Grants: access.FullAccess()})
		must(err)
		call := func(method, path, body string) (int, string) {
			t.Helper()
			r, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
			must(err)
			r.Header.Set("Authorization", "Bearer "+res.RawToken)
			if body != "" {
				r.Header.Set("Content-Type", "application/json")
			}
			resp, err := e.client().Do(r)
			must(err)
			return resp.StatusCode, readBody(t, resp)
		}
		count := func(path string) int {
			t.Helper()
			status, body := call(http.MethodGet, path, "")
			if status != http.StatusOK {
				t.Fatalf("GET %s: %d %s", path, status, body)
			}
			var out struct{ Data []json.RawMessage }
			must(json.Unmarshal([]byte(body), &out))
			return len(out.Data)
		}

		base := "/api/v1/products/" + pid
		for _, c := range []struct{ name, path, body string }{
			{"defect link", base + "/defects/" + def.ID + "/links", `{"target_type":"requirement","target_id":"` + req.ID + `"}`},
			{"test case requirement", base + "/test-cases/" + tc.ID + "/requirements", `{"requirement_id":"` + req.ID + `"}`},
			{"test case environment", base + "/test-cases/" + tc.ID + "/environments", `{"environment_id":"` + env.ID + `"}`},
			{"requirement assignee", base + "/requirements/" + req.ID + "/assignees", `{"user_id":"` + e.admin.ID + `"}`},
			{"defect assignee", base + "/defects/" + def.ID + "/assignees", `{"user_id":"` + e.admin.ID + `"}`},
		} {
			for i := 1; i <= 2; i++ {
				if status, body := call(http.MethodPost, c.path, c.body); status >= 300 {
					t.Errorf("%s, request %d: %d %s", c.name, i, status, body)
				}
			}
			if n := count(c.path); n != 1 {
				t.Errorf("%s: %d after linking twice, want 1", c.name, n)
			}
		}

		// Linking a test case to a requirement again with a type changes the
		// type; without one it keeps it.
		linkType := func() string {
			t.Helper()
			links, err := e.svc.Testing.ListTestCaseRequirements(ctx, e.admin, pid, tc.ID)
			must(err)
			if len(links) != 1 {
				t.Fatalf("%d requirement links, want 1", len(links))
			}
			return links[0].LinkType
		}
		path := base + "/test-cases/" + tc.ID + "/requirements"
		if got := linkType(); got != model.TestCaseLinkRequirement {
			t.Errorf("link type without one given = %q, want %q", got, model.TestCaseLinkRequirement)
		}
		for _, step := range []struct{ body, want string }{
			{`{"requirement_id":"` + req.ID + `","link_type":"definition_of_done"}`, model.TestCaseLinkDefinitionOfDone},
			{`{"requirement_id":"` + req.ID + `"}`, model.TestCaseLinkDefinitionOfDone},
			{`{"requirement_id":"` + req.ID + `","link_type":"requirement"}`, model.TestCaseLinkRequirement},
		} {
			if status, body := call(http.MethodPost, path, step.body); status != http.StatusCreated {
				t.Fatalf("POST %s: %d %s", step.body, status, body)
			}
			if got := linkType(); got != step.want {
				t.Errorf("after %s: link type %q, want %q", step.body, got, step.want)
			}
		}
		if status, body := call(http.MethodPost, path, `{"requirement_id":"`+req.ID+`","link_type":"bogus"}`); status != http.StatusBadRequest {
			t.Errorf("unknown link type: %d %s, want 400", status, body)
		}
	})
}
