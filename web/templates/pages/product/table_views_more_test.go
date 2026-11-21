// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package product

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"wiekko.net/internal/model"
	"wiekko.net/web/templates/patterns"
	"wiekko.net/web/templates/ui"
)

// The remaining tables of UR-145 render through patterns.TableFrame: every
// column sorts, the controls are named by table, and an empty page tells
// "nothing yet" from "nothing matches".

func oneRowView(path string) patterns.TableView {
	return patterns.TableView{
		State:        ui.ListState{BaseURL: "/products/p1/" + path, Filters: map[string]string{}},
		Total:        1,
		ProductTotal: 1,
		TotalPages:   1,
	}
}

func remainingTables() []struct {
	name     string
	key      string // control id prefix
	sorts    []string
	facets   []string
	row      string // text of the one row
	render   func(v patterns.TableView, empty bool) templ.Component
	emptyMsg string
} {
	product := &model.Product{ID: "p1", Name: "Aviary"}
	return []struct {
		name     string
		key      string
		sorts    []string
		facets   []string
		row      string
		render   func(v patterns.TableView, empty bool) templ.Component
		emptyMsg string
	}{
		{
			name: "constraints", key: "constraint",
			sorts:  []string{"identifier", "title", "category", "compliance", "priority", "created"},
			facets: []string{"category_id", "compliance"},
			row:    "Runs offline", emptyMsg: "No constraints yet",
			render: func(v patterns.TableView, empty bool) templ.Component {
				props := ConstraintsProps{Product: product, TableView: v, CategoryPrefixes: map[string]string{"c1": "AC"},
					CategoryOptions: []*model.Category{{ID: "c1", Prefix: "AC", Name: "Architecture"}}}
				if !empty {
					props.Constraints = []*model.Constraint{{ID: "k1", Title: "Runs offline", CategoryID: "c1", SequenceNumber: 3}}
				}
				return ConstraintsTableContent(props)
			},
		},
		{
			name: "components", key: "component",
			sorts:  []string{"name", "milestone", "created"},
			facets: []string{"milestone_id"},
			row:    "Server", emptyMsg: "No components yet",
			render: func(v patterns.TableView, empty bool) templ.Component {
				props := ComponentsProps{Product: product, TableView: v, MilestoneOptions: []*model.Milestone{{ID: "m1", Name: "v1"}}}
				if !empty {
					props.Components = []*model.Component{{ID: "x1", Name: "Server"}}
				}
				return ComponentsTableContent(props)
			},
		},
		{
			name: "capabilities", key: "capability",
			sorts:  []string{"name", "component", "milestone", "created"},
			facets: []string{"component_id", "milestone_id"},
			row:    "Sync", emptyMsg: "No capabilities yet",
			render: func(v patterns.TableView, empty bool) templ.Component {
				props := CapabilitiesProps{Product: product, TableView: v,
					ComponentOptions: []*model.Component{{ID: "x1", Name: "Server"}},
					MilestoneOptions: []*model.Milestone{{ID: "m1", Name: "v1"}}}
				if !empty {
					props.Capabilities = []*model.Capability{{ID: "f1", Name: "Sync"}}
				}
				return CapabilitiesTableContent(props)
			},
		},
		{
			name: "milestones", key: "milestone",
			sorts:  []string{"name", "type", "due", "created"},
			facets: []string{"type"},
			row:    "v1.0", emptyMsg: "No milestones yet",
			render: func(v patterns.TableView, empty bool) templ.Component {
				props := MilestonesProps{Product: product, TableView: v}
				if !empty {
					props.Rows = []*model.Milestone{{ID: "m1", Name: "v1.0", Type: model.MilestoneTypeRelease}}
				}
				return MilestonesTableContent(props)
			},
		},
		{
			name: "sprints", key: "sprint",
			sorts:  []string{"name", "status", "dates", "milestone", "items"},
			facets: []string{"status", "milestone_id"},
			row:    "Sprint 7", emptyMsg: "No sprints yet",
			render: func(v patterns.TableView, empty bool) templ.Component {
				props := SprintsProps{Product: product, TableView: v, MilestoneOptions: []*model.Milestone{{ID: "m1", Name: "v1"}}}
				if !empty {
					props.Sprints = []SprintRow{{Sprint: &model.Sprint{ID: "s1", Name: "Sprint 7", Status: model.SprintStatusActive}}}
				}
				return SprintsTableContent(props)
			},
		},
		{
			name: "campaigns", key: "campaign",
			sorts:  []string{"name", "status", "outcome", "progress", "planned"},
			facets: []string{"status", "outcome"},
			row:    "Release 1 regression", emptyMsg: "No campaigns yet",
			render: func(v patterns.TableView, empty bool) templ.Component {
				props := CampaignsProps{Product: product, TableView: v}
				if !empty {
					props.Campaigns = []CampaignView{{Campaign: &model.TestingCampaign{ID: "c1", Name: "Release 1 regression"}}}
				}
				return CampaignsTableContent(props)
			},
		},
		{
			name: "team", key: "member",
			sorts:  []string{"user", "role"},
			facets: []string{"role"},
			row:    "Ada Lovelace", emptyMsg: "No members",
			render: func(v patterns.TableView, empty bool) templ.Component {
				props := TeamProps{Product: product, TableView: v}
				if !empty {
					props.Members = []TeamMemberView{{UserID: "u1", DisplayName: "Ada Lovelace", Role: model.RoleDeveloper}}
				}
				return TeamTableContent(props)
			},
		},
	}
}

