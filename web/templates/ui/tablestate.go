package ui

import (
	"net/url"
	"sort"
	"strconv"
)

// Query parameter names shared by every sortable, filterable table view.
const (
	ParamSort   = "sort"
	ParamDir    = "dir"
	ParamSearch = "q"
	ParamPage   = "page"
)

// ListState is the complete URL-visible state of a table view: how it is
// sorted, what it is filtered to, and which page is showing. Every control on
// the table derives its href from this one value, which is what keeps the
// controls from clobbering each other — sorting preserves the filters, paging
// preserves the sort, and no parameter is ever written twice.
//
// It lives in a hand-written file rather than a .templ because *_templ.go is
// generated and gitignored, so anything worth testing has to sit beside the
// template rather than inside it.
type ListState struct {
	BaseURL string            // path only, e.g. "/products/p1/defects/table"
	Sort    string            // canonical sort key; "" means the default order
	Dir     string            // "asc" or "desc"
	Search  string            // global search text
	Page    int               // 1-based; values below 1 read as 1
	Filters map[string]string // param name -> value; "" means the filter is off
}

// Query renders the state as a canonical parameter set. Defaults are omitted so
// an untouched table has a clean URL: no sort, no empty search, no "page=1".
func (s ListState) Query() url.Values {
	q := url.Values{}
	if s.Sort != "" {
		q.Set(ParamSort, s.Sort)
		q.Set(ParamDir, s.direction())
	}
	if s.Search != "" {
		q.Set(ParamSearch, s.Search)
	}
	// Filter keys are emitted in sorted order so the same state always produces
	// the same URL — otherwise map iteration would reshuffle the query string
	// between renders and defeat both caching and template tests.
	names := make([]string, 0, len(s.Filters))
	for name := range s.Filters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if v := s.Filters[name]; v != "" {
			q.Set(name, v)
		}
	}
	if s.Page > 1 {
		q.Set(ParamPage, strconv.Itoa(s.Page))
	}
	return q
}

// URL renders the state as a complete link.
func (s ListState) URL() string {
	if q := s.Query().Encode(); q != "" {
		return s.BaseURL + "?" + q
	}
	return s.BaseURL
}

// SortURL returns the link for clicking the given column's header.
//
// A column cycles ascending → descending → unsorted, so a third click undoes
// the sort and returns the list to its natural order. Without that step there
// is no way back: once any column is sorted the default order becomes
// unreachable short of editing the URL. Clicking a different column always
// starts it ascending.
//
// Every step returns to page 1, because row 51 of the old order has nothing to
// do with row 51 of the new one.
func (s ListState) SortURL(field string) string {
	next := s
	next.Page = 1

	switch {
	case s.Sort != field:
		next.Sort = field
		next.Dir = SortAsc
	case s.direction() == SortAsc:
		next.Dir = SortDesc
	default:
		next.Sort = ""
		next.Dir = ""
	}
	return next.URL()
}

// PageURL returns the link for a page of the current result set, preserving the
// sort and every filter.
func (s ListState) PageURL(page int) string {
	next := s
	next.Page = page
	return next.URL()
}

// FilterURL returns the link for setting one filter to value ("" clears it).
// Narrowing the result set resets to page 1: the page the user is on may not
// exist once the filter applies.
func (s ListState) FilterURL(name, value string) string {
	next := s
	next.Filters = make(map[string]string, len(s.Filters)+1)
	for k, v := range s.Filters {
		next.Filters[k] = v
	}
	if value == "" {
		delete(next.Filters, name)
	} else {
		next.Filters[name] = value
	}
	next.Page = 1
	return next.URL()
}

// ClearURL returns the link that drops the search and every filter but keeps
// the sort order. It backs the "clear filters" affordance on an empty result.
func (s ListState) ClearURL() string {
	next := s
	next.Search = ""
	next.Filters = nil
	next.Page = 1
	return next.URL()
}

// SortedBy reports whether field is the active sort column and in which
// direction, for rendering the header's indicator.
func (s ListState) SortedBy(field string) (bool, string) {
	if s.Sort != field {
		return false, SortAsc
	}
	return true, s.direction()
}

// Filtered reports whether any search or filter is narrowing the list. It tells
// an empty result set ("nothing matches") apart from an empty table ("nothing
// exists yet"), which need different empty states.
func (s ListState) Filtered() bool {
	if s.Search != "" {
		return true
	}
	for _, v := range s.Filters {
		if v != "" {
			return true
		}
	}
	return false
}

// HiddenInputs returns the state that a form-driven control must round-trip as
// hidden fields. htmx sends only the inputs it includes, so without these a
// filter change would silently drop the active sort order.
func (s ListState) HiddenInputs() map[string]string {
	out := map[string]string{}
	if s.Sort != "" {
		out[ParamSort] = s.Sort
		out[ParamDir] = s.direction()
	}
	return out
}

// direction normalises Dir so callers never have to defend against an empty or
// bogus value arriving from a hand-edited URL.
func (s ListState) direction() string {
	if s.Dir == SortDesc {
		return SortDesc
	}
	return SortAsc
}

// Sort directions, mirroring store.SortAsc/SortDesc. The template package must
// not import the store, so the constants are restated rather than shared.
const (
	SortAsc  = "asc"
	SortDesc = "desc"
)
