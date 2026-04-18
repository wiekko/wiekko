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
	CategoryID string // empty = all categories
	StatusID   string // empty = all statuses
	SprintID   string // empty = all sprints
}

// RequirementStore defines data access operations for requirements.
type RequirementStore interface {
	Create(ctx context.Context, r *model.Requirement) error
	GetByID(ctx context.Context, id string) (*model.Requirement, error)
	Update(ctx context.Context, r *model.Requirement) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string, filter RequirementFilter) ([]*model.Requirement, error)
	ListByParentID(ctx context.Context, parentID string) ([]*model.Requirement, error)
	ListBySprintID(ctx context.Context, sprintID string) ([]*model.Requirement, error)
	NextSequenceNumber(ctx context.Context, productID string) (int, error)
	Assign(ctx context.Context, a *model.Assignment) error
	Unassign(ctx context.Context, requirementID, userID string) error
	ListAssignees(ctx context.Context, requirementID string) ([]*model.Assignment, error)
}

type requirementStore struct {
	db *sql.DB
}

func (s *requirementStore) Create(ctx context.Context, r *model.Requirement) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO requirements (id, product_id, parent_id, category_id, sequence_number, title, description,
		 definition_of_done, status_id, milestone_id, sprint_id, estimation, priority,
		 is_coarse, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProductID, nullableText(r.ParentID), r.CategoryID, r.SequenceNumber,
		r.Title, r.Description, r.DefinitionOfDone,
		nullableText(r.StatusID), nullableText(r.MilestoneID), nullableText(r.SprintID),
		nullableText(r.Estimation), r.Priority,
		boolToInt(r.IsCoarse), boolToInt(r.IsActive),
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
		boolToInt(r.IsCoarse), boolToInt(r.IsActive),
		r.UpdatedAt, r.ID,
	)
	return err
}

func (s *requirementStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM requirements WHERE id = ?`, id)
	return err
}

func (s *requirementStore) ListByProductID(ctx context.Context, productID string, filter RequirementFilter) ([]*model.Requirement, error) {
	query := `SELECT ` + requirementColumns + ` FROM requirements WHERE product_id = ?`
	args := []any{productID}
	if filter.CategoryID != "" {
		query += ` AND category_id = ?`
		args = append(args, filter.CategoryID)
	}
	if filter.StatusID != "" {
		query += ` AND status_id = ?`
		args = append(args, filter.StatusID)
	}
	if filter.SprintID != "" {
		query += ` AND sprint_id = ?`
		args = append(args, filter.SprintID)
	}
	query += ` ORDER BY priority DESC, created_at`
	return s.listRequirements(ctx, query, args...)
}

func (s *requirementStore) ListByParentID(ctx context.Context, parentID string) ([]*model.Requirement, error) {
	return s.listRequirements(ctx,
		`SELECT `+requirementColumns+` FROM requirements WHERE parent_id = ? ORDER BY priority DESC, created_at`,
		parentID)
}

func (s *requirementStore) ListBySprintID(ctx context.Context, sprintID string) ([]*model.Requirement, error) {
	return s.listRequirements(ctx,
		`SELECT `+requirementColumns+` FROM requirements WHERE sprint_id = ? ORDER BY priority DESC, created_at`,
		sprintID)
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
		 FROM requirement_assignments WHERE requirement_id = ? ORDER BY created_at`,
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

const requirementColumns = `id, product_id, parent_id, category_id, sequence_number, title, description,
	definition_of_done, status_id, milestone_id, sprint_id, estimation, priority,
	is_coarse, is_active, created_at, updated_at`

func scanRequirement(sc scanner) (*model.Requirement, error) {
	var r model.Requirement
	var parentID, statusID, milestoneID, sprintID, estimation sql.NullString
	var isCoarse, isActive int
	err := sc.Scan(&r.ID, &r.ProductID, &parentID, &r.CategoryID, &r.SequenceNumber,
		&r.Title, &r.Description, &r.DefinitionOfDone,
		&statusID, &milestoneID, &sprintID, &estimation, &r.Priority,
		&isCoarse, &isActive, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.ParentID = parentID.String
	r.StatusID = statusID.String
	r.MilestoneID = milestoneID.String
	r.SprintID = sprintID.String
	r.Estimation = estimation.String
	r.IsCoarse = isCoarse != 0
	r.IsActive = isActive != 0
	return &r, nil
}
