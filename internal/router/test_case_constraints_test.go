package router

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// constraintFixture creates a constraint, a capability linked to it, a test
// case to link and one belonging to the capability (FR-150).
type constraintFixture struct {
	constraint *model.Constraint
	identifier string
	linked     *model.TestCase
	derived    *model.TestCase
}

func newConstraintFixture(t *testing.T, e *accountEnv) constraintFixture {
	t.Helper()
	ctx := context.Background()
	pid := e.product.ID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cats, err := e.svc.Category.ListCategories(ctx, e.admin, pid, "constraint")
	must(err)
	if len(cats) == 0 {
		t.Fatal("no constraint categories")
	}
	con, err := e.svc.Constraint.CreateConstraint(ctx, e.admin, service.CreateConstraintInput{
		ProductID: pid, CategoryID: cats[0].ID, Title: "Session cookies are HttpOnly",
	})
	must(err)
	capability, err := e.svc.Component.CreateCapability(ctx, e.admin, service.CreateCapabilityInput{ProductID: pid, Name: "Sessions"})
	must(err)
	must(e.svc.Constraint.LinkCapability(ctx, e.admin, pid, con.ID, capability.ID))
	linked, err := e.svc.Testing.CreateTestCase(ctx, e.admin, service.CreateTestCaseInput{
		ProductID: pid, Title: "Cookie flags", ExecutionMode: "automatic", TestIdentifier: "TestCookieFlags",
	})
	must(err)
	derived, err := e.svc.Testing.CreateTestCase(ctx, e.admin, service.CreateTestCaseInput{
		ProductID: pid, Title: "Session expiry", ExecutionMode: "manual", CapabilityID: capability.ID,
	})
	must(err)
	return constraintFixture{
		constraint: con,
		identifier: model.EntityIdentifier(cats[0].Prefix, con.SequenceNumber),
		linked:     linked,
		derived:    derived,
	}
}

// TestTestCaseConstraintAPI links a test case to a constraint over the REST
// API (FR-150, AC-86): by identifier, listed from both sides, the derived
// test cases marked with via, and the token scopes enforced.
func TestTestCaseConstraintAPI(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		f := newConstraintFixture(t, e)
		token := func(grants access.Grants) string {
			t.Helper()
			res, err := e.svc.PAT.Create(ctx, e.admin, service.CreatePATInput{Name: "t", Grants: grants})
			if err != nil {
				t.Fatal(err)
			}
			return res.RawToken
		}
		full := token(access.FullAccess())
		call := func(tok, method, path, body string) (int, string) {
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
			return resp.StatusCode, readBody(t, resp)
		}
		base := "/api/v1/products/" + e.product.ID
		links := base + "/test-cases/" + f.linked.ID + "/constraints"
		conTests := base + "/constraints/" + f.identifier + "/test-cases"

		testsOnly := token(access.Grants{access.Tests: access.Write})
		if status, body := call(testsOnly, http.MethodPost, links, `{"constraint_id":"`+f.identifier+`"}`); status != http.StatusForbidden {
			t.Errorf("linking without backlog:read: %d %s, want 403", status, body)
		}
		if status, body := call(full, http.MethodPost, links, `{}`); status != http.StatusBadRequest {
			t.Errorf("linking without constraint_id: %d %s, want 400", status, body)
		}
		if status, body := call(full, http.MethodPost, links, `{"constraint_id":"ZZ-99"}`); status != http.StatusNotFound {
			t.Errorf("linking an unknown constraint: %d %s, want 404", status, body)
		}
		for range 2 { // linking a linked pair again changes nothing
			if status, body := call(full, http.MethodPost, links, `{"constraint_id":"`+f.identifier+`"}`); status != http.StatusCreated {
				t.Fatalf("link by identifier: %d %s", status, body)
			}
		}

		status, body := call(full, http.MethodGet, links, "")
		if status != http.StatusOK || !strings.Contains(body, `"identifier":"`+f.identifier+`"`) {
			t.Fatalf("test case constraints: %d %s", status, body)
		}
		if n := strings.Count(body, `"identifier":`); n != 1 {
			t.Errorf("test case lists %d constraints, want 1: %s", n, body)
		}

		status, body = call(full, http.MethodGet, conTests, "")
		if status != http.StatusOK {
			t.Fatalf("constraint test cases: %d %s", status, body)
		}
		for _, want := range []string{`"title":"Cookie flags"`, `"title":"Session expiry"`, `"latest_result":null`, `"via":[]`, `"name":"Sessions"`} {
			if !strings.Contains(body, want) {
				t.Errorf("constraint test cases miss %s: %s", want, body)
			}
		}
		if status, _ := call(token(access.Grants{access.Backlog: access.Read}), http.MethodGet, conTests, ""); status != http.StatusForbidden {
			t.Errorf("constraint test cases without tests:read: %d, want 403", status)
		}

		if status, body := call(full, http.MethodDelete, links+"/"+f.identifier, ""); status != http.StatusOK {
			t.Fatalf("unlink by identifier: %d %s", status, body)
		}
		if _, body := call(full, http.MethodGet, links, ""); strings.Contains(body, f.identifier) {
			t.Errorf("unlinked constraint still listed: %s", body)
		}
	})
}

