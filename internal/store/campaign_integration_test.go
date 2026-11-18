package store

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
)

// campaignFixture seeds a product, two environments and two test cases, and
// returns their IDs.
type campaignFixture struct {
	productID  string
	envA, envB string
	tcMatrix   string // linked to both environments
	tcBare     string // linked to none
}

func seedCampaignFixture(t *testing.T, db *DB) campaignFixture {
	t.Helper()

	now := Now()
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

// seedTestCase adds a bare test case to the product. A campaign holds at most
// one result per test case and environment, so tests that need several
// results without an environment give each its own test case.
func seedTestCase(t *testing.T, db *DB, productID, title string) string {
	t.Helper()
	id := uuid.New().String()
	now := Now()
	if _, err := db.Exec(`INSERT INTO test_cases (id, product_id, title, description, execution_mode, created_at, updated_at)
	      VALUES (?, ?, ?, '', 'manual', ?, ?)`, id, productID, title, now, now); err != nil {
		t.Fatalf("seed test case: %v", err)
	}
	return id
}

func makeCampaign(t *testing.T, s TestingCampaignStore, productID, name, status string) *model.TestingCampaign {
	t.Helper()

	now := Now()
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
	forEachDB(t, func(t *testing.T, db *DB) {
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
	})
}

func TestTestingCampaignStoreNoActiveCampaign(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
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
	})
}

// The partial unique index is what actually enforces FR-95.
func TestTestingCampaignStoreOneActivePerProduct(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedCampaignFixture(t, db)
		store := &testingCampaignStore{db: db}

		makeCampaign(t, store, f.productID, "First", model.CampaignStatusActive)

		now := Now()
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
	})
}

func TestCampaignResultStoreSeedingAndLookups(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedCampaignFixture(t, db)
		campaigns := &testingCampaignStore{db: db}
		results := &campaignResultStore{db: db}
		ctx := context.Background()

		c := makeCampaign(t, campaigns, f.productID, "Release 1.0", model.CampaignStatusActive)
		now := Now()

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
	})
}

// Deleting an environment keeps what a campaign recorded on it: verdicts in
// active campaigns and everything in closed ones stay, with the environment's
// name; results still waiting to run are dropped. Deleting both environments
// of one test must not collide on the "no environment" unique index.
func TestCampaignResultsSurviveEnvironmentDelete(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedCampaignFixture(t, db)
		campaigns := &testingCampaignStore{db: db}
		results := &campaignResultStore{db: db}
		ctx := context.Background()

		active := makeCampaign(t, campaigns, f.productID, "Release 1.0", model.CampaignStatusActive)
		closed := makeCampaign(t, campaigns, f.productID, "Release 0.9", model.CampaignStatusClosed)
		now := Now()
		if err := results.CreateBatch(ctx, []*model.CampaignResult{
			{ID: uuid.New().String(), CampaignID: active.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envA, Status: model.CampaignResultStatusPassed, UpdatedAt: now},
			{ID: uuid.New().String(), CampaignID: active.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envB, Status: model.CampaignResultStatusPending, UpdatedAt: now},
			{ID: uuid.New().String(), CampaignID: active.ID, TestCaseID: f.tcBare, Status: model.CampaignResultStatusPending, UpdatedAt: now},
			{ID: uuid.New().String(), CampaignID: closed.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envA, Status: model.CampaignResultStatusFailed, UpdatedAt: now},
			{ID: uuid.New().String(), CampaignID: closed.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envB, Status: model.CampaignResultStatusPending, UpdatedAt: now},
		}); err != nil {
			t.Fatalf("CreateBatch: %v", err)
		}

		for _, env := range []string{f.envA, f.envB} {
			if err := results.DetachEnvironment(ctx, env); err != nil {
				t.Fatalf("DetachEnvironment: %v", err)
			}
			if _, err := db.Exec(`DELETE FROM runtime_environments WHERE id = ?`, env); err != nil {
				t.Fatalf("delete environment: %v", err)
			}
		}

		remaining, err := results.ListByCampaignID(ctx, active.ID)
		if err != nil {
			t.Fatalf("ListByCampaignID: %v", err)
		}
		// The passed linux row stays; the pending macos row is gone; the
		// environment-less row was never touched.
		names := map[string]string{}
		for _, r := range remaining {
			names[r.EnvironmentName] = r.Status
			if r.EnvironmentID != "" {
				t.Fatalf("result still points at a deleted environment: %+v", r)
			}
		}
		if len(remaining) != 2 || names["linux"] != model.CampaignResultStatusPassed || names[""] != model.CampaignResultStatusPending {
			t.Fatalf("active campaign after deleting environments = %v, want linux passed and the bare row", names)
		}

		kept, err := results.ListByCampaignID(ctx, closed.ID)
		if err != nil {
			t.Fatalf("ListByCampaignID: %v", err)
		}
		if len(kept) != 2 || kept[0].EnvironmentName != "linux" || kept[1].EnvironmentName != "macos" {
			t.Fatalf("closed campaign = %+v, want both rows with their environment names", kept)
		}
	})
}

