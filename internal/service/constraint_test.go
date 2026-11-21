// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
	"nexmap.net/internal/testutil"
)

func newConstraintService(constraints *testutil.MockConstraintStore, categories *testutil.MockCategoryStore, audit *testutil.MockAuditStore, roles *testutil.MockProductRoleStore) *ConstraintService {
	if categories == nil {
		categories = constraintCategories()
	}
	return &ConstraintService{
		owns:        testOwnership(),
		constraints: constraints,
		categories:  categories,
		comments:    &recordingPurger{},
		audit:       audit,
		authz:       &Authorizer{productRoles: roles},
	}
}

// constraintCategories returns a category store whose lookups yield a
// constraint-typed category, which CreateConstraint requires.
func constraintCategories() *testutil.MockCategoryStore {
	return &testutil.MockCategoryStore{
		GetByIDFunc: func(_ context.Context, id string) (*model.Category, error) {
			return &model.Category{ID: id, ProductID: "product-001", EntityType: "constraint", Name: "Architecture", Prefix: "AC"}, nil
		},
	}
}

func TestCreateConstraint(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Constraint
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				CreateFunc:             func(_ context.Context, c *model.Constraint) error { created = c; return nil },
				NextSequenceNumberFunc: func(_ context.Context, _ string) (int, error) { return 7, nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateConstraint(context.Background(), testutil.MakeUser(), CreateConstraintInput{
			ProductID: "product-001", Title: "No secrets in logs", CategoryID: "cat-001",
		})
		if err != nil {
			t.Fatalf("CreateConstraint: %v", err)
		}
		if result.Title != "No secrets in logs" {
			t.Fatalf("Title = %q, want %q", result.Title, "No secrets in logs")
		}
		if result.SequenceNumber != 7 {
			t.Fatalf("SequenceNumber = %d, want 7", result.SequenceNumber)
		}
		if created == nil {
			t.Fatal("constraint was not created")
		}
	})

	t.Run("defaults compliance state to not assessed", func(t *testing.T) {
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				CreateFunc: func(_ context.Context, _ *model.Constraint) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateConstraint(context.Background(), testutil.MakeUser(), CreateConstraintInput{
			ProductID: "product-001", Title: "X", CategoryID: "cat-001",
		})
		if err != nil {
			t.Fatalf("CreateConstraint: %v", err)
		}
		if result.ComplianceState != model.ComplianceNotAssessed {
			t.Fatalf("ComplianceState = %q, want %q", result.ComplianceState, model.ComplianceNotAssessed)
		}
	})

	t.Run("missing title", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, ownerRoles())
		_, err := svc.CreateConstraint(context.Background(), testutil.MakeUser(), CreateConstraintInput{
			ProductID: "product-001", CategoryID: "cat-001",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing category", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, ownerRoles())
		_, err := svc.CreateConstraint(context.Background(), testutil.MakeUser(), CreateConstraintInput{
			ProductID: "product-001", Title: "X",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects a non-constraint category", func(t *testing.T) {
		created := false
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				CreateFunc: func(_ context.Context, _ *model.Constraint) error { created = true; return nil },
			},
			&testutil.MockCategoryStore{
				GetByIDFunc: func(_ context.Context, id string) (*model.Category, error) {
					return &model.Category{ID: id, EntityType: "requirement"}, nil
				},
			},
			noopAudit(), ownerRoles(),
		)

		_, err := svc.CreateConstraint(context.Background(), testutil.MakeUser(), CreateConstraintInput{
			ProductID: "product-001", Title: "X", CategoryID: "cat-001",
		})
		if err == nil {
			t.Fatal("expected error for a requirement category")
		}
		if created {
			t.Fatal("constraint was created despite an invalid category")
		}
	})

	t.Run("forbidden for viewer", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, viewerRoles())
		_, err := svc.CreateConstraint(context.Background(), testutil.MakeUser(), CreateConstraintInput{
			ProductID: "product-001", Title: "X", CategoryID: "cat-001",
		})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetConstraint(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		con := testutil.MakeConstraint()
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Constraint, error) { return con, nil },
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.GetConstraint(context.Background(), testutil.MakeUser(), "product-001", "constraint-001")
		if err != nil {
			t.Fatalf("GetConstraint: %v", err)
		}
		if result.ID != con.ID {
			t.Fatalf("ID = %q, want %q", result.ID, con.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, forbiddenRoles())
		_, err := svc.GetConstraint(context.Background(), testutil.MakeUser(), "product-001", "constraint-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateConstraint(t *testing.T) {
	t.Run("owner can update all fields", func(t *testing.T) {
		con := testutil.MakeConstraint()
		var updated *model.Constraint
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Constraint, error) { return con, nil },
				UpdateFunc:  func(_ context.Context, c *model.Constraint) error { updated = c; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		title := "Reworded"
		state := model.ComplianceViolated
		priority := 5
		result, err := svc.UpdateConstraint(context.Background(), testutil.MakeUser(), "product-001", "constraint-001", UpdateConstraintInput{
			Title: &title, ComplianceState: &state, Priority: &priority,
		})
		if err != nil {
			t.Fatalf("UpdateConstraint: %v", err)
		}
		if result.Title != "Reworded" {
			t.Fatalf("Title = %q, want %q", result.Title, "Reworded")
		}
		if result.ComplianceState != model.ComplianceViolated {
			t.Fatalf("ComplianceState = %q, want %q", result.ComplianceState, model.ComplianceViolated)
		}
		if result.Priority != 5 {
			t.Fatalf("Priority = %d, want 5", result.Priority)
		}
		if updated == nil {
			t.Fatal("constraint was not updated")
		}
	})

	t.Run("nil fields are left unchanged", func(t *testing.T) {
		con := testutil.MakeConstraint(func(c *model.Constraint) { c.Title = "Original" })
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Constraint, error) { return con, nil },
				UpdateFunc:  func(_ context.Context, _ *model.Constraint) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.UpdateConstraint(context.Background(), testutil.MakeUser(), "product-001", "constraint-001", UpdateConstraintInput{})
		if err != nil {
			t.Fatalf("UpdateConstraint: %v", err)
		}
		if result.Title != "Original" {
			t.Fatalf("Title = %q, want it unchanged", result.Title)
		}
	})

	t.Run("clears the milestone when given an empty string", func(t *testing.T) {
		con := testutil.MakeConstraint(func(c *model.Constraint) { c.MilestoneID = "milestone-001" })
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Constraint, error) { return con, nil },
				UpdateFunc:  func(_ context.Context, _ *model.Constraint) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		empty := ""
		result, err := svc.UpdateConstraint(context.Background(), testutil.MakeUser(), "product-001", "constraint-001", UpdateConstraintInput{
			MilestoneID: &empty,
		})
		if err != nil {
			t.Fatalf("UpdateConstraint: %v", err)
		}
		if result.MilestoneID != "" {
			t.Fatalf("MilestoneID = %q, want empty", result.MilestoneID)
		}
	})

	t.Run("forbidden for developer", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, developerRoles())
		_, err := svc.UpdateConstraint(context.Background(), testutil.MakeUser(), "product-001", "constraint-001", UpdateConstraintInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteConstraint(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		if err := svc.DeleteConstraint(context.Background(), testutil.MakeUser(), "product-001", "constraint-001"); err != nil {
			t.Fatalf("DeleteConstraint: %v", err)
		}
		if !deleted {
			t.Fatal("constraint was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, forbiddenRoles())
		err := svc.DeleteConstraint(context.Background(), testutil.MakeUser(), "product-001", "constraint-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListConstraints(t *testing.T) {
	t.Run("passes the filter through", func(t *testing.T) {
		var got store.ConstraintFilter
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				ListByProductIDFunc: func(_ context.Context, _ string, filter store.ConstraintFilter) ([]*model.Constraint, error) {
					got = filter
					return []*model.Constraint{testutil.MakeConstraint()}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListConstraints(context.Background(), testutil.MakeUser(), "product-001", store.ConstraintFilter{CategoryID: "cat-002"})
		if err != nil {
			t.Fatalf("ListConstraints: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
		if got.CategoryID != "cat-002" {
			t.Fatalf("filter.CategoryID = %q, want cat-002", got.CategoryID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListConstraints(context.Background(), testutil.MakeUser(), "product-001", store.ConstraintFilter{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListConstraintsByMilestone(t *testing.T) {
	t.Run("skips constraints from another product", func(t *testing.T) {
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				ListByMilestoneIDFunc: func(_ context.Context, milestoneID string) ([]*model.Constraint, error) {
					if milestoneID != "milestone-001" {
						t.Fatalf("milestoneID = %q, want milestone-001", milestoneID)
					}
					return []*model.Constraint{
						testutil.MakeConstraint(),
						testutil.MakeConstraint(func(c *model.Constraint) {
							c.ID = "constraint-002"
							c.ProductID = "other-product"
						}),
					}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListConstraintsByMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if err != nil {
			t.Fatalf("ListConstraintsByMilestone: %v", err)
		}
		if len(result) != 1 || result[0].ID != "constraint-001" {
			t.Fatalf("result = %v, want just constraint-001", result)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListConstraintsByMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestConstraintCapabilityLinks(t *testing.T) {
	t.Run("link success", func(t *testing.T) {
		var linked *model.ConstraintCapability
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				LinkCapabilityFunc: func(_ context.Context, l *model.ConstraintCapability) error { linked = l; return nil },
			},
			nil, nil, ownerRoles(),
		)

		if err := svc.LinkCapability(context.Background(), testutil.MakeUser(), "product-001", "constraint-001", "capability-001"); err != nil {
			t.Fatalf("LinkCapability: %v", err)
		}
		if linked == nil {
			t.Fatal("link was not created")
		}
		if linked.ConstraintID != "constraint-001" || linked.CapabilityID != "capability-001" {
			t.Fatalf("link = %+v, want constraint-001 → capability-001", linked)
		}
		if linked.ID == "" {
			t.Fatal("link ID was not assigned")
		}
	})

	t.Run("link forbidden for developer", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, developerRoles())
		err := svc.LinkCapability(context.Background(), testutil.MakeUser(), "product-001", "constraint-001", "capability-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("unlink success", func(t *testing.T) {
		unlinked := false
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				UnlinkCapabilityFunc: func(_ context.Context, conID, capID string) error {
					if conID != "constraint-001" || capID != "capability-001" {
						t.Fatalf("unlink args = (%q, %q)", conID, capID)
					}
					unlinked = true
					return nil
				},
			},
			nil, nil, ownerRoles(),
		)

		if err := svc.UnlinkCapability(context.Background(), testutil.MakeUser(), "product-001", "constraint-001", "capability-001"); err != nil {
			t.Fatalf("UnlinkCapability: %v", err)
		}
		if !unlinked {
			t.Fatal("link was not removed")
		}
	})

	t.Run("unlink forbidden for developer", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, developerRoles())
		err := svc.UnlinkCapability(context.Background(), testutil.MakeUser(), "product-001", "constraint-001", "capability-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("list success", func(t *testing.T) {
		svc := newConstraintService(
			&testutil.MockConstraintStore{
				ListCapabilitiesByConstraintFunc: func(_ context.Context, _ string) ([]*model.ConstraintCapability, error) {
					return []*model.ConstraintCapability{testutil.MakeConstraintCapability()}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListConstraintCapabilities(context.Background(), testutil.MakeUser(), "product-001", "constraint-001")
		if err != nil {
			t.Fatalf("ListConstraintCapabilities: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("list forbidden", func(t *testing.T) {
		svc := newConstraintService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListConstraintCapabilities(context.Background(), testutil.MakeUser(), "product-001", "constraint-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
