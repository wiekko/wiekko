package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"nexmap.net/internal/model"
)

// seedSprintProduct inserts a bare product and returns its id. Product names
// are unique, so the caller supplies one.
func seedSprintProduct(t *testing.T, db *DB, name string) string {
	t.Helper()

	now := Now()
	id := uuid.New().String()
	if _, err := db.Exec(`INSERT INTO products (id, name, description, workflow_mode, estimation_mode, created_at, updated_at)
	      VALUES (?, ?, '', 'scrum', 'none', ?, ?)`, id, name, now, now); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	return id
}

func makeStoredSprint(productID, name string) *model.Sprint {
	now := Now()
	return &model.Sprint{
		ID:        uuid.New().String(),
		ProductID: productID,
		Name:      name,
		Status:    model.SprintStatusPlanned,
		StartDate: now,
		EndDate:   now,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestSprintStoreRoundTrip(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		productID := seedSprintProduct(t, db, "Nexmap")
		st := &sprintStore{db: db}
		ctx := context.Background()

		sprint := makeStoredSprint(productID, "Sprint 1")
		if err := st.Create(ctx, sprint); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := st.GetByID(ctx, sprint.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Status != model.SprintStatusPlanned {
			t.Fatalf("Status = %q, want %q", got.Status, model.SprintStatusPlanned)
		}
		if got.MilestoneID != "" {
			t.Fatalf("MilestoneID = %q, want empty", got.MilestoneID)
		}

		got.Status = model.SprintStatusActive
		if err := st.Update(ctx, got); err != nil {
			t.Fatalf("Update: %v", err)
		}

		reloaded, err := st.GetByID(ctx, sprint.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if reloaded.Status != model.SprintStatusActive {
			t.Fatalf("Status = %q, want %q", reloaded.Status, model.SprintStatusActive)
		}
	})
}

func TestSprintStoreGetActiveByProductID(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *DB) {
		productID := seedSprintProduct(t, db, "Nexmap")
		st := &sprintStore{db: db}
		ctx := context.Background()

		planned := makeStoredSprint(productID, "Sprint 1")
		if err := st.Create(ctx, planned); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// No active sprint is an ordinary state, not an error.
		active, err := st.GetActiveByProductID(ctx, productID)
		if err != nil {
			t.Fatalf("GetActiveByProductID: %v", err)
		}
		if active != nil {
			t.Fatalf("active = %+v, want nil", active)
		}

		started := makeStoredSprint(productID, "Sprint 2")
		started.Status = model.SprintStatusActive
		if err := st.Create(ctx, started); err != nil {
			t.Fatalf("Create: %v", err)
		}

		active, err = st.GetActiveByProductID(ctx, productID)
		if err != nil {
			t.Fatalf("GetActiveByProductID: %v", err)
		}
		if active == nil || active.ID != started.ID {
			t.Fatalf("active = %+v, want sprint %q", active, started.ID)
		}

		// Sprints of another product must not leak through.
		otherProduct := seedSprintProduct(t, db, "Other Product")
		otherActive, err := st.GetActiveByProductID(ctx, otherProduct)
		if err != nil {
			t.Fatalf("GetActiveByProductID (other product): %v", err)
		}
		if otherActive != nil {
			t.Fatalf("active = %+v, want nil", otherActive)
		}
	})
}
