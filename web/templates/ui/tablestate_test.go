// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package ui

import (
	"net/url"
	"strings"
	"testing"
)

// parseQuery pulls the query string off a generated URL so assertions can look
// at parameters rather than at string formatting.
func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u.Query()
}

func fullState() ListState {
	return ListState{
		BaseURL: "/products/p1/defects",
		Sort:    "criticality",
		Dir:     SortAsc,
		Search:  "gps",
		Page:    3,
		Filters: map[string]string{
			"status_id":   "s1",
			"criticality": "high",
			"sprint_id":   "",
		},
	}
}

func TestListStateURLOmitsDefaults(t *testing.T) {
	state := ListState{BaseURL: "/products/p1/defects", Page: 1}

	if got := state.URL(); got != "/products/p1/defects" {
		t.Errorf("an untouched table should have a bare URL, got %q", got)
	}
}

func TestListStateURLIsStable(t *testing.T) {
	// Filters live in a map, so without deliberate ordering the same state
	// would render a different query string on each call.
	state := fullState()
	first := state.URL()
	for range 20 {
		if got := state.URL(); got != first {
			t.Fatalf("URL is not stable: %q then %q", first, got)
		}
	}
}

func TestListStateSortURLPreservesSearchAndFilters(t *testing.T) {
	q := parseQuery(t, fullState().SortURL("title"))

	if got := q.Get("q"); got != "gps" {
		t.Errorf("sorting dropped the search: q = %q", got)
	}
	if got := q.Get("status_id"); got != "s1" {
		t.Errorf("sorting dropped a filter: status_id = %q", got)
	}
	if got := q.Get("criticality"); got != "high" {
		t.Errorf("sorting dropped a filter: criticality = %q", got)
	}
	if got := q.Get("sort"); got != "title" {
		t.Errorf("sort = %q, want title", got)
	}
}

func TestListStateSortURLResetsPage(t *testing.T) {
	// Row 51 of one order has nothing to do with row 51 of another, so a
	// re-sort has to return to the first page.
	q := parseQuery(t, fullState().SortURL("title"))

	if q.Has("page") {
		t.Errorf("sorting kept page = %q, want a reset to page 1", q.Get("page"))
	}
}

func TestListStateSortURLCyclesThroughUnsorted(t *testing.T) {
	// Repeatedly clicking one column walks asc → desc → unsorted → asc. The
	// third step is what makes the default order reachable again; without it a
	// table can never be un-sorted once it has been sorted.
	state := ListState{BaseURL: "/x"}

	first := parseQuery(t, state.SortURL("title"))
	if first.Get("sort") != "title" || first.Get("dir") != SortAsc {
		t.Fatalf("first click gave sort=%q dir=%q, want title/asc", first.Get("sort"), first.Get("dir"))
	}

	state.Sort, state.Dir = "title", SortAsc
	second := parseQuery(t, state.SortURL("title"))
	if second.Get("dir") != SortDesc {
		t.Fatalf("second click gave dir = %q, want desc", second.Get("dir"))
	}

	state.Dir = SortDesc
	third := parseQuery(t, state.SortURL("title"))
	if third.Has("sort") || third.Has("dir") {
		t.Fatalf("third click left sort=%q dir=%q, want the sort dropped entirely",
			third.Get("sort"), third.Get("dir"))
	}

	// And back to the start.
	state.Sort, state.Dir = "", ""
	fourth := parseQuery(t, state.SortURL("title"))
	if fourth.Get("sort") != "title" || fourth.Get("dir") != SortAsc {
		t.Errorf("fourth click gave sort=%q dir=%q, want title/asc", fourth.Get("sort"), fourth.Get("dir"))
	}
}

func TestListStateSortURLOtherColumnStartsAscending(t *testing.T) {
	// Switching columns must not inherit the previous column's direction, nor
	// land on the unsorted step of its cycle.
	for _, dir := range []string{SortAsc, SortDesc} {
		state := ListState{BaseURL: "/x", Sort: "title", Dir: dir}
		q := parseQuery(t, state.SortURL("priority"))

		if q.Get("sort") != "priority" {
			t.Errorf("from dir=%s, sort = %q, want priority", dir, q.Get("sort"))
		}
		if q.Get("dir") != SortAsc {
			t.Errorf("from dir=%s, new column started at %q, want asc", dir, q.Get("dir"))
		}
	}
}

func TestListStateClearingTheSortKeepsSearchAndFilters(t *testing.T) {
	// The third click undoes the sort, not the filtering.
	state := fullState()
	state.Sort, state.Dir = "title", SortDesc

	q := parseQuery(t, state.SortURL("title"))

	if q.Has("sort") {
		t.Errorf("sort survived the unsort step: %q", q.Get("sort"))
	}
	if got := q.Get("q"); got != "gps" {
		t.Errorf("unsorting dropped the search: q = %q", got)
	}
	if got := q.Get("status_id"); got != "s1" {
		t.Errorf("unsorting dropped a filter: status_id = %q", got)
	}
}

func TestListStateSortURLDoesNotDuplicateParams(t *testing.T) {
	// The helper this replaced appended "&sort=..." to whatever it was given,
	// so sorting twice produced two sort parameters.
	once := fullState().SortURL("title")
	twice := ListState{
		BaseURL: "/products/p1/defects",
		Sort:    "title",
		Dir:     SortAsc,
		Search:  "gps",
		Filters: map[string]string{"status_id": "s1", "criticality": "high"},
	}.SortURL("title")

	for _, raw := range []string{once, twice} {
		if n := strings.Count(raw, "sort="); n != 1 {
			t.Errorf("%q contains %d sort parameters, want exactly 1", raw, n)
		}
		if n := strings.Count(raw, "dir="); n != 1 {
			t.Errorf("%q contains %d dir parameters, want exactly 1", raw, n)
		}
	}
}

