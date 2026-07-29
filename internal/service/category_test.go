package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newCategoryService(
	categories *testutil.MockCategoryStore,
	requirements *testutil.MockRequirementStore,
	constraints *testutil.MockConstraintStore,
	roles *testutil.MockProductRoleStore,
) *CategoryService {
	if requirements == nil {
		requirements = &testutil.MockRequirementStore{}
	}
	if constraints == nil {
		constraints = &testutil.MockConstraintStore{}
	}
	return &CategoryService{
		categories:   categories,
		requirements: requirements,
		constraints:  constraints,
		audit:        noopAudit(),
		authz:        &Authorizer{productRoles: roles},
	}
}

// categoriesNamed builds a category store whose ListByProductID returns the given
// categories for any entity type, and whose lookups resolve against them.
func categoriesNamed(cats ...*model.Category) *testutil.MockCategoryStore {
	return &testutil.MockCategoryStore{
		CreateFunc: func(_ context.Context, _ *model.Category) error { return nil },
		UpdateFunc: func(_ context.Context, _ *model.Category) error { return nil },
		DeleteFunc: func(_ context.Context, _ string) error { return nil },
		GetByIDFunc: func(_ context.Context, id string) (*model.Category, error) {
			for _, c := range cats {
				if c.ID == id {
					return c, nil
				}
			}
			return nil, errors.New("category not found: " + id)
		},
		ListByProductIDFunc: func(_ context.Context, _, entityType string) ([]*model.Category, error) {
			var out []*model.Category
			for _, c := range cats {
				if c.EntityType == entityType {
					out = append(out, c)
				}
			}
			return out, nil
		},
		GetByProductAndNameFunc: func(_ context.Context, _, entityType, name string) (*model.Category, error) {
			for _, c := range cats {
				if c.EntityType == entityType && c.Name == name {
					return c, nil
				}
			}
			return nil, errors.New("category not found: " + name)
		},
		GetByProductAndPrefixFunc: func(_ context.Context, _, prefix string) (*model.Category, error) {
			for _, c := range cats {
				if c.Prefix == prefix {
					return c, nil
				}
			}
			return nil, errors.New("category not found: " + prefix)
		},
	}
}

func requirementCategory() *model.Category {
	return testutil.MakeCategory()
}

func constraintCategory() *model.Category {
	return testutil.MakeCategory(func(c *model.Category) {
		c.ID = "cat-con-001"
		c.EntityType = "constraint"
		c.Name = "Architecture"
		c.Prefix = "AC"
	})
}

func TestCreateCategoryPrefixValidation(t *testing.T) {
	t.Run("rejects malformed prefixes", func(t *testing.T) {
		for _, prefix := range []string{"F-R", "1X", "TOOLONG", "F R", "!"} {
			svc := newCategoryService(categoriesNamed(), nil, nil, ownerRoles())
			_, err := svc.CreateCategory(context.Background(), testutil.MakeUser(), CreateCategoryInput{
				ProductID: "product-001", EntityType: model.RequirementKindRequirement,
				Name: "Whatever", Prefix: prefix,
			})
			if err == nil {
				t.Fatalf("prefix %q was accepted, want rejection", prefix)
			}
		}
	})

	t.Run("upper-cases and trims", func(t *testing.T) {
		var created *model.Category
		store := categoriesNamed()
		store.CreateFunc = func(_ context.Context, c *model.Category) error { created = c; return nil }
		svc := newCategoryService(store, nil, nil, ownerRoles())

		_, err := svc.CreateCategory(context.Background(), testutil.MakeUser(), CreateCategoryInput{
			ProductID: "product-001", EntityType: model.RequirementKindRequirement,
			Name: "Performance", Prefix: " pf ",
		})
		if err != nil {
			t.Fatalf("CreateCategory: %v", err)
		}
		if created.Prefix != "PF" {
			t.Fatalf("Prefix = %q, want %q", created.Prefix, "PF")
		}
	})
}

