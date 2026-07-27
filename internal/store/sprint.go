package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// SprintStore defines data access operations for sprints.
type SprintStore interface {
	Create(ctx context.Context, sprint *model.Sprint) error
	GetByID(ctx context.Context, id string) (*model.Sprint, error)
	Update(ctx context.Context, sprint *model.Sprint) error
	Delete(ctx context.Context, id string) error
	ListByProductID(ctx context.Context, productID string) ([]*model.Sprint, error)
	ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Sprint, error)
}

type sprintStore struct {
	db *sql.DB
}

func (s *sprintStore) Create(ctx context.Context, sprint *model.Sprint) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sprints (id, product_id, name, milestone_id, start_date, end_date, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sprint.ID, sprint.ProductID, sprint.Name,
		nullableText(sprint.MilestoneID),
		sprint.StartDate, sprint.EndDate,
		sprint.CreatedAt, sprint.UpdatedAt,
	)
	return err
}

func (s *sprintStore) GetByID(ctx context.Context, id string) (*model.Sprint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, product_id, name, milestone_id, start_date, end_date, created_at, updated_at
		 FROM sprints WHERE id = ?`, id)
	sp, err := scanSprint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("sprint not found: %w", err)
	}
	return sp, err
}

func (s *sprintStore) Update(ctx context.Context, sprint *model.Sprint) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sprints SET name = ?, milestone_id = ?, start_date = ?, end_date = ?, updated_at = ?
		 WHERE id = ?`,
		sprint.Name, nullableText(sprint.MilestoneID),
		sprint.StartDate, sprint.EndDate,
		sprint.UpdatedAt, sprint.ID,
	)
	return err
}

func (s *sprintStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sprints WHERE id = ?`, id)
	return err
}

func (s *sprintStore) ListByProductID(ctx context.Context, productID string) ([]*model.Sprint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, name, milestone_id, start_date, end_date, created_at, updated_at
		 FROM sprints WHERE product_id = ? ORDER BY start_date`, productID)
	if err != nil {
		return nil, err
	}
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

func (s *sprintStore) ListByMilestoneID(ctx context.Context, milestoneID string) ([]*model.Sprint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, product_id, name, milestone_id, start_date, end_date, created_at, updated_at
		 FROM sprints WHERE milestone_id = ? ORDER BY start_date`, milestoneID)
	if err != nil {
		return nil, err
	}
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
	err := sc.Scan(&sp.ID, &sp.ProductID, &sp.Name, &milestoneID,
		&sp.StartDate, &sp.EndDate, &sp.CreatedAt, &sp.UpdatedAt)
	if err != nil {
		return nil, err
	}
	sp.MilestoneID = milestoneID.String
	return &sp, nil
}
