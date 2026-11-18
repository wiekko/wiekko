package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// TestPriorityOrder checks how requirements, constraints and defects are
// listed (DF-2): the lowest priority number first, since 1 is the highest
// priority; items without a priority (0) last; then the oldest first; and
// items created in the same second by their identifier, not by chance.
func TestPriorityOrder(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		ctx := context.Background()
		exec := func(query string, args ...any) {
			t.Helper()
			if _, err := db.Exec(query, args...); err != nil {
				t.Fatalf("seed (%s): %v", query, err)
			}
		}
		const t0, t1 = "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"
		now := Now()
		pid, fr, ac := uuid.NewString(), uuid.NewString(), uuid.NewString()
		exec(`INSERT INTO products (id, name, description, workflow_mode, estimation_mode, created_at, updated_at)
		      VALUES (?, 'Aviary', '', 'kanban', 'none', ?, ?)`, pid, now, now)
		exec(`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
		      VALUES (?, ?, 'requirement', 'Functionality', 'FR', 0, ?)`, fr, pid, now)
		exec(`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
		      VALUES (?, ?, 'constraint', 'Architecture', 'AC', 0, ?)`, ac, pid, now)

		// Inserted out of order. The two ties share priority and timestamp, and
		// their IDs sort against their numbers, so only the number puts them in
		// order.
		items := []struct {
			id       string
			seq      int
			title    string
			priority int
			created  string
		}{
			{"00000000-0000-4000-8000-000000000005", 5, "Tie B", 3, t0},
			{"ffffffff-ffff-4fff-bfff-ffffffffffff", 4, "Tie A", 3, t0},
			{uuid.NewString(), 2, "Unprioritised", 0, t0},
			{uuid.NewString(), 3, "Highest", 1, t1},
			{uuid.NewString(), 1, "Second", 2, t0},
		}
		want := []string{"Highest", "Second", "Tie A", "Tie B", "Unprioritised"}

		for _, it := range items {
			exec(`INSERT INTO requirements (id, product_id, kind, category_id, title, sequence_number, priority, created_at, updated_at)
			      VALUES (?, ?, 'requirement', ?, ?, ?, ?, ?, ?)`, it.id, pid, fr, it.title, it.seq, it.priority, it.created, it.created)
			exec(`INSERT INTO constraints (id, product_id, category_id, sequence_number, title, priority, created_at, updated_at)
			      VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, it.id, pid, ac, it.seq, it.title, it.priority, it.created, it.created)
			exec(`INSERT INTO defects (id, product_id, sequence_number, title, description, criticality, priority, is_active, created_at, updated_at)
			      VALUES (?, ?, ?, ?, '', 'low', ?, FALSE, ?, ?)`, it.id, pid, it.seq, it.title, it.priority, it.created, it.created)
		}

		check := func(name string, got []string) {
			t.Helper()
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s order = %v, want %v", name, got, want)
			}
		}

		reqs, err := (&requirementStore{db: db}).ListByProductID(ctx, pid, RequirementFilter{})
		if err != nil {
			t.Fatal(err)
		}
		var titles []string
		for _, r := range reqs {
			titles = append(titles, r.Title)
		}
		check("requirements", titles)

		cons, err := (&constraintStore{db: db}).ListByProductID(ctx, pid, ConstraintFilter{})
		if err != nil {
			t.Fatal(err)
		}
		titles = nil
		for _, c := range cons {
			titles = append(titles, c.Title)
		}
		check("constraints", titles)

		ds := &defectStore{db: db}
		for _, p := range []ListParams{{}, {Sort: "priority", Dir: SortAsc}} {
			defects, err := ds.ListPageByProductID(ctx, pid, DefectFilter{}, p)
			if err != nil {
				t.Fatal(err)
			}
			titles = nil
			for _, d := range defects {
				titles = append(titles, d.Title)
			}
			// Sorting by the column skips created_at, so "Second" (priority 2)
			// still comes second and the ties follow their identifiers.
			check("defects "+p.Sort, titles)
		}
		defects, err := ds.ListPageByProductID(ctx, pid, DefectFilter{}, ListParams{Sort: "priority", Dir: SortDesc})
		if err != nil {
			t.Fatal(err)
		}
		if defects[0].Title != "Unprioritised" || defects[len(defects)-1].Title != "Highest" {
			t.Errorf("defects sorted by priority, descending: first %q, last %q", defects[0].Title, defects[len(defects)-1].Title)
		}
	})
}
