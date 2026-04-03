package service

import (
	"context"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// AuditService provides read access to the audit trail.
type AuditService struct {
	audit store.AuditStore
	authz *Authorizer
}

// ListByEntity returns audit entries for an entity. Requires viewer role.
func (s *AuditService) ListByEntity(ctx context.Context, user *model.User, productID, entityType, entityID string) ([]*model.AuditEntry, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.audit.ListByEntity(ctx, entityType, entityID)
}
