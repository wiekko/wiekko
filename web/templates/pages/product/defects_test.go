package product

import (
	"context"
	"strings"
	"testing"

	"nexmap.net/internal/model"
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

	if got := defectsCountLabel(props); got != "12 items" {
		t.Errorf("unfiltered label = %q, want %q", got, "12 items")
	}

	// Under a filter, showing only the matching count reads as if defects had
	// gone missing.
	props.State.Search = "gps"
	if got := defectsCountLabel(props); got != "12 of 68 items" {
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
