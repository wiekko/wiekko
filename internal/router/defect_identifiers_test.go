package router

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/service"
)

// TestDefectIdentifiersAPI drives defect identifiers over the REST API
// (FR-043): they resolve in paths and bodies, and follow the product's prefix.
func TestDefectIdentifiersAPI(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		grants := access.FullAccess()
		res, err := e.svc.PAT.Create(ctx, e.admin, service.CreatePATInput{Name: "t", Grants: grants})
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

		for _, title := range []string{"First", "Second"} {
			status, body := call(http.MethodPost, base+"/defects", `{"title":"`+title+`"}`)
			if status != http.StatusCreated {
				t.Fatalf("create defect: %d %s", status, body)
			}
		}
		status, body := call(http.MethodGet, base+"/defects/DF-2", "")
		if status != http.StatusOK || !strings.Contains(body, `"identifier":"DF-2"`) || !strings.Contains(body, `"Second"`) {
			t.Fatalf("GET DF-2: %d %s", status, body)
		}
		if status, _ := call(http.MethodGet, base+"/defects/DF-3", ""); status != http.StatusNotFound {
			t.Errorf("GET DF-3: status %d, want 404", status)
		}
		// A requirement identifier never resolves to a defect.
		if status, _ := call(http.MethodGet, base+"/defects/FR-1", ""); status != http.StatusNotFound {
			t.Errorf("GET FR-1 as a defect: status %d, want 404", status)
		}
		if status, body := call(http.MethodPatch, base+"/defects/df-1", `{"criticality":"high"}`); status != http.StatusOK {
			t.Errorf("PATCH df-1: %d %s", status, body)
		}

		// Comments accept a defect identifier as their owner.
		status, body = call(http.MethodPost, base+"/comments", `{"owner_type":"defect","owner_id":"DF-1","body":"seen again"}`)
		if status != http.StatusCreated {
			t.Fatalf("comment on DF-1: %d %s", status, body)
		}
		if status, body := call(http.MethodGet, base+"/comments?owner_type=defect&owner_id=DF-1", ""); status != http.StatusOK || !strings.Contains(body, "seen again") {
			t.Errorf("comments of DF-1: %d %s", status, body)
		}

		// The prefix is configurable, must not clash with a category prefix,
		// and renames every identifier.
		if status, _ := call(http.MethodPatch, base, `{"defect_prefix":"FR"}`); status < 400 {
			t.Errorf("defect prefix FR (a category prefix) accepted: status %d", status)
		}
		status, body = call(http.MethodPatch, base, `{"defect_prefix":"bug"}`)
		if status != http.StatusOK || !strings.Contains(body, `"defect_prefix":"BUG"`) {
			t.Fatalf("set prefix BUG: %d %s", status, body)
		}
		if status, body := call(http.MethodGet, base+"/defects/BUG-2", ""); status != http.StatusOK || !strings.Contains(body, `"identifier":"BUG-2"`) {
			t.Errorf("GET BUG-2: %d %s", status, body)
		}
		if status, _ := call(http.MethodGet, base+"/defects/DF-2", ""); status != http.StatusNotFound {
			t.Errorf("old prefix still resolves: status %d", status)
		}
		if status, _ := call(http.MethodPost, base+"/categories", `{"name":"Bugs","prefix":"BUG","entity_type":"task"}`); status < 400 {
			t.Errorf("category with the defect prefix accepted: status %d", status)
		}
	})
}
