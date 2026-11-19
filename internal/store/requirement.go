package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// RequirementFilter controls which requirements are returned by ListByProductID.
type RequirementFilter struct {
	Kind       string // empty = all kinds (requirements and tasks)
	CategoryID string // empty = all categories
	StatusID   string // empty = all statuses
	SprintID   string // empty = all sprints; FilterNone = no sprint
	// MilestoneID selects one milestone; FilterNone selects items without one.
	MilestoneID string
	// Archived selects archived items (FR-155); the zero value includes
	// them, so only the views that hide archived items have to say so.
	Archived ArchivedFilter
}

// RequirementStore defines data access operations for requirements.
type RequirementStore interface {
	Create(ctx context.Context, r *model.Requirement) error
	GetByID(ctx context.Context, id string) (*model.Requirement, error)
	Update(ctx context.Context, r *model.Requirement) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string, filter RequirementFilter) ([]*model.Requirement, error)
	// ListPageByProductID is ListByProductID with sorting, search and paging
	// (UR-144); CountByProductID counts what the same filter and search match.
	ListPageByProductID(ctx context.Context, productID string, filter RequirementFilter, params ListParams) ([]*model.Requirement, error)
	CountByProductID(ctx context.Context, productID string, filter RequirementFilter, search string) (int, error)
	ListByParentID(ctx context.Context, parentID string) ([]*model.Requirement, error)
	// SetParent moves an item under parentID ("" for top level).
	SetParent(ctx context.Context, id, parentID, updatedAt string) error
	ListBySprintID(ctx context.Context, sprintID string) ([]*model.Requirement, error)
	ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Requirement, error)
	// GetBySequence returns the product's requirement or task with the given
	// sequence number; the number is unique per product across both kinds.
	GetBySequence(ctx context.Context, productID string, seq int) (*model.Requirement, error)
	NextSequenceNumber(ctx context.Context, productID string) (int, error)
	CountByCategoryID(ctx context.Context, categoryID string) (int, error)
	Assign(ctx context.Context, a *model.Assignment) error
	Unassign(ctx context.Context, requirementID, userID string) error
	ListAssignees(ctx context.Context, requirementID string) ([]*model.Assignment, error)
	// SetArchivedAt archives the item at archivedAt, or unarchives it when
	// archivedAt is empty (FR-155).
	SetArchivedAt(ctx context.Context, id, archivedAt, updatedAt string) error
}

type requirementStore struct {
	db *DB
}

func (s *requirementStore) Create(ctx context.Context, r *model.Requirement) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO requirements (id, product_id, kind, parent_id, category_id, sequence_number, title, description,
		 definition_of_done, status_id, milestone_id, sprint_id, estimation, priority,
		 is_coarse, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProductID, r.Kind, nullableText(r.ParentID), r.CategoryID, r.SequenceNumber,
		r.Title, r.Description, r.DefinitionOfDone,
		nullableText(r.StatusID), nullableText(r.MilestoneID), nullableText(r.SprintID),
		nullableText(r.Estimation), r.Priority,
		r.IsCoarse, r.IsActive,
		r.CreatedAt, r.UpdatedAt,
	)
	return err
}

func (s *requirementStore) GetByID(ctx context.Context, id string) (*model.Requirement, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+requirementColumns+` FROM requirements WHERE id = ?`, id)
	r, err := scanRequirement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("requirement not found: %w", err)
	}
	return r, err
}

func (s *requirementStore) GetBySequence(ctx context.Context, productID string, seq int) (*model.Requirement, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+requirementColumns+` FROM requirements WHERE product_id = ? AND sequence_number = ?`, productID, seq)
	r, err := scanRequirement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("requirement not found: %w", err)
	}
	return r, err
}

func (s *requirementStore) Update(ctx context.Context, r *model.Requirement) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE requirements SET parent_id = ?, category_id = ?, title = ?, description = ?,
		 definition_of_done = ?, status_id = ?, milestone_id = ?, sprint_id = ?,
		 estimation = ?, priority = ?, is_coarse = ?, is_active = ?, updated_at = ?
		 WHERE id = ?`,
		nullableText(r.ParentID), r.CategoryID, r.Title, r.Description,
		r.DefinitionOfDone, nullableText(r.StatusID),
		nullableText(r.MilestoneID), nullableText(r.SprintID),
		nullableText(r.Estimation), r.Priority,
		r.IsCoarse, r.IsActive,
		r.UpdatedAt, r.ID,
	)
	return err
}

func (s *requirementStore) SetArchivedAt(ctx context.Context, id, archivedAt, updatedAt string) error {
	return requireChanged(s.db.ExecContext(ctx,
		`UPDATE requirements SET archived_at = ?, updated_at = ? WHERE id = ?`,
		nullableText(archivedAt), updatedAt, id))
}

func (s *requirementStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM requirements WHERE id = ?`, id)
	return err
}