func renderTable(t *testing.T, c templ.Component) string {
	t.Helper()
	return renderToString(t, func(sb *strings.Builder) error { return c.Render(context.Background(), sb) })
}

func TestRemainingTablesFollowTheTableConventions(t *testing.T) {
	for _, tt := range remainingTables() {
		t.Run(tt.name, func(t *testing.T) {
			html := renderTable(t, tt.render(oneRowView(tt.name), false))
			if !strings.Contains(html, tt.row) {
				t.Fatal("the row is missing")
			}
			for _, key := range tt.sorts {
				if !strings.Contains(html, "sort="+key) {
					t.Errorf("column %q is not sortable", key)
				}
			}
			for _, facet := range tt.facets {
				if !strings.Contains(html, `id="`+tt.key+`-filter-`+facet+`"`) {
					t.Errorf("facet %q is missing", facet)
				}
			}
			if !strings.Contains(html, `id="`+tt.key+`-search"`) {
				t.Error("the search box is missing")
			}
		})
	}
}

func TestRemainingTablesEmptyStates(t *testing.T) {
	for _, tt := range remainingTables() {
		t.Run(tt.name, func(t *testing.T) {
			nothingYet := oneRowView(tt.name)
			nothingYet.Total, nothingYet.ProductTotal = 0, 0
			if html := renderTable(t, tt.render(nothingYet, true)); !strings.Contains(html, tt.emptyMsg) {
				t.Errorf("an empty table does not say %q", tt.emptyMsg)
			}

			nothingMatches := oneRowView(tt.name)
			nothingMatches.Total, nothingMatches.ProductTotal = 0, 5
			nothingMatches.State.Search = "zzz"
			html := renderTable(t, tt.render(nothingMatches, true))
			if !strings.Contains(html, "No matching") || !strings.Contains(html, "Clear search and filters") {
				t.Error("a filtered-empty table does not say nothing matches")
			}
			if strings.Contains(html, tt.emptyMsg) {
				t.Error("a filtered-empty table shows the onboarding state")
			}
		})
	}
}

func TestRemainingTablesSwapUpdatesTheCount(t *testing.T) {
	for _, tt := range remainingTables() {
		t.Run(tt.name, func(t *testing.T) {
			v := oneRowView(tt.name)
			v.ProductTotal = 4
			v.State.Filters = map[string]string{tt.facets[0]: "x"}
			v.Fragment = true
			if html := renderTable(t, tt.render(v, false)); !strings.Contains(html, `hx-swap-oob="true"`) || !strings.Contains(html, "1 of 4 items") {
				t.Error("a swap does not update the header count")
			}
		})
	}
}
