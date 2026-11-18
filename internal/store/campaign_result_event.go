package store

import (
	"context"
	"database/sql"

	"nexmap.net/internal/model"
)

// CampaignResultEventStore defines data access operations for the immutable
// campaign result status change log (FR-48).
type CampaignResultEventStore interface {
	Create(ctx context.Context, ev *model.CampaignResultEvent) error
	ListByResultID(ctx context.Context, resultID string) ([]*model.CampaignResultEvent, error)
	ListByCampaignID(ctx context.Context, campaignID string) ([]*model.CampaignResultEvent, error)
	ListByDefectTrail(ctx context.Context, defectID string) ([]*model.CampaignResultEvent, error)
	CountsByCampaignID(ctx context.Context, campaignID string) (map[string]int, error)
}

type campaignResultEventStore struct {
	db *DB
}

const campaignResultEventColumns = `id, result_id, campaign_id, user_id,
	from_status, to_status, defect_id, source, revision, branch, created_at`

func scanCampaignResultEvent(sc scanner) (*model.CampaignResultEvent, error) {
	var ev model.CampaignResultEvent
	var userID, defectID sql.NullString
	if err := sc.Scan(&ev.ID, &ev.ResultID, &ev.CampaignID, &userID,
		&ev.FromStatus, &ev.ToStatus, &defectID, &ev.Source, &ev.Revision, &ev.Branch, &ev.CreatedAt); err != nil {
		return nil, err
	}
	ev.UserID = userID.String
	ev.DefectID = defectID.String
	return &ev, nil
}

func (s *campaignResultEventStore) Create(ctx context.Context, ev *model.CampaignResultEvent) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO campaign_result_events (`+campaignResultEventColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, ev.ResultID, ev.CampaignID, nullableText(ev.UserID),
		ev.FromStatus, ev.ToStatus, nullableText(ev.DefectID),
		ev.Source, ev.Revision, ev.Branch, ev.CreatedAt,
	)
	return err
}

// listBy runs one indexed scan of the log. Timestamps are second-precision and a
// batch retest stamps every event it writes with the same instant, so id breaks
// ties to keep the order stable between calls.
func (s *campaignResultEventStore) listBy(ctx context.Context, where, arg string) ([]*model.CampaignResultEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+campaignResultEventColumns+` FROM campaign_result_events
		 WHERE `+where+` ORDER BY created_at, id`, arg)
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

// ListByDefectTrail returns the complete history of every result the defect ever
// touched, not just the events that name it.
//
// A passing verdict clears the result's defect, so the event that closes a
// verification loop carries no defect_id at all. Filtering on the column alone
// would return the failure and the re-test but drop the pass that proves the fix
// worked, which is the one row worth showing. Selecting the results first and
// then taking their whole history reconstructs the loop end to end.
func (s *campaignResultEventStore) ListByDefectTrail(ctx context.Context, defectID string) ([]*model.CampaignResultEvent, error) {
	return s.listBy(ctx,
		`result_id IN (SELECT result_id FROM campaign_result_events WHERE defect_id = ?)`,
		defectID)
}

// CountsByCampaignID returns how many events each result in the campaign has
// accumulated, keyed by result id. A result that was set once has exactly one
// event, so a count above one means the verdict was changed afterwards.
func (s *campaignResultEventStore) CountsByCampaignID(ctx context.Context, campaignID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT result_id, COUNT(*) FROM campaign_result_events
		 WHERE campaign_id = ? GROUP BY result_id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	counts := make(map[string]int)
	for rows.Next() {
		var resultID string
		var n int
		if err := rows.Scan(&resultID, &n); err != nil {
			return nil, err
		}
		counts[resultID] = n
	}
	return counts, rows.Err()
}