func TestCreateCategoryUniqueness(t *testing.T) {
	// Constraints keep their own sequence counter, so a constraint category
	// reusing a requirement prefix would render two different items as "FR-1".
	t.Run("rejects a prefix used by another entity type", func(t *testing.T) {
		svc := newCategoryService(categoriesNamed(requirementCategory()), nil, nil, ownerRoles())

		_, err := svc.CreateCategory(context.Background(), testutil.MakeUser(), CreateCategoryInput{
			ProductID: "product-001", EntityType: "constraint",
			Name: "Functional Rules", Prefix: "FR",
		})
		if err == nil {
			t.Fatal("cross-type prefix reuse was accepted, want rejection")
		}
		if !strings.Contains(err.Error(), "Functionality") {
			t.Fatalf("error %q does not name the conflicting category", err)
		}
	})

	t.Run("rejects a duplicate name within the same entity type", func(t *testing.T) {
		svc := newCategoryService(categoriesNamed(requirementCategory()), nil, nil, ownerRoles())

		_, err := svc.CreateCategory(context.Background(), testutil.MakeUser(), CreateCategoryInput{
			ProductID: "product-001", EntityType: model.RequirementKindRequirement,
			Name: "Functionality", Prefix: "FN",
		})
		if err == nil {
			t.Fatal("duplicate name was accepted, want rejection")
		}
	})

	t.Run("allows the same name across entity types", func(t *testing.T) {
		svc := newCategoryService(categoriesNamed(constraintCategory()), nil, nil, ownerRoles())

		_, err := svc.CreateCategory(context.Background(), testutil.MakeUser(), CreateCategoryInput{
			ProductID: "product-001", EntityType: model.RequirementKindRequirement,
			Name: "Architecture", Prefix: "AR",
		})
		if err != nil {
			t.Fatalf("CreateCategory: %v", err)
		}
	})
}

func TestUpdateCategoryPrefix(t *testing.T) {
	t.Run("allows rename even when items exist", func(t *testing.T) {
		cat := requirementCategory()
		store := categoriesNamed(cat)
		var updated *model.Category
		store.UpdateFunc = func(_ context.Context, c *model.Category) error { updated = c; return nil }

		svc := newCategoryService(store, &testutil.MockRequirementStore{
			CountByCategoryIDFunc: func(_ context.Context, _ string) (int, error) { return 47, nil },
		}, nil, ownerRoles())

		newPrefix := "xy"
		_, err := svc.UpdateCategory(context.Background(), testutil.MakeUser(), "product-001", cat.ID,
			UpdateCategoryInput{Prefix: &newPrefix})
		if err != nil {
			t.Fatalf("UpdateCategory: %v", err)
		}
		if updated.Prefix != "XY" {
			t.Fatalf("Prefix = %q, want %q", updated.Prefix, "XY")
		}
	})

	t.Run("rejects a prefix taken by a sibling", func(t *testing.T) {
		target := requirementCategory()
		other := testutil.MakeCategory(func(c *model.Category) {
			c.ID = "cat-002"
			c.Name = "Architecture"
			c.Prefix = "AR"
		})
		svc := newCategoryService(categoriesNamed(target, other), nil, nil, ownerRoles())

		newPrefix := "AR"
		_, err := svc.UpdateCategory(context.Background(), testutil.MakeUser(), "product-001", target.ID,
			UpdateCategoryInput{Prefix: &newPrefix})
		if err == nil {
			t.Fatal("duplicate prefix was accepted, want rejection")
		}
	})

	t.Run("keeping its own prefix is not a conflict", func(t *testing.T) {
		cat := requirementCategory()
		svc := newCategoryService(categoriesNamed(cat), nil, nil, ownerRoles())

		samePrefix := "FR"
		if _, err := svc.UpdateCategory(context.Background(), testutil.MakeUser(), "product-001", cat.ID,
			UpdateCategoryInput{Prefix: &samePrefix}); err != nil {
			t.Fatalf("UpdateCategory: %v", err)
		}
	})
}

