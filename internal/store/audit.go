// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"database/sql"
	"strings"

	"wiekko.net/internal/model"
)

// AuditStore defines data access operations for the audit trail.
type AuditStore interface {
	Create(ctx context.Context, entry *model.AuditEntry) error
	ListByEntity(ctx context.Context, entityType, entityID string) ([]*model.AuditEntry, error)
	// List returns the entries matching f, newest first, paged by p.Limit and
	// p.Offset; Count returns how many match in all.
	List(ctx context.Context, f AuditFilter, p ListParams) ([]*model.AuditEntry, error)
	Count(ctx context.Context, f AuditFilter) (int, error)
	// Each streams the entries matching f, oldest first, for an export.
	Each(ctx context.Context, f AuditFilter, fn func(*model.AuditEntry) error) error
	// DeleteByProduct removes every entry of a product and reports how many.
	DeleteByProduct(ctx context.Context, productID string) (int64, error)
	// DeletedProducts lists the products whose deletion the trail recorded and
	// that no longer exist, with the entries still kept for each.
	DeletedProducts(ctx context.Context) ([]*model.DeletedProduct, error)
	// DeletedEntities reports which of ids the trail records as deleted.
	DeletedEntities(ctx context.Context, ids []string) (map[string]bool, error)
}

// AuditFilter narrows an audit listing. Empty fields match everything; From
// and To are RFC 3339 instants, From inclusive and To exclusive.
type AuditFilter struct {
	ProductID  string
	EntityType string
	UserID     string
	Action     string
	From       string
	To         string
}

func (f AuditFilter) where() (string, []any) {
	var conds []string
	var args []any
	add := func(cond, v string) {
		if v != "" {
			conds = append(conds, cond)
			args = append(args, v)
		}
	}
	add("product_id = ?", f.ProductID)
	add("entity_type = ?", f.EntityType)
	add("user_id = ?", f.UserID)
	add("action = ?", f.Action)
	add("created_at >= ?", f.From)
	add("created_at < ?", f.To)
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

type auditStore struct {
	db *DB
}

const auditColumns = `id, product_id, entity_type, entity_id, user_id, action, field_name, old_value, new_value, created_at`

func (s *auditStore) Create(ctx context.Context, entry *model.AuditEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_trail (`+auditColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, nullableText(entry.ProductID), entry.EntityType, entry.EntityID, entry.UserID,
		entry.Action, nullableText(entry.FieldName),
		nullableText(entry.OldValue), nullableText(entry.NewValue),
		entry.CreatedAt,
	)
	return err
}

func (s *auditStore) ListByEntity(ctx context.Context, entityType, entityID string) ([]*model.AuditEntry, error) {
	return s.query(ctx,
		`SELECT `+auditColumns+` FROM audit_trail WHERE entity_type = ? AND entity_id = ? ORDER BY created_at, id`,
		entityType, entityID)
}

func (s *auditStore) List(ctx context.Context, f AuditFilter, p ListParams) ([]*model.AuditEntry, error) {
	where, args := f.where()
	limit, largs := limitClause(p)
	return s.query(ctx,
		`SELECT `+auditColumns+` FROM audit_trail`+where+` ORDER BY created_at DESC, id DESC`+limit,
		append(args, largs...)...)
}

func (s *auditStore) Count(ctx context.Context, f AuditFilter) (int, error) {
	where, args := f.where()
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_trail`+where, args...).Scan(&n)
	return n, err
}

func (s *auditStore) Each(ctx context.Context, f AuditFilter, fn func(*model.AuditEntry) error) error {
	where, args := f.where()
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+auditColumns+` FROM audit_trail`+where+` ORDER BY created_at, id`, args...)
	if err != nil {
		return err
	}
	defer closeRows(rows)
	for rows.Next() {
		e, err := scanAuditEntry(rows)
		if err != nil {
			return err
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *auditStore) DeleteByProduct(ctx context.Context, productID string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_trail WHERE product_id = ?`, productID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *auditStore) DeletedProducts(ctx context.Context) ([]*model.DeletedProduct, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.product_id, d.old_value, d.created_at,
		        (SELECT COUNT(*) FROM audit_trail a WHERE a.product_id = d.product_id)
		 FROM audit_trail d
		 WHERE d.entity_type = 'product' AND d.action = 'delete' AND d.product_id IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM products p WHERE p.id = d.product_id)
		 ORDER BY d.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []*model.DeletedProduct
	for rows.Next() {
		var p model.DeletedProduct
		var name sql.NullString
		if err := rows.Scan(&p.ID, &name, &p.DeletedAt, &p.Entries); err != nil {
			return nil, err
		}
		p.Name = name.String
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (s *auditStore) DeletedEntities(ctx context.Context, ids []string) (map[string]bool, error) {
	out := make(map[string]bool)
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT entity_id FROM audit_trail WHERE action = 'delete' AND entity_id IN (?`+
			strings.Repeat(", ?", len(ids)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *auditStore) query(ctx context.Context, q string, args ...any) ([]*model.AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var entries []*model.AuditEntry
	for rows.Next() {
		e, err := scanAuditEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func scanAuditEntry(rows *Rows) (*model.AuditEntry, error) {
	var e model.AuditEntry
	var productID, fieldName, oldValue, newValue sql.NullString
	if err := rows.Scan(&e.ID, &productID, &e.EntityType, &e.EntityID, &e.UserID,
		&e.Action, &fieldName, &oldValue, &newValue, &e.CreatedAt); err != nil {
		return nil, err
	}
	e.ProductID = productID.String
	e.FieldName = fieldName.String
	e.OldValue = oldValue.String
	e.NewValue = newValue.String
	return &e, nil
}
