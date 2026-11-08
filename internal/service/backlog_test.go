package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
	"nexmap.net/internal/testutil"
)

func newBacklogService(requirements *testutil.MockRequirementStore, defects *testutil.MockDefectStore, audit *testutil.MockAuditStore, roles *testutil.MockProductRoleStore) *BacklogService {
	return &BacklogService{
		owns:         testOwnership(),
		requirements: requirements,
		defects:      defects,
		categories:   backlogCategories(),
		statuses:     backlogStatuses(),
		comments:     &recordingPurger{},
		audit:        audit,
		authz:        &Authorizer{productRoles: roles},
	}
}

// backlogStatuses returns the seeded workflow for each entity type, with "New"
// ("<entityType>-new") as the default and "Done" ("<entityType>-done") as the
// closed status. GetByID finds nothing, so status-change side effects (such as
// re-testing on defect close) stay off unless a test wires its own store.
func backlogStatuses() *testutil.MockStatusStore {
	return &testutil.MockStatusStore{
		GetByIDFunc: func(context.Context, string) (*model.Status, error) { return nil, errors.New("status not found") },
		ListByProductIDFunc: func(_ context.Context, productID, entityType string) ([]*model.Status, error) {
			return []*model.Status{
				{ID: entityType + "-new", ProductID: productID, EntityType: entityType, Name: "New", Position: 1, IsDefault: true},
				{ID: entityType + "-done", ProductID: productID, EntityType: entityType, Name: "Done", Position: 2, IsClosed: true},
			}, nil
		},
	}
}

// backlogCategories resolves "cat-*" IDs as requirement categories and
// "taskcat-*" IDs as task categories, so kind checks can be exercised without
// each test wiring its own category store.
func backlogCategories() *testutil.MockCategoryStore {
	return &testutil.MockCategoryStore{
		GetByIDFunc: func(_ context.Context, id string) (*model.Category, error) {
			return testutil.MakeCategory(func(c *model.Category) {
				c.ID = id
				if strings.HasPrefix(id, "taskcat") {
					c.EntityType = model.RequirementKindTask
					c.Prefix = "MT"
					c.Name = "Maintenance"
				}
			}), nil
		},
		ListByProductIDFunc: func(_ context.Context, _, entityType string) ([]*model.Category, error) {
			id := "cat-001"
			if entityType == model.RequirementKindTask {
				id = "taskcat-001"
			}
			return []*model.Category{testutil.MakeCategory(func(c *model.Category) {
				c.ID = id
				c.EntityType = entityType
			})}, nil
		},
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
		if created.IsActive {
			t.Fatal("a requirement outside any sprint must not start active")
		}
	})

	t.Run("created in a sprint starts active", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				CreateFunc: func(_ context.Context, _ *model.Requirement) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)
		r, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{
			ProductID: "product-001", Title: "Planned", CategoryID: "cat-001", SprintID: "sprint-001",
		})
		if err != nil {
			t.Fatalf("CreateRequirement: %v", err)
		}
		if !r.IsActive {
			t.Fatal("IsActive should be true for an item created in a sprint")
		}
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

// --- Tasks ---

func TestCreateTask(t *testing.T) {
	t.Run("defaults to a task category", func(t *testing.T) {
		var created *model.Requirement
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				CreateFunc: func(_ context.Context, r *model.Requirement) error { created = r; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		_, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{
			ProductID: "product-001", Kind: model.RequirementKindTask, Title: "Upgrade Go toolchain",
		})
		if err != nil {
			t.Fatalf("CreateRequirement: %v", err)
		}
		if created.Kind != model.RequirementKindTask {
			t.Errorf("Kind = %q, want %q", created.Kind, model.RequirementKindTask)
		}
		if created.CategoryID != "taskcat-001" {
			t.Errorf("CategoryID = %q, want the seeded task category", created.CategoryID)
		}
	})

	t.Run("kind defaults to requirement", func(t *testing.T) {
		var created *model.Requirement
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				CreateFunc: func(_ context.Context, r *model.Requirement) error { created = r; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		if _, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{
			ProductID: "product-001", Title: "X", CategoryID: "cat-001",
		}); err != nil {
			t.Fatalf("CreateRequirement: %v", err)
		}
		if created.Kind != model.RequirementKindRequirement {
			t.Errorf("Kind = %q, want %q", created.Kind, model.RequirementKindRequirement)
		}
	})

	t.Run("rejects an unknown kind", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, ownerRoles())
		_, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{
			ProductID: "product-001", Kind: "epic", Title: "X",
		})
		if err == nil {
			t.Fatal("expected an error for kind \"epic\"")
		}
	})

	t.Run("rejects a category of the wrong kind", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				CreateFunc: func(_ context.Context, _ *model.Requirement) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		// "cat-001" resolves to a requirement category.
		_, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{
			ProductID: "product-001", Kind: model.RequirementKindTask, Title: "X", CategoryID: "cat-001",
		})
		if err == nil {
			t.Fatal("expected a task filed under a requirement category to be rejected")
		}
	})
}

