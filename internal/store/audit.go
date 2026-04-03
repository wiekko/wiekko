package store

import (
	"context"
	"database/sql"

	"nexmap.net/internal/model"
)

// AuditStore defines data access operations for the audit trail.
type AuditStore interface {
	Create(ctx context.Context, entry *model.AuditEntry) error
	ListByEntity(ctx context.Context, entityType, entityID string) ([]*model.AuditEntry, error)
}

type auditStore struct {
	db *sql.DB
}

func (s *auditStore) Create(ctx context.Context, entry *model.AuditEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_trail (id, entity_type, entity_id, user_id, action, field_name, old_value, new_value, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.EntityType, entry.EntityID, entry.UserID,
		entry.Action, nullableText(entry.FieldName),
		nullableText(entry.OldValue), nullableText(entry.NewValue),
		entry.CreatedAt,
	)
	return err
}

func (s *auditStore) ListByEntity(ctx context.Context, entityType, entityID string) ([]*model.AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, entity_type, entity_id, user_id, action, field_name, old_value, new_value, created_at
		 FROM audit_trail WHERE entity_type = ? AND entity_id = ? ORDER BY created_at`,
		entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var entries []*model.AuditEntry
	for rows.Next() {
		var e model.AuditEntry
		var fieldName, oldValue, newValue sql.NullString
		if err := rows.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.UserID,
			&e.Action, &fieldName, &oldValue, &newValue, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.FieldName = fieldName.String
		e.OldValue = oldValue.String
		e.NewValue = newValue.String
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}
