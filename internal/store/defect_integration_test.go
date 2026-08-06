package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// defectFixture seeds a product with two statuses, two environments, a sprint
// and a set of defects chosen to make ordering mistakes visible: the
// criticalities are deliberately in an order where alphabetical and severity
// sorting disagree, and several rows share a priority so paging has ties to
// break.
type defectFixture struct {
	productID              string
	statusOpen, statusDone string // positions 1 and 2
	envLinux, envMacOS     string
	sprintID               string
}

func seedDefectFixture(t *testing.T, db *sql.DB, name string) defectFixture {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	f := defectFixture{
		productID:  uuid.NewString(),
		statusOpen: uuid.NewString(),
		statusDone: uuid.NewString(),
		envLinux:   uuid.NewString(),
		envMacOS:   uuid.NewString(),
		sprintID:   uuid.NewString(),
	}

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("seed (%s): %v", query, err)
		}
	}

	exec(`INSERT INTO products (id, name, description, workflow_mode, estimation_mode, created_at, updated_at)
	      VALUES (?, ?, '', 'kanban', 'none', ?, ?)`, f.productID, name, now, now)
	// "Open" sorts after "Done" alphabetically but before it by position, so a
	// status sort that reads the name instead of the position gets caught.
	exec(`INSERT INTO statuses (id, product_id, entity_type, name, position, created_at)
	      VALUES (?, ?, 'defect', 'Open', 1, ?)`, f.statusOpen, f.productID, now)
	exec(`INSERT INTO statuses (id, product_id, entity_type, name, position, created_at)
	      VALUES (?, ?, 'defect', 'Done', 2, ?)`, f.statusDone, f.productID, now)
	exec(`INSERT INTO runtime_environments (id, product_id, name, created_at) VALUES (?, ?, 'linux', ?)`, f.envLinux, f.productID, now)
	exec(`INSERT INTO runtime_environments (id, product_id, name, created_at) VALUES (?, ?, 'macos', ?)`, f.envMacOS, f.productID, now)
	exec(`INSERT INTO sprints (id, product_id, name, status, start_date, end_date, created_at, updated_at)
	      VALUES (?, ?, 'Sprint 1', 'active', ?, ?, ?, ?)`, f.sprintID, f.productID, now, now, now, now)

	return f
}

// addDefect inserts one defect and returns its id.
func (f defectFixture) addDefect(t *testing.T, db *sql.DB, title, criticality, statusID, envID, sprintID string, priority int, created string) string {
	t.Helper()

	id := uuid.NewString()
	_, err := db.Exec(`INSERT INTO defects
		(id, product_id, title, description, criticality, status_id, environment_id, sprint_id, priority, is_active, created_at, updated_at)
		VALUES (?, ?, ?, '', ?, ?, ?, ?, ?, 0, ?, ?)`,
		id, f.productID, title, criticality,
		nullableText(statusID), nullableText(envID), nullableText(sprintID),
		priority, created, created)
	if err != nil {
		t.Fatalf("insert defect %q: %v", title, err)
	}
	return id
}

// seedSortableDefects inserts one defect per criticality, in an order that is
// neither the alphabetical nor the severity order, so a passing sort test
// cannot be an accident of insertion order.
func seedSortableDefects(t *testing.T, db *sql.DB, f defectFixture) {
	t.Helper()

	f.addDefect(t, db, "Medium issue", "medium", f.statusDone, f.envMacOS, "", 2, "2026-01-03T00:00:00Z")
	f.addDefect(t, db, "Critical issue", "critical", f.statusOpen, f.envLinux, f.sprintID, 1, "2026-01-01T00:00:00Z")
	f.addDefect(t, db, "Low issue", "low", f.statusDone, "", "", 4, "2026-01-04T00:00:00Z")
	f.addDefect(t, db, "High issue", "high", f.statusOpen, f.envMacOS, f.sprintID, 3, "2026-01-02T00:00:00Z")
}

func TestDefectStoreListFilters(t *testing.T) {
	db := newTestDB(t)
	f := seedDefectFixture(t, db, "Aviary")
	seedSortableDefects(t, db, f)
	s := &defectStore{db: db}
	ctx := context.Background()

	tests := []struct {
		name   string
		filter DefectFilter
		want   int
	}{
		{"no filter returns all", DefectFilter{}, 4},
		{"criticality narrows", DefectFilter{Criticality: "critical"}, 1},
		{"status narrows", DefectFilter{StatusID: f.statusOpen}, 2},
		{"environment narrows", DefectFilter{EnvironmentID: f.envMacOS}, 2},
		{"sprint narrows", DefectFilter{SprintID: f.sprintID}, 2},
		{"filters combine", DefectFilter{StatusID: f.statusOpen, EnvironmentID: f.envMacOS}, 1},
		{"unknown id matches nothing", DefectFilter{StatusID: uuid.NewString()}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ListByProductID(ctx, f.productID, tt.filter)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("got %d defects, want %d", len(got), tt.want)
			}

			// The count backs the pagination, so it has to agree with the list
			// it is counting or the last page will be wrong.
			total, err := s.CountByProductID(ctx, f.productID, tt.filter, "")
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if total != tt.want {
				t.Errorf("count returned %d, want %d", total, tt.want)
			}
		})
	}
}

