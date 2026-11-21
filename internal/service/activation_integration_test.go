// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"wiekko.net/internal/auth/builtin"
	"wiekko.net/internal/config"
	"wiekko.net/internal/crypto"
	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/model"
	"wiekko.net/internal/storage"
	"wiekko.net/internal/store"
)

// activationEnv is a kanban product with the default columns, an owner and a
// developer, on a real database: activation is computed in SQL, so mocks
// would not exercise it.
type activationEnv struct {
	svc      *Services
	st       *store.Store // for users, which services do not create directly
	owner    *model.User
	dev      *model.User
	product  *model.Product
	statuses map[string]map[string]*model.Status // entity type -> name -> status
}

func newActivationEnv(t *testing.T, driver string, conn *sql.DB) *activationEnv {
	t.Helper()
	ctx := context.Background()
	st := store.New(conn, store.Dialect(driver), crypto.TestKeyRing())
	blobs, err := storage.NewFS(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st, builtin.New(st.User, 1), &config.Config{SessionTTL: time.Hour}, blobs)

	now := time.Now().UTC().Format(time.RFC3339)
	mkUser := func(name string, admin bool) *model.User {
		u := &model.User{
			ID: uuid.NewString(), Username: name, Email: name + "@example.com", DisplayName: name,
			PasswordHash: "x", IsAdmin: admin, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
		}
		if err := st.User.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	e := &activationEnv{
		svc: svc, st: st, owner: mkUser("olga", true), dev: mkUser("dev", false),
		statuses: map[string]map[string]*model.Status{},
	}

	e.product, err = svc.Product.Create(ctx, e.owner, CreateProductInput{Name: "Board", WorkflowMode: model.WorkflowModeKanban})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Product.AddMember(ctx, e.owner, e.product.ID, e.dev.ID, model.RoleDeveloper); err != nil {
		t.Fatal(err)
	}
	for _, et := range []string{"requirement", "defect"} {
		list, err := svc.Milestone.ListStatuses(ctx, e.owner, e.product.ID, et)
		if err != nil {
			t.Fatal(err)
		}
		e.statuses[et] = map[string]*model.Status{}
		for _, s := range list {
			e.statuses[et][s.Name] = s
		}
	}
	return e
}

func (e *activationEnv) status(et, name string) *string {
	id := e.statuses[et][name].ID
	return &id
}

func (e *activationEnv) requirement(t *testing.T) *model.Requirement {
	t.Helper()
	r, err := e.svc.Backlog.CreateRequirement(context.Background(), e.owner, CreateRequirementInput{
		ProductID: e.product.ID, Title: "Item " + uuid.NewString()[:8],
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *activationEnv) reload(t *testing.T, id string) *model.Requirement {
	t.Helper()
	r, err := e.svc.Backlog.GetRequirement(context.Background(), e.owner, e.product.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestKanbanActivationByColumn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newActivationEnv(t, driver, conn)
		ctx := context.Background()
		move := func(u *model.User, id string, status *string) error {
			_, err := e.svc.Backlog.UpdateRequirement(ctx, u, e.product.ID, id, UpdateRequirementInput{StatusID: status})
			return err
		}

		t.Run("default columns", func(t *testing.T) {
			want := map[string]bool{"New": false, "In Progress": true, "In Review": true, "Done": false}
			for _, et := range []string{"requirement", "defect"} {
				for name, active := range want {
					if got := e.statuses[et][name].IsActive; got != active {
						t.Errorf("%s column %q active = %v, want %v", et, name, got, active)
					}
				}
			}
		})

		t.Run("new item is inactive and locked for developers", func(t *testing.T) {
			r := e.requirement(t)
			if r.IsActive || e.reload(t, r.ID).IsActive {
				t.Fatal("item in the default column is active")
			}
			if err := move(e.dev, r.ID, e.status("requirement", "In Progress")); !errors.Is(err, ErrForbidden) {
				t.Fatalf("developer moved an inactive item: %v", err)
			}
		})

		t.Run("owner activates by moving, developer moves on and back out", func(t *testing.T) {
			r := e.requirement(t)
			if err := move(e.owner, r.ID, e.status("requirement", "In Progress")); err != nil {
				t.Fatal(err)
			}
			if !e.reload(t, r.ID).IsActive {
				t.Fatal("item in an active column is not active")
			}
			if err := move(e.dev, r.ID, e.status("requirement", "In Review")); err != nil {
				t.Fatalf("developer move between active columns: %v", err)
			}
			got, err := e.svc.Backlog.UpdateRequirement(ctx, e.dev, e.product.ID, r.ID, UpdateRequirementInput{StatusID: e.status("requirement", "Done")})
			if err != nil {
				t.Fatalf("developer move into an inactive column: %v", err)
			}
			if got.IsActive {
				t.Error("update result still reports the item as active")
			}
			if e.reload(t, r.ID).IsActive {
				t.Fatal("item in Done is still active")
			}
			if err := move(e.dev, r.ID, e.status("requirement", "In Progress")); !errors.Is(err, ErrForbidden) {
				t.Fatalf("developer reopened a finished item: %v", err)
			}
		})

		t.Run("is_active cannot be set directly", func(t *testing.T) {
			r := e.requirement(t)
			on := true
			if _, err := e.svc.Backlog.UpdateRequirement(ctx, e.dev, e.product.ID, r.ID, UpdateRequirementInput{IsActive: &on}); !errors.Is(err, ErrForbidden) {
				t.Errorf("developer activation: %v, want forbidden", err)
			}
			if _, err := e.svc.Backlog.UpdateRequirement(ctx, e.owner, e.product.ID, r.ID, UpdateRequirementInput{IsActive: &on}); err == nil {
				t.Error("owner set is_active in kanban mode")
			}
		})

		t.Run("defects follow the same rule", func(t *testing.T) {
			d, err := e.svc.Backlog.CreateDefect(ctx, e.dev, CreateDefectInput{ProductID: e.product.ID, Title: "Crash", Criticality: "major"})
			if err != nil {
				t.Fatal(err)
			}
			upd := func(u *model.User, name string) error {
				_, err := e.svc.Backlog.UpdateDefect(ctx, u, e.product.ID, d.ID, UpdateDefectInput{StatusID: e.status("defect", name)})
				return err
			}
			if err := upd(e.dev, "In Progress"); !errors.Is(err, ErrForbidden) {
				t.Fatalf("developer moved an inactive defect: %v", err)
			}
			if err := upd(e.owner, "In Progress"); err != nil {
				t.Fatal(err)
			}
			if err := upd(e.dev, "Done"); err != nil {
				t.Fatalf("developer closed an active defect: %v", err)
			}
		})

		t.Run("toggling a column changes its items", func(t *testing.T) {
			r := e.requirement(t)
			on, off := true, false
			newCol := e.statuses["requirement"]["New"]
			if _, err := e.svc.Milestone.UpdateStatus(ctx, e.owner, e.product.ID, newCol.ID, UpdateStatusInput{IsActive: &on}); err != nil {
				t.Fatal(err)
			}
			if !e.reload(t, r.ID).IsActive {
				t.Fatal("item did not become active with its column")
			}
			// Status sync is on by default, so the defect column followed.
			defNew, err := e.svc.Milestone.GetStatus(ctx, e.owner, e.product.ID, e.statuses["defect"]["New"].ID)
			if err != nil || !defNew.IsActive {
				t.Fatalf("synced defect column not activated: %+v, %v", defNew, err)
			}
			if _, err := e.svc.Milestone.UpdateStatus(ctx, e.owner, e.product.ID, newCol.ID, UpdateStatusInput{IsActive: &off}); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("switching to scrum activates by sprint", func(t *testing.T) {
			r := e.requirement(t)
			if err := move(e.owner, r.ID, e.status("requirement", "In Progress")); err != nil {
				t.Fatal(err)
			}
			scrum := model.WorkflowModeScrum
			if _, err := e.svc.Product.Update(ctx, e.owner, e.product.ID, UpdateProductInput{WorkflowMode: &scrum}); err != nil {
				t.Fatal(err)
			}
			if e.reload(t, r.ID).IsActive {
				t.Fatal("item without a sprint is active after switching to scrum")
			}
			on := true
			if _, err := e.svc.Backlog.UpdateRequirement(ctx, e.owner, e.product.ID, r.ID, UpdateRequirementInput{IsActive: &on}); err != nil {
				t.Fatalf("owner activation in scrum mode: %v", err)
			}
			if !e.reload(t, r.ID).IsActive {
				t.Fatal("owner activation in scrum mode did not stick")
			}
		})
	})
}
