// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// RuntimeEnvironmentStore defines data access operations for runtime environments.
type RuntimeEnvironmentStore interface {
	Create(ctx context.Context, env *model.RuntimeEnvironment) error
	GetByID(ctx context.Context, id string) (*model.RuntimeEnvironment, error)
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.RuntimeEnvironment, error)
}

type runtimeEnvironmentStore struct {
	db *DB
}

func (s *runtimeEnvironmentStore) Create(ctx context.Context, env *model.RuntimeEnvironment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runtime_environments (id, product_id, name, created_at) VALUES (?, ?, ?, ?)`,
		env.ID, env.ProductID, env.Name, env.CreatedAt,
	)
	return err
}

func (s *runtimeEnvironmentStore) GetByID(ctx context.Context, id string) (*model.RuntimeEnvironment, error) {
	var env model.RuntimeEnvironment
	err := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, name, created_at FROM runtime_environments WHERE id = ?`, id,
	).Scan(&env.ID, &env.ProductID, &env.Name, &env.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("runtime environment not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &env, nil
}

func (s *runtimeEnvironmentStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM runtime_environments WHERE id = ?`, id)
	return err
}

func (s *runtimeEnvironmentStore) ListByProductID(ctx context.Context, productID string) ([]*model.RuntimeEnvironment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, name, created_at
		 FROM runtime_environments WHERE product_id = ? ORDER BY name`, productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var envs []*model.RuntimeEnvironment
	for rows.Next() {
		var env model.RuntimeEnvironment
		if err := rows.Scan(&env.ID, &env.ProductID, &env.Name, &env.CreatedAt); err != nil {
			return nil, err
		}
		envs = append(envs, &env)
	}
	return envs, rows.Err()
}
