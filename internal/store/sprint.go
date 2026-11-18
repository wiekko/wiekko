package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// sprintColumns is the column list shared by every sprint SELECT.
const sprintColumns = `id, product_id, name, milestone_id, status, start_date, end_date, created_at, updated_at`

// SprintStore defines data access operations for sprints.
type SprintStore interface {
	Create(ctx context.Context, sprint *model.Sprint) error
	GetByID(ctx context.Context, id string) (*model.Sprint, error)
	Update(ctx context.Context, sprint *model.Sprint) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.Sprint, error)
	ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Sprint, error)
	// GetActiveByProductID returns the product's active sprint, or nil when
	// none is active. At most one sprint per product may be active.
	GetActiveByProductID(ctx context.Context, productID string) (*model.Sprint, error)
}

type sprintStore struct {
	db *DB
}

func (s *sprintStore) Create(ctx context.Context, sprint *model.Sprint) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sprints (id, product_id, name, milestone_id, status, start_date, end_date, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sprint.ID, sprint.ProductID, sprint.Name,
		nullableText(sprint.MilestoneID),
		sprint.Status,
		sprint.StartDate, sprint.EndDate,
		sprint.CreatedAt, sprint.UpdatedAt,
	)
	return err
}

func (s *sprintStore) GetByID(ctx context.Context, id string) (*model.Sprint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sprintColumns+` FROM sprints WHERE id = ?`, id)
	sp, err := scanSprint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("sprint not found: %w", err)
	}
	return sp, err
}

func (s *sprintStore) Update(ctx context.Context, sprint *model.Sprint) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sprints SET name = ?, milestone_id = ?, status = ?, start_date = ?, end_date = ?, updated_at = ?
		 WHERE id = ?`,
		sprint.Name, nullableText(sprint.MilestoneID),
		sprint.Status,
		sprint.StartDate, sprint.EndDate,
		sprint.UpdatedAt, sprint.ID,
	)
	return err
}

// Delete removes a sprint. Its items go back to the backlog and, as when an
// item is taken out of a sprint, are no longer active (FR-91): the backlog is
// the product owner's again. The foreign keys would only clear sprint_id, so
// the items are released explicitly in the same transaction.
func (s *sprintStore) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, table := range []string{"requirements", "defects"} {
		if _, err := tx.ExecContext(ctx,
			`UPDATE `+table+` SET sprint_id = NULL, is_active = ? WHERE sprint_id = ?`, false, id); err != nil {
			return fmt.Errorf("release %s: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sprints WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sprintStore) ListByProductID(ctx context.Context, productID string) ([]*model.Sprint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sprintColumns+` FROM sprints WHERE product_id = ? ORDER BY start_date`, productID)
	if err != nil {
		return nil, err
	}
	return collectSprints(rows)
}

func (s *sprintStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Sprint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sprintColumns+` FROM sprints WHERE milestone_id = ? ORDER BY start_date`, milestoneID)
	if err != nil {
		return nil, err
	}
	return collectSprints(rows)
}

func (s *sprintStore) GetActiveByProductID(ctx context.Context, productID string) (*model.Sprint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sprintColumns+` FROM sprints WHERE product_id = ? AND status = ? ORDER BY start_date LIMIT 1`,
		productID, model.SprintStatusActive)
	sp, err := scanSprint(row)
	if errors.Is(err, sql.ErrNoRows) {
		// No active sprint is an ordinary state, not an error.
		return nil, nil
	}
	return sp, err
}

func collectSprints(rows *Rows) ([]*model.Sprint, error) {
	defer closeRows(rows)

	var sprints []*model.Sprint
	for rows.Next() {
		sp, err := scanSprint(rows)
		if err != nil {
			return nil, err
		}
		sprints = append(sprints, sp)
	}
	return sprints, rows.Err()
}

func scanSprint(sc scanner) (*model.Sprint, error) {
	var sp model.Sprint
	var milestoneID sql.NullString
	err := sc.Scan(&sp.ID, &sp.ProductID, &sp.Name, &milestoneID, &sp.Status,
		&sp.StartDate, &sp.EndDate, &sp.CreatedAt, &sp.UpdatedAt)
	if err != nil {
		return nil, err
	}
	sp.MilestoneID = milestoneID.String
	return &sp, nil
}
