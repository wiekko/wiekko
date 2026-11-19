package controller

import (
	"cmp"
	"net/http"
	"slices"
	"strings"

	"nexmap.net/web/templates/ui"
)

// rowTable sorts, searches, filters and pages the rows of a table view held
// in memory (UR-145). It backs the tables whose lists are small and whose
// columns are partly derived in Go — sprint item counts, campaign progress and
// verdicts, member names — so a SQL query per table would buy nothing. The
// server still sends one page at a time with the whole state in the URL, as
// UC-77 asks; it is the same contract as the SQL-backed tables, so the
// templates cannot tell the two apart.
type rowTable[T any] struct {
	// sorts maps each sortable column's key to a comparison by meaning
	// (UC-74), e.g. a lifecycle status by its place in the lifecycle.
	sorts map[string]func(a, b T) int
	// search lists the texts of a row the search box matches against: the
	// text columns as shown (UC-73).
	search func(T) []string
	// facets maps each facet's query parameter to whether a row matches a
	// value of it.
	facets map[string]func(row T, value string) bool
}

// readState reads the table's state out of the request; nothing in it can
// fail (see listQuery).
func (t rowTable[T]) readState(r *http.Request, baseURL string) ui.ListState {
	q := r.URL.Query()
	sortKey, dir, search, page := listQuery(q, func(key string) bool {
		_, ok := t.sorts[key]
		return ok
	})
	filters := make(map[string]string, len(t.facets))
	for name := range t.facets {
		filters[name] = q.Get(name)
	}
	return ui.ListState{BaseURL: baseURL, Sort: sortKey, Dir: dir, Search: search, Page: page, Filters: filters}
}

// apply narrows rows to the state's search and facets, sorts them and returns
// the page the state asks for with the number of matching rows. A page past
// the end moves the state to the last page. Rows keep their given order where
// the sort sees them as equal, and when no column is sorted.
func (t rowTable[T]) apply(rows []T, state *ui.ListState) (page []T, total int) {
	needle := strings.ToLower(strings.TrimSpace(state.Search))
	matched := make([]T, 0, len(rows))
	for _, row := range rows {
		if t.matches(row, state.Filters, needle) {
			matched = append(matched, row)
		}
	}

	if less, ok := t.sorts[state.Sort]; ok && state.Sort != "" {
		desc := state.Dir == ui.SortDesc
		slices.SortStableFunc(matched, func(a, b T) int {
			if desc {
				return less(b, a)
			}
			return less(a, b)
		})
	}

	total = len(matched)
	// ListState reads pages below 1 as page 1.
	if state.Page < 1 {
		state.Page = 1
	}
	pageBeyondEnd(state, total)
	start := (state.Page - 1) * tablePageSize
	if start >= total {
		return nil, total
	}
	end := min(start+tablePageSize, total)
	return matched[start:end], total
}

func (t rowTable[T]) matches(row T, filters map[string]string, needle string) bool {
	for name, value := range filters {
		if value == "" {
			continue
		}
		if match, ok := t.facets[name]; ok && !match(row, value) {
			return false
		}
	}
	if needle == "" || t.search == nil {
		return true
	}
	for _, text := range t.search(row) {
		if strings.Contains(strings.ToLower(text), needle) {
			return true
		}
	}
	return false
}

// byText compares texts the way people read them: case-insensitively.
func byText(a, b string) int {
	return cmp.Compare(strings.ToLower(a), strings.ToLower(b))
}

// byRank compares values by their place in order, e.g. a lifecycle; values
// not in it come last.
func byRank(order ...string) func(a, b string) int {
	rank := make(map[string]int, len(order))
	for i, v := range order {
		rank[v] = i
	}
	pos := func(v string) int {
		if r, ok := rank[v]; ok {
			return r
		}
		return len(order)
	}
	return func(a, b string) int { return cmp.Compare(pos(a), pos(b)) }
}

// byOptional compares values that may be unset ("" or zero) so that unset
// ones come last in ascending order, the way an unset due date belongs after
// every set one rather than before.
func byOptional[V cmp.Ordered](a, b V) int {
	var zero V
	switch {
	case a == zero && b == zero:
		return 0
	case a == zero:
		return 1
	case b == zero:
		return -1
	}
	return cmp.Compare(a, b)
}

// byPriority compares priorities: 1 is the highest, 0 means none and comes
// last.
func byPriority(a, b int) int { return byOptional(a, b) }

// optionalRef matches the facet value of an optional reference: "none"
// selects rows without one.
func optionalRef(id, value string) bool {
	if value == ui.FilterNone {
		return id == ""
	}
	return id == value
}
