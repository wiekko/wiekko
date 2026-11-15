package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// AuditService provides read access to the audit trail.
type AuditService struct {
	audit    store.AuditStore
	products store.ProductStore // for the purge: only deleted products qualify
	tx       store.Transactor
	authz    *Authorizer
	owns     *ownership
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

// ErrProductExists is returned when an administrator tries to purge the audit
// trail of a product that still exists: a live product keeps its history.
var ErrProductExists = errors.New("this product still exists; only the history of deleted products can be purged")

// AuditFilterInput is an administrator's audit query as it arrives from a
// form or query string. From and To are dates (2006-01-02) or RFC 3339
// instants; a date in To includes the whole day.
type AuditFilterInput struct {
	ProductID, EntityType, UserID, Action, From, To string
}

func (in AuditFilterInput) resolve() (store.AuditFilter, error) {
	f := store.AuditFilter{
		ProductID:  strings.TrimSpace(in.ProductID),
		EntityType: strings.TrimSpace(in.EntityType),
		UserID:     strings.TrimSpace(in.UserID),
		Action:     strings.TrimSpace(in.Action),
	}
	for _, id := range []string{f.ProductID, f.UserID} {
		if id != "" {
			if _, err := uuid.Parse(id); err != nil {
				return f, fmt.Errorf("invalid ID %q", id)
			}
		}
	}
	var err error
	if f.From, err = auditInstant(in.From, false); err != nil {
		return f, err
	}
	if f.To, err = auditInstant(in.To, true); err != nil {
		return f, err
	}
	return f, nil
}

// auditInstant normalizes a date or instant to the RFC 3339 form the trail is
// stored in, so the comparison works on both engines. A bare date as the end
// of a range moves to the next midnight: "to 2026-03-01" includes that day.
func auditInstant(v string, end bool) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if t, err := time.Parse(time.DateOnly, v); err == nil {
		if end {
			t = t.AddDate(0, 0, 1)
		}
		return t.UTC().Format(time.RFC3339), nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return "", fmt.Errorf("invalid date %q; use YYYY-MM-DD", v)
	}
	return t.UTC().Format(time.RFC3339), nil
}

func requireAdmin(user *model.User) error {
	if user == nil || !user.IsAdmin {
		return fmt.Errorf("%w: administrators only", ErrForbidden)
	}
	return nil
}

// AdminList returns one page of the whole audit trail, newest first, and the
// number of entries matching in all. Administrators only.
func (s *AuditService) AdminList(ctx context.Context, user *model.User, in AuditFilterInput, limit, offset int) ([]*model.AuditEntry, int, error) {
	if err := requireAdmin(user); err != nil {
		return nil, 0, err
	}
	f, err := in.resolve()
	if err != nil {
		return nil, 0, err
	}
	total, err := s.audit.Count(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	entries, err := s.audit.List(ctx, f, store.ListParams{Limit: limit, Offset: offset})
	return entries, total, err
}

// AdminExport streams the entries matching in, oldest first, for an archive.
// Administrators only.
func (s *AuditService) AdminExport(ctx context.Context, user *model.User, in AuditFilterInput, fn func(*model.AuditEntry) error) error {
	if err := requireAdmin(user); err != nil {
		return err
	}
	f, err := in.resolve()
	if err != nil {
		return err
	}
	return s.audit.Each(ctx, f, fn)
}

// AdminDeletedProducts lists deleted products whose history is still kept.
// Administrators only.
func (s *AuditService) AdminDeletedProducts(ctx context.Context, user *model.User) ([]*model.DeletedProduct, error) {
	if err := requireAdmin(user); err != nil {
		return nil, err
	}
	return s.audit.DeletedProducts(ctx)
}

// AdminDeletedEntities reports which of ids the trail records as deleted, so
// the admin page links only to items that still exist. Administrators only.
func (s *AuditService) AdminDeletedEntities(ctx context.Context, user *model.User, ids []string) (map[string]bool, error) {
	if err := requireAdmin(user); err != nil {
		return nil, err
	}
	return s.audit.DeletedEntities(ctx, ids)
}

// AdminPurgeProduct deletes the audit trail of a deleted product and records
// the purge itself, without a product, so it outlives what it removed.
// Administrators only; a product that still exists keeps its history.
func (s *AuditService) AdminPurgeProduct(ctx context.Context, user *model.User, productID string) (int64, error) {
	if err := requireAdmin(user); err != nil {
		return 0, err
	}
	if _, err := uuid.Parse(productID); err != nil {
		return 0, fmt.Errorf("%w: product", ErrNotFound)
	}
	var purged int64
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, "audit-purge:"+productID); err != nil {
			return err
		}
		if _, err := s.products.GetByID(ctx, productID); err == nil {
			return ErrProductExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		name := ""
		deleted, err := s.audit.DeletedProducts(ctx)
		if err != nil {
			return err
		}
		for _, p := range deleted {
			if p.ID == productID {
				name = p.Name
			}
		}
		if purged, err = s.audit.DeleteByProduct(ctx, productID); err != nil {
			return err
		}
		if purged == 0 {
			return fmt.Errorf("%w: no audit entries for this product", ErrNotFound)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "product", EntityID: productID,
			UserID: user.ID, Action: "purge_audit", FieldName: "entries",
			OldValue: name, NewValue: strconv.FormatInt(purged, 10),
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	})
	if err != nil {
		return 0, err
	}
	return purged, nil
}