func TestDefectStoreListScopesToProduct(t *testing.T) {
	db := newTestDB(t)
	f := seedDefectFixture(t, db, "Aviary")
	seedSortableDefects(t, db, f)

	// A second product whose defects must never leak into the first's list.
	other := seedDefectFixture(t, db, "Finch Tracker")
	other.addDefect(t, db, "Critical issue", "critical", "", "", "", 1, "2026-01-01T00:00:00Z")

	s := &defectStore{db: db}
	got, err := s.ListByProductID(context.Background(), f.productID, DefectFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d defects, want 4 from the first product only", len(got))
	}
}

func TestDefectStoreSortOrder(t *testing.T) {
	db := newTestDB(t)
	f := seedDefectFixture(t, db, "Aviary")
	seedSortableDefects(t, db, f)
	s := &defectStore{db: db}
	ctx := context.Background()

	tests := []struct {
		name   string
		params ListParams
		want   []string
	}{
		{
			// Alphabetically this would be critical, high, low, medium.
			name:   "criticality sorts by severity, not alphabetically",
			params: ListParams{Sort: "criticality", Dir: SortAsc},
			want:   []string{"Critical issue", "High issue", "Medium issue", "Low issue"},
		},
		{
			name:   "criticality descending reverses severity",
			params: ListParams{Sort: "criticality", Dir: SortDesc},
			want:   []string{"Low issue", "Medium issue", "High issue", "Critical issue"},
		},
		{
			name:   "title sorts alphabetically",
			params: ListParams{Sort: "title", Dir: SortAsc},
			want:   []string{"Critical issue", "High issue", "Low issue", "Medium issue"},
		},
		{
			name:   "priority sorts numerically",
			params: ListParams{Sort: "priority", Dir: SortAsc},
			want:   []string{"Critical issue", "Medium issue", "High issue", "Low issue"},
		},
		{
			name:   "created sorts chronologically",
			params: ListParams{Sort: "created", Dir: SortAsc},
			want:   []string{"Critical issue", "High issue", "Medium issue", "Low issue"},
		},
		{
			name:   "unknown sort key falls back to the default order",
			params: ListParams{Sort: "; DROP TABLE defects", Dir: SortAsc},
			want:   []string{"Low issue", "High issue", "Medium issue", "Critical issue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ListPageByProductID(ctx, f.productID, DefectFilter{}, tt.params)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			titles := make([]string, len(got))
			for i, d := range got {
				titles[i] = d.Title
			}
			if len(titles) != len(tt.want) {
				t.Fatalf("got %d defects %v, want %d", len(titles), titles, len(tt.want))
			}
			for i := range titles {
				if titles[i] != tt.want[i] {
					t.Fatalf("order = %v, want %v", titles, tt.want)
				}
			}
		})
	}
}

// TestDefectStoreSortByStatus checks that the status sort follows the column
// order a product owner configured rather than the status name. Two defects
// share each status, so this asserts the status sequence rather than the
// titles: which of two equally-ranked defects comes first is the tiebreaker's
// business, not the sort key's.
func TestDefectStoreSortByStatus(t *testing.T) {
	db := newTestDB(t)
	f := seedDefectFixture(t, db, "Aviary")
	seedSortableDefects(t, db, f)
	s := &defectStore{db: db}

	got, err := s.ListPageByProductID(context.Background(), f.productID, DefectFilter{},
		ListParams{Sort: "status", Dir: SortAsc})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d defects, want 4", len(got))
	}

	// "Open" is position 1 and "Done" position 2, so Open comes first even
	// though "Done" precedes "Open" alphabetically.
	want := []string{f.statusOpen, f.statusOpen, f.statusDone, f.statusDone}
	for i, status := range want {
		if got[i].StatusID != status {
			t.Errorf("position %d has status %q, want %q — status must sort by position, not name",
				i, got[i].StatusID, status)
		}
	}
}