// A campaign keeps its copy of a test case: edits reach draft campaigns only,
// and deleting the test case leaves closed campaigns and the verdicts of
// active ones in place, while draft campaigns and unexecuted results drop it.
func TestCampaignTestsOutliveTheTestCase(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedCampaignFixture(t, db)
		campaigns := &testingCampaignStore{db: db}
		results := &campaignResultStore{db: db}
		ctx := context.Background()
		now := Now()

		byStatus := map[string]*model.TestingCampaign{}
		for _, status := range []string{model.CampaignStatusDraft, model.CampaignStatusActive, model.CampaignStatusClosed} {
			byStatus[status] = makeCampaign(t, campaigns, f.productID, "C "+status, status)
		}
		var batch []*model.CampaignResult
		for status, c := range byStatus {
			verdict := model.CampaignResultStatusPassed
			if status == model.CampaignStatusDraft {
				verdict = model.CampaignResultStatusPending
			}
			batch = append(batch,
				&model.CampaignResult{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envA, Status: verdict, UpdatedAt: now},
				&model.CampaignResult{ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcMatrix, EnvironmentID: f.envB, Status: model.CampaignResultStatusPending, UpdatedAt: now},
			)
		}
		if err := results.CreateBatch(ctx, batch); err != nil {
			t.Fatalf("CreateBatch: %v", err)
		}

		if _, err := db.Exec(`UPDATE test_cases SET title = 'Renamed' WHERE id = ?`, f.tcMatrix); err != nil {
			t.Fatal(err)
		}
		if err := results.RefreshDraftTests(ctx, f.tcMatrix); err != nil {
			t.Fatalf("RefreshDraftTests: %v", err)
		}
		for status, c := range byStatus {
			tests, err := results.ListTests(ctx, c.ID)
			if err != nil || len(tests) != 1 {
				t.Fatalf("%s: tests = %v, %v", status, tests, err)
			}
			want := "Login rejects a bad password"
			if status == model.CampaignStatusDraft {
				want = "Renamed"
			}
			if tests[0].Title != want {
				t.Fatalf("%s campaign title = %q, want %q", status, tests[0].Title, want)
			}
		}

		if err := results.DetachTestCase(ctx, f.tcMatrix); err != nil {
			t.Fatalf("DetachTestCase: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM test_cases WHERE id = ?`, f.tcMatrix); err != nil {
			t.Fatalf("delete test case: %v", err)
		}
		want := map[string]int{model.CampaignStatusDraft: 0, model.CampaignStatusActive: 1, model.CampaignStatusClosed: 2}
		for status, c := range byStatus {
			rows, err := results.ListByCampaignID(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != want[status] {
				t.Fatalf("%s campaign keeps %d results, want %d", status, len(rows), want[status])
			}
			for _, r := range rows {
				if r.TestCaseID != "" || r.TestTitle != "Login rejects a bad password" {
					t.Fatalf("%s campaign result = %+v, want detached with the copied title", status, r)
				}
			}
		}
		if tests, _ := results.ListTests(ctx, byStatus[model.CampaignStatusDraft].ID); len(tests) != 0 {
			t.Fatalf("draft campaign still lists %d tests", len(tests))
		}
	})
}

