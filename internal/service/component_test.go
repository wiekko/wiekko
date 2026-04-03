package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newComponentService(components *testutil.MockComponentStore, functionalities *testutil.MockFunctionalityStore, audit *testutil.MockAuditStore, roles *testutil.MockProductRoleStore) *ComponentService {
	return &ComponentService{
		components:      components,
		functionalities: functionalities,
		audit:           audit,
		authz:           &Authorizer{productRoles: roles},
	}
}

// --- Components ---

func TestCreateComponent(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Component
		svc := newComponentService(
			&testutil.MockComponentStore{
				CreateFunc: func(_ context.Context, c *model.Component) error { created = c; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateComponent(context.Background(), testutil.MakeUser(), CreateComponentInput{
			ProductID: "product-001", Name: "Backend",
		})
		if err != nil {
			t.Fatalf("CreateComponent: %v", err)
		}
		if result.Name != "Backend" {
			t.Fatalf("Name = %q, want %q", result.Name, "Backend")
		}
		if created == nil {
			t.Fatal("component was not created")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, ownerRoles())
		_, err := svc.CreateComponent(context.Background(), testutil.MakeUser(), CreateComponentInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateComponent(context.Background(), testutil.MakeUser(), CreateComponentInput{ProductID: "product-001", Name: "X"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetComponent(t *testing.T) {
	component := testutil.MakeComponent()

	t.Run("success", func(t *testing.T) {
		svc := newComponentService(
			&testutil.MockComponentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Component, error) { return component, nil },
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.GetComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001")
		if err != nil {
			t.Fatalf("GetComponent: %v", err)
		}
		if result.ID != component.ID {
			t.Fatalf("ID = %q, want %q", result.ID, component.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.GetComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateComponent(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		component := testutil.MakeComponent()
		var updated *model.Component
		svc := newComponentService(
			&testutil.MockComponentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Component, error) { return component, nil },
				UpdateFunc:  func(_ context.Context, c *model.Component) error { updated = c; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		newName := "Updated"
		result, err := svc.UpdateComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001", UpdateComponentInput{Name: &newName})
		if err != nil {
			t.Fatalf("UpdateComponent: %v", err)
		}
		if result.Name != "Updated" {
			t.Fatalf("Name = %q, want %q", result.Name, "Updated")
		}
		if updated == nil {
			t.Fatal("component was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdateComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001", UpdateComponentInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteComponent(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newComponentService(
			&testutil.MockComponentStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		err := svc.DeleteComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001")
		if err != nil {
			t.Fatalf("DeleteComponent: %v", err)
		}
		if !deleted {
			t.Fatal("component was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		err := svc.DeleteComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListComponents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newComponentService(
			&testutil.MockComponentStore{
				ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.Component, error) {
					return []*model.Component{testutil.MakeComponent()}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListComponents(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListComponents: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListComponents(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Functionalities ---

func TestCreateFunctionality(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Functionality
		svc := newComponentService(nil,
			&testutil.MockFunctionalityStore{
				CreateFunc: func(_ context.Context, f *model.Functionality) error { created = f; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateFunctionality(context.Background(), testutil.MakeUser(), CreateFunctionalityInput{
			ProductID: "product-001", Name: "Login",
		})
		if err != nil {
			t.Fatalf("CreateFunctionality: %v", err)
		}
		if result.Name != "Login" {
			t.Fatalf("Name = %q, want %q", result.Name, "Login")
		}
		if created == nil {
			t.Fatal("functionality was not created")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, ownerRoles())
		_, err := svc.CreateFunctionality(context.Background(), testutil.MakeUser(), CreateFunctionalityInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateFunctionality(context.Background(), testutil.MakeUser(), CreateFunctionalityInput{ProductID: "product-001", Name: "X"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetFunctionality(t *testing.T) {
	functionality := testutil.MakeFunctionality()

	t.Run("success", func(t *testing.T) {
		svc := newComponentService(nil,
			&testutil.MockFunctionalityStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Functionality, error) { return functionality, nil },
			},
			nil, viewerRoles(),
		)

		result, err := svc.GetFunctionality(context.Background(), testutil.MakeUser(), "product-001", "functionality-001")
		if err != nil {
			t.Fatalf("GetFunctionality: %v", err)
		}
		if result.ID != functionality.ID {
			t.Fatalf("ID = %q, want %q", result.ID, functionality.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.GetFunctionality(context.Background(), testutil.MakeUser(), "product-001", "functionality-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateFunctionality(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		functionality := testutil.MakeFunctionality()
		var updated *model.Functionality
		svc := newComponentService(nil,
			&testutil.MockFunctionalityStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Functionality, error) { return functionality, nil },
				UpdateFunc:  func(_ context.Context, f *model.Functionality) error { updated = f; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		newName := "Updated"
		result, err := svc.UpdateFunctionality(context.Background(), testutil.MakeUser(), "product-001", "functionality-001", UpdateFunctionalityInput{Name: &newName})
		if err != nil {
			t.Fatalf("UpdateFunctionality: %v", err)
		}
		if result.Name != "Updated" {
			t.Fatalf("Name = %q, want %q", result.Name, "Updated")
		}
		if updated == nil {
			t.Fatal("functionality was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdateFunctionality(context.Background(), testutil.MakeUser(), "product-001", "functionality-001", UpdateFunctionalityInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteFunctionality(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newComponentService(nil,
			&testutil.MockFunctionalityStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		err := svc.DeleteFunctionality(context.Background(), testutil.MakeUser(), "product-001", "functionality-001")
		if err != nil {
			t.Fatalf("DeleteFunctionality: %v", err)
		}
		if !deleted {
			t.Fatal("functionality was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		err := svc.DeleteFunctionality(context.Background(), testutil.MakeUser(), "product-001", "functionality-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListFunctionalities(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newComponentService(nil,
			&testutil.MockFunctionalityStore{
				ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.Functionality, error) {
					return []*model.Functionality{testutil.MakeFunctionality()}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListFunctionalities(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListFunctionalities: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListFunctionalities(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListFunctionalitiesByComponent(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newComponentService(nil,
			&testutil.MockFunctionalityStore{
				ListByComponentIDFunc: func(_ context.Context, _ string) ([]*model.Functionality, error) {
					return []*model.Functionality{testutil.MakeFunctionality()}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListFunctionalitiesByComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001")
		if err != nil {
			t.Fatalf("ListFunctionalitiesByComponent: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListFunctionalitiesByComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
