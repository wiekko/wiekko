// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

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
	Criticality   string // empty = all
	StatusID      string // empty = all
	SprintID      string // empty = all
	EnvironmentID string // empty = all
	MilestoneID   string // empty = all
	// Archived selects archived defects (FR-155); the zero value includes
	// them, so only the views that hide archived defects have to say so.
	Archived ArchivedFilter
}

// DefectSortColumns maps the sort keys the UI and API may ask for onto SQL
// expressions. Keys absent from this map fall back to the default order, so
// nothing a caller sends can reach the ORDER BY clause verbatim.
//
// criticality and status cannot sort on their stored values: criticality is a
// text enum whose alphabetical order (critical, high, low, medium) is not its
// severity order, and a defect's status is a foreign key whose meaningful order
// lives in statuses.position.
var DefectSortColumns = map[string]string{
	"title":       `LOWER(d.title)`,
	"criticality": `CASE d.criticality WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END`,
	"status":      `COALESCE(s.position, 9999)`,
	"environment": `LOWER(COALESCE(e.name, ''))`,
	"identifier":  `d.sequence_number`,
	"priority":    priorityRank("d.priority"),
	"created":     `d.created_at`,
}

// defectDefaultOrder lists the highest priority first, then the oldest
// defect; the identifier breaks ties within one second (DF-2).
var defectDefaultOrder = priorityRank("d.priority") + `, d.created_at, d.sequence_number`

// defectListFrom joins the tables whose values are sortable or searchable but
// not stored on the defect itself. Both joins are outer: status and environment
// are optional, and a defect missing one must not drop out of the list.
const defectListFrom = ` FROM defects d
	JOIN products p ON d.product_id = p.id
	LEFT JOIN statuses s ON d.status_id = s.id
	LEFT JOIN runtime_environments e ON d.environment_id = e.id`

// DefectStore defines data access operations for defects.
type DefectStore interface {
	Create(ctx context.Context, d *model.Defect) error
	GetByID(ctx context.Context, id string) (*model.Defect, error)
	GetBySequence(ctx context.Context, productID string, seq int) (*model.Defect, error)
	NextSequenceNumber(ctx context.Context, productID string) (int, error)
	Update(ctx context.Context, d *model.Defect) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string, filter DefectFilter) ([]*model.Defect, error)
	ListPageByProductID(ctx context.Context, productID string, filter DefectFilter, params ListParams) ([]*model.Defect, error)
	CountByProductID(ctx context.Context, productID string, filter DefectFilter, search string) (int, error)
	ListBySprintID(ctx context.Context, sprintID string) ([]*model.Defect, error)
	ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Defect, error)
	Assign(ctx context.Context, a *model.Assignment) error
	Unassign(ctx context.Context, defectID, userID string) error
	ListAssignees(ctx context.Context, defectID string) ([]*model.Assignment, error)
	// UnassignFromProduct removes userID from every defect of productID and
	// returns those defects' IDs, for their history (UR-171).
	UnassignFromProduct(ctx context.Context, productID, userID string) ([]string, error)
	AddLink(ctx context.Context, link *model.DefectLink) error
	RemoveLink(ctx context.Context, id string) error
	// RemoveLinksToTarget removes every link pointing at targetID. target_id
	// is polymorphic, so no foreign key does this when the target is deleted.
	RemoveLinksToTarget(ctx context.Context, targetID string) error
	GetLink(ctx context.Context, id string) (*model.DefectLink, error)
	ListLinks(ctx context.Context, defectID string) ([]*model.DefectLink, error)
	ListLinksByTarget(ctx context.Context, targetType, targetID string) ([]*model.DefectLink, error)
	// SetArchivedAt archives the defect at archivedAt, or unarchives it when
	// archivedAt is empty (FR-155).
	SetArchivedAt(ctx context.Context, id, archivedAt, updatedAt string) error
}

type defectStore struct {
	db *DB
}

func (s *defectStore) Create(ctx context.Context, d *model.Defect) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO defects (id, product_id, sequence_number, title, description, criticality,
		 status_id, milestone_id, sprint_id, environment_id, estimation, priority, is_active,
		 created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProductID, d.SequenceNumber, d.Title, d.Description, d.Criticality,
		nullableText(d.StatusID), nullableText(d.MilestoneID), nullableText(d.SprintID),
		nullableText(d.EnvironmentID), nullableText(d.Estimation), d.Priority, d.IsActive,
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

// GetBySequence returns the product's defect with the given sequence number.
func (s *defectStore) GetBySequence(ctx context.Context, productID string, seq int) (*model.Defect, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+defectColumns+` FROM defects WHERE product_id = ? AND sequence_number = ?`, productID, seq)
	d, err := scanDefect(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("defect not found: %w", err)
	}
	return d, err
}

