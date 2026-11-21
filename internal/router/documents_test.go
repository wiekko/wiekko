// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

// TestDocumentsAPI drives documents over the REST API (FR-50..FR-58,
// FR-141, SC-54, SC-55).
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

		// FR-141: without documents:read only the identifiers show.
		backlogOnly := token(access.Grants{access.Backlog: access.Read})
		resp, out = do(backlogOnly, http.MethodGet, base+"/requirements/FR-1", "")
		if resp.StatusCode != http.StatusOK || !strings.Contains(out, `"documents":[{"identifier":"DOC-1"}]`) {
			t.Errorf("FR-1 for a token without documents:read: %d %s", resp.StatusCode, out)
		}
		// SC-54/405: documents need their scope, links the linked item's too.
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

// TestDocumentFormLargeUnicodeBody submits the document web form the way a
// browser does — multipart, with the CSRF token as a form field — with a
// body near the cap that is mostly non-ASCII. URL-encoded, it would exceed
// the request body limit and fail the CSRF check.
func TestDocumentFormLargeUnicodeBody(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
		var cookie *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "nexmap_session" && c.Value != "" {
				cookie = c
			}
		}
		if cookie == nil {
			t.Fatalf("no session cookie, status %d", resp.StatusCode)
		}
		csrf := csrfToken(t, e, cookie)
		base := "/products/" + e.product.ID + "/documents"

		get := func(path string) (*http.Response, string) {
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
			return resp, readBody(t, resp)
		}
		submit := func(title, body string) (*http.Response, string) {
			t.Helper()
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			for _, f := range [][2]string{{"_csrf", csrf}, {"title", title}, {"format", "markdown"}, {"state", "draft"}, {"body", body}} {
				if err := mw.WriteField(f[0], f[1]); err != nil {
					t.Fatal(err)
				}
			}
			if err := mw.Close(); err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(http.MethodPost, e.srv.URL+base, &buf)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.AddCookie(cookie)
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp, readBody(t, resp)
		}

		// The form itself must be multipart.
		if _, page := get(base + "/new"); !strings.Contains(page, `enctype="multipart/form-data"`) {
			t.Fatal("the document form is not multipart")
		}

		// About 500 KiB of emoji, CJK, Cyrillic and umlauts, with the CRLF
		// line breaks browsers send.
		line := "Grüße 🐦🪶 鳥の観察 Наблюдение за птицами — ✓\r\n"
		lf := strings.ReplaceAll(line, "\r\n", "\n")
		submitted := strings.Repeat(line, 500*1024/len(lf))
		want := strings.ReplaceAll(submitted, "\r\n", "\n")
		if len(want) > service.MaxDocumentBodyBytes {
			t.Fatalf("test body of %d bytes exceeds the cap", len(want))
		}
		if n := len(url.Values{"body": {submitted}}.Encode()); n <= middleware.DefaultMaxBody {
			t.Fatalf("URL-encoded test body is only %d bytes; it should exceed the body limit", n)
		}

		resp, out := submit("Field guide", submitted)
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("create: status %d, body: %.300s", resp.StatusCode, out)
		}
		loc := resp.Header.Get("Location")
		if !strings.HasPrefix(loc, base+"/") {
			t.Fatalf("create redirects to %q", loc)
		}
		resp, raw := get(loc + "/raw")
		if resp.StatusCode != http.StatusOK || raw != want {
			t.Fatalf("raw: status %d, %d bytes, want %d bytes unchanged", resp.StatusCode, len(raw), len(want))
		}

		// Over the cap: the form comes back with the size error, not a 403.
		resp, out = submit("Too long", strings.Repeat("a", service.MaxDocumentBodyBytes+1))
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(out, "larger than 512 KiB") {
			t.Errorf("over the cap: status %d, body: %.300s", resp.StatusCode, out)
		}
	})
}
