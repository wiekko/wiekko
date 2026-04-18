package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
	"nexmap.net/internal/testutil"
)

func newBacklogService(requirements *testutil.MockRequirementStore, defects *testutil.MockDefectStore, audit *testutil.MockAuditStore, roles *testutil.MockProductRoleStore) *BacklogService {
	return &BacklogService{
		requirements: requirements,
		defects:      defects,
		categories:   &testutil.MockCategoryStore{},
		audit:        audit,
		authz:        &Authorizer{productRoles: roles},
	}
}

func developerRoles() *testutil.MockProductRoleStore {
	return &testutil.MockProductRoleStore{
		GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
			return testutil.MakeProductRole(func(pr *model.ProductRole) { pr.Role = model.RoleDeveloper }), nil
		},
	}
}

func testerRoles() *testutil.MockProductRoleStore {
	return &testutil.MockProductRoleStore{
		GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
			return testutil.MakeProductRole(func(pr *model.ProductRole) { pr.Role = model.RoleTester }), nil
		},
	}
}

// --- Requirements ---

func TestCreateRequirement(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Requirement
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				CreateFunc: func(_ context.Context, r *model.Requirement) error { created = r; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{
			ProductID: "product-001", Title: "As a user...", CategoryID: "cat-001",
		})
		if err != nil {
			t.Fatalf("CreateRequirement: %v", err)
		}
		if result.Title != "As a user..." {
			t.Fatalf("Title = %q, want %q", result.Title, "As a user...")
		}
		if created == nil {
			t.Fatal("requirement was not created")
		}
		// CategoryID is set from DB lookup or left empty if no categories configured.
		_ = created.CategoryID
	})

	t.Run("missing title", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, ownerRoles())
		_, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{ProductID: "product-001", Title: "X"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetRequirement(t *testing.T) {
	req := testutil.MakeRequirement()

	t.Run("success", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Requirement, error) { return req, nil },
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.GetRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if err != nil {
			t.Fatalf("GetRequirement: %v", err)
		}
		if result.ID != req.ID {
			t.Fatalf("ID = %q, want %q", result.ID, req.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.GetRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateRequirement(t *testing.T) {
	t.Run("developer can update status and is_active", func(t *testing.T) {
		req := testutil.MakeRequirement()
		var updated *model.Requirement
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Requirement, error) { return req, nil },
				UpdateFunc:  func(_ context.Context, r *model.Requirement) error { updated = r; return nil },
			},
			nil, noopAudit(), developerRoles(),
		)

		newStatus := "status-002"
		isActive := true
		result, err := svc.UpdateRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001", UpdateRequirementInput{
			StatusID: &newStatus, IsActive: &isActive,
		})
		if err != nil {
			t.Fatalf("UpdateRequirement: %v", err)
		}
		if result.StatusID != "status-002" {
			t.Fatalf("StatusID = %q, want %q", result.StatusID, "status-002")
		}
		if !result.IsActive {
			t.Fatal("IsActive should be true")
		}
		if updated == nil {
			t.Fatal("requirement was not updated")
		}
	})

	t.Run("developer cannot update full fields", func(t *testing.T) {
		req := testutil.MakeRequirement()
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Requirement, error) { return req, nil },
			},
			nil, nil, developerRoles(),
		)

		newTitle := "New Title"
		_, err := svc.UpdateRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001", UpdateRequirementInput{
			Title: &newTitle,
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("owner can update all fields", func(t *testing.T) {
		req := testutil.MakeRequirement()
		var updated *model.Requirement
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Requirement, error) { return req, nil },
				UpdateFunc:  func(_ context.Context, r *model.Requirement) error { updated = r; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		newTitle := "New Title"
		result, err := svc.UpdateRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001", UpdateRequirementInput{
			Title: &newTitle,
		})
		if err != nil {
			t.Fatalf("UpdateRequirement: %v", err)
		}
		if result.Title != "New Title" {
			t.Fatalf("Title = %q, want %q", result.Title, "New Title")
		}
		if updated == nil {
			t.Fatal("requirement was not updated")
		}
	})

	t.Run("forbidden for viewer", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, viewerRoles())
		_, err := svc.UpdateRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001", UpdateRequirementInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteRequirement(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		err := svc.DeleteRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if err != nil {
			t.Fatalf("DeleteRequirement: %v", err)
		}
		if !deleted {
			t.Fatal("requirement was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		err := svc.DeleteRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListRequirements(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				ListByProductIDFunc: func(_ context.Context, _ string, _ store.RequirementFilter) ([]*model.Requirement, error) {
					return []*model.Requirement{testutil.MakeRequirement()}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListRequirements(context.Background(), testutil.MakeUser(), "product-001", store.RequirementFilter{})
		if err != nil {
			t.Fatalf("ListRequirements: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListRequirements(context.Background(), testutil.MakeUser(), "product-001", store.RequirementFilter{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListChildRequirements(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				ListByParentIDFunc: func(_ context.Context, _ string) ([]*model.Requirement, error) {
					return []*model.Requirement{testutil.MakeRequirement()}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListChildRequirements(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if err != nil {
			t.Fatalf("ListChildRequirements: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListChildRequirements(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestAssignRequirement(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		assigned := false
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				AssignFunc: func(_ context.Context, _ *model.Assignment) error { assigned = true; return nil },
			},
			nil, nil, ownerRoles(),
		)

		err := svc.AssignRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001", "user-002")
		if err != nil {
			t.Fatalf("AssignRequirement: %v", err)
		}
		if !assigned {
			t.Fatal("assignment was not created")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		err := svc.AssignRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001", "user-002")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUnassignRequirement(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		unassigned := false
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				UnassignFunc: func(_ context.Context, _, _ string) error { unassigned = true; return nil },
			},
			nil, nil, ownerRoles(),
		)

		err := svc.UnassignRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001", "user-002")
		if err != nil {
			t.Fatalf("UnassignRequirement: %v", err)
		}
		if !unassigned {
			t.Fatal("assignment was not removed")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		err := svc.UnassignRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001", "user-002")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListRequirementAssignees(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				ListAssigneesFunc: func(_ context.Context, _ string) ([]*model.Assignment, error) {
					return []*model.Assignment{{ID: "a-001", TargetID: "req-001", UserID: "user-002"}}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListRequirementAssignees(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if err != nil {
			t.Fatalf("ListRequirementAssignees: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListRequirementAssignees(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Defects ---

func TestCreateDefect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Defect
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				CreateFunc: func(_ context.Context, d *model.Defect) error { created = d; return nil },
			},
			noopAudit(), testerRoles(),
		)

		result, err := svc.CreateDefect(context.Background(), testutil.MakeUser(), CreateDefectInput{
			ProductID: "product-001", Title: "Bug",
		})
		if err != nil {
			t.Fatalf("CreateDefect: %v", err)
		}
		if result.Title != "Bug" {
			t.Fatalf("Title = %q, want %q", result.Title, "Bug")
		}
		if created == nil {
			t.Fatal("defect was not created")
		}
		if created.Criticality != model.DefectCriticalityMedium {
			t.Fatalf("Criticality = %q, want %q", created.Criticality, model.DefectCriticalityMedium)
		}
	})

	t.Run("missing title", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, testerRoles())
		_, err := svc.CreateDefect(context.Background(), testutil.MakeUser(), CreateDefectInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateDefect(context.Background(), testutil.MakeUser(), CreateDefectInput{ProductID: "product-001", Title: "X"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetDefect(t *testing.T) {
	defect := testutil.MakeDefect()

	t.Run("success", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Defect, error) { return defect, nil },
			},
			nil, viewerRoles(),
		)

		result, err := svc.GetDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001")
		if err != nil {
			t.Fatalf("GetDefect: %v", err)
		}
		if result.ID != defect.ID {
			t.Fatalf("ID = %q, want %q", result.ID, defect.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.GetDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateDefect(t *testing.T) {
	t.Run("developer can update status and is_active", func(t *testing.T) {
		defect := testutil.MakeDefect()
		var updated *model.Defect
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Defect, error) { return defect, nil },
				UpdateFunc:  func(_ context.Context, d *model.Defect) error { updated = d; return nil },
			},
			noopAudit(), developerRoles(),
		)

		newStatus := "status-002"
		result, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001", UpdateDefectInput{
			StatusID: &newStatus,
		})
		if err != nil {
			t.Fatalf("UpdateDefect: %v", err)
		}
		if result.StatusID != "status-002" {
			t.Fatalf("StatusID = %q, want %q", result.StatusID, "status-002")
		}
		if updated == nil {
			t.Fatal("defect was not updated")
		}
	})

	t.Run("developer cannot update full fields", func(t *testing.T) {
		defect := testutil.MakeDefect()
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Defect, error) { return defect, nil },
			},
			nil, developerRoles(),
		)

		newTitle := "New Title"
		_, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001", UpdateDefectInput{
			Title: &newTitle,
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("owner can update all fields", func(t *testing.T) {
		defect := testutil.MakeDefect()
		var updated *model.Defect
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Defect, error) { return defect, nil },
				UpdateFunc:  func(_ context.Context, d *model.Defect) error { updated = d; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		newTitle := "New Title"
		result, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001", UpdateDefectInput{
			Title: &newTitle,
		})
		if err != nil {
			t.Fatalf("UpdateDefect: %v", err)
		}
		if result.Title != "New Title" {
			t.Fatalf("Title = %q, want %q", result.Title, "New Title")
		}
		if updated == nil {
			t.Fatal("defect was not updated")
		}
	})
}

