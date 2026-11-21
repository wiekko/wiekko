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

// MilestoneStore defines data access operations for milestones.
type MilestoneStore interface {
	Create(ctx context.Context, m *model.Milestone) error
	GetByID(ctx context.Context, id string) (*model.Milestone, error)
	Update(ctx context.Context, m *model.Milestone) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.Milestone, error)
}

type milestoneStore struct {
	db *DB
}

func (s *milestoneStore) Create(ctx context.Context, m *model.Milestone) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO milestones (id, product_id, name, description, type, due_date, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ProductID, m.Name, m.Description, m.Type,
		nullableText(m.DueDate), m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (s *milestoneStore) GetByID(ctx context.Context, id string) (*model.Milestone, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, name, description, type, due_date, created_at, updated_at
		 FROM milestones WHERE id = ?`, id)
	m, err := scanMilestone(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("milestone not found: %w", err)
	}
	return m, err
}

func (s *milestoneStore) Update(ctx context.Context, m *model.Milestone) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE milestones SET name = ?, description = ?, type = ?, due_date = ?, updated_at = ?
		 WHERE id = ?`,
		m.Name, m.Description, m.Type,
		nullableText(m.DueDate), m.UpdatedAt, m.ID,
	)
	return err
}

func (s *milestoneStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM milestones WHERE id = ?`, id)
	return err
}

func (s *milestoneStore) ListByProductID(ctx context.Context, productID string) ([]*model.Milestone, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, name, description, type, due_date, created_at, updated_at
		 FROM milestones WHERE product_id = ? ORDER BY due_date, name`, productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var milestones []*model.Milestone
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, err
		}
		milestones = append(milestones, m)
	}
	return milestones, rows.Err()
}

func scanMilestone(sc scanner) (*model.Milestone, error) {
	var m model.Milestone
	var dueDate sql.NullString
	err := sc.Scan(&m.ID, &m.ProductID, &m.Name, &m.Description, &m.Type,
		&dueDate, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	m.DueDate = dueDate.String
	return &m, nil
}
