// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newAuditService(audit *testutil.MockAuditStore, roles *testutil.MockProductRoleStore) *AuditService {
	return &AuditService{
		owns:  testOwnership(),
		audit: audit,
		authz: &Authorizer{productRoles: roles},
	}
}

func TestListByEntity(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		entries := []*model.AuditEntry{testutil.MakeAuditEntry()}
		svc := newAuditService(
			&testutil.MockAuditStore{
				ListByEntityFunc: func(_ context.Context, _, _ string) ([]*model.AuditEntry, error) {
					return entries, nil
				},
			},
			&testutil.MockProductRoleStore{
				GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
					return testutil.MakeProductRole(func(pr *model.ProductRole) { pr.Role = model.RoleViewer }), nil
				},
			},
		)

		result, err := svc.ListByEntity(context.Background(), testutil.MakeUser(), "product-001", "product", "product-001")
		if err != nil {
			t.Fatalf("ListByEntity: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newAuditService(nil,
			&testutil.MockProductRoleStore{
				GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
					return nil, errors.New("not found")
				},
			},
		)

		_, err := svc.ListByEntity(context.Background(), testutil.MakeUser(), "product-001", "product", "product-001")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
