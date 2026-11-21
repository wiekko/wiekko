// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package product

import (
	"strings"
	"testing"

	"github.com/a-h/templ"

	"nexmap.net/web/templates/patterns"
)

// UR-164: on list pages, only the table scrolls. Its column headers stick to
// the top of its scroll box, and the page keeps breadcrumb, header, actions
// and filters in view. Each level between the page root and the table's card
// has to take part, or the table grows with its rows and the page scrolls
// again.

func assertScrollingTable(t *testing.T, name, html string) {
	t.Helper()
	if !strings.Contains(html, `class="space-y-4 `+patterns.TableViewRegionClass+`"`) {
		t.Errorf("%s: the table's controls and card are not a shrinkable region", name)
	}
	if !strings.Contains(html, "tall:min-h-0 tall:flex-col") || !strings.Contains(html, "overflow-hidden rounded-lg") {
		t.Errorf("%s: the table is not in a TableCard", name)
	}
	if !strings.Contains(html, `class="relative min-h-0 overflow-auto"`) {
		t.Errorf("%s: the table does not scroll inside its card", name)
	}
	if !strings.Contains(html, `<thead class="sticky top-0`) {
		t.Errorf("%s: the column headers do not stick", name)
	}
}

func TestListTablesScrollUnderStickyHeaders(t *testing.T) {
	assertScrollingTable(t, "requirements", renderBacklogItems(t, backlogItemsTestProps("requirement")))
	assertScrollingTable(t, "test cases", renderTestCases(t, testCasesTestProps()))
	assertScrollingTable(t, "defects", renderDefectsTable(t, defectsTestProps()))
	// The team table too: it is the team page root, with the forms above the
	// table, that keeps scrolling as a whole.
	for _, tc := range remainingTables() {
		assertScrollingTable(t, tc.name, renderTable(t, tc.render(oneRowView(tc.name), false)))
	}
}

func TestListPagesKeepTheirHeaderInView(t *testing.T) {
	pages := map[string]templ.Component{
		"requirements": BacklogItemsPage(backlogItemsTestProps("requirement")),
		"test cases":   TestCasesPage(testCasesTestProps()),
		"defects":      DefectsPage(defectsTestProps()),
	}
	for name, page := range pages {
		html := renderTable(t, page)
		if !strings.Contains(html, `class="`+patterns.TableViewPageClass+`"`) {
			t.Errorf("%s: the page root does not fill the main area", name)
		}
		if strings.Count(html, patterns.TableViewRegionClass) < 3 {
			t.Errorf("%s: a level between page root and table card does not shrink", name)
		}
	}
}