func TestCampaignResultStoreDefectLinkage(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedCampaignFixture(t, db)
		campaigns := &testingCampaignStore{db: db}
		results := &campaignResultStore{db: db}
		events := &campaignResultEventStore{db: db}
		ctx := context.Background()

		now := Now()
		defectID := uuid.New().String()
		if _, err := db.Exec(`INSERT INTO defects (id, product_id, title, description, criticality, priority, is_active, created_at, updated_at)
	                      VALUES (?, ?, 'Login is broken', '', 'high', 0, FALSE, ?, ?)`, defectID, f.productID, now, now); err != nil {
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
	})
}

func TestCampaignResultStoreDeleteByCampaignAndTestCase(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedCampaignFixture(t, db)
		campaigns := &testingCampaignStore{db: db}
		results := &campaignResultStore{db: db}
		ctx := context.Background()

		c := makeCampaign(t, campaigns, f.productID, "Release 1.0", model.CampaignStatusDraft)
		now := Now()
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
	})
}

// The defect trail must survive the fact that a passing verdict clears the
// result's defect: the closing event carries no defect_id, and a plain column
// filter would drop exactly the row that proves the fix worked.
func TestCampaignResultEventStoreDefectTrail(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedCampaignFixture(t, db)
		campaigns := &testingCampaignStore{db: db}
		results := &campaignResultStore{db: db}
		events := &campaignResultEventStore{db: db}
		ctx := context.Background()

		now := Now()
		defectID := uuid.New().String()
		if _, err := db.Exec(`INSERT INTO defects (id, product_id, title, description, criticality, priority, is_active, created_at, updated_at)
	                      VALUES (?, ?, 'Login is broken', '', 'high', 0, FALSE, ?, ?)`, defectID, f.productID, now, now); err != nil {
			t.Fatalf("seed defect: %v", err)
		}

		c := makeCampaign(t, campaigns, f.productID, "Release 1.0", model.CampaignStatusActive)

		// The verified result, and a second one that never touched the defect.
		verified := uuid.New().String()
		unrelated := uuid.New().String()
		for _, id := range []string{verified, unrelated} {
			if err := results.Create(ctx, &model.CampaignResult{
				ID: id, CampaignID: c.ID, TestCaseID: seedTestCase(t, db, f.productID, "Case "+id),
				Status: model.CampaignResultStatusPending, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("Create result: %v", err)
			}
		}

		record := func(resultID, from, to, defect, at string) {
			t.Helper()
			if err := events.Create(ctx, &model.CampaignResultEvent{
				ID: uuid.New().String(), ResultID: resultID, CampaignID: c.ID,
				FromStatus: from, ToStatus: to, DefectID: defect, CreatedAt: at,
			}); err != nil {
				t.Fatalf("event Create: %v", err)
			}
		}

		// The full loop: failed with the defect, reopened for re-test, then passed.
		// The pass clears the defect, so its event names no defect at all.
		record(verified, model.CampaignResultStatusPending, model.CampaignResultStatusFailed, defectID, "2026-08-01T10:00:00Z")
		record(verified, model.CampaignResultStatusFailed, model.CampaignResultStatusRetest, defectID, "2026-08-02T10:00:00Z")
		record(verified, model.CampaignResultStatusRetest, model.CampaignResultStatusPassed, "", "2026-08-03T10:00:00Z")
		record(unrelated, model.CampaignResultStatusPending, model.CampaignResultStatusPassed, "", "2026-08-03T11:00:00Z")

		trail, err := events.ListByDefectTrail(ctx, defectID)
		if err != nil {
			t.Fatalf("ListByDefectTrail: %v", err)
		}
		if len(trail) != 3 {
			t.Fatalf("len(trail) = %d, want 3 (the unrelated result must not appear)", len(trail))
		}
		for i, want := range []string{
			model.CampaignResultStatusFailed,
			model.CampaignResultStatusRetest,
			model.CampaignResultStatusPassed,
		} {
			if trail[i].ToStatus != want {
				t.Fatalf("trail[%d].ToStatus = %q, want %q", i, trail[i].ToStatus, want)
			}
			if trail[i].ResultID != verified {
				t.Fatalf("trail[%d].ResultID = %q, want the verified result", i, trail[i].ResultID)
			}
		}
		// The row that matters carries no defect_id and is only reachable through
		// the result, which is the whole reason for the subquery.
		if trail[2].DefectID != "" {
			t.Fatalf("closing event DefectID = %q, want empty", trail[2].DefectID)
		}
	})
}

