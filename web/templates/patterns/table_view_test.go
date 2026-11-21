// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package patterns

import (
	"context"
	"strings"
	"testing"

	"wiekko.net/web/templates"
	"wiekko.net/web/templates/ui"
)

func tableControlsProps() TableControlsProps {
	return TableControlsProps{
		Key:    "task",
		Target: "#tasks-table",
		State:  ui.ListState{BaseURL: "/products/p1/tasks", Filters: map[string]string{}},
		Noun:   "tasks",
		Facets: []TableFacet{
			{Name: "status_id", Label: "Status", Options: FacetOptions("All statuses", "", templates.SelectOption{Value: "s1", Label: "Open"})},
		},
	}
}

func renderTableControls(t *testing.T, props TableControlsProps) string {
	t.Helper()
	return renderToString(t, func(sb *strings.Builder) error {
		return TableControls(props).Render(context.Background(), sb)
	})
}

func TestTableControlsNameTheirElementsByKey(t *testing.T) {
	html := renderTableControls(t, tableControlsProps())

	// The search box keeps focus across swaps only through a stable id, and
	// every control has to include the whole control set.
	for _, want := range []string{
		`id="task-controls"`, `id="task-search"`, `id="task-filter-panel"`, `id="task-filter-status_id"`,
		`hx-include="#task-controls [name]"`, `hx-target="#tasks-table"`, `placeholder="Search tasks…"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("controls lack %s", want)
		}
	}
	if strings.Contains(html, "Clear all") {
		t.Error("the reset control rendered with nothing to reset")
	}
}

func TestTableControlsOfferOneResetForSearchAndFacets(t *testing.T) {
	props := tableControlsProps()
	props.State.Sort = "title"
	props.State.Search = "sync"
	props.State.Filters = map[string]string{"status_id": "s1"}

	html := renderTableControls(t, props)

	if strings.Count(html, "Clear all") != 1 {
		t.Error("an active search and facet need exactly one reset control (UC-75)")
	}
	if !strings.Contains(html, `href="/products/p1/tasks?dir=asc&amp;sort=title"`) {
		t.Error("the reset control does not keep the sort and drop the rest")
	}
	if !strings.Contains(html, `<details class="group/filters" open>`) {
		t.Error("an active facet does not open the drawer")
	}
}

func TestTableControlsWithoutFacetsHaveNoDrawer(t *testing.T) {
	props := tableControlsProps()
	props.Facets = nil

	if html := renderTableControls(t, props); strings.Contains(html, "<details") || strings.Contains(html, "filter-panel") {
		t.Error("a drawer rendered with no facets to put in it")
	}
}

func TestTableCountSwapsOutOfBand(t *testing.T) {
	html := renderToString(t, func(sb *strings.Builder) error {
		return TableCount("tasks-count", "3 of 9 items").Render(context.Background(), sb)
	})
	if !strings.Contains(html, `id="tasks-count"`) || !strings.Contains(html, `hx-swap-oob="true"`) || !strings.Contains(html, "3 of 9 items") {
		t.Errorf("count fragment cannot replace the header count: %s", html)
	}
}

func TestTableNoMatchesOffersTheWayBack(t *testing.T) {
	state := ui.ListState{BaseURL: "/products/p1/test-cases", Search: "x"}
	html := renderToString(t, func(sb *strings.Builder) error {
		return TableNoMatches(state, "#test-cases-table", "test cases").Render(context.Background(), sb)
	})
	for _, want := range []string{"No matching test cases", "No test case matches", "Clear search and filters", `href="/products/p1/test-cases"`} {
		if !strings.Contains(html, want) {
			t.Errorf("filtered-empty state lacks %q", want)
		}
	}
}

func TestTableCountLabel(t *testing.T) {
	state := ui.ListState{}
	if got := TableCountLabel(state, 12, 68); got != "12 items" {
		t.Errorf("unfiltered: %q", got)
	}
	state.Filters = map[string]string{"mode": "manual"}
	if got := TableCountLabel(state, 12, 68); got != "12 of 68 items" {
		t.Errorf("filtered: %q", got)
	}
}

func TestFacetOptions(t *testing.T) {
	got := FacetOptions("All components", "No component", templates.SelectOption{Value: "c1", Label: "Server"})
	want := []templates.SelectOption{
		{Value: "", Label: "All components"},
		{Value: "c1", Label: "Server"},
		{Value: FacetNone, Label: "No component"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("option %d = %v, want %v", i, got[i], want[i])
		}
	}
	if opts := FacetOptions("All modes", ""); len(opts) != 1 {
		t.Errorf("no none option was asked for, got %v", opts)
	}
}