// TestTestCaseConstraintPages links from both detail pages (FR-150, UC-88):
// the tabs exist with counts, linking and unlinking from either side, the
// constraint tab showing the derived test case read-only, and creating a test
// case from the constraint page.
func TestTestCaseConstraintPages(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		f := newConstraintFixture(t, e)
		pid := e.product.ID
		get := adminPageGetter(t, e)

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
		send := func(method, path string, form url.Values) (int, string) {
			t.Helper()
			r, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			r.AddCookie(cookie)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
			r.Header.Set("HX-Request", "true")
			resp, err := e.client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			return resp.StatusCode, readBody(t, resp)
		}

		conPage := "/products/" + pid + "/constraints/" + f.constraint.ID
		tcPage := "/products/" + pid + "/test-cases/" + f.linked.ID
		if body := get(conPage, nil); !strings.Contains(body, conPage+"/tabs/test-cases") {
			t.Error("constraint page has no Test Cases tab")
		}
		if body := get(tcPage, nil); !strings.Contains(body, tcPage+"/tabs/constraints") {
			t.Error("test case page has no Constraints tab")
		}

		// Link from the test case page.
		if body := get(tcPage+"/constraints/link-search?q="+url.QueryEscape(strings.ToLower(f.identifier)), nil); !strings.Contains(body, f.constraint.Title) {
			t.Errorf("constraint search by identifier misses %s", f.identifier)
		}
		status, body := send(http.MethodPost, tcPage+"/constraints/link", url.Values{"constraint_id": {f.constraint.ID}})
		if status != http.StatusOK || !strings.Contains(body, f.constraint.Title) || !strings.Contains(body, `hx-swap-oob`) {
			t.Fatalf("link from the test case: %d %q", status, body)
		}

		tab := get(conPage+"/tabs/test-cases", nil)
		if !strings.Contains(tab, f.linked.Title) || !strings.Contains(tab, conPage+"/test-cases/"+f.linked.ID) {
			t.Error("constraint tab misses the linked test case or its unlink button")
		}
		if !strings.Contains(tab, `data-derived="test-cases"`) || !strings.Contains(tab, f.derived.Title) ||
			strings.Contains(tab, conPage+"/test-cases/"+f.derived.ID) {
			t.Error("constraint tab does not show the capability's test case read-only under Via capabilities")
		}
		if !strings.Contains(tab, `data-tab-count="2"`) {
			t.Error("constraint tab count is not linked plus derived")
		}

		// Unlink from the constraint page.
		if status, body := send(http.MethodDelete, conPage+"/test-cases/"+f.linked.ID, nil); status != http.StatusOK || strings.Contains(body, conPage+"/test-cases/"+f.linked.ID) {
			t.Fatalf("unlink from the constraint: %d", status)
		}
		if body := get(tcPage+"/tabs/constraints", nil); strings.Contains(body, f.constraint.Title) {
			t.Error("test case still lists the unlinked constraint")
		}

		// Create a test case from the constraint page.
		if body := get(conPage+"/test-cases/create-modal", nil); !strings.Contains(body, `hx-post="`+conPage+`/test-cases"`) {
			t.Error("create modal does not post to the constraint")
		}
		status, body = send(http.MethodPost, conPage+"/test-cases", url.Values{"title": {"Secure flag set"}, "execution_mode": {"manual"}})
		if status != http.StatusOK || !strings.Contains(body, "Secure flag set") {
			t.Fatalf("create and link: %d %q", status, body)
		}
	})
}
