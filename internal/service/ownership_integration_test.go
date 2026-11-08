package service_test

// Cross-product isolation through the real service layer and SQL: a member of
// Aviary (and an administrator) acting under Aviary's product ID must not be
// able to read or change Finch Tracker's entities by naming their IDs. This
// lives in the external test package because it seeds through devseed, which
// imports service.

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/auth/builtin"
	"nexmap.net/internal/config"
	"nexmap.net/internal/crypto"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/devseed"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

type isolationEnv struct {
	svc           *service.Services
	admin, dev    *model.User // dev: developer in Aviary, not a member of Finch
	aviary, finch string
}

func newIsolationEnv(t *testing.T, driver string, conn *sql.DB) *isolationEnv {
	t.Helper()
	ctx := context.Background()
	st := store.New(conn, store.Dialect(driver), crypto.TestKeyRing())
	provider := builtin.New(st.User, 1)
	blobs, err := storage.NewFS(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(st, provider, &config.Config{SessionTTL: time.Hour, PBKDF2Iterations: 1}, blobs)

	now := time.Now().UTC().Format(time.RFC3339)
	admin := &model.User{
		ID: uuid.NewString(), Username: "admin", Email: "admin@localhost", DisplayName: "Admin",
		PasswordHash: "x", IsAdmin: true, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := st.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := devseed.Seed(ctx, svc, st, provider, admin); err != nil {
		t.Fatalf("seed: %v", err)
	}
	dev, err := st.User.GetByUsername(ctx, "cy")
	if err != nil {
		t.Fatalf("demo user cy: %v", err)
	}
	env := &isolationEnv{svc: svc, admin: admin, dev: dev}
	products, err := svc.Product.List(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range products {
		switch p.Name {
		case "Aviary":
			env.aviary = p.ID
		case "Finch Tracker":
			env.finch = p.ID
		}
	}
	if env.aviary == "" || env.finch == "" {
		t.Fatalf("seed products missing: %+v", products)
	}
	if _, err := svc.Backlog.ListRequirements(ctx, dev, env.finch, store.RequirementFilter{}); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("precondition: cy must not be a Finch member, got %v", err)
	}
	return env
}

// first returns a function taking a list call's results and returning its
// first item, so it can wrap a multi-value call: first[T](t, "x")(svc.List…).
func first[T any](t *testing.T, what string) func([]T, error) T {
	return func(items []T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatalf("list %s: %v", what, err)
		}
		if len(items) == 0 {
			t.Fatalf("seed has no %s", what)
		}
		return items[0]
	}
}

func wantNotFound(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("%s: err = %v, want ErrNotFound", what, err)
	}
}

