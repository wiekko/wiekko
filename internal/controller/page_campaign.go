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
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	campaigns, err := c.testing.ListCampaigns(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
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
// close campaigns (owner or admin, per FR-93 and FR-106).
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
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !c.canEditCampaigns(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/campaigns", noticeNeedsOwner)
		return
	}

	scope := c.buildScopeData(r, user, productID, nil)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignFormPage(productpage.CampaignFormProps{
		Meta: templates.PageMeta{
			Title:     "New Campaign",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		TestCases:     scope.TestCases,
		Components:    scope.Components,
		Capabilities:  scope.Capabilities,
		PyramidLevels: scope.PyramidLevels,
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
		scope := c.buildScopeData(r, user, productID, selected)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.CampaignFormPage(productpage.CampaignFormProps{
			Meta: templates.PageMeta{
				Title:     "New Campaign",
				CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
			},
			DisplayName:   user.DisplayName,
			IsAdmin:       user.IsAdmin,
			Product:       p,
			Error:         errorText(err),
			TestCases:     scope.TestCases,
			Components:    scope.Components,
			Capabilities:  scope.Capabilities,
			PyramidLevels: scope.PyramidLevels,
			Name:          input.Name,
			Description:   input.Description,
			StartsAt:      input.StartsAt,
			EndsAt:        input.EndsAt,
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
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !c.canEditCampaigns(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/campaigns/"+campaignID, noticeNeedsOwner)
		return
	}

	campaign, err := c.testing.GetCampaign(r.Context(), user, productID, campaignID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/campaigns", http.StatusSeeOther)
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
		IfUpdatedAt: r.FormValue("updated_at"),
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
			Error:       errorText(err),
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
		http.Error(w, errorText(err), http.StatusConflict)
		return
	}

	w.Header().Set("HX-Redirect", fmt.Sprintf("/products/%s/campaigns/%s", productID, campaignID))
	w.WriteHeader(http.StatusOK)
}

// CampaignCloseConfirm renders GET /products/{id}/campaigns/{campaignId}/close/confirm.
// The dialog states how many results are still open (FR-107).
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
		http.Error(w, errorText(err), http.StatusConflict)
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
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	campaign, err := c.testing.GetCampaign(r.Context(), user, productID, campaignID)
	if err != nil || campaign.ProductID != productID {
		http.Redirect(w, r, "/products/"+productID+"/campaigns", http.StatusSeeOther)
		return
	}

	matrix, results, err := c.loadMatrixProps(r, user, productID, campaign, role, matrixGrouping(r))
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/campaigns", http.StatusSeeOther)
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

	filter := campaignHistoryFilter(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignHistoryTabContent(productpage.CampaignHistoryProps{
		ProductID:  productID,
		CampaignID: campaignID,
		Filter:     filter,
		Entries:    c.buildCampaignHistory(r, user, productID, campaignID, filter),
	}).Render(r.Context(), w)
}

// campaignHistoryFilter reads the feed filter, defaulting to the full history.
func campaignHistoryFilter(r *http.Request) string {
	switch f := r.URL.Query().Get("filter"); f {
	case productpage.CampaignHistoryFilterOverrides,
		productpage.CampaignHistoryFilterRetests,
		productpage.CampaignHistoryFilterFailures:
		return f
	default:
		return productpage.CampaignHistoryFilterAll
	}
}

// buildCampaignHistory merges the campaign's lifecycle audit trail with its
// result change log into one feed, newest first. Both answer the same question,
// so they belong in one place rather than in two tabs.
func (c *PageProductController) buildCampaignHistory(r *http.Request, user *model.User, productID, campaignID, filter string) []productpage.CampaignHistoryEntry {
	audits := c.buildAuditViews(r, user, productID, "testing_campaign", campaignID)
	events, _ := c.testing.ListResultEvents(r.Context(), user, productID, campaignID)

	entries := make([]productpage.CampaignHistoryEntry, 0, len(audits)+len(events))
	for i := range audits {
		entry := audits[i]
		entries = append(entries, productpage.CampaignHistoryEntry{
			Kind:     "audit",
			At:       formatTimestamp(entry.CreatedAt),
			RawAt:    entry.CreatedAt,
			UserName: entry.UserName,
			Audit:    &entry,
		})
	}
	entries = append(entries, c.buildResultHistory(r, user, productID, campaignID, events)...)

	// Newest first. Ties keep the store's order, which is already deterministic.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].RawAt > entries[j].RawAt })

	if filter == productpage.CampaignHistoryFilterAll {
		return entries
	}
	kept := make([]productpage.CampaignHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if matchesHistoryFilter(entry, filter) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// buildResultHistory turns result events into feed rows. Overrides are detected
// by walking each result's own events in order, so a batch re-test — which
// stamps many results at one instant — never reads as one tester overriding
// another.
func (c *PageProductController) buildResultHistory(r *http.Request, user *model.User, productID, campaignID string, events []*model.CampaignResultEvent) []productpage.CampaignHistoryEntry {
	if len(events) == 0 {
		return nil
	}

	names := c.displayNames(r, user)
	defects := c.defectTitleMap(r, user, productID, events)
	envs := c.buildEnvironmentMap(r, user, productID)

	results, _ := c.testing.ListCampaignResults(r.Context(), user, productID, campaignID)
	byResult := make(map[string]*model.CampaignResult, len(results))
	for _, result := range results {
		byResult[result.ID] = result
	}
	previousUser := make(map[string]string, len(byResult))
	entries := make([]productpage.CampaignHistoryEntry, 0, len(events))
	for _, ev := range events {
		entry := productpage.CampaignHistoryEntry{
			Kind:        "result",
			At:          formatTimestamp(ev.CreatedAt),
			RawAt:       ev.CreatedAt,
			UserName:    eventActorName(names, ev.UserID),
			FromStatus:  ev.FromStatus,
			ToStatus:    ev.ToStatus,
			DefectID:    ev.DefectID,
			DefectTitle: defects[ev.DefectID],
			IsOverride:  isOverride(previousUser[ev.ResultID], ev.UserID),
			IsRetest: ev.ToStatus == model.CampaignResultStatusRetest ||
				ev.FromStatus == model.CampaignResultStatusRetest,
		}
		if result, ok := byResult[ev.ResultID]; ok {
			entry.TestCase = result.TestTitle
			if result.TestCaseID == "" {
				entry.TestCase += " (deleted)"
			}
			entry.Environment = resultEnvironment(result, envs)
		}
		entries = append(entries, entry)
		previousUser[ev.ResultID] = ev.UserID
	}
	return entries
}

func matchesHistoryFilter(entry productpage.CampaignHistoryEntry, filter string) bool {
	if entry.Kind != "result" {
		return false
	}
	switch filter {
	case productpage.CampaignHistoryFilterOverrides:
		return entry.IsOverride
	case productpage.CampaignHistoryFilterRetests:
		return entry.IsRetest
	case productpage.CampaignHistoryFilterFailures:
		return entry.ToStatus == model.CampaignResultStatusFailed
	default:
		return true
	}
}

// buildProgressTimeline replays the event log into a cumulative count of
// settled results (UC-85).
//
// One point per event turns a real campaign into hundreds of near-identical
// bars, so the series is bucketed to the last state of each day. The individual
// changes are not lost — they are the campaign's History tab.
func buildProgressTimeline(events []*model.CampaignResultEvent, total int) []productpage.CampaignProgressPoint {
	if total == 0 || len(events) == 0 {
		return nil
	}

	// A result can flip back and forth, so track the latest status per result
	// rather than counting transitions.
	current := make(map[string]string, total)
	points := make([]productpage.CampaignProgressPoint, 0, len(events))

	for i, ev := range events {
		current[ev.ResultID] = ev.ToStatus

		// Keep the day's final state, and always the most recent event so the
		// series ends where the campaign actually stands.
		if i < len(events)-1 && eventDay(events[i+1].CreatedAt) == eventDay(ev.CreatedAt) {
			continue
		}

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

// eventDay is the date part of an RFC 3339 timestamp.
func eventDay(ts string) string {
	if len(ts) < 10 {
		return ts
	}
	return ts[:10]
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
// that define no environment (UC-79).
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
	// as narrow as the data allows. A deleted environment keeps a column of its
	// own under the name the results recorded.
	usedEnvs := map[string]string{}
	needsBareColumn := false
	byTest := map[string][]*model.CampaignResult{}
	var testOrder []string
	for _, result := range results {
		if key := matrixColumnKey(result); key == "" {
			needsBareColumn = true
		} else if result.EnvironmentID != "" {
			usedEnvs[key] = envNames[result.EnvironmentID]
		} else {
			usedEnvs[key] = result.EnvironmentName + " (deleted)"
		}
		if _, seen := byTest[result.CampaignTestID]; !seen {
			testOrder = append(testOrder, result.CampaignTestID)
		}
		byTest[result.CampaignTestID] = append(byTest[result.CampaignTestID], result)
	}

	envKeys := make([]string, 0, len(usedEnvs))
	for key := range usedEnvs {
		envKeys = append(envKeys, key)
	}
	sort.Slice(envKeys, func(i, j int) bool {
		return usedEnvs[envKeys[i]] < usedEnvs[envKeys[j]]
	})

	columns := make([]productpage.CampaignMatrixColumn, 0, len(envKeys)+1)
	for _, key := range envKeys {
		columns = append(columns, productpage.CampaignMatrixColumn{ID: key, Label: usedEnvs[key]})
	}
	if needsBareColumn || len(columns) == 0 {
		columns = append(columns, productpage.CampaignMatrixColumn{})
	}

	// Rows, bucketed by their group label. The row shows the campaign's copy
	// of the test case; a deleted test case gets a group of its own.
	rowsByGroup := map[string][]productpage.CampaignMatrixRow{}
	for _, testID := range testOrder {
		testResults := byTest[testID]
		tc := c.campaignTestCase(r, user, productID, testResults[0])

		byEnv := make(map[string]*model.CampaignResult, len(testResults))
		for _, result := range testResults {
			byEnv[matrixColumnKey(result)] = result
		}
		cells := make([]*model.CampaignResult, len(columns))
		for i, col := range columns {
			cells[i] = byEnv[col.ID]
		}

		label := "Deleted test cases"
		if tc.ID != "" {
			label = matrixGroupLabel(groupBy, tc, componentNames, capabilityNames, pyramidNames)
		}
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

	// A missing churn map only costs the cells their dot, so a failure here must
	// not take the whole matrix down with it.
	churn, _ := c.testing.ResultChurn(r.Context(), user, productID, campaign.ID)

	return productpage.CampaignMatrixProps{
		ProductID:  productID,
		CampaignID: campaign.ID,
		CSRFToken:  middleware.CSRFTokenFromContext(r.Context()),
		GroupBy:    groupBy,
		Columns:    columns,
		Groups:     groups,
		// Results are writable only while the campaign is active (FR-124).
		CanExecute: campaign.Status == model.CampaignStatusActive && c.canExecuteTests(user, role),
		Churn:      churn,
	}, results, nil
}

// matrixColumnKey names a result's matrix column: its environment, the
// recorded name of a deleted one, or "" for a test without an environment
// matrix.
func matrixColumnKey(result *model.CampaignResult) string {
	switch {
	case result.EnvironmentID != "":
		return result.EnvironmentID
	case result.EnvironmentName != "":
		return "deleted:" + result.EnvironmentName
	}
	return ""
}

// resultEnvironment labels a result's environment, marking a deleted one.
func resultEnvironment(result *model.CampaignResult, envNames map[string]string) string {
	if result.EnvironmentID != "" {
		if name, ok := envNames[result.EnvironmentID]; ok {
			return name
		}
	}
	if result.EnvironmentName != "" {
		return result.EnvironmentName + " (deleted)"
	}
	return ""
}

// campaignTestCase is the test case a campaign result belongs to, as the
// campaign knows it: the live test case under the title the campaign copied,
// or, once the test case is deleted, a stand-in without an ID carrying just
// that title.
func (c *PageProductController) campaignTestCase(r *http.Request, user *model.User, productID string, result *model.CampaignResult) *model.TestCase {
	if result.TestCaseID != "" {
		if tc, err := c.testing.GetTestCase(r.Context(), user, productID, result.TestCaseID); err == nil {
			copied := *tc
			copied.Title = result.TestTitle
			return &copied
		}
	}
	return &model.TestCase{ProductID: productID, Title: result.TestTitle + " (deleted)"}
}

// matrixGroupLabel picks a row's group heading. Test cases attached to neither
// a component nor a capability sit at product level (UC-81).
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

// campaignScopeData is everything the scope picker needs: the selectable rows
// plus the lists its filter dropdowns are built from.
type campaignScopeData struct {
	TestCases     []productpage.CampaignTestCaseOption
	Components    []*model.Component
	Capabilities  []*model.Capability
	PyramidLevels []*model.PyramidLevel
}

// buildScopeData lists every test case as a scope candidate, pre-checking the
// ones already selected so a failed submission keeps the user's picks. Each
// lookup list is fetched once and serves double duty: resolving ID → name for
// the row subtitles, and populating the matching filter dropdown.
func (c *PageProductController) buildScopeData(r *http.Request, user *model.User, productID string, selected []string) campaignScopeData {
	components, _ := c.component.ListComponents(r.Context(), user, productID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
	levels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)

	data := campaignScopeData{
		Components:    components,
		Capabilities:  capabilities,
		PyramidLevels: levels,
	}

	testCases, err := c.testing.ListTestCases(r.Context(), user, productID)
	if err != nil {
		return data
	}

	chosen := make(map[string]bool, len(selected))
	for _, id := range selected {
		chosen[id] = true
	}

	componentNames := make(map[string]string, len(components))
	for _, x := range components {
		componentNames[x.ID] = x.Name
	}
	capabilityNames := make(map[string]string, len(capabilities))
	for _, x := range capabilities {
		capabilityNames[x.ID] = x.Name
	}
	levelNames := make(map[string]string, len(levels))
	for _, x := range levels {
		levelNames[x.ID] = x.Name
	}

	data.TestCases = make([]productpage.CampaignTestCaseOption, 0, len(testCases))
	for _, tc := range testCases {
		data.TestCases = append(data.TestCases, productpage.CampaignTestCaseOption{
			TestCase:       tc,
			ComponentName:  componentNames[tc.ComponentID],
			CapabilityName: capabilityNames[tc.CapabilityID],
			PyramidName:    levelNames[tc.PyramidLevelID],
			Selected:       chosen[tc.ID],
		})
	}
	return data
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

	campaign, err := c.testing.GetCampaign(r.Context(), user, productID, campaignID)
	if err != nil || campaign.ProductID != productID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	result, tc, err := c.loadResultWithTestCase(r, user, productID, campaignID, resultID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// The panel is readable at viewer level so a closed campaign can still be
	// traced back; only the status picker is gated (FR-124). The write itself is
	// authorized again in SetResultStatus.
	canExecute := campaign.Status == model.CampaignStatusActive && c.canExecuteTests(user, role)

	events, _ := c.testing.ListEventsForResult(r.Context(), user, productID, campaignID, resultID)

	// The description the campaign copied: what this campaign tested, even
	// if the test case has been edited or deleted since.
	description := ""
	if tests, err := c.testing.ListCampaignTests(r.Context(), user, productID, campaignID); err == nil {
		for _, t := range tests {
			if t.ID == result.CampaignTestID {
				description = t.Description
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CampaignResultStatusMenu(productpage.CampaignResultMenuProps{
		Description: description,
		ProductID:   productID,
		CampaignID:  campaignID,
		Result:      result,
		TestCase:    tc,
		Environment: resultEnvironment(result, c.buildEnvironmentMap(r, user, productID)),
		GroupBy:     matrixGrouping(r),
		CSRFToken:   middleware.CSRFTokenFromContext(r.Context()),
		CanExecute:  canExecute,
		Events:      c.buildResultEventViews(r, user, productID, events),
	}).Render(r.Context(), w)
}

// buildResultEventViews turns the raw change log into display rows, resolving
// actor names and defect titles. Events arrive oldest first and stay that way:
// a single result's history reads as a story.
func (c *PageProductController) buildResultEventViews(r *http.Request, user *model.User, productID string, events []*model.CampaignResultEvent) []productpage.ResultEventView {
	if len(events) == 0 {
		return nil
	}

	names := c.displayNames(r, user)
	defects := c.defectTitleMap(r, user, productID, events)

	views := make([]productpage.ResultEventView, 0, len(events))
	previousUser := ""
	for _, ev := range events {
		views = append(views, productpage.ResultEventView{
			UserName:   eventActorName(names, ev.UserID),
			FromStatus: ev.FromStatus,
			ToStatus:   ev.ToStatus,
			DefectID:   ev.DefectID,
			// A defect that has since been deleted leaves no title, and the row
			// simply drops the link rather than showing a bare id.
			DefectTitle: defects[ev.DefectID],
			At:          formatTimestamp(ev.CreatedAt),
			IsOverride:  isOverride(previousUser, ev.UserID),
		})
		previousUser = ev.UserID
	}
	return views
}

// isOverride reports whether an event revised someone else's verdict. The very
// first change on a result overrides nothing, and neither does a person
// re-testing their own result.
func isOverride(previousUser, currentUser string) bool {
	return previousUser != "" && currentUser != "" && previousUser != currentUser
}

// eventActorName labels the actor behind an event. An empty user id means the
// change came from the system — a defect closing reopens results for re-test
// without anyone touching them — or that the account has since been deleted.
func eventActorName(names map[string]string, userID string) string {
	if userID == "" {
		return "System"
	}
	if name, ok := names[userID]; ok && name != "" {
		return name
	}
	return "Unknown user"
}

// defectLabel names a defect by identifier and title, e.g. "DF-17 Login fails".
func defectLabel(d *model.Defect) string {
	return d.Identifier + " " + d.Title
}

// defectTitleMap resolves only the defects the given events actually reference.
func (c *PageProductController) defectTitleMap(r *http.Request, user *model.User, productID string, events []*model.CampaignResultEvent) map[string]string {
	titles := make(map[string]string)
	for _, ev := range events {
		if ev.DefectID == "" || titles[ev.DefectID] != "" {
			continue
		}
		if d, err := c.backlog.GetDefect(r.Context(), user, productID, ev.DefectID); err == nil {
			titles[ev.DefectID] = defectLabel(d)
		}
	}
	return titles
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
		http.Error(w, errorText(err), http.StatusConflict)
		return
	}

	c.writeCampaignMatrixTabOOB(w, r, user, productID, campaignID)
}

// CampaignResultDefectModal renders GET .../results/{resultId}/defect-modal.
// Failing a result requires a defect, so the tester lands on the defect form
// with the campaign result carried through as the link target (FR-121).
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
	environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormModal(productpage.DefectFormProps{
		Meta: templates.PageMeta{
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:  user.DisplayName,
		IsAdmin:      user.IsAdmin,
		Product:      p,
		Statuses:     statuses,
		Milestones:   milestones,
		Sprints:      sprints,
		Environments: environments,
		LinkTo:       fmt.Sprintf("campaign-result:%s:%s:%s", campaignID, result.ID, matrixGrouping(r)),
		Title:        fmt.Sprintf("%s failed", tc.Title),
		Criticality:  model.DefectCriticalityMedium,
		StatusID:     defaultStatusID(statuses),
		// The cell already says which environment broke, so carry it over rather
		// than making the tester pick it again.
		EnvironmentID: result.EnvironmentID,
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
		ProductID:     productID,
		Title:         r.FormValue("title"),
		Description:   r.FormValue("description"),
		Criticality:   r.FormValue("criticality"),
		StatusID:      r.FormValue("status_id"),
		MilestoneID:   r.FormValue("milestone_id"),
		SprintID:      r.FormValue("sprint_id"),
		EnvironmentID: r.FormValue("environment_id"),
		Estimation:    r.FormValue("estimation"),
		Priority:      priority,
	}

	rerender := func(message string) {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormModal(productpage.DefectFormProps{
			Meta: templates.PageMeta{
				CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
			},
			DisplayName:   user.DisplayName,
			IsAdmin:       user.IsAdmin,
			Product:       p,
			Statuses:      statuses,
			Milestones:    milestones,
			Sprints:       sprints,
			Environments:  environments,
			Error:         message,
			LinkTo:        fmt.Sprintf("campaign-result:%s:%s:%s", campaignID, result.ID, matrixGrouping(r)),
			Title:         input.Title,
			Description:   input.Description,
			Criticality:   input.Criticality,
			StatusID:      input.StatusID,
			MilestoneID:   input.MilestoneID,
			SprintID:      input.SprintID,
			EnvironmentID: input.EnvironmentID,
			Estimation:    input.Estimation,
			Priority:      r.FormValue("priority"),
		}).Render(r.Context(), w)
	}

	defect, err := c.backlog.CreateDefect(r.Context(), user, input)
	if err != nil {
		rerender(errorText(err))
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defect.ID,
		model.DefectLinkTargetTestCase, result.TestCaseID); err != nil {
		rerender("created defect but failed to link it to the test case: " + errorText(err))
		return
	}

	if _, err := c.testing.SetResultStatus(r.Context(), user, productID, campaignID, resultID,
		model.CampaignResultStatusFailed, defect.ID); err != nil {
		rerender("created defect but failed to record the result: " + errorText(err))
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
		return result, c.campaignTestCase(r, user, productID, result), nil
	}
	return nil, nil, errors.New("campaign result not found")
}
