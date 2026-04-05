package controller

import (
	"fmt"
	"net/http"
	"strings"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
)

// PageProductController handles product pages.
type PageProductController struct {
	product    *service.ProductService
	dashboard  *service.DashboardService
	settings   *service.SettingService
	user       *service.UserService
	invitation *service.InvitationService
	milestone  *service.MilestoneService
	audit      *service.AuditService
}

// NewPageProduct creates a PageProductController.
func NewPageProduct(product *service.ProductService, dashboard *service.DashboardService, settings *service.SettingService, user *service.UserService, invitation *service.InvitationService, milestone *service.MilestoneService, audit *service.AuditService) *PageProductController {
	return &PageProductController{product: product, dashboard: dashboard, settings: settings, user: user, invitation: invitation, milestone: milestone, audit: audit}
}

// CreateForm renders GET /products/new.
func (c *PageProductController) CreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	if !c.canCreateProduct(r, user) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CreatePage(productpage.CreateProps{
		Meta: templates.PageMeta{
			Title:     "Create Product",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
	}).Render(r.Context(), w)
}

// Create handles POST /products/new.
func (c *PageProductController) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	if !c.canCreateProduct(r, user) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	estimationMode := r.FormValue("estimation_mode")
	workflowMode := r.FormValue("workflow_mode")

	product, err := c.product.Create(r.Context(), user, service.CreateProductInput{
		Name:           name,
		Description:    description,
		EstimationMode: estimationMode,
		WorkflowMode:   workflowMode,
	})
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.CreatePage(productpage.CreateProps{
			Meta: templates.PageMeta{
				Title:     "Create Product",
				CSRFToken: csrfToken,
			},
			DisplayName:    user.DisplayName,
			IsAdmin:        user.IsAdmin,
			Name:           name,
			Description:    description,
			EstimationMode: estimationMode,
			WorkflowMode:   workflowMode,
			Error:          err.Error(),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+product.ID, http.StatusSeeOther)
}

