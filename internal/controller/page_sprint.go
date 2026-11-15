package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
)

// Sprint pages are the scrum counterpart to the kanban board. Every handler
// bounces kanban-mode products to /board, mirroring page_board.go's redirect in
// the other direction, so a product only ever has one live workflow surface.

// Sprints renders GET /products/{id}/sprints.
func (c *PageProductController) Sprints(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, ok := c.loadScrumProduct(w, r, user, productID)
	if !ok {
		return
	}

	sprints, err := c.milestone.ListSprints(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}

	milestoneNames := c.buildMilestoneNameMap(r, user, productID)
	rows := make([]productpage.SprintRow, 0, len(sprints))
	for _, sp := range sprints {
		rows = append(rows, productpage.SprintRow{
			Sprint:        sp,
			MilestoneName: milestoneNames[sp.MilestoneID],
			ItemCount:     c.countSprintItems(r, user, productID, sp.ID),
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SprintsPage(productpage.SprintsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Sprints",
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Sprints:        rows,
		IsOwnerOrAdmin: isOwnerOrAdmin(user, role),
	}).Render(r.Context(), w)
}

// SprintDetail renders GET /products/{id}/sprints/{sprintId}.
func (c *PageProductController) SprintDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	p, role, ok := c.loadScrumProduct(w, r, user, productID)
	if !ok {
		return
	}

	sprint, err := c.milestone.GetSprint(r.Context(), user, productID, sprintID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/sprints", http.StatusSeeOther)
		return
	}

	milestoneNames := c.buildMilestoneNameMap(r, user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SprintDetailPage(productpage.SprintDetailProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - " + sprint.Name,
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Sprint:         sprint,
		MilestoneName:  milestoneNames[sprint.MilestoneID],
		IsOwnerOrAdmin: isOwnerOrAdmin(user, role),
		Board:          c.buildSprintBoard(r, user, p, role, sprintID),
	}).Render(r.Context(), w)
}

// SprintCreateForm renders GET /products/{id}/sprints/new.
func (c *PageProductController) SprintCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, ok := c.loadScrumProduct(w, r, user, productID)
	if !ok {
		return
	}
	if !isOwnerOrAdmin(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/sprints", noticeNeedsOwner)
		return
	}

	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SprintFormPage(productpage.SprintFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - New Sprint",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Milestones:  milestones,
	}).Render(r.Context(), w)
}

// SprintCreate handles POST /products/{id}/sprints.
func (c *PageProductController) SprintCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, ok := c.loadScrumProduct(w, r, user, productID)
	if !ok {
		return
	}

	name := r.FormValue("name")
	milestoneID := r.FormValue("milestone_id")
	startDate := r.FormValue("start_date")
	endDate := r.FormValue("end_date")

	_, err := c.milestone.CreateSprint(r.Context(), user, service.CreateSprintInput{
		ProductID:   productID,
		Name:        name,
		MilestoneID: milestoneID,
		StartDate:   sprintDateToRFC3339(startDate),
		EndDate:     sprintDateToRFC3339(endDate),
	})
	if err != nil {
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.SprintFormPage(productpage.SprintFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - New Sprint",
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Milestones:  milestones,
			Name:        name,
			MilestoneID: milestoneID,
			StartDate:   startDate,
			EndDate:     endDate,
			Error:       errorText(err),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/sprints", http.StatusSeeOther)
}

// SprintEditForm renders GET /products/{id}/sprints/{sprintId}/edit.
func (c *PageProductController) SprintEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	p, role, ok := c.loadScrumProduct(w, r, user, productID)
	if !ok {
		return
	}
	if !isOwnerOrAdmin(user, role) {
		redirectWithNotice(w, r, fmt.Sprintf("/products/%s/sprints/%s", productID, sprintID), noticeNeedsOwner)
		return
	}

	sprint, err := c.milestone.GetSprint(r.Context(), user, productID, sprintID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/sprints", http.StatusSeeOther)
		return
	}

	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SprintFormPage(productpage.SprintFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Edit " + sprint.Name,
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Sprint:      sprint,
		Milestones:  milestones,
		IsEdit:      true,
		ReturnTo:    editReturnTarget(r),
		Name:        sprint.Name,
		MilestoneID: sprint.MilestoneID,
		StartDate:   milestoneEditDate(sprint.StartDate),
		EndDate:     milestoneEditDate(sprint.EndDate),
	}).Render(r.Context(), w)
}

