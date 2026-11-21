// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"testing"

	"wiekko.net/internal/model"
	"wiekko.net/internal/testutil"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		in     string
		prefix string
		seq    int
		ok     bool
	}{
		{"FR-12", "FR", 12, true},
		{"mt-3", "MT", 3, true},
		{" AC42-7 ", "AC42", 7, true},
		{"FR-0", "", 0, false},
		{"FR12", "", 0, false},
		{"1FR-2", "", 0, false},
		{"TOOLONG-1", "", 0, false},
		{"8d0e11fa-3486-4359-8c96-ae7b0f8e0541", "", 0, false},
		{"", "", 0, false},
	}
	for _, tt := range tests {
		prefix, seq, ok := parseRef(tt.in)
		if prefix != tt.prefix || seq != tt.seq || ok != tt.ok {
			t.Errorf("parseRef(%q) = %q, %d, %v; want %q, %d, %v", tt.in, prefix, seq, ok, tt.prefix, tt.seq, tt.ok)
		}
	}
}

// refCategories serves the categories FR (requirement), MT (task) and AC
// (constraint) of product-001.
func refCategories() *testutil.MockCategoryStore {
	cats := map[string]*model.Category{
		"FR": {ID: "cat-fr", ProductID: "product-001", EntityType: model.RequirementKindRequirement, Prefix: "FR"},
		"MT": {ID: "cat-mt", ProductID: "product-001", EntityType: model.RequirementKindTask, Prefix: "MT"},
		"AC": {ID: "cat-ac", ProductID: "product-001", EntityType: "constraint", Prefix: "AC"},
	}
	return &testutil.MockCategoryStore{
		GetByProductAndPrefixFunc: func(_ context.Context, productID, prefix string) (*model.Category, error) {
			if c, ok := cats[prefix]; ok && productID == c.ProductID {
				return c, nil
			}
			return nil, errors.New("category not found")
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
	}
}

func TestResolveRequirementRef(t *testing.T) {
	reqs := &testutil.MockRequirementStore{
		GetBySequenceFunc: func(_ context.Context, productID string, seq int) (*model.Requirement, error) {
			if productID == "product-001" && seq == 12 {
				return &model.Requirement{ID: "req-12", ProductID: productID, CategoryID: "cat-fr", SequenceNumber: 12}, nil
			}
			return nil, errors.New("requirement not found")
		},
	}
	svc := &BacklogService{requirements: reqs, categories: refCategories(), authz: &Authorizer{productRoles: viewerRoles()}}
	ctx := context.Background()
	user := testutil.MakeUser()

	if id, err := svc.ResolveRequirementRef(ctx, user, "product-001", "fr-12"); err != nil || id != "req-12" {
		t.Fatalf("FR-12 = %q, %v; want req-12", id, err)
	}
	// IDs pass through untouched.
	if id, err := svc.ResolveRequirementRef(ctx, user, "product-001", "some-uuid-value"); err != nil || id != "some-uuid-value" {
		t.Fatalf("passthrough = %q, %v", id, err)
	}
	for _, ref := range []string{
		"MT-12", // right number, wrong category: must not alias FR-12
		"AC-12", // constraint prefix
		"FR-13", // no such number
		"XX-12", // unknown prefix
	} {
		if _, err := svc.ResolveRequirementRef(ctx, user, "product-001", ref); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", ref, err)
		}
	}
	if _, err := svc.ResolveRequirementRef(ctx, user, "product-002", "FR-12"); !errors.Is(err, ErrNotFound) {
		t.Errorf("other product: err = %v, want ErrNotFound", err)
	}

	denied := &BacklogService{requirements: reqs, categories: refCategories(), authz: &Authorizer{productRoles: forbiddenRoles()}}
	if _, err := denied.ResolveRequirementRef(ctx, user, "product-001", "FR-12"); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-member: err = %v, want ErrForbidden", err)
	}
}

func TestResolveConstraintRef(t *testing.T) {
	cons := &testutil.MockConstraintStore{
		GetBySequenceFunc: func(_ context.Context, productID string, seq int) (*model.Constraint, error) {
			if seq == 4 {
				return &model.Constraint{ID: "con-4", ProductID: productID, CategoryID: "cat-ac", SequenceNumber: 4}, nil
			}
			return nil, errors.New("constraint not found")
		},
	}
	svc := &ConstraintService{constraints: cons, categories: refCategories(), authz: &Authorizer{productRoles: viewerRoles()}}
	ctx := context.Background()
	user := testutil.MakeUser()

	if id, err := svc.ResolveConstraintRef(ctx, user, "product-001", "AC-4"); err != nil || id != "con-4" {
		t.Fatalf("AC-4 = %q, %v", id, err)
	}
	if _, err := svc.ResolveConstraintRef(ctx, user, "product-001", "FR-4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FR-4 as constraint: err = %v, want ErrNotFound", err)
	}
}

func TestCategoryPrefixes(t *testing.T) {
	ctx := context.Background()
	backlog := &BacklogService{categories: refCategories(), authz: &Authorizer{productRoles: viewerRoles()}}
	m, err := backlog.CategoryPrefixes(ctx, testutil.MakeUser(), "product-001")
	if err != nil {
		t.Fatal(err)
	}
	if m["cat-fr"] != "FR" || m["cat-mt"] != "MT" || m["cat-ac"] != "" {
		t.Fatalf("backlog prefixes = %v", m)
	}
	cons := &ConstraintService{categories: refCategories(), authz: &Authorizer{productRoles: viewerRoles()}}
	m, err = cons.CategoryPrefixes(ctx, testutil.MakeUser(), "product-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m["cat-ac"] != "AC" {
		t.Fatalf("constraint prefixes = %v", m)
	}
}
