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
	"nexmap.net/internal/service"
)

// TestDocumentsAPI drives documents over the REST API (FR-044..FR-052,
// FR-909, SC-404, SC-405).
func TestDocumentsAPI(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		token := func(grants access.Grants) string {
			t.Helper()
			res, err := e.svc.PAT.Create(ctx, e.admin, service.CreatePATInput{Name: "t", Grants: grants})
			if err != nil {
				t.Fatal(err)
			}
			return res.RawToken
		}
		full := token(access.FullAccess())
		do := func(tok, method, path, body string) (*http.Response, string) {
			t.Helper()
			req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+tok)
			if body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp, readBody(t, resp)
		}
		call := func(method, path, body string) (int, string) {
			t.Helper()
			resp, b := do(full, method, path, body)
			return resp.StatusCode, b
		}
		base := "/api/v1/products/" + e.product.ID

		// A body with markup is stored verbatim and comes back verbatim.
		body := "<script>alert(1)</script>\n[x](javascript:alert(1))\n"
		payload, _ := json.Marshal(map[string]string{"title": "Sync protocol", "body": body, "state": "current"})
		status, out := call(http.MethodPost, base+"/documents", string(payload))
		if status != http.StatusCreated || !strings.Contains(out, `"identifier":"DOC-1"`) || !strings.Contains(out, `"file_name":"DOC-1.md"`) {
			t.Fatalf("create: %d %s", status, out)
		}
		var got struct {
			Data map[string]any `json:"data"`
		}
		status, out = call(http.MethodGet, base+"/documents/doc-1", "")
		if err := json.Unmarshal([]byte(out), &got); status != http.StatusOK || err != nil || got.Data["body"] != body {
			t.Fatalf("GET DOC-1: %d %s", status, out)
		}
		if status, out := call(http.MethodPost, base+"/documents", `{"title":"x","format":"html"}`); status != http.StatusBadRequest {
			t.Errorf("unknown format: %d %s", status, out)
		}

		// The raw body is a plain-text attachment, never rendered.
		resp, raw := do(full, http.MethodGet, base+"/documents/DOC-1/raw", "")
		if resp.StatusCode != http.StatusOK || raw != body {
			t.Fatalf("raw: %d %q", resp.StatusCode, raw)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Errorf("raw Content-Type = %q", ct)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || !strings.Contains(cd, "DOC-1.md") {
			t.Errorf("raw Content-Disposition = %q", cd)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Error("raw body may be sniffed")
		}

		// Revisions, diff and restore.
		if status, out := call(http.MethodPatch, base+"/documents/DOC-1", `{"body":"v2\n"}`); status != http.StatusOK || !strings.Contains(out, `"revision":2`) {
			t.Fatalf("update: %d %s", status, out)
		}
		if status, out := call(http.MethodGet, base+"/documents/DOC-1/diff", ""); status != http.StatusOK || !strings.Contains(out, `+v2`) {
			t.Errorf("diff: %d %s", status, out)
		}
		if status, out := call(http.MethodGet, base+"/documents/DOC-1/diff?from=1&to=9", ""); status != http.StatusNotFound {
			t.Errorf("diff to a missing revision: %d %s", status, out)
		}
		if status, out := call(http.MethodGet, base+"/documents/DOC-1/revisions", ""); status != http.StatusOK || strings.Count(out, `"revision"`) != 2 {
			t.Errorf("revisions: %d %s", status, out)
		}
		if status, out := call(http.MethodGet, base+"/documents/DOC-1/revisions/1", ""); status != http.StatusOK || !strings.Contains(out, "alert") {
			t.Errorf("revision 1: %d %s", status, out)
		}
		if status, out := call(http.MethodPost, base+"/documents/DOC-1/restore", `{"revision":1}`); status != http.StatusOK || !strings.Contains(out, `"revision":3`) {
			t.Errorf("restore: %d %s", status, out)
		}

		// Links by identifier, read from both sides.
		status, out = call(http.MethodPost, base+"/requirements", `{"title":"Sync sightings"}`)
		if status != http.StatusCreated {
			t.Fatalf("create requirement: %d %s", status, out)
		}
		if status, out := call(http.MethodPost, base+"/documents/DOC-1/requirements", `{"target_id":"FR-1"}`); status != http.StatusCreated {
			t.Fatalf("link FR-1: %d %s", status, out)
		}
		if status, out := call(http.MethodGet, base+"/documents/DOC-1/requirements", ""); status != http.StatusOK || !strings.Contains(out, `"identifier":"FR-1"`) {
			t.Errorf("linked requirements: %d %s", status, out)
		}
		if status, out := call(http.MethodGet, base+"/requirements/FR-1/documents", ""); status != http.StatusOK || !strings.Contains(out, `"identifier":"DOC-1"`) {
			t.Errorf("documents of FR-1: %d %s", status, out)
		}
		if status, out := call(http.MethodGet, base+"/requirements/FR-1", ""); status != http.StatusOK ||
			!strings.Contains(out, `"documents":[{`) || !strings.Contains(out, `"title":"Sync protocol"`) {
			t.Errorf("FR-1 does not name its documents: %d %s", status, out)
		}

		// FR-909: without documents:read only the identifiers show.
		backlogOnly := token(access.Grants{access.Backlog: access.Read})
		resp, out = do(backlogOnly, http.MethodGet, base+"/requirements/FR-1", "")
		if resp.StatusCode != http.StatusOK || !strings.Contains(out, `"documents":[{"identifier":"DOC-1"}]`) {
			t.Errorf("FR-1 for a token without documents:read: %d %s", resp.StatusCode, out)
		}
		// SC-404/405: documents need their scope, links the linked item's too.
		if resp, _ := do(backlogOnly, http.MethodGet, base+"/documents/DOC-1", ""); resp.StatusCode != http.StatusForbidden {
			t.Errorf("document read without the documents scope: %d", resp.StatusCode)
		}
		docsOnly := token(access.Grants{access.Documents: access.Write})
		if resp, _ := do(docsOnly, http.MethodGet, base+"/documents/DOC-1", ""); resp.StatusCode != http.StatusOK {
			t.Errorf("document read with the documents scope: %d", resp.StatusCode)
		}
		if resp, _ := do(docsOnly, http.MethodPost, base+"/documents/DOC-1/requirements", `{"target_id":"FR-1"}`); resp.StatusCode != http.StatusForbidden {
			t.Errorf("link without backlog:read: %d", resp.StatusCode)
		}

		if status, out := call(http.MethodDelete, base+"/documents/DOC-1/requirements/FR-1", ""); status != http.StatusOK {
			t.Errorf("unlink FR-1: %d %s", status, out)
		}

		// The prefix is configurable and apart from the others.
		if status, _ := call(http.MethodPatch, base, `{"document_prefix":"DF"}`); status < 400 {
			t.Error("document prefix DF (the defect prefix) accepted")
		}
		if status, out := call(http.MethodPatch, base, `{"document_prefix":"spec"}`); status != http.StatusOK || !strings.Contains(out, `"document_prefix":"SPEC"`) {
			t.Fatalf("set prefix SPEC: %d %s", status, out)
		}
		if status, _ := call(http.MethodGet, base+"/documents/SPEC-1", ""); status != http.StatusOK {
			t.Errorf("GET SPEC-1: %d", status)
		}
		if status, _ := call(http.MethodDelete, base+"/documents/SPEC-1", ""); status != http.StatusNoContent {
			t.Errorf("delete: %d", status)
		}
	})
}