// TestDefectStoreSortByEnvironment checks the sort key that comes from an outer
// join. It asserts the environment sequence rather than the titles, because two
// defects share an environment and their order relative to each other is
// settled by the tiebreaker, not by the sort key.
func TestDefectStoreSortByEnvironment(t *testing.T) {
	db := newTestDB(t)
	f := seedDefectFixture(t, db, "Aviary")
	seedSortableDefects(t, db, f)
	s := &defectStore{db: db}

	got, err := s.ListPageByProductID(context.Background(), f.productID, DefectFilter{},
		ListParams{Sort: "environment", Dir: SortAsc})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d defects, want 4 — a defect without an environment must not be dropped by the join", len(got))
	}

	// COALESCE puts the unset environment first, then linux, then the two macos rows.
	want := []string{"", f.envLinux, f.envMacOS, f.envMacOS}
	for i, env := range want {
		if got[i].EnvironmentID != env {
			t.Errorf("position %d has environment %q, want %q", i, got[i].EnvironmentID, env)
		}
	}
}

func TestDefectStoreSearch(t *testing.T) {
	db := newTestDB(t)
	f := seedDefectFixture(t, db, "Aviary")
	seedSortableDefects(t, db, f)
	f.addDefect(t, db, "Export drops 100% of rows", "low", f.statusOpen, "", "", 5, "2026-01-05T00:00:00Z")
	s := &defectStore{db: db}
	ctx := context.Background()

	tests := []struct {
		name   string
		search string
		want   int
	}{
		{"empty search matches everything", "", 5},
		{"matches a title substring", "issue", 4},
		{"is case-insensitive", "CRITICAL ISSUE", 1},
		{"matches the criticality value", "medium", 1},
		{"matches the joined status name", "done", 2},
		{"matches the joined environment name", "macos", 2},
		{"matches nothing when absent", "nonexistent", 0},
		// Without escaping, a literal % would be read as a wildcard and match
		// every row instead of the one that contains it.
		{"treats % as a literal", "100%", 1},
		{"treats _ as a literal", "_", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ListPageByProductID(ctx, f.productID, DefectFilter{}, ListParams{Search: tt.search})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("search %q returned %d defects, want %d", tt.search, len(got), tt.want)
			}

			total, err := s.CountByProductID(ctx, f.productID, DefectFilter{}, tt.search)
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if total != tt.want {
				t.Errorf("count for search %q returned %d, want %d", tt.search, total, tt.want)
			}
		})
	}
}

func TestDefectStorePagingCoversEveryRowExactlyOnce(t *testing.T) {
	db := newTestDB(t)
	f := seedDefectFixture(t, db, "Aviary")
	ctx := context.Background()

	// Every defect shares one priority and one timestamp, so the sort key
	// cannot distinguish them. Without a unique tiebreaker in the ORDER BY the
	// engine is free to return them in a different order per query, and paging
	// would then repeat some rows while skipping others.
	const total = 25
	for range total {
		f.addDefect(t, db, "Tied defect", "medium", f.statusOpen, "", "", 1, "2026-01-01T00:00:00Z")
	}

	s := &defectStore{db: db}
	const pageSize = 10

	seen := map[string]bool{}
	for offset := 0; offset < total; offset += pageSize {
		page, err := s.ListPageByProductID(ctx, f.productID, DefectFilter{}, ListParams{
			Sort: "priority", Dir: SortAsc, Limit: pageSize, Offset: offset,
		})
		if err != nil {
			t.Fatalf("page at offset %d: %v", offset, err)
		}
		for _, d := range page {
			if seen[d.ID] {
				t.Errorf("defect %s appeared on more than one page", d.ID)
			}
			seen[d.ID] = true
		}
	}

	if len(seen) != total {
		t.Errorf("paging returned %d distinct defects, want %d", len(seen), total)
	}
}

func TestDefectStoreLimitAndOffset(t *testing.T) {
	db := newTestDB(t)
	f := seedDefectFixture(t, db, "Aviary")
	seedSortableDefects(t, db, f)
	s := &defectStore{db: db}
	ctx := context.Background()

	all, err := s.ListPageByProductID(ctx, f.productID, DefectFilter{}, ListParams{Sort: "title", Dir: SortAsc})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 defects, got %d", len(all))
	}

	// A zero limit must mean "everything", so callers wanting the full list
	// need no special case.
	page, err := s.ListPageByProductID(ctx, f.productID, DefectFilter{}, ListParams{
		Sort: "title", Dir: SortAsc, Limit: 2, Offset: 2,
	})
	if err != nil {
		t.Fatalf("list page: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("expected a page of 2, got %d", len(page))
	}
	if page[0].Title != all[2].Title || page[1].Title != all[3].Title {
		t.Errorf("offset page = %v, want %v",
			[]string{page[0].Title, page[1].Title},
			[]string{all[2].Title, all[3].Title})
	}
}