func TestCrossProductIsolation(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newIsolationEnv(t, driver, conn)
		ctx := context.Background()
		svc, admin, dev, aviary, finch := e.svc, e.admin, e.dev, e.aviary, e.finch

		// Finch entities, looked up legitimately.
		fReq := first[*model.Requirement](t, "finch requirements")(svc.Backlog.ListRequirements(ctx, admin, finch, store.RequirementFilter{}))
		fDefect := first[*model.Defect](t, "finch defects")(svc.Backlog.ListDefects(ctx, admin, finch, store.DefectFilter{}))
		fTC := first[*model.TestCase](t, "finch test cases")(svc.Testing.ListTestCases(ctx, admin, finch))
		fStatus := first[*model.Status](t, "finch statuses")(svc.Milestone.ListStatuses(ctx, admin, finch, "requirement"))
		fEnv := first[*model.RuntimeEnvironment](t, "finch environments")(svc.Milestone.ListRuntimeEnvironments(ctx, admin, finch))
		fCat := first[*model.Category](t, "finch categories")(svc.Category.ListCategories(ctx, admin, finch, "requirement"))
		// Aviary entities to act from.
		aReq := first[*model.Requirement](t, "aviary requirements")(svc.Backlog.ListRequirements(ctx, admin, aviary, store.RequirementFilter{}))
		aDefect := first[*model.Defect](t, "aviary defects")(svc.Backlog.ListDefects(ctx, admin, aviary, store.DefectFilter{}))
		aCap := first[*model.Capability](t, "aviary capabilities")(svc.Component.ListCapabilities(ctx, admin, aviary))

		t.Run("positive control", func(t *testing.T) {
			if _, err := svc.Backlog.GetRequirement(ctx, dev, aviary, aReq.ID); err != nil {
				t.Fatalf("own requirement: %v", err)
			}
			if _, err := svc.Audit.ListByEntity(ctx, dev, aviary, "requirement", aReq.ID); err != nil {
				t.Fatalf("own audit trail: %v", err)
			}
		})

		for _, u := range []*model.User{dev, admin} {
			name := "member"
			if u.IsAdmin {
				name = "admin"
			}
			t.Run(name+" reads", func(t *testing.T) {
				_, err := svc.Backlog.GetRequirement(ctx, u, aviary, fReq.ID)
				wantNotFound(t, "GetRequirement", err)
				_, err = svc.Backlog.ListChildRequirements(ctx, u, aviary, fReq.ID)
				wantNotFound(t, "ListChildRequirements", err)
				_, err = svc.Backlog.ListRequirementAssignees(ctx, u, aviary, fReq.ID)
				wantNotFound(t, "ListRequirementAssignees", err)
				_, err = svc.Backlog.GetDefect(ctx, u, aviary, fDefect.ID)
				wantNotFound(t, "GetDefect", err)
				_, err = svc.Backlog.ListDefectLinks(ctx, u, aviary, fDefect.ID)
				wantNotFound(t, "ListDefectLinks", err)
				_, err = svc.Testing.GetTestCase(ctx, u, aviary, fTC.ID)
				wantNotFound(t, "GetTestCase", err)
				_, err = svc.Testing.ListResultsForTestCase(ctx, u, aviary, fTC.ID)
				wantNotFound(t, "ListResultsForTestCase", err)
				_, err = svc.Milestone.GetStatus(ctx, u, aviary, fStatus.ID)
				wantNotFound(t, "GetStatus", err)
				_, err = svc.Category.GetCategory(ctx, u, aviary, fCat.ID)
				wantNotFound(t, "GetCategory", err)
				_, err = svc.Audit.ListByEntity(ctx, u, aviary, "requirement", fReq.ID)
				wantNotFound(t, "Audit.ListByEntity", err)
				_, err = svc.Component.ListRequirementCapabilities(ctx, u, aviary, fReq.ID)
				wantNotFound(t, "ListRequirementCapabilities", err)
			})
		}

		t.Run("member writes", func(t *testing.T) {
			title := "hijacked"
			_, err := svc.Backlog.UpdateRequirement(ctx, dev, aviary, fReq.ID, service.UpdateRequirementInput{StatusID: &fStatus.ID})
			wantNotFound(t, "UpdateRequirement", err)
			_, err = svc.Backlog.UpdateDefect(ctx, dev, aviary, fDefect.ID, service.UpdateDefectInput{Title: &title})
			wantNotFound(t, "UpdateDefect", err)
			_, err = svc.Discussion.CreateComment(ctx, dev, aviary, model.OwnerTypeRequirement, fReq.ID, "hello")
			wantNotFound(t, "CreateComment on foreign owner", err)
			// A reference to a Finch entity inside an Aviary write.
			err = svc.Backlog.AddDefectLink(ctx, dev, aviary, aDefect.ID, model.DefectLinkTargetRequirement, fReq.ID)
			wantNotFound(t, "AddDefectLink to foreign target", err)
		})

		t.Run("admin writes", func(t *testing.T) {
			err := svc.Backlog.DeleteRequirement(ctx, admin, aviary, fReq.ID)
			wantNotFound(t, "DeleteRequirement", err)
			err = svc.Backlog.DeleteDefect(ctx, admin, aviary, fDefect.ID)
			wantNotFound(t, "DeleteDefect", err)
			err = svc.Milestone.DeleteRuntimeEnvironment(ctx, admin, aviary, fEnv.ID)
			wantNotFound(t, "DeleteRuntimeEnvironment", err)
			err = svc.Component.LinkRequirementToCapability(ctx, admin, finch, aCap.ID, fReq.ID)
			wantNotFound(t, "LinkRequirementToCapability with foreign capability", err)
			_, err = svc.Backlog.UpdateRequirement(ctx, admin, aviary, aReq.ID, service.UpdateRequirementInput{StatusID: &fStatus.ID})
			wantNotFound(t, "UpdateRequirement with foreign status", err)
			_, err = svc.Backlog.CreateRequirement(ctx, admin, service.CreateRequirementInput{ProductID: aviary, CategoryID: fCat.ID, Title: "x"})
			wantNotFound(t, "CreateRequirement with foreign category", err)
		})

		t.Run("finch data untouched", func(t *testing.T) {
			r, err := svc.Backlog.GetRequirement(ctx, admin, finch, fReq.ID)
			if err != nil {
				t.Fatalf("finch requirement gone: %v", err)
			}
			if r.StatusID != fReq.StatusID {
				t.Errorf("finch requirement status changed: %q → %q", fReq.StatusID, r.StatusID)
			}
			d, err := svc.Backlog.GetDefect(ctx, admin, finch, fDefect.ID)
			if err != nil || d.Title != fDefect.Title {
				t.Errorf("finch defect changed or gone: %+v, %v", d, err)
			}
			envs, err := svc.Milestone.ListRuntimeEnvironments(ctx, admin, finch)
			if err != nil || len(envs) == 0 || envs[0].ID != fEnv.ID {
				t.Errorf("finch environment gone: %v", err)
			}
			links, err := svc.Backlog.ListDefectLinks(ctx, admin, aviary, aDefect.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range links {
				if l.TargetID == fReq.ID {
					t.Error("foreign defect link was created")
				}
			}
		})
	})
}