func TestDeleteCategory(t *testing.T) {
	t.Run("refuses while items are assigned", func(t *testing.T) {
		cat := requirementCategory()
		spare := testutil.MakeCategory(func(c *model.Category) { c.ID = "cat-002"; c.Name = "Architecture"; c.Prefix = "AR" })
		svc := newCategoryService(categoriesNamed(cat, spare), &testutil.MockRequirementStore{
			CountByCategoryIDFunc: func(_ context.Context, _ string) (int, error) { return 47, nil },
		}, nil, ownerRoles())

		err := svc.DeleteCategory(context.Background(), testutil.MakeUser(), "product-001", cat.ID)
		if err == nil {
			t.Fatal("delete of an in-use category succeeded, want rejection")
		}
		if !strings.Contains(err.Error(), "47 items") {
			t.Fatalf("error %q does not report the item count", err)
		}
	})

	t.Run("counts constraints for constraint categories", func(t *testing.T) {
		cat := constraintCategory()
		spare := testutil.MakeCategory(func(c *model.Category) {
			c.ID = "cat-con-002"
			c.EntityType = "constraint"
			c.Name = "Security"
			c.Prefix = "SC"
		})
		svc := newCategoryService(categoriesNamed(cat, spare), nil, &testutil.MockConstraintStore{
			CountByCategoryIDFunc: func(_ context.Context, _ string) (int, error) { return 3, nil },
		}, ownerRoles())

		err := svc.DeleteCategory(context.Background(), testutil.MakeUser(), "product-001", cat.ID)
		if err == nil || !strings.Contains(err.Error(), "3 items") {
			t.Fatalf("err = %v, want a refusal naming 3 items", err)
		}
	})

	t.Run("refuses the last category of an entity type", func(t *testing.T) {
		cat := requirementCategory()
		svc := newCategoryService(categoriesNamed(cat), nil, nil, ownerRoles())

		err := svc.DeleteCategory(context.Background(), testutil.MakeUser(), "product-001", cat.ID)
		if err == nil {
			t.Fatal("delete of the last category succeeded, want rejection")
		}
		if !strings.Contains(err.Error(), "last requirement category") {
			t.Fatalf("error %q does not explain the last-category rule", err)
		}
	})

	t.Run("succeeds for an unused, non-last category", func(t *testing.T) {
		cat := requirementCategory()
		spare := testutil.MakeCategory(func(c *model.Category) { c.ID = "cat-002"; c.Name = "Architecture"; c.Prefix = "AR" })
		store := categoriesNamed(cat, spare)
		var deleted string
		store.DeleteFunc = func(_ context.Context, id string) error { deleted = id; return nil }
		svc := newCategoryService(store, nil, nil, ownerRoles())

		if err := svc.DeleteCategory(context.Background(), testutil.MakeUser(), "product-001", cat.ID); err != nil {
			t.Fatalf("DeleteCategory: %v", err)
		}
		if deleted != cat.ID {
			t.Fatalf("deleted = %q, want %q", deleted, cat.ID)
		}
	})
}

func TestCategoryUsageCounts(t *testing.T) {
	cat := requirementCategory()
	other := testutil.MakeCategory(func(c *model.Category) { c.ID = "cat-002"; c.Name = "Architecture"; c.Prefix = "AR" })
	svc := newCategoryService(categoriesNamed(cat, other), &testutil.MockRequirementStore{
		CountByCategoryIDFunc: func(_ context.Context, categoryID string) (int, error) {
			if categoryID == cat.ID {
				return 12, nil
			}
			return 0, nil
		},
	}, nil, viewerRoles())

	counts, err := svc.UsageCounts(context.Background(), testutil.MakeUser(), "product-001", model.RequirementKindRequirement)
	if err != nil {
		t.Fatalf("UsageCounts: %v", err)
	}
	if counts[cat.ID] != 12 {
		t.Fatalf("counts[%q] = %d, want 12", cat.ID, counts[cat.ID])
	}
	if counts[other.ID] != 0 {
		t.Fatalf("counts[%q] = %d, want 0", other.ID, counts[other.ID])
	}
}

func TestCategoryAuthorization(t *testing.T) {
	cat := requirementCategory()

	t.Run("developer cannot create", func(t *testing.T) {
		svc := newCategoryService(categoriesNamed(), nil, nil, developerRoles())
		_, err := svc.CreateCategory(context.Background(), testutil.MakeUser(), CreateCategoryInput{
			ProductID: "product-001", EntityType: model.RequirementKindRequirement,
			Name: "Performance", Prefix: "PF",
		})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("developer cannot update", func(t *testing.T) {
		svc := newCategoryService(categoriesNamed(cat), nil, nil, developerRoles())
		name := "Renamed"
		_, err := svc.UpdateCategory(context.Background(), testutil.MakeUser(), "product-001", cat.ID,
			UpdateCategoryInput{Name: &name})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("developer cannot delete", func(t *testing.T) {
		svc := newCategoryService(categoriesNamed(cat), nil, nil, developerRoles())
		err := svc.DeleteCategory(context.Background(), testutil.MakeUser(), "product-001", cat.ID)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("viewer can list", func(t *testing.T) {
		svc := newCategoryService(categoriesNamed(cat), nil, nil, viewerRoles())
		cats, err := svc.ListCategories(context.Background(), testutil.MakeUser(), "product-001", model.RequirementKindRequirement)
		if err != nil {
			t.Fatalf("ListCategories: %v", err)
		}
		if len(cats) != 1 {
			t.Fatalf("len(cats) = %d, want 1", len(cats))
		}
	})
}
