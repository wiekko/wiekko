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
	CreateBatch(ctx context.Context, results []*model.CampaignResult) error
	Update(ctx context.Context, cr *model.CampaignResult) error
	GetByID(ctx context.Context, id string) (*model.CampaignResult, error)
	GetByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID, environmentID string) (*model.CampaignResult, error)
	ListByCampaignID(ctx context.Context, campaignID string) ([]*model.CampaignResult, error)
	ListByTestCaseID(ctx context.Context, testCaseID string) ([]*model.CampaignResult, error)
	ListByDefectID(ctx context.Context, defectID string) ([]*model.CampaignResult, error)
	DeleteByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID string) error
	CountsByProductID(ctx context.Context, productID string) (map[string]map[string]int, error)
}

type campaignResultStore struct {
	db *sql.DB
}

const campaignResultColumns = `id, campaign_id, test_case_id, environment_id, status,
	defect_id, executed_by, executed_at, updated_at`

func scanCampaignResult(sc scanner) (*model.CampaignResult, error) {
	var cr model.CampaignResult
	var envID, defectID, executedBy, executedAt sql.NullString
	if err := sc.Scan(&cr.ID, &cr.CampaignID, &cr.TestCaseID, &envID, &cr.Status,
		&defectID, &executedBy, &executedAt, &cr.UpdatedAt); err != nil {
		return nil, err
	}
	cr.EnvironmentID = envID.String
	cr.DefectID = defectID.String
	cr.ExecutedBy = executedBy.String
	cr.ExecutedAt = executedAt.String
	return &cr, nil
}

const insertCampaignResult = `INSERT INTO campaign_results (` + campaignResultColumns + `)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

func campaignResultArgs(cr *model.CampaignResult) []any {
	return []any{
		cr.ID, cr.CampaignID, cr.TestCaseID, nullableText(cr.EnvironmentID), cr.Status,
		nullableText(cr.DefectID), nullableText(cr.ExecutedBy), nullableText(cr.ExecutedAt),
		cr.UpdatedAt,
	}
}

func (s *campaignResultStore) Create(ctx context.Context, cr *model.CampaignResult) error {
	_, err := s.db.ExecContext(ctx, insertCampaignResult, campaignResultArgs(cr)...)
	return err
}

// CreateBatch seeds a campaign's result matrix in a single transaction, so a
// campaign never ends up half-populated (FR-321).
func (s *campaignResultStore) CreateBatch(ctx context.Context, results []*model.CampaignResult) error {
	if len(results) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, insertCampaignResult)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, cr := range results {
		if _, err := stmt.ExecContext(ctx, campaignResultArgs(cr)...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *campaignResultStore) Update(ctx context.Context, cr *model.CampaignResult) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE campaign_results
		 SET status = ?, defect_id = ?, executed_by = ?, executed_at = ?, updated_at = ?
		 WHERE id = ?`,
		cr.Status, nullableText(cr.DefectID), nullableText(cr.ExecutedBy),
		nullableText(cr.ExecutedAt), cr.UpdatedAt, cr.ID,
	)
	return err
}

func (s *campaignResultStore) GetByID(ctx context.Context, id string) (*model.CampaignResult, error) {
	cr, err := scanCampaignResult(s.db.QueryRowContext(ctx,
		`SELECT `+campaignResultColumns+` FROM campaign_results WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("campaign result not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *campaignResultStore) GetByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID, environmentID string) (*model.CampaignResult, error) {
	query := `SELECT ` + campaignResultColumns + `
		 FROM campaign_results WHERE campaign_id = ? AND test_case_id = ?`
	args := []any{campaignID, testCaseID}

	if environmentID == "" {
		query += ` AND environment_id IS NULL`
	} else {
		query += ` AND environment_id = ?`
		args = append(args, environmentID)
	}

	cr, err := scanCampaignResult(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("campaign result not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *campaignResultStore) listBy(ctx context.Context, where string, arg string) ([]*model.CampaignResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+campaignResultColumns+` FROM campaign_results WHERE `+where+
			` ORDER BY test_case_id, environment_id`, arg)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var results []*model.CampaignResult
	for rows.Next() {
		cr, err := scanCampaignResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, cr)
	}
	return results, rows.Err()
}

func (s *campaignResultStore) ListByCampaignID(ctx context.Context, campaignID string) ([]*model.CampaignResult, error) {
	return s.listBy(ctx, "campaign_id = ?", campaignID)
}

func (s *campaignResultStore) ListByTestCaseID(ctx context.Context, testCaseID string) ([]*model.CampaignResult, error) {
	return s.listBy(ctx, "test_case_id = ?", testCaseID)
}

func (s *campaignResultStore) ListByDefectID(ctx context.Context, defectID string) ([]*model.CampaignResult, error) {
	return s.listBy(ctx, "defect_id = ?", defectID)
}

func (s *campaignResultStore) DeleteByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM campaign_results WHERE campaign_id = ? AND test_case_id = ?`,
		campaignID, testCaseID)
	return err
}

// CountsByProductID returns result counts per campaign and status for a whole
// product in one query, so the campaign list can show progress without an N+1.
func (s *campaignResultStore) CountsByProductID(ctx context.Context, productID string) (map[string]map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cr.campaign_id, cr.status, COUNT(*)
		 FROM campaign_results cr
		 JOIN testing_campaigns tc ON tc.id = cr.campaign_id
		 WHERE tc.product_id = ?
		 GROUP BY cr.campaign_id, cr.status`, productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	counts := make(map[string]map[string]int)
	for rows.Next() {
		var campaignID, status string
		var n int
		if err := rows.Scan(&campaignID, &status, &n); err != nil {
			return nil, err
		}
		if counts[campaignID] == nil {
			counts[campaignID] = make(map[string]int)
		}
		counts[campaignID][status] = n
	}
	return counts, rows.Err()
}
