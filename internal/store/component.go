package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// ComponentStore defines data access operations for components.
type ComponentStore interface {
	Create(ctx context.Context, c *model.Component) error
	GetByID(ctx context.Context, id string) (*model.Component, error)
	Update(ctx context.Context, c *model.Component) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.Component, error)
	ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Component, error)
}

type componentStore struct {
	db *sql.DB
}

func (s *componentStore) Create(ctx context.Context, c *model.Component) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO components (id, product_id, name, description, milestone_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProductID, c.Name, c.Description,
		nullableText(c.MilestoneID), c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (s *componentStore) GetByID(ctx context.Context, id string) (*model.Component, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, name, description, milestone_id, created_at, updated_at
		 FROM components WHERE id = ?`, id)
	c, err := scanComponent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("component not found: %w", err)
	}
	return c, err
}

func (s *componentStore) Update(ctx context.Context, c *model.Component) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE components SET name = ?, description = ?, milestone_id = ?, updated_at = ?
		 WHERE id = ?`,
		c.Name, c.Description, nullableText(c.MilestoneID),
		c.UpdatedAt, c.ID,
	)
	return err
}

func (s *componentStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM components WHERE id = ?`, id)
	return err
}

func (s *componentStore) ListByProductID(ctx context.Context, productID string) ([]*model.Component, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, name, description, milestone_id, created_at, updated_at
		 FROM components WHERE product_id = ? ORDER BY name`, productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var components []*model.Component
	for rows.Next() {
		c, err := scanComponent(rows)
		if err != nil {
			return nil, err
		}
		components = append(components, c)
	}
	return components, rows.Err()
}

func (s *componentStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Component, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, name, description, milestone_id, created_at, updated_at
		 FROM components WHERE milestone_id = ? ORDER BY name`, milestoneID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var components []*model.Component
	for rows.Next() {
		c, err := scanComponent(rows)
		if err != nil {
			return nil, err
		}
		components = append(components, c)
	}
	return components, rows.Err()
}

func scanComponent(sc scanner) (*model.Component, error) {
	var c model.Component
	var milestoneID sql.NullString
	err := sc.Scan(&c.ID, &c.ProductID, &c.Name, &c.Description,
		&milestoneID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.MilestoneID = milestoneID.String
	return &c, nil
}