// NextSequenceNumber returns the next free defect number of the product. A
// concurrent insert can take it first; the unique index then rejects the
// second insert, and the caller retries (see BacklogService.CreateDefect).
func (s *defectStore) NextSequenceNumber(ctx context.Context, productID string) (int, error) {
	var maxSeq sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(sequence_number) FROM defects WHERE product_id = ?`, productID).Scan(&maxSeq)
	if err != nil {
		return 0, err
	}
	if maxSeq.Valid {
		return int(maxSeq.Int64) + 1, nil
	}
	return 1, nil
}

func (s *defectStore) Update(ctx context.Context, d *model.Defect) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE defects SET title = ?, description = ?, criticality = ?,
		 status_id = ?, milestone_id = ?, sprint_id = ?, environment_id = ?,
		 estimation = ?, priority = ?, is_active = ?, updated_at = ?
		 WHERE id = ?`,
		d.Title, d.Description, d.Criticality,
		nullableText(d.StatusID), nullableText(d.MilestoneID), nullableText(d.SprintID),
		nullableText(d.EnvironmentID), nullableText(d.Estimation), d.Priority, d.IsActive,
		d.UpdatedAt, d.ID,
	)
	return err
}

func (s *defectStore) SetArchivedAt(ctx context.Context, id, archivedAt, updatedAt string) error {
	return requireChanged(s.db.ExecContext(ctx,
		`UPDATE defects SET archived_at = ?, updated_at = ? WHERE id = ?`,
		nullableText(archivedAt), updatedAt, id))
}

func (s *defectStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM defects WHERE id = ?`, id)
	return err
}

// defectListWhere builds the WHERE clause shared by ListByProductID and
// CountByProductID. Keeping it in one place is what stops the page query and
// the count that sizes its pagination from drifting apart.
func defectListWhere(productID string, filter DefectFilter, search string) (string, []any) {
	where := ` WHERE d.product_id = ?`
	args := []any{productID}
	if filter.Criticality != "" {
		where += ` AND d.criticality = ?`
		args = append(args, filter.Criticality)
	}
	if filter.StatusID != "" {
		where += ` AND d.status_id = ?`
		args = append(args, filter.StatusID)
	}
	if filter.SprintID != "" {
		where += ` AND d.sprint_id = ?`
		args = append(args, filter.SprintID)
	}
	if filter.EnvironmentID != "" {
		where += ` AND d.environment_id = ?`
		args = append(args, filter.EnvironmentID)
	}
	if filter.MilestoneID != "" {
		where += ` AND d.milestone_id = ?`
		args = append(args, filter.MilestoneID)
	}
	where += filter.Archived.where("d.archived_at")
	// The global search deliberately spans only the columns a substring match
	// is meaningful for, the identifier (DF-17) among them. Priority is numeric, so "1" would match 10 and 21;
	// created_at is RFC 3339 while the column renders as "12 Jul", so matching
	// the stored form would not match what the user is looking at.
	if pattern := searchPattern(search); search != "" {
		where += ` AND (LOWER(d.title) LIKE ? ESCAPE '` + LikeEscape + `'` +
			` OR LOWER(p.defect_prefix || '-' || CAST(d.sequence_number AS TEXT)) LIKE ? ESCAPE '` + LikeEscape + `'` +
			` OR LOWER(d.criticality) LIKE ? ESCAPE '` + LikeEscape + `'` +
			` OR LOWER(COALESCE(s.name, '')) LIKE ? ESCAPE '` + LikeEscape + `'` +
			` OR LOWER(COALESCE(e.name, '')) LIKE ? ESCAPE '` + LikeEscape + `')`
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	return where, args
}

func (s *defectStore) ListByProductID(ctx context.Context, productID string, filter DefectFilter) ([]*model.Defect, error) {
	return s.ListPageByProductID(ctx, productID, filter, ListParams{})
}

func (s *defectStore) ListPageByProductID(ctx context.Context, productID string, filter DefectFilter, params ListParams) ([]*model.Defect, error) {
	where, args := defectListWhere(productID, filter, params.Search)
	query := `SELECT ` + defectColumnsQualified + defectListFrom + where
	// Ties fall back to the identifier, then the ID (DF-2).
	query += orderClause(params, DefectSortColumns, defectDefaultOrder, `d.sequence_number, d.id`)

	limit, limitArgs := limitClause(params)
	query += limit
	args = append(args, limitArgs...)

	return s.listDefects(ctx, query, args...)
}

func (s *defectStore) CountByProductID(ctx context.Context, productID string, filter DefectFilter, search string) (int, error) {
	where, args := defectListWhere(productID, filter, search)
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*)`+defectListFrom+where, args...).Scan(&total)
	return total, err
}

