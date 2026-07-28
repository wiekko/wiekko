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
	GetActiveByProductID(ctx context.Context, productID string) (*model.TestingCampaign, error)
}

type testingCampaignStore struct {
	db *sql.DB
}

const testingCampaignColumns = `id, product_id, name, description, status,
	starts_at, ends_at, closed_at, closed_by, created_at, updated_at`

func scanTestingCampaign(sc scanner) (*model.TestingCampaign, error) {
	var tc model.TestingCampaign
	var startsAt, endsAt, closedAt, closedBy sql.NullString
	if err := sc.Scan(&tc.ID, &tc.ProductID, &tc.Name, &tc.Description, &tc.Status,
		&startsAt, &endsAt, &closedAt, &closedBy, &tc.CreatedAt, &tc.UpdatedAt); err != nil {
		return nil, err
	}
	tc.StartsAt = startsAt.String
	tc.EndsAt = endsAt.String
	tc.ClosedAt = closedAt.String
	tc.ClosedBy = closedBy.String
	return &tc, nil
}

func (s *testingCampaignStore) Create(ctx context.Context, tc *model.TestingCampaign) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO testing_campaigns (`+testingCampaignColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tc.ID, tc.ProductID, tc.Name, tc.Description, tc.Status,
		nullableText(tc.StartsAt), nullableText(tc.EndsAt),
		nullableText(tc.ClosedAt), nullableText(tc.ClosedBy),
		tc.CreatedAt, tc.UpdatedAt,
	)
	return err
}

func (s *testingCampaignStore) GetByID(ctx context.Context, id string) (*model.TestingCampaign, error) {
	tc, err := scanTestingCampaign(s.db.QueryRowContext(ctx,
		`SELECT `+testingCampaignColumns+` FROM testing_campaigns WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("testing campaign not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return tc, nil
}

func (s *testingCampaignStore) Update(ctx context.Context, tc *model.TestingCampaign) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE testing_campaigns
		 SET name = ?, description = ?, status = ?, starts_at = ?, ends_at = ?,
		     closed_at = ?, closed_by = ?, updated_at = ?
		 WHERE id = ?`,
		tc.Name, tc.Description, tc.Status,
		nullableText(tc.StartsAt), nullableText(tc.EndsAt),
		nullableText(tc.ClosedAt), nullableText(tc.ClosedBy),
		tc.UpdatedAt, tc.ID,
	)
	return err
}

func (s *testingCampaignStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM testing_campaigns WHERE id = ?`, id)
	return err
}

func (s *testingCampaignStore) ListByProductID(ctx context.Context, productID string) ([]*model.TestingCampaign, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+testingCampaignColumns+`
		 FROM testing_campaigns WHERE product_id = ? ORDER BY created_at DESC`, productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var campaigns []*model.TestingCampaign
	for rows.Next() {
		tc, err := scanTestingCampaign(rows)
		if err != nil {
			return nil, err
		}
		campaigns = append(campaigns, tc)
	}
	return campaigns, rows.Err()
}

// GetActiveByProductID returns the product's active campaign, or nil when there
// is none. A product may have at most one active campaign (FR-322).
func (s *testingCampaignStore) GetActiveByProductID(ctx context.Context, productID string) (*model.TestingCampaign, error) {
	tc, err := scanTestingCampaign(s.db.QueryRowContext(ctx,
		`SELECT `+testingCampaignColumns+`
		 FROM testing_campaigns WHERE product_id = ? AND status = ?`,
		productID, model.CampaignStatusActive))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return tc, nil
}
