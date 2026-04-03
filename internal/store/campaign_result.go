package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// CampaignResultStore defines data access operations for campaign results.
type CampaignResultStore interface {
	Create(ctx context.Context, cr *model.CampaignResult) error
	Update(ctx context.Context, cr *model.CampaignResult) error
	GetByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID, environmentID string) (*model.CampaignResult, error)
	ListByCampaignID(ctx context.Context, campaignID string) ([]*model.CampaignResult, error)
}

type campaignResultStore struct {
	db *sql.DB
}

func (s *campaignResultStore) Create(ctx context.Context, cr *model.CampaignResult) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO campaign_results (id, campaign_id, test_case_id, environment_id, status, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		cr.ID, cr.CampaignID, cr.TestCaseID,
		nullableText(cr.EnvironmentID), cr.Status, cr.UpdatedAt,
	)
	return err
}

func (s *campaignResultStore) Update(ctx context.Context, cr *model.CampaignResult) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE campaign_results SET status = ?, updated_at = ? WHERE id = ?`,
		cr.Status, cr.UpdatedAt, cr.ID,
	)
	return err
}

func (s *campaignResultStore) GetByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID, environmentID string) (*model.CampaignResult, error) {
	var cr model.CampaignResult
	var envID sql.NullString

	query := `SELECT id, campaign_id, test_case_id, environment_id, status, updated_at
		 FROM campaign_results WHERE campaign_id = ? AND test_case_id = ?`
	args := []any{campaignID, testCaseID}

	if environmentID == "" {
		query += ` AND environment_id IS NULL`
	} else {
		query += ` AND environment_id = ?`
		args = append(args, environmentID)
	}

	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&cr.ID, &cr.CampaignID, &cr.TestCaseID, &envID, &cr.Status, &cr.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("campaign result not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	cr.EnvironmentID = envID.String
	return &cr, nil
}

func (s *campaignResultStore) ListByCampaignID(ctx context.Context, campaignID string) ([]*model.CampaignResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, campaign_id, test_case_id, environment_id, status, updated_at
		 FROM campaign_results WHERE campaign_id = ? ORDER BY test_case_id, environment_id`,
		campaignID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var results []*model.CampaignResult
	for rows.Next() {
		var cr model.CampaignResult
		var envID sql.NullString
		if err := rows.Scan(&cr.ID, &cr.CampaignID, &cr.TestCaseID, &envID, &cr.Status, &cr.UpdatedAt); err != nil {
			return nil, err
		}
		cr.EnvironmentID = envID.String
		results = append(results, &cr)
	}
	return results, rows.Err()
}
