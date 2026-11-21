// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"testing"

	"nexmap.net/internal/model"
)

func sprintSummaryStatuses() (req, def []*model.Status) {
	req = []*model.Status{
		{ID: "req-open", Name: "Open", Position: 1},
		{ID: "req-done", Name: "Done", Position: 2, IsClosed: true},
	}
	def = []*model.Status{
		{ID: "def-open", Name: "Open", Position: 1},
		{ID: "def-done", Name: "Done", Position: 2, IsClosed: true},
	}
	return req, def
}

func TestBuildSprintSummary_CountsCompletionAcrossEntityTypes(t *testing.T) {
	reqStatuses, defStatuses := sprintSummaryStatuses()
	p := &model.Product{ID: "p1", EstimationMode: model.EstimationModeDisabled}
	items := []*model.Requirement{
		{ID: "r1", Kind: model.RequirementKindRequirement, StatusID: "req-open"},
		{ID: "r2", Kind: model.RequirementKindRequirement, StatusID: "req-done"},
		{ID: "t1", Kind: model.RequirementKindTask, StatusID: "req-done"},
	}
	defects := []*model.Defect{{ID: "d1", StatusID: "def-open"}}

	summary := buildSprintSummary(p, items, defects, reqStatuses, defStatuses)

	if summary.TotalItems != 4 {
		t.Errorf("TotalItems = %d, want 4", summary.TotalItems)
	}
	if summary.CompletedItems != 2 {
		t.Errorf("CompletedItems = %d, want 2", summary.CompletedItems)
	}
	if summary.Percent != 50 {
		t.Errorf("Percent = %d, want 50", summary.Percent)
	}
	if summary.ShowEstimation {
		t.Error("estimation totals should be hidden when estimation is disabled")
	}
}

// Requirement and defect status sets run in parallel, so the breakdown groups
// them by name the way the merged board does.
func TestBuildSprintSummary_GroupsStatusesByName(t *testing.T) {
	reqStatuses, defStatuses := sprintSummaryStatuses()
	p := &model.Product{ID: "p1"}
	items := []*model.Requirement{{ID: "r1", StatusID: "req-open"}}
	defects := []*model.Defect{{ID: "d1", StatusID: "def-open"}}

	summary := buildSprintSummary(p, items, defects, reqStatuses, defStatuses)

	if len(summary.Statuses) != 1 {
		t.Fatalf("expected one grouped status, got %+v", summary.Statuses)
	}
	if summary.Statuses[0].Name != "Open" || summary.Statuses[0].Count != 2 {
		t.Errorf("unexpected status row: %+v", summary.Statuses[0])
	}
	if summary.Statuses[0].IsClosed {
		t.Error("Open should not be marked closed")
	}
}

func TestBuildSprintSummary_StatuslessItemsAreCounted(t *testing.T) {
	reqStatuses, defStatuses := sprintSummaryStatuses()
	p := &model.Product{ID: "p1"}
	items := []*model.Requirement{{ID: "r1", StatusID: ""}}

	summary := buildSprintSummary(p, items, nil, reqStatuses, defStatuses)

	if summary.TotalItems != 1 || summary.CompletedItems != 0 {
		t.Errorf("unexpected counts: total=%d completed=%d", summary.TotalItems, summary.CompletedItems)
	}
	if len(summary.Statuses) != 1 || summary.Statuses[0].Name != "No status" {
		t.Errorf("expected a 'No status' row, got %+v", summary.Statuses)
	}
}

func TestBuildSprintSummary_Estimation(t *testing.T) {
	reqStatuses, defStatuses := sprintSummaryStatuses()
	items := []*model.Requirement{
		{ID: "r1", StatusID: "req-open", Estimation: "3"},
		{ID: "r2", StatusID: "req-done", Estimation: "5"},
		// Unparseable values are skipped rather than counted as zero, so the
		// total stays honest about what it actually covers.
		{ID: "r3", StatusID: "req-open", Estimation: "L"},
		{ID: "r4", StatusID: "req-open", Estimation: ""},
	}

	t.Run("story points are summed", func(t *testing.T) {
		p := &model.Product{ID: "p1", EstimationMode: model.EstimationModeStoryPoints}
		summary := buildSprintSummary(p, items, nil, reqStatuses, defStatuses)

		if !summary.ShowEstimation {
			t.Fatal("expected estimation totals to be shown")
		}
		if summary.EstimationLabel != "points" {
			t.Errorf("EstimationLabel = %q, want %q", summary.EstimationLabel, "points")
		}
		if summary.TotalEstimation != "8" {
			t.Errorf("TotalEstimation = %q, want %q", summary.TotalEstimation, "8")
		}
		if summary.CompletedEstimation != "5" {
			t.Errorf("CompletedEstimation = %q, want %q", summary.CompletedEstimation, "5")
		}
	})

	t.Run("hours are summed", func(t *testing.T) {
		p := &model.Product{ID: "p1", EstimationMode: model.EstimationModeHours}
		summary := buildSprintSummary(p, items, nil, reqStatuses, defStatuses)

		if !summary.ShowEstimation || summary.EstimationLabel != "hours" {
			t.Errorf("unexpected estimation display: %+v", summary)
		}
	})

	t.Run("t-shirt sizes show counts only", func(t *testing.T) {
		p := &model.Product{ID: "p1", EstimationMode: model.EstimationModeTShirtSizes}
		summary := buildSprintSummary(p, items, nil, reqStatuses, defStatuses)

		if summary.ShowEstimation {
			t.Error("ordinal sizes cannot be summed, so no totals should be shown")
		}
		if summary.TotalItems != 4 {
			t.Errorf("TotalItems = %d, want 4", summary.TotalItems)
		}
	})
}