// SprintUpdate handles POST /products/{id}/sprints/{sprintId}.
func (c *PageProductController) SprintUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	p, _, ok := c.loadScrumProduct(w, r, user, productID)
	if !ok {
		return
	}

	sprint, err := c.milestone.GetSprint(r.Context(), user, productID, sprintID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/sprints", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	milestoneID := r.FormValue("milestone_id")
	startDate := r.FormValue("start_date")
	endDate := r.FormValue("end_date")
	startRFC := sprintDateToRFC3339(startDate)
	endRFC := sprintDateToRFC3339(endDate)

	_, err = c.milestone.UpdateSprint(r.Context(), user, productID, sprintID, service.UpdateSprintInput{
		IfUpdatedAt: r.FormValue("updated_at"),
		Name:        &name,
		MilestoneID: &milestoneID,
		StartDate:   &startRFC,
		EndDate:     &endRFC,
	})
	if err != nil {
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.SprintFormPage(productpage.SprintFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - Edit " + sprint.Name,
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Sprint:      sprint,
			Milestones:  milestones,
			IsEdit:      true,
			ReturnTo:    r.FormValue("return"),
			Name:        name,
			MilestoneID: milestoneID,
			StartDate:   startDate,
			EndDate:     endDate,
			Error:       errorText(err),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		fmt.Sprintf("/products/%s/sprints/%s", productID, sprintID),
		fmt.Sprintf("/products/%s/sprints", productID),
	), http.StatusSeeOther)
}

// SprintDelete handles DELETE /products/{id}/sprints/{sprintId}.
func (c *PageProductController) SprintDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	if err := c.milestone.DeleteSprint(r.Context(), user, productID, sprintID); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("HX-Redirect", "/products/"+productID+"/sprints")
	w.WriteHeader(http.StatusOK)
}

// SprintDeleteConfirm renders GET /products/{id}/sprints/{sprintId}/delete/confirm.
func (c *PageProductController) SprintDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	sprint, err := c.milestone.GetSprint(r.Context(), user, productID, sprintID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SprintDeleteConfirmation(sprint.Name, productID, sprintID,
		middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// SprintStart handles POST /products/{id}/sprints/{sprintId}/start.
func (c *PageProductController) SprintStart(w http.ResponseWriter, r *http.Request) {
	c.setSprintStatus(w, r, model.SprintStatusActive)
}

// SprintClose handles POST /products/{id}/sprints/{sprintId}/close.
func (c *PageProductController) SprintClose(w http.ResponseWriter, r *http.Request) {
	c.setSprintStatus(w, r, model.SprintStatusCompleted)
}

// setSprintStatus applies a lifecycle transition and reloads the detail page.
// The status badge, the available action and the board all change together, so
// a full reload is both simpler and more honest than a partial swap.
func (c *PageProductController) setSprintStatus(w http.ResponseWriter, r *http.Request, status string) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	if _, err := c.milestone.SetSprintStatus(r.Context(), user, productID, sprintID, status); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("HX-Redirect", fmt.Sprintf("/products/%s/sprints/%s", productID, sprintID))
	w.WriteHeader(http.StatusOK)
}

// --- Tabs ---

// SprintBoardTab renders GET /products/{id}/sprints/{sprintId}/tabs/board.
func (c *PageProductController) SprintBoardTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SprintBoardTabContent(c.buildSprintBoard(r, user, p, role, sprintID)).Render(r.Context(), w)
}

// SprintPlanningTab renders GET /products/{id}/sprints/{sprintId}/tabs/planning.
func (c *PageProductController) SprintPlanningTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	props := c.buildSprintPlanning(r, user, productID, sprintID)
	// Assigning a sprint is a full requirement update, which the service
	// restricts to owners — so planning is owner-only, not developer-only.
	props.CanPlan = isOwnerOrAdmin(user, role)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SprintPlanningTabContent(props).Render(r.Context(), w)
}

// SprintSummaryTab renders GET /products/{id}/sprints/{sprintId}/tabs/summary.
func (c *PageProductController) SprintSummaryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	items, defects := c.loadSprintItems(r, user, productID, sprintID)
	reqStatuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	defStatuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SprintSummaryTabContent(
		buildSprintSummary(p, items, defects, reqStatuses, defStatuses),
	).Render(r.Context(), w)
}

// SprintHistoryTab renders GET /products/{id}/sprints/{sprintId}/tabs/history.
func (c *PageProductController) SprintHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	sprintID := r.PathValue("sprintId")

	views := c.buildAuditViews(r, user, productID, "sprint", sprintID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SprintHistoryTabContent(views).Render(r.Context(), w)
}