// Overview renders GET /products/{id}.
func (c *PageProductController) Overview(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	summaries, err := c.dashboard.ProductSummaries(r.Context(), user)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var summary *service.ProductSummary
	for i := range summaries {
		if summaries[i].Product.ID == productID {
			summary = &summaries[i]
			break
		}
	}
	if summary == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	isOwnerOrAdmin := user.IsAdmin || summary.Role == model.RoleOwner

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.OverviewPage(productpage.OverviewProps{
		Meta: templates.PageMeta{
			Title:     summary.Product.Name,
			CSRFToken: csrfToken,
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          summary.Product,
		Role:             summary.Role,
		OpenRequirements: summary.OpenRequirements,
		ActiveDefects:    summary.ActiveDefects,
		CurrentSprint:    summary.CurrentSprint,
		IsOwnerOrAdmin:   isOwnerOrAdmin,
	}).Render(r.Context(), w)
}

// Settings renders GET /products/{id}/settings.
func (c *PageProductController) Settings(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !user.IsAdmin && role != model.RoleOwner {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SettingsPage(productpage.SettingsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Settings",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
	}).Render(r.Context(), w)
}

// SettingsGeneral renders the General tab content fragment.
func (c *PageProductController) SettingsGeneral(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	p, err := c.product.GetByID(r.Context(), user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.GeneralTabContent(p).Render(r.Context(), w)
}

// SettingsStatuses renders the Statuses tab content fragment.
func (c *PageProductController) SettingsStatuses(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	product, err := c.product.GetByID(r.Context(), user, productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var defStatuses []*model.Status
	if !product.StatusSync {
		defStatuses, err = c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.StatusesTabContent(productID, product.StatusSync, reqStatuses, defStatuses).Render(r.Context(), w)
}

// SettingsEnvironments renders the Environments tab content fragment.
func (c *PageProductController) SettingsEnvironments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	envs, err := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.EnvironmentsTabContent(productID, envs).Render(r.Context(), w)
}

// SettingsPyramid renders the Pyramid Levels tab content fragment.
func (c *PageProductController) SettingsPyramid(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	levels, err := c.milestone.ListPyramidLevels(r.Context(), user, productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.PyramidTabContent(productID, levels).Render(r.Context(), w)
}

// CreateStatus handles POST /products/{id}/settings/statuses.
func (c *PageProductController) CreateStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	entityType := r.FormValue("entity_type")
	name := r.FormValue("name")

	if name == "" || entityType == "" {
		http.Error(w, "name and entity_type are required", http.StatusBadRequest)
		return
	}

	// Determine position: append to end.
	existing, _ := c.milestone.ListStatuses(r.Context(), user, productID, entityType)
	position := len(existing) + 1

	isDefault := r.FormValue("is_default") == "on"
	isClosed := r.FormValue("is_closed") == "on"

	_, err := c.milestone.CreateStatus(r.Context(), user, service.CreateStatusInput{
		ProductID:  productID,
		EntityType: entityType,
		Name:       name,
		Position:   position,
		IsDefault:  isDefault,
		IsClosed:   isClosed,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	product, err := c.product.GetByID(r.Context(), user, productID)
	if err == nil && product.StatusSync {
		c.renderSyncedStatusList(w, r, user, productID)
	} else {
		c.renderStatusList(w, r, user, productID, entityType)
	}
}

// UpdateStatus handles PATCH /products/{id}/settings/statuses/{statusId}.
func (c *PageProductController) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	statusID := r.PathValue("statusId")

	input := service.UpdateStatusInput{}
	if v := r.FormValue("name"); v != "" {
		input.Name = &v
	}
	if r.Form != nil {
		if _, ok := r.Form["is_default"]; ok {
			val := r.FormValue("is_default") == "on"
			input.IsDefault = &val
		}
		if _, ok := r.Form["is_closed"]; ok {
			val := r.FormValue("is_closed") == "on"
			input.IsClosed = &val
		}
	}

	// Get the status to know its entity type for re-rendering.
	status, err := c.milestone.GetStatus(r.Context(), user, productID, statusID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	_, err = c.milestone.UpdateStatus(r.Context(), user, productID, statusID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	product, pErr := c.product.GetByID(r.Context(), user, productID)
	if pErr == nil && product.StatusSync {
		c.renderSyncedStatusList(w, r, user, productID)
	} else {
		c.renderStatusList(w, r, user, productID, status.EntityType)
	}
}

// DeleteStatus handles DELETE /products/{id}/settings/statuses/{statusId}.
func (c *PageProductController) DeleteStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	statusID := r.PathValue("statusId")

	// Get the status to know its entity type for re-rendering.
	status, err := c.milestone.GetStatus(r.Context(), user, productID, statusID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := c.milestone.DeleteStatus(r.Context(), user, productID, statusID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	product, pErr := c.product.GetByID(r.Context(), user, productID)
	if pErr == nil && product.StatusSync {
		c.renderSyncedStatusList(w, r, user, productID)
	} else {
		c.renderStatusList(w, r, user, productID, status.EntityType)
	}
}

// renderStatusList re-renders the status list fragment for a given entity type.
func (c *PageProductController) renderStatusList(w http.ResponseWriter, r *http.Request, user *model.User, productID, entityType string) {
	statuses, err := c.milestone.ListStatuses(r.Context(), user, productID, entityType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.StatusListFragment(productID, entityType, statuses).Render(r.Context(), w)
}

// renderSyncedStatusList re-renders the synced status list fragment.
func (c *PageProductController) renderSyncedStatusList(w http.ResponseWriter, r *http.Request, user *model.User, productID string) {
	statuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SyncedStatusListFragment(productID, statuses).Render(r.Context(), w)
}

// SplitStatuses handles POST /products/{id}/settings/statuses/split.
func (c *PageProductController) SplitStatuses(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	if err := c.product.SplitStatuses(r.Context(), user, productID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// Re-render the full statuses tab content in split mode.
	reqStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.StatusesTabContent(productID, false, reqStatuses, defStatuses).Render(r.Context(), w)
}

// CreateEnvironment handles POST /products/{id}/settings/environments.
func (c *PageProductController) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	name := r.FormValue("name")

	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	_, err := c.milestone.CreateRuntimeEnvironment(r.Context(), user, productID, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.renderEnvironmentList(w, r, user, productID)
}

// DeleteEnvironment handles DELETE /products/{id}/settings/environments/{envId}.
func (c *PageProductController) DeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	envID := r.PathValue("envId")

	if err := c.milestone.DeleteRuntimeEnvironment(r.Context(), user, productID, envID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.renderEnvironmentList(w, r, user, productID)
}

// renderEnvironmentList re-renders the environment list fragment.
func (c *PageProductController) renderEnvironmentList(w http.ResponseWriter, r *http.Request, user *model.User, productID string) {
	envs, err := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.EnvironmentListFragment(productID, envs).Render(r.Context(), w)
}

// CreatePyramidLevel handles POST /products/{id}/settings/pyramid.
func (c *PageProductController) CreatePyramidLevel(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	name := r.FormValue("name")
	parentID := r.FormValue("parent_id")

	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	// Determine position: append to end.
	existing, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
	position := 1
	for _, l := range existing {
		if l.ParentID == parentID && l.Position >= position {
			position = l.Position + 1
		}
	}

	_, err := c.milestone.CreatePyramidLevel(r.Context(), user, service.CreatePyramidLevelInput{
		ProductID: productID,
		ParentID:  parentID,
		Name:      name,
		Position:  position,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.renderPyramidList(w, r, user, productID)
}

// UpdatePyramidLevel handles PATCH /products/{id}/settings/pyramid/{levelId}.
func (c *PageProductController) UpdatePyramidLevel(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	levelID := r.PathValue("levelId")

	input := service.UpdatePyramidLevelInput{}
	if v := r.FormValue("name"); v != "" {
		input.Name = &v
	}

	_, err := c.milestone.UpdatePyramidLevel(r.Context(), user, productID, levelID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.renderPyramidList(w, r, user, productID)
}

// DeletePyramidLevel handles DELETE /products/{id}/settings/pyramid/{levelId}.
func (c *PageProductController) DeletePyramidLevel(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	levelID := r.PathValue("levelId")

	if err := c.milestone.DeletePyramidLevel(r.Context(), user, productID, levelID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.renderPyramidList(w, r, user, productID)
}

// renderPyramidList re-renders the pyramid level list fragment.
func (c *PageProductController) renderPyramidList(w http.ResponseWriter, r *http.Request, user *model.User, productID string) {
	levels, err := c.milestone.ListPyramidLevels(r.Context(), user, productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.PyramidListFragment(productID, levels).Render(r.Context(), w)
}

// UpdateProduct handles PATCH /products/{id}/settings (auto-save).
func (c *PageProductController) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	field := r.FormValue("field")
	value := r.FormValue("value")

	input := service.UpdateProductInput{}
	switch field {
	case "name":
		input.Name = &value
	case "description":
		input.Description = &value
	case "estimation_mode":
		input.EstimationMode = &value
	case "workflow_mode":
		input.WorkflowMode = &value
	default:
		http.Error(w, "unknown field", http.StatusBadRequest)
		return
	}

	updated, err := c.product.Update(r.Context(), user, productID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.SettingField(updated, field).Render(r.Context(), w)
}

// Team renders GET /products/{id}/team.
func (c *PageProductController) Team(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	isOwnerOrAdmin := user.IsAdmin || role == model.RoleOwner
	members := c.buildTeamMembers(r, user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TeamPage(productpage.TeamProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Team",
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Members:        members,
		IsOwnerOrAdmin: isOwnerOrAdmin,
	}).Render(r.Context(), w)
}

// AddMember handles POST /products/{id}/team/members.
func (c *PageProductController) AddMember(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	targetUserID := r.FormValue("user_id")
	role := r.FormValue("role")

	if targetUserID == "" || role == "" {
		http.Error(w, "user and role are required", http.StatusBadRequest)
		return
	}

	if err := c.product.AddMember(r.Context(), user, productID, targetUserID, role); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	_, role2, _ := c.loadProductWithRole(r, user, productID)
	isOwnerOrAdmin := user.IsAdmin || role2 == model.RoleOwner
	members := c.buildTeamMembers(r, user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MemberTableBody(members, productID, isOwnerOrAdmin).Render(r.Context(), w)
	_ = productpage.AddMemberFormOOB(productID, "").Render(r.Context(), w)
}

// UpdateMemberRole handles PATCH /products/{id}/team/members/{userId}.
func (c *PageProductController) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	targetUserID := r.PathValue("userId")

	role := r.FormValue("role")
	if role == "" {
		http.Error(w, "role is required", http.StatusBadRequest)
		return
	}

	if err := c.product.UpdateMember(r.Context(), user, productID, targetUserID, role); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	_, callerRole, _ := c.loadProductWithRole(r, user, productID)
	isOwnerOrAdmin := user.IsAdmin || callerRole == model.RoleOwner
	members := c.buildTeamMembers(r, user, productID)

	// Find the updated member row.
	for _, m := range members {
		if m.UserID == targetUserID {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = productpage.MemberRow(m, productID, isOwnerOrAdmin).Render(r.Context(), w)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveMember handles DELETE /products/{id}/team/members/{userId}.
func (c *PageProductController) RemoveMember(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	targetUserID := r.PathValue("userId")

	if err := c.product.RemoveMember(r.Context(), user, productID, targetUserID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// TransferOwnership handles POST /products/{id}/team/transfer.
func (c *PageProductController) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	newOwnerID := r.FormValue("new_owner_id")
	if newOwnerID == "" {
		http.Error(w, "new owner is required", http.StatusBadRequest)
		return
	}

	if err := c.product.TransferOwnership(r.Context(), user, productID, newOwnerID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// After transfer, current user is no longer owner — redirect to reload the page.
	w.Header().Set("HX-Redirect", fmt.Sprintf("/products/%s/team", productID))
	w.WriteHeader(http.StatusOK)
}

// SearchUsers handles GET /products/{id}/team/search-users?q=...
func (c *PageProductController) SearchUsers(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	query := r.URL.Query().Get("q")

	if len(query) < 2 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return
	}

	// Get current members to exclude.
	members, _ := c.product.ListMembers(r.Context(), user, productID)
	memberSet := make(map[string]bool, len(members))
	for _, m := range members {
		memberSet[m.UserID] = true
	}

	// Search all active users and filter in Go (personal data is encrypted).
	allUsers, _ := c.user.List(r.Context(), store.UserFilter{Status: "active"})
	queryLower := strings.ToLower(query)

	var matches []productpage.UserSearchResult
	for _, u := range allUsers {
		if memberSet[u.ID] {
			continue
		}
		nameMatch := strings.Contains(strings.ToLower(u.DisplayName), queryLower)
		usernameMatch := strings.Contains(strings.ToLower(u.Username), queryLower)
		if nameMatch || usernameMatch {
			matches = append(matches, productpage.UserSearchResult{
				UserID:      u.ID,
				DisplayName: u.DisplayName,
				Username:    u.Username,
			})
		}
		if len(matches) >= 10 {
			break
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.UserSearchResults(matches).Render(r.Context(), w)
}

// InviteMember handles POST /products/{id}/team/invite.
func (c *PageProductController) InviteMember(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	email := r.FormValue("email")
	role := r.FormValue("role")

	if email == "" || role == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.InviteFormOOB(productID, "Email and role are required.").Render(r.Context(), w)
		return
	}

	result, err := c.invitation.Create(r.Context(), user.ID, email, productID, role)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.InviteFormOOB(productID, err.Error()).Render(r.Context(), w)
		return
	}

	inviteURL := fmt.Sprintf("/accept-invite/%s", result.RawToken)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.InviteSuccessOOB(productID, inviteURL).Render(r.Context(), w)
}

// InviteFormReset handles GET /products/{id}/team/invite-form (resets the invite form).
func (c *PageProductController) InviteFormReset(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.InviteForm(productID, "").Render(r.Context(), w)
}

// DeleteProduct handles DELETE /products/{id}.
func (c *PageProductController) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	if err := c.product.Delete(r.Context(), user, productID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

// DeleteProductConfirm handles GET /products/{id}/delete/confirm.
func (c *PageProductController) DeleteProductConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	p, err := c.product.GetByID(r.Context(), user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DeleteProductConfirmation(p.Name, productID).Render(r.Context(), w)
}

// --- helpers ---

func (c *PageProductController) canCreateProduct(r *http.Request, user *model.User) bool {
	if user.IsAdmin {
		return true
	}
	val, _ := c.settings.Get(r.Context(), "product_creation_enabled")
	return val == "true"
}

func (c *PageProductController) loadProductWithRole(r *http.Request, user *model.User, productID string) (*model.Product, string, error) {
	p, err := c.product.GetByID(r.Context(), user, productID)
	if err != nil {
		return nil, "", err
	}

	role := ""
	if user.IsAdmin {
		role = "admin"
	}
	members, err := c.product.ListMembers(r.Context(), user, productID)
	if err == nil {
		for _, m := range members {
			if m.UserID == user.ID {
				role = m.Role
				break
			}
		}
	}
	return p, role, nil
}

func (c *PageProductController) buildTeamMembers(r *http.Request, user *model.User, productID string) []productpage.TeamMemberView {
	members, _ := c.product.ListMembers(r.Context(), user, productID)

	allUsers, _ := c.user.List(r.Context(), store.UserFilter{Status: "active"})
	userMap := make(map[string]*model.User, len(allUsers))
	for _, u := range allUsers {
		userMap[u.ID] = u
	}

	views := make([]productpage.TeamMemberView, 0, len(members))
	for _, m := range members {
		displayName := m.UserID
		username := ""
		if u, ok := userMap[m.UserID]; ok {
			displayName = u.DisplayName
			username = u.Username
		}
		views = append(views, productpage.TeamMemberView{
			UserID:      m.UserID,
			DisplayName: displayName,
			Username:    username,
			Role:        m.Role,
			IsOwner:     m.Role == model.RoleOwner,
		})
	}

	return views
}

// --- Milestones ---

// Milestones renders GET /products/{id}/milestones.
func (c *PageProductController) Milestones(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	milestones, err := c.milestone.ListMilestones(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}

	view := r.URL.Query().Get("view")
	if view != "timeline" {
		view = "table"
	}

	isOwnerOrAdmin := user.IsAdmin || role == model.RoleOwner

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestonesPage(productpage.MilestonesProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Milestones",
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Milestones:     milestones,
		IsOwnerOrAdmin: isOwnerOrAdmin,
		View:           view,
	}).Render(r.Context(), w)
}

// MilestoneDetail renders GET /products/{id}/milestones/{milestoneId}.
func (c *PageProductController) MilestoneDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	m, err := c.milestone.GetMilestone(r.Context(), user, productID, milestoneID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/milestones", http.StatusSeeOther)
		return
	}

	isOwnerOrAdmin := user.IsAdmin || role == model.RoleOwner

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneDetailPage(productpage.MilestoneDetailProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - " + m.Name,
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Milestone:      m,
		IsOwnerOrAdmin: isOwnerOrAdmin,
	}).Render(r.Context(), w)
}

// MilestoneCreateForm renders GET /products/{id}/milestones/new.
func (c *PageProductController) MilestoneCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !user.IsAdmin && role != model.RoleOwner {
		http.Redirect(w, r, "/products/"+productID+"/milestones", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneFormPage(productpage.MilestoneFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - New Milestone",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
	}).Render(r.Context(), w)
}

// MilestoneCreate handles POST /products/{id}/milestones.
func (c *PageProductController) MilestoneCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	milestoneType := r.FormValue("type")
	dueDate := r.FormValue("due_date")

	_, err = c.milestone.CreateMilestone(r.Context(), user, service.CreateMilestoneInput{
		ProductID:   productID,
		Name:        name,
		Description: description,
		Type:        milestoneType,
		DueDate:     dueDate,
	})
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.MilestoneFormPage(productpage.MilestoneFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - New Milestone",
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Name:        name,
			Description: description,
			Type:        milestoneType,
			DueDate:     dueDate,
			Error:       err.Error(),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/milestones", http.StatusSeeOther)
}

// MilestoneEditForm renders GET /products/{id}/milestones/{milestoneId}/edit.
func (c *PageProductController) MilestoneEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !user.IsAdmin && role != model.RoleOwner {
		http.Redirect(w, r, fmt.Sprintf("/products/%s/milestones/%s", productID, milestoneID), http.StatusSeeOther)
		return
	}

	m, err := c.milestone.GetMilestone(r.Context(), user, productID, milestoneID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/milestones", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneFormPage(productpage.MilestoneFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Edit " + m.Name,
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Milestone:   m,
		IsEdit:      true,
		Name:        m.Name,
		Description: m.Description,
		Type:        m.Type,
		DueDate:     milestoneEditDate(m.DueDate),
	}).Render(r.Context(), w)
}

// MilestoneUpdate handles POST /products/{id}/milestones/{milestoneId}.
func (c *PageProductController) MilestoneUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	m, err := c.milestone.GetMilestone(r.Context(), user, productID, milestoneID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/milestones", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	milestoneType := r.FormValue("type")
	dueDate := r.FormValue("due_date")

	input := service.UpdateMilestoneInput{
		Name:        &name,
		Description: &description,
		Type:        &milestoneType,
		DueDate:     &dueDate,
	}

	_, err = c.milestone.UpdateMilestone(r.Context(), user, productID, milestoneID, input)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.MilestoneFormPage(productpage.MilestoneFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - Edit " + m.Name,
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Milestone:   m,
			IsEdit:      true,
			Name:        name,
			Description: description,
			Type:        milestoneType,
			DueDate:     dueDate,
			Error:       err.Error(),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/products/%s/milestones/%s", productID, milestoneID), http.StatusSeeOther)
}

// MilestoneDelete handles DELETE /products/{id}/milestones/{milestoneId}.
func (c *PageProductController) MilestoneDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if err := c.milestone.DeleteMilestone(r.Context(), user, productID, milestoneID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("HX-Redirect", "/products/"+productID+"/milestones")
	w.WriteHeader(http.StatusOK)
}

// MilestoneDeleteConfirm renders GET /products/{id}/milestones/{milestoneId}/delete/confirm.
func (c *PageProductController) MilestoneDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	m, err := c.milestone.GetMilestone(r.Context(), user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneDeleteConfirmation(m.Name, productID, milestoneID).Render(r.Context(), w)
}

// MilestoneOverviewTab renders GET /products/{id}/milestones/{milestoneId}/tabs/overview.
func (c *PageProductController) MilestoneOverviewTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	m, err := c.milestone.GetMilestone(r.Context(), user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneOverviewTabContent(m).Render(r.Context(), w)
}

// MilestoneItemsTab renders GET /products/{id}/milestones/{milestoneId}/tabs/items.
func (c *PageProductController) MilestoneItemsTab(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneItemsTabContent(productID).Render(r.Context(), w)
}

// MilestoneSprintsTab renders GET /products/{id}/milestones/{milestoneId}/tabs/sprints.
func (c *PageProductController) MilestoneSprintsTab(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneSprintsTabContent(productID).Render(r.Context(), w)
}

// MilestoneHistoryTab renders GET /products/{id}/milestones/{milestoneId}/tabs/history.
func (c *PageProductController) MilestoneHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	entries, err := c.audit.ListByEntity(r.Context(), user, productID, "milestone", milestoneID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Resolve user display names.
	allUsers, _ := c.user.List(r.Context(), store.UserFilter{})
	userMap := make(map[string]string, len(allUsers))
	for _, u := range allUsers {
		userMap[u.ID] = u.DisplayName
	}

	views := make([]templates.AuditEntryView, 0, len(entries))
	for _, e := range entries {
		name := e.UserID
		if dn, ok := userMap[e.UserID]; ok {
			name = dn
		}
		views = append(views, templates.AuditEntryView{
			ID:        e.ID,
			UserName:  name,
			Action:    e.Action,
			FieldName: e.FieldName,
			OldValue:  e.OldValue,
			NewValue:  e.NewValue,
			CreatedAt: e.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneHistoryTabContent(views).Render(r.Context(), w)
}

// milestoneEditDate extracts the date part from an RFC 3339 string for the date input.
func milestoneEditDate(rfc3339 string) string {
	if len(rfc3339) >= 10 {
		return rfc3339[:10]
	}
	return rfc3339
}
