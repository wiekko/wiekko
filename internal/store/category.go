// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"wiekko.net/internal/model"
)

// CategoryStore defines data access operations for categories.
type CategoryStore interface {
	Create(ctx context.Context, cat *model.Category) error
	GetByID(ctx context.Context, id string) (*model.Category, error)
	Update(ctx context.Context, cat *model.Category) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID, entityType string) ([]*model.Category, error)
	GetByProductAndName(ctx context.Context, productID, entityType, name string) (*model.Category, error)
	// GetByProductAndPrefix looks a prefix up across every entity type, matching
	// the product-wide UNIQUE(product_id, prefix) index.
	GetByProductAndPrefix(ctx context.Context, productID, prefix string) (*model.Category, error)
}

type categoryStore struct {
	db *DB
}

func (s *categoryStore) Create(ctx context.Context, cat *model.Category) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO categories (id, product_id, entity_type, name, prefix, position, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cat.ID, cat.ProductID, cat.EntityType, cat.Name, cat.Prefix, cat.Position, cat.CreatedAt,
	)
	return err
}

func (s *categoryStore) GetByID(ctx context.Context, id string) (*model.Category, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, entity_type, name, prefix, position, created_at
		 FROM categories WHERE id = ?`, id)
	cat, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("category not found: %w", err)
	}
	return cat, err
}

func (s *categoryStore) Update(ctx context.Context, cat *model.Category) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE categories SET name = ?, prefix = ?, position = ? WHERE id = ?`,
		cat.Name, cat.Prefix, cat.Position, cat.ID,
	)
	return err
}

func (s *categoryStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
	return err
}

func (s *categoryStore) ListByProductID(ctx context.Context, productID, entityType string) ([]*model.Category, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, entity_type, name, prefix, position, created_at
		 FROM categories WHERE product_id = ? AND entity_type = ? ORDER BY position, name`,
		productID, entityType)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var cats []*model.Category
	for rows.Next() {
		cat, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		cats = append(cats, cat)
	}
	return cats, rows.Err()
}

func (s *categoryStore) GetByProductAndName(ctx context.Context, productID, entityType, name string) (*model.Category, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, entity_type, name, prefix, position, created_at
		 FROM categories WHERE product_id = ? AND entity_type = ? AND name = ?`,
		productID, entityType, name)
	cat, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("category not found: %w", err)
	}
	return cat, err
}

func (s *categoryStore) GetByProductAndPrefix(ctx context.Context, productID, prefix string) (*model.Category, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, entity_type, name, prefix, position, created_at
		 FROM categories WHERE product_id = ? AND prefix = ?`,
		productID, prefix)
	cat, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("category not found: %w", err)
	}
	return cat, err
}

func scanCategory(sc scanner) (*model.Category, error) {
	var cat model.Category
	err := sc.Scan(&cat.ID, &cat.ProductID, &cat.EntityType, &cat.Name,
		&cat.Prefix, &cat.Position, &cat.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &cat, nil
}