// RequirementSortColumns maps the sort keys of the requirement and task lists
// onto SQL expressions (UR-144). Category and status sort by their configured
// position, not their name, so the order matches the one set up in the
// product settings and on the board (UC-74).
var RequirementSortColumns = map[string]string{
	"identifier": `r.sequence_number`,
	"title":      `LOWER(r.title)`,
	"category":   `COALESCE(c.position, 9999)`,
	"status":     `COALESCE(s.position, 9999)`,
	"priority":   priorityRank("r.priority"),
	"created":    `r.created_at`,
}

// requirementDefaultOrder is itemOrder on the aliased table.
var requirementDefaultOrder = priorityRank("r.priority") + `, r.created_at, r.sequence_number`

// requirementListFrom joins the tables whose values the list sorts or searches
// on. Both joins are outer so an item never drops out of the list.
const requirementListFrom = ` FROM requirements r
	LEFT JOIN categories c ON r.category_id = c.id
	LEFT JOIN statuses s ON r.status_id = s.id`

// requirementListWhere builds the WHERE clause shared by the page query and
// its count, so the two cannot drift apart.
func requirementListWhere(productID string, filter RequirementFilter, search string) (string, []any) {
	where := ` WHERE r.product_id = ?`
	args := []any{productID}
	if filter.Kind != "" {
		where += ` AND r.kind = ?`
		args = append(args, filter.Kind)
	}
	if filter.CategoryID != "" {
		where += ` AND r.category_id = ?`
		args = append(args, filter.CategoryID)
	}
	if filter.StatusID != "" {
		where += ` AND r.status_id = ?`
		args = append(args, filter.StatusID)
	}
	where, args = optionalRefWhere(where, args, "r.sprint_id", filter.SprintID)
	where, args = optionalRefWhere(where, args, "r.milestone_id", filter.MilestoneID)
	where += filter.Archived.where("r.archived_at")
	// The search spans the text columns the list shows: the identifier, the
	// title, the category and the status. Priority and dates are left out, as
	// a substring match on them does not mean what the user sees (UC-73).
	if search != "" {
		pattern := searchPattern(search)
		like := ` LIKE ? ESCAPE '` + LikeEscape + `'`
		where += ` AND (LOWER(r.title)` + like +
			` OR LOWER(COALESCE(c.prefix, '') || '-' || CAST(r.sequence_number AS TEXT))` + like +
			` OR LOWER(COALESCE(c.name, ''))` + like +
			` OR LOWER(COALESCE(s.name, ''))` + like + `)`
		args = append(args, pattern, pattern, pattern, pattern)
	}
	return where, args
}

func (s *requirementStore) ListByProductID(ctx context.Context, productID string, filter RequirementFilter) ([]*model.Requirement, error) {
	return s.ListPageByProductID(ctx, productID, filter, ListParams{})
}

func (s *requirementStore) ListPageByProductID(ctx context.Context, productID string, filter RequirementFilter, params ListParams) ([]*model.Requirement, error) {
	where, args := requirementListWhere(productID, filter, params.Search)
	query := `SELECT ` + requirementColumnsQualified + requirementListFrom + where
	query += orderClause(params, RequirementSortColumns, requirementDefaultOrder, `r.sequence_number, r.id`)
	limit, limitArgs := limitClause(params)
	query += limit
	args = append(args, limitArgs...)
	return s.listRequirements(ctx, query, args...)
}

func (s *requirementStore) CountByProductID(ctx context.Context, productID string, filter RequirementFilter, search string) (int, error) {
	where, args := requirementListWhere(productID, filter, search)
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+requirementListFrom+where, args...).Scan(&total)
	return total, err
}

func (s *requirementStore) ListByParentID(ctx context.Context, parentID string) ([]*model.Requirement, error) {
	return s.listRequirements(ctx,
		`SELECT `+requirementColumns+` FROM requirements WHERE parent_id = ? ORDER BY `+itemOrder,
		parentID)
}

