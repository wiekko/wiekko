package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// TestCaseStore defines data access operations for test cases.
type TestCaseStore interface {
	Create(ctx context.Context, tc *model.TestCase) error
	GetByID(ctx context.Context, id string) (*model.TestCase, error)
	Update(ctx context.Context, tc *model.TestCase) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.TestCase, error)
	ListByCapabilityID(ctx context.Context, capabilityID string) ([]*model.TestCase, error)
	ListByComponentID(ctx context.Context, componentID string) ([]*model.TestCase, error)
	LinkRequirement(ctx context.Context, link *model.TestCaseRequirement) error
	UnlinkRequirement(ctx context.Context, testCaseID, requirementID string) error
	ListRequirementLinks(ctx context.Context, testCaseID string) ([]*model.TestCaseRequirement, error)
	ListTestCaseLinksByRequirement(ctx context.Context, requirementID string) ([]*model.TestCaseRequirement, error)
	AddEnvironment(ctx context.Context, id, testCaseID, environmentID string) error
	RemoveEnvironment(ctx context.Context, testCaseID, environmentID string) error
	ListEnvironments(ctx context.Context, testCaseID string) ([]string, error)
}

type testCaseStore struct {
	db *DB
}

func (s *testCaseStore) Create(ctx context.Context, tc *model.TestCase) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO test_cases (id, product_id, component_id, capability_id, title, description,
		 execution_mode, test_identifier, pyramid_level_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tc.ID, tc.ProductID, nullableText(tc.ComponentID), nullableText(tc.CapabilityID),
		tc.Title, tc.Description, tc.ExecutionMode,
		nullableText(tc.TestIdentifier), nullableText(tc.PyramidLevelID),
		tc.CreatedAt, tc.UpdatedAt,
	)
	return err
}

func (s *testCaseStore) GetByID(ctx context.Context, id string) (*model.TestCase, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+testCaseColumns+` FROM test_cases WHERE id = ?`, id)
	tc, err := scanTestCase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("test case not found: %w", err)
	}
	return tc, err
}

func (s *testCaseStore) Update(ctx context.Context, tc *model.TestCase) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE test_cases SET component_id = ?, capability_id = ?, title = ?, description = ?,
		 execution_mode = ?, test_identifier = ?, pyramid_level_id = ?, updated_at = ?
		 WHERE id = ?`,
		nullableText(tc.ComponentID), nullableText(tc.CapabilityID),
		tc.Title, tc.Description, tc.ExecutionMode,
		nullableText(tc.TestIdentifier), nullableText(tc.PyramidLevelID),
		tc.UpdatedAt, tc.ID,
	)
	return err
}

func (s *testCaseStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM test_cases WHERE id = ?`, id)
	return err
}

func (s *testCaseStore) ListByProductID(ctx context.Context, productID string) ([]*model.TestCase, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+testCaseColumns+` FROM test_cases WHERE product_id = ? ORDER BY title`,
		productID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var cases []*model.TestCase
	for rows.Next() {
		tc, err := scanTestCase(rows)
		if err != nil {
			return nil, err
		}
		cases = append(cases, tc)
	}
	return cases, rows.Err()
}

func (s *testCaseStore) ListByCapabilityID(ctx context.Context, capabilityID string) ([]*model.TestCase, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+testCaseColumns+` FROM test_cases WHERE capability_id = ? ORDER BY title`,
		capabilityID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var cases []*model.TestCase
	for rows.Next() {
		tc, err := scanTestCase(rows)
		if err != nil {
			return nil, err
		}
		cases = append(cases, tc)
	}
	return cases, rows.Err()
}

func (s *testCaseStore) ListByComponentID(ctx context.Context, componentID string) ([]*model.TestCase, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+testCaseColumns+` FROM test_cases WHERE component_id = ? ORDER BY title`,
		componentID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var cases []*model.TestCase
	for rows.Next() {
		tc, err := scanTestCase(rows)
		if err != nil {
			return nil, err
		}
		cases = append(cases, tc)
	}
	return cases, rows.Err()
}

func (s *testCaseStore) LinkRequirement(ctx context.Context, link *model.TestCaseRequirement) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO test_case_requirements (id, test_case_id, requirement_id, link_type, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		link.ID, link.TestCaseID, link.RequirementID, link.LinkType, link.CreatedAt,
	)
	return err
}

func (s *testCaseStore) UnlinkRequirement(ctx context.Context, testCaseID, requirementID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM test_case_requirements WHERE test_case_id = ? AND requirement_id = ?`,
		testCaseID, requirementID)
	return err
}

func (s *testCaseStore) ListRequirementLinks(ctx context.Context, testCaseID string) ([]*model.TestCaseRequirement, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, test_case_id, requirement_id, link_type, created_at
		 FROM test_case_requirements WHERE test_case_id = ? ORDER BY created_at`,
		testCaseID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var links []*model.TestCaseRequirement
	for rows.Next() {
		var l model.TestCaseRequirement
		if err := rows.Scan(&l.ID, &l.TestCaseID, &l.RequirementID, &l.LinkType, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}

func (s *testCaseStore) ListTestCaseLinksByRequirement(ctx context.Context, requirementID string) ([]*model.TestCaseRequirement, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, test_case_id, requirement_id, link_type, created_at
		 FROM test_case_requirements WHERE requirement_id = ? ORDER BY created_at`,
		requirementID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var links []*model.TestCaseRequirement
	for rows.Next() {
		var l model.TestCaseRequirement
		if err := rows.Scan(&l.ID, &l.TestCaseID, &l.RequirementID, &l.LinkType, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}

func (s *testCaseStore) AddEnvironment(ctx context.Context, id, testCaseID, environmentID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO test_case_environments (id, test_case_id, environment_id) VALUES (?, ?, ?)`,
		id, testCaseID, environmentID,
	)
	return err
}

func (s *testCaseStore) RemoveEnvironment(ctx context.Context, testCaseID, environmentID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM test_case_environments WHERE test_case_id = ? AND environment_id = ?`,
		testCaseID, environmentID)
	return err
}

func (s *testCaseStore) ListEnvironments(ctx context.Context, testCaseID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT environment_id FROM test_case_environments WHERE test_case_id = ?`,
		testCaseID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var envIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		envIDs = append(envIDs, id)
	}
	return envIDs, rows.Err()
}

const testCaseColumns = `id, product_id, component_id, capability_id, title, description,
	execution_mode, test_identifier, pyramid_level_id, created_at, updated_at`

func scanTestCase(sc scanner) (*model.TestCase, error) {
	var tc model.TestCase
	var componentID, capabilityID, testIdentifier, pyramidLevelID sql.NullString
	err := sc.Scan(&tc.ID, &tc.ProductID, &componentID, &capabilityID,
		&tc.Title, &tc.Description, &tc.ExecutionMode,
		&testIdentifier, &pyramidLevelID, &tc.CreatedAt, &tc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	tc.ComponentID = componentID.String
	tc.CapabilityID = capabilityID.String
	tc.TestIdentifier = testIdentifier.String
	tc.PyramidLevelID = pyramidLevelID.String
	return &tc, nil
}
