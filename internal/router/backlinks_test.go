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

// TestStructureBacklinks checks that links stored between structure items
// and other items can be seen and followed from both ends (UR-159, UC-87):
// component names link to the component, a capability lists its
// constraints, a component its defects, and both their documents.
func TestStructureBacklinks(t *testing.T) {
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
		category := func(entityType, prefix string) string {
			t.Helper()
			cats, err := e.svc.Category.ListCategories(ctx, admin, pid, entityType)
			must(err)
			for _, c := range cats {
				if c.Prefix == prefix {
					return c.ID
				}
			}
			t.Fatalf("no %s category %s", entityType, prefix)
			return ""
		}

		comp, err := e.svc.Component.CreateComponent(ctx, admin, service.CreateComponentInput{ProductID: pid, Name: "Server"})
		must(err)
		capab, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, ComponentID: comp.ID, Name: "Backlog"})
		must(err)
		productLevel, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, Name: "Search"})
		must(err)
		req, err := e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{ProductID: pid, Kind: model.RequirementKindRequirement, Title: "Record a sighting"})
		must(err)
		must(e.svc.Component.LinkRequirementToCapability(ctx, admin, pid, capab.ID, req.ID))
		sc := category("constraint", "SC")
		linkedCon, err := e.svc.Constraint.CreateConstraint(ctx, admin, service.CreateConstraintInput{ProductID: pid, CategoryID: sc, Title: "All traffic uses TLS"})
		must(err)
		must(e.svc.Constraint.LinkCapability(ctx, admin, pid, linkedCon.ID, capab.ID))
		otherCon, err := e.svc.Constraint.CreateConstraint(ctx, admin, service.CreateConstraintInput{ProductID: pid, CategoryID: sc, Title: "Passwords are hashed"})
		must(err)
		compDefect, err := e.svc.Backlog.CreateDefect(ctx, admin, service.CreateDefectInput{ProductID: pid, Title: "Server crashes on start", Criticality: "high"})
		must(err)
		must(e.svc.Backlog.AddDefectLink(ctx, admin, pid, compDefect.ID, model.DefectLinkTargetComponent, comp.ID))
		capDefect, err := e.svc.Backlog.CreateDefect(ctx, admin, service.CreateDefectInput{ProductID: pid, Title: "Backlog loses order", Criticality: "low"})
		must(err)
		must(e.svc.Backlog.AddDefectLink(ctx, admin, pid, capDefect.ID, model.DefectLinkTargetCapability, capab.ID))
		_, err = e.svc.Document.CreateDocument(ctx, admin, service.CreateDocumentInput{ProductID: pid, Title: "Server protocol", Body: "x", ComponentID: comp.ID})
		must(err)
		_, err = e.svc.Document.CreateDocument(ctx, admin, service.CreateDocumentInput{ProductID: pid, Title: "Backlog schema", Body: "y", CapabilityID: capab.ID})
		must(err)

		signIn := func(username string) *http.Cookie {
			t.Helper()
			resp, _ := e.post("/login", url.Values{"username": {username}, "password": {goodPassword}})
			for _, c := range resp.Cookies() {
				if c.Name == "nexmap_session" && c.Value != "" {
					return c
				}
			}
			t.Fatalf("%s: no session cookie, status %d", username, resp.StatusCode)
			return nil
		}
		owner := signIn("root")
		developer := e.invite("dev", model.RoleDeveloper)
		do := func(cookie *http.Cookie, method, path string, form url.Values) (int, string) {
			t.Helper()
			req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(form.Encode()))
			must(err)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(cookie)
			if method != http.MethodGet {
				req.Header.Set("X-CSRF-Token", csrfToken(t, e, cookie))
				req.Header.Set("HX-Request", "true")
			}
			resp, err := e.client().Do(req)
			must(err)
			return resp.StatusCode, readBody(t, resp)
		}
		get := func(cookie *http.Cookie, path string) string {
			t.Helper()
			status, body := do(cookie, http.MethodGet, path, nil)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status %d", path, status)
			}
			return body
		}
		expect := func(what, body string, want ...string) {
			t.Helper()
			for _, w := range want {
				if !strings.Contains(body, w) {
					t.Errorf("%s: missing %q", what, w)
				}
			}
		}
		refuse := func(what, body string, unwanted ...string) {
			t.Helper()
			for _, u := range unwanted {
				if strings.Contains(body, u) {
					t.Errorf("%s: unexpected %q", what, u)
				}
			}
		}

		p := "/products/" + pid
		compURL := p + "/components/" + comp.ID
		capURL := p + "/capabilities/" + capab.ID
		compLink := `href="` + compURL + `"`

		// Component names link to the component.
		expect("capability page", get(owner, capURL), compLink, capURL+"/tabs/constraints", "Backlog schema")
		expect("capability overview", get(owner, capURL+"/tabs/overview"), compLink, "Backlog schema")
		refuse("capability overview", get(owner, capURL+"/tabs/overview"), "Server protocol")
		expect("product-level capability", get(owner, p+"/capabilities/"+productLevel.ID+"/tabs/overview"), "None (top-level)")
		expect("requirement capabilities", get(owner, p+"/requirements/"+req.ID+"/tabs/capabilities"), compLink)
		expect("defect capabilities", get(owner, p+"/defects/"+capDefect.ID+"/tabs/capabilities"), compLink)

		// A capability lists its constraints; owners link and unlink them.
		expect("capability constraints", get(owner, capURL+"/tabs/constraints"), linkedCon.Title, "SC-", "Unlink constraint", "Link existing")
		refuse("capability constraints", get(owner, capURL+"/tabs/constraints"), otherCon.Title)
		devTab := get(developer, capURL+"/tabs/constraints")
		expect("capability constraints for a developer", devTab, linkedCon.Title)
		refuse("capability constraints for a developer", devTab, "Unlink constraint", "Link existing")
		if status, _ := do(developer, http.MethodGet, capURL+"/constraints/link-modal", nil); status != http.StatusForbidden {
			t.Errorf("developer opens the constraint link modal: status %d, want 403", status)
		}
		if status, _ := do(developer, http.MethodPost, capURL+"/constraints/link", url.Values{"constraint_id": {otherCon.ID}}); status < 400 {
			t.Errorf("developer links a constraint: status %d", status)
		}
		search := get(owner, capURL+"/constraints/link-search?q=")
		expect("constraint search", search, otherCon.Title)
		refuse("constraint search", search, linkedCon.Title)
		if status, body := do(owner, http.MethodPost, capURL+"/constraints/link", url.Values{"constraint_id": {otherCon.ID}}); status != http.StatusOK {
			t.Fatalf("link a constraint: %d %s", status, body)
		}
		if status, body := do(owner, http.MethodDelete, capURL+"/constraints/"+linkedCon.ID, nil); status != http.StatusOK || strings.Contains(body, linkedCon.Title) || !strings.Contains(body, otherCon.Title) {
			t.Errorf("unlink a constraint: %d %s", status, body)
		}
		cons, err := e.svc.Constraint.ListCapabilityConstraints(ctx, admin, pid, capab.ID)
		must(err)
		if len(cons) != 1 || cons[0].ID != otherCon.ID {
			t.Errorf("capability constraints after link and unlink: %v", cons)
		}
		// The constraint's own Capabilities tab shows the same link.
		expect("constraint capabilities", get(owner, p+"/constraints/"+otherCon.ID+"/tabs/capabilities"), capURL)

		// A component lists its own defects and documents.
		expect("component page", get(owner, compURL), compURL+"/tabs/defects", "Server protocol")
		compOverview := get(owner, compURL+"/tabs/overview")
		expect("component overview", compOverview, "Server protocol")
		refuse("component overview", compOverview, "Backlog schema")
		defects := get(owner, compURL+"/tabs/defects")
		expect("component defects", defects, compDefect.Title, "Unlink defect")
		// The capability's defect is only derived (UR-160), not linked directly.
		if i := strings.Index(defects, `data-derived="defects"`); i < 0 || strings.Contains(defects[:i], capDefect.Title) {
			t.Errorf("component defects: %q should be listed via the capability only", capDefect.Title)
		}
		expect("component defect search", get(owner, compURL+"/defects/link-search?q=backlog"), capDefect.Title)
		if status, body := do(owner, http.MethodPost, compURL+"/defects/link", url.Values{"defect_id": {capDefect.ID}}); status != http.StatusOK {
			t.Fatalf("link a defect to the component: %d %s", status, body)
		}
		expect("component defects after linking", get(owner, compURL+"/tabs/defects"), capDefect.Title)
		expect("defect components", get(owner, p+"/defects/"+capDefect.ID+"/tabs/components"), compURL)
		_, linkIDs, err := e.svc.Backlog.ListLinkedDefects(ctx, admin, pid, model.DefectLinkTargetComponent, comp.ID)
		must(err)
		if status, body := do(owner, http.MethodDelete, compURL+"/defects/"+linkIDs[compDefect.ID], nil); status != http.StatusOK || strings.Contains(body, compDefect.Title) {
			t.Errorf("unlink a defect from the component: %d %s", status, body)
		}
		expect("defect create modal", get(owner, compURL+"/defects/create-modal"), `hx-post="`+compURL+`/defects"`)
		if status, body := do(owner, http.MethodPost, compURL+"/defects", url.Values{"title": {"Disk fills up"}, "criticality": {"medium"}}); status != http.StatusOK {
			t.Fatalf("create a defect on the component: %d %s", status, body)
		}
		expect("component defects after creating", get(owner, compURL+"/tabs/defects"), "Disk fills up")
		// The capability's Defects tab still works through the shared handlers.
		capDefects := get(owner, capURL+"/tabs/defects")
		expect("capability defects", capDefects, capDefect.Title, capURL+"/defects/link-modal")
	})
}

