package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
)

// CampaignResultStore defines data access operations for campaign results and
// the campaign tests they belong to.
//
// Callers name a result's test by its test case (CampaignResult.TestCaseID):
// creating results takes a copy of the test case into the campaign the first
// time it joins it, and reads report the live test case's ID alongside the
// copy's title, empty once the test case is gone.
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

	// ListTests returns a campaign's copies of its test cases.
	ListTests(ctx context.Context, campaignID string) ([]*model.CampaignTest, error)
	// RefreshDraftTests copies a test case's current content into the draft
	// campaigns holding it; active and closed campaigns keep theirs.
	RefreshDraftTests(ctx context.Context, testCaseID string) error
	// CampaignsWithTestCase and CampaignsWithEnvironment list the draft and
	// active campaigns a deletion would change, for locking them first.
	CampaignsWithTestCase(ctx context.Context, testCaseID string) ([]string, error)
	CampaignsWithEnvironment(ctx context.Context, environmentID string) ([]string, error)
	// DetachTestCase and DetachEnvironment prepare the deletion of a test case
	// or environment: draft campaigns drop it, active campaigns drop the
	// results still waiting to run (pending, retest) and keep the verdicts,
	// closed campaigns keep everything. The foreign keys then detach what is
	// kept.
	DetachTestCase(ctx context.Context, testCaseID string) error
	DetachEnvironment(ctx context.Context, environmentID string) error
}

type campaignResultStore struct {
	db *DB
}

const campaignResultSelect = `SELECT cr.id, cr.campaign_id, cr.campaign_test_id, ct.test_case_id, ct.title,
	cr.environment_id, cr.environment_name, cr.status, cr.defect_id, cr.executed_by, cr.executed_at, cr.updated_at
	FROM campaign_results cr JOIN campaign_tests ct ON ct.id = cr.campaign_test_id`

const campaignResultOrder = ` ORDER BY ct.title, ct.id, cr.environment_name, cr.id`

func scanCampaignResult(sc scanner) (*model.CampaignResult, error) {
	var cr model.CampaignResult
	var testCaseID, envID, envName, defectID, executedBy, executedAt sql.NullString
	if err := sc.Scan(&cr.ID, &cr.CampaignID, &cr.CampaignTestID, &testCaseID, &cr.TestTitle,
		&envID, &envName, &cr.Status, &defectID, &executedBy, &executedAt, &cr.UpdatedAt); err != nil {
		return nil, err
	}
	cr.TestCaseID = testCaseID.String
	cr.EnvironmentID = envID.String
	cr.EnvironmentName = envName.String
	cr.DefectID = defectID.String
	cr.ExecutedBy = executedBy.String
	cr.ExecutedAt = executedAt.String
	return &cr, nil
}

func (s *campaignResultStore) Create(ctx context.Context, cr *model.CampaignResult) error {
	return s.CreateBatch(ctx, []*model.CampaignResult{cr})
}

