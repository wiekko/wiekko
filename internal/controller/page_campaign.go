package controller

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
)

// --- Campaign list ---

// Campaigns renders GET /products/{id}/campaigns.
func (c *PageProductController) Campaigns(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	campaigns, err := c.testing.ListCampaigns(r.Context(), user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// One grouped query for the whole product rather than one per campaign.
	counts, _ := c.testing.ListCampaignProgress(r.Context(), user, productID)

	views := make([]productpage.CampaignView, 0, len(campaigns))
	for _, campaign := range campaigns {
		views = append(views, campaignView(campaign, counts[campaign.ID]))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignsPage(productpage.CampaignsProps{
		Meta: templates.PageMeta{
			Title:     "Campaigns",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Campaigns:   views,
		CanEdit:     c.canEditCampaigns(user, role),
	}).Render(r.Context(), w)
}

// campaignView derives the progress summary shown on lists and headers.
func campaignView(campaign *model.TestingCampaign, counts map[string]int) productpage.CampaignView {
	if counts == nil {
		counts = map[string]int{}
	}

	total, settled := 0, 0
	for status, n := range counts {
		total += n
		if !model.IsIncompleteResultStatus(status) {
			settled += n
		}
	}

	percent := 0
	if total > 0 {
		percent = settled * 100 / total
	}

	return productpage.CampaignView{
		Campaign: campaign,
		Counts:   counts,
		Total:    total,
		Settled:  settled,
		Percent:  percent,
		Verdict:  model.CampaignVerdict(counts),
	}
}

// canEditCampaigns reports whether the user may schedule, edit, activate or
// close campaigns (owner or admin, per FR-320 and FR-332).
func (c *PageProductController) canEditCampaigns(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner
}

// canExecuteTests reports whether the user may record results (tester+).
func (c *PageProductController) canExecuteTests(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner ||
		role == model.RoleDeveloper || role == model.RoleTester
}

// --- Campaign create / edit ---

// CampaignCreateForm renders GET /products/{id}/campaigns/new.
func (c *PageProductController) CampaignCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canEditCampaigns(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignFormPage(productpage.CampaignFormProps{
		Meta: templates.PageMeta{
			Title:     "New Campaign",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		TestCases:   c.buildScopeOptions(r, user, productID, nil),
		Components:  c.listComponents(r, user, productID),
	}).Render(r.Context(), w)
}

// CampaignCreate handles POST /products/{id}/campaigns.
func (c *PageProductController) CampaignCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canEditCampaigns(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	selected := r.Form["test_case_ids"]
	input := service.CreateCampaignInput{
		ProductID:   productID,
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		StartsAt:    normalizeFormDate(r.FormValue("starts_at")),
		EndsAt:      normalizeFormDate(r.FormValue("ends_at")),
		TestCaseIDs: selected,
	}

	campaign, err := c.testing.CreateCampaign(r.Context(), user, input)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.CampaignFormPage(productpage.CampaignFormProps{
			Meta: templates.PageMeta{
				Title:     "New Campaign",
				CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Error:       err.Error(),
			TestCases:   c.buildScopeOptions(r, user, productID, selected),
			Components:  c.listComponents(r, user, productID),
			Name:        input.Name,
			Description: input.Description,
			StartsAt:    input.StartsAt,
			EndsAt:      input.EndsAt,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/products/%s/campaigns/%s", productID, campaign.ID), http.StatusSeeOther)
}

// CampaignEditForm renders GET /products/{id}/campaigns/{campaignId}/edit.
func (c *PageProductController) CampaignEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canEditCampaigns(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	campaign, err := c.testing.GetCampaign(r.Context(), user, productID, campaignID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignFormPage(productpage.CampaignFormProps{
		Meta: templates.PageMeta{
			Title:     "Edit Campaign",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Campaign:    campaign,
		IsEdit:      true,
		ReturnTo:    editReturnTarget(r),
		Name:        campaign.Name,
		Description: campaign.Description,
		StartsAt:    campaign.StartsAt,
		EndsAt:      campaign.EndsAt,
	}).Render(r.Context(), w)
}

// CampaignUpdate handles POST /products/{id}/campaigns/{campaignId}.
func (c *PageProductController) CampaignUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canEditCampaigns(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	startsAt := normalizeFormDate(r.FormValue("starts_at"))
	endsAt := normalizeFormDate(r.FormValue("ends_at"))

	_, err = c.testing.UpdateCampaign(r.Context(), user, productID, campaignID, service.UpdateCampaignInput{
		Name:        &name,
		Description: &description,
		StartsAt:    &startsAt,
		EndsAt:      &endsAt,
	})
	if err != nil {
		campaign, getErr := c.testing.GetCampaign(r.Context(), user, productID, campaignID)
		if getErr != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.CampaignFormPage(productpage.CampaignFormProps{
			Meta: templates.PageMeta{
				Title:     "Edit Campaign",
				CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Campaign:    campaign,
			IsEdit:      true,
			Error:       err.Error(),
			ReturnTo:    r.FormValue("return"),
			Name:        name,
			Description: description,
			StartsAt:    startsAt,
			EndsAt:      endsAt,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		fmt.Sprintf("/products/%s/campaigns/%s", productID, campaignID),
		fmt.Sprintf("/products/%s/campaigns", productID),
	), http.StatusSeeOther)
}

// CampaignDelete handles DELETE /products/{id}/campaigns/{campaignId}.
func (c *PageProductController) CampaignDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	if err := c.testing.DeleteCampaign(r.Context(), user, productID, campaignID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("HX-Redirect", fmt.Sprintf("/products/%s/campaigns", productID))
	w.WriteHeader(http.StatusOK)
}

// CampaignDeleteConfirm renders GET /products/{id}/campaigns/{campaignId}/delete/confirm.
func (c *PageProductController) CampaignDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	campaign, err := c.testing.GetCampaign(r.Context(), user, productID, campaignID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignDeleteConfirmation(campaign.Name, productID, campaignID,
		middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// --- Lifecycle ---

// CampaignActivate handles POST /products/{id}/campaigns/{campaignId}/activate.
func (c *PageProductController) CampaignActivate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	if _, err := c.testing.ActivateCampaign(r.Context(), user, productID, campaignID); err != nil {
		// The message names the blocking campaign, so it is worth surfacing.
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("HX-Redirect", fmt.Sprintf("/products/%s/campaigns/%s", productID, campaignID))
	w.WriteHeader(http.StatusOK)
}

// CampaignCloseConfirm renders GET /products/{id}/campaigns/{campaignId}/close/confirm.
// The dialog states how many results are still open (FR-333).
func (c *PageProductController) CampaignCloseConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	results, err := c.testing.ListCampaignResults(r.Context(), user, productID, campaignID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	incomplete := 0
	for _, result := range results {
		if model.IsIncompleteResultStatus(result.Status) {
			incomplete++
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignCloseConfirmation(productpage.CampaignCloseProps{
		ProductID:  productID,
		CampaignID: campaignID,
		Incomplete: incomplete,
		CSRFToken:  middleware.CSRFTokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// CampaignClose handles POST /products/{id}/campaigns/{campaignId}/close.
// The confirmation dialog has already reported any incomplete results, so this
// closes with force.
func (c *PageProductController) CampaignClose(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	if _, err := c.testing.CloseCampaign(r.Context(), user, productID, campaignID, true); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("HX-Redirect", fmt.Sprintf("/products/%s/campaigns/%s", productID, campaignID))
	w.WriteHeader(http.StatusOK)
}

// --- Campaign detail ---

// CampaignDetail renders GET /products/{id}/campaigns/{campaignId}.
func (c *PageProductController) CampaignDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	campaign, err := c.testing.GetCampaign(r.Context(), user, productID, campaignID)
	if err != nil || campaign.ProductID != productID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	matrix, results, err := c.loadMatrixProps(r, user, productID, campaign, role, matrixGrouping(r))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignDetailPage(productpage.CampaignDetailProps{
		Meta: templates.PageMeta{
			Title:     campaign.Name,
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Campaign:    campaign,
		Progress:    campaignView(campaign, countResults(results)),
		Matrix:      matrix,
		CanEdit:     c.canEditCampaigns(user, role),
	}).Render(r.Context(), w)
}

// CampaignMatrixTab renders GET /products/{id}/campaigns/{campaignId}/tabs/matrix.
func (c *PageProductController) CampaignMatrixTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	props, err := c.loadMatrixTabProps(r, user, productID, campaignID, matrixGrouping(r))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignMatrixTabContent(props).Render(r.Context(), w)
}

// CampaignProgressTab renders GET /products/{id}/campaigns/{campaignId}/tabs/progress.
func (c *PageProductController) CampaignProgressTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	campaign, err := c.testing.GetCampaign(r.Context(), user, productID, campaignID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	results, err := c.testing.ListCampaignResults(r.Context(), user, productID, campaignID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	events, _ := c.testing.ListResultEvents(r.Context(), user, productID, campaignID)

	view := campaignView(campaign, countResults(results))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignProgressTabContent(productpage.CampaignProgressProps{
		View:     view,
		Timeline: buildProgressTimeline(events, view.Total),
	}).Render(r.Context(), w)
}

// CampaignHistoryTab renders GET /products/{id}/campaigns/{campaignId}/tabs/history.
func (c *PageProductController) CampaignHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")

	entries := c.buildAuditViews(r, user, productID, "testing_campaign", campaignID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignHistoryTabContent(entries).Render(r.Context(), w)
}

// buildProgressTimeline replays the event log into a cumulative count of
// settled results, one point per recorded change (UC-207).
func buildProgressTimeline(events []*model.CampaignResultEvent, total int) []productpage.CampaignProgressPoint {
	if total == 0 || len(events) == 0 {
		return nil
	}

	// A result can flip back and forth, so track the latest status per result
	// rather than counting transitions.
	current := make(map[string]string, total)
	points := make([]productpage.CampaignProgressPoint, 0, len(events))

	for _, ev := range events {
		current[ev.ResultID] = ev.ToStatus

		settled := 0
		for _, status := range current {
			if !model.IsIncompleteResultStatus(status) {
				settled++
			}
		}
		points = append(points, productpage.CampaignProgressPoint{
			At:      ev.CreatedAt,
			Settled: settled,
			Percent: settled * 100 / total,
		})
	}
	return points
}

func countResults(results []*model.CampaignResult) map[string]int {
	counts := make(map[string]int, len(results))
	for _, result := range results {
		counts[result.Status]++
	}
	return counts
}

// --- Matrix assembly ---

func (c *PageProductController) loadMatrixTabProps(r *http.Request, user *model.User, productID, campaignID, groupBy string) (productpage.CampaignMatrixProps, error) {
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.CampaignMatrixProps{}, err
	}
	campaign, err := c.testing.GetCampaign(r.Context(), user, productID, campaignID)
	if err != nil || campaign.ProductID != productID {
		return productpage.CampaignMatrixProps{}, errors.New("campaign not found")
	}
	props, _, err := c.loadMatrixProps(r, user, productID, campaign, role, groupBy)
	return props, err
}

// loadMatrixProps assembles the execution matrix: one row per test case, one
// column per runtime environment in use, plus a trailing column for test cases
// that define no environment (UC-201).
func (c *PageProductController) loadMatrixProps(r *http.Request, user *model.User, productID string, campaign *model.TestingCampaign, role, groupBy string) (productpage.CampaignMatrixProps, []*model.CampaignResult, error) {
	results, err := c.testing.ListCampaignResults(r.Context(), user, productID, campaign.ID)
	if err != nil {
		return productpage.CampaignMatrixProps{}, nil, err
	}

	if groupBy != "pyramid" {
		groupBy = "component"
	}

	envNames := c.buildEnvironmentMap(r, user, productID)
	componentNames := c.buildComponentMap(r, user, productID)
	capabilityNames := c.buildCapabilityMap(r, user, productID)
	pyramidNames := c.buildPyramidLevelMap(r, user, productID)

	// Columns: only environments this campaign actually uses, so the table stays
	// as narrow as the data allows.
	usedEnvs := map[string]bool{}
	needsBareColumn := false
	byTestCase := map[string][]*model.CampaignResult{}
	for _, result := range results {
		if result.EnvironmentID == "" {
			needsBareColumn = true
		} else {
			usedEnvs[result.EnvironmentID] = true
		}
		byTestCase[result.TestCaseID] = append(byTestCase[result.TestCaseID], result)
	}

	envIDs := make([]string, 0, len(usedEnvs))
	for id := range usedEnvs {
		envIDs = append(envIDs, id)
	}
	sort.Slice(envIDs, func(i, j int) bool {
		return envNames[envIDs[i]] < envNames[envIDs[j]]
	})

	columns := make([]productpage.CampaignMatrixColumn, 0, len(envIDs)+1)
	for _, id := range envIDs {
		columns = append(columns, productpage.CampaignMatrixColumn{ID: id, Label: envNames[id]})
	}
	if needsBareColumn || len(columns) == 0 {
		columns = append(columns, productpage.CampaignMatrixColumn{})
	}

	// Rows, bucketed by their group label.
	rowsByGroup := map[string][]productpage.CampaignMatrixRow{}
	for testCaseID, testCaseResults := range byTestCase {
		tc, err := c.testing.GetTestCase(r.Context(), user, productID, testCaseID)
		if err != nil {
			continue
		}

		byEnv := make(map[string]*model.CampaignResult, len(testCaseResults))
		for _, result := range testCaseResults {
			byEnv[result.EnvironmentID] = result
		}
		cells := make([]*model.CampaignResult, len(columns))
		for i, col := range columns {
			cells[i] = byEnv[col.ID]
		}

		label := matrixGroupLabel(groupBy, tc, componentNames, capabilityNames, pyramidNames)
		rowsByGroup[label] = append(rowsByGroup[label], productpage.CampaignMatrixRow{
			TestCase: tc,
			Cells:    cells,
		})
	}

	labels := make([]string, 0, len(rowsByGroup))
	for label := range rowsByGroup {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	groups := make([]productpage.CampaignMatrixGroup, 0, len(labels))
	for _, label := range labels {
		rows := rowsByGroup[label]
		sort.Slice(rows, func(i, j int) bool { return rows[i].TestCase.Title < rows[j].TestCase.Title })
		groups = append(groups, productpage.CampaignMatrixGroup{Label: label, Rows: rows})
	}

	return productpage.CampaignMatrixProps{
		ProductID:  productID,
		CampaignID: campaign.ID,
		CSRFToken:  middleware.CSRFTokenFromContext(r.Context()),
		GroupBy:    groupBy,
		Columns:    columns,
		Groups:     groups,
		// Results are writable only while the campaign is active (FR-505).
		CanExecute: campaign.Status == model.CampaignStatusActive && c.canExecuteTests(user, role),
	}, results, nil
}

// matrixGroupLabel picks a row's group heading. Test cases attached to neither
// a component nor a capability sit at product level (UC-203).
func matrixGroupLabel(groupBy string, tc *model.TestCase, components, capabilities, pyramids map[string]string) string {
	if groupBy == "pyramid" {
		if name, ok := pyramids[tc.PyramidLevelID]; ok && name != "" {
			return name
		}
		return "No pyramid level"
	}
	if name, ok := components[tc.ComponentID]; ok && name != "" {
		return name
	}
	if name, ok := capabilities[tc.CapabilityID]; ok && name != "" {
		return name
	}
	return "Product level"
}

func (c *PageProductController) buildEnvironmentMap(r *http.Request, user *model.User, productID string) map[string]string {
	envs, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
	m := make(map[string]string, len(envs))
	for _, env := range envs {
		m[env.ID] = env.Name
	}
	return m
}

func (c *PageProductController) listComponents(r *http.Request, user *model.User, productID string) []*model.Component {
	components, _ := c.component.ListComponents(r.Context(), user, productID)
	return components
}

// buildScopeOptions lists every test case as a scope candidate, pre-checking
// the ones already selected so a failed submission keeps the user's picks.
func (c *PageProductController) buildScopeOptions(r *http.Request, user *model.User, productID string, selected []string) []productpage.CampaignTestCaseOption {
	testCases, err := c.testing.ListTestCases(r.Context(), user, productID)
	if err != nil {
		return nil
	}

	chosen := make(map[string]bool, len(selected))
	for _, id := range selected {
		chosen[id] = true
	}

	components := c.buildComponentMap(r, user, productID)
	capabilities := c.buildCapabilityMap(r, user, productID)
	pyramids := c.buildPyramidLevelMap(r, user, productID)

	opts := make([]productpage.CampaignTestCaseOption, 0, len(testCases))
	for _, tc := range testCases {
		opts = append(opts, productpage.CampaignTestCaseOption{
			TestCase:       tc,
			ComponentName:  components[tc.ComponentID],
			CapabilityName: capabilities[tc.CapabilityID],
			PyramidName:    pyramids[tc.PyramidLevelID],
			Selected:       chosen[tc.ID],
		})
	}
	return opts
}

// matrixGrouping reads the requested matrix grouping, which reaches the server
// as a query param on GETs and as a form value on the status POST.
func matrixGrouping(r *http.Request) string {
	if group := r.URL.Query().Get("group"); group != "" {
		return group
	}
	return r.FormValue("group")
}

// normalizeFormDate turns a date input's "YYYY-MM-DD" into the RFC 3339 stamps
// the rest of the model uses. Empty stays empty.
func normalizeFormDate(value string) string {
	if value == "" {
		return ""
	}
	if len(value) == 10 {
		return value + "T00:00:00Z"
	}
	return value
}

// --- Result status ---

// CampaignResultStatusMenu renders GET .../results/{resultId}/status-menu.
func (c *PageProductController) CampaignResultStatusMenu(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")
	resultID := r.PathValue("resultId")

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canExecuteTests(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	result, tc, err := c.loadResultWithTestCase(r, user, productID, campaignID, resultID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignResultStatusMenu(productpage.CampaignResultMenuProps{
		ProductID:   productID,
		CampaignID:  campaignID,
		Result:      result,
		TestCase:    tc,
		Environment: c.buildEnvironmentMap(r, user, productID)[result.EnvironmentID],
		GroupBy:     matrixGrouping(r),
		CSRFToken:   middleware.CSRFTokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// CampaignResultSetStatus handles POST .../results/{resultId}/status.
func (c *PageProductController) CampaignResultSetStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")
	resultID := r.PathValue("resultId")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	_, err := c.testing.SetResultStatus(r.Context(), user, productID, campaignID, resultID,
		r.FormValue("status"), r.FormValue("defect_id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	c.writeCampaignMatrixTabOOB(w, r, user, productID, campaignID)
}

// CampaignResultDefectModal renders GET .../results/{resultId}/defect-modal.
// Failing a result requires a defect, so the tester lands on the defect form
// with the campaign result carried through as the link target (FR-502).
func (c *PageProductController) CampaignResultDefectModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")
	resultID := r.PathValue("resultId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canCreateDefects(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	result, tc, err := c.loadResultWithTestCase(r, user, productID, campaignID, resultID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormModal(productpage.DefectFormProps{
		Meta: templates.PageMeta{
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Statuses:    statuses,
		Milestones:  milestones,
		Sprints:     sprints,
		LinkTo:      fmt.Sprintf("campaign-result:%s:%s:%s", campaignID, result.ID, matrixGrouping(r)),
		Title:       fmt.Sprintf("%s failed", tc.Title),
		Criticality: model.DefectCriticalityMedium,
		StatusID:    defaultStatusID(statuses),
	}).Render(r.Context(), w)
}

// CampaignResultFailWithDefect handles POST .../results/{resultId}/defect.
// It creates the defect, links it to the test case, and only then fails the
// result — so a result is never marked failed without its explanation.
func (c *PageProductController) CampaignResultFailWithDefect(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	campaignID := r.PathValue("campaignId")
	resultID := r.PathValue("resultId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canCreateDefects(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	result, _, err := c.loadResultWithTestCase(r, user, productID, campaignID, resultID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	priority, _ := strconv.Atoi(r.FormValue("priority"))
	input := service.CreateDefectInput{
		ProductID:   productID,
		Title:       r.FormValue("title"),
		Description: r.FormValue("description"),
		Criticality: r.FormValue("criticality"),
		StatusID:    r.FormValue("status_id"),
		MilestoneID: r.FormValue("milestone_id"),
		SprintID:    r.FormValue("sprint_id"),
		Estimation:  r.FormValue("estimation"),
		Priority:    priority,
	}

	rerender := func(message string) {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormModal(productpage.DefectFormProps{
			Meta: templates.PageMeta{
				CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Statuses:    statuses,
			Milestones:  milestones,
			Sprints:     sprints,
			Error:       message,
			LinkTo:      fmt.Sprintf("campaign-result:%s:%s:%s", campaignID, result.ID, matrixGrouping(r)),
			Title:       input.Title,
			Description: input.Description,
			Criticality: input.Criticality,
			StatusID:    input.StatusID,
			MilestoneID: input.MilestoneID,
			SprintID:    input.SprintID,
			Estimation:  input.Estimation,
			Priority:    r.FormValue("priority"),
		}).Render(r.Context(), w)
	}

	defect, err := c.backlog.CreateDefect(r.Context(), user, input)
	if err != nil {
		rerender(err.Error())
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defect.ID,
		model.DefectLinkTargetTestCase, result.TestCaseID); err != nil {
		rerender("created defect but failed to link it to the test case: " + err.Error())
		return
	}

	if _, err := c.testing.SetResultStatus(r.Context(), user, productID, campaignID, resultID,
		model.CampaignResultStatusFailed, defect.ID); err != nil {
		rerender("created defect but failed to record the result: " + err.Error())
		return
	}

	c.writeCampaignMatrixTabOOB(w, r, user, productID, campaignID)
}

// writeCampaignMatrixTabOOB repaints the matrix out of band, which also clears
// #modal and so dismisses whichever dialog triggered the change.
func (c *PageProductController) writeCampaignMatrixTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, campaignID string) {
	props, err := c.loadMatrixTabProps(r, user, productID, campaignID, matrixGrouping(r))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "matrix", productpage.CampaignMatrixTabContent(props))
}

func (c *PageProductController) loadResultWithTestCase(r *http.Request, user *model.User, productID, campaignID, resultID string) (*model.CampaignResult, *model.TestCase, error) {
	results, err := c.testing.ListCampaignResults(r.Context(), user, productID, campaignID)
	if err != nil {
		return nil, nil, err
	}
	for _, result := range results {
		if result.ID != resultID {
			continue
		}
		tc, err := c.testing.GetTestCase(r.Context(), user, productID, result.TestCaseID)
		if err != nil {
			return nil, nil, err
		}
		return result, tc, nil
	}
	return nil, nil, errors.New("campaign result not found")
}
