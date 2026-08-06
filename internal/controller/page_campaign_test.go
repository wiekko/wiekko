package controller

import (
	"testing"

	"nexmap.net/internal/model"
	productpage "nexmap.net/web/templates/pages/product"
)

func TestIsOverride(t *testing.T) {
	tests := []struct {
		name            string
		previous, actor string
		want            bool
	}{
		{"first change overrides nothing", "", "alice", false},
		{"same tester revising their own verdict", "alice", "alice", false},
		{"another tester revising the verdict", "alice", "bob", true},
		{"system re-test is not an override", "alice", "", false},
		{"tester acting after a system re-test", "", "bob", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOverride(tt.previous, tt.actor); got != tt.want {
				t.Fatalf("isOverride(%q, %q) = %v, want %v", tt.previous, tt.actor, got, tt.want)
			}
		})
	}
}

func TestEventActorName(t *testing.T) {
	names := map[string]string{"user-001": "Alice"}

	if got := eventActorName(names, "user-001"); got != "Alice" {
		t.Fatalf("known user = %q, want Alice", got)
	}
	// A defect closing reopens results without anyone acting.
	if got := eventActorName(names, ""); got != "System" {
		t.Fatalf("empty user = %q, want System", got)
	}
	// A deleted account must not leak a raw id into the page.
	if got := eventActorName(names, "user-999"); got != "Unknown user" {
		t.Fatalf("unknown user = %q, want Unknown user", got)
	}
}

func TestMatchesHistoryFilter(t *testing.T) {
	result := productpage.CampaignHistoryEntry{
		Kind:       "result",
		ToStatus:   model.CampaignResultStatusFailed,
		IsOverride: true,
	}
	retest := productpage.CampaignHistoryEntry{
		Kind:     "result",
		ToStatus: model.CampaignResultStatusRetest,
		IsRetest: true,
	}
	audit := productpage.CampaignHistoryEntry{Kind: "audit"}

	tests := []struct {
		name   string
		entry  productpage.CampaignHistoryEntry
		filter string
		want   bool
	}{
		{"override matches overrides", result, productpage.CampaignHistoryFilterOverrides, true},
		{"retest does not match overrides", retest, productpage.CampaignHistoryFilterOverrides, false},
		{"retest matches retests", retest, productpage.CampaignHistoryFilterRetests, true},
		{"failure matches failures", result, productpage.CampaignHistoryFilterFailures, true},
		{"retest does not match failures", retest, productpage.CampaignHistoryFilterFailures, false},
		// Every named filter is about results, so lifecycle entries drop out.
		// The unfiltered feed keeps them, but takes a path that never reaches
		// here.
		{"audit entry is filtered out", audit, productpage.CampaignHistoryFilterOverrides, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesHistoryFilter(tt.entry, tt.filter); got != tt.want {
				t.Fatalf("matchesHistoryFilter(%s) = %v, want %v", tt.filter, got, tt.want)
			}
		})
	}
}

func TestBuildProgressTimelineBucketsByDay(t *testing.T) {
	events := []*model.CampaignResultEvent{
		{ResultID: "a", ToStatus: model.CampaignResultStatusPassed, CreatedAt: "2026-08-01T09:00:00Z"},
		{ResultID: "b", ToStatus: model.CampaignResultStatusPassed, CreatedAt: "2026-08-01T15:00:00Z"},
		{ResultID: "c", ToStatus: model.CampaignResultStatusFailed, CreatedAt: "2026-08-02T09:00:00Z"},
		{ResultID: "d", ToStatus: model.CampaignResultStatusPassed, CreatedAt: "2026-08-03T09:00:00Z"},
	}

	points := buildProgressTimeline(events, 4)
	if len(points) != 3 {
		t.Fatalf("len(points) = %d, want one per day", len(points))
	}
	// The first day collapses to its final state, not its first.
	if points[0].Settled != 2 || points[0].At != "2026-08-01T15:00:00Z" {
		t.Fatalf("points[0] = %+v, want the day's last state with 2 settled", points[0])
	}
	if points[2].Settled != 4 || points[2].Percent != 100 {
		t.Fatalf("points[2] = %+v, want 4 settled at 100%%", points[2])
	}
}

func TestBuildProgressTimelineIgnoresIncompleteStatuses(t *testing.T) {
	// A result sent back for re-test stops counting as settled again.
	events := []*model.CampaignResultEvent{
		{ResultID: "a", ToStatus: model.CampaignResultStatusFailed, CreatedAt: "2026-08-01T09:00:00Z"},
		{ResultID: "a", ToStatus: model.CampaignResultStatusRetest, CreatedAt: "2026-08-02T09:00:00Z"},
	}

	points := buildProgressTimeline(events, 2)
	if len(points) != 2 {
		t.Fatalf("len(points) = %d, want 2", len(points))
	}
	if points[0].Settled != 1 {
		t.Fatalf("points[0].Settled = %d, want 1", points[0].Settled)
	}
	if points[1].Settled != 0 {
		t.Fatalf("points[1].Settled = %d, want 0 once the result is re-opened", points[1].Settled)
	}
}
