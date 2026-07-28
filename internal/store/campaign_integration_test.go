package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"nexmap.net/internal/database"
	"nexmap.net/internal/model"
)

// newTestDB opens a migrated SQLite database. These tests exercise the real SQL
// rather than mocks, so schema and query mistakes surface here.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "store.db") + "?_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := database.Migrate(db, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// campaignFixture seeds a product, two environments and two test cases, and
// returns their IDs.
type campaignFixture struct {
	productID  string
	envA, envB string
	tcMatrix   string // linked to both environments
	tcBare     string // linked to none
}

func seedCampaignFixture(t *testing.T, db *sql.DB) campaignFixture {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	f := campaignFixture{
		productID: uuid.New().String(),
		envA:      uuid.New().String(),
		envB:      uuid.New().String(),
		tcMatrix:  uuid.New().String(),
		tcBare:    uuid.New().String(),
	}

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("seed (%s): %v", query, err)
		}
	}

	exec(`INSERT INTO products (id, name, description, workflow_mode, estimation_mode, created_at, updated_at)
	      VALUES (?, 'Nexmap', '', 'kanban', 'none', ?, ?)`, f.productID, now, now)
	exec(`INSERT INTO runtime_environments (id, product_id, name, created_at) VALUES (?, ?, 'linux', ?)`, f.envA, f.productID, now)
	exec(`INSERT INTO runtime_environments (id, product_id, name, created_at) VALUES (?, ?, 'macos', ?)`, f.envB, f.productID, now)

	for _, tc := range []struct{ id, title string }{
		{f.tcMatrix, "Login rejects a bad password"},
		{f.tcBare, "Health endpoint responds"},
	} {
		exec(`INSERT INTO test_cases (id, product_id, title, description, execution_mode, created_at, updated_at)
		      VALUES (?, ?, ?, '', 'manual', ?, ?)`, tc.id, f.productID, tc.title, now, now)
	}
	exec(`INSERT INTO test_case_environments (id, test_case_id, environment_id) VALUES (?, ?, ?)`,
		uuid.New().String(), f.tcMatrix, f.envA)
	exec(`INSERT INTO test_case_environments (id, test_case_id, environment_id) VALUES (?, ?, ?)`,
		uuid.New().String(), f.tcMatrix, f.envB)

	return f
}

func makeCampaign(t *testing.T, s TestingCampaignStore, productID, name, status string) *model.TestingCampaign {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.TestingCampaign{
		ID:        uuid.New().String(),
		ProductID: productID,
		Name:      name,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.Create(context.Background(), c); err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	return c
}

func TestTestingCampaignStoreRoundTrip(t *testing.T) {
	db := newTestDB(t)
	f := seedCampaignFixture(t, db)
	store := &testingCampaignStore{db: db}
	ctx := context.Background()

	c := makeCampaign(t, store, f.productID, "Release 1.0", model.CampaignStatusDraft)

	got, err := store.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	// Optional columns are stored NULL and must read back as empty strings.
	if got.StartsAt != "" || got.EndsAt != "" || got.ClosedAt != "" || got.ClosedBy != "" {
		t.Fatalf("expected empty optional fields, got %+v", got)
	}
	if got.Status != model.CampaignStatusDraft {
		t.Fatalf("Status = %q, want draft", got.Status)
	}

	got.Status = model.CampaignStatusActive
	got.StartsAt = "2026-07-01T00:00:00Z"
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}

	active, err := store.GetActiveByProductID(ctx, f.productID)
	if err != nil {
		t.Fatalf("GetActiveByProductID: %v", err)
	}
	if active == nil || active.ID != c.ID {
		t.Fatalf("GetActiveByProductID = %+v, want campaign %s", active, c.ID)
	}
	if active.StartsAt != "2026-07-01T00:00:00Z" {
		t.Fatalf("StartsAt = %q, want the stored value", active.StartsAt)
	}
}

func TestTestingCampaignStoreNoActiveCampaign(t *testing.T) {
	db := newTestDB(t)
	f := seedCampaignFixture(t, db)
	store := &testingCampaignStore{db: db}

	makeCampaign(t, store, f.productID, "Draft only", model.CampaignStatusDraft)

	active, err := store.GetActiveByProductID(context.Background(), f.productID)
	if err != nil {
		t.Fatalf("GetActiveByProductID: %v", err)
	}
	if active != nil {
		t.Fatalf("GetActiveByProductID = %+v, want nil", active)
	}
}