func TestListStatePageURLPreservesEverythingElse(t *testing.T) {
	q := parseQuery(t, fullState().PageURL(5))

	if got := q.Get("page"); got != "5" {
		t.Errorf("page = %q, want 5", got)
	}
	if got := q.Get("sort"); got != "criticality" {
		t.Errorf("paging dropped the sort: sort = %q", got)
	}
	if got := q.Get("dir"); got != SortAsc {
		t.Errorf("paging dropped the direction: dir = %q", got)
	}
	if got := q.Get("q"); got != "gps" {
		t.Errorf("paging dropped the search: q = %q", got)
	}
	if got := q.Get("status_id"); got != "s1" {
		t.Errorf("paging dropped a filter: status_id = %q", got)
	}
}

func TestListStateFilterURL(t *testing.T) {
	state := fullState()

	t.Run("setting a filter resets the page but keeps the sort", func(t *testing.T) {
		q := parseQuery(t, state.FilterURL("sprint_id", "sp1"))

		if got := q.Get("sprint_id"); got != "sp1" {
			t.Errorf("sprint_id = %q, want sp1", got)
		}
		if q.Has("page") {
			t.Errorf("filtering kept page = %q, want a reset", q.Get("page"))
		}
		if got := q.Get("sort"); got != "criticality" {
			t.Errorf("filtering dropped the sort: sort = %q", got)
		}
	})

	t.Run("clearing one filter leaves the others", func(t *testing.T) {
		q := parseQuery(t, state.FilterURL("status_id", ""))

		if q.Has("status_id") {
			t.Errorf("status_id survived being cleared: %q", q.Get("status_id"))
		}
		if got := q.Get("criticality"); got != "high" {
			t.Errorf("clearing one filter dropped another: criticality = %q", got)
		}
	})

	t.Run("does not mutate the receiver", func(t *testing.T) {
		before := state.Filters["status_id"]
		_ = state.FilterURL("status_id", "changed")
		if state.Filters["status_id"] != before {
			t.Errorf("FilterURL mutated the original state: status_id = %q, want %q",
				state.Filters["status_id"], before)
		}
	})
}

func TestListStateClearURLKeepsSortOnly(t *testing.T) {
	q := parseQuery(t, fullState().ClearURL())

	if q.Has("q") || q.Has("status_id") || q.Has("criticality") {
		t.Errorf("clearing left a search or filter behind: %v", q)
	}
	if got := q.Get("sort"); got != "criticality" {
		t.Errorf("clearing dropped the sort: sort = %q", got)
	}
}

func TestListStateSortedBy(t *testing.T) {
	state := ListState{Sort: "title", Dir: SortDesc}

	if active, dir := state.SortedBy("title"); !active || dir != SortDesc {
		t.Errorf("SortedBy(title) = %v/%q, want true/desc", active, dir)
	}
	if active, _ := state.SortedBy("priority"); active {
		t.Error("SortedBy(priority) reported active for an unsorted column")
	}

	// A direction that never got set, or arrived mangled from a hand-edited
	// URL, has to read as ascending rather than leak through.
	bogus := ListState{Sort: "title", Dir: "sideways"}
	if _, dir := bogus.SortedBy("title"); dir != SortAsc {
		t.Errorf("a bogus direction rendered as %q, want asc", dir)
	}
}

func TestListStateFiltered(t *testing.T) {
	tests := []struct {
		name  string
		state ListState
		want  bool
	}{
		{"nothing set", ListState{}, false},
		{"empty filter values only", ListState{Filters: map[string]string{"status_id": ""}}, false},
		{"search set", ListState{Search: "gps"}, true},
		{"filter set", ListState{Filters: map[string]string{"status_id": "s1"}}, true},
		// Sorting is not filtering: a sorted table still shows every row, so it
		// must not trigger the "nothing matches" empty state.
		{"sort only", ListState{Sort: "title", Dir: SortAsc}, false},
		{"page only", ListState{Page: 4}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Filtered(); got != tt.want {
				t.Errorf("Filtered() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListStateHiddenInputsCarrySort(t *testing.T) {
	// htmx ships only the inputs it is told to include, so the sort order has
	// to survive as hidden fields or a filter change would silently reset it.
	got := fullState().HiddenInputs()

	if got["sort"] != "criticality" {
		t.Errorf("hidden sort = %q, want criticality", got["sort"])
	}
	if got["dir"] != SortAsc {
		t.Errorf("hidden dir = %q, want asc", got["dir"])
	}

	// Page must never be a hidden input: including it would stop a search or
	// filter change from returning to page 1.
	if _, ok := got["page"]; ok {
		t.Error("page must not round-trip as a hidden input")
	}

	if len(ListState{}.HiddenInputs()) != 0 {
		t.Error("an unsorted table should emit no hidden inputs")
	}
}

func TestListStateEscapesQueryValues(t *testing.T) {
	state := ListState{
		BaseURL: "/products/p1/defects",
		Search:  "100% & more",
		Filters: map[string]string{"status_id": "a b"},
	}

	q := parseQuery(t, state.URL())
	if got := q.Get("q"); got != "100% & more" {
		t.Errorf("search did not survive a round trip: %q", got)
	}
	if got := q.Get("status_id"); got != "a b" {
		t.Errorf("filter did not survive a round trip: %q", got)
	}
}