func (s *requirementStore) SetParent(ctx context.Context, id, parentID, updatedAt string) error {
	return requireChanged(s.db.ExecContext(ctx,
		`UPDATE requirements SET parent_id = ?, updated_at = ? WHERE id = ?`,
		nullableText(parentID), updatedAt, id))
}

func (s *requirementStore) ListBySprintID(ctx context.Context, sprintID string) ([]*model.Requirement, error) {
	return s.listRequirements(ctx,
		`SELECT `+requirementColumns+` FROM requirements WHERE sprint_id = ? ORDER BY `+itemOrder,
		sprintID)
}

func (s *requirementStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Requirement, error) {
	return s.listRequirements(ctx,
		`SELECT `+requirementColumns+` FROM requirements WHERE milestone_id = ? ORDER BY `+itemOrder,
		milestoneID)
}

func (s *requirementStore) Assign(ctx context.Context, a *model.Assignment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO requirement_assignments (id, requirement_id, user_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		a.ID, a.TargetID, a.UserID, a.CreatedAt,
	)
	return err
}

func (s *requirementStore) Unassign(ctx context.Context, requirementID, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM requirement_assignments WHERE requirement_id = ? AND user_id = ?`,
		requirementID, userID)
	return err
}

func (s *requirementStore) ListAssignees(ctx context.Context, requirementID string) ([]*model.Assignment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, requirement_id, user_id, created_at
		 FROM requirement_assignments WHERE requirement_id = ? ORDER BY created_at, id`,
		requirementID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var assignments []*model.Assignment
	for rows.Next() {
		var a model.Assignment
		if err := rows.Scan(&a.ID, &a.TargetID, &a.UserID, &a.CreatedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, &a)
	}
	return assignments, rows.Err()
}

func (s *requirementStore) listRequirements(ctx context.Context, query string, args ...any) ([]*model.Requirement, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var reqs []*model.Requirement
	for rows.Next() {
		r, err := scanRequirement(rows)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
	}
	return reqs, rows.Err()
}

func (s *requirementStore) NextSequenceNumber(ctx context.Context, productID string) (int, error) {
	var maxSeq sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(sequence_number) FROM requirements WHERE product_id = ?`,
		productID).Scan(&maxSeq)
	if err != nil {
		return 0, err
	}
	if maxSeq.Valid {
		return int(maxSeq.Int64) + 1, nil
	}
	return 1, nil
}

func (s *requirementStore) CountByCategoryID(ctx context.Context, categoryID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM requirements WHERE category_id = ?`,
		categoryID).Scan(&count)
	return count, err
}

// requirementColumns reads is_active through activeColumn, so in kanban mode
// it reports the activation of the item's column rather than the stored flag.
var requirementColumns = `id, product_id, kind, parent_id, category_id, sequence_number, title, description,
	definition_of_done, status_id, milestone_id, sprint_id, estimation, priority,
	is_coarse, ` + activeColumn("requirements") + `, archived_at, created_at, updated_at`

// requirementColumnsQualified is requirementColumns for queries that alias
// requirements as r and join other tables.
var requirementColumnsQualified = `r.id, r.product_id, r.kind, r.parent_id, r.category_id, r.sequence_number, r.title, r.description,
	r.definition_of_done, r.status_id, r.milestone_id, r.sprint_id, r.estimation, r.priority,
	r.is_coarse, ` + activeColumn("r") + `, r.archived_at, r.created_at, r.updated_at`

func scanRequirement(sc scanner) (*model.Requirement, error) {
	var r model.Requirement
	var parentID, statusID, milestoneID, sprintID, estimation, archivedAt sql.NullString
	err := sc.Scan(&r.ID, &r.ProductID, &r.Kind, &parentID, &r.CategoryID, &r.SequenceNumber,
		&r.Title, &r.Description, &r.DefinitionOfDone,
		&statusID, &milestoneID, &sprintID, &estimation, &r.Priority,
		&r.IsCoarse, &r.IsActive, &archivedAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.ParentID = parentID.String
	r.StatusID = statusID.String
	r.MilestoneID = milestoneID.String
	r.SprintID = sprintID.String
	r.Estimation = estimation.String
	r.ArchivedAt = archivedAt.String
	return &r, nil
}
