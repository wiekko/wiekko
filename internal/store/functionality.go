package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// FunctionalityStore defines data access operations for functionalities.
type FunctionalityStore interface {
	Create(ctx context.Context, f *model.Functionality) error
	GetByID(ctx context.Context, id string) (*model.Functionality, error)
	Update(ctx context.Context, f *model.Functionality) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.Functionality, error)
	ListByComponentID(ctx context.Context, componentID string) ([]*model.Functionality, error)
}

type functionalityStore struct {
	db *sql.DB
}

func (s *functionalityStore) Create(ctx context.Context, f *model.Functionality) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO functionalities (id, product_id, component_id, name, description, milestone_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.ProductID, nullableText(f.ComponentID),
		f.Name, f.Description, nullableText(f.MilestoneID),
		f.CreatedAt, f.UpdatedAt,
	)
	return err
}

func (s *functionalityStore) GetByID(ctx context.Context, id string) (*model.Functionality, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, component_id, name, description, milestone_id, created_at, updated_at
		 FROM functionalities WHERE id = ?`, id)
	f, err := scanFunctionality(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("functionality not found: %w", err)
	}
	return f, err
}

func (s *functionalityStore) Update(ctx context.Context, f *model.Functionality) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE functionalities SET component_id = ?, name = ?, description = ?, milestone_id = ?, updated_at = ?
		 WHERE id = ?`,
		nullableText(f.ComponentID), f.Name, f.Description,
		nullableText(f.MilestoneID), f.UpdatedAt, f.ID,
	)
	return err
}

func (s *functionalityStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM functionalities WHERE id = ?`, id)
	return err
}

func (s *functionalityStore) ListByProductID(ctx context.Context, productID string) ([]*model.Functionality, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, component_id, name, description, milestone_id, created_at, updated_at
		 FROM functionalities WHERE product_id = ? ORDER BY name`, productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var funcs []*model.Functionality
	for rows.Next() {
		f, err := scanFunctionality(rows)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, f)
	}
	return funcs, rows.Err()
}

func (s *functionalityStore) ListByComponentID(ctx context.Context, componentID string) ([]*model.Functionality, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, component_id, name, description, milestone_id, created_at, updated_at
		 FROM functionalities WHERE component_id = ? ORDER BY name`, componentID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var funcs []*model.Functionality
	for rows.Next() {
		f, err := scanFunctionality(rows)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, f)
	}
	return funcs, rows.Err()
}

func scanFunctionality(sc scanner) (*model.Functionality, error) {
	var f model.Functionality
	var componentID, milestoneID sql.NullString
	err := sc.Scan(&f.ID, &f.ProductID, &componentID, &f.Name, &f.Description,
		&milestoneID, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	f.ComponentID = componentID.String
	f.MilestoneID = milestoneID.String
	return &f, nil
}
