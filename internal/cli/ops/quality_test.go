// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package ops

import (
	"reflect"
	"testing"
)

// TestSortDefectsByPriority checks the CLI's defect order (DF-2): most severe
// first, then the lowest priority number, with unprioritised defects (0) last.
func TestSortDefectsByPriority(t *testing.T) {
	list := []DefectSummary{
		{Identifier: "DF-1", Criticality: "high", Priority: 0},
		{Identifier: "DF-2", Criticality: "high", Priority: 3},
		{Identifier: "DF-3", Criticality: "critical", Priority: 5},
		{Identifier: "DF-4", Criticality: "high", Priority: 1},
	}
	sortDefects(list)
	var got []string
	for _, d := range list {
		got = append(got, d.Identifier)
	}
	if want := []string{"DF-3", "DF-4", "DF-2", "DF-1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}
