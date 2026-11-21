// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
)

// Table list queries of the requirements, tasks (UR-144, UR-145) and test
// cases (UR-146) lists: sorting by meaning, search, facets and paging, on
// every engine.

type backlogListFixture struct {
	productID              string
	catLater, catEarlier   string // positions 2 and 1; "Alpha" sorts first by name
	statusOpen, statusDone string
	milestoneID, sprintID  string
}

func seedBacklogListFixture(t *testing.T, db *DB) backlogListFixture {
	t.Helper()
	now := Now()
	f := backlogListFixture{
		productID:   uuid.NewString(),
		catLater:    uuid.NewString(),
		catEarlier:  uuid.NewString(),
		statusOpen:  uuid.NewString(),
		statusDone:  uuid.NewString(),
		milestoneID: uuid.NewString(),
		sprintID:    uuid.NewString(),
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("seed (%s): %v", query, err)
		}
	}
	exec(`INSERT INTO products (id, name, description, workflow_mode, estimation_mode, created_at, updated_at)
	      VALUES (?, 'Aviary', '', 'scrum', 'none', ?, ?)`, f.productID, now, now)
	// By name "Alpha" comes first, by position "Zulu" does: a category sort
	// reading the name gets caught.
	exec(`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
	      VALUES (?, ?, 'requirement', 'Alpha', 'AR', 2, ?)`, f.catLater, f.productID, now)
	exec(`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
	      VALUES (?, ?, 'requirement', 'Zulu', 'ZR', 1, ?)`, f.catEarlier, f.productID, now)
	exec(`INSERT INTO statuses (id, product_id, entity_type, name, position, created_at)
	      VALUES (?, ?, 'requirement', 'Open', 1, ?)`, f.statusOpen, f.productID, now)
	exec(`INSERT INTO statuses (id, product_id, entity_type, name, position, created_at)
	      VALUES (?, ?, 'requirement', 'Done', 2, ?)`, f.statusDone, f.productID, now)
	exec(`INSERT INTO milestones (id, product_id, name, description, created_at, updated_at)
	      VALUES (?, ?, 'v1', '', ?, ?)`, f.milestoneID, f.productID, now, now)
	exec(`INSERT INTO sprints (id, product_id, name, status, start_date, end_date, created_at, updated_at)
	      VALUES (?, ?, 'Sprint 1', 'active', ?, ?, ?, ?)`, f.sprintID, f.productID, now, now, now, now)
	return f
}

