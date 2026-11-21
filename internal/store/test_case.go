package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// TestCaseFilter selects the test cases of the test case list (UR-146). Empty
// fields match everything; FilterNone in a reference field selects test cases
// without one.
type TestCaseFilter struct {
	ComponentID    string
	CapabilityID   string
	PyramidLevelID string
	ExecutionMode  string
}

// TestCaseSortColumns maps the sort keys of the test case list onto SQL
// expressions. Pyramid levels sort by their configured position, so unit tests
// come before end-to-end tests rather than alphabetically (UC-74).
var TestCaseSortColumns = map[string]string{
	"title":         `LOWER(t.title)`,
	"component":     `LOWER(COALESCE(co.name, ''))`,
	"capability":    `LOWER(COALESCE(ca.name, ''))`,
	"pyramid_level": `COALESCE(pl.position, 9999)`,
	"mode":          `t.execution_mode`,
	"created":       `t.created_at`,
}

// testCaseListFrom joins the tables whose names the list shows, sorts and
// searches on; all joins are outer, as every reference is optional.
const testCaseListFrom = ` FROM test_cases t
	LEFT JOIN components co ON t.component_id = co.id
	LEFT JOIN capabilities ca ON t.capability_id = ca.id
	LEFT JOIN pyramid_levels pl ON t.pyramid_level_id = pl.id`

// testCaseListWhere builds the WHERE clause shared by the page query and its
// count.
func testCaseListWhere(productID string, filter TestCaseFilter, search string) (string, []any) {
	where := ` WHERE t.product_id = ?`
	args := []any{productID}
	where, args = optionalRefWhere(where, args, "t.component_id", filter.ComponentID)
	where, args = optionalRefWhere(where, args, "t.capability_id", filter.CapabilityID)
	where, args = optionalRefWhere(where, args, "t.pyramid_level_id", filter.PyramidLevelID)
	if filter.ExecutionMode != "" {
		where += ` AND t.execution_mode = ?`
		args = append(args, filter.ExecutionMode)
	}
	// The search spans the text columns: title, test identifier (how automatic
	// tests are referred to), component, capability and pyramid level (UC-73).
	if search != "" {
		pattern := searchPattern(search)
		like := ` LIKE ? ESCAPE '` + LikeEscape + `'`
		where += ` AND (LOWER(t.title)` + like +
			` OR LOWER(COALESCE(t.test_identifier, ''))` + like +
			` OR LOWER(COALESCE(co.name, ''))` + like +
			` OR LOWER(COALESCE(ca.name, ''))` + like +
			` OR LOWER(COALESCE(pl.name, ''))` + like + `)`
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	return where, args
}

// TestCaseStore defines data access operations for test cases.
type TestCaseStore interface {
	Create(ctx context.Context, tc *model.TestCase) error
	GetByID(ctx context.Context, id string) (*model.TestCase, error)
	Update(ctx context.Context, tc *model.TestCase) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.TestCase, error)
	// ListPageByProductID lists one page of the product's test cases, sorted,
	// searched and filtered (UR-146); CountByProductID counts what the same
	// filter and search match.
	ListPageByProductID(ctx context.Context, productID string, filter TestCaseFilter, params ListParams) ([]*model.TestCase, error)
	CountByProductID(ctx context.Context, productID string, filter TestCaseFilter, search string) (int, error)
	ListByCapabilityID(ctx context.Context, capabilityID string) ([]*model.TestCase, error)
	ListByComponentID(ctx context.Context, componentID string) ([]*model.TestCase, error)
	LinkRequirement(ctx context.Context, link *model.TestCaseRequirement) error
	UnlinkRequirement(ctx context.Context, testCaseID, requirementID string) error
	ListRequirementLinks(ctx context.Context, testCaseID string) ([]*model.TestCaseRequirement, error)
	ListTestCaseLinksByRequirement(ctx context.Context, requirementID string) ([]*model.TestCaseRequirement, error)
	// LinkConstraint links a test case to a constraint (FR-150); linking a
	// linked pair again changes nothing.
	LinkConstraint(ctx context.Context, link *model.TestCaseConstraint) error
	UnlinkConstraint(ctx context.Context, testCaseID, constraintID string) error
	// ListConstraintLinks and ListLinksByConstraint list the links of a test
	// case and of a constraint, oldest first.
	ListConstraintLinks(ctx context.Context, testCaseID string) ([]*model.TestCaseConstraint, error)
	ListLinksByConstraint(ctx context.Context, constraintID string) ([]*model.TestCaseConstraint, error)
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

func (s *testCaseStore) ListPageByProductID(ctx context.Context, productID string, filter TestCaseFilter, params ListParams) ([]*model.TestCase, error) {
	where, args := testCaseListWhere(productID, filter, params.Search)
	query := `SELECT ` + testCaseColumnsQualified + testCaseListFrom + where
	query += orderClause(params, TestCaseSortColumns, `LOWER(t.title)`, `LOWER(t.title), t.id`)
	limit, limitArgs := limitClause(params)
	query += limit
	args = append(args, limitArgs...)

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *testCaseStore) CountByProductID(ctx context.Context, productID string, filter TestCaseFilter, search string) (int, error) {
	where, args := testCaseListWhere(productID, filter, search)
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+testCaseListFrom+where, args...).Scan(&total)
	return total, err
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
		 FROM test_case_requirements WHERE test_case_id = ? ORDER BY created_at, id`,
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
		 FROM test_case_requirements WHERE requirement_id = ? ORDER BY created_at, id`,
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

func (s *testCaseStore) LinkConstraint(ctx context.Context, link *model.TestCaseConstraint) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO test_case_constraints (id, test_case_id, constraint_id, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (test_case_id, constraint_id) DO NOTHING`,
		link.ID, link.TestCaseID, link.ConstraintID, link.CreatedAt,
	)
	return err
}

func (s *testCaseStore) UnlinkConstraint(ctx context.Context, testCaseID, constraintID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM test_case_constraints WHERE test_case_id = ? AND constraint_id = ?`,
		testCaseID, constraintID)
	return err
}

func (s *testCaseStore) ListConstraintLinks(ctx context.Context, testCaseID string) ([]*model.TestCaseConstraint, error) {
	return s.constraintLinks(ctx, "test_case_id", testCaseID)
}

func (s *testCaseStore) ListLinksByConstraint(ctx context.Context, constraintID string) ([]*model.TestCaseConstraint, error) {
	return s.constraintLinks(ctx, "constraint_id", constraintID)
}

// constraintLinks lists the test case to constraint links whose column (a
// constant, never user input) holds id.
func (s *testCaseStore) constraintLinks(ctx context.Context, column, id string) ([]*model.TestCaseConstraint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, test_case_id, constraint_id, created_at
		 FROM test_case_constraints WHERE `+column+` = ? ORDER BY created_at, id`,
		id)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var links []*model.TestCaseConstraint
	for rows.Next() {
		var l model.TestCaseConstraint
		if err := rows.Scan(&l.ID, &l.TestCaseID, &l.ConstraintID, &l.CreatedAt); err != nil {
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

// testCaseColumnsQualified is testCaseColumns on the aliased table t.
const testCaseColumnsQualified = `t.id, t.product_id, t.component_id, t.capability_id, t.title, t.description,
	t.execution_mode, t.test_identifier, t.pyramid_level_id, t.created_at, t.updated_at`

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