// The partial unique index is what actually enforces FR-322.
func TestTestingCampaignStoreOneActivePerProduct(t *testing.T) {
	db := newTestDB(t)
	f := seedCampaignFixture(t, db)
	store := &testingCampaignStore{db: db}

	makeCampaign(t, store, f.productID, "First", model.CampaignStatusActive)

	now := time.Now().UTC().Format(time.RFC3339)
	second := &model.TestingCampaign{
		ID:        uuid.New().String(),
		ProductID: f.productID,
		Name:      "Second",
		Status:    model.CampaignStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.Create(context.Background(), second); err == nil {
		t.Fatal("expected the schema to reject a second active campaign")
	}

	// Draft and closed campaigns are unconstrained.
	makeCampaign(t, store, f.productID, "Draft", model.CampaignStatusDraft)
	makeCampaign(t, store, f.productID, "Closed", model.CampaignStatusClosed)
}

func TestCampaignResultStoreSeedingAndLookups(t *testing.T) {
	db := newTestDB(t)
	f := seedCampaignFixture(t, db)
	campaigns := &testingCampaignStore{db: db}
	results := &campaignResultStore{db: db}
	ctx := context.Background()

	c := makeCampaign(t, campaigns, f.productID, "Release 1.0", model.CampaignStatusActive)
	now := time.Now().UTC().Format(time.RFC3339)

	seed := []*model.CampaignResult{
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envA, Status: model.CampaignResultStatusPending, UpdatedAt: now},
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envB, Status: model.CampaignResultStatusPending, UpdatedAt: now},
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcBare, Status: model.CampaignResultStatusPending, UpdatedAt: now},
	}
	if err := results.CreateBatch(ctx, seed); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	all, err := results.ListByCampaignID(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListByCampaignID: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("listed %d results, want 3", len(all))
	}

	// The environment-less row round-trips as an empty string, not "NULL".
	bare, err := results.GetByCampaignAndTestCase(ctx, c.ID, f.tcBare, "")
	if err != nil {
		t.Fatalf("GetByCampaignAndTestCase: %v", err)
	}
	if bare.EnvironmentID != "" {
		t.Fatalf("EnvironmentID = %q, want empty", bare.EnvironmentID)
	}

	byTestCase, err := results.ListByTestCaseID(ctx, f.tcMatrix)
	if err != nil {
		t.Fatalf("ListByTestCaseID: %v", err)
	}
	if len(byTestCase) != 2 {
		t.Fatalf("listed %d results for the matrix test case, want 2", len(byTestCase))
	}

	counts, err := results.CountsByProductID(ctx, f.productID)
	if err != nil {
		t.Fatalf("CountsByProductID: %v", err)
	}
	if counts[c.ID][model.CampaignResultStatusPending] != 3 {
		t.Fatalf("counts = %v, want 3 pending", counts[c.ID])
	}

	// CreateBatch is transactional: a duplicate must roll the whole batch back.
	before := len(all)
	dup := []*model.CampaignResult{
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envA, Status: model.CampaignResultStatusPending, UpdatedAt: now},
	}
	if err := results.CreateBatch(ctx, dup); err == nil {
		t.Fatal("expected a duplicate campaign/test case/environment to be rejected")
	}
	after, _ := results.ListByCampaignID(ctx, c.ID)
	if len(after) != before {
		t.Fatalf("listed %d results after a failed batch, want %d", len(after), before)
	}
}

