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

// TestConstraintComplianceMenu drives the compliance menu of a constraint's
// page header (UR-168): owners set the compliance without the edit form and
// get the menu and the overview field back; others see the badge alone and
// are refused.
func TestConstraintComplianceMenu(t *testing.T) {
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
		cats, err := e.svc.Category.ListCategories(ctx, e.admin, pid, "constraint")
		must(err)
		if len(cats) == 0 {
			t.Fatal("no constraint category")
		}
		con, err := e.svc.Constraint.CreateConstraint(ctx, e.admin, service.CreateConstraintInput{
			ProductID: pid, CategoryID: cats[0].ID, Title: "All traffic uses TLS",
		})
		must(err)

		page := "/products/" + pid + "/constraints/" + con.ID
		action := page + "/compliance"
		get := adminPageGetter(t, e)
		if body := get(page, nil); !strings.Contains(body, `hx-post="`+action+`"`) || !strings.Contains(body, `id="constraint-compliance-field"`) {
			t.Fatal("the owner's constraint page offers no compliance menu")
		}

		resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
		var admin *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "wiekko_session" && c.Value != "" {
				admin = c
			}
		}
		if admin == nil {
			t.Fatal("no session cookie")
		}
		set := func(cookie *http.Cookie, state string) (int, string) {
			t.Helper()
			r, err := http.NewRequest(http.MethodPost, e.srv.URL+action, strings.NewReader(url.Values{"compliance_state": {state}}.Encode()))
			must(err)
			r.AddCookie(cookie)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
			r.Header.Set("HX-Request", "true")
			resp, err := e.client().Do(r)
			must(err)
			return resp.StatusCode, readBody(t, resp)
		}

		status, body := set(admin, model.ComplianceViolated)
		if status != http.StatusOK {
			t.Fatalf("setting the compliance: %d %q", status, body)
		}
		if !strings.Contains(body, `id="constraint-compliance"`) || !strings.Contains(body, `hx-swap-oob="true"`) || !strings.Contains(body, "Violated") {
			t.Error("the answer lacks the menu or the overview field with the new state")
		}
		got, err := e.svc.Constraint.GetConstraint(ctx, e.admin, pid, con.ID)
		must(err)
		if got.ComplianceState != model.ComplianceViolated {
			t.Errorf("compliance is %q, want violated", got.ComplianceState)
		}

		if status, _ := set(admin, "unknown"); status != http.StatusUnprocessableEntity {
			t.Errorf("an unknown state answers %d, want 422", status)
		}

		developer := e.invite("dev", model.RoleDeveloper)
		if status, _ := set(developer, model.ComplianceMet); status != http.StatusForbidden {
			t.Errorf("a developer setting the compliance gets %d, want 403", status)
		}
		got, err = e.svc.Constraint.GetConstraint(ctx, e.admin, pid, con.ID)
		must(err)
		if got.ComplianceState != model.ComplianceViolated {
			t.Error("a developer changed the compliance")
		}

		r, err := http.NewRequest(http.MethodGet, e.srv.URL+page, nil)
		must(err)
		r.AddCookie(developer)
		dresp, err := e.client().Do(r)
		must(err)
		if body := readBody(t, dresp); strings.Contains(body, action) || !strings.Contains(body, "Violated") {
			t.Error("a developer's constraint page offers the compliance menu, or shows no compliance")
		}
	})
}
