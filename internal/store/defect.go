package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// DefectFilter controls which defects are returned by ListByProductID.
type DefectFilter struct {
	Criticality string // empty = all
	StatusID    string // empty = all
	SprintID    string // empty = all
}

// DefectStore defines data access operations for defects.
type DefectStore interface {
	Create(ctx context.Context, d *model.Defect) error
	GetByID(ctx context.Context, id string) (*model.Defect, error)
	Update(ctx context.Context, d *model.Defect) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string, filter DefectFilter) ([]*model.Defect, error)
	ListBySprintID(ctx context.Context, sprintID string) ([]*model.Defect, error)
	ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Defect, error)
	Assign(ctx context.Context, a *model.Assignment) error
	Unassign(ctx context.Context, defectID, userID string) error
	ListAssignees(ctx context.Context, defectID string) ([]*model.Assignment, error)
	AddLink(ctx context.Context, link *model.DefectLink) error
	RemoveLink(ctx context.Context, id string) error
	ListLinks(ctx context.Context, defectID string) ([]*model.DefectLink, error)
	ListLinksByTarget(ctx context.Context, targetType, targetID string) ([]*model.DefectLink, error)
}

type defectStore struct {
	db *sql.DB
}

func (s *defectStore) Create(ctx context.Context, d *model.Defect) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO defects (id, product_id, title, description, criticality,
		 status_id, milestone_id, sprint_id, estimation, priority, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProductID, d.Title, d.Description, d.Criticality,
		nullableText(d.StatusID), nullableText(d.MilestoneID), nullableText(d.SprintID),
		nullableText(d.Estimation), d.Priority, boolToInt(d.IsActive),
		d.CreatedAt, d.UpdatedAt,
	)
	return err
}

func (s *defectStore) GetByID(ctx context.Context, id string) (*model.Defect, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+defectColumns+` FROM defects WHERE id = ?`, id)
	d, err := scanDefect(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("defect not found: %w", err)
	}
	return d, err
}

func (s *defectStore) Update(ctx context.Context, d *model.Defect) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE defects SET title = ?, description = ?, criticality = ?,
		 status_id = ?, milestone_id = ?, sprint_id = ?,
		 estimation = ?, priority = ?, is_active = ?, updated_at = ?
		 WHERE id = ?`,
		d.Title, d.Description, d.Criticality,
		nullableText(d.StatusID), nullableText(d.MilestoneID), nullableText(d.SprintID),
		nullableText(d.Estimation), d.Priority, boolToInt(d.IsActive),
		d.UpdatedAt, d.ID,
	)
	return err
}

func (s *defectStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM defects WHERE id = ?`, id)
	return err
}

func (s *defectStore) ListByProductID(ctx context.Context, productID string, filter DefectFilter) ([]*model.Defect, error) {
	query := `SELECT ` + defectColumns + ` FROM defects WHERE product_id = ?`
	args := []any{productID}
	if filter.Criticality != "" {
		query += ` AND criticality = ?`
		args = append(args, filter.Criticality)
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
	return s.listDefects(ctx, query, args...)
}

func (s *defectStore) ListBySprintID(ctx context.Context, sprintID string) ([]*model.Defect, error) {
	return s.listDefects(ctx,
		`SELECT `+defectColumns+` FROM defects WHERE sprint_id = ? ORDER BY priority DESC, created_at`,
		sprintID)
}

func (s *defectStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Defect, error) {
	return s.listDefects(ctx,
		`SELECT `+defectColumns+` FROM defects WHERE milestone_id = ? ORDER BY priority DESC, created_at`,
		milestoneID)
}

func (s *defectStore) Assign(ctx context.Context, a *model.Assignment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO defect_assignments (id, defect_id, user_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		a.ID, a.TargetID, a.UserID, a.CreatedAt,
	)
	return err
}

func (s *defectStore) Unassign(ctx context.Context, defectID, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM defect_assignments WHERE defect_id = ? AND user_id = ?`,
		defectID, userID)
	return err
}

func (s *defectStore) ListAssignees(ctx context.Context, defectID string) ([]*model.Assignment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, defect_id, user_id, created_at
		 FROM defect_assignments WHERE defect_id = ? ORDER BY created_at`,
		defectID)
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

func (s *defectStore) AddLink(ctx context.Context, link *model.DefectLink) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO defect_links (id, defect_id, target_type, target_id, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		link.ID, link.DefectID, link.TargetType, link.TargetID, link.CreatedAt,
	)
	return err
}

func (s *defectStore) RemoveLink(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM defect_links WHERE id = ?`, id)
	return err
}

func (s *defectStore) ListLinks(ctx context.Context, defectID string) ([]*model.DefectLink, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, defect_id, target_type, target_id, created_at
		 FROM defect_links WHERE defect_id = ? ORDER BY created_at`,
		defectID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var links []*model.DefectLink
	for rows.Next() {
		var l model.DefectLink
		if err := rows.Scan(&l.ID, &l.DefectID, &l.TargetType, &l.TargetID, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}

func (s *defectStore) ListLinksByTarget(ctx context.Context, targetType, targetID string) ([]*model.DefectLink, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, defect_id, target_type, target_id, created_at
		 FROM defect_links WHERE target_type = ? AND target_id = ? ORDER BY created_at`,
		targetType, targetID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var links []*model.DefectLink
	for rows.Next() {
		var l model.DefectLink
		if err := rows.Scan(&l.ID, &l.DefectID, &l.TargetType, &l.TargetID, &l.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, &l)
	}
	return links, rows.Err()
}

func (s *defectStore) listDefects(ctx context.Context, query string, args ...any) ([]*model.Defect, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var defects []*model.Defect
	for rows.Next() {
		d, err := scanDefect(rows)
		if err != nil {
			return nil, err
		}
		defects = append(defects, d)
	}
	return defects, rows.Err()
}

const defectColumns = `id, product_id, title, description, criticality,
	status_id, milestone_id, sprint_id, estimation, priority, is_active, created_at, updated_at`

func scanDefect(sc scanner) (*model.Defect, error) {
	var d model.Defect
	var statusID, milestoneID, sprintID, estimation sql.NullString
	var isActive int
	err := sc.Scan(&d.ID, &d.ProductID, &d.Title, &d.Description, &d.Criticality,
		&statusID, &milestoneID, &sprintID, &estimation, &d.Priority,
		&isActive, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	d.StatusID = statusID.String
	d.MilestoneID = milestoneID.String
	d.SprintID = sprintID.String
	d.Estimation = estimation.String
	d.IsActive = isActive != 0
	return &d, nil
}
