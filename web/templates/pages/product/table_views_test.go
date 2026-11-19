package product

import (
	"context"
	"strings"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/web/templates/ui"
)

// The requirements, tasks (UR-144, UR-145) and test cases (UR-146) tables
// follow the table conventions UC-72 to UC-78 like the defects table.

func backlogItemsTestProps(kind string) BacklogItemsProps {
	return BacklogItemsProps{
		Product: &model.Product{ID: "p1", Name: "Aviary"},
		Kind:    kind,
		Items: []*model.Requirement{
			{ID: "r1", Kind: kind, Title: "Sync sightings", CategoryID: "c1", SequenceNumber: 7, StatusID: "s1", Priority: 1, CreatedAt: "2026-01-01T00:00:00Z"},
		},
		CategoryPrefixes: map[string]string{"c1": "FR"},
		Statuses:         map[string]*model.Status{"s1": {ID: "s1", Name: "Open"}},
		Total:            1,
		ProductTotal:     1,
		TotalPages:       1,
		StatusOptions:    []*model.Status{{ID: "s1", Name: "Open"}},
		CategoryOptions:  []*model.Category{{ID: "c1", Name: "Functional", Prefix: "FR"}},
		State: ui.ListState{
			BaseURL: "/products/p1/" + backlogItemsPath(kind),
			Filters: map[string]string{},
		},
	}
}

func renderBacklogItems(t *testing.T, props BacklogItemsProps) string {
	t.Helper()
	return renderToString(t, func(sb *strings.Builder) error {
		return BacklogItemsTableContent(props).Render(context.Background(), sb)
	})
}

func TestBacklogItemsTableIsSortableByEveryColumn(t *testing.T) {
	for _, kind := range []string{model.RequirementKindRequirement, model.RequirementKindTask} {
		html := renderBacklogItems(t, backlogItemsTestProps(kind))
		for _, key := range []string{"identifier", "title", "category", "status", "priority", "created"} {
			if !strings.Contains(html, "sort="+key) {
				t.Errorf("%s: column %q is not sortable", kind, key)
			}
		}
		if !strings.Contains(html, `hx-target="#`+backlogItemsPath(kind)+`-table"`) {
			t.Errorf("%s: controls do not swap the %s table", kind, kind)
		}
	}
}

func TestBacklogItemsTableHasSearchAndFacets(t *testing.T) {
	props := backlogItemsTestProps(model.RequirementKindRequirement)
	html := renderBacklogItems(t, props)

	for _, want := range []string{`id="requirement-search"`, "All statuses", "All categories", "FR — Functional", "Hide archived"} {
		if !strings.Contains(html, want) {
			t.Errorf("requirements controls lack %q", want)
		}
	}
	// Milestone and sprint facets only show when there is something to pick.
	for _, absent := range []string{"All milestones", "All sprints"} {
		if strings.Contains(html, absent) {
			t.Errorf("empty facet %q rendered", absent)
		}
	}

	props.MilestoneOptions = []*model.Milestone{{ID: "m1", Name: "v1"}}
	props.SprintOptions = []*model.Sprint{{ID: "sp1", Name: "Sprint 1"}}
	html = renderBacklogItems(t, props)
	for _, want := range []string{"All milestones", "No milestone", "All sprints", "No sprint"} {
		if !strings.Contains(html, want) {
			t.Errorf("facet option %q missing", want)
		}
	}
}

func TestBacklogItemsEmptyStates(t *testing.T) {
	t.Run("all archived", func(t *testing.T) {
		props := backlogItemsTestProps(model.RequirementKindTask)
		props.Items, props.Total, props.ProductTotal = nil, 0, 4
		html := renderBacklogItems(t, props)
		if !strings.Contains(html, "All tasks are archived") || !strings.Contains(html, "archived=include") {
			t.Error("a list of archived items only does not offer to show them")
		}
	})
	t.Run("nothing matches", func(t *testing.T) {
		props := backlogItemsTestProps(model.RequirementKindRequirement)
		props.Items, props.Total, props.ProductTotal = nil, 0, 4
		props.State.Search = "nothing"
		html := renderBacklogItems(t, props)
		if !strings.Contains(html, "No matching requirements") || !strings.Contains(html, "Clear search and filters") {
			t.Error("a filtered-empty list does not say nothing matches")
		}
		if strings.Contains(html, "Create your first requirement") {
			t.Error("a filtered-empty list invites creating the first requirement")
		}
	})
}