// CreateBatch seeds a campaign's result matrix in a single transaction, so a
// campaign never ends up half-populated (FR-94). Each test case joining the
// campaign is copied into it first.
func (s *campaignResultStore) CreateBatch(ctx context.Context, results []*model.CampaignResult) error {
	if len(results) == 0 {
		return nil
	}
	return s.db.InTx(ctx, func(ctx context.Context) error {
		tests := make(map[[2]string]string)
		for _, cr := range results {
			key := [2]string{cr.CampaignID, cr.TestCaseID}
			testID, ok := tests[key]
			if !ok {
				var err error
				if testID, err = s.campaignTest(ctx, cr.CampaignID, cr.TestCaseID); err != nil {
					return err
				}
				tests[key] = testID
			}
			cr.CampaignTestID = testID
			if _, err := s.db.ExecContext(ctx,
				`INSERT INTO campaign_results (id, campaign_id, campaign_test_id, environment_id, environment_name,
				 status, defect_id, executed_by, executed_at, updated_at)
				 VALUES (?, ?, ?, ?, (SELECT name FROM runtime_environments WHERE id = ?), ?, ?, ?, ?, ?)`,
				cr.ID, cr.CampaignID, testID, nullableText(cr.EnvironmentID), nullableText(cr.EnvironmentID),
				cr.Status, nullableText(cr.DefectID), nullableText(cr.ExecutedBy), nullableText(cr.ExecutedAt),
				cr.UpdatedAt,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// campaignTest returns the campaign's copy of a test case, taking the copy
// if the test case is new to the campaign.
func (s *campaignResultStore) campaignTest(ctx context.Context, campaignID, testCaseID string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM campaign_tests WHERE campaign_id = ? AND test_case_id = ?`,
		campaignID, testCaseID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id = uuid.New().String()
	err = requireChanged(s.db.ExecContext(ctx,
		`INSERT INTO campaign_tests (id, campaign_id, test_case_id, title, description, execution_mode, test_identifier, created_at)
		 SELECT ?, ?, id, title, description, execution_mode, COALESCE(test_identifier, ''), updated_at
		 FROM test_cases WHERE id = ?`,
		id, campaignID, testCaseID))
	if errors.Is(err, ErrNotChanged) {
		return "", fmt.Errorf("test case %s not found: %w", testCaseID, sql.ErrNoRows)
	}
	return id, err
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
	cr, err := scanCampaignResult(s.db.QueryRowContext(ctx, campaignResultSelect+` WHERE cr.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("campaign result not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *campaignResultStore) GetByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID, environmentID string) (*model.CampaignResult, error) {
	query := campaignResultSelect + ` WHERE cr.campaign_id = ? AND ct.test_case_id = ?`
	args := []any{campaignID, testCaseID}

	if environmentID == "" {
		query += ` AND cr.environment_id IS NULL AND cr.environment_name IS NULL`
	} else {
		query += ` AND cr.environment_id = ?`
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
	rows, err := s.db.QueryContext(ctx, campaignResultSelect+` WHERE `+where+campaignResultOrder, arg)
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
	return s.listBy(ctx, "cr.campaign_id = ?", campaignID)
}

func (s *campaignResultStore) ListByTestCaseID(ctx context.Context, testCaseID string) ([]*model.CampaignResult, error) {
	return s.listBy(ctx, "ct.test_case_id = ?", testCaseID)
}

func (s *campaignResultStore) ListByDefectID(ctx context.Context, defectID string) ([]*model.CampaignResult, error) {
	return s.listBy(ctx, "cr.defect_id = ?", defectID)
}

// DeleteByCampaignAndTestCase removes a test case from a campaign: its copy,
// and with it (cascade) its results.
func (s *campaignResultStore) DeleteByCampaignAndTestCase(ctx context.Context, campaignID, testCaseID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM campaign_tests WHERE campaign_id = ? AND test_case_id = ?`,
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

func (s *campaignResultStore) ListTests(ctx context.Context, campaignID string) ([]*model.CampaignTest, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, campaign_id, test_case_id, title, description, execution_mode, test_identifier, created_at
		 FROM campaign_tests WHERE campaign_id = ? ORDER BY title, id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var tests []*model.CampaignTest
	for rows.Next() {
		var t model.CampaignTest
		var testCaseID sql.NullString
		if err := rows.Scan(&t.ID, &t.CampaignID, &testCaseID, &t.Title, &t.Description,
			&t.ExecutionMode, &t.TestIdentifier, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.TestCaseID = testCaseID.String
		tests = append(tests, &t)
	}
	return tests, rows.Err()
}

func (s *campaignResultStore) RefreshDraftTests(ctx context.Context, testCaseID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE campaign_tests SET
		   title = (SELECT title FROM test_cases WHERE id = ?),
		   description = (SELECT description FROM test_cases WHERE id = ?),
		   execution_mode = (SELECT execution_mode FROM test_cases WHERE id = ?),
		   test_identifier = (SELECT COALESCE(test_identifier, '') FROM test_cases WHERE id = ?)
		 WHERE test_case_id = ?
		   AND campaign_id IN (SELECT id FROM testing_campaigns WHERE status = 'draft')`,
		testCaseID, testCaseID, testCaseID, testCaseID, testCaseID)
	return err
}

func (s *campaignResultStore) campaignIDs(ctx context.Context, query string, arg string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	// Sorted, so concurrent deletions take the campaign locks in one order.
	sort.Strings(ids)
	return ids, rows.Err()
}

func (s *campaignResultStore) CampaignsWithTestCase(ctx context.Context, testCaseID string) ([]string, error) {
	return s.campaignIDs(ctx,
		`SELECT DISTINCT c.id FROM campaign_tests ct JOIN testing_campaigns c ON c.id = ct.campaign_id
		 WHERE ct.test_case_id = ? AND c.status <> 'closed'`, testCaseID)
}

func (s *campaignResultStore) CampaignsWithEnvironment(ctx context.Context, environmentID string) ([]string, error) {
	return s.campaignIDs(ctx,
		`SELECT DISTINCT c.id FROM campaign_results cr JOIN testing_campaigns c ON c.id = cr.campaign_id
		 WHERE cr.environment_id = ? AND c.status <> 'closed'`, environmentID)
}

// openResults selects the results a deletion drops: everything in draft
// campaigns, the results without a verdict in active ones.
const openResults = `campaign_id IN (SELECT id FROM testing_campaigns WHERE status = 'draft')
	OR (campaign_id IN (SELECT id FROM testing_campaigns WHERE status = 'active') AND status IN ('pending', 'retest'))`

func (s *campaignResultStore) DetachTestCase(ctx context.Context, testCaseID string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM campaign_results
		 WHERE campaign_test_id IN (SELECT id FROM campaign_tests WHERE test_case_id = ?)
		   AND (`+openResults+`)`, testCaseID); err != nil {
		return err
	}
	return s.dropEmptyTests(ctx, `test_case_id = ?`, testCaseID)
}

func (s *campaignResultStore) DetachEnvironment(ctx context.Context, environmentID string) error {
	var tests []string
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT campaign_test_id FROM campaign_results WHERE environment_id = ?`, environmentID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			closeRows(rows)
			return err
		}
		tests = append(tests, id)
	}
	closeRows(rows)
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM campaign_results WHERE environment_id = ? AND (`+openResults+`)`, environmentID); err != nil {
		return err
	}
	for _, id := range tests {
		if err := s.dropEmptyTests(ctx, `id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// dropEmptyTests removes the matching campaign tests of draft and active
// campaigns that a detach left without results; a test without a single
// result would linger invisibly in the campaign's scope.
func (s *campaignResultStore) dropEmptyTests(ctx context.Context, where, arg string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM campaign_tests WHERE `+where+`
		   AND campaign_id IN (SELECT id FROM testing_campaigns WHERE status <> 'closed')
		   AND NOT EXISTS (SELECT 1 FROM campaign_results r WHERE r.campaign_test_id = campaign_tests.id)`, arg)
	return err
}