func (s *defectStore) ListBySprintID(ctx context.Context, sprintID string) ([]*model.Defect, error) {
	return s.listDefects(ctx,
		`SELECT `+defectColumns+` FROM defects WHERE sprint_id = ? ORDER BY `+itemOrder,
		sprintID)
}

func (s *defectStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Defect, error) {
	return s.listDefects(ctx,
		`SELECT `+defectColumns+` FROM defects WHERE milestone_id = ? ORDER BY `+itemOrder,
		milestoneID)
}

func (s *defectStore) Assign(ctx context.Context, a *model.Assignment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO defect_assignments (id, defect_id, user_id, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (defect_id, user_id) DO NOTHING`,
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

func (s *defectStore) UnassignFromProduct(ctx context.Context, productID, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.id FROM defect_assignments a
		 JOIN defects d ON d.id = a.defect_id
		 WHERE d.product_id = ? AND a.user_id = ? ORDER BY d.id`,
		productID, userID)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM defect_assignments WHERE user_id = ?
		 AND defect_id IN (SELECT id FROM defects WHERE product_id = ?)`,
		userID, productID)
	return ids, err
}

func (s *defectStore) ListAssignees(ctx context.Context, defectID string) ([]*model.Assignment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, defect_id, user_id, created_at
		 FROM defect_assignments WHERE defect_id = ? ORDER BY created_at, id`,
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
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (defect_id, target_type, target_id) DO NOTHING`,
		link.ID, link.DefectID, link.TargetType, link.TargetID, link.CreatedAt,
	)
	return err
}

func (s *defectStore) RemoveLinksToTarget(ctx context.Context, targetID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM defect_links WHERE target_id = ?`, targetID)
	return err
}

func (s *defectStore) RemoveLink(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM defect_links WHERE id = ?`, id)
	return err
}

func (s *defectStore) GetLink(ctx context.Context, id string) (*model.DefectLink, error) {
	var l model.DefectLink
	err := s.db.QueryRowContext(ctx,
		`SELECT id, defect_id, target_type, target_id, created_at FROM defect_links WHERE id = ?`, id).
		Scan(&l.ID, &l.DefectID, &l.TargetType, &l.TargetID, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("defect link not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *defectStore) ListLinks(ctx context.Context, defectID string) ([]*model.DefectLink, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, defect_id, target_type, target_id, created_at
		 FROM defect_links WHERE defect_id = ? ORDER BY created_at, id`,
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
		 FROM defect_links WHERE target_type = ? AND target_id = ? ORDER BY created_at, id`,
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

// defectColumns reads is_active through activeColumn, like requirementColumns.
//
// The product's defect prefix comes along through a subquery, so every read
// yields the identifier with the prefix currently configured (FR-49).
var defectColumns = `id, product_id, sequence_number,
	(SELECT dp.defect_prefix FROM products dp WHERE dp.id = defects.product_id),
	title, description, criticality,
	status_id, milestone_id, sprint_id, environment_id, estimation, priority, ` + activeColumn("defects") + `,
	archived_at, created_at, updated_at`

// defectColumnsQualified is defectColumns for the joined list query, where bare
// column names would be ambiguous. Both feed the same scanDefect, so the column
// order has to stay identical.
var defectColumnsQualified = `d.id, d.product_id, d.sequence_number, p.defect_prefix, d.title, d.description, d.criticality,
	d.status_id, d.milestone_id, d.sprint_id, d.environment_id, d.estimation, d.priority, ` + activeColumn("d") + `,
	d.archived_at, d.created_at, d.updated_at`

func scanDefect(sc scanner) (*model.Defect, error) {
	var d model.Defect
	var statusID, milestoneID, sprintID, environmentID, estimation, archivedAt sql.NullString
	var prefix string
	err := sc.Scan(&d.ID, &d.ProductID, &d.SequenceNumber, &prefix, &d.Title, &d.Description, &d.Criticality,
		&statusID, &milestoneID, &sprintID, &environmentID, &estimation, &d.Priority,
		&d.IsActive, &archivedAt, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	d.StatusID = statusID.String
	d.MilestoneID = milestoneID.String
	d.SprintID = sprintID.String
	d.EnvironmentID = environmentID.String
	d.Estimation = estimation.String
	d.ArchivedAt = archivedAt.String
	d.Identifier = model.EntityIdentifier(prefix, d.SequenceNumber)
	return &d, nil
}
