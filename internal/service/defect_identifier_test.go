package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

// Defect identifiers (FR-49).

func prefixedProducts(prefix string) *testutil.MockProductStore {
	return &testutil.MockProductStore{
		GetByIDFunc: func(_ context.Context, id string) (*model.Product, error) {
			return testutil.MakeProduct(func(p *model.Product) { p.ID = id; p.DefectPrefix = prefix }), nil
		},
		UpdateFunc: func(_ context.Context, _ *model.Product) error { return nil },
	}
}

func TestCreateDefectNumbers(t *testing.T) {
	t.Run("draws the next number and returns the identifier", func(t *testing.T) {
		var created *model.Defect
		svc := newBacklogService(nil, &testutil.MockDefectStore{
			NextSequenceNumberFunc: func(_ context.Context, _ string) (int, error) { return 7, nil },
			CreateFunc:             func(_ context.Context, d *model.Defect) error { created = d; return nil },
		}, noopAudit(), testerRoles())
		svc.products = prefixedProducts("BUG")

		d, err := svc.CreateDefect(context.Background(), testutil.MakeUser(), CreateDefectInput{ProductID: "product-001", Title: "x"})
		if err != nil {
			t.Fatalf("CreateDefect: %v", err)
		}
		if created.SequenceNumber != 7 || d.Identifier != "BUG-7" {
			t.Fatalf("number %d, identifier %q; want 7, BUG-7", created.SequenceNumber, d.Identifier)
		}
	})

	t.Run("other insert errors are not retried", func(t *testing.T) {
		attempts := 0
		svc := newBacklogService(nil, &testutil.MockDefectStore{
			CreateFunc: func(_ context.Context, _ *model.Defect) error { attempts++; return errors.New("disk full") },
		}, noopAudit(), testerRoles())
		if _, err := svc.CreateDefect(context.Background(), testutil.MakeUser(), CreateDefectInput{ProductID: "product-001", Title: "x"}); err == nil {
			t.Fatal("expected an error")
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d, want 1", attempts)
		}
	})
}

func TestResolveDefectRef(t *testing.T) {
	svc := newBacklogService(nil, &testutil.MockDefectStore{
		GetBySequenceFunc: func(_ context.Context, productID string, seq int) (*model.Defect, error) {
			if productID == "product-001" && seq == 17 {
				return &model.Defect{ID: "defect-17"}, nil
			}
			return nil, errors.New("not found")
		},
	}, nil, viewerRoles())
	svc.products = prefixedProducts("BUG")
	ctx, user := context.Background(), testutil.MakeUser()

	for _, ref := range []string{"BUG-17", "bug-17", " BUG-17 "} {
		if id, err := svc.ResolveDefectRef(ctx, user, "product-001", ref); err != nil || id != "defect-17" {
			t.Errorf("%q = %q, %v; want defect-17", ref, id, err)
		}
	}
	for _, ref := range []string{"DF-17", "FR-17", "BUG-18"} {
		if _, err := svc.ResolveDefectRef(ctx, user, "product-001", ref); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: err = %v, want ErrNotFound", ref, err)
		}
	}
	if id, err := svc.ResolveDefectRef(ctx, user, "product-001", "0b9d3c4e-uuid"); err != nil || id != "0b9d3c4e-uuid" {
		t.Errorf("IDs must pass through unchanged: %q, %v", id, err)
	}
	if _, err := svc.ResolveDefectRef(ctx, user, "product-001", ""); err != nil {
		t.Errorf("an empty reference passes through: %v", err)
	}
	svc.authz = &Authorizer{productRoles: forbiddenRoles()}
	if _, err := svc.ResolveDefectRef(ctx, user, "product-001", "BUG-17"); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-member resolved an identifier: %v", err)
	}
}

func TestUpdateDefectPrefix(t *testing.T) {
	newSvc := func(stored **model.Product) *ProductService {
		products := prefixedProducts(model.DefaultDefectPrefix)
		products.UpdateFunc = func(_ context.Context, p *model.Product) error { *stored = p; return nil }
		svc := newProductService(products, ownerRoles(), nil, noopAudit())
		svc.categories = &testutil.MockCategoryStore{
			GetByProductAndPrefixFunc: func(_ context.Context, _, prefix string) (*model.Category, error) {
				if prefix == "FR" {
					return &model.Category{Name: "Functionality", EntityType: "requirement", Prefix: "FR"}, nil
				}
				return nil, errors.New("not found")
			},
		}
		return svc
	}
	ctx, user := context.Background(), testutil.MakeUser()

	var stored *model.Product
	p, err := newSvc(&stored).Update(ctx, user, "product-001", UpdateProductInput{DefectPrefix: new(" bug ")})
	if err != nil || p.DefectPrefix != "BUG" || stored.DefectPrefix != "BUG" {
		t.Fatalf("prefix = %+v, %v; want BUG, normalised", p, err)
	}
	for _, bad := range []string{"", "1X", "TOOLONGX", "D-F", "fr"} {
		stored = nil
		if _, err := newSvc(&stored).Update(ctx, user, "product-001", UpdateProductInput{DefectPrefix: new(bad)}); err == nil {
			t.Errorf("prefix %q accepted", bad)
		}
		if stored != nil {
			t.Errorf("prefix %q stored despite the error", bad)
		}
	}
}

func TestCategoryPrefixAvoidsDefectPrefix(t *testing.T) {
	svc := newCategoryService(categoriesNamed(), nil, nil, ownerRoles())
	svc.products = prefixedProducts("BUG")
	_, err := svc.CreateCategory(context.Background(), testutil.MakeUser(), CreateCategoryInput{
		ProductID: "product-001", EntityType: "requirement", Name: "Bugs", Prefix: "bug"})
	if err == nil {
		t.Fatal("a category took the defect prefix")
	}
}