func TestBuildSprintSummary_EmptySprint(t *testing.T) {
	reqStatuses, defStatuses := sprintSummaryStatuses()
	summary := buildSprintSummary(&model.Product{ID: "p1"}, nil, nil, reqStatuses, defStatuses)

	if summary.TotalItems != 0 || summary.Percent != 0 {
		t.Errorf("unexpected summary for an empty sprint: %+v", summary)
	}
}

func TestParseEstimation(t *testing.T) {
	tests := []struct {
		in    string
		want  float64
		valid bool
	}{
		{"3", 3, true},
		{" 2.5 ", 2.5, true},
		{"0", 0, true},
		{"", 0, false},
		{"XL", 0, false},
		{"about 3", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseEstimation(tt.in)
		if ok != tt.valid || got != tt.want {
			t.Errorf("parseEstimation(%q) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.valid)
		}
	}
}

func TestSprintDateToRFC3339(t *testing.T) {
	if got := sprintDateToRFC3339("2026-08-03"); got != "2026-08-03T00:00:00Z" {
		t.Errorf("sprintDateToRFC3339 = %q, want %q", got, "2026-08-03T00:00:00Z")
	}
	// Values that already carry a time pass through untouched.
	full := "2026-08-03T09:30:00Z"
	if got := sprintDateToRFC3339(full); got != full {
		t.Errorf("sprintDateToRFC3339 = %q, want %q", got, full)
	}
	if got := sprintDateToRFC3339(""); got != "" {
		t.Errorf("sprintDateToRFC3339(\"\") = %q, want empty", got)
	}
}

func TestPlanningSide(t *testing.T) {
	closed := map[string]bool{"done": true}

	tests := []struct {
		name         string
		itemSprintID string
		statusID     string
		wantSide     int
		wantInclude  bool
	}{
		{"unassigned and open goes to the backlog", "", "open", planningSideBacklog, true},
		{"unassigned without a status goes to the backlog", "", "", planningSideBacklog, true},
		{"in this sprint goes to the sprint side", "sprint-1", "open", planningSideSprint, true},
		// A finished item still in the sprint must remain visible, otherwise
		// there is no way to drag it back out.
		{"finished but in this sprint stays visible", "sprint-1", "done", planningSideSprint, true},
		{"finished and unassigned is hidden", "", "done", planningSideBacklog, false},
		{"assigned to another sprint is hidden", "sprint-2", "open", planningSideBacklog, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			side, include := planningSide(tt.itemSprintID, tt.statusID, "sprint-1", closed)
			if include != tt.wantInclude {
				t.Fatalf("include = %v, want %v", include, tt.wantInclude)
			}
			if include && side != tt.wantSide {
				t.Errorf("side = %d, want %d", side, tt.wantSide)
			}
		})
	}
}

func TestIsOwnerOrAdmin(t *testing.T) {
	tests := []struct {
		name string
		user *model.User
		role string
		want bool
	}{
		{"admin", &model.User{IsAdmin: true}, "", true},
		{"owner", &model.User{}, model.RoleOwner, true},
		// Planning assigns a sprint, which is a full requirement update — the
		// service restricts that to owners, so developers must not see it.
		{"developer", &model.User{}, model.RoleDeveloper, false},
		{"viewer", &model.User{}, model.RoleViewer, false},
	}
	for _, tt := range tests {
		if got := isOwnerOrAdmin(tt.user, tt.role); got != tt.want {
			t.Errorf("%s: isOwnerOrAdmin = %v, want %v", tt.name, got, tt.want)
		}
	}
}