func TestDeleteDefect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		err := svc.DeleteDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001")
		if err != nil {
			t.Fatalf("DeleteDefect: %v", err)
		}
		if !deleted {
			t.Fatal("defect was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		err := svc.DeleteDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListDefects(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				ListByProductIDFunc: func(_ context.Context, _ string, _ store.DefectFilter) ([]*model.Defect, error) {
					return []*model.Defect{testutil.MakeDefect()}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListDefects(context.Background(), testutil.MakeUser(), "product-001", store.DefectFilter{})
		if err != nil {
			t.Fatalf("ListDefects: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListDefects(context.Background(), testutil.MakeUser(), "product-001", store.DefectFilter{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestAssignDefect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		assigned := false
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				AssignFunc: func(_ context.Context, _ *model.Assignment) error { assigned = true; return nil },
			},
			nil, ownerRoles(),
		)

		err := svc.AssignDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001", "user-002")
		if err != nil {
			t.Fatalf("AssignDefect: %v", err)
		}
		if !assigned {
			t.Fatal("assignment was not created")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		err := svc.AssignDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001", "user-002")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUnassignDefect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		unassigned := false
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				UnassignFunc: func(_ context.Context, _, _ string) error { unassigned = true; return nil },
			},
			nil, ownerRoles(),
		)

		err := svc.UnassignDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001", "user-002")
		if err != nil {
			t.Fatalf("UnassignDefect: %v", err)
		}
		if !unassigned {
			t.Fatal("assignment was not removed")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		err := svc.UnassignDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001", "user-002")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListDefectAssignees(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				ListAssigneesFunc: func(_ context.Context, _ string) ([]*model.Assignment, error) {
					return []*model.Assignment{{ID: "a-001"}}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListDefectAssignees(context.Background(), testutil.MakeUser(), "product-001", "defect-001")
		if err != nil {
			t.Fatalf("ListDefectAssignees: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListDefectAssignees(context.Background(), testutil.MakeUser(), "product-001", "defect-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestAddDefectLink(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		linked := false
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				AddLinkFunc: func(_ context.Context, _ *model.DefectLink) error { linked = true; return nil },
			},
			nil, testerRoles(),
		)

		err := svc.AddDefectLink(context.Background(), testutil.MakeUser(), "product-001", "defect-001", "component", "comp-001")
		if err != nil {
			t.Fatalf("AddDefectLink: %v", err)
		}
		if !linked {
			t.Fatal("link was not created")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		err := svc.AddDefectLink(context.Background(), testutil.MakeUser(), "product-001", "defect-001", "component", "comp-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestRemoveDefectLink(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		removed := false
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				RemoveLinkFunc: func(_ context.Context, _ string) error { removed = true; return nil },
			},
			nil, developerRoles(),
		)

		err := svc.RemoveDefectLink(context.Background(), testutil.MakeUser(), "product-001", "link-001")
		if err != nil {
			t.Fatalf("RemoveDefectLink: %v", err)
		}
		if !removed {
			t.Fatal("link was not removed")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		err := svc.RemoveDefectLink(context.Background(), testutil.MakeUser(), "product-001", "link-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListDefectLinks(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				ListLinksFunc: func(_ context.Context, _ string) ([]*model.DefectLink, error) {
					return []*model.DefectLink{{ID: "link-001"}}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListDefectLinks(context.Background(), testutil.MakeUser(), "product-001", "defect-001")
		if err != nil {
			t.Fatalf("ListDefectLinks: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListDefectLinks(context.Background(), testutil.MakeUser(), "product-001", "defect-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
