package router

import (
	"context"
	"database/sql"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nexmap.net/internal/access"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

var updateSpec = flag.Bool("update-spec", false, "rewrite testdata/specification.md")

// TestSpecificationExport exports a small product over the REST API
// (FR-137) and compares it with a golden file, which is what the export of
// a real product looks like.
func TestSpecificationExport(t *testing.T) {
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
		desc := "Birds, offline."
		_, err := e.svc.Product.Update(ctx, admin, pid, service.UpdateProductInput{Description: &desc})
		must(err)

		// Created out of name order: the export sorts by name.
		sync, err := e.svc.Component.CreateComponent(ctx, admin, service.CreateComponentInput{ProductID: pid, Name: "Sync engine", Description: "Keeps devices in step."})
		must(err)
		_, err = e.svc.Component.CreateComponent(ctx, admin, service.CreateComponentInput{ProductID: pid, Name: "accounts"})
		must(err)
		queue, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, ComponentID: sync.ID, Name: "Offline queue", Description: "Queues writes."})
		must(err)
		signIn, err := e.svc.Component.CreateCapability(ctx, admin, service.CreateCapabilityInput{ProductID: pid, Name: "Sign-in"})
		must(err)

		fr := category(model.RequirementKindRequirement, "FR")
		record, err := e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{ProductID: pid, CategoryID: fr,
			Title: "Record a sighting", Description: "A user records what they saw.\r\n", DefinitionOfDone: "- saved offline\n- synced later",
			Priority: 9})
		must(err)
		_, err = e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{ProductID: pid, CategoryID: fr, ParentID: record.ID,
			Title: "Attach a photo"})
		must(err)
		_, err = e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{ProductID: pid, CategoryID: category(model.RequirementKindRequirement, "SR"),
			ParentID: record.ID, Title: "Sightings are private"})
		must(err)
		_, err = e.svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{ProductID: pid, Kind: model.RequirementKindTask,
			CategoryID: category(model.RequirementKindTask, "MT"), ParentID: record.ID, Title: "Upgrade SQLite"})
		must(err)
		tls, err := e.svc.Constraint.CreateConstraint(ctx, admin, service.CreateConstraintInput{ProductID: pid,
			CategoryID: category("constraint", "SC"), Title: "All traffic uses TLS"})
		must(err)
		must(e.svc.Component.LinkRequirementToCapability(ctx, admin, pid, queue.ID, record.ID))
		must(e.svc.Constraint.LinkCapability(ctx, admin, pid, tls.ID, signIn.ID))

		mint := func(grants access.Grants) string {
			t.Helper()
			res, err := e.svc.PAT.Create(ctx, admin, service.CreatePATInput{Name: "t", Grants: grants})
			must(err)
			return res.RawToken
		}
		get := func(token, query string) (int, string, string) {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, e.srv.URL+"/api/v1/products/"+pid+"/specification"+query, nil)
			must(err)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := e.client().Do(req)
			must(err)
			return resp.StatusCode, resp.Header.Get("Content-Type"), readBody(t, resp)
		}
		reader := mint(access.Grants{access.Structure: access.Read, access.Backlog: access.Read})

		status, ctype, withTasks := get(reader, "?tasks=true")
		if status != http.StatusOK || !strings.HasPrefix(ctype, "text/markdown") {
			t.Fatalf("export: %d %s %s", status, ctype, withTasks)
		}
		golden := filepath.Join("testdata", "specification.md")
		if *updateSpec && driver == dbtest.SQLite {
			must(os.WriteFile(golden, []byte(withTasks), 0o644))
		}
		want, err := os.ReadFile(golden)
		must(err)
		if withTasks != string(want) {
			t.Errorf("export differs from %s:\n%s", golden, withTasks)
		}

		_, _, again := get(reader, "?tasks=true")
		if again != withTasks {
			t.Error("two exports of an unchanged product differ")
		}
		_, _, plain := get(reader, "")
		if strings.Contains(plain, "## Tasks") || strings.Contains(plain, "Upgrade SQLite") {
			t.Error("tasks exported without being asked for")
		}
		if status, _, _ := get(reader, "?tasks=maybe"); status != http.StatusBadRequest {
			t.Errorf("tasks=maybe: status %d, want 400", status)
		}
		// The export spans the structure and the backlog, so it needs both scopes.
		if status, _, _ := get(mint(access.Grants{access.Backlog: access.Read}), ""); status != http.StatusForbidden {
			t.Errorf("export without the structure scope: status %d", status)
		}

		// Documents (FR-139): listed for a token that may read them, current
		// ones by default, drafts on request; left out silently otherwise.
		protocol, err := e.svc.Document.CreateDocument(ctx, admin, service.CreateDocumentInput{ProductID: pid,
			Title: "Sync protocol", Body: "# Sync\n", State: model.DocumentStateCurrent})
		must(err)
		_, err = e.svc.Document.CreateDocument(ctx, admin, service.CreateDocumentInput{ProductID: pid,
			Title: "Sighting schema", Format: model.DocumentFormatJSON, Body: "{}\n"})
		must(err)
		must(e.svc.Document.LinkDocument(ctx, admin, pid, protocol.ID, model.DocumentLinkRequirement, record.ID))
		must(e.svc.Document.LinkDocument(ctx, admin, pid, protocol.ID, model.DocumentLinkConstraint, tls.ID))
		if _, _, out := get(reader, ""); strings.Contains(out, "## Documents") || strings.Contains(out, "DOC-1") {
			t.Errorf("documents exported for a token without the documents scope:\n%s", out)
		}
		docReader := mint(access.Grants{access.Structure: access.Read, access.Backlog: access.Read, access.Documents: access.Read})
		_, _, out := get(docReader, "")
		for _, want := range []string{"## Documents", "* **DOC-1**: Sync protocol", "File: `DOC-1.md`. Format: markdown. Revision 1.",
			"Requirements: FR-1. Constraints: SC-1.", "Capabilities: Offline queue. Documents: DOC-1."} {
			if !strings.Contains(out, want) {
				t.Errorf("export lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "DOC-2") {
			t.Error("a draft document was exported without drafts=true")
		}
		if _, _, out := get(docReader, "?drafts=true"); !strings.Contains(out, "* **DOC-2**: Sighting schema") || !strings.Contains(out, "State: draft.") {
			t.Errorf("drafts=true does not list the draft:\n%s", out)
		}
	})
}
