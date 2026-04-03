package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// TestingCampaignStore defines data access operations for testing campaigns.
type TestingCampaignStore interface {
	Create(ctx context.Context, tc *model.TestingCampaign) error
	GetByID(ctx context.Context, id string) (*model.TestingCampaign, error)
	Update(ctx context.Context, tc *model.TestingCampaign) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.TestingCampaign, error)
}

type testingCampaignStore struct {
	db *sql.DB
}

func (s *testingCampaignStore) Create(ctx context.Context, tc *model.TestingCampaign) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO testing_campaigns (id, product_id, name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		tc.ID, tc.ProductID, tc.Name, tc.CreatedAt, tc.UpdatedAt,
	)
	return err
}

func (s *testingCampaignStore) GetByID(ctx context.Context, id string) (*model.TestingCampaign, error) {
	var tc model.TestingCampaign
	err := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, name, created_at, updated_at
		 FROM testing_campaigns WHERE id = ?`, id,
	).Scan(&tc.ID, &tc.ProductID, &tc.Name, &tc.CreatedAt, &tc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("testing campaign not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &tc, nil
}

func (s *testingCampaignStore) Update(ctx context.Context, tc *model.TestingCampaign) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE testing_campaigns SET name = ?, updated_at = ? WHERE id = ?`,
		tc.Name, tc.UpdatedAt, tc.ID,
	)
	return err
}

func (s *testingCampaignStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM testing_campaigns WHERE id = ?`, id)
	return err
}

func (s *testingCampaignStore) ListByProductID(ctx context.Context, productID string) ([]*model.TestingCampaign, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, name, created_at, updated_at
		 FROM testing_campaigns WHERE product_id = ? ORDER BY created_at DESC`, productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var campaigns []*model.TestingCampaign
	for rows.Next() {
		var tc model.TestingCampaign
		if err := rows.Scan(&tc.ID, &tc.ProductID, &tc.Name, &tc.CreatedAt, &tc.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, &tc)
	}
	return campaigns, rows.Err()
}
