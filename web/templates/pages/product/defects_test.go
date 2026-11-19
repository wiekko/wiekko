package product

import (
	"context"
	html2 "html"
	"strings"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/web/templates/patterns"
	"nexmap.net/web/templates/ui"
)

func defectsTestProps() DefectsProps {
	return DefectsProps{
		Product: &model.Product{ID: "p1", Name: "Aviary"},
		Defects: []*model.Defect{
			{ID: "d1", Title: "GPS drift", Criticality: "high", StatusID: "s1", Priority: 1, CreatedAt: "2026-01-01T00:00:00Z"},
		},
		Statuses:     map[string]*model.Status{"s1": {ID: "s1", Name: "In Progress"}},
		Environments: map[string]string{"e1": "Android 14"},
		Total:        1,
		ProductTotal: 1,
		TotalPages:   1,
		StatusOptions: []*model.Status{
			{ID: "s1", Name: "In Progress"},
			{ID: "s2", Name: "Done"},
		},
		EnvOptions:    []*model.RuntimeEnvironment{{ID: "e1", Name: "Android 14"}},
		SprintOptions: []*model.Sprint{{ID: "sp1", Name: "Sprint 13"}},
		State: ui.ListState{
			BaseURL: "/products/p1/defects",
			Filters: map[string]string{},
		},
	}
}

func renderDefectsTable(t *testing.T, props DefectsProps) string {
	t.Helper()
	return renderToString(t, func(sb *strings.Builder) error {
		return DefectsTableContent(props).Render(context.Background(), sb)
	})
}

func TestDefectsTableRendersSortableHeaders(t *testing.T) {
	html := renderDefectsTable(t, defectsTestProps())

	for _, key := range []string{"title", "criticality", "status", "environment", "priority", "created"} {
		if !strings.Contains(html, "sort="+key) {
			t.Errorf("column %q is not sortable — no sort=%s link rendered", key, key)
		}
	}
}

func TestDefectsTableSortLinksCarryTheActiveSearchAndFilters(t *testing.T) {
	props := defectsTestProps()
	props.State.Search = "gps"
	props.State.Filters = map[string]string{"criticality": "high"}

	html := renderDefectsTable(t, props)

	// A sort click that dropped the current search or facet would silently
	// widen the result set the user is looking at.
	if !strings.Contains(html, "q=gps") {
		t.Error("sort links do not carry the active search")
	}
	if !strings.Contains(html, "criticality=high") {
		t.Error("sort links do not carry the active facet")
	}
}

func TestDefectsTableSearchBoxKeepsItsValue(t *testing.T) {
	props := defectsTestProps()
	props.State.Search = "gps drift"

	html := renderDefectsTable(t, props)

	if !strings.Contains(html, `value="gps drift"`) {
		t.Error("the search box lost its text on re-render")
	}
}

func TestDefectsSearchInputHasAStableID(t *testing.T) {
	html := renderDefectsTable(t, defectsTestProps())

	// Each keystroke swaps this input away and renders a new one. htmx only
	// restores focus and the caret position if the replaced element can be
	// found again by id, so without one the box loses focus mid-word.
	if !strings.Contains(html, `id="defect-search"`) {
		t.Error("the search box has no id, so htmx cannot restore focus across swaps")
	}
}

func TestDefectsFacetPanelIsASiblingOfTheControlRow(t *testing.T) {
	props := defectsTestProps()
	props.State.Filters = map[string]string{"status_id": "s1"}

	html := renderDefectsTable(t, props)

	// Inside <details> the panel inherited that element's box: in flow it grew
	// the flex item and shifted the search box; positioned out of flow it
	// covered the table header. It has to be a full-width sibling, revealed by
	// the group-has variant, so that opening it pushes the table down.
	panel := strings.Index(html, `id="defect-filter-panel"`)
	if panel < 0 {
		t.Fatal("no facet panel rendered")
	}
	if !strings.Contains(html, "group-has-[details[open]]:grid") {
		t.Error("the panel is not tied to the drawer's open state")
	}
	if strings.Contains(html, "absolute inset-x-0 top-full") {
		t.Error("the panel is positioned out of flow; opening it will cover the table")
	}

	// A panel nested in the toggle would appear before </details>.
	if closing := strings.Index(html, "</details>"); closing > panel {
		t.Error("the panel is still inside <details>, so it will carry that element's box")
	}
}

