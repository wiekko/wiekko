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
	"wiekko.net/internal/service"
)

// TestArchiveAPI archives a requirement and a defect over the REST API
// (FR-155, AC-86): PATCH archived, list endpoints leaving archived items out
// unless asked, and single reads still serving them.
func TestArchiveAPI(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		res, err := e.svc.PAT.Create(ctx, e.admin, service.CreatePATInput{Name: "t", Grants: access.FullAccess()})
		if err != nil {
			t.Fatal(err)
		}
		call := func(method, path, body string) (int, string) {
			t.Helper()
			req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+res.RawToken)
			if body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp.StatusCode, readBody(t, resp)
		}
		base := "/api/v1/products/" + e.product.ID
		done := map[string]string{}
		for _, et := range []string{"requirement", "defect"} {
			list, err := e.svc.Milestone.ListStatuses(ctx, e.admin, e.product.ID, et)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range list {
				if s.IsClosed {
					done[et] = s.ID
				}
			}
		}

		for _, it := range []struct{ collection, create, ref, entity string }{
			{"requirements", `{"title":"Shipped feature"}`, "FR-1", "requirement"},
			{"defects", `{"title":"Fixed crash"}`, "DF-1", "defect"},
		} {
			if status, body := call(http.MethodPost, base+"/"+it.collection, it.create); status != http.StatusCreated {
				t.Fatalf("create %s: %d %s", it.ref, status, body)
			}
			item := base + "/" + it.collection + "/" + it.ref

			if status, body := call(http.MethodPatch, item, `{"archived":true}`); status != http.StatusBadRequest {
				t.Errorf("archiving open %s: %d %s, want 400", it.ref, status, body)
			}
			if status, body := call(http.MethodPatch, item, `{"status_id":"`+done[it.entity]+`"}`); status != http.StatusOK {
				t.Fatalf("close %s: %d %s", it.ref, status, body)
			}
			status, body := call(http.MethodPatch, item, `{"archived":true}`)
			if status != http.StatusOK || !strings.Contains(body, `"archived":true`) {
				t.Fatalf("archive %s: %d %s", it.ref, status, body)
			}

			list := base + "/" + it.collection
			for query, want := range map[string]bool{"": false, "?archived=include": true, "?archived=only": true} {
				status, body := call(http.MethodGet, list+query, "")
				if status != http.StatusOK {
					t.Fatalf("GET %s%s: %d %s", it.collection, query, status, body)
				}
				if got := strings.Contains(body, `"identifier":"`+it.ref+`"`); got != want {
					t.Errorf("GET %s%s lists %s: %v, want %v", it.collection, query, it.ref, got, want)
				}
			}
			if status, _ := call(http.MethodGet, list+"?archived=maybe", ""); status != http.StatusBadRequest {
				t.Errorf("GET %s?archived=maybe: %d, want 400", it.collection, status)
			}
			if status, body := call(http.MethodGet, item, ""); status != http.StatusOK || !strings.Contains(body, `"archived":true`) {
				t.Errorf("GET archived %s: %d %s", it.ref, status, body)
			}

			status, body = call(http.MethodPatch, item, `{"archived":false}`)
			if status != http.StatusOK || !strings.Contains(body, `"archived":false`) {
				t.Errorf("unarchive %s: %d %s", it.ref, status, body)
			}
		}
	})
}
