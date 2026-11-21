// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wiekko.net/internal/access"
	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/model"
	"wiekko.net/internal/service"
	"wiekko.net/internal/store"
)

func strp(s string) *string { return &s }

// TestDocuments drives documents through the service layer on every engine
// (FR-50..FR-58, FR-108, FR-109).
func TestDocuments(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		ctx := context.Background()
		env := newIsolationEnv(t, driver, conn)
		svc, owner, pid := env.svc, env.admin, env.aviary
		docs := svc.Document

		create := func(in service.CreateDocumentInput) (*model.Document, error) {
			in.ProductID = pid
			return docs.CreateDocument(ctx, owner, in)
		}

		// Validation: the same rules as all other user-written text.
		for name, in := range map[string]service.CreateDocumentInput{
			"empty title":             {Title: "  "},
			"control in title":        {Title: "Sync\x1b[31m"},
			"invalid UTF-8":           {Title: "Sync", Body: "\xff\xfe"},
			"NUL in body":             {Title: "Sync", Body: "a\x00b"},
			"oversized body":          {Title: "Sync", Body: strings.Repeat("x", service.MaxDocumentBodyBytes+1)},
			"unknown format":          {Title: "Sync", Format: "html"},
			"unknown state":           {Title: "Sync", State: "binding"},
			"successor while current": {Title: "Sync", State: model.DocumentStateCurrent, SupersededByID: "x"},
		} {
			if _, err := create(in); err == nil {
				t.Errorf("%s: accepted", name)
			}
		}

		// Bodies are kept byte for byte, active content included: it is
		// neutralised when rendered, not rewritten when stored.
		body := "# Sync\r\n\r\n<script>alert(1)</script>\r\n"
		d, err := create(service.CreateDocumentInput{Title: "  Sync protocol ", Body: body, State: model.DocumentStateCurrent})
		if err != nil {
			t.Fatal(err)
		}
		if d.Identifier != fmt.Sprintf("DOC-%d", d.SequenceNumber) || d.Title != "Sync protocol" || d.Format != model.DocumentFormatMarkdown || d.Revision != 1 {
			t.Fatalf("created = %+v", d)
		}
		if got, _ := docs.GetDocument(ctx, owner, pid, d.ID); got.Body != body {
			t.Fatalf("body changed on the way: %q", got.Body)
		}

		// Only owners write documents; developers may read.
		if _, err := docs.CreateDocument(ctx, env.dev, service.CreateDocumentInput{ProductID: pid, Title: "x"}); !errors.Is(err, service.ErrForbidden) {
			t.Errorf("developer created a document: %v", err)
		}
		if _, err := docs.GetDocument(ctx, env.dev, pid, d.ID); err != nil {
			t.Errorf("developer cannot read: %v", err)
		}
		// Another product's view of the document does not exist.
		if _, err := docs.GetDocument(ctx, owner, env.finch, d.ID); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("document reached under another product: %v", err)
		}

		// Content changes make revisions; state changes do not.
		if d, err = docs.UpdateDocument(ctx, owner, pid, d.ID, service.UpdateDocumentInput{Body: strp("# Sync v2\n")}); err != nil || d.Revision != 2 {
			t.Fatalf("update body = %+v, %v", d, err)
		}
		if d, err = docs.UpdateDocument(ctx, owner, pid, d.ID, service.UpdateDocumentInput{State: strp(model.DocumentStateDraft)}); err != nil || d.Revision != 2 {
			t.Fatalf("update state = %+v, %v", d, err)
		}
		diff, err := docs.Diff(ctx, owner, pid, d.ID, 1, 2)
		if err != nil || !strings.Contains(diff.Unified(d.Identifier), "+# Sync v2") {
			t.Fatalf("diff = %+v, %v", diff, err)
		}
		if _, err := docs.Diff(ctx, owner, pid, d.ID, 1, 9); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("diff against a missing revision: %v", err)
		}

		// Restoring writes a new revision with the old content.
		if d, err = docs.RestoreRevision(ctx, owner, pid, d.ID, 1); err != nil || d.Revision != 3 || d.Body != body {
			t.Fatalf("restore = %+v, %v", d, err)
		}
		revs, _ := docs.ListRevisions(ctx, owner, pid, d.ID)
		if len(revs) != 3 {
			t.Fatalf("revisions = %d, want 3", len(revs))
		}

		// Superseding names another document of the product, never itself.
		next, err := create(service.CreateDocumentInput{Title: "Sync protocol v3", Format: model.DocumentFormatProto, Body: "syntax = \"proto3\";\n"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := docs.UpdateDocument(ctx, owner, pid, d.ID, service.UpdateDocumentInput{
			State: strp(model.DocumentStateSuperseded), SupersededByID: strp(d.ID)}); err == nil {
			t.Error("a document superseded itself")
		}
		if d, err = docs.UpdateDocument(ctx, owner, pid, d.ID, service.UpdateDocumentInput{
			State: strp(model.DocumentStateSuperseded), SupersededByID: strp(next.ID)}); err != nil || d.SupersededByID != next.ID {
			t.Fatalf("supersede = %+v, %v", d, err)
		}
		if d, err = docs.UpdateDocument(ctx, owner, pid, d.ID, service.UpdateDocumentInput{
			State: strp(model.DocumentStateCurrent)}); err != nil || d.SupersededByID != "" {
			t.Fatalf("leaving superseded kept the successor: %+v, %v", d, err)
		}

		// Identifiers resolve with the current prefix only.
		if id, err := docs.ResolveDocumentRef(ctx, owner, pid, strings.ToLower(next.Identifier)); err != nil || id != next.ID {
			t.Fatalf("resolve %s = %q, %v", next.Identifier, id, err)
		}
		if _, err := docs.ResolveDocumentRef(ctx, owner, pid, "DF-1"); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("a defect identifier resolved to a document: %v", err)
		}

		// Links: requirements, constraints and test cases, but no tasks.
		reqs, err := svc.Backlog.ListRequirements(ctx, owner, pid, store.RequirementFilter{})
		if err != nil {
			t.Fatal(err)
		}
		var reqID, taskID string
		for _, r := range reqs {
			if r.Kind == model.RequirementKindRequirement && reqID == "" {
				reqID = r.ID
			}
			if r.Kind == model.RequirementKindTask && taskID == "" {
				taskID = r.ID
			}
		}
		// The seed links documents to some constraints; take one without any,
		// so the check below sees only the document linked here.
		cons, _ := svc.Constraint.ListConstraints(ctx, owner, pid, store.ConstraintFilter{})
		var conID string
		for _, c := range cons {
			if got, err := docs.DocumentsFor(ctx, owner, pid, model.DocumentLinkConstraint, c.ID); err == nil && len(got) == 0 {
				conID = c.ID
				break
			}
		}
		tcs, _ := svc.Testing.ListTestCases(ctx, owner, pid)
		if reqID == "" || conID == "" || len(tcs) == 0 {
			t.Fatal("seed lacks requirements, unlinked constraints or test cases")
		}
		for _, l := range []struct{ typ, id string }{
			{model.DocumentLinkRequirement, reqID}, {model.DocumentLinkConstraint, conID}, {model.DocumentLinkTestCase, tcs[0].ID},
		} {
			if err := docs.LinkDocument(ctx, owner, pid, d.ID, l.typ, l.id); err != nil {
				t.Fatalf("link %s: %v", l.typ, err)
			}
			// Linking twice is harmless.
			if err := docs.LinkDocument(ctx, owner, pid, d.ID, l.typ, l.id); err != nil {
				t.Fatalf("relink %s: %v", l.typ, err)
			}
			if got, err := docs.DocumentsFor(ctx, env.dev, pid, l.typ, l.id); err != nil || len(got) != 1 || got[0].ID != d.ID {
				t.Fatalf("documents of %s = %+v, %v", l.typ, got, err)
			}
		}
		if taskID != "" {
			if err := docs.LinkDocument(ctx, owner, pid, d.ID, model.DocumentLinkRequirement, taskID); !errors.Is(err, service.ErrNotFound) {
				t.Errorf("a task was linked: %v", err)
			}
		}
		if err := docs.LinkDocument(ctx, owner, pid, d.ID, "task", reqID); err == nil {
			t.Error("link to an unknown target type accepted")
		}
		if links, _ := docs.ListLinks(ctx, owner, pid, d.ID); len(links) != 3 {
			t.Fatalf("links = %d, want 3", len(links))
		}
		if err := docs.UnlinkDocument(ctx, owner, pid, d.ID, model.DocumentLinkTestCase, tcs[0].ID); err != nil {
			t.Fatal(err)
		}

		// The three kinds of prefix stay apart (FR-56).
		if _, err := svc.Product.Update(ctx, owner, pid, service.UpdateProductInput{DocumentPrefix: strp("df")}); err == nil {
			t.Error("documents took the defect prefix")
		}
		if _, err := svc.Product.Update(ctx, owner, pid, service.UpdateProductInput{DocumentPrefix: strp("FR")}); err == nil {
			t.Error("documents took a category prefix")
		}
		if _, err := svc.Product.Update(ctx, owner, pid, service.UpdateProductInput{DefectPrefix: strp("DOC")}); err == nil {
			t.Error("defects took the document prefix")
		}
		if _, err := svc.Category.CreateCategory(ctx, owner, service.CreateCategoryInput{ProductID: pid, EntityType: "requirement", Name: "Docs", Prefix: "DOC"}); err == nil {
			t.Error("a category took the document prefix")
		}
		if _, err := svc.Product.Update(ctx, owner, pid, service.UpdateProductInput{DocumentPrefix: strp(" spec ")}); err != nil {
			t.Fatal(err)
		}
		if got, _ := docs.GetDocument(ctx, owner, pid, next.ID); got.Identifier != fmt.Sprintf("SPEC-%d", next.SequenceNumber) {
			t.Errorf("identifier after renaming = %q", got.Identifier)
		}

		// Comments need the documents scope on a token (SC-55).
		readOnly := access.WithRestriction(ctx, &access.Restriction{Grants: access.Grants{access.Discussion: access.Write}})
		if _, err := svc.Discussion.CreateComment(readOnly, owner, pid, model.OwnerTypeDocument, d.ID, "hi"); !errors.Is(err, service.ErrForbidden) {
			t.Errorf("comment without the documents scope: %v", err)
		}
		if _, err := svc.Discussion.CreateComment(ctx, env.dev, pid, model.OwnerTypeDocument, d.ID, "hi"); err != nil {
			t.Errorf("developer cannot comment on a document: %v", err)
		}

		entries, err := svc.Audit.ListByEntity(ctx, owner, pid, "document", d.ID)
		if err != nil || len(entries) == 0 {
			t.Errorf("no audit trail for documents: %v", err)
		}

		// Deleting removes revisions, links and comments.
		if err := docs.DeleteDocument(ctx, owner, pid, d.ID); err != nil {
			t.Fatal(err)
		}
		if got, _ := docs.DocumentsFor(ctx, owner, pid, model.DocumentLinkRequirement, reqID); len(got) != 0 {
			t.Errorf("links outlived the document: %+v", got)
		}
		if comments, _ := svc.Discussion.ListComments(ctx, owner, pid, model.OwnerTypeDocument, d.ID); len(comments) != 0 {
			t.Errorf("comments outlived the document: %+v", comments)
		}
	})
}