// TestDerivedLinks checks the links derived one step along the product
// structure (UR-160): a requirement shows its components via its
// capabilities, a task the capabilities of the requirement it refines, and a
// component the requirements and defects of its capabilities. Derived links
// are read-only, name what they come through, and never repeat a direct link.
func TestDerivedLinks(t *testing.T) {
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
		component := func(name string) *model.Component {
			t.Helper()
			c, err := e.svc.Component.CreateComponent(ctx, admin, service.CreateComponentInput{ProductID: pid, Name: name})
			must(err)
			return c
		}
		capability := func(name string, comp *model.Component) *model.Capability {
			t.Helper()
			in := service.CreateCapabilityInput{ProductID: pid, Name: name}
			if comp != nil {
				in.ComponentID = comp.ID
			}
			c, err := e.svc.Component.CreateCapability(ctx, admin, in)
			must(err)
			return c
		}
		item := func(kind, title, parentID string, caps ...*model.Capability) *model.Requirement {
			t.Helper()
			r, err := e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{ProductID: pid, Kind: kind, Title: title, ParentID: parentID})
			must(err)
			for _, c := range caps {
				must(e.svc.Component.LinkRequirementToCapability(ctx, admin, pid, c.ID, r.ID))
			}
			return r
		}
		defect := func(title string, links ...[2]string) *model.Defect {
			t.Helper()
			d, err := e.svc.Backlog.CreateDefect(ctx, admin, service.CreateDefectInput{ProductID: pid, Title: title, Criticality: "medium"})
			must(err)
			for _, l := range links {
				must(e.svc.Backlog.AddDefectLink(ctx, admin, pid, d.ID, l[0], l[1]))
			}
			return d
		}

		server, web := component("Server"), component("Web UI")
		backlog, search := capability("Backlog", server), capability("Search", server)
		board := capability("Board", web)
		audit := capability("Audit trail", nil) // product level: no component
		req := item(model.RequirementKindRequirement, "Record a sighting", "", backlog, search, board, audit)
		other := item(model.RequirementKindRequirement, "Drag cards", "", board)
		task := item(model.RequirementKindTask, "Write the handler", req.ID)
		subtask := item(model.RequirementKindTask, "Add the route", task.ID)
		loose := item(model.RequirementKindTask, "Tidy up", "")
		both := defect("Crash on save",
			[2]string{model.DefectLinkTargetComponent, server.ID}, [2]string{model.DefectLinkTargetCapability, backlog.ID})
		twice := defect("Slow queries",
			[2]string{model.DefectLinkTargetCapability, backlog.ID}, [2]string{model.DefectLinkTargetCapability, search.ID})
		elsewhere := defect("Card flickers", [2]string{model.DefectLinkTargetCapability, board.ID})

		cookie := func() *http.Cookie {
			resp, _ := e.post("/login", url.Values{"username": {"root"}, "password": {goodPassword}})
			for _, c := range resp.Cookies() {
				if c.Name == "nexmap_session" && c.Value != "" {
					return c
				}
			}
			t.Fatalf("no session cookie, status %d", resp.StatusCode)
			return nil
		}()
		get := func(path string) string {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
			must(err)
			req.AddCookie(cookie)
			resp, err := e.client().Do(req)
			must(err)
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status %d", path, resp.StatusCode)
			}
			return body
		}
		// section returns the part of a page from a data-derived marker on.
		section := func(body, marker string) string {
			t.Helper()
			i := strings.Index(body, `data-derived="`+marker+`"`)
			if i < 0 {
				t.Fatalf("no derived %s section", marker)
			}
			return body[i:]
		}
		p := "/products/" + pid
		capHref := func(c *model.Capability) string { return `href="` + p + "/capabilities/" + c.ID + `"` }
		compHref := func(c *model.Component) string { return `href="` + p + "/components/" + c.ID + `"` }

		// A requirement shows its components, each via its capabilities;
		// the product-level capability adds none.
		for _, page := range []string{p + "/requirements/" + req.ID, p + "/requirements/" + req.ID + "/tabs/overview"} {
			comps := section(get(page), "components")
			for _, want := range []string{compHref(server), compHref(web), capHref(backlog), capHref(search), capHref(board)} {
				if !strings.Contains(comps, want) {
					t.Errorf("%s: components missing %s", page, want)
				}
			}
			if strings.Contains(comps, capHref(audit)) {
				t.Errorf("%s: product-level capability listed under components", page)
			}
			if strings.Count(comps, compHref(server)) != 1 {
				t.Errorf("%s: Server listed %d times, want once", page, strings.Count(comps, compHref(server)))
			}
		}

		// A task, also below another task, inherits the requirement's
		// capabilities and their components; a top-level task has none.
		for _, tk := range []*model.Requirement{task, subtask} {
			page := get(p + "/tasks/" + tk.ID)
			caps := section(page, "capabilities")
			for _, want := range []string{capHref(backlog), capHref(audit), compHref(server), `href="` + p + "/requirements/" + req.ID + `"`} {
				if !strings.Contains(caps, want) {
					t.Errorf("task %s: inherited capabilities missing %s", tk.Title, want)
				}
			}
		}
		if strings.Contains(get(p+"/tasks/"+loose.ID), `data-derived="capabilities"`) {
			t.Error("a top-level task shows inherited capabilities")
		}

		// A component shows the requirements of its capabilities, once each.
		reqs := get(p + "/components/" + server.ID + "/tabs/requirements")
		if !strings.Contains(reqs, req.Title) || strings.Contains(reqs, other.Title) {
			t.Errorf("component requirements: want %q only, got %s", req.Title, reqs)
		}
		if strings.Count(reqs, req.Title) != 1 || !strings.Contains(reqs, capHref(backlog)) || !strings.Contains(reqs, capHref(search)) {
			t.Errorf("component requirements: want %q once, via Backlog and Search", req.Title)
		}
		if strings.Contains(reqs, "Unlink") {
			t.Error("derived requirements offer an unlink")
		}

		// A component's Defects tab lists defects of its capabilities below
		// its own, read-only, and a direct link wins over a derived one.
		defects := get(p + "/components/" + server.ID + "/tabs/defects")
		derived := section(defects, "defects")
		direct := defects[:strings.Index(defects, `data-derived="defects"`)]
		if !strings.Contains(direct, both.Title) || strings.Contains(derived, both.Title) {
			t.Errorf("defect linked directly and via a capability: want it under direct links only")
		}
		if strings.Count(derived, twice.Title) != 1 || !strings.Contains(derived, capHref(backlog)) || !strings.Contains(derived, capHref(search)) {
			t.Errorf("defect via two capabilities: want it once, via both")
		}
		if strings.Contains(defects, elsewhere.Title) {
			t.Error("defect of another component's capability listed")
		}
		if strings.Contains(derived, "Unlink defect") {
			t.Error("derived defects offer an unlink")
		}
	})
}
