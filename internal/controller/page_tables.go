package controller

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
	"nexmap.net/web/templates/ui"
)

// Table views (UR-144, UR-145, UR-146): the requirements, tasks and test cases
// lists sort, search, filter and paginate on the server, with the whole state
// in the URL (UC-72 to UC-78), the way the defects list does.

// tablePageSize is the number of rows per page of a table view (UC-77).
const tablePageSize = 50

// tableSwap reports whether the request is one of a table's own controls
// swapping the table, which gets the table fragment instead of the full page.
// htmx names the swapped element in HX-Target; other htmx requests for the
// same URL — restoring a page from history, navigating into the main content —
// need the whole page.
func tableSwap(r *http.Request, tableID string) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == tableID
}

// listQuery reads the sort, search and page state every table view shares.
// Nothing here can fail: a stale bookmark or a hand-edited URL renders the
// default view, so unknown sort keys and bad pages are corrected, not
// rejected.
func listQuery(q url.Values, sortable func(key string) bool) (sortKey, dir, search string, page int) {
	sortKey = q.Get(ui.ParamSort)
	if !sortable(sortKey) {
		sortKey = ""
	}
	dir = ui.SortAsc
	if q.Get(ui.ParamDir) == ui.SortDesc {
		dir = ui.SortDesc
	}
	page, err := strconv.Atoi(q.Get(ui.ParamPage))
	if err != nil || page < 1 {
		page = 1
	}
	return sortKey, dir, strings.TrimSpace(q.Get(ui.ParamSearch)), page
}

// sortColumn reports whether a sort key is one of a store's sort columns.
func sortColumn(columns map[string]string) func(string) bool {
	return func(key string) bool {
		_, ok := columns[key]
		return ok
	}
}

// listParams is the store query for one page of a table view.
func listParams(state ui.ListState) store.ListParams {
	return store.ListParams{
		Sort:   state.Sort,
		Dir:    state.Dir,
		Search: state.Search,
		Limit:  tablePageSize,
		Offset: (state.Page - 1) * tablePageSize,
	}
}

// totalPages sizes the pagination of total rows.
func totalPages(total int) int {
	return (total + tablePageSize - 1) / tablePageSize
}

// pageBeyondEnd reports whether the requested page lies past the last one and
// moves state there. A page beyond the end renders as empty, which looks like
// "no matches" rather than "you have paged too far", so the caller re-queries.
func pageBeyondEnd(state *ui.ListState, total int) bool {
	pages := totalPages(total)
	if pages > 0 && state.Page > pages {
		state.Page = pages
		return true
	}
	return false
}

// Query parameters of the requirements and tasks list facets.
const (
	backlogFilterStatus    = "status_id"
	backlogFilterCategory  = "category_id"
	backlogFilterMilestone = "milestone_id"
	backlogFilterSprint    = "sprint_id"
	backlogFilterArchived  = "archived"
)

// parseBacklogListState reads the state of the requirements or tasks list.
// Archived items are hidden by default (FR-155).
func parseBacklogListState(r *http.Request, baseURL, kind string) (store.RequirementFilter, ui.ListState) {
	q := r.URL.Query()
	archived, _ := store.ParseArchivedFilter(q.Get(backlogFilterArchived), store.ArchivedExclude)
	filter := store.RequirementFilter{
		Kind:        kind,
		StatusID:    q.Get(backlogFilterStatus),
		CategoryID:  q.Get(backlogFilterCategory),
		MilestoneID: q.Get(backlogFilterMilestone),
		SprintID:    q.Get(backlogFilterSprint),
		Archived:    archived,
	}
	sortKey, dir, search, page := listQuery(q, sortColumn(store.RequirementSortColumns))
	state := ui.ListState{
		BaseURL: baseURL,
		Sort:    sortKey,
		Dir:     dir,
		Search:  search,
		Page:    page,
		Filters: map[string]string{
			backlogFilterStatus:    filter.StatusID,
			backlogFilterCategory:  filter.CategoryID,
			backlogFilterMilestone: filter.MilestoneID,
			backlogFilterSprint:    filter.SprintID,
			backlogFilterArchived:  archivedParam(archived, store.ArchivedExclude),
		},
	}
	return filter, state
}

// loadBacklogItemsProps assembles the requirements or tasks list (kind), for
// both the full page and the htmx fragment, so the two cannot render
// different views of the same URL.
func (c *PageProductController) loadBacklogItemsProps(r *http.Request, user *model.User, productID, kind string) (productpage.BacklogItemsProps, error) {
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.BacklogItemsProps{}, err
	}

	path := "requirements"
	title := "Requirements"
	if kind == model.RequirementKindTask {
		path, title = "tasks", "Tasks"
	}
	filter, state := parseBacklogListState(r, "/products/"+productID+"/"+path, kind)

	items, total, err := c.backlog.ListRequirementsPage(r.Context(), user, productID, filter, listParams(state))
	if err != nil {
		return productpage.BacklogItemsProps{}, err
	}
	if pageBeyondEnd(&state, total) {
		if items, _, err = c.backlog.ListRequirementsPage(r.Context(), user, productID, filter, listParams(state)); err != nil {
			return productpage.BacklogItemsProps{}, err
		}
	}

	// Hidden archived items count as existing ones (FR-155), so "all
	// archived" is not mistaken for "nothing yet".
	productTotal := total
	if state.Filtered() || filter.Archived != store.ArchivedInclude {
		productTotal, _ = c.backlog.CountRequirements(r.Context(), user, productID, kind)
	}

	// Tasks share the requirement status set; categories are per kind.
	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, model.RequirementKindRequirement)
	categories, _ := c.category.ListCategories(r.Context(), user, productID, kind)
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
	statusMap := make(map[string]*model.Status, len(statuses))
	for _, s := range statuses {
		statusMap[s.ID] = s
	}
	prefixes := make(map[string]string, len(categories))
	for _, cat := range categories {
		prefixes[cat.ID] = cat.Prefix
	}

	return productpage.BacklogItemsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - " + title,
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Kind:             kind,
		Items:            items,
		CategoryPrefixes: prefixes,
		Statuses:         statusMap,
		CanEdit:          c.canEditRequirements(user, role),
		Bulk:             bulkBar(bulkTableOf(kind), p, user, role, total),
		State:            state,
		Total:            total,
		ProductTotal:     productTotal,
		TotalPages:       totalPages(total),
		StatusOptions:    statuses,
		CategoryOptions:  categories,
		MilestoneOptions: milestones,
		SprintOptions:    sprints,
	}, nil
}