func (f backlogListFixture) add(t *testing.T, s *requirementStore, kind, title, categoryID, statusID, milestoneID, sprintID string, priority int, created string) *model.Requirement {
	t.Helper()
	seq, err := s.NextSequenceNumber(context.Background(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	r := &model.Requirement{
		ID: uuid.NewString(), ProductID: f.productID, Kind: kind, CategoryID: categoryID,
		SequenceNumber: seq, Title: title, StatusID: statusID, MilestoneID: milestoneID,
		SprintID: sprintID, Priority: priority, CreatedAt: created, UpdatedAt: created,
	}
	if err := s.Create(context.Background(), r); err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	return r
}

func titles(items []*model.Requirement) []string {
	out := make([]string, len(items))
	for i, r := range items {
		out[i] = r.Title
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRequirementStoreTableList(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedBacklogListFixture(t, db)
		s := &requirementStore{db: db}
		ctx := context.Background()
		req := model.RequirementKindRequirement

		f.add(t, s, req, "Sync sightings", f.catLater, f.statusDone, f.milestoneID, "", 2, "2026-01-02T00:00:00Z")
		f.add(t, s, req, "Export 100% of rows", f.catEarlier, f.statusOpen, "", f.sprintID, 1, "2026-01-03T00:00:00Z")
		f.add(t, s, req, "Bird gallery", f.catLater, f.statusOpen, "", "", 3, "2026-01-01T00:00:00Z")
		archived := f.add(t, s, req, "Old import", f.catEarlier, f.statusDone, "", "", 4, "2026-01-04T00:00:00Z")
		if err := s.SetArchivedAt(ctx, archived.ID, Now(), Now()); err != nil {
			t.Fatal(err)
		}
		f.add(t, s, model.RequirementKindTask, "Upgrade Go", f.catLater, f.statusOpen, "", "", 1, "2026-01-05T00:00:00Z")

		reqs := RequirementFilter{Kind: req, Archived: ArchivedExclude}

		t.Run("sorts by meaning", func(t *testing.T) {
			tests := []struct {
				params ListParams
				want   []string
			}{
				{ListParams{}, []string{"Export 100% of rows", "Sync sightings", "Bird gallery"}}, // priority
				{ListParams{Sort: "title"}, []string{"Bird gallery", "Export 100% of rows", "Sync sightings"}},
				{ListParams{Sort: "title", Dir: SortDesc}, []string{"Sync sightings", "Export 100% of rows", "Bird gallery"}},
				// Zulu (position 1) before Alpha (position 2), ties by identifier.
				{ListParams{Sort: "category"}, []string{"Export 100% of rows", "Sync sightings", "Bird gallery"}},
				// Open (position 1) before Done (position 2).
				{ListParams{Sort: "status"}, []string{"Export 100% of rows", "Bird gallery", "Sync sightings"}},
				{ListParams{Sort: "created"}, []string{"Bird gallery", "Sync sightings", "Export 100% of rows"}},
				{ListParams{Sort: "identifier", Dir: SortDesc}, []string{"Bird gallery", "Export 100% of rows", "Sync sightings"}},
				// An unknown key falls back to the default order.
				{ListParams{Sort: "title; DROP TABLE requirements"}, []string{"Export 100% of rows", "Sync sightings", "Bird gallery"}},
			}
			for _, tt := range tests {
				got, err := s.ListPageByProductID(ctx, f.productID, reqs, tt.params)
				if err != nil {
					t.Fatal(err)
				}
				if !equalStrings(titles(got), tt.want) {
					t.Errorf("sort %q %q: got %v, want %v", tt.params.Sort, tt.params.Dir, titles(got), tt.want)
				}
			}
		})

		t.Run("searches the text columns", func(t *testing.T) {
			tests := []struct {
				search string
				want   int
			}{
				{"", 3},
				{"SIGHTINGS", 1},
				{"zr-2", 1},    // identifier
				{"alpha", 2},   // category name
				{"done", 1},    // status name
				{"100%", 1},    // % is literal
				{"_", 0},       // _ is literal
				{"upgrade", 0}, // tasks are another kind
			}
			for _, tt := range tests {
				got, err := s.ListPageByProductID(ctx, f.productID, reqs, ListParams{Search: tt.search})
				if err != nil {
					t.Fatal(err)
				}
				total, err := s.CountByProductID(ctx, f.productID, reqs, tt.search)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != tt.want || total != tt.want {
					t.Errorf("search %q: listed %d, counted %d, want %d", tt.search, len(got), total, tt.want)
				}
			}
		})

		t.Run("filters by facet", func(t *testing.T) {
			tests := []struct {
				name   string
				filter RequirementFilter
				want   int
			}{
				{"status", RequirementFilter{Kind: req, StatusID: f.statusOpen, Archived: ArchivedExclude}, 2},
				{"category", RequirementFilter{Kind: req, CategoryID: f.catEarlier, Archived: ArchivedExclude}, 1},
				{"milestone", RequirementFilter{Kind: req, MilestoneID: f.milestoneID}, 1},
				{"no milestone", RequirementFilter{Kind: req, MilestoneID: FilterNone, Archived: ArchivedExclude}, 2},
				{"sprint", RequirementFilter{Kind: req, SprintID: f.sprintID}, 1},
				{"no sprint", RequirementFilter{Kind: req, SprintID: FilterNone}, 3},
				{"archived only", RequirementFilter{Kind: req, Archived: ArchivedOnly}, 1},
				{"archived included", RequirementFilter{Kind: req}, 4},
				{"tasks", RequirementFilter{Kind: model.RequirementKindTask}, 1},
			}
			for _, tt := range tests {
				got, err := s.ListPageByProductID(ctx, f.productID, tt.filter, ListParams{})
				if err != nil {
					t.Fatal(err)
				}
				total, err := s.CountByProductID(ctx, f.productID, tt.filter, "")
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != tt.want || total != tt.want {
					t.Errorf("%s: listed %d, counted %d, want %d", tt.name, len(got), total, tt.want)
				}
			}
		})

		t.Run("pages", func(t *testing.T) {
			all, err := s.ListPageByProductID(ctx, f.productID, reqs, ListParams{Sort: "title"})
			if err != nil {
				t.Fatal(err)
			}
			second, err := s.ListPageByProductID(ctx, f.productID, reqs, ListParams{Sort: "title", Limit: 2, Offset: 2})
			if err != nil {
				t.Fatal(err)
			}
			if len(second) != 1 || second[0].ID != all[2].ID {
				t.Errorf("second page of 2: got %v, want [%s]", titles(second), all[2].Title)
			}
		})

		t.Run("ListByProductID keeps the backlog order", func(t *testing.T) {
			got, err := s.ListByProductID(ctx, f.productID, RequirementFilter{Kind: req})
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"Export 100% of rows", "Sync sightings", "Bird gallery", "Old import"}
			if !equalStrings(titles(got), want) {
				t.Errorf("got %v, want %v", titles(got), want)
			}
		})
	})
}

func TestTestCaseStoreTableList(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		now := Now()
		productID := uuid.NewString()
		compA, compB := uuid.NewString(), uuid.NewString()
		capab := uuid.NewString()
		levelUnit, levelE2E := uuid.NewString(), uuid.NewString()
		exec := func(query string, args ...any) {
			t.Helper()
			if _, err := db.Exec(query, args...); err != nil {
				t.Fatalf("seed (%s): %v", query, err)
			}
		}
		exec(`INSERT INTO products (id, name, description, workflow_mode, estimation_mode, created_at, updated_at)
		      VALUES (?, 'Aviary', '', 'kanban', 'none', ?, ?)`, productID, now, now)
		exec(`INSERT INTO components (id, product_id, name, created_at, updated_at) VALUES (?, ?, 'Server', ?, ?)`, compA, productID, now, now)
		exec(`INSERT INTO components (id, product_id, name, created_at, updated_at) VALUES (?, ?, 'App', ?, ?)`, compB, productID, now, now)
		exec(`INSERT INTO capabilities (id, product_id, name, created_at, updated_at) VALUES (?, ?, 'Sync', ?, ?)`, capab, productID, now, now)
		// By name "E2E" sorts before "Unit"; by position unit tests come first.
		exec(`INSERT INTO pyramid_levels (id, product_id, name, position, created_at) VALUES (?, ?, 'Unit', 1, ?)`, levelUnit, productID, now)
		exec(`INSERT INTO pyramid_levels (id, product_id, name, position, created_at) VALUES (?, ?, 'E2E', 2, ?)`, levelE2E, productID, now)

		s := &testCaseStore{db: db}
		ctx := context.Background()
		add := func(title, component, capability, level, mode, identifier, created string) {
			t.Helper()
			err := s.Create(ctx, &model.TestCase{
				ID: uuid.NewString(), ProductID: productID, ComponentID: component, CapabilityID: capability,
				Title: title, ExecutionMode: mode, TestIdentifier: identifier, PyramidLevelID: level,
				CreatedAt: created, UpdatedAt: created,
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		add("Login works", compA, capab, levelE2E, model.ExecutionModeManual, "", "2026-01-02T00:00:00Z")
		add("Parses sightings", compA, "", levelUnit, model.ExecutionModeAutomatic, "TestParseSightings", "2026-01-01T00:00:00Z")
		add("Gallery scrolls", compB, "", "", model.ExecutionModeManual, "", "2026-01-03T00:00:00Z")

		names := func(cases []*model.TestCase) []string {
			out := make([]string, len(cases))
			for i, tc := range cases {
				out[i] = tc.Title
			}
			return out
		}

		sorts := []struct {
			params ListParams
			want   []string
		}{
			{ListParams{}, []string{"Gallery scrolls", "Login works", "Parses sightings"}},
			{ListParams{Sort: "component"}, []string{"Gallery scrolls", "Login works", "Parses sightings"}},
			{ListParams{Sort: "capability", Dir: SortDesc}, []string{"Login works", "Gallery scrolls", "Parses sightings"}},
			// Unit (position 1), E2E (position 2), then no level.
			{ListParams{Sort: "pyramid_level"}, []string{"Parses sightings", "Login works", "Gallery scrolls"}},
			{ListParams{Sort: "mode"}, []string{"Parses sightings", "Gallery scrolls", "Login works"}},
			{ListParams{Sort: "created", Dir: SortDesc}, []string{"Gallery scrolls", "Login works", "Parses sightings"}},
		}
		for _, tt := range sorts {
			got, err := s.ListPageByProductID(ctx, productID, TestCaseFilter{}, tt.params)
			if err != nil {
				t.Fatal(err)
			}
			if !equalStrings(names(got), tt.want) {
				t.Errorf("sort %q %q: got %v, want %v", tt.params.Sort, tt.params.Dir, names(got), tt.want)
			}
		}

		filters := []struct {
			name   string
			filter TestCaseFilter
			search string
			want   int
		}{
			{"everything", TestCaseFilter{}, "", 3},
			{"component", TestCaseFilter{ComponentID: compA}, "", 2},
			{"capability", TestCaseFilter{CapabilityID: capab}, "", 1},
			{"no capability", TestCaseFilter{CapabilityID: FilterNone}, "", 2},
			{"pyramid level", TestCaseFilter{PyramidLevelID: levelUnit}, "", 1},
			{"no pyramid level", TestCaseFilter{PyramidLevelID: FilterNone}, "", 1},
			{"mode", TestCaseFilter{ExecutionMode: model.ExecutionModeManual}, "", 2},
			{"combined", TestCaseFilter{ComponentID: compA, ExecutionMode: model.ExecutionModeManual}, "", 1},
			{"search title", TestCaseFilter{}, "GALLERY", 1},
			{"search test identifier", TestCaseFilter{}, "testparse", 1},
			{"search component", TestCaseFilter{}, "server", 2},
			{"search capability", TestCaseFilter{}, "sync", 1},
			{"search pyramid level", TestCaseFilter{}, "e2e", 1},
			{"search and facet", TestCaseFilter{ComponentID: compB}, "server", 0},
		}
		for _, tt := range filters {
			got, err := s.ListPageByProductID(ctx, productID, tt.filter, ListParams{Search: tt.search})
			if err != nil {
				t.Fatal(err)
			}
			total, err := s.CountByProductID(ctx, productID, tt.filter, tt.search)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want || total != tt.want {
				t.Errorf("%s: listed %d, counted %d, want %d", tt.name, len(got), total, tt.want)
			}
		}

		page, err := s.ListPageByProductID(ctx, productID, TestCaseFilter{}, ListParams{Limit: 2, Offset: 2})
		if err != nil {
			t.Fatal(err)
		}
		if !equalStrings(names(page), []string{"Parses sightings"}) {
			t.Errorf("second page of 2: got %v", names(page))
		}
	})
}
