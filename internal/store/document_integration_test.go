package store

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
)

// TestDocumentStore covers documents, their revisions and links
// (FR-044..FR-052) on every engine.
func TestDocumentStore(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		s := &documentStore{db: db}
		products := &productStore{db: db}
		f := seedDefectFixture(t, db, "Aviary")
		now := Now()

		newDoc := func(seq int, title, body, format, state string) *model.Document {
			t.Helper()
			d := &model.Document{ID: uuid.NewString(), ProductID: f.productID, SequenceNumber: seq,
				Title: title, Body: body, Format: format, State: state, Revision: 1, CreatedAt: now, UpdatedAt: now}
			rev := &model.DocumentRevision{ID: uuid.NewString(), DocumentID: d.ID, Revision: 1,
				Title: title, Body: body, Format: format, UserID: uuid.NewString(), CreatedAt: now}
			if err := s.Create(ctx, d, rev); err != nil {
				t.Fatal(err)
			}
			return d
		}

		if n, err := s.NextSequenceNumber(ctx, f.productID); err != nil || n != 1 {
			t.Fatalf("first number = %d, %v; want 1", n, err)
		}
		// Bodies are stored byte for byte, CRLF and trailing blanks included.
		body := "# Sync\r\n\r\nFrames are <b>binary</b>.  \r\n"
		proto := newDoc(1, "Sync protocol", body, model.DocumentFormatMarkdown, model.DocumentStateCurrent)
		schema := newDoc(2, "Sighting schema", `{"type":"object"}`, model.DocumentFormatJSON, model.DocumentStateDraft)

		got, err := s.GetByID(ctx, proto.ID)
		if err != nil || got.Identifier != "DOC-1" || got.Body != body || got.Revision != 1 {
			t.Fatalf("GetByID = %+v, %v", got, err)
		}
		if got, err := s.GetBySequence(ctx, f.productID, 2); err != nil || got.ID != schema.ID {
			t.Fatalf("GetBySequence(2) = %+v, %v", got, err)
		}
		dup := &model.Document{ID: uuid.NewString(), ProductID: f.productID, SequenceNumber: 2, Title: "dup",
			Format: model.DocumentFormatText, State: model.DocumentStateDraft, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err := s.Create(ctx, dup, &model.DocumentRevision{ID: uuid.NewString(), DocumentID: dup.ID, Revision: 1,
			Title: "dup", Format: model.DocumentFormatText, UserID: uuid.NewString(), CreatedAt: now}); err == nil {
			t.Fatal("a second document with number 2 was accepted")
		}
		if _, err := s.GetByID(ctx, dup.ID); err == nil {
			t.Fatal("a rejected document left a row behind")
		}

		// A content change adds a revision in the same write.
		got.Body = "# Sync v2\n"
		got.Revision = 2
		if err := s.Update(ctx, got, &model.DocumentRevision{ID: uuid.NewString(), DocumentID: got.ID, Revision: 2,
			Title: got.Title, Body: got.Body, Format: got.Format, UserID: uuid.NewString(), CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		revs, err := s.ListRevisions(ctx, proto.ID)
		if err != nil || len(revs) != 2 || revs[0].Revision != 2 || revs[1].Body != body {
			t.Fatalf("revisions = %+v, %v", revs, err)
		}
		if r, err := s.GetRevision(ctx, proto.ID, 1); err != nil || r.Body != body {
			t.Fatalf("revision 1 = %+v, %v", r, err)
		}
		if _, err := s.GetRevision(ctx, proto.ID, 3); err == nil {
			t.Fatal("found a revision that was never written")
		}

		// Lists leave the body out unless asked, filter and search by identifier.
		list, err := s.ListPageByProductID(ctx, f.productID, DocumentFilter{}, ListParams{})
		if err != nil || len(list) != 2 || list[0].Body != "" {
			t.Fatalf("list = %+v, %v", list, err)
		}
		if list, _ := s.ListPageByProductID(ctx, f.productID, DocumentFilter{WithBody: true}, ListParams{}); list[0].Body == "" {
			t.Fatal("WithBody left the body out")
		}
		if n, err := s.CountByProductID(ctx, f.productID, DocumentFilter{State: model.DocumentStateCurrent}, ""); err != nil || n != 1 {
			t.Fatalf("count current = %d, %v", n, err)
		}
		if found, err := s.ListPageByProductID(ctx, f.productID, DocumentFilter{}, ListParams{Search: "doc-2"}); err != nil ||
			len(found) != 1 || found[0].ID != schema.ID {
			t.Fatalf("search doc-2 = %+v, %v", found, err)
		}

		// Renaming the prefix renames every identifier.
		p, err := products.GetByID(ctx, f.productID)
		if err != nil || p.DocumentPrefix != model.DefaultDocumentPrefix {
			t.Fatalf("product prefix = %q, %v", p.DocumentPrefix, err)
		}
		p.DocumentPrefix = "SPEC"
		if err := products.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.GetByID(ctx, schema.ID); got.Identifier != "SPEC-2" {
			t.Fatalf("identifier after rename = %q", got.Identifier)
		}

		// Links to a constraint, listed from both sides.
		catID, conID := uuid.NewString(), uuid.NewString()
		if _, err := db.Exec(`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
			VALUES (?, ?, 'constraint', 'Security', 'SC', 0, ?)`, catID, f.productID, now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO constraints (id, product_id, category_id, sequence_number, title, created_at, updated_at)
			VALUES (?, ?, ?, 1, 'TLS', ?, ?)`, conID, f.productID, catID, now, now); err != nil {
			t.Fatal(err)
		}
		link := &model.DocumentLink{ID: uuid.NewString(), DocumentID: proto.ID, TargetType: model.DocumentLinkConstraint,
			TargetID: conID, CreatedAt: now}
		if err := s.Link(ctx, link); err != nil {
			t.Fatal(err)
		}
		if err := s.Link(ctx, &model.DocumentLink{ID: uuid.NewString(), DocumentID: proto.ID,
			TargetType: model.DocumentLinkConstraint, TargetID: conID, CreatedAt: now}); err == nil {
			t.Fatal("a duplicate link was accepted")
		}
		if err := s.Link(ctx, &model.DocumentLink{ID: uuid.NewString(), DocumentID: proto.ID,
			TargetType: "task", TargetID: conID, CreatedAt: now}); err == nil {
			t.Fatal("a link to an unknown target type was accepted")
		}
		links, err := s.ListLinks(ctx, proto.ID)
		if err != nil || len(links) != 1 || links[0].TargetID != conID || links[0].TargetType != model.DocumentLinkConstraint {
			t.Fatalf("links = %+v, %v", links, err)
		}
		docs, err := s.ListByTarget(ctx, model.DocumentLinkConstraint, conID)
		if err != nil || len(docs) != 1 || docs[0].Identifier != "SPEC-1" || docs[0].Body != "" {
			t.Fatalf("documents of constraint = %+v, %v", docs, err)
		}
		if err := s.Unlink(ctx, proto.ID, model.DocumentLinkConstraint, conID); err != nil {
			t.Fatal(err)
		}
		if links, _ := s.ListLinks(ctx, proto.ID); len(links) != 0 {
			t.Fatalf("links after unlink = %+v", links)
		}

		// Superseding: deleting the successor clears the pointer; deleting a
		// document removes its revisions.
		got, _ = s.GetByID(ctx, proto.ID)
		got.State = model.DocumentStateSuperseded
		got.SupersededByID = schema.ID
		if err := s.Update(ctx, got, nil); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(ctx, schema.ID); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.GetByID(ctx, proto.ID); got.SupersededByID != "" || got.Revision != 2 {
			t.Fatalf("after deleting the successor = %+v", got)
		}
		if err := s.Delete(ctx, proto.ID); err != nil {
			t.Fatal(err)
		}
		if revs, _ := s.ListRevisions(ctx, proto.ID); len(revs) != 0 {
			t.Fatalf("revisions outlived their document: %+v", revs)
		}
	})
}