// serveBacklogItems renders the requirements or tasks list: the full page for
// a normal request, the table fragment for a swap by the table's controls.
// Both come from the one URL so the address bar always describes the view
// (DOC-1, "URL = state").
func (c *PageProductController) serveBacklogItems(w http.ResponseWriter, r *http.Request, kind string) {
	user := middleware.UserFromContext(r.Context())
	props, err := c.loadBacklogItemsProps(r, user, r.PathValue("id"), kind)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if tableSwap(r, productpage.BacklogItemsTableID(kind)) {
		props.Fragment = true
		_ = productpage.BacklogItemsTableContent(props).Render(r.Context(), w)
		return
	}
	_ = productpage.BacklogItemsPage(props).Render(r.Context(), w)
}

// Requirements renders GET /products/{id}/requirements (UR-144).
func (c *PageProductController) Requirements(w http.ResponseWriter, r *http.Request) {
	c.serveBacklogItems(w, r, model.RequirementKindRequirement)
}

// Tasks renders GET /products/{id}/tasks (UR-145).
func (c *PageProductController) Tasks(w http.ResponseWriter, r *http.Request) {
	c.serveBacklogItems(w, r, model.RequirementKindTask)
}

// Query parameters of the test case list facets.
const (
	testCaseFilterComponent    = "component_id"
	testCaseFilterCapability   = "capability_id"
	testCaseFilterPyramidLevel = "pyramid_level_id"
	testCaseFilterMode         = "mode"
)

// parseTestCaseListState reads the state of the test cases list (UR-146).
func parseTestCaseListState(r *http.Request, baseURL string) (store.TestCaseFilter, ui.ListState) {
	q := r.URL.Query()
	filter := store.TestCaseFilter{
		ComponentID:    q.Get(testCaseFilterComponent),
		CapabilityID:   q.Get(testCaseFilterCapability),
		PyramidLevelID: q.Get(testCaseFilterPyramidLevel),
		ExecutionMode:  q.Get(testCaseFilterMode),
	}
	sortKey, dir, search, page := listQuery(q, sortColumn(store.TestCaseSortColumns))
	state := ui.ListState{
		BaseURL: baseURL,
		Sort:    sortKey,
		Dir:     dir,
		Search:  search,
		Page:    page,
		Filters: map[string]string{
			testCaseFilterComponent:    filter.ComponentID,
			testCaseFilterCapability:   filter.CapabilityID,
			testCaseFilterPyramidLevel: filter.PyramidLevelID,
			testCaseFilterMode:         filter.ExecutionMode,
		},
	}
	return filter, state
}

// loadTestCasesProps assembles the test cases list for the page and the
// fragment alike.
func (c *PageProductController) loadTestCasesProps(r *http.Request, user *model.User, productID string) (productpage.TestCasesProps, error) {
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.TestCasesProps{}, err
	}

	filter, state := parseTestCaseListState(r, "/products/"+productID+"/test-cases")
	testCases, total, err := c.testing.ListTestCasesPage(r.Context(), user, productID, filter, listParams(state))
	if err != nil {
		return productpage.TestCasesProps{}, err
	}
	if pageBeyondEnd(&state, total) {
		if testCases, _, err = c.testing.ListTestCasesPage(r.Context(), user, productID, filter, listParams(state)); err != nil {
			return productpage.TestCasesProps{}, err
		}
	}
	productTotal := total
	if state.Filtered() {
		productTotal, _ = c.testing.CountTestCases(r.Context(), user, productID)
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
	levels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
	componentNames := make(map[string]string, len(components))
	for _, comp := range components {
		componentNames[comp.ID] = comp.Name
	}
	capabilityNames := make(map[string]string, len(capabilities))
	for _, capab := range capabilities {
		capabilityNames[capab.ID] = capab.Name
	}
	levelNames := make(map[string]string, len(levels))
	for _, l := range levels {
		levelNames[l.ID] = l.Name
	}

	return productpage.TestCasesProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Test Cases",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:         user.DisplayName,
		IsAdmin:             user.IsAdmin,
		Product:             p,
		TestCases:           testCases,
		Components:          componentNames,
		Capabilities:        capabilityNames,
		PyramidLevels:       levelNames,
		CanEdit:             c.canEditTestCases(user, role),
		State:               state,
		Total:               total,
		ProductTotal:        productTotal,
		TotalPages:          totalPages(total),
		ComponentOptions:    components,
		CapabilityOptions:   capabilities,
		PyramidLevelOptions: levels,
	}, nil
}

// TestCases renders GET /products/{id}/test-cases (UR-146): the full page, or
// the table fragment for a swap by the table's controls.
func (c *PageProductController) TestCases(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	props, err := c.loadTestCasesProps(r, user, r.PathValue("id"))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if tableSwap(r, "test-cases-table") {
		props.Fragment = true
		_ = productpage.TestCasesTableContent(props).Render(r.Context(), w)
		return
	}
	_ = productpage.TestCasesPage(props).Render(r.Context(), w)
}
