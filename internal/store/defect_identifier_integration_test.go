package store

import (
	"context"
	"testing"

	"nexmap.net/internal/model"
)

// TestDefectIdentifiers covers defect numbering and identifiers (FR-49).
func TestDefectIdentifiers(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		s := &defectStore{db: db}
		products := &productStore{db: db}
		f := seedDefectFixture(t, db, "Aviary")
		other := seedDefectFixture(t, db, "Finch")

		if n, err := s.NextSequenceNumber(ctx, f.productID); err != nil || n != 1 {
			t.Fatalf("first number = %d, %v; want 1", n, err)
		}
		first := f.addDefect(t, db, "Login fails", "high", f.statusOpen, "", "", 0, Now())
		second := f.addDefect(t, db, "Export hangs", "low", f.statusOpen, "", "", 0, Now())
		// Numbers are per product: the other product starts at 1 again.
		otherFirst := other.addDefect(t, db, "Elsewhere", "low", other.statusOpen, "", "", 0, Now())

		d, err := s.GetByID(ctx, second)
		if err != nil || d.SequenceNumber != 2 || d.Identifier != "DF-2" {
			t.Fatalf("second defect = %+v, %v; want DF-2", d, err)
		}
		if d, err := s.GetByID(ctx, otherFirst); err != nil || d.Identifier != "DF-1" {
			t.Fatalf("other product's first defect = %+v, %v; want DF-1", d, err)
		}
		if d, err := s.GetBySequence(ctx, f.productID, 1); err != nil || d.ID != first {
			t.Fatalf("GetBySequence(1) = %+v, %v; want %s", d, err, first)
		}
		if _, err := s.GetBySequence(ctx, f.productID, 3); err == nil {
			t.Fatal("GetBySequence found a number that was never issued")
		}

		// Numbers are unique per product.
		dup := &model.Defect{ID: "00000000-0000-0000-0000-000000000099", ProductID: f.productID, SequenceNumber: 2,
			Title: "dup", Criticality: "low", CreatedAt: Now(), UpdatedAt: Now()}
		if err := s.Create(ctx, dup); err == nil {
			t.Fatal("a second defect with number 2 was accepted")
		}

		// Renaming the prefix renames every identifier.
		p, err := products.GetByID(ctx, f.productID)
		if err != nil || p.DefectPrefix != model.DefaultDefectPrefix {
			t.Fatalf("product prefix = %q, %v; want the default", p.DefectPrefix, err)
		}
		p.DefectPrefix = "BUG"
		if err := products.Update(ctx, p); err != nil {
			t.Fatal(err)
		}
		list, err := s.ListByProductID(ctx, f.productID, DefectFilter{})
		if err != nil || len(list) != 2 {
			t.Fatalf("list = %v, %v", list, err)
		}
		for _, d := range list {
			if d.Identifier != model.EntityIdentifier("BUG", d.SequenceNumber) {
				t.Errorf("listed identifier %q, want the BUG prefix", d.Identifier)
			}
		}

		// The list searches and sorts by identifier.
		found, err := s.ListPageByProductID(ctx, f.productID, DefectFilter{}, ListParams{Search: "bug-2"})
		if err != nil || len(found) != 1 || found[0].ID != second {
			t.Fatalf("search bug-2 = %v, %v", found, err)
		}
		sorted, err := s.ListPageByProductID(ctx, f.productID, DefectFilter{}, ListParams{Sort: "identifier", Dir: SortDesc})
		if err != nil || len(sorted) != 2 || sorted[0].ID != second {
			t.Fatalf("sort by identifier desc = %v, %v", sorted, err)
		}
	})
}