func TestDefectsClearButtonAppearsOnlyWhenFiltered(t *testing.T) {
	props := defectsTestProps()
	if html := renderDefectsTable(t, props); strings.Contains(html, "Clear all") {
		t.Error("the clear control rendered with nothing to clear")
	}

	t.Run("a search alone is enough to show it", func(t *testing.T) {
		props := defectsTestProps()
		props.State.Search = "gps"
		if !strings.Contains(renderDefectsTable(t, props), "Clear all") {
			t.Error("no clear control offered for an active search")
		}
	})

	t.Run("a facet alone is enough to show it", func(t *testing.T) {
		props := defectsTestProps()
		props.State.Filters = map[string]string{"status_id": "s1"}
		if !strings.Contains(renderDefectsTable(t, props), "Clear all") {
			t.Error("no clear control offered for an active facet")
		}
	})
}

func TestDefectsClearButtonResetsEverythingButTheSort(t *testing.T) {
	props := defectsTestProps()
	props.State.Sort = "criticality"
	props.State.Dir = ui.SortDesc
	props.State.Search = "gps"
	props.State.Page = 3
	props.State.Filters = map[string]string{"status_id": "s1", "criticality": "high"}

	html := renderDefectsTable(t, props)

	target := props.State.ClearURL()
	// templ escapes & as &amp; in the rendered href, so the link is matched in
	// its escaped form while the query itself is asserted below.
	if !strings.Contains(html, html2.EscapeString(target)) {
		t.Fatalf("clear control does not link to %q", target)
	}
	// Re-sorting is not something the user asked to undo, so the sort survives
	// while the search, the facets and the page do not.
	if !strings.Contains(target, "sort=criticality") || !strings.Contains(target, "dir=desc") {
		t.Errorf("clearing dropped the sort order: %q", target)
	}
	for _, param := range []string{"q=", "status_id=", "criticality=high", "page="} {
		if strings.Contains(target, param) {
			t.Errorf("clearing left %q behind in %q", param, target)
		}
	}
}

func TestDefectsTableCarriesSortAsHiddenInputs(t *testing.T) {
	props := defectsTestProps()
	props.State.Sort = "criticality"
	props.State.Dir = ui.SortDesc

	html := renderDefectsTable(t, props)

	// htmx sends only the inputs it includes, so without these a filter change
	// would silently reset the table to its default order.
	if !strings.Contains(html, `name="sort"`) || !strings.Contains(html, `value="criticality"`) {
		t.Error("the active sort key does not round-trip as a hidden input")
	}
	if !strings.Contains(html, `name="dir"`) || !strings.Contains(html, `value="desc"`) {
		t.Error("the active sort direction does not round-trip as a hidden input")
	}
	if strings.Contains(html, `name="page"`) {
		t.Error("page must not round-trip as a hidden input, or filtering would not reset it")
	}
}

func TestDefectsTableFilteredEmptyIsNotTheOnboardingState(t *testing.T) {
	props := defectsTestProps()
	props.Defects = nil
	props.Total = 0
	props.ProductTotal = 68
	props.State.Search = "nonexistent"

	html := renderDefectsTable(t, props)

	if !strings.Contains(html, "No matching defects") {
		t.Error("a filtered-empty list should say nothing matches")
	}
	// The onboarding CTA would be wrong here: the product has 68 defects, they
	// are simply filtered out.
	if strings.Contains(html, "Report your first defect") {
		t.Error("a filtered-empty list rendered the onboarding call to action")
	}
	if !strings.Contains(html, "Clear search and filters") {
		t.Error("a filtered-empty list offers no way back")
	}
}