// SprintItemMove handles PATCH /products/{id}/sprints/{sprintId}/items/{itemId}.
// The form carries `entity_type` and the target `sprint_id`; an empty sprint_id
// removes the item from the sprint. Returns 204, which is what board.js expects.
func (c *PageProductController) SprintItemMove(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	itemID := r.PathValue("itemId")

	entityType := strings.TrimSpace(r.FormValue("entity_type"))
	sprintID := strings.TrimSpace(r.FormValue("sprint_id"))

	var err error
	switch entityType {
	case "defect":
		_, err = c.backlog.UpdateDefect(r.Context(), user, productID, itemID, service.UpdateDefectInput{
			IfUpdatedAt: r.FormValue("updated_at"),
			SprintID:    &sprintID,
		})
	case model.RequirementKindRequirement, model.RequirementKindTask:
		// Tasks live in the requirements table, so both take the same path.
		_, err = c.backlog.UpdateRequirement(r.Context(), user, productID, itemID, service.UpdateRequirementInput{
			IfUpdatedAt: r.FormValue("updated_at"),
			SprintID:    &sprintID,
		})
	default:
		http.Error(w, fmt.Sprintf("unknown entity type %q", entityType), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, errorText(err), http.StatusForbidden)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

// loadScrumProduct loads the product and bounces kanban-mode products to the
// board. It reports whether the caller should continue.
func (c *PageProductController) loadScrumProduct(w http.ResponseWriter, r *http.Request, user *model.User, productID string) (*model.Product, string, bool) {
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return nil, "", false
	}
	if p.WorkflowMode != model.WorkflowModeScrum {
		http.Redirect(w, r, "/products/"+productID+"/board", http.StatusSeeOther)
		return nil, "", false
	}
	return p, role, true
}

func isOwnerOrAdmin(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner
}

func (c *PageProductController) buildMilestoneNameMap(r *http.Request, user *model.User, productID string) map[string]string {
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	names := make(map[string]string, len(milestones))
	for _, m := range milestones {
		names[m.ID] = m.Name
	}
	return names
}

// loadSprintItems returns the backlog items and defects assigned to a sprint.
func (c *PageProductController) loadSprintItems(r *http.Request, user *model.User, productID, sprintID string) ([]*model.Requirement, []*model.Defect) {
	items, _ := c.backlog.ListRequirements(r.Context(), user, productID, store.RequirementFilter{SprintID: sprintID})
	defects, _ := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{SprintID: sprintID})
	return items, defects
}

func (c *PageProductController) countSprintItems(r *http.Request, user *model.User, productID, sprintID string) int {
	items, defects := c.loadSprintItems(r, user, productID, sprintID)
	return len(items) + len(defects)
}

// buildSprintBoard projects the sprint's items onto the product's status
// columns — the same projection the product board uses, scoped by sprint.
func (c *PageProductController) buildSprintBoard(r *http.Request, user *model.User, p *model.Product, role, sprintID string) templates.BoardSurfaceProps {
	reqStatuses, _ := c.milestone.ListStatuses(r.Context(), user, p.ID, "requirement")
	defStatuses, _ := c.milestone.ListStatuses(r.Context(), user, p.ID, "defect")
	items, defects := c.loadSprintItems(r, user, p.ID, sprintID)
	prefixes := c.buildBacklogPrefixMap(r, user, p.ID)

	return buildBoardSurface(p.ID, p, canMoveCards(user, role), canMoveInactive(user, role),
		reqStatuses, defStatuses, items, defects, prefixes, "sprint")
}

// buildSprintPlanning splits the product's open work into "not in any sprint"
// and "in this sprint". Closed items are left out: a sprint is planned from
// work that still needs doing.
func (c *PageProductController) buildSprintPlanning(r *http.Request, user *model.User, productID, sprintID string) productpage.SprintPlanningTabProps {
	prefixes := c.buildBacklogPrefixMap(r, user, productID)
	closedReq := c.closedStatusIDs(r, user, productID, "requirement")
	closedDef := c.closedStatusIDs(r, user, productID, "defect")
	moveURL := func(itemID string) string {
		return fmt.Sprintf("/products/%s/sprints/%s/items/%s", productID, sprintID, itemID)
	}

	props := productpage.SprintPlanningTabProps{ProductID: productID, SprintID: sprintID}

	allItems, _ := c.backlog.ListRequirements(r.Context(), user, productID, store.RequirementFilter{})
	for _, item := range allItems {
		side, ok := planningSide(item.SprintID, item.StatusID, sprintID, closedReq)
		if !ok {
			continue
		}
		card := backlogCard(productID, item, prefixes)
		card.MoveURL = moveURL(item.ID)
		if side == planningSideSprint {
			props.InSprint = append(props.InSprint, card)
		} else {
			props.Backlog = append(props.Backlog, card)
		}
	}

	allDefects, _ := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{})
	for _, d := range allDefects {
		side, ok := planningSide(d.SprintID, d.StatusID, sprintID, closedDef)
		if !ok {
			continue
		}
		card := defectCard(productID, d)
		card.MoveURL = moveURL(d.ID)
		if side == planningSideSprint {
			props.InSprint = append(props.InSprint, card)
		} else {
			props.Backlog = append(props.Backlog, card)
		}
	}

	return props
}