// Deleting a runtime environment must remove its matrix rows rather than
// collapsing them into indistinguishable NULL rows.
func TestCampaignResultsCascadeOnEnvironmentDelete(t *testing.T) {
	db := newTestDB(t)
	f := seedCampaignFixture(t, db)
	campaigns := &testingCampaignStore{db: db}
	results := &campaignResultStore{db: db}
	ctx := context.Background()

	c := makeCampaign(t, campaigns, f.productID, "Release 1.0", model.CampaignStatusActive)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := results.CreateBatch(ctx, []*model.CampaignResult{
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envA, Status: model.CampaignResultStatusPassed, UpdatedAt: now},
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envB, Status: model.CampaignResultStatusPending, UpdatedAt: now},
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcBare, Status: model.CampaignResultStatusPending, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM runtime_environments WHERE id = ?`, f.envA); err != nil {
		t.Fatalf("delete environment: %v", err)
	}

	remaining, err := results.ListByCampaignID(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListByCampaignID: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("listed %d results after deleting an environment, want 2", len(remaining))
	}

	// Exactly one environment-less row must remain: the one seeded that way.
	bare := 0
	for _, result := range remaining {
		if result.EnvironmentID == "" {
			bare++
		}
	}
	if bare != 1 {
		t.Fatalf("found %d environment-less results, want 1", bare)
	}
}

func TestCampaignResultStoreDefectLinkage(t *testing.T) {
	db := newTestDB(t)
	f := seedCampaignFixture(t, db)
	campaigns := &testingCampaignStore{db: db}
	results := &campaignResultStore{db: db}
	events := &campaignResultEventStore{db: db}
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339)
	defectID := uuid.New().String()
	if _, err := db.Exec(`INSERT INTO defects (id, product_id, title, description, criticality, priority, is_active, created_at, updated_at)
	                      VALUES (?, ?, 'Login is broken', '', 'high', 0, 0, ?, ?)`, defectID, f.productID, now, now); err != nil {
		t.Fatalf("seed defect: %v", err)
	}

	c := makeCampaign(t, campaigns, f.productID, "Release 1.0", model.CampaignStatusActive)
	resultID := uuid.New().String()
	if err := results.Create(ctx, &model.CampaignResult{
		ID: resultID, CampaignID: c.ID, TestCaseID: f.tcBare,
		Status: model.CampaignResultStatusPending, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	result, err := results.GetByID(ctx, resultID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	result.Status = model.CampaignResultStatusFailed
	result.DefectID = defectID
	result.ExecutedAt = now
	if err := results.Update(ctx, result); err != nil {
		t.Fatalf("Update: %v", err)
	}

	byDefect, err := results.ListByDefectID(ctx, defectID)
	if err != nil {
		t.Fatalf("ListByDefectID: %v", err)
	}
	if len(byDefect) != 1 || byDefect[0].ID != resultID {
		t.Fatalf("ListByDefectID = %+v, want the failed result", byDefect)
	}

	if err := events.Create(ctx, &model.CampaignResultEvent{
		ID: uuid.New().String(), ResultID: resultID, CampaignID: c.ID,
		FromStatus: model.CampaignResultStatusPending,
		ToStatus:   model.CampaignResultStatusFailed,
		DefectID:   defectID, CreatedAt: now,
	}); err != nil {
		t.Fatalf("event Create: %v", err)
	}

	logged, err := events.ListByCampaignID(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListByCampaignID: %v", err)
	}
	if len(logged) != 1 || logged[0].ToStatus != model.CampaignResultStatusFailed {
		t.Fatalf("events = %+v, want one pending->failed entry", logged)
	}
	// user_id was never set and must survive the NULL round trip.
	if logged[0].UserID != "" {
		t.Fatalf("UserID = %q, want empty", logged[0].UserID)
	}

	// Deleting the defect must not delete the history that references it.
	if _, err := db.Exec(`DELETE FROM defects WHERE id = ?`, defectID); err != nil {
		t.Fatalf("delete defect: %v", err)
	}
	orphaned, err := results.GetByID(ctx, resultID)
	if err != nil {
		t.Fatalf("GetByID after defect delete: %v", err)
	}
	if orphaned.DefectID != "" {
		t.Fatalf("DefectID = %q, want empty after the defect was deleted", orphaned.DefectID)
	}
	if orphaned.Status != model.CampaignResultStatusFailed {
		t.Fatalf("Status = %q, want the result to keep its verdict", orphaned.Status)
	}
}

func TestCampaignResultStoreDeleteByCampaignAndTestCase(t *testing.T) {
	db := newTestDB(t)
	f := seedCampaignFixture(t, db)
	campaigns := &testingCampaignStore{db: db}
	results := &campaignResultStore{db: db}
	ctx := context.Background()

	c := makeCampaign(t, campaigns, f.productID, "Release 1.0", model.CampaignStatusDraft)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := results.CreateBatch(ctx, []*model.CampaignResult{
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envA, Status: model.CampaignResultStatusPending, UpdatedAt: now},
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envB, Status: model.CampaignResultStatusPending, UpdatedAt: now},
		{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcBare, Status: model.CampaignResultStatusPending, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	// Dropping a test case from a draft scope removes all of its environments.
	if err := results.DeleteByCampaignAndTestCase(ctx, c.ID, f.tcMatrix); err != nil {
		t.Fatalf("DeleteByCampaignAndTestCase: %v", err)
	}
	remaining, err := results.ListByCampaignID(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListByCampaignID: %v", err)
	}
	if len(remaining) != 1 || remaining[0].TestCaseID != f.tcBare {
		t.Fatalf("remaining = %+v, want only the bare test case", remaining)
	}
}
