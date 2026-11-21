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

// PyramidLevelStore defines data access operations for pyramid levels.
type PyramidLevelStore interface {
	Create(ctx context.Context, pl *model.PyramidLevel) error
	GetByID(ctx context.Context, id string) (*model.PyramidLevel, error)
	Update(ctx context.Context, pl *model.PyramidLevel) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.PyramidLevel, error)
}

type pyramidLevelStore struct {
	db *DB
}

func (s *pyramidLevelStore) Create(ctx context.Context, pl *model.PyramidLevel) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pyramid_levels (id, product_id, parent_id, name, position, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		pl.ID, pl.ProductID, nullableText(pl.ParentID),
		pl.Name, pl.Position, pl.CreatedAt,
	)
	return err
}

func (s *pyramidLevelStore) GetByID(ctx context.Context, id string) (*model.PyramidLevel, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, parent_id, name, position, created_at
		 FROM pyramid_levels WHERE id = ?`, id)
	pl, err := scanPyramidLevel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("pyramid level not found: %w", err)
	}
	return pl, err
}

func (s *pyramidLevelStore) Update(ctx context.Context, pl *model.PyramidLevel) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE pyramid_levels SET parent_id = ?, name = ?, position = ?
		 WHERE id = ?`,
		nullableText(pl.ParentID), pl.Name, pl.Position, pl.ID,
	)
	return err
}

func (s *pyramidLevelStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pyramid_levels WHERE id = ?`, id)
	return err
}

func (s *pyramidLevelStore) ListByProductID(ctx context.Context, productID string) ([]*model.PyramidLevel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, parent_id, name, position, created_at
		 FROM pyramid_levels WHERE product_id = ? ORDER BY position`, productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var levels []*model.PyramidLevel
	for rows.Next() {
		pl, err := scanPyramidLevel(rows)
		if err != nil {
			return nil, err
		}
		levels = append(levels, pl)
	}
	return levels, rows.Err()
}

func scanPyramidLevel(sc scanner) (*model.PyramidLevel, error) {
	var pl model.PyramidLevel
	var parentID sql.NullString
	err := sc.Scan(&pl.ID, &pl.ProductID, &parentID, &pl.Name,
		&pl.Position, &pl.CreatedAt)
	if err != nil {
		return nil, err
	}
	pl.ParentID = parentID.String
	return &pl, nil
}