func TestCampaignResultEventStoreCountsByCampaignID(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedCampaignFixture(t, db)
		campaigns := &testingCampaignStore{db: db}
		results := &campaignResultStore{db: db}
		events := &campaignResultEventStore{db: db}
		ctx := context.Background()

		now := Now()
		c := makeCampaign(t, campaigns, f.productID, "Release 1.0", model.CampaignStatusActive)

		changed := uuid.New().String()
		setOnce := uuid.New().String()
		untouched := uuid.New().String()
		for _, id := range []string{changed, setOnce, untouched} {
			if err := results.Create(ctx, &model.CampaignResult{
				ID: id, CampaignID: c.ID, TestCaseID: seedTestCase(t, db, f.productID, "Case "+id),
				Status: model.CampaignResultStatusPending, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("Create result: %v", err)
			}
		}

		record := func(resultID, to string) {
			t.Helper()
			if err := events.Create(ctx, &model.CampaignResultEvent{
				ID: uuid.New().String(), ResultID: resultID, CampaignID: c.ID,
				ToStatus: to, CreatedAt: now,
			}); err != nil {
				t.Fatalf("event Create: %v", err)
			}
		}
		record(changed, model.CampaignResultStatusPassed)
		record(changed, model.CampaignResultStatusFailed)
		record(setOnce, model.CampaignResultStatusPassed)

		counts, err := events.CountsByCampaignID(ctx, c.ID)
		if err != nil {
			t.Fatalf("CountsByCampaignID: %v", err)
		}
		if counts[changed] != 2 {
			t.Fatalf("counts[changed] = %d, want 2", counts[changed])
		}
		if counts[setOnce] != 1 {
			t.Fatalf("counts[setOnce] = %d, want 1", counts[setOnce])
		}
		// A result nobody ever set has no events and simply does not appear.
		if _, ok := counts[untouched]; ok {
			t.Fatalf("counts[untouched] = %d, want no entry", counts[untouched])
		}
	})
}

// TestCampaignResultOnePerTestCaseWithoutEnvironment: the UNIQUE constraint
// treats NULL environments as distinct, so a partial index has to stop a
// second environment-less result for the same test case.
func TestCampaignResultOnePerTestCaseWithoutEnvironment(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		f := seedCampaignFixture(t, db)
		c := makeCampaign(t, &testingCampaignStore{db: db}, f.productID, "Release 1.0", model.CampaignStatusDraft)
		results := &campaignResultStore{db: db}
		create := func() error {
			return results.Create(context.Background(), &model.CampaignResult{
				ID: uuid.New().String(), CampaignID: c.ID, TestCaseID: f.tcBare,
				Status: model.CampaignResultStatusPending, UpdatedAt: Now(),
			})
		}
		if err := create(); err != nil {
			t.Fatalf("first result: %v", err)
		}
		if err := create(); err == nil {
			t.Fatal("second result without an environment was accepted")
		}
	})
}