func TestValidateParent(t *testing.T) {
	requirement := testutil.MakeRequirement()
	task := testutil.MakeTask()

	byID := func(items ...*model.Requirement) func(context.Context, string) (*model.Requirement, error) {
		return func(_ context.Context, id string) (*model.Requirement, error) {
			for _, item := range items {
				if item.ID == id {
					return item, nil
				}
			}
			return nil, errors.New("not found: " + id)
		}
	}

	t.Run("a requirement cannot hang off a task", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: byID(requirement, task),
				CreateFunc:  func(_ context.Context, _ *model.Requirement) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		_, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{
			ProductID: "product-001", Title: "X", CategoryID: "cat-001", ParentID: task.ID,
		})
		if err == nil {
			t.Fatal("expected a requirement parented to a task to be rejected")
		}
	})

	t.Run("a task can hang off a requirement", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: byID(requirement, task),
				CreateFunc:  func(_ context.Context, _ *model.Requirement) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		if _, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{
			ProductID: "product-001", Kind: model.RequirementKindTask, Title: "X",
			CategoryID: "taskcat-001", ParentID: requirement.ID,
		}); err != nil {
			t.Fatalf("expected a task parented to a requirement to be allowed, got %v", err)
		}
	})

	t.Run("rejects a parent in another product", func(t *testing.T) {
		foreign := testutil.MakeRequirement(func(r *model.Requirement) {
			r.ID = "req-foreign"
			r.ProductID = "product-999"
		})
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: byID(foreign),
				CreateFunc:  func(_ context.Context, _ *model.Requirement) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		_, err := svc.CreateRequirement(context.Background(), testutil.MakeUser(), CreateRequirementInput{
			ProductID: "product-001", Title: "X", CategoryID: "cat-001", ParentID: foreign.ID,
		})
		if err == nil {
			t.Fatal("expected a cross-product parent to be rejected")
		}
	})

	t.Run("rejects a cycle", func(t *testing.T) {
		// child -> parent -> grandparent, then try to reparent child's own
		// descendant chain back onto child.
		child := testutil.MakeRequirement(func(r *model.Requirement) { r.ID = "req-child" })
		descendant := testutil.MakeRequirement(func(r *model.Requirement) {
			r.ID = "req-descendant"
			r.ParentID = child.ID
		})
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: byID(child, descendant),
				UpdateFunc:  func(_ context.Context, _ *model.Requirement) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		parentID := descendant.ID
		_, err := svc.UpdateRequirement(context.Background(), testutil.MakeUser(), "product-001", child.ID,
			UpdateRequirementInput{ParentID: &parentID})
		if err == nil {
			t.Fatal("expected a cyclic parent link to be rejected")
		}
	})

	t.Run("rejects self-parenting", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: byID(requirement),
				UpdateFunc:  func(_ context.Context, _ *model.Requirement) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		parentID := requirement.ID
		_, err := svc.UpdateRequirement(context.Background(), testutil.MakeUser(), "product-001", requirement.ID,
			UpdateRequirementInput{ParentID: &parentID})
		if err == nil {
			t.Fatal("expected self-parenting to be rejected")
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
		req := testutil.MakeRequirement(func(r *model.Requirement) { r.IsActive = true })
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

	// FR-407: developers may change the status only of activated items.
	t.Run("developer status change requires activation", func(t *testing.T) {
		newStatus := "status-002"
		active := true
		cases := []struct {
			name    string
			req     *model.Requirement
			input   UpdateRequirementInput
			wantErr bool
		}{
			{"inactive item refused", testutil.MakeRequirement(), UpdateRequirementInput{StatusID: &newStatus}, true},
			{"activating in same request refused", testutil.MakeRequirement(), UpdateRequirementInput{StatusID: &newStatus, IsActive: &active}, true},
			// is_active is the only record of activation.
			{"sprint without is_active refused", testutil.MakeRequirement(func(r *model.Requirement) { r.SprintID = "sprint-001" }), UpdateRequirementInput{StatusID: &newStatus}, true},
			{"active item allowed", testutil.MakeRequirement(func(r *model.Requirement) { r.IsActive = true }), UpdateRequirementInput{StatusID: &newStatus}, false},
			{"activation alone allowed", testutil.MakeRequirement(), UpdateRequirementInput{IsActive: &active}, false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc := newBacklogService(
					&testutil.MockRequirementStore{
						GetByIDFunc: func(_ context.Context, _ string) (*model.Requirement, error) { return tc.req, nil },
						UpdateFunc:  func(_ context.Context, _ *model.Requirement) error { return nil },
					},
					nil, noopAudit(), developerRoles(),
				)
				_, err := svc.UpdateRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001", tc.input)
				if tc.wantErr && !errors.Is(err, ErrForbidden) {
					t.Fatalf("err = %v, want ErrForbidden", err)
				}
				if !tc.wantErr && err != nil {
					t.Fatalf("UpdateRequirement: %v", err)
				}
			})
		}
	})

	// FR-318: planning into a sprint activates, going back to the backlog deactivates.
	t.Run("sprint assignment and activation", func(t *testing.T) {
		sprint, other, none := "sprint-001", "sprint-002", ""
		inSprint := func(r *model.Requirement) { r.SprintID = sprint; r.IsActive = true }
		kanban := func(r *model.Requirement) { r.IsActive = true }
		cases := []struct {
			name       string
			req        *model.Requirement
			sprint     *string
			wantActive bool
		}{
			{"moving into a sprint activates", testutil.MakeRequirement(), &sprint, true},
			{"leaving a sprint deactivates", testutil.MakeRequirement(inSprint), &none, false},
			{"moving to another sprint stays active", testutil.MakeRequirement(inSprint), &other, true},
			{"unchanged empty sprint keeps kanban activation", testutil.MakeRequirement(kanban), &none, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc := newBacklogService(
					&testutil.MockRequirementStore{
						GetByIDFunc: func(_ context.Context, _ string) (*model.Requirement, error) { return tc.req, nil },
						UpdateFunc:  func(_ context.Context, _ *model.Requirement) error { return nil },
					},
					nil, noopAudit(), ownerRoles(),
				)
				got, err := svc.UpdateRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001",
					UpdateRequirementInput{SprintID: tc.sprint})
				if err != nil {
					t.Fatalf("UpdateRequirement: %v", err)
				}
				if got.IsActive != tc.wantActive {
					t.Fatalf("IsActive = %v, want %v", got.IsActive, tc.wantActive)
				}
			})
		}
	})

	t.Run("owner can change status of inactive item", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Requirement, error) { return testutil.MakeRequirement(), nil },
				UpdateFunc:  func(_ context.Context, _ *model.Requirement) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)
		newStatus := "status-002"
		if _, err := svc.UpdateRequirement(context.Background(), testutil.MakeUser(), "product-001", "req-001",
			UpdateRequirementInput{StatusID: &newStatus}); err != nil {
			t.Fatalf("UpdateRequirement: %v", err)
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

// --- Default status ---

func TestCreateAppliesDefaultStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("requirement without status gets the requirement default", func(t *testing.T) {
		var created *model.Requirement
		svc := newBacklogService(&testutil.MockRequirementStore{
			CreateFunc: func(_ context.Context, r *model.Requirement) error { created = r; return nil },
		}, nil, noopAudit(), ownerRoles())

		if _, err := svc.CreateRequirement(ctx, testutil.MakeUser(), CreateRequirementInput{ProductID: "product-001", Title: "X"}); err != nil {
			t.Fatalf("CreateRequirement: %v", err)
		}
		if created.StatusID != "requirement-new" {
			t.Errorf("StatusID = %q, want %q", created.StatusID, "requirement-new")
		}
	})

	t.Run("task without status gets the requirement default", func(t *testing.T) {
		var created *model.Requirement
		svc := newBacklogService(&testutil.MockRequirementStore{
			CreateFunc: func(_ context.Context, r *model.Requirement) error { created = r; return nil },
		}, nil, noopAudit(), ownerRoles())

		if _, err := svc.CreateRequirement(ctx, testutil.MakeUser(), CreateRequirementInput{ProductID: "product-001", Kind: model.RequirementKindTask, Title: "X"}); err != nil {
			t.Fatalf("CreateRequirement: %v", err)
		}
		if created.StatusID != "requirement-new" {
			t.Errorf("StatusID = %q, want %q", created.StatusID, "requirement-new")
		}
	})

	t.Run("defect without status gets the defect default", func(t *testing.T) {
		var created *model.Defect
		svc := newBacklogService(nil, &testutil.MockDefectStore{
			CreateFunc: func(_ context.Context, d *model.Defect) error { created = d; return nil },
		}, noopAudit(), testerRoles())

		if _, err := svc.CreateDefect(ctx, testutil.MakeUser(), CreateDefectInput{ProductID: "product-001", Title: "Bug"}); err != nil {
			t.Fatalf("CreateDefect: %v", err)
		}
		if created.StatusID != "defect-new" {
			t.Errorf("StatusID = %q, want %q", created.StatusID, "defect-new")
		}
	})

	t.Run("explicit status is kept", func(t *testing.T) {
		var req *model.Requirement
		var def *model.Defect
		svc := newBacklogService(
			&testutil.MockRequirementStore{CreateFunc: func(_ context.Context, r *model.Requirement) error { req = r; return nil }},
			&testutil.MockDefectStore{CreateFunc: func(_ context.Context, d *model.Defect) error { def = d; return nil }},
			noopAudit(), ownerRoles(),
		)
		svc.statuses = &testutil.MockStatusStore{} // must not be consulted

		if _, err := svc.CreateRequirement(ctx, testutil.MakeUser(), CreateRequirementInput{ProductID: "product-001", Title: "X", StatusID: "requirement-done"}); err != nil {
			t.Fatalf("CreateRequirement: %v", err)
		}
		if req.StatusID != "requirement-done" {
			t.Errorf("requirement StatusID = %q, want %q", req.StatusID, "requirement-done")
		}
		if _, err := svc.CreateDefect(ctx, testutil.MakeUser(), CreateDefectInput{ProductID: "product-001", Title: "Bug", StatusID: "defect-done"}); err != nil {
			t.Fatalf("CreateDefect: %v", err)
		}
		if def.StatusID != "defect-done" {
			t.Errorf("defect StatusID = %q, want %q", def.StatusID, "defect-done")
		}
	})

	t.Run("no default status leaves the status empty", func(t *testing.T) {
		var req *model.Requirement
		var def *model.Defect
		svc := newBacklogService(
			&testutil.MockRequirementStore{CreateFunc: func(_ context.Context, r *model.Requirement) error { req = r; return nil }},
			&testutil.MockDefectStore{CreateFunc: func(_ context.Context, d *model.Defect) error { def = d; return nil }},
			noopAudit(), ownerRoles(),
		)
		svc.statuses = &testutil.MockStatusStore{
			ListByProductIDFunc: func(_ context.Context, _, entityType string) ([]*model.Status, error) {
				return []*model.Status{{ID: entityType + "-open", EntityType: entityType, Name: "Open"}}, nil
			},
		}

		if _, err := svc.CreateRequirement(ctx, testutil.MakeUser(), CreateRequirementInput{ProductID: "product-001", Title: "X"}); err != nil {
			t.Fatalf("CreateRequirement: %v", err)
		}
		if req.StatusID != "" {
			t.Errorf("requirement StatusID = %q, want empty", req.StatusID)
		}
		if _, err := svc.CreateDefect(ctx, testutil.MakeUser(), CreateDefectInput{ProductID: "product-001", Title: "Bug"}); err != nil {
			t.Fatalf("CreateDefect: %v", err)
		}
		if def.StatusID != "" {
			t.Errorf("defect StatusID = %q, want empty", def.StatusID)
		}
	})

	t.Run("status lookup failure aborts creation", func(t *testing.T) {
		svc := newBacklogService(&testutil.MockRequirementStore{
			CreateFunc: func(_ context.Context, _ *model.Requirement) error {
				t.Fatal("requirement must not be created")
				return nil
			},
		}, &testutil.MockDefectStore{
			CreateFunc: func(_ context.Context, _ *model.Defect) error { t.Fatal("defect must not be created"); return nil },
		}, noopAudit(), ownerRoles())
		boom := errors.New("db down")
		svc.statuses = &testutil.MockStatusStore{
			ListByProductIDFunc: func(context.Context, string, string) ([]*model.Status, error) { return nil, boom },
		}

		if _, err := svc.CreateRequirement(ctx, testutil.MakeUser(), CreateRequirementInput{ProductID: "product-001", Title: "X"}); !errors.Is(err, boom) {
			t.Errorf("CreateRequirement err = %v, want %v", err, boom)
		}
		if _, err := svc.CreateDefect(ctx, testutil.MakeUser(), CreateDefectInput{ProductID: "product-001", Title: "Bug"}); !errors.Is(err, boom) {
			t.Errorf("CreateDefect err = %v, want %v", err, boom)
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
		if created.IsActive {
			t.Fatal("a defect outside any sprint must not start active")
		}
	})

	t.Run("created in a sprint starts active", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				CreateFunc: func(_ context.Context, _ *model.Defect) error { return nil },
			},
			noopAudit(), testerRoles(),
		)
		d, err := svc.CreateDefect(context.Background(), testutil.MakeUser(), CreateDefectInput{
			ProductID: "product-001", Title: "Bug", SprintID: "sprint-001",
		})
		if err != nil {
			t.Fatalf("CreateDefect: %v", err)
		}
		if !d.IsActive {
			t.Fatal("IsActive should be true for a defect created in a sprint")
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
	t.Run("taking a defect out of its sprint deactivates it", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Defect, error) {
					return testutil.MakeDefect(func(d *model.Defect) { d.SprintID = "sprint-001"; d.IsActive = true }), nil
				},
				UpdateFunc: func(_ context.Context, _ *model.Defect) error { return nil },
			},
			noopAudit(), ownerRoles(),
		)
		none := ""
		d, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001",
			UpdateDefectInput{SprintID: &none})
		if err != nil {
			t.Fatalf("UpdateDefect: %v", err)
		}
		if d.IsActive {
			t.Fatal("IsActive should be false after going back to the backlog")
		}
	})

	t.Run("moving a defect into a sprint activates it", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Defect, error) { return testutil.MakeDefect(), nil },
				UpdateFunc:  func(_ context.Context, _ *model.Defect) error { return nil },
			},
			noopAudit(), ownerRoles(),
		)
		sprint := "sprint-001"
		d, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001",
			UpdateDefectInput{SprintID: &sprint})
		if err != nil {
			t.Fatalf("UpdateDefect: %v", err)
		}
		if !d.IsActive {
			t.Fatal("IsActive should be true after planning into a sprint")
		}
	})

	t.Run("developer cannot change status of inactive defect", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Defect, error) { return testutil.MakeDefect(), nil },
				UpdateFunc:  func(_ context.Context, _ *model.Defect) error { return nil },
			},
			noopAudit(), developerRoles(),
		)
		newStatus := "status-002"
		_, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(), "product-001", "defect-001",
			UpdateDefectInput{StatusID: &newStatus})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("developer can update status and is_active", func(t *testing.T) {
		defect := testutil.MakeDefect(func(d *model.Defect) { d.IsActive = true })
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

	t.Run("rejects unknown target type", func(t *testing.T) {
		linked := false
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				AddLinkFunc: func(_ context.Context, _ *model.DefectLink) error { linked = true; return nil },
			},
			nil, testerRoles(),
		)

		err := svc.AddDefectLink(context.Background(), testutil.MakeUser(), "product-001", "defect-001", "sprint", "sprint-001")
		if err == nil {
			t.Fatal("expected an error for an unsupported target type")
		}
		if linked {
			t.Fatal("link was created despite an invalid target type")
		}
	})

	t.Run("rejects empty target id", func(t *testing.T) {
		svc := newBacklogService(nil, &testutil.MockDefectStore{}, nil, testerRoles())

		err := svc.AddDefectLink(context.Background(), testutil.MakeUser(), "product-001", "defect-001", model.DefectLinkTargetComponent, "")
		if err == nil {
			t.Fatal("expected an error for an empty target id")
		}
	})

	t.Run("accepts every supported target type", func(t *testing.T) {
		for _, targetType := range []string{
			model.DefectLinkTargetComponent,
			model.DefectLinkTargetCapability,
			model.DefectLinkTargetTestCase,
			model.DefectLinkTargetRequirement,
		} {
			svc := newBacklogService(nil,
				&testutil.MockDefectStore{
					AddLinkFunc: func(_ context.Context, _ *model.DefectLink) error { return nil },
				},
				nil, testerRoles(),
			)

			if err := svc.AddDefectLink(context.Background(), testutil.MakeUser(), "product-001", "defect-001", targetType, "target-001"); err != nil {
				t.Fatalf("AddDefectLink(%q): %v", targetType, err)
			}
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

func TestListDefectLinkTargets(t *testing.T) {
	allLinks := func(_ context.Context, _ string) ([]*model.DefectLink, error) {
		return []*model.DefectLink{
			{ID: "link-001", TargetType: model.DefectLinkTargetComponent, TargetID: "comp-001"},
			{ID: "link-002", TargetType: model.DefectLinkTargetCapability, TargetID: "cap-001"},
			{ID: "link-003", TargetType: model.DefectLinkTargetComponent, TargetID: "comp-002"},
			{ID: "link-004", TargetType: model.DefectLinkTargetRequirement, TargetID: "req-001"},
		}, nil
	}

	t.Run("filters by target type and maps link IDs", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{ListLinksFunc: allLinks},
			nil, viewerRoles(),
		)

		targetIDs, linkIDs, err := svc.ListDefectLinkTargets(context.Background(), testutil.MakeUser(), "product-001", "defect-001", model.DefectLinkTargetComponent)
		if err != nil {
			t.Fatalf("ListDefectLinkTargets: %v", err)
		}
		if len(targetIDs) != 2 {
			t.Fatalf("len(targetIDs) = %d, want 2", len(targetIDs))
		}
		if targetIDs[0] != "comp-001" || targetIDs[1] != "comp-002" {
			t.Fatalf("targetIDs = %v, want [comp-001 comp-002]", targetIDs)
		}
		if linkIDs["comp-001"] != "link-001" || linkIDs["comp-002"] != "link-003" {
			t.Fatalf("linkIDs = %v, want comp-001→link-001, comp-002→link-003", linkIDs)
		}
	})

	t.Run("returns empty for a type with no links", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{ListLinksFunc: allLinks},
			nil, viewerRoles(),
		)

		targetIDs, _, err := svc.ListDefectLinkTargets(context.Background(), testutil.MakeUser(), "product-001", "defect-001", model.DefectLinkTargetTestCase)
		if err != nil {
			t.Fatalf("ListDefectLinkTargets: %v", err)
		}
		if len(targetIDs) != 0 {
			t.Fatalf("len(targetIDs) = %d, want 0", len(targetIDs))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, _, err := svc.ListDefectLinkTargets(context.Background(), testutil.MakeUser(), "product-001", "defect-001", model.DefectLinkTargetComponent)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListRequirementsByMilestone(t *testing.T) {
	t.Run("skips requirements from another product", func(t *testing.T) {
		svc := newBacklogService(
			&testutil.MockRequirementStore{
				ListByMilestoneIDFunc: func(_ context.Context, milestoneID string) ([]*model.Requirement, error) {
					if milestoneID != "milestone-001" {
						t.Fatalf("milestoneID = %q, want milestone-001", milestoneID)
					}
					return []*model.Requirement{
						{ID: "req-001", ProductID: "product-001", Title: "A"},
						{ID: "req-002", ProductID: "other-product", Title: "B"},
					}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListRequirementsByMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if err != nil {
			t.Fatalf("ListRequirementsByMilestone: %v", err)
		}
		if len(result) != 1 || result[0].ID != "req-001" {
			t.Fatalf("result = %v, want just req-001", result)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListRequirementsByMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListDefectsByMilestone(t *testing.T) {
	t.Run("skips defects from another product", func(t *testing.T) {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				ListByMilestoneIDFunc: func(_ context.Context, _ string) ([]*model.Defect, error) {
					return []*model.Defect{
						{ID: "defect-001", ProductID: "product-001", Title: "A"},
						{ID: "defect-002", ProductID: "other-product", Title: "B"},
					}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListDefectsByMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if err != nil {
			t.Fatalf("ListDefectsByMilestone: %v", err)
		}
		if len(result) != 1 || result[0].ID != "defect-001" {
			t.Fatalf("result = %v, want just defect-001", result)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newBacklogService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListDefectsByMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Defect close hook (FR-504) ---

// retestRecorder captures calls from the backlog service into the testing service.
type retestRecorder struct {
	defectIDs []string
	err       error
}

func (r *retestRecorder) MarkResultsForRetest(_ context.Context, defectID, _ string) error {
	if r.err != nil {
		return r.err
	}
	r.defectIDs = append(r.defectIDs, defectID)
	return nil
}

func TestUpdateDefectReopensCampaignResults(t *testing.T) {
	// statuses maps a status ID to whether that status is closed.
	statusStore := func(closed map[string]bool) *testutil.MockStatusStore {
		return &testutil.MockStatusStore{
			GetByIDFunc: func(_ context.Context, id string) (*model.Status, error) {
				return testutil.MakeStatus(func(s *model.Status) {
					s.ID = id
					s.EntityType = "defect"
					s.IsClosed = closed[id]
				}), nil
			},
		}
	}

	newSvc := func(from string, closed map[string]bool, recorder *retestRecorder) *BacklogService {
		svc := newBacklogService(nil,
			&testutil.MockDefectStore{
				GetByIDFunc: func(_ context.Context, id string) (*model.Defect, error) {
					return testutil.MakeDefect(func(d *model.Defect) {
						d.ID = id
						d.StatusID = from
					}), nil
				},
				UpdateFunc: func(_ context.Context, _ *model.Defect) error { return nil },
			},
			noopAudit(), ownerRoles(),
		)
		svc.statuses = statusStore(closed)
		svc.retester = recorder
		return svc
	}

	closedStatuses := map[string]bool{"status-open": false, "status-closed": true}

	t.Run("closing a defect flags its results for retest", func(t *testing.T) {
		recorder := &retestRecorder{}
		svc := newSvc("status-open", closedStatuses, recorder)

		toClosed := "status-closed"
		if _, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(),
			"product-001", "defect-001", UpdateDefectInput{StatusID: &toClosed}); err != nil {
			t.Fatalf("UpdateDefect: %v", err)
		}
		if len(recorder.defectIDs) != 1 || recorder.defectIDs[0] != "defect-001" {
			t.Fatalf("retest calls = %v, want [defect-001]", recorder.defectIDs)
		}
	})

	t.Run("a non-closing status change does not", func(t *testing.T) {
		recorder := &retestRecorder{}
		svc := newSvc("status-closed", closedStatuses, recorder)

		toOpen := "status-open"
		if _, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(),
			"product-001", "defect-001", UpdateDefectInput{StatusID: &toOpen}); err != nil {
			t.Fatalf("UpdateDefect: %v", err)
		}
		if len(recorder.defectIDs) != 0 {
			t.Fatalf("retest calls = %v, want none", recorder.defectIDs)
		}
	})

	t.Run("staying closed does not re-trigger", func(t *testing.T) {
		recorder := &retestRecorder{}
		svc := newSvc("status-closed", closedStatuses, recorder)

		stillClosed := "status-closed"
		if _, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(),
			"product-001", "defect-001", UpdateDefectInput{StatusID: &stillClosed}); err != nil {
			t.Fatalf("UpdateDefect: %v", err)
		}
		if len(recorder.defectIDs) != 0 {
			t.Fatalf("retest calls = %v, want none", recorder.defectIDs)
		}
	})

	t.Run("a retest failure fails the update", func(t *testing.T) {
		recorder := &retestRecorder{err: errors.New("boom")}
		svc := newSvc("status-open", closedStatuses, recorder)

		toClosed := "status-closed"
		if _, err := svc.UpdateDefect(context.Background(), testutil.MakeUser(),
			"product-001", "defect-001", UpdateDefectInput{StatusID: &toClosed}); err == nil {
			t.Fatal("expected the retest error to surface")
		}
	})
}
