package service

import (
	"context"
	"fmt"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// AuditService provides read access to the audit trail.
type AuditService struct {
	audit store.AuditStore
	authz *Authorizer
	owns  *ownership
}

// ListByEntity returns audit entries for an entity. Requires viewer role.
func (s *AuditService) ListByEntity(ctx context.Context, user *model.User, productID, entityType, entityID string) ([]*model.AuditEntry, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	// The audit table is keyed by entity only; make sure the entity belongs to
	// the product before handing out its history.
	kind, ok := auditEntityKinds[entityType]
	if !ok {
		return nil, fmt.Errorf("%w: unknown audit entity type %q", ErrNotFound, entityType)
	}
	if err := s.owns.require(ctx, productID, ref{kind, entityID}); err != nil {
		return nil, err
	}
	return s.audit.ListByEntity(ctx, entityType, entityID)
}
