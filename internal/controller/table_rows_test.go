// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"cmp"
	"fmt"
	"net/http/httptest"
	"slices"
	"testing"

	"nexmap.net/web/templates/ui"
)

type tableRow struct {
	name, status, ref, due string
	n                      int
}

var testRowTable = rowTable[tableRow]{
	sorts: map[string]func(a, b tableRow) int{
		"name":   func(a, b tableRow) int { return byText(a.name, b.name) },
		"status": func(a, b tableRow) int { return byRank("open", "doing", "done")(a.status, b.status) },
		"due":    func(a, b tableRow) int { return byOptional(a.due, b.due) },
		"n":      func(a, b tableRow) int { return cmp.Compare(a.n, b.n) },
	},
	search: func(r tableRow) []string { return []string{r.name} },
	facets: map[string]func(r tableRow, v string) bool{
		"status": func(r tableRow, v string) bool { return r.status == v },
		"ref":    func(r tableRow, v string) bool { return optionalRef(r.ref, v) },
	},
}

var testRows = []tableRow{
	{name: "beta", status: "done", ref: "m1", due: "2026-03-01", n: 2},
	{name: "Alpha", status: "open", due: "", n: 1},
	{name: "gamma", status: "doing", ref: "m1", due: "2026-01-01", n: 2},
	{name: "delta 100%", status: "open", ref: "m2", due: "2026-02-01", n: 3},
}

func rowNames(rows []tableRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.name
	}
	return out
}

func TestRowTableReadsStateFromTheURL(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?sort=due&dir=desc&q=+al+&page=2&status=open&ref=none&bogus=1", nil)
	state := testRowTable.readState(r, "/x")
	if state.Sort != "due" || state.Dir != ui.SortDesc || state.Search != "al" || state.Page != 2 {
		t.Errorf("state = %+v", state)
	}
	if state.Filters["status"] != "open" || state.Filters["ref"] != "none" {
		t.Errorf("facets = %v", state.Filters)
	}
	if _, ok := state.Filters["bogus"]; ok {
		t.Error("a parameter that is no facet became one")
	}

	r = httptest.NewRequest("GET", "/x?sort=created;drop&page=-3", nil)
	if state := testRowTable.readState(r, "/x"); state.Sort != "" || state.Page != 1 {
		t.Errorf("an unknown sort key or a bad page was not corrected: %+v", state)
	}
}

func TestRowTableApply(t *testing.T) {
	tests := []struct {
		name  string
		state ui.ListState
		want  []string
	}{
		{"keeps the given order unsorted", ui.ListState{}, []string{"beta", "Alpha", "gamma", "delta 100%"}},
		{"sorts text case-insensitively", ui.ListState{Sort: "name"}, []string{"Alpha", "beta", "delta 100%", "gamma"}},
		{"sorts descending", ui.ListState{Sort: "name", Dir: ui.SortDesc}, []string{"gamma", "delta 100%", "beta", "Alpha"}},
		{"sorts by meaning", ui.ListState{Sort: "status"}, []string{"Alpha", "delta 100%", "gamma", "beta"}},
		{"puts unset values last", ui.ListState{Sort: "due"}, []string{"gamma", "delta 100%", "beta", "Alpha"}},
		{"keeps ties in the given order", ui.ListState{Sort: "n"}, []string{"Alpha", "beta", "gamma", "delta 100%"}},
		{"searches case-insensitively", ui.ListState{Search: "ALP"}, []string{"Alpha"}},
		{"searches literally", ui.ListState{Search: "100%"}, []string{"delta 100%"}},
		{"filters by facet", ui.ListState{Filters: map[string]string{"status": "open"}}, []string{"Alpha", "delta 100%"}},
		{"filters by an unset reference", ui.ListState{Filters: map[string]string{"ref": ui.FilterNone}}, []string{"Alpha"}},
		{"combines facets and search", ui.ListState{Search: "a", Filters: map[string]string{"ref": "m1"}}, []string{"beta", "gamma"}},
		{"ignores unknown facets", ui.ListState{Filters: map[string]string{"nope": "x"}}, []string{"beta", "Alpha", "gamma", "delta 100%"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.state
			got, total := testRowTable.apply(testRows, &state)
			if !slices.Equal(rowNames(got), tt.want) || total != len(tt.want) {
				t.Errorf("got %v (total %d), want %v", rowNames(got), total, tt.want)
			}
		})
	}
	if !slices.Equal(rowNames(testRows), []string{"beta", "Alpha", "gamma", "delta 100%"}) {
		t.Error("sorting reordered the caller's slice")
	}
}

func TestRowTablePages(t *testing.T) {
	rows := make([]tableRow, 0, tablePageSize+5)
	for i := range tablePageSize + 5 {
		rows = append(rows, tableRow{name: fmt.Sprintf("row %03d", i)})
	}

	state := ui.ListState{Page: 2}
	page, total := testRowTable.apply(rows, &state)
	if total != tablePageSize+5 || len(page) != 5 || page[0].name != fmt.Sprintf("row %03d", tablePageSize) {
		t.Errorf("second page: %d rows of %d, first %v", len(page), total, rowNames(page[:1]))
	}

	// A page past the end lands on the last page instead of an empty one.
	state = ui.ListState{Page: 9}
	page, _ = testRowTable.apply(rows, &state)
	if state.Page != 2 || len(page) != 5 {
		t.Errorf("page 9 of 2 moved to page %d with %d rows", state.Page, len(page))
	}

	state = ui.ListState{Page: 3, Search: "nothing"}
	if page, total := testRowTable.apply(rows, &state); len(page) != 0 || total != 0 || state.Page != 3 {
		t.Errorf("no match: %d rows, total %d, page %d", len(page), total, state.Page)
	}
}
