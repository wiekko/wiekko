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

	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
)

// TestDetailPageAssignees drives the assignees of a requirement's detail page
// (UR-171): shown in the Overview, changed from the header by owners, by
// developers for themselves on activated items, and recorded in the history.
func TestDetailPageAssignees(t *testing.T) {
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
		req, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{ProductID: pid, Title: "Record a sighting"})
		must(err)
		page := "/products/" + pid + "/requirements/" + req.ID
		action := page + "/assignees"

		dev := e.invite("dev", model.RoleDeveloper)
		viewer := e.invite("viewer", model.RoleViewer)
		resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
		var admin *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "nexmap_session" && c.Value != "" {
				admin = c
			}
		}
		if admin == nil {
			t.Fatal("no session cookie")
		}
		users, err := e.svc.User.List(ctx, store.UserFilter{})
		must(err)
		var devUser *model.User
		for _, u := range users {
			if u.Username == "dev" {
				devUser = u
			}
		}
		if devUser == nil {
			t.Fatal("no developer")
		}

		do := func(cookie *http.Cookie, method, path string, form url.Values) (int, string) {
			t.Helper()
			body := strings.NewReader("")
			if form != nil {
				body = strings.NewReader(form.Encode())
			}
			r, err := http.NewRequest(method, e.srv.URL+path, body)
			must(err)
			r.AddCookie(cookie)
			if method == http.MethodPost {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
				r.Header.Set("HX-Request", "true")
			}
			resp, err := e.client().Do(r)
			must(err)
			return resp.StatusCode, readBody(t, resp)
		}

		// The owner's page: an empty field, and a menu listing the members.
		_, body := do(admin, http.MethodGet, page, nil)
		if !strings.Contains(body, `id="item-assignees-field"`) || !strings.Contains(body, "Unassigned") || !strings.Contains(body, `hx-post="`+action+`"`) {
			t.Fatal("the owner's page lacks the assignee field or control")
		}

		status, body := do(admin, http.MethodPost, action, url.Values{"user_id": {devUser.ID}, "assign": {"true"}})
		if status != http.StatusOK || !strings.Contains(body, `id="item-assignees"`) || !strings.Contains(body, `hx-swap-oob="true"`) {
			t.Fatalf("assigning: %d %s", status, body)
		}
		as, _ := e.svc.Backlog.ListRequirementAssignees(ctx, e.admin, pid, req.ID)
		if len(as) != 1 || as[0].UserID != devUser.ID {
			t.Fatalf("assignees %v", as)
		}
		if _, body = do(admin, http.MethodGet, page+"/tabs/history", nil); !strings.Contains(body, "Assigned dev") {
			t.Errorf("history does not name the assignee:\n%s", body)
		}

		// A developer: no control on an item not activated for work, and the
		// server refuses; "Unassign me" once it is.
		if _, body = do(dev, http.MethodGet, page, nil); strings.Contains(body, `hx-post="`+action+`"`) {
			t.Error("a developer is offered the control on an item not activated")
		}
		if status, _ = do(dev, http.MethodPost, action, url.Values{"user_id": {devUser.ID}, "assign": {"false"}}); status != http.StatusForbidden {
			t.Errorf("developer on an item not activated: %d", status)
		}
		statuses, err := e.svc.Milestone.ListStatuses(ctx, e.admin, pid, model.RequirementKindRequirement)
		must(err)
		for _, s := range statuses {
			if s.IsActive {
				_, err = e.svc.Backlog.UpdateRequirement(ctx, e.admin, pid, req.ID, service.UpdateRequirementInput{StatusID: &s.ID})
				must(err)
				break
			}
		}
		if _, body = do(dev, http.MethodGet, page, nil); !strings.Contains(body, "Unassign me") {
			t.Error("a developer on an activated item is not offered to unassign themselves")
		}
		if status, _ = do(dev, http.MethodPost, action, url.Values{"user_id": {devUser.ID}, "assign": {"false"}}); status != http.StatusOK {
			t.Errorf("developer unassigning themselves: %d", status)
		}

		// A viewer sees the field but no control, and is refused.
		if _, body = do(viewer, http.MethodGet, page, nil); !strings.Contains(body, `id="item-assignees-field"`) || strings.Contains(body, `hx-post="`+action+`"`) {
			t.Error("a viewer's page shows no field or offers the control")
		}
		if status, _ = do(viewer, http.MethodPost, action, url.Values{"user_id": {devUser.ID}, "assign": {"true"}}); status != http.StatusForbidden {
			t.Errorf("viewer assigning: %d", status)
		}
	})
}