func TestDefectsTableFacetDrawerOpensWhenFiltered(t *testing.T) {
	const (
		collapsed = `<details class="group/filters">`
		expanded  = `<details class="group/filters" open>`
	)

	props := defectsTestProps()
	html := renderDefectsTable(t, props)
	if !strings.Contains(html, collapsed) {
		t.Error("the filter drawer should start collapsed when nothing is filtered")
	}

	// A filter the user cannot see is a filter they cannot undo, so an active
	// facet has to force the drawer open.
	props.State.Filters = map[string]string{"status_id": "s1"}
	html = renderDefectsTable(t, props)
	if !strings.Contains(html, expanded) {
		t.Error("the filter drawer should be open when a facet is active")
	}

	// A search is not a facet. Every keystroke re-renders these controls, so
	// opening the drawer for a non-empty search made it spring open by itself
	// as soon as the user started typing.
	searching := defectsTestProps()
	searching.State.Search = "gps"
	if html := renderDefectsTable(t, searching); !strings.Contains(html, collapsed) {
		t.Error("typing in the search box should not open the filter drawer")
	}
	// A filter hidden inside a collapsed drawer would be invisible, so the
	// count is badged on the summary.
	if !strings.Contains(html, "Filters") {
		t.Error("no filter drawer rendered")
	}
}

func TestDefectsTableFacetsOfferAnAllOption(t *testing.T) {
	html := renderDefectsTable(t, defectsTestProps())

	for _, label := range []string{"All statuses", "All criticalities", "All environments", "All sprints"} {
		if !strings.Contains(html, label) {
			t.Errorf("facet is missing its clear option %q", label)
		}
	}
}

func TestDefectsTableHidesEmptyFacets(t *testing.T) {
	props := defectsTestProps()
	props.EnvOptions = nil
	props.SprintOptions = nil

	html := renderDefectsTable(t, props)

	// Offering a facet with nothing but "All" to choose from is noise.
	if strings.Contains(html, "All environments") {
		t.Error("the environment facet rendered for a product with no environments")
	}
	if strings.Contains(html, "All sprints") {
		t.Error("the sprint facet rendered for a product with no sprints")
	}
}

func TestDefectsTableOmitsEnvironmentColumnWhenUnused(t *testing.T) {
	props := defectsTestProps()
	props.Environments = nil

	html := renderDefectsTable(t, props)

	if strings.Contains(html, "sort=environment") {
		t.Error("the environment column rendered for a product that defines none")
	}
}

func TestDefectsTablePaginationOnlyWhenItPages(t *testing.T) {
	props := defectsTestProps()
	if html := renderDefectsTable(t, props); strings.Contains(html, "Pagination") {
		t.Error("a single-page list rendered pagination")
	}

	props.TotalPages = 3
	props.State.Page = 2
	html := renderDefectsTable(t, props)
	if !strings.Contains(html, "page=3") {
		t.Error("pagination does not link to the next page")
	}
	if !strings.Contains(html, `aria-label="Pagination"`) {
		t.Error("no pagination rendered for a multi-page list")
	}
}

func TestDefectsCountLabelNamesBothNumbersWhenFiltered(t *testing.T) {
	props := defectsTestProps()
	props.Total = 12
	props.ProductTotal = 68

	if got := patterns.TableCountLabel(props.State, props.Total, props.ProductTotal); got != "12 items" {
		t.Errorf("unfiltered label = %q, want %q", got, "12 items")
	}

	// Under a filter, showing only the matching count reads as if defects had
	// gone missing.
	props.State.Search = "gps"
	if got := patterns.TableCountLabel(props.State, props.Total, props.ProductTotal); got != "12 of 68 items" {
		t.Errorf("filtered label = %q, want %q", got, "12 of 68 items")
	}
}

func TestDefectsTableEscapesUserText(t *testing.T) {
	props := defectsTestProps()
	props.Defects[0].Title = `<script>alert(1)</script>`
	props.State.Search = `"><script>`

	html := renderDefectsTable(t, props)

	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("a defect title was rendered unescaped")
	}
	if strings.Contains(html, `value=""><script>`) {
		t.Error("the search value was rendered unescaped")
	}
}
