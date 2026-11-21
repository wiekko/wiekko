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
)

// TestArchivePages drives archiving through the web UI (FR-155): the button
// on the detail page, the lists hiding archived items unless asked, the board
// leaving them out and traceability tabs marking them.
func TestArchivePages(t *testing.T) {
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

		var doneReq, doneDef string
		for et, id := range map[string]*string{"requirement": &doneReq, "defect": &doneDef} {
			list, err := e.svc.Milestone.ListStatuses(ctx, e.admin, pid, et)
			must(err)
			for _, s := range list {
				if s.IsClosed {
					*id = s.ID
				}
			}
		}
		m, err := e.svc.Milestone.CreateMilestone(ctx, e.admin, service.CreateMilestoneInput{ProductID: pid, Name: "v1"})
		must(err)
		req, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{
			ProductID: pid, Title: "Long finished feature", StatusID: doneReq, MilestoneID: m.ID,
		})
		must(err)
		def, err := e.svc.Backlog.CreateDefect(ctx, e.admin, service.CreateDefectInput{
			ProductID: pid, Title: "Long fixed crash", StatusID: doneDef,
		})
		must(err)

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
		do := func(method, path string) (int, string, http.Header) {
			t.Helper()
			r, err := http.NewRequest(method, e.srv.URL+path, nil)
			must(err)
			r.AddCookie(cookie)
			if method != http.MethodGet {
				r.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
				r.Header.Set("HX-Request", "true")
			}
			resp, err := e.client().Do(r)
			must(err)
			return resp.StatusCode, readBody(t, resp), resp.Header
		}
		page := func(path string) string {
			t.Helper()
			status, body, _ := do(http.MethodGet, path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: %d", path, status)
			}
			return body
		}

		reqURL := "/products/" + pid + "/requirements/" + req.ID
		defURL := "/products/" + pid + "/defects/" + def.ID
		if body := page(reqURL); !strings.Contains(body, reqURL+"/archive") {
			t.Error("detail page of a closed requirement offers no Archive button")
		}

		for _, item := range []string{reqURL, defURL} {
			status, body, h := do(http.MethodPost, item+"/archive")
			if status != http.StatusOK || h.Get("HX-Redirect") != item {
				t.Fatalf("POST %s/archive: %d %q, redirect %q", item, status, body, h.Get("HX-Redirect"))
			}
			body = page(item)
			if !strings.Contains(body, item+"/unarchive") || !strings.Contains(body, ">Archived<") {
				t.Errorf("%s: archived item shows no badge or Unarchive button", item)
			}
		}

		// Lists hide archived items unless asked, and say why they look empty.
		if body := page("/products/" + pid + "/requirements"); strings.Contains(body, req.Title) || !strings.Contains(body, "All requirements are archived") {
			t.Error("requirements list shows the archived requirement or no hint")
		}
		if body := page("/products/" + pid + "/requirements?archived=include"); !strings.Contains(body, req.Title) {
			t.Error("requirements list with archived=include misses the archived requirement")
		}
		if body := page("/products/" + pid + "/defects"); strings.Contains(body, def.Title) {
			t.Error("defects list shows the archived defect")
		}
		if body := page("/products/" + pid + "/defects?archived=only"); !strings.Contains(body, def.Title) {
			t.Error("defects list with archived=only misses the archived defect")
		}
		if body := page("/products/" + pid + "/board"); strings.Contains(body, req.Title) || strings.Contains(body, def.Title) {
			t.Error("the board shows archived items")
		}
		// Traceability keeps archived items, marked.
		if body := page("/products/" + pid + "/milestones/" + m.ID + "/tabs/requirements"); !strings.Contains(body, req.Title) || !strings.Contains(body, ">Archived<") {
			t.Error("milestone requirements tab misses the archived requirement or its badge")
		}

		// Reopening brings the requirement back.
		open := ""
		list, err := e.svc.Milestone.ListStatuses(ctx, e.admin, pid, model.RequirementKindRequirement)
		must(err)
		for _, s := range list {
			if !s.IsClosed {
				open = s.ID
				break
			}
		}
		_, err = e.svc.Backlog.UpdateRequirement(ctx, e.admin, pid, req.ID, service.UpdateRequirementInput{StatusID: &open})
		must(err)
		if body := page("/products/" + pid + "/requirements"); !strings.Contains(body, req.Title) {
			t.Error("reopened requirement is still hidden")
		}

		// An open item cannot be archived; the refusal is a readable message.
		status, body, _ := do(http.MethodPost, reqURL+"/archive")
		if status != http.StatusUnprocessableEntity || !strings.Contains(body, "closed status") {
			t.Errorf("archiving an open requirement: %d %q", status, body)
		}
	})
}

