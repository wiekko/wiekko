package devseed

import (
	"context"
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/auth/builtin"
	"nexmap.net/internal/config"
	"nexmap.net/internal/crypto"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

// TestSeed is the guard against the fixtures rotting. The seeder drives the
// real service layer, so a service that gains a validation rule or changes a
// precondition breaks here rather than silently at a developer's next startup.
func TestSeed(t *testing.T) {
	// Services discard audit write errors, so a dialect bug there would only
	// show as missing rows. Seeding is deterministic: every engine must end up
	// with the same number of audit entries.
	auditRows := map[string]int{}
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		testSeed(t, driver, db)
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM audit_trail`).Scan(&n); err != nil {
			t.Fatalf("count audit rows: %v", err)
		}
		if n == 0 {
			t.Error("seeding wrote no audit entries")
		}
		auditRows[driver] = n
	})
	if a, b := auditRows[dbtest.SQLite], auditRows[dbtest.Postgres]; a != 0 && b != 0 && a != b {
		t.Errorf("audit entries after seeding: sqlite %d, postgres %d", a, b)
	}
}

func testSeed(t *testing.T, driver string, db *sql.DB) {
	ctx := context.Background()
	st, svc, provider := newSeedEnv(t, driver, db)
	admin := newAdmin(t, st, provider)

	if err := Seed(ctx, svc, st, provider, admin); err != nil {
		t.Fatalf("seed: %v", err)
	}

	products, err := svc.Product.List(ctx, admin)
	if err != nil {
		t.Fatalf("list products: %v", err)
	}
	if len(products) != 2 {
		t.Fatalf("expected 2 products, got %d", len(products))
	}

	byName := map[string]*model.Product{}
	for _, p := range products {
		byName[p.Name] = p
	}
	aviary, ok := byName["Aviary"]
	if !ok {
		t.Fatal("expected an Aviary product")
	}
	finch, ok := byName["Finch Tracker"]
	if !ok {
		t.Fatal("expected a Finch Tracker product")
	}
	if aviary.WorkflowMode != model.WorkflowModeScrum {
		t.Errorf("Aviary workflow mode = %q, want scrum", aviary.WorkflowMode)
	}
	if finch.WorkflowMode != model.WorkflowModeKanban {
		t.Errorf("Finch Tracker workflow mode = %q, want kanban", finch.WorkflowMode)
	}
	if finch.EstimationMode != model.EstimationModeDisabled {
		t.Errorf("Finch Tracker estimation mode = %q, want disabled", finch.EstimationMode)
	}

	t.Run("demo users hold their roles", func(t *testing.T) {
		users, err := svc.User.List(ctx, store.UserFilter{})
		if err != nil {
			t.Fatalf("list users: %v", err)
		}
		ids := map[string]string{}
		for _, u := range users {
			ids[u.Username] = u.ID
		}
		for _, username := range []string{"ada", "bo", "cy"} {
			if ids[username] == "" {
				t.Fatalf("expected demo user %q", username)
			}
		}

		members, err := svc.Product.ListMembers(ctx, admin, aviary.ID)
		if err != nil {
			t.Fatalf("list members: %v", err)
		}
		roles := map[string]string{}
		for _, m := range members {
			roles[m.UserID] = m.Role
		}
		want := map[string]string{
			ids["ada"]: model.RoleViewer,
			ids["bo"]:  model.RoleTester,
			ids["cy"]:  model.RoleDeveloper,
		}
		for userID, role := range want {
			if roles[userID] != role {
				t.Errorf("Aviary role for %s = %q, want %q", userID, roles[userID], role)
			}
		}
	})

	t.Run("demo password authenticates", func(t *testing.T) {
		if _, err := provider.Authenticate(ctx, "bo", DemoPassword); err != nil {
			t.Errorf("authenticate bo with the demo password: %v", err)
		}
	})

	t.Run("backlog is populated", func(t *testing.T) {
		reqs, err := svc.Backlog.ListRequirements(ctx, admin, aviary.ID, store.RequirementFilter{})
		if err != nil {
			t.Fatalf("list requirements: %v", err)
		}
		if len(reqs) < 15 {
			t.Errorf("expected at least 15 Aviary backlog items, got %d", len(reqs))
		}

		// The backlog tree only renders as a tree if something has a parent.
		children := 0
		tasks := 0
		for _, r := range reqs {
			if r.ParentID != "" {
				children++
			}
			if r.Kind == model.RequirementKindTask {
				tasks++
			}
		}
		if children == 0 {
			t.Error("expected some Aviary requirements to have a parent")
		}
		if tasks == 0 {
			t.Error("expected some Aviary backlog items to be tasks")
		}

		defects, err := svc.Backlog.ListDefects(ctx, admin, aviary.ID, store.DefectFilter{})
		if err != nil {
			t.Fatalf("list defects: %v", err)
		}
		// Aviary must stay over one page so a fresh dev database exercises
		// pagination without anyone having to click 50 defects into existence.
		if len(defects) <= 50 {
			t.Errorf("expected more than 50 Aviary defects so the list pages, got %d", len(defects))
		}

		// Creation dates have to differ, or sorting by Created has no defined
		// order and paging through it can repeat rows.
		seen := map[string]bool{}
		for _, d := range defects {
			seen[d.CreatedAt] = true
		}
		if len(seen) < 10 {
			t.Errorf("expected Aviary defects spread over many creation dates, got %d distinct", len(seen))
		}
	})

	t.Run("sprint 13 is the active sprint", func(t *testing.T) {
		active, err := svc.Milestone.GetActiveSprint(ctx, admin, aviary.ID)
		if err != nil {
			t.Fatalf("get active sprint: %v", err)
		}
		if active == nil {
			t.Fatal("expected an active sprint")
		}
		if active.Name != "Sprint 13" {
			t.Errorf("active sprint = %q, want Sprint 13", active.Name)
		}

		sprints, err := svc.Milestone.ListSprints(ctx, admin, aviary.ID)
		if err != nil {
			t.Fatalf("list sprints: %v", err)
		}
		statuses := map[string]string{}
		for _, sp := range sprints {
			statuses[sp.Name] = sp.Status
		}
		if statuses["Sprint 12"] != model.SprintStatusCompleted {
			t.Errorf("Sprint 12 status = %q, want completed", statuses["Sprint 12"])
		}
		if statuses["Sprint 14"] != model.SprintStatusPlanned {
			t.Errorf("Sprint 14 status = %q, want planned", statuses["Sprint 14"])
		}
	})

	t.Run("campaigns reach their intended states", func(t *testing.T) {
		campaigns, err := svc.Testing.ListCampaigns(ctx, admin, aviary.ID)
		if err != nil {
			t.Fatalf("list campaigns: %v", err)
		}
		if len(campaigns) != 2 {
			t.Fatalf("expected 2 Aviary campaigns, got %d", len(campaigns))
		}

		byName := map[string]*model.TestingCampaign{}
		for _, c := range campaigns {
			byName[c.Name] = c
		}
		closed, ok := byName["Release 0.1 Regression"]
		if !ok {
			t.Fatal("expected the Release 0.1 Regression campaign")
		}
		if closed.Status != model.CampaignStatusClosed {
			t.Errorf("Release 0.1 Regression status = %q, want closed", closed.Status)
		}
		current, ok := byName["Sprint 13 Verification"]
		if !ok {
			t.Fatal("expected the Sprint 13 Verification campaign")
		}
		if current.Status != model.CampaignStatusActive {
			t.Errorf("Sprint 13 Verification status = %q, want active", current.Status)
		}

		// The active campaign is meant to look half-finished: some decided
		// rows, some failures with a defect behind them, some still pending.
		results, err := svc.Testing.ListCampaignResults(ctx, admin, aviary.ID, current.ID)
		if err != nil {
			t.Fatalf("list campaign results: %v", err)
		}
		counts := map[string]int{}
		for _, r := range results {
			counts[r.Status]++
			if r.Status == model.CampaignResultStatusFailed && r.DefectID == "" {
				t.Errorf("failed result %s has no defect linked", r.ID)
			}
		}
		for _, status := range []string{
			model.CampaignResultStatusPassed,
			model.CampaignResultStatusFailed,
			model.CampaignResultStatusPending,
		} {
			if counts[status] == 0 {
				t.Errorf("expected at least one %q result in the active campaign", status)
			}
		}
	})

	t.Run("closing a defect pushed its result to re-test", func(t *testing.T) {
		// FR-123: closing a defect moves the results it failed to re-test. The
		// fixture closes the photo defect while its campaign is still running,
		// so a result in the active campaign must carry that status.
		campaigns, err := svc.Testing.ListCampaigns(ctx, admin, aviary.ID)
		if err != nil {
			t.Fatalf("list campaigns: %v", err)
		}
		for _, c := range campaigns {
			results, err := svc.Testing.ListCampaignResults(ctx, admin, aviary.ID, c.ID)
			if err != nil {
				t.Fatalf("list results: %v", err)
			}

			retests := 0
			for _, r := range results {
				if r.Status == model.CampaignResultStatusRetest {
					retests++
				}
			}

			switch c.Name {
			case "Sprint 13 Verification":
				if retests == 0 {
					t.Error("expected a re-test result in the active campaign")
				}
			case "Release 0.1 Regression":
				// A closed campaign keeps the verdict it was closed with, so
				// closing the field-trial defect afterwards must not move any
				// of its results.
				if retests != 0 {
					t.Errorf("closed campaign has %d re-test results, want 0", retests)
				}
				counts := map[string]int{}
				for _, r := range results {
					counts[r.Status]++
				}
				if got := model.CampaignVerdict(counts); got != model.CampaignVerdictFailed {
					t.Errorf("closed campaign verdict = %q, want %q", got, model.CampaignVerdictFailed)
				}
			}
		}
	})

	t.Run("test cases and environments exist for both products", func(t *testing.T) {
		for _, p := range []*model.Product{aviary, finch} {
			cases, err := svc.Testing.ListTestCases(ctx, admin, p.ID)
			if err != nil {
				t.Fatalf("list test cases of %s: %v", p.Name, err)
			}
			if len(cases) == 0 {
				t.Errorf("expected test cases for %s", p.Name)
			}
			envs, err := svc.Milestone.ListRuntimeEnvironments(ctx, admin, p.ID)
			if err != nil {
				t.Fatalf("list environments of %s: %v", p.Name, err)
			}
			if len(envs) == 0 {
				t.Errorf("expected runtime environments for %s", p.Name)
			}
		}
	})
}

// --- helpers ---

// newSeedEnv builds the same object graph main.go wires up, against a migrated
// throwaway database.
func newSeedEnv(t *testing.T, driver string, db *sql.DB) (*store.Store, *service.Services, *builtin.Provider) {
	t.Helper()

	keys, err := crypto.NewKeyRing(testKeyJSON())
	if err != nil {
		t.Fatalf("key ring: %v", err)
	}

	blobs, err := storage.NewFS(filepath.Join(t.TempDir(), "uploads"), 1<<20)
	if err != nil {
		t.Fatalf("blob storage: %v", err)
	}

	cfg := &config.Config{
		SessionTTL: time.Hour,
		// Deliberately far below the production iteration count: the seeder
		// hashes four passwords and this test does not benchmark PBKDF2.
		PBKDF2Iterations: 1,
	}

	st := store.New(db, store.Dialect(driver), keys)
	provider := builtin.New(st.User, cfg.PBKDF2Iterations)
	return st, service.New(st, provider, cfg, blobs), provider
}

// newAdmin creates the bootstrap admin the seeder acts as, mirroring what
// bootstrapAdmin does in main.go.
func newAdmin(t *testing.T, st *store.Store, provider *builtin.Provider) *model.User {
	t.Helper()

	hash, err := provider.HashPassword("adminpassword")
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	admin := &model.User{
		ID:           uuid.New().String(),
		Username:     "admin",
		Email:        "admin@localhost",
		DisplayName:  "Administrator",
		PasswordHash: hash,
		IsAdmin:      true,
		Status:       model.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := st.User.Create(context.Background(), admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return admin
}

func testKeyJSON() string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	return `[{"id":"test","key":"` + base64.StdEncoding.EncodeToString(raw) + `"}]`
}