func TestBacklogItemsFragmentUpdatesTheCount(t *testing.T) {
	props := backlogItemsTestProps(model.RequirementKindRequirement)
	props.ProductTotal = 9
	props.State.Filters = map[string]string{"status_id": "s1"}

	if html := renderBacklogItems(t, props); strings.Contains(html, "hx-swap-oob") {
		t.Error("the full page renders an out-of-band count")
	}
	props.Fragment = true
	html := renderBacklogItems(t, props)
	if !strings.Contains(html, `id="requirements-count" hx-swap-oob="true"`) || !strings.Contains(html, "1 of 9 items") {
		t.Error("a table swap does not update the header count")
	}
}

func TestBacklogItemsTableLinksByKind(t *testing.T) {
	html := renderBacklogItems(t, backlogItemsTestProps(model.RequirementKindTask))
	if !strings.Contains(html, `href="/products/p1/tasks/r1"`) || strings.Contains(html, "/requirements/") {
		t.Error("task rows do not link to the task pages")
	}
	if !strings.Contains(html, "FR-7") {
		t.Error("the identifier is missing")
	}
}

func testCasesTestProps() TestCasesProps {
	return TestCasesProps{
		Product: &model.Product{ID: "p1", Name: "Aviary"},
		TestCases: []*model.TestCase{
			{ID: "t1", Title: "Parses sightings", ComponentID: "c1", PyramidLevelID: "l1", ExecutionMode: model.ExecutionModeAutomatic, CreatedAt: "2026-01-01T00:00:00Z"},
		},
		Components:          map[string]string{"c1": "Server"},
		PyramidLevels:       map[string]string{"l1": "Unit"},
		Total:               1,
		ProductTotal:        1,
		TotalPages:          1,
		ComponentOptions:    []*model.Component{{ID: "c1", Name: "Server"}},
		CapabilityOptions:   []*model.Capability{{ID: "f1", Name: "Sync"}},
		PyramidLevelOptions: []*model.PyramidLevel{{ID: "l1", Name: "Unit"}},
		State:               ui.ListState{BaseURL: "/products/p1/test-cases", Filters: map[string]string{}},
	}
}

func renderTestCases(t *testing.T, props TestCasesProps) string {
	t.Helper()
	return renderToString(t, func(sb *strings.Builder) error {
		return TestCasesTableContent(props).Render(context.Background(), sb)
	})
}

func TestTestCasesTableIsSortableByEveryColumn(t *testing.T) {
	html := renderTestCases(t, testCasesTestProps())
	for _, key := range []string{"title", "component", "capability", "pyramid_level", "mode", "created"} {
		if !strings.Contains(html, "sort="+key) {
			t.Errorf("column %q is not sortable", key)
		}
	}
}

func TestTestCasesTableFacets(t *testing.T) {
	html := renderTestCases(t, testCasesTestProps())
	// UC-83: test cases filter by pyramid level; the references are optional,
	// so each reference facet can select the test cases without one.
	for _, want := range []string{
		`id="test-case-search"`,
		`id="test-case-filter-component_id"`, "No component",
		`id="test-case-filter-capability_id"`, "No capability",
		`id="test-case-filter-pyramid_level_id"`, "No level",
		`id="test-case-filter-mode"`, "Automatic", "Manual",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("test case controls lack %q", want)
		}
	}

	props := testCasesTestProps()
	props.ComponentOptions, props.CapabilityOptions, props.PyramidLevelOptions = nil, nil, nil
	html = renderTestCases(t, props)
	for _, absent := range []string{"All components", "All capabilities", "All levels"} {
		if strings.Contains(html, absent) {
			t.Errorf("empty facet %q rendered", absent)
		}
	}
	if !strings.Contains(html, "All modes") {
		t.Error("the mode facet needs no configuration and must always show")
	}
}

func TestTestCasesFilteredEmptyIsNotTheOnboardingState(t *testing.T) {
	props := testCasesTestProps()
	props.TestCases, props.Total, props.ProductTotal = nil, 0, 12
	props.State.Filters = map[string]string{"mode": "manual"}

	html := renderTestCases(t, props)
	if !strings.Contains(html, "No matching test cases") || !strings.Contains(html, "Clear search and filters") {
		t.Error("a filtered-empty list does not say nothing matches")
	}
	if strings.Contains(html, "Create your first test case") {
		t.Error("a filtered-empty list invites creating the first test case")
	}
}

func TestTestCasesPaginationCarriesTheState(t *testing.T) {
	props := testCasesTestProps()
	props.TotalPages = 3
	props.State.Page = 2
	props.State.Filters = map[string]string{"mode": "manual"}

	html := renderTestCases(t, props)
	if !strings.Contains(html, "mode=manual&amp;page=3") {
		t.Error("the next page link drops the active facet")
	}
}