// TestArchiveRowActions drives the archive quick action of the table rows
// (UR-166): closed items offer Archive, archived ones Unarchive, open ones a
// disabled button, and a change made from the list reloads the list instead
// of opening the item.
func TestArchiveRowActions(t *testing.T) {
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
		closedStatus := func(et string) string {
			list, err := e.svc.Milestone.ListStatuses(ctx, e.admin, pid, et)
			must(err)
			for _, s := range list {
				if s.IsClosed {
					return s.ID
				}
			}
			t.Fatalf("no closed %s status", et)
			return ""
		}
		done, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{
			ProductID: pid, Title: "Finished feature", StatusID: closedStatus("requirement"),
		})
		must(err)
		open, err := e.svc.Backlog.CreateRequirement(ctx, e.admin, service.CreateRequirementInput{ProductID: pid, Title: "Open feature"})
		must(err)
		def, err := e.svc.Backlog.CreateDefect(ctx, e.admin, service.CreateDefectInput{
			ProductID: pid, Title: "Fixed crash", StatusID: closedStatus("defect"),
		})
		must(err)

		get := adminPageGetter(t, e)
		reqs := "/products/" + pid + "/requirements"
		body := get(reqs, nil)
		if !strings.Contains(body, reqs+"/"+done.ID+"/archive") {
			t.Error("a closed requirement's row offers no Archive action")
		}
		if strings.Contains(body, reqs+"/"+open.ID+"/archive") || !strings.Contains(body, "Only closed requirements can be archived") {
			t.Error("an open requirement's row does not show a disabled Archive action")
		}
		defs := "/products/" + pid + "/defects"
		if body := get(defs, nil); !strings.Contains(body, defs+"/"+def.ID+"/archive") {
			t.Error("a closed defect's row offers no Archive action")
		}

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
		quick := func(path string) {
			t.Helper()
			r, err := http.NewRequest(http.MethodPost, e.srv.URL+path, strings.NewReader("return=list"))
			must(err)
			r.AddCookie(cookie)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
			r.Header.Set("HX-Request", "true")
			resp, err := e.client().Do(r)
			must(err)
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK || resp.Header.Get("HX-Refresh") != "true" || resp.Header.Get("HX-Redirect") != "" {
				t.Fatalf("POST %s from the list: %d %q, refresh %q", path, resp.StatusCode, body, resp.Header.Get("HX-Refresh"))
			}
		}

		quick(reqs + "/" + done.ID + "/archive")
		quick(defs + "/" + def.ID + "/archive")
		if body := get(reqs+"?archived=include", nil); !strings.Contains(body, reqs+"/"+done.ID+"/unarchive") {
			t.Error("an archived requirement's row offers no Unarchive action")
		}
		if body := get(defs+"?archived=only", nil); !strings.Contains(body, defs+"/"+def.ID+"/unarchive") {
			t.Error("an archived defect's row offers no Unarchive action")
		}
		quick(reqs + "/" + done.ID + "/unarchive")
		if body := get(reqs, nil); !strings.Contains(body, "Finished feature") {
			t.Error("the requirement unarchived from its row is still hidden")
		}
	})
}
