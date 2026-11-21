// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package model

import "testing"

func TestCampaignVerdict(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		want   string
	}{
		{
			name:   "no results at all",
			counts: map[string]int{},
			want:   CampaignVerdictPassed,
		},
		{
			name:   "every result passed",
			counts: map[string]int{CampaignResultStatusPassed: 3},
			want:   CampaignVerdictPassed,
		},
		{
			name:   "skipped results are ignored",
			counts: map[string]int{CampaignResultStatusPassed: 2, CampaignResultStatusSkipped: 5},
			want:   CampaignVerdictPassed,
		},
		{
			name:   "all skipped still passes",
			counts: map[string]int{CampaignResultStatusSkipped: 4},
			want:   CampaignVerdictPassed,
		},
		{
			name:   "a failure fails the campaign",
			counts: map[string]int{CampaignResultStatusPassed: 9, CampaignResultStatusFailed: 1},
			want:   CampaignVerdictFailed,
		},
		{
			name:   "pending leaves it incomplete",
			counts: map[string]int{CampaignResultStatusPassed: 1, CampaignResultStatusPending: 1},
			want:   CampaignVerdictIncomplete,
		},
		{
			name:   "blocked leaves it incomplete",
			counts: map[string]int{CampaignResultStatusPassed: 1, CampaignResultStatusBlocked: 1},
			want:   CampaignVerdictIncomplete,
		},
		{
			name:   "awaiting re-test leaves it incomplete",
			counts: map[string]int{CampaignResultStatusPassed: 1, CampaignResultStatusRetest: 1},
			want:   CampaignVerdictIncomplete,
		},
		{
			name:   "incomplete outranks a failure",
			counts: map[string]int{CampaignResultStatusFailed: 1, CampaignResultStatusPending: 1},
			want:   CampaignVerdictIncomplete,
		},
		{
			name:   "a zeroed count does not count",
			counts: map[string]int{CampaignResultStatusPassed: 1, CampaignResultStatusPending: 0},
			want:   CampaignVerdictPassed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CampaignVerdict(tt.counts); got != tt.want {
				t.Fatalf("CampaignVerdict(%v) = %q, want %q", tt.counts, got, tt.want)
			}
		})
	}
}

func TestIsIncompleteResultStatus(t *testing.T) {
	incomplete := []string{
		CampaignResultStatusPending,
		CampaignResultStatusBlocked,
		CampaignResultStatusRetest,
	}
	settled := []string{
		CampaignResultStatusPassed,
		CampaignResultStatusFailed,
		CampaignResultStatusSkipped,
	}

	for _, status := range incomplete {
		if !IsIncompleteResultStatus(status) {
			t.Errorf("IsIncompleteResultStatus(%q) = false, want true", status)
		}
	}
	for _, status := range settled {
		if IsIncompleteResultStatus(status) {
			t.Errorf("IsIncompleteResultStatus(%q) = true, want false", status)
		}
	}
}

func TestValidCampaignResultStatus(t *testing.T) {
	for _, status := range []string{
		CampaignResultStatusPending, CampaignResultStatusPassed, CampaignResultStatusFailed,
		CampaignResultStatusBlocked, CampaignResultStatusSkipped, CampaignResultStatusRetest,
	} {
		if !ValidCampaignResultStatus(status) {
			t.Errorf("ValidCampaignResultStatus(%q) = false, want true", status)
		}
	}
	for _, status := range []string{"", "done", "re-test", "PASSED"} {
		if ValidCampaignResultStatus(status) {
			t.Errorf("ValidCampaignResultStatus(%q) = true, want false", status)
		}
	}
}
