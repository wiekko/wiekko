package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// StatusStore defines data access operations for configurable statuses.
type StatusStore interface {
	Create(ctx context.Context, status *model.Status) error
	GetByID(ctx context.Context, id string) (*model.Status, error)
	Update(ctx context.Context, status *model.Status) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID, entityType string) ([]*model.Status, error)
	GetDefault(ctx context.Context, productID, entityType string) (*model.Status, error)
	GetByProductAndName(ctx context.Context, productID, entityType, name string) (*model.Status, error)
}

type statusStore struct {
	db *DB
}

func (s *statusStore) Create(ctx context.Context, status *model.Status) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO statuses (id, product_id, entity_type, name, position, is_default, is_closed, is_active, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		status.ID, status.ProductID, status.EntityType, status.Name,
		status.Position, status.IsDefault, status.IsClosed, status.IsActive,
		status.CreatedAt,
	)
	return err
}

func (s *statusStore) GetByID(ctx context.Context, id string) (*model.Status, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, entity_type, name, position, is_default, is_closed, is_active, created_at
		 FROM statuses WHERE id = ?`, id)
	st, err := scanStatus(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("status not found: %w", err)
	}
	return st, err
}

func (s *statusStore) Update(ctx context.Context, status *model.Status) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE statuses SET name = ?, position = ?, is_default = ?, is_closed = ?, is_active = ?
		 WHERE id = ?`,
		status.Name, status.Position,
		status.IsDefault, status.IsClosed, status.IsActive,
		status.ID,
	)
	return err
}

func (s *statusStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM statuses WHERE id = ?`, id)
	return err
}

func (s *statusStore) ListByProductID(ctx context.Context, productID, entityType string) ([]*model.Status, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, entity_type, name, position, is_default, is_closed, is_active, created_at
		 FROM statuses WHERE product_id = ? AND entity_type = ? ORDER BY position`,
		productID, entityType)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var statuses []*model.Status
	for rows.Next() {
		st, err := scanStatus(rows)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, st)
	}
	return statuses, rows.Err()
}

func (s *statusStore) GetDefault(ctx context.Context, productID, entityType string) (*model.Status, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, entity_type, name, position, is_default, is_closed, is_active, created_at
		 FROM statuses WHERE product_id = ? AND entity_type = ? AND is_default = TRUE
		 LIMIT 1`, productID, entityType)
	st, err := scanStatus(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("default status not found: %w", err)
	}
	return st, err
}

func (s *statusStore) GetByProductAndName(ctx context.Context, productID, entityType, name string) (*model.Status, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, entity_type, name, position, is_default, is_closed, is_active, created_at
		 FROM statuses WHERE product_id = ? AND entity_type = ? AND name = ?
		 LIMIT 1`, productID, entityType, name)
	st, err := scanStatus(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("status not found: %w", err)
	}
	return st, err
}

func scanStatus(sc scanner) (*model.Status, error) {
	var st model.Status
	err := sc.Scan(&st.ID, &st.ProductID, &st.EntityType, &st.Name,
		&st.Position, &st.IsDefault, &st.IsClosed, &st.IsActive, &st.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &st, nil
}
