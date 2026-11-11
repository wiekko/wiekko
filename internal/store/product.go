package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// ProductStore defines data access operations for products.
type ProductStore interface {
	Create(ctx context.Context, product *model.Product) error
	GetByID(ctx context.Context, id string) (*model.Product, error)
	Update(ctx context.Context, product *model.Product) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*model.Product, error)
	// ResetActivationToSprints sets the stored is_active of the product's
	// requirements, tasks and defects to whether they are planned into a
	// sprint: what activation means once a product switches to scrum (FR-318).
	ResetActivationToSprints(ctx context.Context, productID string) error
}

type productStore struct {
	db *DB
}

func (s *productStore) Create(ctx context.Context, product *model.Product) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO products (id, name, description, estimation_mode, workflow_mode, status_sync, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		product.ID, product.Name, product.Description,
		product.EstimationMode, product.WorkflowMode,
		product.StatusSync,
		product.CreatedAt, product.UpdatedAt,
	)
	return err
}

func (s *productStore) GetByID(ctx context.Context, id string) (*model.Product, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, estimation_mode, workflow_mode, status_sync, created_at, updated_at
		 FROM products WHERE id = ?`, id)
	p, err := scanProduct(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("product not found: %w", err)
	}
	return p, err
}

func (s *productStore) Update(ctx context.Context, product *model.Product) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE products SET name = ?, description = ?, estimation_mode = ?, workflow_mode = ?, status_sync = ?, updated_at = ?
		 WHERE id = ?`,
		product.Name, product.Description,
		product.EstimationMode, product.WorkflowMode,
		product.StatusSync,
		product.UpdatedAt, product.ID,
	)
	return err
}

func (s *productStore) ResetActivationToSprints(ctx context.Context, productID string) error {
	for _, table := range []string{"requirements", "defects"} {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE `+table+` SET is_active = (sprint_id IS NOT NULL) WHERE product_id = ?`, productID); err != nil {
			return fmt.Errorf("reset %s activation: %w", table, err)
		}
	}
	return nil
}

func (s *productStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM products WHERE id = ?`, id)
	return err
}

func (s *productStore) List(ctx context.Context) ([]*model.Product, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, estimation_mode, workflow_mode, status_sync, created_at, updated_at
		 FROM products ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var products []*model.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func scanProduct(sc scanner) (*model.Product, error) {
	var p model.Product
	err := sc.Scan(&p.ID, &p.Name, &p.Description,
		&p.EstimationMode, &p.WorkflowMode,
		&p.StatusSync,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
