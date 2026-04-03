package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newProductService(products *testutil.MockProductStore, roles *testutil.MockProductRoleStore, settings *testutil.MockSettingStore, audit *testutil.MockAuditStore) *ProductService {
	return &ProductService{
		products:     products,
		productRoles: roles,
		settings:     settings,
		audit:        audit,
		authz:        &Authorizer{productRoles: roles},
	}
}

func noopAudit() *testutil.MockAuditStore {
	return &testutil.MockAuditStore{
		CreateFunc: func(_ context.Context, _ *model.AuditEntry) error { return nil },
	}
}

func ownerRoles() *testutil.MockProductRoleStore {
	return &testutil.MockProductRoleStore{
		GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
			return testutil.MakeProductRole(), nil
		},
		CreateFunc: func(_ context.Context, _ *model.ProductRole) error { return nil },
	}
}

func viewerRoles() *testutil.MockProductRoleStore {
	return &testutil.MockProductRoleStore{
		GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
			return testutil.MakeProductRole(func(pr *model.ProductRole) { pr.Role = model.RoleViewer }), nil
		},
	}
}

func forbiddenRoles() *testutil.MockProductRoleStore {
	return &testutil.MockProductRoleStore{
		GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
			return nil, errors.New("not found")
		},
	}
}

func TestProductCreate(t *testing.T) {
	t.Run("admin bypasses setting check", func(t *testing.T) {
		var created *model.Product
		var roleCreated *model.ProductRole
		roles := &testutil.MockProductRoleStore{
			CreateFunc: func(_ context.Context, r *model.ProductRole) error {
				roleCreated = r
				return nil
			},
		}
		svc := newProductService(
			&testutil.MockProductStore{
				CreateFunc: func(_ context.Context, p *model.Product) error { created = p; return nil },
			},
			roles, nil, noopAudit(),
		)

		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		result, err := svc.Create(context.Background(), admin, CreateProductInput{Name: "New Product"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if result.Name != "New Product" {
			t.Fatalf("Name = %q, want %q", result.Name, "New Product")
		}
		if created == nil {
			t.Fatal("product was not created")
		}
		if roleCreated == nil {
			t.Fatal("owner role was not created")
		}
		if roleCreated.Role != model.RoleOwner {
			t.Fatalf("role = %q, want %q", roleCreated.Role, model.RoleOwner)
		}
	})

	t.Run("non-admin with creation enabled", func(t *testing.T) {
		roles := &testutil.MockProductRoleStore{
			CreateFunc: func(_ context.Context, _ *model.ProductRole) error { return nil },
		}
		svc := newProductService(
			&testutil.MockProductStore{
				CreateFunc: func(_ context.Context, _ *model.Product) error { return nil },
			},
			roles,
			&testutil.MockSettingStore{
				GetFunc: func(_ context.Context, key string) (string, error) {
					if key == "product_creation_enabled" {
						return "true", nil
					}
					return "", errors.New("not found")
				},
			},
			noopAudit(),
		)

		result, err := svc.Create(context.Background(), testutil.MakeUser(), CreateProductInput{Name: "New Product"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if result.Name != "New Product" {
			t.Fatalf("Name = %q, want %q", result.Name, "New Product")
		}
	})

	t.Run("non-admin with creation disabled", func(t *testing.T) {
		svc := newProductService(nil, nil,
			&testutil.MockSettingStore{
				GetFunc: func(_ context.Context, _ string) (string, error) {
					return "false", nil
				},
			},
			nil,
		)

		_, err := svc.Create(context.Background(), testutil.MakeUser(), CreateProductInput{Name: "New Product"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newProductService(nil, nil, nil, nil)
		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		_, err := svc.Create(context.Background(), admin, CreateProductInput{})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("defaults estimation and workflow mode", func(t *testing.T) {
		var created *model.Product
		roles := &testutil.MockProductRoleStore{
			CreateFunc: func(_ context.Context, _ *model.ProductRole) error { return nil },
		}
		svc := newProductService(
			&testutil.MockProductStore{
				CreateFunc: func(_ context.Context, p *model.Product) error { created = p; return nil },
			},
			roles, nil, noopAudit(),
		)

		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		_, err := svc.Create(context.Background(), admin, CreateProductInput{Name: "P"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if created.EstimationMode != model.EstimationModeDisabled {
			t.Fatalf("EstimationMode = %q, want %q", created.EstimationMode, model.EstimationModeDisabled)
		}
		if created.WorkflowMode != model.WorkflowModeKanban {
			t.Fatalf("WorkflowMode = %q, want %q", created.WorkflowMode, model.WorkflowModeKanban)
		}
	})
}

func TestProductGetByID(t *testing.T) {
	product := testutil.MakeProduct()

	t.Run("success", func(t *testing.T) {
		svc := newProductService(
			&testutil.MockProductStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Product, error) { return product, nil },
			},
			viewerRoles(), nil, nil,
		)

		result, err := svc.GetByID(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if result.ID != product.ID {
			t.Fatalf("ID = %q, want %q", result.ID, product.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newProductService(nil, forbiddenRoles(), nil, nil)
		_, err := svc.GetByID(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestProductUpdate(t *testing.T) {
	t.Run("success with partial update", func(t *testing.T) {
		product := testutil.MakeProduct()
		var updated *model.Product
		svc := newProductService(
			&testutil.MockProductStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Product, error) { return product, nil },
				UpdateFunc:  func(_ context.Context, p *model.Product) error { updated = p; return nil },
			},
			ownerRoles(), nil, noopAudit(),
		)

		newName := "Updated"
		result, err := svc.Update(context.Background(), testutil.MakeUser(), "product-001", UpdateProductInput{Name: &newName})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if result.Name != "Updated" {
			t.Fatalf("Name = %q, want %q", result.Name, "Updated")
		}
		if updated == nil {
			t.Fatal("product was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newProductService(nil, forbiddenRoles(), nil, nil)
		_, err := svc.Update(context.Background(), testutil.MakeUser(), "product-001", UpdateProductInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestProductDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newProductService(
			&testutil.MockProductStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			ownerRoles(), nil, noopAudit(),
		)

		err := svc.Delete(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if !deleted {
			t.Fatal("product was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newProductService(nil, forbiddenRoles(), nil, nil)
		err := svc.Delete(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestProductList(t *testing.T) {
	products := []*model.Product{testutil.MakeProduct()}

	t.Run("admin sees all", func(t *testing.T) {
		svc := newProductService(
			&testutil.MockProductStore{
				ListFunc: func(_ context.Context) ([]*model.Product, error) { return products, nil },
			},
			nil, nil, nil,
		)

		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		result, err := svc.List(context.Background(), admin)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("non-admin sees only own products", func(t *testing.T) {
		svc := newProductService(
			&testutil.MockProductStore{
				GetByIDFunc: func(_ context.Context, id string) (*model.Product, error) {
					return testutil.MakeProduct(func(p *model.Product) { p.ID = id }), nil
				},
			},
			&testutil.MockProductRoleStore{
				ListByUserIDFunc: func(_ context.Context, _ string) ([]*model.ProductRole, error) {
					return []*model.ProductRole{testutil.MakeProductRole()}, nil
				},
			},
			nil, nil,
		)

		result, err := svc.List(context.Background(), testutil.MakeUser())
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})
}

func TestProductAddMember(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.ProductRole
		roles := &testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
				return testutil.MakeProductRole(), nil
			},
			CreateFunc: func(_ context.Context, r *model.ProductRole) error { created = r; return nil },
		}
		svc := newProductService(nil, roles, nil, nil)

		err := svc.AddMember(context.Background(), testutil.MakeUser(), "product-001", "user-002", model.RoleDeveloper)
		if err != nil {
			t.Fatalf("AddMember: %v", err)
		}
		if created == nil {
			t.Fatal("role was not created")
		}
		if created.Role != model.RoleDeveloper {
			t.Fatalf("role = %q, want %q", created.Role, model.RoleDeveloper)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newProductService(nil, forbiddenRoles(), nil, nil)
		err := svc.AddMember(context.Background(), testutil.MakeUser(), "product-001", "user-002", model.RoleDeveloper)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestProductUpdateMember(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var updated *model.ProductRole
		roles := &testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
				return testutil.MakeProductRole(), nil
			},
			UpdateFunc: func(_ context.Context, r *model.ProductRole) error { updated = r; return nil },
		}
		svc := newProductService(nil, roles, nil, nil)

		err := svc.UpdateMember(context.Background(), testutil.MakeUser(), "product-001", "user-002", model.RoleTester)
		if err != nil {
			t.Fatalf("UpdateMember: %v", err)
		}
		if updated == nil {
			t.Fatal("role was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newProductService(nil, forbiddenRoles(), nil, nil)
		err := svc.UpdateMember(context.Background(), testutil.MakeUser(), "product-001", "user-002", model.RoleTester)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestProductRemoveMember(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		roles := &testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
				return testutil.MakeProductRole(), nil
			},
			DeleteFunc: func(_ context.Context, _, _ string) error { deleted = true; return nil },
		}
		svc := newProductService(nil, roles, nil, nil)

		err := svc.RemoveMember(context.Background(), testutil.MakeUser(), "product-001", "user-002")
		if err != nil {
			t.Fatalf("RemoveMember: %v", err)
		}
		if !deleted {
			t.Fatal("role was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newProductService(nil, forbiddenRoles(), nil, nil)
		err := svc.RemoveMember(context.Background(), testutil.MakeUser(), "product-001", "user-002")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestProductListMembers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		roles := &testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
				return testutil.MakeProductRole(func(pr *model.ProductRole) { pr.Role = model.RoleViewer }), nil
			},
			ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.ProductRole, error) {
				return []*model.ProductRole{testutil.MakeProductRole()}, nil
			},
		}
		svc := newProductService(nil, roles, nil, nil)

		result, err := svc.ListMembers(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListMembers: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newProductService(nil, forbiddenRoles(), nil, nil)
		_, err := svc.ListMembers(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestProductTransferOwnership(t *testing.T) {
	t.Run("new owner has existing role", func(t *testing.T) {
		updates := []*model.ProductRole{}
		roles := &testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, userID, _ string) (*model.ProductRole, error) {
				if userID == "user-002" {
					return testutil.MakeProductRole(func(pr *model.ProductRole) {
						pr.UserID = "user-002"
						pr.Role = model.RoleDeveloper
					}), nil
				}
				return testutil.MakeProductRole(), nil
			},
			UpdateFunc: func(_ context.Context, r *model.ProductRole) error {
				updates = append(updates, r)
				return nil
			},
		}
		svc := newProductService(nil, roles, nil, nil)

		err := svc.TransferOwnership(context.Background(), testutil.MakeUser(), "product-001", "user-002")
		if err != nil {
			t.Fatalf("TransferOwnership: %v", err)
		}
		if len(updates) != 2 {
			t.Fatalf("len(updates) = %d, want 2", len(updates))
		}
		if updates[0].Role != model.RoleDeveloper {
			t.Fatalf("old owner role = %q, want %q", updates[0].Role, model.RoleDeveloper)
		}
		if updates[1].Role != model.RoleOwner {
			t.Fatalf("new owner role = %q, want %q", updates[1].Role, model.RoleOwner)
		}
	})

	t.Run("new owner has no existing role", func(t *testing.T) {
		var createdRole *model.ProductRole
		roles := &testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, userID, _ string) (*model.ProductRole, error) {
				if userID == "user-002" {
					return nil, errors.New("not found")
				}
				return testutil.MakeProductRole(), nil
			},
			UpdateFunc: func(_ context.Context, _ *model.ProductRole) error { return nil },
			CreateFunc: func(_ context.Context, r *model.ProductRole) error { createdRole = r; return nil },
		}
		svc := newProductService(nil, roles, nil, nil)

		err := svc.TransferOwnership(context.Background(), testutil.MakeUser(), "product-001", "user-002")
		if err != nil {
			t.Fatalf("TransferOwnership: %v", err)
		}
		if createdRole == nil {
			t.Fatal("role was not created for new owner")
		}
		if createdRole.Role != model.RoleOwner {
			t.Fatalf("role = %q, want %q", createdRole.Role, model.RoleOwner)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newProductService(nil, forbiddenRoles(), nil, nil)
		err := svc.TransferOwnership(context.Background(), testutil.MakeUser(), "product-001", "user-002")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
