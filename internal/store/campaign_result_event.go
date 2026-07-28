package store

import (
	"context"
	"database/sql"

	"nexmap.net/internal/model"
)

// CampaignResultEventStore defines data access operations for the immutable
// campaign result status change log (FR-042).
type CampaignResultEventStore interface {
	Create(ctx context.Context, ev *model.CampaignResultEvent) error
	ListByResultID(ctx context.Context, resultID string) ([]*model.CampaignResultEvent, error)
	ListByCampaignID(ctx context.Context, campaignID string) ([]*model.CampaignResultEvent, error)
}

type campaignResultEventStore struct {
	db *sql.DB
}

const campaignResultEventColumns = `id, result_id, campaign_id, user_id,
	from_status, to_status, defect_id, created_at`

func scanCampaignResultEvent(sc scanner) (*model.CampaignResultEvent, error) {
	var ev model.CampaignResultEvent
	var userID, defectID sql.NullString
	if err := sc.Scan(&ev.ID, &ev.ResultID, &ev.CampaignID, &userID,
		&ev.FromStatus, &ev.ToStatus, &defectID, &ev.CreatedAt); err != nil {
		return nil, err
	}
	ev.UserID = userID.String
	ev.DefectID = defectID.String
	return &ev, nil
}

func (s *campaignResultEventStore) Create(ctx context.Context, ev *model.CampaignResultEvent) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO campaign_result_events (`+campaignResultEventColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, ev.ResultID, ev.CampaignID, nullableText(ev.UserID),
		ev.FromStatus, ev.ToStatus, nullableText(ev.DefectID), ev.CreatedAt,
	)
	return err
}

func (s *campaignResultEventStore) listBy(ctx context.Context, where, arg string) ([]*model.CampaignResultEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+campaignResultEventColumns+` FROM campaign_result_events
		 WHERE `+where+` ORDER BY created_at`, arg)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var events []*model.CampaignResultEvent
	for rows.Next() {
		ev, err := scanCampaignResultEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (s *campaignResultEventStore) ListByResultID(ctx context.Context, resultID string) ([]*model.CampaignResultEvent, error) {
	return s.listBy(ctx, "result_id = ?", resultID)
}

func (s *campaignResultEventStore) ListByCampaignID(ctx context.Context, campaignID string) ([]*model.CampaignResultEvent, error) {
	return s.listBy(ctx, "campaign_id = ?", campaignID)
}
