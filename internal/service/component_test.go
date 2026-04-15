package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newComponentService(components *testutil.MockComponentStore, capabilities *testutil.MockCapabilityStore, audit *testutil.MockAuditStore, roles *testutil.MockProductRoleStore) *ComponentService {
	return &ComponentService{
		components:   components,
		capabilities: capabilities,
		audit:        audit,
		authz:        &Authorizer{productRoles: roles},
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

// --- Capabilities ---

func TestCreateCapability(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Capability
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				CreateFunc: func(_ context.Context, c *model.Capability) error { created = c; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateCapability(context.Background(), testutil.MakeUser(), CreateCapabilityInput{
			ProductID: "product-001", Name: "Login",
		})
		if err != nil {
			t.Fatalf("CreateCapability: %v", err)
		}
		if result.Name != "Login" {
			t.Fatalf("Name = %q, want %q", result.Name, "Login")
		}
		if created == nil {
			t.Fatal("capability was not created")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, ownerRoles())
		_, err := svc.CreateCapability(context.Background(), testutil.MakeUser(), CreateCapabilityInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateCapability(context.Background(), testutil.MakeUser(), CreateCapabilityInput{ProductID: "product-001", Name: "X"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetCapability(t *testing.T) {
	capability := testutil.MakeCapability()

	t.Run("success", func(t *testing.T) {
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Capability, error) { return capability, nil },
			},
			nil, viewerRoles(),
		)

		result, err := svc.GetCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001")
		if err != nil {
			t.Fatalf("GetCapability: %v", err)
		}
		if result.ID != capability.ID {
			t.Fatalf("ID = %q, want %q", result.ID, capability.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.GetCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateCapability(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		capability := testutil.MakeCapability()
		var updated *model.Capability
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Capability, error) { return capability, nil },
				UpdateFunc:  func(_ context.Context, c *model.Capability) error { updated = c; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		newName := "Updated"
		result, err := svc.UpdateCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001", UpdateCapabilityInput{Name: &newName})
		if err != nil {
			t.Fatalf("UpdateCapability: %v", err)
		}
		if result.Name != "Updated" {
			t.Fatalf("Name = %q, want %q", result.Name, "Updated")
		}
		if updated == nil {
			t.Fatal("capability was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdateCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001", UpdateCapabilityInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteCapability(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		err := svc.DeleteCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001")
		if err != nil {
			t.Fatalf("DeleteCapability: %v", err)
		}
		if !deleted {
			t.Fatal("capability was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		err := svc.DeleteCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListCapabilities(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.Capability, error) {
					return []*model.Capability{testutil.MakeCapability()}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListCapabilities(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListCapabilities: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListCapabilities(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListCapabilitiesByComponent(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				ListByComponentIDFunc: func(_ context.Context, _ string) ([]*model.Capability, error) {
					return []*model.Capability{testutil.MakeCapability()}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListCapabilitiesByComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001")
		if err != nil {
			t.Fatalf("ListCapabilitiesByComponent: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListCapabilitiesByComponent(context.Background(), testutil.MakeUser(), "product-001", "component-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Requirement-Capability Links ---

func TestLinkRequirementToCapability(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		linked := false
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				LinkRequirementFunc: func(_ context.Context, _ *model.RequirementCapability) error { linked = true; return nil },
			},
			nil, ownerRoles(),
		)

		err := svc.LinkRequirementToCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001", "req-001")
		if err != nil {
			t.Fatalf("LinkRequirementToCapability: %v", err)
		}
		if !linked {
			t.Fatal("requirement was not linked")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		err := svc.LinkRequirementToCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUnlinkRequirementFromCapability(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		unlinked := false
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				UnlinkRequirementFunc: func(_ context.Context, _, _ string) error { unlinked = true; return nil },
			},
			nil, ownerRoles(),
		)

		err := svc.UnlinkRequirementFromCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001", "req-001")
		if err != nil {
			t.Fatalf("UnlinkRequirementFromCapability: %v", err)
		}
		if !unlinked {
			t.Fatal("requirement was not unlinked")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		err := svc.UnlinkRequirementFromCapability(context.Background(), testutil.MakeUser(), "product-001", "capability-001", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListCapabilityRequirements(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				ListRequirementsByCapabilityFunc: func(_ context.Context, _ string) ([]*model.RequirementCapability, error) {
					return []*model.RequirementCapability{testutil.MakeRequirementCapability()}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListCapabilityRequirements(context.Background(), testutil.MakeUser(), "product-001", "capability-001")
		if err != nil {
			t.Fatalf("ListCapabilityRequirements: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListCapabilityRequirements(context.Background(), testutil.MakeUser(), "product-001", "capability-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListRequirementCapabilities(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newComponentService(nil,
			&testutil.MockCapabilityStore{
				ListCapabilitiesByRequirementFunc: func(_ context.Context, _ string) ([]*model.RequirementCapability, error) {
					return []*model.RequirementCapability{testutil.MakeRequirementCapability()}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListRequirementCapabilities(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if err != nil {
			t.Fatalf("ListRequirementCapabilities: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newComponentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListRequirementCapabilities(context.Background(), testutil.MakeUser(), "product-001", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