// Which side of the planning surface an item belongs on.
const (
	planningSideBacklog = iota
	planningSideSprint
)

// planningSide decides where an item sits on the planning surface, and whether
// it belongs there at all. Items in this sprint are always shown, whatever their
// status — a sprint's contents must stay visible so they can be dragged out.
// The backlog side shows only unassigned, still-open work.
func planningSide(itemSprintID, statusID, sprintID string, closed map[string]bool) (side int, include bool) {
	if itemSprintID == sprintID {
		return planningSideSprint, true
	}
	if itemSprintID != "" || closed[statusID] {
		return planningSideBacklog, false
	}
	return planningSideBacklog, true
}

func (c *PageProductController) closedStatusIDs(r *http.Request, user *model.User, productID, entityType string) map[string]bool {
	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, entityType)
	closed := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		if s.IsClosed {
			closed[s.ID] = true
		}
	}
	return closed
}

// buildSprintSummary computes completion stats for the sprint's contents. It
// works off data the caller already loaded — there is no historical series
// behind this, so velocity and burndown are deliberately absent.
func buildSprintSummary(
	p *model.Product,
	items []*model.Requirement,
	defects []*model.Defect,
	reqStatuses, defStatuses []*model.Status,
) productpage.SprintSummaryTabProps {
	closedReq := closedStatusSet(reqStatuses)
	closedDef := closedStatusSet(defStatuses)
	statusNames := map[string]*model.Status{}
	for _, s := range reqStatuses {
		statusNames[s.ID] = s
	}
	for _, s := range defStatuses {
		statusNames[s.ID] = s
	}

	props := productpage.SprintSummaryTabProps{
		TotalItems: len(items) + len(defects),
	}

	// Counting by status name rather than id merges the parallel requirement
	// and defect sets, which is how the merged board presents them too.
	counts := map[string]int{}
	closedByName := map[string]bool{}
	defaultByName := map[string]bool{}
	var order []string
	var totalEstimation, completedEstimation float64
	track := func(statusID, estimation string, closed bool) {
		name := "No status"
		isDefault := true // an item with no status sits at the entry column
		if s, ok := statusNames[statusID]; ok {
			name = s.Name
			isDefault = s.IsDefault
		}
		if _, seen := counts[name]; !seen {
			order = append(order, name)
		}
		counts[name]++
		closedByName[name] = closed
		defaultByName[name] = isDefault

		value, ok := parseEstimation(estimation)
		if ok {
			totalEstimation += value
		}
		if closed {
			props.CompletedItems++
			if ok {
				completedEstimation += value
			}
		}
	}

	for _, item := range items {
		track(item.StatusID, item.Estimation, closedReq[item.StatusID])
	}
	for _, d := range defects {
		track(d.StatusID, d.Estimation, closedDef[d.StatusID])
	}

	for _, name := range order {
		props.Statuses = append(props.Statuses, productpage.SprintStatusCount{
			Name: name, IsDefault: defaultByName[name], IsClosed: closedByName[name], Count: counts[name],
		})
	}

	if props.TotalItems > 0 {
		props.Percent = props.CompletedItems * 100 / props.TotalItems
	}

	if label, ok := numericEstimationLabel(p.EstimationMode); ok {
		props.ShowEstimation = true
		props.EstimationLabel = label
		props.TotalEstimation = formatEstimation(totalEstimation)
		props.CompletedEstimation = formatEstimation(completedEstimation)
	}

	return props
}

func closedStatusSet(statuses []*model.Status) map[string]bool {
	closed := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		if s.IsClosed {
			closed[s.ID] = true
		}
	}
	return closed
}

// numericEstimationLabel reports whether estimations can be summed, and under
// what name. T-shirt sizes are ordinal, so they get counts only.
func numericEstimationLabel(mode string) (string, bool) {
	switch mode {
	case model.EstimationModeStoryPoints:
		return "points", true
	case model.EstimationModeHours:
		return "hours", true
	default:
		return "", false
	}
}

// parseEstimation reads an estimation as a number. Values that don't parse
// (a stray t-shirt size, a free-text note) are skipped rather than counted as
// zero, so the total stays honest.
func parseEstimation(estimation string) (float64, bool) {
	estimation = strings.TrimSpace(estimation)
	if estimation == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(estimation, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func formatEstimation(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// sprintDateToRFC3339 widens a date input's YYYY-MM-DD into the RFC 3339 the
// store expects. Anything already carrying a time is passed through.
func sprintDateToRFC3339(date string) string {
	date = strings.TrimSpace(date)
	if len(date) == 10 {
		return date + "T00:00:00Z"
	}
	return date
}
