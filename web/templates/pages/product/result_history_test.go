package product

import (
	"context"
	"strings"
	"testing"

	"nexmap.net/internal/model"
)

func renderToString(t *testing.T, render func(*strings.Builder) error) string {
	t.Helper()
	var sb strings.Builder
	if err := render(&sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func resultMenuProps(canExecute bool) CampaignResultMenuProps {
	return CampaignResultMenuProps{
		ProductID:   "product-001",
		CampaignID:  "campaign-001",
		Result:      &model.CampaignResult{ID: "cr-001", Status: model.CampaignResultStatusFailed},
		TestCase:    &model.TestCase{ID: "tc-001", Title: "Login rejects a bad password"},
		Environment: "linux",
		CanExecute:  canExecute,
		Events: []ResultEventView{
			{UserName: "Alice", ToStatus: model.CampaignResultStatusPassed, At: "1 Aug 2026, 10:00"},
			{
				UserName: "Bob", FromStatus: model.CampaignResultStatusPassed,
				ToStatus: model.CampaignResultStatusFailed, At: "2 Aug 2026, 10:00",
				DefectID: "defect-001", DefectTitle: "Login is broken", IsOverride: true,
			},
		},
	}
}

// A viewer, or anyone looking at a closed campaign, still gets the history — it
// is the panel's whole purpose — but no way to set a status.
func TestResultPanelHidesTheStatusPickerWithoutExecuteRights(t *testing.T) {
	out := renderToString(t, func(sb *strings.Builder) error {
		return CampaignResultStatusMenu(resultMenuProps(false)).Render(context.Background(), sb)
	})

	if strings.Contains(out, "/results/cr-001/status") {
		t.Fatalf("read-only panel offers a status control: %s", out)
	}
	for _, want := range []string{"History", "Alice", "Bob", "Login is broken"} {
		if !strings.Contains(out, want) {
			t.Fatalf("read-only panel is missing %q", want)
		}
	}
}

func TestResultPanelShowsTheStatusPickerForTesters(t *testing.T) {
	out := renderToString(t, func(sb *strings.Builder) error {
		return CampaignResultStatusMenu(resultMenuProps(true)).Render(context.Background(), sb)
	})

	if !strings.Contains(out, "/results/cr-001/status") {
		t.Fatalf("tester panel offers no status control: %s", out)
	}
	if !strings.Contains(out, "History") {
		t.Fatal("tester panel dropped the history")
	}
}

// The wording is deliberate: a changed verdict is reported as a change, never
// as a conflict, so the log does not discourage testers from correcting results.
func TestResultHistoryWordingStaysNeutral(t *testing.T) {
	out := renderToString(t, func(sb *strings.Builder) error {
		return CampaignResultStatusMenu(resultMenuProps(false)).Render(context.Background(), sb)
	})

	if !strings.Contains(out, "Changed by Bob") || !strings.Contains(out, "Set by Alice") {
		t.Fatalf("unexpected actor phrasing: %s", out)
	}
	for _, banned := range []string{"conflict", "Conflict", "override", "Override"} {
		if strings.Contains(out, banned) {
			t.Fatalf("blame-shaped wording %q leaked into the panel", banned)
		}
	}
}

// A lifecycle entry carries no result fields, and a result entry carries no
// audit pointer. The feed must render both without reaching through a nil.
func TestCampaignHistoryRendersMixedEntries(t *testing.T) {
	out := renderToString(t, func(sb *strings.Builder) error {
		return CampaignHistoryTabContent(CampaignHistoryProps{
			ProductID:  "product-001",
			CampaignID: "campaign-001",
			Filter:     CampaignHistoryFilterAll,
			Entries: []CampaignHistoryEntry{
				{
					Kind: "result", At: "2 Aug 2026, 10:00", UserName: "Bob",
					TestCase: "Login rejects a bad password", Environment: "linux",
					FromStatus: model.CampaignResultStatusPassed,
					ToStatus:   model.CampaignResultStatusFailed,
					DefectID:   "defect-001", DefectTitle: "Login is broken",
					IsOverride: true,
				},
				// Audit is nil here on purpose: the feed must survive it.
				{Kind: "audit", At: "1 Aug 2026, 09:00", UserName: "Alice"},
			},
		}).Render(context.Background(), sb)
	})

	for _, want := range []string{"Bob", "Alice", "Login rejects a bad password", "Login is broken", "Changed the result of"} {
		if !strings.Contains(out, want) {
			t.Fatalf("feed is missing %q", want)
		}
	}
}

func TestCampaignHistoryFilterMarksTheActiveChoice(t *testing.T) {
	out := renderToString(t, func(sb *strings.Builder) error {
		return CampaignHistoryTabContent(CampaignHistoryProps{
			ProductID: "product-001", CampaignID: "campaign-001",
			Filter: CampaignHistoryFilterRetests,
		}).Render(context.Background(), sb)
	})

	if !strings.Contains(out, "filter=retests") || !strings.Contains(out, "filter=all") {
		t.Fatalf("filter controls are incomplete: %s", out)
	}
	// The empty state must explain the filter, not the campaign.
	if !strings.Contains(out, "re-testing") {
		t.Fatalf("empty state does not reflect the active filter: %s", out)
	}
}

func TestDefectVerificationShowsTheClosingPass(t *testing.T) {
	out := renderToString(t, func(sb *strings.Builder) error {
		return defectVerification(DefectOverviewProps{
			ProductID: "product-001",
			Defect:    &model.Defect{ID: "defect-001", Title: "Login is broken"},
			Verification: []DefectVerificationGroup{{
				TestCase: "Login rejects a bad password", Environment: "linux",
				CampaignName: "Release 2.4", Verified: true,
				Events: []ResultEventView{
					{UserName: "Bob", ToStatus: model.CampaignResultStatusFailed, At: "1 Aug 2026, 10:00"},
					{UserName: "System", ToStatus: model.CampaignResultStatusRetest, At: "2 Aug 2026, 10:00"},
					{UserName: "Carol", ToStatus: model.CampaignResultStatusPassed, At: "3 Aug 2026, 10:00", IsOverride: true},
				},
			}},
		}).Render(context.Background(), sb)
	})

	for _, want := range []string{"Verification", "Release 2.4", "Verified", "Bob", "System", "Carol"} {
		if !strings.Contains(out, want) {
			t.Fatalf("verification section is missing %q", want)
		}
	}
}

// An unverified defect is a real answer, so the section stays visible and says
// so rather than disappearing.
func TestDefectVerificationStatesWhenNothingIsLinked(t *testing.T) {
	out := renderToString(t, func(sb *strings.Builder) error {
		return defectVerification(DefectOverviewProps{
			ProductID: "product-001",
			Defect:    &model.Defect{ID: "defect-001", Title: "Login is broken"},
		}).Render(context.Background(), sb)
	})

	if !strings.Contains(out, "Verification") || !strings.Contains(out, "No campaign result has been linked") {
		t.Fatalf("empty verification section is wrong: %s", out)
	}
}
