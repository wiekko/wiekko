package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// ProductRoleStore defines data access operations for product roles.
type ProductRoleStore interface {
	Create(ctx context.Context, role *model.ProductRole) error
	GetByUserAndProduct(ctx context.Context, userID, productID string) (*model.ProductRole, error)
	Update(ctx context.Context, role *model.ProductRole) error
	Delete(ctx context.Context, userID, productID string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.ProductRole, error)
	ListByUserID(ctx context.Context, userID string) ([]*model.ProductRole, error)
}

type productRoleStore struct {
	db *sql.DB
}

func (s *productRoleStore) Create(ctx context.Context, role *model.ProductRole) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO product_roles (id, user_id, product_id, role, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		role.ID, role.UserID, role.ProductID, role.Role, role.CreatedAt,
	)
	return err
}

func (s *productRoleStore) GetByUserAndProduct(ctx context.Context, userID, productID string) (*model.ProductRole, error) {
	var r model.ProductRole
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, product_id, role, created_at
		 FROM product_roles WHERE user_id = ? AND product_id = ?`,
		userID, productID,
	).Scan(&r.ID, &r.UserID, &r.ProductID, &r.Role, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("product role not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *productRoleStore) Update(ctx context.Context, role *model.ProductRole) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE product_roles SET role = ? WHERE user_id = ? AND product_id = ?`,
		role.Role, role.UserID, role.ProductID,
	)
	return err
}

func (s *productRoleStore) Delete(ctx context.Context, userID, productID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM product_roles WHERE user_id = ? AND product_id = ?`,
		userID, productID)
	return err
}

func (s *productRoleStore) ListByProductID(ctx context.Context, productID string) ([]*model.ProductRole, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, product_id, role, created_at
		 FROM product_roles WHERE product_id = ? ORDER BY created_at`,
		productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var roles []*model.ProductRole
	for rows.Next() {
		var r model.ProductRole
		if err := rows.Scan(&r.ID, &r.UserID, &r.ProductID, &r.Role, &r.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, &r)
	}
	return roles, rows.Err()
}

func (s *productRoleStore) ListByUserID(ctx context.Context, userID string) ([]*model.ProductRole, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, product_id, role, created_at
		 FROM product_roles WHERE user_id = ? ORDER BY created_at`,
		userID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var roles []*model.ProductRole
	for rows.Next() {
		var r model.ProductRole
		if err := rows.Scan(&r.ID, &r.UserID, &r.ProductID, &r.Role, &r.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, &r)
	}
	return roles, rows.Err()
}
