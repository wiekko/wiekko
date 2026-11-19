package controller

import (
	"net/http/httptest"
	"testing"

	"nexmap.net/internal/store"
	"nexmap.net/web/templates/ui"
)

// parseState is a shorthand for driving parseDefectListState from a query string.
func parseState(t *testing.T, query string) (store.DefectFilter, store.ListParams, ui.ListState) {
	t.Helper()

	r := httptest.NewRequest("GET", "/products/p1/defects"+query, nil)
	return parseDefectListState(r, "/products/p1/defects")
}

func TestParseDefectListStateDefaults(t *testing.T) {
	filter, params, state := parseState(t, "")

	// Archived defects are hidden by default (FR-155); nothing else filters.
	if filter != (store.DefectFilter{Archived: store.ArchivedExclude}) {
		t.Errorf("expected only archived defects filtered out, got %+v", filter)
	}
	if params.Sort != "" {
		t.Errorf("sort = %q, want the entity default", params.Sort)
	}
	if params.Limit != defectsPageSize {
		t.Errorf("limit = %d, want %d", params.Limit, defectsPageSize)
	}
	if params.Offset != 0 {
		t.Errorf("offset = %d, want 0 for the first page", params.Offset)
	}
	if state.Page != 1 {
		t.Errorf("page = %d, want 1", state.Page)
	}
	if state.Filtered() {
		t.Error("an unfiltered request reported itself as filtered")
	}
}

func TestParseDefectListStateReadsFacets(t *testing.T) {
	filter, _, state := parseState(t,
		"?status_id=s1&criticality=high&sprint_id=sp1&environment_id=e1")

	want := store.DefectFilter{
		Criticality:   "high",
		StatusID:      "s1",
		SprintID:      "sp1",
		EnvironmentID: "e1",
		Archived:      store.ArchivedExclude,
	}
	if filter != want {
		t.Errorf("filter = %+v, want %+v", filter, want)
	}

	// The same values have to reach the view state, or the selects would render
	// blank while the list stayed filtered.
	if state.Filters["status_id"] != "s1" || state.Filters["criticality"] != "high" {
		t.Errorf("view state lost a facet: %+v", state.Filters)
	}
	if !state.Filtered() {
		t.Error("a request with facets did not report itself as filtered")
	}
}

func TestParseDefectListStateSortKeyIsAllowlisted(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"a known key is kept", "?sort=criticality", "criticality"},
		{"an unknown key falls back to the default", "?sort=nonsense", ""},
		// A stale bookmark must render the default view rather than an error,
		// and nothing a caller sends may reach the ORDER BY clause.
		{"an injection attempt falls back", "?sort=title;DROP+TABLE+defects", ""},
		{"an empty key falls back", "?sort=", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, params, state := parseState(t, tt.query)

			if params.Sort != tt.want {
				t.Errorf("params.Sort = %q, want %q", params.Sort, tt.want)
			}
			if state.Sort != tt.want {
				t.Errorf("state.Sort = %q, want %q", state.Sort, tt.want)
			}
		})
	}
}

func TestParseDefectListStateDirection(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"?sort=title&dir=desc", ui.SortDesc},
		{"?sort=title&dir=asc", ui.SortAsc},
		{"?sort=title", ui.SortAsc},
		{"?sort=title&dir=sideways", ui.SortAsc},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			_, params, state := parseState(t, tt.query)

			if params.Dir != tt.want {
				t.Errorf("params.Dir = %q, want %q", params.Dir, tt.want)
			}
			if state.Dir != tt.want {
				t.Errorf("state.Dir = %q, want %q", state.Dir, tt.want)
			}
		})
	}
}

func TestParseDefectListStatePaging(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantPage   int
		wantOffset int
	}{
		{"page 1 starts at zero", "?page=1", 1, 0},
		{"page 3 skips two pages", "?page=3", 3, 2 * defectsPageSize},
		{"a missing page reads as the first", "", 1, 0},
		// Junk in the URL must not produce a negative offset, which SQLite
		// rejects outright.
		{"a zero page clamps", "?page=0", 1, 0},
		{"a negative page clamps", "?page=-5", 1, 0},
		{"a non-numeric page clamps", "?page=abc", 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, params, state := parseState(t, tt.query)

			if state.Page != tt.wantPage {
				t.Errorf("page = %d, want %d", state.Page, tt.wantPage)
			}
			if params.Offset != tt.wantOffset {
				t.Errorf("offset = %d, want %d", params.Offset, tt.wantOffset)
			}
			if params.Offset < 0 {
				t.Errorf("offset %d is negative", params.Offset)
			}
		})
	}
}

func TestParseDefectListStateSearch(t *testing.T) {
	_, params, state := parseState(t, "?q=+gps+drift+")

	// Surrounding whitespace would otherwise become part of the LIKE pattern
	// and match nothing.
	if params.Search != "gps drift" {
		t.Errorf("search = %q, want it trimmed to %q", params.Search, "gps drift")
	}
	if state.Search != "gps drift" {
		t.Errorf("state search = %q, want %q", state.Search, "gps drift")
	}
	if !state.Filtered() {
		t.Error("a request with a search did not report itself as filtered")
	}
}

func TestParseDefectListStateBaseURLIsThePageURL(t *testing.T) {
	// The controls push their request URL into the address bar, so the base has
	// to be the page's own URL — not a separate fragment endpoint.
	_, _, state := parseState(t, "?sort=title")

	if state.BaseURL != "/products/p1/defects" {
		t.Errorf("BaseURL = %q, want the page URL", state.BaseURL)
	}
}

func TestParseDefectListStateRoundTripsThroughURLs(t *testing.T) {
	// The URLs the table emits must parse back into the state that produced
	// them, or a sort click would lose the filters it rendered with.
	_, _, state := parseState(t, "?sort=criticality&dir=desc&q=gps&status_id=s1&page=2")

	filter, params, next := parseState(t, "?"+state.Query().Encode())

	if params.Sort != "criticality" || params.Dir != ui.SortDesc {
		t.Errorf("sort did not survive the round trip: %q/%q", params.Sort, params.Dir)
	}
	if params.Search != "gps" {
		t.Errorf("search did not survive the round trip: %q", params.Search)
	}
	if filter.StatusID != "s1" {
		t.Errorf("filter did not survive the round trip: %q", filter.StatusID)
	}
	if next.Page != 2 {
		t.Errorf("page did not survive the round trip: %d", next.Page)
	}
}

// The archived facet (FR-155) reaches the filter and the view state; its
// default, hiding archived defects, stays out of the URL.
func TestParseDefectListStateArchivedFacet(t *testing.T) {
	for query, want := range map[string]struct {
		filter store.ArchivedFilter
		facet  string
	}{
		"":                  {store.ArchivedExclude, ""},
		"?archived=include": {store.ArchivedInclude, "include"},
		"?archived=only":    {store.ArchivedOnly, "only"},
		"?archived=bogus":   {store.ArchivedExclude, ""},
	} {
		filter, _, state := parseState(t, query)
		if filter.Archived != want.filter {
			t.Errorf("%q: filter.Archived = %q, want %q", query, filter.Archived, want.filter)
		}
		if state.Filters["archived"] != want.facet {
			t.Errorf("%q: facet = %q, want %q", query, state.Filters["archived"], want.facet)
		}
		if state.Filtered() != (want.facet != "") {
			t.Errorf("%q: Filtered() = %v", query, state.Filtered())
		}
	}
}
