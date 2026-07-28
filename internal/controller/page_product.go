package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
	"nexmap.net/web/templates/patterns"
)

// PageProductController handles product pages.
type PageProductController struct {
	product    *service.ProductService
	dashboard  *service.DashboardService
	settings   *service.SettingService
	user       *service.UserService
	invitation *service.InvitationService
	milestone  *service.MilestoneService
	component  *service.ComponentService
	testing    *service.TestingService
	backlog    *service.BacklogService
	category   *service.CategoryService
	constraint *service.ConstraintService
	discussion *service.DiscussionService
	audit      *service.AuditService

	// maxUploadBytes mirrors the router's cap so the upload form can state the
	// limit and rejections can name it.
	maxUploadBytes int64
}

// NewPageProduct creates a PageProductController.
func NewPageProduct(product *service.ProductService, dashboard *service.DashboardService, settings *service.SettingService, user *service.UserService, invitation *service.InvitationService, milestone *service.MilestoneService, component *service.ComponentService, testing *service.TestingService, backlog *service.BacklogService, category *service.CategoryService, constraint *service.ConstraintService, discussion *service.DiscussionService, audit *service.AuditService, maxUploadBytes int64) *PageProductController {
	return &PageProductController{product: product, dashboard: dashboard, settings: settings, user: user, invitation: invitation, milestone: milestone, component: component, testing: testing, backlog: backlog, category: category, constraint: constraint, discussion: discussion, audit: audit, maxUploadBytes: maxUploadBytes}
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

// MoveStatus handles POST /products/{id}/settings/statuses/{statusId}/move.
// Swaps the status with its neighbor based on form field `direction` ("up" or "down").
func (c *PageProductController) MoveStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	statusID := r.PathValue("statusId")
	direction := r.FormValue("direction")

	status, err := c.milestone.GetStatus(r.Context(), user, productID, statusID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	statuses, err := c.milestone.ListStatuses(r.Context(), user, productID, status.EntityType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	idx := -1
	for i, s := range statuses {
		if s.ID == statusID {
			idx = i
			break
		}
	}

	neighborIdx := -1
	switch direction {
	case "up":
		if idx > 0 {
			neighborIdx = idx - 1
		}
	case "down":
		if idx >= 0 && idx < len(statuses)-1 {
			neighborIdx = idx + 1
		}
	}

	if neighborIdx >= 0 {
		target := statuses[idx]
		neighbor := statuses[neighborIdx]
		// Swap positions via two UpdateStatus calls. The service mirrors each
		// change to the sibling entity-type status when StatusSync is true.
		if _, err := c.milestone.UpdateStatus(r.Context(), user, productID, target.ID, service.UpdateStatusInput{Position: &neighbor.Position}); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if _, err := c.milestone.UpdateStatus(r.Context(), user, productID, neighbor.ID, service.UpdateStatusInput{Position: &target.Position}); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
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

	// When workflow mode changes, send an OOB swap to update the sidebar nav item.
	if field == "workflow_mode" {
		_ = patterns.SidebarWorkflowItemOOB(updated.ID, updated.WorkflowMode).Render(r.Context(), w)
	}
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
	_ = productpage.DeleteProductConfirmation(p.Name, productID, middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// --- helpers ---

// editReturnTarget reads the edit-flow return target from a GET request's
// query string. Empty string means the default (redirect to detail view).
func editReturnTarget(r *http.Request) string {
	return r.URL.Query().Get("return")
}

// postEditRedirect chooses where to send the user after a successful edit.
// If the form carried `return=list`, it sends them to listPath; otherwise
// detailPath (the detail view).
func postEditRedirect(r *http.Request, detailPath, listPath string) string {
	if r.FormValue("return") == "list" {
		return listPath
	}
	return detailPath
}

func (c *PageProductController) canCreateProduct(r *http.Request, user *model.User) bool {
	if user.IsAdmin {
		return true
	}
	val, _ := c.settings.Get(r.Context(), "product_creation_enabled")
	return val == "true"
}

// writeTabOOB writes an out-of-band swap of a tab panel. The given content
// becomes the panel's new innerHTML; the primary swap target (typically
// #modal) receives an empty body, which closes the modal.
func (c *PageProductController) writeTabOOB(w http.ResponseWriter, r *http.Request, panelID string, content templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div id="tab-panel-%s" hx-swap-oob="innerHTML">`, panelID)
	_ = content.Render(r.Context(), w)
	fmt.Fprint(w, `</div>`)
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
		ReturnTo:    editReturnTarget(r),
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
			ReturnTo:    r.FormValue("return"),
			Name:        name,
			Description: description,
			Type:        milestoneType,
			DueDate:     dueDate,
			Error:       err.Error(),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		fmt.Sprintf("/products/%s/milestones/%s", productID, milestoneID),
		fmt.Sprintf("/products/%s/milestones", productID),
	), http.StatusSeeOther)
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
	_ = productpage.MilestoneDeleteConfirmation(m.Name, productID, milestoneID, middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
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

// --- Milestone link tabs (FR-013, FR-025, FR-029) ---
//
// A milestone link is a nullable FK on the child entity, so linking is an
// update of that column and unlinking clears it. Every write path requires
// owner role, so one predicate gates all six tabs.

func (c *PageProductController) canEditMilestoneLinks(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner
}

func (c *PageProductController) canEditMilestoneLinksFor(r *http.Request, user *model.User, productID string) bool {
	if user.IsAdmin {
		return true
	}
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return false
	}
	return c.canEditMilestoneLinks(user, role)
}

// MilestoneComponentsTab renders GET /products/{id}/milestones/{milestoneId}/tabs/components.
func (c *PageProductController) MilestoneComponentsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	props, err := c.loadMilestoneComponentsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneComponentsTabContent(props).Render(r.Context(), w)
}

// MilestoneComponentLinkModal renders GET /products/{id}/milestones/{milestoneId}/components/link-modal.
func (c *PageProductController) MilestoneComponentLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneComponentLinkModal(productID, milestoneID).Render(r.Context(), w)
}

// MilestoneComponentLinkSearch renders GET /products/{id}/milestones/{milestoneId}/components/link-search.
// Candidates already on another milestone are offered too, labelled with it.
func (c *PageProductController) MilestoneComponentLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.component.ListComponents(r.Context(), user, productID)

	candidates := make([]*model.Component, 0, len(all))
	for _, comp := range all {
		if comp.MilestoneID == milestoneID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(comp.Name), query) {
			continue
		}
		candidates = append(candidates, comp)
	}

	milestoneNames := c.buildMilestoneMap(r, user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneComponentLinkResults(productID, milestoneID, candidates, milestoneNames).Render(r.Context(), w)
}

// MilestoneComponentLink handles POST /products/{id}/milestones/{milestoneId}/components/link.
func (c *PageProductController) MilestoneComponentLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.FormValue("component_id")

	if targetID == "" {
		http.Error(w, "component_id is required", http.StatusBadRequest)
		return
	}
	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mid := milestoneID
	if _, err := c.component.UpdateComponent(r.Context(), user, productID, targetID, service.UpdateComponentInput{
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeMilestoneComponentsTabOOB(w, r, user, productID, milestoneID)
}

// MilestoneComponentUnlink handles DELETE /products/{id}/milestones/{milestoneId}/components/{componentId}.
// The path parameter is the component's own ID; unlinking clears its milestone FK.
func (c *PageProductController) MilestoneComponentUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.PathValue("componentId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	empty := ""
	if _, err := c.component.UpdateComponent(r.Context(), user, productID, targetID, service.UpdateComponentInput{
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadMilestoneComponentsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneComponentsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadMilestoneComponentsTabProps(r *http.Request, user *model.User, productID, milestoneID string) (productpage.MilestoneComponentsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.MilestoneComponentsTabProps{}, err
	}

	items, _ := c.component.ListComponentsByMilestone(r.Context(), user, productID, milestoneID)

	return productpage.MilestoneComponentsTabProps{
		ProductID:   productID,
		MilestoneID: milestoneID,
		CSRFToken:   csrfToken,
		CanWrite:    c.canEditMilestoneLinks(user, role),
		Components:  items,
	}, nil
}

func (c *PageProductController) writeMilestoneComponentsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, milestoneID string) {
	props, err := c.loadMilestoneComponentsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "components", productpage.MilestoneComponentsTabContent(props))
}

// MilestoneCapabilitiesTab renders GET /products/{id}/milestones/{milestoneId}/tabs/capabilities.
func (c *PageProductController) MilestoneCapabilitiesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	props, err := c.loadMilestoneCapabilitiesTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneCapabilitiesTabContent(props).Render(r.Context(), w)
}

// MilestoneCapabilityLinkModal renders GET /products/{id}/milestones/{milestoneId}/capabilities/link-modal.
func (c *PageProductController) MilestoneCapabilityLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneCapabilityLinkModal(productID, milestoneID).Render(r.Context(), w)
}

// MilestoneCapabilityLinkSearch renders GET /products/{id}/milestones/{milestoneId}/capabilities/link-search.
// Candidates already on another milestone are offered too, labelled with it.
func (c *PageProductController) MilestoneCapabilityLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.component.ListCapabilities(r.Context(), user, productID)

	candidates := make([]*model.Capability, 0, len(all))
	for _, cap := range all {
		if cap.MilestoneID == milestoneID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(cap.Name), query) {
			continue
		}
		candidates = append(candidates, cap)
	}

	milestoneNames := c.buildMilestoneMap(r, user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneCapabilityLinkResults(productID, milestoneID, candidates, milestoneNames).Render(r.Context(), w)
}

// MilestoneCapabilityLink handles POST /products/{id}/milestones/{milestoneId}/capabilities/link.
func (c *PageProductController) MilestoneCapabilityLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.FormValue("capability_id")

	if targetID == "" {
		http.Error(w, "capability_id is required", http.StatusBadRequest)
		return
	}
	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mid := milestoneID
	if _, err := c.component.UpdateCapability(r.Context(), user, productID, targetID, service.UpdateCapabilityInput{
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeMilestoneCapabilitiesTabOOB(w, r, user, productID, milestoneID)
}

// MilestoneCapabilityUnlink handles DELETE /products/{id}/milestones/{milestoneId}/capabilities/{capabilityId}.
// The path parameter is the capability's own ID; unlinking clears its milestone FK.
func (c *PageProductController) MilestoneCapabilityUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.PathValue("capabilityId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	empty := ""
	if _, err := c.component.UpdateCapability(r.Context(), user, productID, targetID, service.UpdateCapabilityInput{
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadMilestoneCapabilitiesTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneCapabilitiesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadMilestoneCapabilitiesTabProps(r *http.Request, user *model.User, productID, milestoneID string) (productpage.MilestoneCapabilitiesTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.MilestoneCapabilitiesTabProps{}, err
	}

	items, _ := c.component.ListCapabilitiesByMilestone(r.Context(), user, productID, milestoneID)

	return productpage.MilestoneCapabilitiesTabProps{
		ProductID:    productID,
		MilestoneID:  milestoneID,
		CSRFToken:    csrfToken,
		CanWrite:     c.canEditMilestoneLinks(user, role),
		Capabilities: items,
	}, nil
}

func (c *PageProductController) writeMilestoneCapabilitiesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, milestoneID string) {
	props, err := c.loadMilestoneCapabilitiesTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "capabilities", productpage.MilestoneCapabilitiesTabContent(props))
}

// MilestoneRequirementsTab renders GET /products/{id}/milestones/{milestoneId}/tabs/requirements.
func (c *PageProductController) MilestoneRequirementsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	props, err := c.loadMilestoneRequirementsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneRequirementsTabContent(props).Render(r.Context(), w)
}

// MilestoneRequirementLinkModal renders GET /products/{id}/milestones/{milestoneId}/requirements/link-modal.
func (c *PageProductController) MilestoneRequirementLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneRequirementLinkModal(productID, milestoneID).Render(r.Context(), w)
}

// MilestoneRequirementLinkSearch renders GET /products/{id}/milestones/{milestoneId}/requirements/link-search.
// Candidates already on another milestone are offered too, labelled with it.
func (c *PageProductController) MilestoneRequirementLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)

	candidates := make([]*model.Requirement, 0, len(all))
	for _, req := range all {
		if req.MilestoneID == milestoneID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(req.Title), query) {
			continue
		}
		candidates = append(candidates, req)
	}

	milestoneNames := c.buildMilestoneMap(r, user, productID)
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneRequirementLinkResults(productID, milestoneID, candidates, milestoneNames, prefixes).Render(r.Context(), w)
}

// MilestoneRequirementLink handles POST /products/{id}/milestones/{milestoneId}/requirements/link.
func (c *PageProductController) MilestoneRequirementLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.FormValue("requirement_id")

	if targetID == "" {
		http.Error(w, "requirement_id is required", http.StatusBadRequest)
		return
	}
	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mid := milestoneID
	if _, err := c.backlog.UpdateRequirement(r.Context(), user, productID, targetID, service.UpdateRequirementInput{
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeMilestoneRequirementsTabOOB(w, r, user, productID, milestoneID)
}

// MilestoneRequirementUnlink handles DELETE /products/{id}/milestones/{milestoneId}/requirements/{requirementId}.
// The path parameter is the requirement's own ID; unlinking clears its milestone FK.
func (c *PageProductController) MilestoneRequirementUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.PathValue("requirementId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	empty := ""
	if _, err := c.backlog.UpdateRequirement(r.Context(), user, productID, targetID, service.UpdateRequirementInput{
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadMilestoneRequirementsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneRequirementsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadMilestoneRequirementsTabProps(r *http.Request, user *model.User, productID, milestoneID string) (productpage.MilestoneRequirementsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.MilestoneRequirementsTabProps{}, err
	}

	items, _ := c.backlog.ListRequirementsByMilestone(r.Context(), user, productID, milestoneID)
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	return productpage.MilestoneRequirementsTabProps{
		ProductID:        productID,
		MilestoneID:      milestoneID,
		CSRFToken:        csrfToken,
		CanWrite:         c.canEditMilestoneLinks(user, role),
		Requirements:     items,
		CategoryPrefixes: prefixes,
	}, nil
}

func (c *PageProductController) writeMilestoneRequirementsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, milestoneID string) {
	props, err := c.loadMilestoneRequirementsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "requirements", productpage.MilestoneRequirementsTabContent(props))
}

// MilestoneConstraintsTab renders GET /products/{id}/milestones/{milestoneId}/tabs/constraints.
func (c *PageProductController) MilestoneConstraintsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	props, err := c.loadMilestoneConstraintsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneConstraintsTabContent(props).Render(r.Context(), w)
}

// MilestoneConstraintLinkModal renders GET /products/{id}/milestones/{milestoneId}/constraints/link-modal.
func (c *PageProductController) MilestoneConstraintLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneConstraintLinkModal(productID, milestoneID).Render(r.Context(), w)
}

// MilestoneConstraintLinkSearch renders GET /products/{id}/milestones/{milestoneId}/constraints/link-search.
// Candidates already on another milestone are offered too, labelled with it.
func (c *PageProductController) MilestoneConstraintLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.constraint.ListConstraints(r.Context(), user, productID, store.ConstraintFilter{})

	candidates := make([]*model.Constraint, 0, len(all))
	for _, con := range all {
		if con.MilestoneID == milestoneID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(con.Title), query) {
			continue
		}
		candidates = append(candidates, con)
	}

	milestoneNames := c.buildMilestoneMap(r, user, productID)
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "constraint")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneConstraintLinkResults(productID, milestoneID, candidates, milestoneNames, prefixes).Render(r.Context(), w)
}

// MilestoneConstraintLink handles POST /products/{id}/milestones/{milestoneId}/constraints/link.
func (c *PageProductController) MilestoneConstraintLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.FormValue("constraint_id")

	if targetID == "" {
		http.Error(w, "constraint_id is required", http.StatusBadRequest)
		return
	}
	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mid := milestoneID
	if _, err := c.constraint.UpdateConstraint(r.Context(), user, productID, targetID, service.UpdateConstraintInput{
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeMilestoneConstraintsTabOOB(w, r, user, productID, milestoneID)
}

// MilestoneConstraintUnlink handles DELETE /products/{id}/milestones/{milestoneId}/constraints/{constraintId}.
// The path parameter is the constraint's own ID; unlinking clears its milestone FK.
func (c *PageProductController) MilestoneConstraintUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.PathValue("constraintId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	empty := ""
	if _, err := c.constraint.UpdateConstraint(r.Context(), user, productID, targetID, service.UpdateConstraintInput{
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadMilestoneConstraintsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneConstraintsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadMilestoneConstraintsTabProps(r *http.Request, user *model.User, productID, milestoneID string) (productpage.MilestoneConstraintsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.MilestoneConstraintsTabProps{}, err
	}

	items, _ := c.constraint.ListConstraintsByMilestone(r.Context(), user, productID, milestoneID)
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "constraint")

	return productpage.MilestoneConstraintsTabProps{
		ProductID:        productID,
		MilestoneID:      milestoneID,
		CSRFToken:        csrfToken,
		CanWrite:         c.canEditMilestoneLinks(user, role),
		Constraints:      items,
		CategoryPrefixes: prefixes,
	}, nil
}

func (c *PageProductController) writeMilestoneConstraintsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, milestoneID string) {
	props, err := c.loadMilestoneConstraintsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "constraints", productpage.MilestoneConstraintsTabContent(props))
}

// MilestoneDefectsTab renders GET /products/{id}/milestones/{milestoneId}/tabs/defects.
func (c *PageProductController) MilestoneDefectsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	props, err := c.loadMilestoneDefectsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneDefectsTabContent(props).Render(r.Context(), w)
}

// MilestoneDefectLinkModal renders GET /products/{id}/milestones/{milestoneId}/defects/link-modal.
func (c *PageProductController) MilestoneDefectLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneDefectLinkModal(productID, milestoneID).Render(r.Context(), w)
}

// MilestoneDefectLinkSearch renders GET /products/{id}/milestones/{milestoneId}/defects/link-search.
// Candidates already on another milestone are offered too, labelled with it.
func (c *PageProductController) MilestoneDefectLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{})

	candidates := make([]*model.Defect, 0, len(all))
	for _, d := range all {
		if d.MilestoneID == milestoneID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(d.Title), query) {
			continue
		}
		candidates = append(candidates, d)
	}

	milestoneNames := c.buildMilestoneMap(r, user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneDefectLinkResults(productID, milestoneID, candidates, milestoneNames).Render(r.Context(), w)
}

// MilestoneDefectLink handles POST /products/{id}/milestones/{milestoneId}/defects/link.
func (c *PageProductController) MilestoneDefectLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.FormValue("defect_id")

	if targetID == "" {
		http.Error(w, "defect_id is required", http.StatusBadRequest)
		return
	}
	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mid := milestoneID
	if _, err := c.backlog.UpdateDefect(r.Context(), user, productID, targetID, service.UpdateDefectInput{
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeMilestoneDefectsTabOOB(w, r, user, productID, milestoneID)
}

// MilestoneDefectUnlink handles DELETE /products/{id}/milestones/{milestoneId}/defects/{defectId}.
// The path parameter is the defect's own ID; unlinking clears its milestone FK.
func (c *PageProductController) MilestoneDefectUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.PathValue("defectId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	empty := ""
	if _, err := c.backlog.UpdateDefect(r.Context(), user, productID, targetID, service.UpdateDefectInput{
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadMilestoneDefectsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneDefectsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadMilestoneDefectsTabProps(r *http.Request, user *model.User, productID, milestoneID string) (productpage.MilestoneDefectsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.MilestoneDefectsTabProps{}, err
	}

	items, _ := c.backlog.ListDefectsByMilestone(r.Context(), user, productID, milestoneID)

	return productpage.MilestoneDefectsTabProps{
		ProductID:   productID,
		MilestoneID: milestoneID,
		CSRFToken:   csrfToken,
		CanWrite:    c.canEditMilestoneLinks(user, role),
		Defects:     items,
	}, nil
}

func (c *PageProductController) writeMilestoneDefectsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, milestoneID string) {
	props, err := c.loadMilestoneDefectsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "defects", productpage.MilestoneDefectsTabContent(props))
}

// MilestoneSprintsTab renders GET /products/{id}/milestones/{milestoneId}/tabs/sprints.
func (c *PageProductController) MilestoneSprintsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	props, err := c.loadMilestoneSprintsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneSprintsTabContent(props).Render(r.Context(), w)
}

// MilestoneSprintLinkModal renders GET /products/{id}/milestones/{milestoneId}/sprints/link-modal.
func (c *PageProductController) MilestoneSprintLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneSprintLinkModal(productID, milestoneID).Render(r.Context(), w)
}

// MilestoneSprintLinkSearch renders GET /products/{id}/milestones/{milestoneId}/sprints/link-search.
// Candidates already on another milestone are offered too, labelled with it.
func (c *PageProductController) MilestoneSprintLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.milestone.ListSprints(r.Context(), user, productID)

	candidates := make([]*model.Sprint, 0, len(all))
	for _, sp := range all {
		if sp.MilestoneID == milestoneID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(sp.Name), query) {
			continue
		}
		candidates = append(candidates, sp)
	}

	milestoneNames := c.buildMilestoneMap(r, user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneSprintLinkResults(productID, milestoneID, candidates, milestoneNames).Render(r.Context(), w)
}

// MilestoneSprintLink handles POST /products/{id}/milestones/{milestoneId}/sprints/link.
func (c *PageProductController) MilestoneSprintLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.FormValue("sprint_id")

	if targetID == "" {
		http.Error(w, "sprint_id is required", http.StatusBadRequest)
		return
	}
	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mid := milestoneID
	if _, err := c.milestone.UpdateSprint(r.Context(), user, productID, targetID, service.UpdateSprintInput{
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeMilestoneSprintsTabOOB(w, r, user, productID, milestoneID)
}

// MilestoneSprintUnlink handles DELETE /products/{id}/milestones/{milestoneId}/sprints/{sprintId}.
// The path parameter is the sprint's own ID; unlinking clears its milestone FK.
func (c *PageProductController) MilestoneSprintUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")
	targetID := r.PathValue("sprintId")

	if !c.canEditMilestoneLinksFor(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	empty := ""
	if _, err := c.milestone.UpdateSprint(r.Context(), user, productID, targetID, service.UpdateSprintInput{
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadMilestoneSprintsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.MilestoneSprintsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadMilestoneSprintsTabProps(r *http.Request, user *model.User, productID, milestoneID string) (productpage.MilestoneSprintsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.MilestoneSprintsTabProps{}, err
	}

	items, _ := c.milestone.ListSprintsByMilestone(r.Context(), user, productID, milestoneID)

	return productpage.MilestoneSprintsTabProps{
		ProductID:   productID,
		MilestoneID: milestoneID,
		CSRFToken:   csrfToken,
		CanWrite:    c.canEditMilestoneLinks(user, role),
		Sprints:     items,
	}, nil
}

func (c *PageProductController) writeMilestoneSprintsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, milestoneID string) {
	props, err := c.loadMilestoneSprintsTabProps(r, user, productID, milestoneID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "sprints", productpage.MilestoneSprintsTabContent(props))
}

// MilestoneHistoryTab renders GET /products/{id}/milestones/{milestoneId}/tabs/history.
func (c *PageProductController) MilestoneHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	milestoneID := r.PathValue("milestoneId")

	views := c.buildAuditViews(r, user, productID, "milestone", milestoneID)

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

// --- Components ---

// Components renders GET /products/{id}/components.
func (c *PageProductController) Components(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	components, err := c.component.ListComponents(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}

	milestoneMap := c.buildMilestoneMap(r, user, productID)
	isOwnerOrAdmin := user.IsAdmin || role == model.RoleOwner

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentsPage(productpage.ComponentsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Components",
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Components:     components,
		Milestones:     milestoneMap,
		IsOwnerOrAdmin: isOwnerOrAdmin,
	}).Render(r.Context(), w)
}

// ComponentDetail renders GET /products/{id}/components/{componentId}.
func (c *PageProductController) ComponentDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	comp, err := c.component.GetComponent(r.Context(), user, productID, componentID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/components", http.StatusSeeOther)
		return
	}

	milestoneName := c.resolveMilestoneName(r, user, productID, comp.MilestoneID)
	isOwnerOrAdmin := user.IsAdmin || role == model.RoleOwner

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentDetailPage(productpage.ComponentDetailProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - " + comp.Name,
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Component:      comp,
		MilestoneName:  milestoneName,
		IsOwnerOrAdmin: isOwnerOrAdmin,
	}).Render(r.Context(), w)
}

// ComponentCreateForm renders GET /products/{id}/components/new.
func (c *PageProductController) ComponentCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !user.IsAdmin && role != model.RoleOwner {
		http.Redirect(w, r, "/products/"+productID+"/components", http.StatusSeeOther)
		return
	}

	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentFormPage(productpage.ComponentFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - New Component",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Milestones:  milestones,
	}).Render(r.Context(), w)
}

// ComponentCreate handles POST /products/{id}/components.
func (c *PageProductController) ComponentCreate(w http.ResponseWriter, r *http.Request) {
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
	milestoneID := r.FormValue("milestone_id")

	_, err = c.component.CreateComponent(r.Context(), user, service.CreateComponentInput{
		ProductID:   productID,
		Name:        name,
		Description: description,
		MilestoneID: milestoneID,
	})
	if err != nil {
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.ComponentFormPage(productpage.ComponentFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - New Component",
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Milestones:  milestones,
			Name:        name,
			Description: description,
			MilestoneID: milestoneID,
			Error:       err.Error(),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/components", http.StatusSeeOther)
}

// ComponentEditForm renders GET /products/{id}/components/{componentId}/edit.
func (c *PageProductController) ComponentEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !user.IsAdmin && role != model.RoleOwner {
		http.Redirect(w, r, fmt.Sprintf("/products/%s/components/%s", productID, componentID), http.StatusSeeOther)
		return
	}

	comp, err := c.component.GetComponent(r.Context(), user, productID, componentID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/components", http.StatusSeeOther)
		return
	}

	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentFormPage(productpage.ComponentFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Edit " + comp.Name,
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Component:   comp,
		Milestones:  milestones,
		IsEdit:      true,
		ReturnTo:    editReturnTarget(r),
		Name:        comp.Name,
		Description: comp.Description,
		MilestoneID: comp.MilestoneID,
	}).Render(r.Context(), w)
}

// ComponentUpdate handles POST /products/{id}/components/{componentId}.
func (c *PageProductController) ComponentUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	comp, err := c.component.GetComponent(r.Context(), user, productID, componentID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/components", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	milestoneID := r.FormValue("milestone_id")

	input := service.UpdateComponentInput{
		Name:        &name,
		Description: &description,
		MilestoneID: &milestoneID,
	}

	_, err = c.component.UpdateComponent(r.Context(), user, productID, componentID, input)
	if err != nil {
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.ComponentFormPage(productpage.ComponentFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - Edit " + comp.Name,
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Component:   comp,
			Milestones:  milestones,
			IsEdit:      true,
			ReturnTo:    r.FormValue("return"),
			Name:        name,
			Description: description,
			MilestoneID: milestoneID,
			Error:       err.Error(),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		fmt.Sprintf("/products/%s/components/%s", productID, componentID),
		fmt.Sprintf("/products/%s/components", productID),
	), http.StatusSeeOther)
}

// ComponentDelete handles DELETE /products/{id}/components/{componentId}.
func (c *PageProductController) ComponentDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	if err := c.component.DeleteComponent(r.Context(), user, productID, componentID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("HX-Redirect", "/products/"+productID+"/components")
	w.WriteHeader(http.StatusOK)
}

// ComponentDeleteConfirm renders GET /products/{id}/components/{componentId}/delete/confirm.
func (c *PageProductController) ComponentDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	comp, err := c.component.GetComponent(r.Context(), user, productID, componentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentDeleteConfirmation(comp.Name, productID, componentID, middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// ComponentOverviewTab renders GET /products/{id}/components/{componentId}/tabs/overview.
func (c *PageProductController) ComponentOverviewTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	comp, err := c.component.GetComponent(r.Context(), user, productID, componentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	milestoneName := c.resolveMilestoneName(r, user, productID, comp.MilestoneID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentOverviewTabContent(comp, milestoneName).Render(r.Context(), w)
}

// --- Component → Capability (FK 1:N, re-parent semantics) ---

func (c *PageProductController) ComponentCapabilitiesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	props, err := c.loadComponentCapabilitiesTabProps(r, user, productID, componentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentCapabilitiesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) ComponentCapabilityLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentCapabilityLinkModal(productID, componentID).Render(r.Context(), w)
}

func (c *PageProductController) ComponentCapabilityLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.component.ListCapabilities(r.Context(), user, productID)

	candidates := make([]*model.Capability, 0, len(all))
	for _, cap := range all {
		if cap.ComponentID == componentID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(cap.Name), query) {
			continue
		}
		candidates = append(candidates, cap)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentCapabilityLinkResults(productID, componentID, candidates).Render(r.Context(), w)
}

func (c *PageProductController) ComponentCapabilityLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")
	capabilityID := r.FormValue("capability_id")

	if capabilityID == "" {
		http.Error(w, "capability_id is required", http.StatusBadRequest)
		return
	}

	newCompID := componentID
	if _, err := c.component.UpdateCapability(r.Context(), user, productID, capabilityID, service.UpdateCapabilityInput{
		ComponentID: &newCompID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeComponentCapabilitiesTabOOB(w, r, user, productID, componentID)
}

func (c *PageProductController) ComponentCapabilityCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityFormModal(productpage.CapabilityFormProps{
		Meta:        templates.PageMeta{CSRFToken: csrfToken},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Components:  components,
		Milestones:  milestones,
		LinkTo:      "component:" + componentID,
		ComponentID: componentID,
	}).Render(r.Context(), w)
}

// ComponentCapabilityCreateAndLink handles POST /products/{id}/components/{componentId}/capabilities.
// FK semantics: just create the capability with ComponentID set; no separate link step.
func (c *PageProductController) ComponentCapabilityCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	milestoneID := r.FormValue("milestone_id")

	rerenderForm := func(errMsg string) {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.CapabilityFormModal(productpage.CapabilityFormProps{
			Meta:        templates.PageMeta{CSRFToken: csrfToken},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Components:  components,
			Milestones:  milestones,
			LinkTo:      "component:" + componentID,
			Name:        name,
			Description: description,
			ComponentID: componentID,
			MilestoneID: milestoneID,
			Error:       errMsg,
		}).Render(r.Context(), w)
	}

	_, err = c.component.CreateCapability(r.Context(), user, service.CreateCapabilityInput{
		ProductID:   productID,
		ComponentID: componentID,
		Name:        name,
		Description: description,
		MilestoneID: milestoneID,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	c.writeComponentCapabilitiesTabOOB(w, r, user, productID, componentID)
}

// ComponentCapabilityUnlink clears the ComponentID FK on the given capability.
func (c *PageProductController) ComponentCapabilityUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")
	capabilityID := r.PathValue("capabilityId")

	empty := ""
	if _, err := c.component.UpdateCapability(r.Context(), user, productID, capabilityID, service.UpdateCapabilityInput{
		ComponentID: &empty,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadComponentCapabilitiesTabProps(r, user, productID, componentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentCapabilitiesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadComponentCapabilitiesTabProps(r *http.Request, user *model.User, productID, componentID string) (productpage.ComponentCapabilitiesTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.ComponentCapabilitiesTabProps{}, err
	}

	capabilities, _ := c.component.ListCapabilitiesByComponent(r.Context(), user, productID, componentID)

	return productpage.ComponentCapabilitiesTabProps{
		ProductID:    productID,
		ComponentID:  componentID,
		CSRFToken:    csrfToken,
		CanWrite:     user.IsAdmin || role == model.RoleOwner,
		Capabilities: capabilities,
	}, nil
}

func (c *PageProductController) writeComponentCapabilitiesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, componentID string) {
	props, err := c.loadComponentCapabilitiesTabProps(r, user, productID, componentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "capabilities", productpage.ComponentCapabilitiesTabContent(props))
}

// --- Component → TestCase (FK 1:N, re-parent semantics) ---

func (c *PageProductController) ComponentTestCasesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	props, err := c.loadComponentTestCasesTabProps(r, user, productID, componentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentTestCasesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) ComponentTestCaseLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentTestCaseLinkModal(productID, componentID).Render(r.Context(), w)
}

func (c *PageProductController) ComponentTestCaseLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.testing.ListTestCases(r.Context(), user, productID)

	candidates := make([]*model.TestCase, 0, len(all))
	for _, tc := range all {
		if tc.ComponentID == componentID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(tc.Title), query) {
			continue
		}
		candidates = append(candidates, tc)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentTestCaseLinkResults(productID, componentID, candidates).Render(r.Context(), w)
}

func (c *PageProductController) ComponentTestCaseLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")
	testCaseID := r.FormValue("test_case_id")

	if testCaseID == "" {
		http.Error(w, "test_case_id is required", http.StatusBadRequest)
		return
	}

	newCompID := componentID
	if _, err := c.testing.UpdateTestCase(r.Context(), user, productID, testCaseID, service.UpdateTestCaseInput{
		ComponentID: &newCompID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeComponentTestCasesTabOOB(w, r, user, productID, componentID)
}

func (c *PageProductController) ComponentTestCaseCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
	pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseFormModal(productpage.TestCaseFormProps{
		Meta:          templates.PageMeta{CSRFToken: csrfToken},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		Components:    components,
		Capabilities:  capabilities,
		PyramidLevels: pyramidLevels,
		LinkTo:        "component:" + componentID,
		ComponentID:   componentID,
	}).Render(r.Context(), w)
}

// ComponentTestCaseCreateAndLink handles POST /products/{id}/components/{componentId}/test-cases.
// FK semantics: just create the test case with ComponentID set; no separate link step.
func (c *PageProductController) ComponentTestCaseCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	executionMode := r.FormValue("execution_mode")
	testIdentifier := r.FormValue("test_identifier")
	capabilityID := r.FormValue("capability_id")
	pyramidLevelID := r.FormValue("pyramid_level_id")

	rerenderForm := func(errMsg string) {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
		pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.TestCaseFormModal(productpage.TestCaseFormProps{
			Meta:           templates.PageMeta{CSRFToken: csrfToken},
			DisplayName:    user.DisplayName,
			IsAdmin:        user.IsAdmin,
			Product:        p,
			Components:     components,
			Capabilities:   capabilities,
			PyramidLevels:  pyramidLevels,
			LinkTo:         "component:" + componentID,
			Title:          title,
			Description:    description,
			ExecutionMode:  executionMode,
			TestIdentifier: testIdentifier,
			ComponentID:    componentID,
			CapabilityID:   capabilityID,
			PyramidLevelID: pyramidLevelID,
			Error:          errMsg,
		}).Render(r.Context(), w)
	}

	_, err = c.testing.CreateTestCase(r.Context(), user, service.CreateTestCaseInput{
		ProductID:      productID,
		Title:          title,
		Description:    description,
		ExecutionMode:  executionMode,
		TestIdentifier: testIdentifier,
		ComponentID:    componentID,
		CapabilityID:   capabilityID,
		PyramidLevelID: pyramidLevelID,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	c.writeComponentTestCasesTabOOB(w, r, user, productID, componentID)
}

// ComponentTestCaseUnlink clears the ComponentID FK on the given test case.
func (c *PageProductController) ComponentTestCaseUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")
	testCaseID := r.PathValue("testCaseId")

	empty := ""
	if _, err := c.testing.UpdateTestCase(r.Context(), user, productID, testCaseID, service.UpdateTestCaseInput{
		ComponentID: &empty,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadComponentTestCasesTabProps(r, user, productID, componentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentTestCasesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadComponentTestCasesTabProps(r *http.Request, user *model.User, productID, componentID string) (productpage.ComponentTestCasesTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.ComponentTestCasesTabProps{}, err
	}

	testCases, _ := c.testing.ListTestCasesByComponent(r.Context(), user, productID, componentID)

	return productpage.ComponentTestCasesTabProps{
		ProductID:   productID,
		ComponentID: componentID,
		CSRFToken:   csrfToken,
		CanWrite:    c.canLinkTestCasesForRole(user, role),
		TestCases:   testCases,
	}, nil
}

func (c *PageProductController) writeComponentTestCasesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, componentID string) {
	props, err := c.loadComponentTestCasesTabProps(r, user, productID, componentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "test-cases", productpage.ComponentTestCasesTabContent(props))
}

// ComponentHistoryTab renders GET /products/{id}/components/{componentId}/tabs/history.
func (c *PageProductController) ComponentHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	componentID := r.PathValue("componentId")

	views := c.buildAuditViews(r, user, productID, "component", componentID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ComponentHistoryTabContent(views).Render(r.Context(), w)
}

// --- Capabilities ---

// Capabilities renders GET /products/{id}/capabilities.
func (c *PageProductController) Capabilities(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	funcs, err := c.component.ListCapabilities(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}

	componentMap := c.buildComponentMap(r, user, productID)
	milestoneMap := c.buildMilestoneMap(r, user, productID)
	isOwnerOrAdmin := user.IsAdmin || role == model.RoleOwner

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilitiesPage(productpage.CapabilitiesProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Capabilities",
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Capabilities:   funcs,
		Components:     componentMap,
		Milestones:     milestoneMap,
		IsOwnerOrAdmin: isOwnerOrAdmin,
	}).Render(r.Context(), w)
}

// CapabilityDetail renders GET /products/{id}/capabilities/{capabilityId}.
func (c *PageProductController) CapabilityDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	f, err := c.component.GetCapability(r.Context(), user, productID, capabilityID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/capabilities", http.StatusSeeOther)
		return
	}

	componentName := c.resolveComponentName(r, user, productID, f.ComponentID)
	milestoneName := c.resolveMilestoneName(r, user, productID, f.MilestoneID)
	isOwnerOrAdmin := user.IsAdmin || role == model.RoleOwner

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityDetailPage(productpage.CapabilityDetailProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - " + f.Name,
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Capability:     f,
		ComponentName:  componentName,
		MilestoneName:  milestoneName,
		IsOwnerOrAdmin: isOwnerOrAdmin,
	}).Render(r.Context(), w)
}

// CapabilityCreateForm renders GET /products/{id}/capabilities/new.
func (c *PageProductController) CapabilityCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !user.IsAdmin && role != model.RoleOwner {
		http.Redirect(w, r, "/products/"+productID+"/capabilities", http.StatusSeeOther)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityFormPage(productpage.CapabilityFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - New Capability",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Components:  components,
		Milestones:  milestones,
	}).Render(r.Context(), w)
}

// CapabilityCreate handles POST /products/{id}/capabilities.
func (c *PageProductController) CapabilityCreate(w http.ResponseWriter, r *http.Request) {
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
	componentID := r.FormValue("component_id")
	milestoneID := r.FormValue("milestone_id")

	_, err = c.component.CreateCapability(r.Context(), user, service.CreateCapabilityInput{
		ProductID:   productID,
		ComponentID: componentID,
		Name:        name,
		Description: description,
		MilestoneID: milestoneID,
	})
	if err != nil {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.CapabilityFormPage(productpage.CapabilityFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - New Capability",
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Components:  components,
			Milestones:  milestones,
			Name:        name,
			Description: description,
			ComponentID: componentID,
			MilestoneID: milestoneID,
			Error:       err.Error(),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/capabilities", http.StatusSeeOther)
}

// CapabilityEditForm renders GET /products/{id}/capabilities/{capabilityId}/edit.
func (c *PageProductController) CapabilityEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !user.IsAdmin && role != model.RoleOwner {
		http.Redirect(w, r, fmt.Sprintf("/products/%s/capabilities/%s", productID, capabilityID), http.StatusSeeOther)
		return
	}

	f, err := c.component.GetCapability(r.Context(), user, productID, capabilityID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/capabilities", http.StatusSeeOther)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityFormPage(productpage.CapabilityFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Edit " + f.Name,
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Capability:  f,
		Components:  components,
		Milestones:  milestones,
		IsEdit:      true,
		ReturnTo:    editReturnTarget(r),
		Name:        f.Name,
		Description: f.Description,
		ComponentID: f.ComponentID,
		MilestoneID: f.MilestoneID,
	}).Render(r.Context(), w)
}

// CapabilityUpdate handles POST /products/{id}/capabilities/{capabilityId}.
func (c *PageProductController) CapabilityUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	f, err := c.component.GetCapability(r.Context(), user, productID, capabilityID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/capabilities", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	componentID := r.FormValue("component_id")
	milestoneID := r.FormValue("milestone_id")

	input := service.UpdateCapabilityInput{
		Name:        &name,
		Description: &description,
		ComponentID: &componentID,
		MilestoneID: &milestoneID,
	}

	_, err = c.component.UpdateCapability(r.Context(), user, productID, capabilityID, input)
	if err != nil {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.CapabilityFormPage(productpage.CapabilityFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - Edit " + f.Name,
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Capability:  f,
			Components:  components,
			Milestones:  milestones,
			IsEdit:      true,
			ReturnTo:    r.FormValue("return"),
			Name:        name,
			Description: description,
			ComponentID: componentID,
			MilestoneID: milestoneID,
			Error:       err.Error(),
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		fmt.Sprintf("/products/%s/capabilities/%s", productID, capabilityID),
		fmt.Sprintf("/products/%s/capabilities", productID),
	), http.StatusSeeOther)
}

// CapabilityDelete handles DELETE /products/{id}/capabilities/{capabilityId}.
func (c *PageProductController) CapabilityDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	if err := c.component.DeleteCapability(r.Context(), user, productID, capabilityID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("HX-Redirect", "/products/"+productID+"/capabilities")
	w.WriteHeader(http.StatusOK)
}

// CapabilityDeleteConfirm renders GET /products/{id}/capabilities/{capabilityId}/delete/confirm.
func (c *PageProductController) CapabilityDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	f, err := c.component.GetCapability(r.Context(), user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityDeleteConfirmation(f.Name, productID, capabilityID, middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// CapabilityOverviewTab renders GET /products/{id}/capabilities/{capabilityId}/tabs/overview.
func (c *PageProductController) CapabilityOverviewTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	f, err := c.component.GetCapability(r.Context(), user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	componentName := c.resolveComponentName(r, user, productID, f.ComponentID)
	milestoneName := c.resolveMilestoneName(r, user, productID, f.MilestoneID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityOverviewTabContent(productID, f, componentName, milestoneName).Render(r.Context(), w)
}

// --- Capability → Requirement (M:N reverse) ---

func (c *PageProductController) CapabilityRequirementsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	props, err := c.loadCapabilityRequirementsTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityRequirementsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityRequirementLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityRequirementLinkModal(productID, capabilityID).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityRequirementLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
	links, _ := c.component.ListCapabilityRequirements(r.Context(), user, productID, capabilityID)
	linkedIDs := make(map[string]struct{}, len(links))
	for _, l := range links {
		linkedIDs[l.RequirementID] = struct{}{}
	}

	candidates := make([]*model.Requirement, 0, len(all))
	for _, req := range all {
		if _, ok := linkedIDs[req.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(req.Title), query) {
			continue
		}
		candidates = append(candidates, req)
	}
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityRequirementLinkResults(productID, capabilityID, candidates, prefixes).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityRequirementLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")
	requirementID := r.FormValue("requirement_id")

	if requirementID == "" {
		http.Error(w, "requirement_id is required", http.StatusBadRequest)
		return
	}

	if err := c.component.LinkRequirementToCapability(r.Context(), user, productID, capabilityID, requirementID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeCapabilityRequirementsTabOOB(w, r, user, productID, capabilityID)
}

func (c *PageProductController) CapabilityRequirementCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
	requirements, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
	categories, _ := c.category.ListCategories(r.Context(), user, productID, "requirement")
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementFormModal(productpage.RequirementFormProps{
		Meta:             templates.PageMeta{CSRFToken: csrfToken},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Requirements:     requirements,
		Categories:       categories,
		CategoryPrefixes: prefixes,
		Statuses:         statuses,
		Milestones:       milestones,
		Sprints:          sprints,
		LinkTo:           "capability:" + capabilityID,
		StatusID:         defaultStatusID(statuses),
		Priority:         "0",
	}).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityRequirementCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	description := r.FormValue("description")
	definitionOfDone := r.FormValue("definition_of_done")
	categoryID := r.FormValue("category_id")
	parentID := r.FormValue("parent_id")
	statusID := r.FormValue("status_id")
	milestoneID := r.FormValue("milestone_id")
	sprintID := r.FormValue("sprint_id")
	estimation := strings.TrimSpace(r.FormValue("estimation"))
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)
	isCoarse := r.FormValue("is_coarse") == "on"

	rerenderForm := func(errMsg string) {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		requirements, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
		categories, _ := c.category.ListCategories(r.Context(), user, productID, "requirement")
		prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.RequirementFormModal(productpage.RequirementFormProps{
			Meta:             templates.PageMeta{CSRFToken: csrfToken},
			DisplayName:      user.DisplayName,
			IsAdmin:          user.IsAdmin,
			Product:          p,
			Requirements:     requirements,
			Categories:       categories,
			CategoryPrefixes: prefixes,
			Statuses:         statuses,
			Milestones:       milestones,
			Sprints:          sprints,
			LinkTo:           "capability:" + capabilityID,
			Title:            title,
			Description:      description,
			DefinitionOfDone: definitionOfDone,
			CategoryID:       categoryID,
			ParentID:         parentID,
			StatusID:         statusID,
			MilestoneID:      milestoneID,
			SprintID:         sprintID,
			Estimation:       estimation,
			Priority:         priorityStr,
			IsCoarse:         isCoarse,
			Error:            errMsg,
		}).Render(r.Context(), w)
	}

	req, err := c.backlog.CreateRequirement(r.Context(), user, service.CreateRequirementInput{
		ProductID:        productID,
		ParentID:         parentID,
		Title:            title,
		Description:      description,
		DefinitionOfDone: definitionOfDone,
		CategoryID:       categoryID,
		StatusID:         statusID,
		MilestoneID:      milestoneID,
		SprintID:         sprintID,
		Estimation:       estimation,
		Priority:         priority,
		IsCoarse:         isCoarse,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	if err := c.component.LinkRequirementToCapability(r.Context(), user, productID, capabilityID, req.ID); err != nil {
		rerenderForm("created requirement but failed to link: " + err.Error())
		return
	}

	c.writeCapabilityRequirementsTabOOB(w, r, user, productID, capabilityID)
}

func (c *PageProductController) CapabilityRequirementUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")
	requirementID := r.PathValue("requirementId")

	if err := c.component.UnlinkRequirementFromCapability(r.Context(), user, productID, capabilityID, requirementID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadCapabilityRequirementsTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityRequirementsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadCapabilityRequirementsTabProps(r *http.Request, user *model.User, productID, capabilityID string) (productpage.CapabilityRequirementsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.CapabilityRequirementsTabProps{}, err
	}

	links, _ := c.component.ListCapabilityRequirements(r.Context(), user, productID, capabilityID)
	requirements := make([]*model.Requirement, 0, len(links))
	for _, link := range links {
		req, err := c.backlog.GetRequirement(r.Context(), user, productID, link.RequirementID)
		if err != nil {
			continue
		}
		requirements = append(requirements, req)
	}
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	return productpage.CapabilityRequirementsTabProps{
		ProductID:        productID,
		CapabilityID:     capabilityID,
		CSRFToken:        csrfToken,
		CanWrite:         user.IsAdmin || role == model.RoleOwner,
		Requirements:     requirements,
		CategoryPrefixes: prefixes,
	}, nil
}

func (c *PageProductController) writeCapabilityRequirementsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, capabilityID string) {
	props, err := c.loadCapabilityRequirementsTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "requirements", productpage.CapabilityRequirementsTabContent(props))
}

// --- Capability → TestCase (FK 1:N, re-parent semantics) ---

// CapabilityTestCasesTab renders GET /products/{id}/capabilities/{capabilityId}/tabs/test-cases.
func (c *PageProductController) CapabilityTestCasesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	props, err := c.loadCapabilityTestCasesTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityTestCasesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityTestCaseLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityTestCaseLinkModal(productID, capabilityID).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityTestCaseLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.testing.ListTestCases(r.Context(), user, productID)

	candidates := make([]*model.TestCase, 0, len(all))
	for _, tc := range all {
		if tc.CapabilityID == capabilityID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(tc.Title), query) {
			continue
		}
		candidates = append(candidates, tc)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityTestCaseLinkResults(productID, capabilityID, candidates).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityTestCaseLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")
	testCaseID := r.FormValue("test_case_id")

	if testCaseID == "" {
		http.Error(w, "test_case_id is required", http.StatusBadRequest)
		return
	}

	newCapID := capabilityID
	if _, err := c.testing.UpdateTestCase(r.Context(), user, productID, testCaseID, service.UpdateTestCaseInput{
		CapabilityID: &newCapID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeCapabilityTestCasesTabOOB(w, r, user, productID, capabilityID)
}

func (c *PageProductController) CapabilityTestCaseCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
	pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseFormModal(productpage.TestCaseFormProps{
		Meta:          templates.PageMeta{CSRFToken: csrfToken},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		Components:    components,
		Capabilities:  capabilities,
		PyramidLevels: pyramidLevels,
		LinkTo:        "capability:" + capabilityID,
		CapabilityID:  capabilityID,
	}).Render(r.Context(), w)
}

// CapabilityTestCaseCreateAndLink handles POST /products/{id}/capabilities/{capabilityId}/test-cases.
// FK semantics: just create the test case with CapabilityID set; no separate link step.
func (c *PageProductController) CapabilityTestCaseCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	executionMode := r.FormValue("execution_mode")
	testIdentifier := r.FormValue("test_identifier")
	componentID := r.FormValue("component_id")
	pyramidLevelID := r.FormValue("pyramid_level_id")

	rerenderForm := func(errMsg string) {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
		pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.TestCaseFormModal(productpage.TestCaseFormProps{
			Meta:           templates.PageMeta{CSRFToken: csrfToken},
			DisplayName:    user.DisplayName,
			IsAdmin:        user.IsAdmin,
			Product:        p,
			Components:     components,
			Capabilities:   capabilities,
			PyramidLevels:  pyramidLevels,
			LinkTo:         "capability:" + capabilityID,
			Title:          title,
			Description:    description,
			ExecutionMode:  executionMode,
			TestIdentifier: testIdentifier,
			ComponentID:    componentID,
			CapabilityID:   capabilityID,
			PyramidLevelID: pyramidLevelID,
			Error:          errMsg,
		}).Render(r.Context(), w)
	}

	_, err = c.testing.CreateTestCase(r.Context(), user, service.CreateTestCaseInput{
		ProductID:      productID,
		Title:          title,
		Description:    description,
		ExecutionMode:  executionMode,
		TestIdentifier: testIdentifier,
		ComponentID:    componentID,
		CapabilityID:   capabilityID,
		PyramidLevelID: pyramidLevelID,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	c.writeCapabilityTestCasesTabOOB(w, r, user, productID, capabilityID)
}

// CapabilityTestCaseUnlink clears the CapabilityID FK on the given test case.
func (c *PageProductController) CapabilityTestCaseUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")
	testCaseID := r.PathValue("testCaseId")

	empty := ""
	if _, err := c.testing.UpdateTestCase(r.Context(), user, productID, testCaseID, service.UpdateTestCaseInput{
		CapabilityID: &empty,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadCapabilityTestCasesTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityTestCasesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadCapabilityTestCasesTabProps(r *http.Request, user *model.User, productID, capabilityID string) (productpage.CapabilityTestCasesTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.CapabilityTestCasesTabProps{}, err
	}

	testCases, _ := c.testing.ListTestCasesByCapability(r.Context(), user, productID, capabilityID)

	return productpage.CapabilityTestCasesTabProps{
		ProductID:    productID,
		CapabilityID: capabilityID,
		CSRFToken:    csrfToken,
		CanWrite:     c.canLinkTestCasesForRole(user, role),
		TestCases:    testCases,
	}, nil
}

func (c *PageProductController) writeCapabilityTestCasesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, capabilityID string) {
	props, err := c.loadCapabilityTestCasesTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "test-cases", productpage.CapabilityTestCasesTabContent(props))
}

// --- Capability → Defect (polymorphic via DefectLink) ---

// CapabilityDefectsTab renders GET /products/{id}/capabilities/{capabilityId}/tabs/defects.
func (c *PageProductController) CapabilityDefectsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	props, err := c.loadCapabilityDefectsTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityDefectsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityDefectLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityDefectLinkModal(productID, capabilityID).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityDefectLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{})
	linked, _, _ := c.backlog.ListLinkedDefects(r.Context(), user, productID, model.DefectLinkTargetCapability, capabilityID)
	linkedIDs := make(map[string]struct{}, len(linked))
	for _, d := range linked {
		linkedIDs[d.ID] = struct{}{}
	}

	candidates := make([]*model.Defect, 0, len(all))
	for _, d := range all {
		if _, ok := linkedIDs[d.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(d.Title), query) {
			continue
		}
		candidates = append(candidates, d)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityDefectLinkResults(productID, capabilityID, candidates).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityDefectLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")
	defectID := r.FormValue("defect_id")

	if defectID == "" {
		http.Error(w, "defect_id is required", http.StatusBadRequest)
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defectID, model.DefectLinkTargetCapability, capabilityID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeCapabilityDefectsTabOOB(w, r, user, productID, capabilityID)
}

func (c *PageProductController) CapabilityDefectCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormModal(productpage.DefectFormProps{
		Meta:        templates.PageMeta{CSRFToken: csrfToken},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Statuses:    statuses,
		Milestones:  milestones,
		Sprints:     sprints,
		LinkTo:      "capability:" + capabilityID,
		Criticality: "medium",
		StatusID:    defaultStatusID(statuses),
	}).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityDefectCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	criticality := r.FormValue("criticality")
	statusID := r.FormValue("status_id")
	milestoneID := r.FormValue("milestone_id")
	sprintID := r.FormValue("sprint_id")
	estimation := r.FormValue("estimation")
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)

	rerenderForm := func(errMsg string) {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormModal(productpage.DefectFormProps{
			Meta:        templates.PageMeta{CSRFToken: csrfToken},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Statuses:    statuses,
			Milestones:  milestones,
			Sprints:     sprints,
			LinkTo:      "capability:" + capabilityID,
			Title:       title,
			Description: description,
			Criticality: criticality,
			StatusID:    statusID,
			MilestoneID: milestoneID,
			SprintID:    sprintID,
			Estimation:  estimation,
			Priority:    priorityStr,
			Error:       errMsg,
		}).Render(r.Context(), w)
	}

	d, err := c.backlog.CreateDefect(r.Context(), user, service.CreateDefectInput{
		ProductID:   productID,
		Title:       title,
		Description: description,
		Criticality: criticality,
		StatusID:    statusID,
		MilestoneID: milestoneID,
		SprintID:    sprintID,
		Estimation:  estimation,
		Priority:    priority,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, d.ID, model.DefectLinkTargetCapability, capabilityID); err != nil {
		rerenderForm("created defect but failed to link: " + err.Error())
		return
	}

	c.writeCapabilityDefectsTabOOB(w, r, user, productID, capabilityID)
}

func (c *PageProductController) CapabilityDefectUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")
	linkID := r.PathValue("linkId")

	if err := c.backlog.RemoveDefectLink(r.Context(), user, productID, linkID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadCapabilityDefectsTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityDefectsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadCapabilityDefectsTabProps(r *http.Request, user *model.User, productID, capabilityID string) (productpage.CapabilityDefectsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.CapabilityDefectsTabProps{}, err
	}

	defects, linkIDs, _ := c.backlog.ListLinkedDefects(r.Context(), user, productID, model.DefectLinkTargetCapability, capabilityID)

	return productpage.CapabilityDefectsTabProps{
		ProductID:    productID,
		CapabilityID: capabilityID,
		CSRFToken:    csrfToken,
		CanLink:      c.canAddDefectLinksForRole(user, role),
		CanUnlink:    c.canRemoveDefectLinksForRole(user, role),
		Defects:      defects,
		LinkIDs:      linkIDs,
	}, nil
}

func (c *PageProductController) writeCapabilityDefectsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, capabilityID string) {
	props, err := c.loadCapabilityDefectsTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "defects", productpage.CapabilityDefectsTabContent(props))
}

// CapabilityHistoryTab renders GET /products/{id}/capabilities/{capabilityId}/tabs/history.
func (c *PageProductController) CapabilityHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	views := c.buildAuditViews(r, user, productID, "capability", capabilityID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityHistoryTabContent(views).Render(r.Context(), w)
}

// --- shared helpers for components/capabilities ---

// buildMilestoneMap returns a map of milestone ID → name for the product.
func (c *PageProductController) buildMilestoneMap(r *http.Request, user *model.User, productID string) map[string]string {
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	m := make(map[string]string, len(milestones))
	for _, ms := range milestones {
		m[ms.ID] = ms.Name
	}
	return m
}

// buildComponentMap returns a map of component ID → name for the product.
func (c *PageProductController) buildComponentMap(r *http.Request, user *model.User, productID string) map[string]string {
	components, _ := c.component.ListComponents(r.Context(), user, productID)
	m := make(map[string]string, len(components))
	for _, comp := range components {
		m[comp.ID] = comp.Name
	}
	return m
}

// resolveMilestoneName returns the milestone name for the given ID, or empty string.
func (c *PageProductController) resolveMilestoneName(r *http.Request, user *model.User, productID, milestoneID string) string {
	if milestoneID == "" {
		return ""
	}
	ms, err := c.milestone.GetMilestone(r.Context(), user, productID, milestoneID)
	if err != nil {
		return ""
	}
	return ms.Name
}

// resolveComponentName returns the component name for the given ID, or empty string.
func (c *PageProductController) resolveComponentName(r *http.Request, user *model.User, productID, componentID string) string {
	if componentID == "" {
		return ""
	}
	comp, err := c.component.GetComponent(r.Context(), user, productID, componentID)
	if err != nil {
		return ""
	}
	return comp.Name
}

// buildAuditViews fetches audit entries and resolves user display names.
func (c *PageProductController) buildAuditViews(r *http.Request, user *model.User, productID, entityType, entityID string) []templates.AuditEntryView {
	entries, err := c.audit.ListByEntity(r.Context(), user, productID, entityType, entityID)
	if err != nil {
		return nil
	}

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
	return views
}

// --- Test Case Pages ---

// TestCases renders GET /products/{id}/test-cases.
func (c *PageProductController) TestCases(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	testCases, _ := c.testing.ListTestCases(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCasesPage(productpage.TestCasesProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Test Cases",
			CSRFToken: csrfToken,
		},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		TestCases:     testCases,
		Components:    c.buildComponentMap(r, user, productID),
		PyramidLevels: c.buildPyramidLevelMap(r, user, productID),
		CanEdit:       c.canEditTestCases(user, role),
	}).Render(r.Context(), w)
}

// TestCaseDetail renders GET /products/{id}/test-cases/{testCaseId}.
func (c *PageProductController) TestCaseDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tc, err := c.testing.GetTestCase(r.Context(), user, productID, testCaseID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/test-cases", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseDetailPage(productpage.TestCaseDetailProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - " + tc.Title,
			CSRFToken: csrfToken,
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		TestCase:         tc,
		ComponentName:    c.resolveComponentName(r, user, productID, tc.ComponentID),
		CapabilityName:   c.resolveCapabilityName(r, user, productID, tc.CapabilityID),
		PyramidLevelName: c.resolvePyramidLevelName(r, user, productID, tc.PyramidLevelID),
		CanEdit:          c.canEditTestCases(user, role),
	}).Render(r.Context(), w)
}

// TestCaseCreateForm renders GET /products/{id}/test-cases/new.
func (c *PageProductController) TestCaseCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
	pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseFormPage(productpage.TestCaseFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - New Test Case",
			CSRFToken: csrfToken,
		},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		Components:    components,
		Capabilities:  capabilities,
		PyramidLevels: pyramidLevels,
		ExecutionMode: "manual",
	}).Render(r.Context(), w)
}

// TestCaseCreate handles POST /products/{id}/test-cases.
func (c *PageProductController) TestCaseCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	executionMode := r.FormValue("execution_mode")
	testIdentifier := r.FormValue("test_identifier")
	componentID := r.FormValue("component_id")
	capabilityID := r.FormValue("capability_id")
	pyramidLevelID := r.FormValue("pyramid_level_id")

	_, err = c.testing.CreateTestCase(r.Context(), user, service.CreateTestCaseInput{
		ProductID:      productID,
		ComponentID:    componentID,
		CapabilityID:   capabilityID,
		Title:          title,
		Description:    description,
		ExecutionMode:  executionMode,
		TestIdentifier: testIdentifier,
		PyramidLevelID: pyramidLevelID,
	})
	if err != nil {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
		pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.TestCaseFormPage(productpage.TestCaseFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - New Test Case",
				CSRFToken: csrfToken,
			},
			DisplayName:    user.DisplayName,
			IsAdmin:        user.IsAdmin,
			Product:        p,
			Components:     components,
			Capabilities:   capabilities,
			PyramidLevels:  pyramidLevels,
			Error:          err.Error(),
			Title:          title,
			Description:    description,
			ExecutionMode:  executionMode,
			TestIdentifier: testIdentifier,
			ComponentID:    componentID,
			CapabilityID:   capabilityID,
			PyramidLevelID: pyramidLevelID,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/test-cases", http.StatusSeeOther)
}

// TestCaseEditForm renders GET /products/{id}/test-cases/{testCaseId}/edit.
func (c *PageProductController) TestCaseEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tc, err := c.testing.GetTestCase(r.Context(), user, productID, testCaseID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/test-cases", http.StatusSeeOther)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
	pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseFormPage(productpage.TestCaseFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Edit " + tc.Title,
			CSRFToken: csrfToken,
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		TestCase:       tc,
		Components:     components,
		Capabilities:   capabilities,
		PyramidLevels:  pyramidLevels,
		IsEdit:         true,
		ReturnTo:       editReturnTarget(r),
		Title:          tc.Title,
		Description:    tc.Description,
		ExecutionMode:  tc.ExecutionMode,
		TestIdentifier: tc.TestIdentifier,
		ComponentID:    tc.ComponentID,
		CapabilityID:   tc.CapabilityID,
		PyramidLevelID: tc.PyramidLevelID,
	}).Render(r.Context(), w)
}

// TestCaseUpdate handles POST /products/{id}/test-cases/{testCaseId}.
func (c *PageProductController) TestCaseUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tc, err := c.testing.GetTestCase(r.Context(), user, productID, testCaseID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/test-cases", http.StatusSeeOther)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	executionMode := r.FormValue("execution_mode")
	testIdentifier := r.FormValue("test_identifier")
	componentID := r.FormValue("component_id")
	capabilityID := r.FormValue("capability_id")
	pyramidLevelID := r.FormValue("pyramid_level_id")

	_, err = c.testing.UpdateTestCase(r.Context(), user, productID, testCaseID, service.UpdateTestCaseInput{
		Title:          &title,
		Description:    &description,
		ExecutionMode:  &executionMode,
		TestIdentifier: &testIdentifier,
		ComponentID:    &componentID,
		CapabilityID:   &capabilityID,
		PyramidLevelID: &pyramidLevelID,
	})
	if err != nil {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
		pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.TestCaseFormPage(productpage.TestCaseFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - Edit " + tc.Title,
				CSRFToken: csrfToken,
			},
			DisplayName:    user.DisplayName,
			IsAdmin:        user.IsAdmin,
			Product:        p,
			TestCase:       tc,
			Components:     components,
			Capabilities:   capabilities,
			PyramidLevels:  pyramidLevels,
			IsEdit:         true,
			ReturnTo:       r.FormValue("return"),
			Error:          err.Error(),
			Title:          title,
			Description:    description,
			ExecutionMode:  executionMode,
			TestIdentifier: testIdentifier,
			ComponentID:    componentID,
			CapabilityID:   capabilityID,
			PyramidLevelID: pyramidLevelID,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		"/products/"+productID+"/test-cases/"+testCaseID,
		"/products/"+productID+"/test-cases",
	), http.StatusSeeOther)
}

// TestCaseDelete handles DELETE /products/{id}/test-cases/{testCaseId}.
func (c *PageProductController) TestCaseDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	_ = c.testing.DeleteTestCase(r.Context(), user, productID, testCaseID)

	w.Header().Set("HX-Redirect", "/products/"+productID+"/test-cases")
	w.WriteHeader(http.StatusOK)
}

// TestCaseDeleteConfirm renders GET /products/{id}/test-cases/{testCaseId}/delete/confirm.
func (c *PageProductController) TestCaseDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	tc, err := c.testing.GetTestCase(r.Context(), user, productID, testCaseID)
	if err != nil {
		http.Error(w, "test case not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseDeleteConfirmation(tc.Title, productID, testCaseID, middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// TestCaseOverviewTab renders GET /products/{id}/test-cases/{testCaseId}/tabs/overview.
func (c *PageProductController) TestCaseOverviewTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	tc, err := c.testing.GetTestCase(r.Context(), user, productID, testCaseID)
	if err != nil {
		http.Error(w, "test case not found", http.StatusNotFound)
		return
	}

	componentName := c.resolveComponentName(r, user, productID, tc.ComponentID)
	capabilityName := c.resolveCapabilityName(r, user, productID, tc.CapabilityID)
	pyramidLevelName := c.resolvePyramidLevelName(r, user, productID, tc.PyramidLevelID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseOverviewTabContent(productID, tc, componentName, capabilityName, pyramidLevelName).Render(r.Context(), w)
}

// TestCaseRequirementsTab renders GET /products/{id}/test-cases/{testCaseId}/tabs/requirements.
func (c *PageProductController) TestCaseRequirementsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	props, err := c.loadTestCaseRequirementsTabProps(r, user, productID, testCaseID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseRequirementsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) TestCaseRequirementLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseRequirementLinkModal(productID, testCaseID).Render(r.Context(), w)
}

func (c *PageProductController) TestCaseRequirementLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
	links, _ := c.testing.ListTestCaseRequirements(r.Context(), user, productID, testCaseID)
	linkedIDs := make(map[string]struct{}, len(links))
	for _, l := range links {
		linkedIDs[l.RequirementID] = struct{}{}
	}

	candidates := make([]*model.Requirement, 0, len(all))
	for _, req := range all {
		if _, ok := linkedIDs[req.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(req.Title), query) {
			continue
		}
		candidates = append(candidates, req)
	}
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseRequirementLinkResults(productID, testCaseID, candidates, prefixes).Render(r.Context(), w)
}

func (c *PageProductController) TestCaseRequirementLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")
	requirementID := r.FormValue("requirement_id")

	if requirementID == "" {
		http.Error(w, "requirement_id is required", http.StatusBadRequest)
		return
	}

	if err := c.testing.LinkRequirement(r.Context(), user, productID, testCaseID, requirementID, "requirement"); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeTestCaseRequirementsTabOOB(w, r, user, productID, testCaseID)
}

func (c *PageProductController) TestCaseRequirementCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
	requirements, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
	categories, _ := c.category.ListCategories(r.Context(), user, productID, "requirement")
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementFormModal(productpage.RequirementFormProps{
		Meta:             templates.PageMeta{CSRFToken: csrfToken},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Requirements:     requirements,
		Categories:       categories,
		CategoryPrefixes: prefixes,
		Statuses:         statuses,
		Milestones:       milestones,
		Sprints:          sprints,
		LinkTo:           "test_case:" + testCaseID,
		StatusID:         defaultStatusID(statuses),
		Priority:         "0",
	}).Render(r.Context(), w)
}

func (c *PageProductController) TestCaseRequirementCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	description := r.FormValue("description")
	definitionOfDone := r.FormValue("definition_of_done")
	categoryID := r.FormValue("category_id")
	parentID := r.FormValue("parent_id")
	statusID := r.FormValue("status_id")
	milestoneID := r.FormValue("milestone_id")
	sprintID := r.FormValue("sprint_id")
	estimation := strings.TrimSpace(r.FormValue("estimation"))
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)
	isCoarse := r.FormValue("is_coarse") == "on"

	rerenderForm := func(errMsg string) {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		requirements, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
		categories, _ := c.category.ListCategories(r.Context(), user, productID, "requirement")
		prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.RequirementFormModal(productpage.RequirementFormProps{
			Meta:             templates.PageMeta{CSRFToken: csrfToken},
			DisplayName:      user.DisplayName,
			IsAdmin:          user.IsAdmin,
			Product:          p,
			Requirements:     requirements,
			Categories:       categories,
			CategoryPrefixes: prefixes,
			Statuses:         statuses,
			Milestones:       milestones,
			Sprints:          sprints,
			LinkTo:           "test_case:" + testCaseID,
			Title:            title,
			Description:      description,
			DefinitionOfDone: definitionOfDone,
			CategoryID:       categoryID,
			ParentID:         parentID,
			StatusID:         statusID,
			MilestoneID:      milestoneID,
			SprintID:         sprintID,
			Estimation:       estimation,
			Priority:         priorityStr,
			IsCoarse:         isCoarse,
			Error:            errMsg,
		}).Render(r.Context(), w)
	}

	req, err := c.backlog.CreateRequirement(r.Context(), user, service.CreateRequirementInput{
		ProductID:        productID,
		ParentID:         parentID,
		Title:            title,
		Description:      description,
		DefinitionOfDone: definitionOfDone,
		CategoryID:       categoryID,
		StatusID:         statusID,
		MilestoneID:      milestoneID,
		SprintID:         sprintID,
		Estimation:       estimation,
		Priority:         priority,
		IsCoarse:         isCoarse,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	if err := c.testing.LinkRequirement(r.Context(), user, productID, testCaseID, req.ID, "requirement"); err != nil {
		rerenderForm("created requirement but failed to link: " + err.Error())
		return
	}

	c.writeTestCaseRequirementsTabOOB(w, r, user, productID, testCaseID)
}

func (c *PageProductController) TestCaseRequirementUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")
	requirementID := r.PathValue("requirementId")

	if err := c.testing.UnlinkRequirement(r.Context(), user, productID, testCaseID, requirementID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadTestCaseRequirementsTabProps(r, user, productID, testCaseID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseRequirementsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadTestCaseRequirementsTabProps(r *http.Request, user *model.User, productID, testCaseID string) (productpage.TestCaseRequirementsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.TestCaseRequirementsTabProps{}, err
	}

	links, _ := c.testing.ListTestCaseRequirements(r.Context(), user, productID, testCaseID)
	requirements := make([]*model.Requirement, 0, len(links))
	for _, link := range links {
		req, err := c.backlog.GetRequirement(r.Context(), user, productID, link.RequirementID)
		if err != nil {
			continue
		}
		requirements = append(requirements, req)
	}
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	return productpage.TestCaseRequirementsTabProps{
		ProductID:        productID,
		TestCaseID:       testCaseID,
		CSRFToken:        csrfToken,
		CanWrite:         c.canLinkTestCasesForRole(user, role),
		Requirements:     requirements,
		CategoryPrefixes: prefixes,
	}, nil
}

func (c *PageProductController) writeTestCaseRequirementsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, testCaseID string) {
	props, err := c.loadTestCaseRequirementsTabProps(r, user, productID, testCaseID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "requirements", productpage.TestCaseRequirementsTabContent(props))
}

// TestCaseCampaignsTab renders GET /products/{id}/test-cases/{testCaseId}/tabs/campaigns.
func (c *PageProductController) TestCaseCampaignsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	results, err := c.testing.ListResultsForTestCase(r.Context(), user, productID, testCaseID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	envNames := c.buildEnvironmentMap(r, user, productID)
	campaignNames := map[string]string{}
	for _, campaign := range c.listCampaignsQuietly(r, user, productID) {
		campaignNames[campaign.ID] = campaign.Name
	}

	views := make([]productpage.TestCaseCampaignResultView, 0, len(results))
	for _, result := range results {
		view := productpage.TestCaseCampaignResultView{
			Result:       result,
			CampaignName: campaignNames[result.CampaignID],
			Environment:  envNames[result.EnvironmentID],
			ExecutedBy:   c.resolveUserDisplayName(r, user, result.ExecutedBy),
		}
		if result.DefectID != "" {
			if d, err := c.backlog.GetDefect(r.Context(), user, productID, result.DefectID); err == nil {
				view.DefectTitle = d.Title
			} else {
				view.DefectTitle = "Defect"
			}
		}
		views = append(views, view)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseCampaignsTabContent(productpage.TestCaseCampaignsTabProps{
		ProductID: productID,
		Results:   views,
	}).Render(r.Context(), w)
}

func (c *PageProductController) listCampaignsQuietly(r *http.Request, user *model.User, productID string) []*model.TestingCampaign {
	campaigns, _ := c.testing.ListCampaigns(r.Context(), user, productID)
	return campaigns
}

// resolveUserDisplayName resolves a user ID to a display name, falling back to
// the raw ID so a deleted account still shows something meaningful.
func (c *PageProductController) resolveUserDisplayName(r *http.Request, actor *model.User, userID string) string {
	if userID == "" {
		return ""
	}
	if userID == actor.ID {
		return actor.DisplayName
	}
	u, err := c.user.GetByID(r.Context(), userID)
	if err != nil || u.DisplayName == "" {
		return userID
	}
	return u.DisplayName
}

// TestCaseHistoryTab renders GET /products/{id}/test-cases/{testCaseId}/tabs/history.
func (c *PageProductController) TestCaseHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	entries := c.buildAuditViews(r, user, productID, "test_case", testCaseID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseHistoryTabContent(entries).Render(r.Context(), w)
}

// canEditTestCases returns true if the user can create/edit/delete test cases (developer+).
func (c *PageProductController) canEditTestCases(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner || role == model.RoleDeveloper
}

// buildPyramidLevelMap returns a map of pyramid level ID → name for the product.
func (c *PageProductController) buildPyramidLevelMap(r *http.Request, user *model.User, productID string) map[string]string {
	levels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
	m := make(map[string]string, len(levels))
	for _, l := range levels {
		m[l.ID] = l.Name
	}
	return m
}

// buildCapabilityMap returns a map of capability ID → name for the product.
func (c *PageProductController) buildCapabilityMap(r *http.Request, user *model.User, productID string) map[string]string {
	funcs, _ := c.component.ListCapabilities(r.Context(), user, productID)
	m := make(map[string]string, len(funcs))
	for _, f := range funcs {
		m[f.ID] = f.Name
	}
	return m
}

// resolveCapabilityName returns the capability name for the given ID, or empty string.
func (c *PageProductController) resolveCapabilityName(r *http.Request, user *model.User, productID, capabilityID string) string {
	if capabilityID == "" {
		return ""
	}
	f, err := c.component.GetCapability(r.Context(), user, productID, capabilityID)
	if err != nil {
		return ""
	}
	return f.Name
}

// resolvePyramidLevelName returns the pyramid level name for the given ID, or empty string.
func (c *PageProductController) resolvePyramidLevelName(r *http.Request, user *model.User, productID, pyramidLevelID string) string {
	if pyramidLevelID == "" {
		return ""
	}
	pl, err := c.milestone.GetPyramidLevel(r.Context(), user, productID, pyramidLevelID)
	if err != nil {
		return ""
	}
	return pl.Name
}

// --- Shared helpers for requirements/defects ---

// buildStatusMap returns a map of status ID → name for the given entity type.
func (c *PageProductController) buildStatusMap(r *http.Request, user *model.User, productID, entityType string) map[string]string {
	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, entityType)
	m := make(map[string]string, len(statuses))
	for _, s := range statuses {
		m[s.ID] = s.Name
	}
	return m
}

// resolveStatusName returns the status name for the given ID and entity type, or empty string.
func (c *PageProductController) resolveStatusName(r *http.Request, user *model.User, productID, statusID, entityType string) string {
	if statusID == "" {
		return ""
	}
	statuses, err := c.milestone.ListStatuses(r.Context(), user, productID, entityType)
	if err != nil {
		return ""
	}
	for _, s := range statuses {
		if s.ID == statusID {
			return s.Name
		}
	}
	return ""
}

// buildSprintMap returns a map of sprint ID → name for the product.
func (c *PageProductController) buildSprintMap(r *http.Request, user *model.User, productID string) map[string]string {
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
	m := make(map[string]string, len(sprints))
	for _, s := range sprints {
		m[s.ID] = s.Name
	}
	return m
}

// resolveSprintName returns the sprint name for the given ID, or empty string.
func (c *PageProductController) resolveSprintName(r *http.Request, user *model.User, productID, sprintID string) string {
	if sprintID == "" {
		return ""
	}
	sprints, err := c.milestone.ListSprints(r.Context(), user, productID)
	if err != nil {
		return ""
	}
	for _, s := range sprints {
		if s.ID == sprintID {
			return s.Name
		}
	}
	return ""
}

// canEditRequirements returns true if the user can create/edit/delete requirements (owner or admin).
func (c *PageProductController) canEditRequirements(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner
}

// canCreateDefects returns true if the user can create defects (tester+ or admin).
func (c *PageProductController) canCreateDefects(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner || role == model.RoleDeveloper || role == model.RoleTester
}

// canEditDefects returns true if the user can edit/delete defects (owner or admin).
func (c *PageProductController) canEditDefects(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner
}

// buildCategoryPrefixMap returns a map of category ID to prefix for the given entity type.
func (c *PageProductController) buildCategoryPrefixMap(r *http.Request, user *model.User, productID, entityType string) map[string]string {
	cats, err := c.category.ListCategories(r.Context(), user, productID, entityType)
	if err != nil {
		return nil
	}
	m := make(map[string]string, len(cats))
	for _, cat := range cats {
		m[cat.ID] = cat.Prefix
	}
	return m
}

// requirementsOnly excludes tasks from a requirement listing. Requirements and
// tasks share the requirements table, so every requirement-facing list, picker
// and parent dropdown has to say so explicitly.
var requirementsOnly = store.RequirementFilter{Kind: model.RequirementKindRequirement}

// tasksOnly is the mirror of requirementsOnly for the task pages.
var tasksOnly = store.RequirementFilter{Kind: model.RequirementKindTask}

// buildBacklogPrefixMap merges the requirement and task category prefixes into
// one map, for views that render both kinds side by side (the board). Category
// IDs are UUIDs, so the two sets cannot collide.
func (c *PageProductController) buildBacklogPrefixMap(r *http.Request, user *model.User, productID string) map[string]string {
	m := c.buildCategoryPrefixMap(r, user, productID, model.RequirementKindRequirement)
	if m == nil {
		m = make(map[string]string)
	}
	for id, prefix := range c.buildCategoryPrefixMap(r, user, productID, model.RequirementKindTask) {
		m[id] = prefix
	}
	return m
}

// buildCategoryNameMap returns a map of category ID to name for the given entity type.
func (c *PageProductController) buildCategoryNameMap(r *http.Request, user *model.User, productID, entityType string) map[string]string {
	cats, err := c.category.ListCategories(r.Context(), user, productID, entityType)
	if err != nil {
		return nil
	}
	m := make(map[string]string, len(cats))
	for _, cat := range cats {
		m[cat.ID] = cat.Name
	}
	return m
}

// categoryPrefix looks up the prefix for a category ID from a prefix map, falling back to "??".
func categoryPrefix(prefixes map[string]string, categoryID string) string {
	if p, ok := prefixes[categoryID]; ok {
		return p
	}
	return "??"
}

// defaultStatusID returns the ID of the status flagged IsDefault, or "" if none.
func defaultStatusID(statuses []*model.Status) string {
	for _, s := range statuses {
		if s.IsDefault {
			return s.ID
		}
	}
	return ""
}

// --- Requirement Pages ---

// Requirements renders GET /products/{id}/requirements.
func (c *PageProductController) Requirements(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	requirements, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementsPage(productpage.RequirementsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Requirements",
			CSRFToken: csrfToken,
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Requirements:     requirements,
		CategoryPrefixes: prefixes,
		Statuses:         c.buildStatusMap(r, user, productID, "requirement"),
		CanEdit:          c.canEditRequirements(user, role),
	}).Render(r.Context(), w)
}

// RequirementDetail renders GET /products/{id}/requirements/{requirementId}.
func (c *PageProductController) RequirementDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	req, err := c.backlog.GetRequirement(r.Context(), user, productID, requirementID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/requirements", http.StatusSeeOther)
		return
	}

	// Load categories for prefix and name mapping.
	// Merged across kinds: a requirement can be refined into child tasks
	// (FR-326), so the children table has to resolve task prefixes too.
	prefixes := c.buildBacklogPrefixMap(r, user, productID)
	catNames := c.buildCategoryNameMap(r, user, productID, "requirement")

	// Resolve parent requirement name if set.
	var parentName, parentIdentifier string
	if req.ParentID != "" {
		if parent, pErr := c.backlog.GetRequirement(r.Context(), user, productID, req.ParentID); pErr == nil {
			parentName = parent.Title
			parentIdentifier = model.EntityIdentifier(categoryPrefix(prefixes, parent.CategoryID), parent.SequenceNumber)
		}
	}

	// Load child requirements.
	children, _ := c.backlog.ListChildRequirements(r.Context(), user, productID, requirementID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementDetailPage(productpage.RequirementDetailProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - " + req.Title,
			CSRFToken: csrfToken,
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Requirement:      req,
		CategoryPrefixes: prefixes,
		CategoryNames:    catNames,
		StatusName:       c.resolveStatusName(r, user, productID, req.StatusID, "requirement"),
		MilestoneName:    c.resolveMilestoneName(r, user, productID, req.MilestoneID),
		SprintName:       c.resolveSprintName(r, user, productID, req.SprintID),
		ParentName:       parentName,
		ParentIdentifier: parentIdentifier,
		Children:         children,
		CanEdit:          c.canEditRequirements(user, role),
	}).Render(r.Context(), w)
}

// RequirementCreateForm renders GET /products/{id}/requirements/new.
func (c *PageProductController) RequirementCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
	requirements, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
	categories, _ := c.category.ListCategories(r.Context(), user, productID, "requirement")
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementFormPage(productpage.RequirementFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - New Requirement",
			CSRFToken: csrfToken,
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Requirements:     requirements,
		Categories:       categories,
		CategoryPrefixes: prefixes,
		Statuses:         statuses,
		Milestones:       milestones,
		Sprints:          sprints,
		CategoryID:       "",
		StatusID:         defaultStatusID(statuses),
		Priority:         "0",
	}).Render(r.Context(), w)
}

// RequirementCreate handles POST /products/{id}/requirements.
func (c *PageProductController) RequirementCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	description := r.FormValue("description")
	definitionOfDone := r.FormValue("definition_of_done")
	categoryID := r.FormValue("category_id")
	parentID := r.FormValue("parent_id")
	statusID := r.FormValue("status_id")
	milestoneID := r.FormValue("milestone_id")
	sprintID := r.FormValue("sprint_id")
	estimation := strings.TrimSpace(r.FormValue("estimation"))
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)
	isCoarse := r.FormValue("is_coarse") == "on"

	_, createErr := c.backlog.CreateRequirement(r.Context(), user, service.CreateRequirementInput{
		ProductID:        productID,
		ParentID:         parentID,
		Title:            title,
		Description:      description,
		DefinitionOfDone: definitionOfDone,
		CategoryID:       categoryID,
		StatusID:         statusID,
		MilestoneID:      milestoneID,
		SprintID:         sprintID,
		Estimation:       estimation,
		Priority:         priority,
		IsCoarse:         isCoarse,
	})
	if createErr != nil {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		requirements, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
		categories, _ := c.category.ListCategories(r.Context(), user, productID, "requirement")
		prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.RequirementFormPage(productpage.RequirementFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - New Requirement",
				CSRFToken: csrfToken,
			},
			DisplayName:      user.DisplayName,
			IsAdmin:          user.IsAdmin,
			Product:          p,
			Requirements:     requirements,
			Categories:       categories,
			CategoryPrefixes: prefixes,
			Statuses:         statuses,
			Milestones:       milestones,
			Sprints:          sprints,
			Error:            createErr.Error(),
			Title:            title,
			Description:      description,
			DefinitionOfDone: definitionOfDone,
			CategoryID:       categoryID,
			ParentID:         parentID,
			StatusID:         statusID,
			MilestoneID:      milestoneID,
			SprintID:         sprintID,
			Estimation:       estimation,
			Priority:         priorityStr,
			IsCoarse:         isCoarse,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/requirements", http.StatusSeeOther)
}

// RequirementEditForm renders GET /products/{id}/requirements/{requirementId}/edit.
func (c *PageProductController) RequirementEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	req, err := c.backlog.GetRequirement(r.Context(), user, productID, requirementID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/requirements", http.StatusSeeOther)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
	requirements, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
	categories, _ := c.category.ListCategories(r.Context(), user, productID, "requirement")
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementFormPage(productpage.RequirementFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Edit " + req.Title,
			CSRFToken: csrfToken,
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Requirement:      req,
		Requirements:     requirements,
		Categories:       categories,
		CategoryPrefixes: prefixes,
		Statuses:         statuses,
		Milestones:       milestones,
		Sprints:          sprints,
		IsEdit:           true,
		ReturnTo:         editReturnTarget(r),
		Title:            req.Title,
		Description:      req.Description,
		DefinitionOfDone: req.DefinitionOfDone,
		CategoryID:       req.CategoryID,
		ParentID:         req.ParentID,
		StatusID:         req.StatusID,
		MilestoneID:      req.MilestoneID,
		SprintID:         req.SprintID,
		Estimation:       req.Estimation,
		Priority:         fmt.Sprintf("%d", req.Priority),
		IsCoarse:         req.IsCoarse,
	}).Render(r.Context(), w)
}

// RequirementUpdate handles POST /products/{id}/requirements/{requirementId}.
func (c *PageProductController) RequirementUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	req, err := c.backlog.GetRequirement(r.Context(), user, productID, requirementID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/requirements", http.StatusSeeOther)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	description := r.FormValue("description")
	definitionOfDone := r.FormValue("definition_of_done")
	categoryID := r.FormValue("category_id")
	parentID := r.FormValue("parent_id")
	statusID := r.FormValue("status_id")
	milestoneID := r.FormValue("milestone_id")
	sprintID := r.FormValue("sprint_id")
	estimation := strings.TrimSpace(r.FormValue("estimation"))
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)
	isCoarse := r.FormValue("is_coarse") == "on"

	_, updateErr := c.backlog.UpdateRequirement(r.Context(), user, productID, requirementID, service.UpdateRequirementInput{
		ParentID:         &parentID,
		Title:            &title,
		Description:      &description,
		DefinitionOfDone: &definitionOfDone,
		CategoryID:       &categoryID,
		StatusID:         &statusID,
		MilestoneID:      &milestoneID,
		SprintID:         &sprintID,
		Estimation:       &estimation,
		Priority:         &priority,
		IsCoarse:         &isCoarse,
	})
	if updateErr != nil {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		requirements, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
		categories, _ := c.category.ListCategories(r.Context(), user, productID, "requirement")
		prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.RequirementFormPage(productpage.RequirementFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - Edit " + req.Title,
				CSRFToken: csrfToken,
			},
			DisplayName:      user.DisplayName,
			IsAdmin:          user.IsAdmin,
			Product:          p,
			Requirement:      req,
			Requirements:     requirements,
			Categories:       categories,
			CategoryPrefixes: prefixes,
			Statuses:         statuses,
			Milestones:       milestones,
			Sprints:          sprints,
			IsEdit:           true,
			ReturnTo:         r.FormValue("return"),
			Error:            updateErr.Error(),
			Title:            title,
			Description:      description,
			DefinitionOfDone: definitionOfDone,
			CategoryID:       categoryID,
			ParentID:         parentID,
			StatusID:         statusID,
			MilestoneID:      milestoneID,
			SprintID:         sprintID,
			Estimation:       estimation,
			Priority:         priorityStr,
			IsCoarse:         isCoarse,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		"/products/"+productID+"/requirements/"+requirementID,
		"/products/"+productID+"/requirements",
	), http.StatusSeeOther)
}

// RequirementDelete handles DELETE /products/{id}/requirements/{requirementId}.
func (c *PageProductController) RequirementDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	_ = c.backlog.DeleteRequirement(r.Context(), user, productID, requirementID)
	w.Header().Set("HX-Redirect", "/products/"+productID+"/requirements")
	w.WriteHeader(http.StatusOK)
}

// RequirementDeleteConfirm renders GET /products/{id}/requirements/{requirementId}/delete/confirm.
func (c *PageProductController) RequirementDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	req, err := c.backlog.GetRequirement(r.Context(), user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementDeleteConfirmation(req.Title, productID, requirementID, middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// RequirementOverviewTab renders GET /products/{id}/requirements/{requirementId}/tabs/overview.
func (c *PageProductController) RequirementOverviewTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	req, err := c.backlog.GetRequirement(r.Context(), user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Merged across kinds: a requirement can be refined into child tasks
	// (FR-326), so the children table has to resolve task prefixes too.
	prefixes := c.buildBacklogPrefixMap(r, user, productID)
	catNames := c.buildCategoryNameMap(r, user, productID, "requirement")

	var parentName, parentIdentifier string
	if req.ParentID != "" {
		if parent, pErr := c.backlog.GetRequirement(r.Context(), user, productID, req.ParentID); pErr == nil {
			parentName = parent.Title
			parentIdentifier = model.EntityIdentifier(categoryPrefix(prefixes, parent.CategoryID), parent.SequenceNumber)
		}
	}
	children, _ := c.backlog.ListChildRequirements(r.Context(), user, productID, requirementID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementOverviewTabContent(productpage.RequirementDetailProps{
		Product:          p,
		Requirement:      req,
		CategoryPrefixes: prefixes,
		CategoryNames:    catNames,
		StatusName:       c.resolveStatusName(r, user, productID, req.StatusID, "requirement"),
		MilestoneName:    c.resolveMilestoneName(r, user, productID, req.MilestoneID),
		SprintName:       c.resolveSprintName(r, user, productID, req.SprintID),
		ParentName:       parentName,
		ParentIdentifier: parentIdentifier,
		Children:         children,
		CanEdit:          c.canEditRequirements(user, role),
	}).Render(r.Context(), w)
}

// RequirementCapabilitiesTab renders GET /products/{id}/requirements/{requirementId}/tabs/capabilities.
func (c *PageProductController) RequirementCapabilitiesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	props, err := c.loadRequirementCapabilitiesTabProps(r, user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementCapabilitiesTabContent(props).Render(r.Context(), w)
}

// RequirementCapabilityLinkModal renders GET /products/{id}/requirements/{requirementId}/capabilities/link-modal.
func (c *PageProductController) RequirementCapabilityLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementCapabilityLinkModal(productID, requirementID).Render(r.Context(), w)
}

// RequirementCapabilityLinkSearch renders GET /products/{id}/requirements/{requirementId}/capabilities/link-search.
func (c *PageProductController) RequirementCapabilityLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.component.ListCapabilities(r.Context(), user, productID)
	linked, _ := c.component.ListRequirementCapabilities(r.Context(), user, productID, requirementID)
	linkedIDs := make(map[string]struct{}, len(linked))
	for _, link := range linked {
		linkedIDs[link.CapabilityID] = struct{}{}
	}

	candidates := make([]*model.Capability, 0, len(all))
	for _, cap := range all {
		if _, ok := linkedIDs[cap.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(cap.Name), query) {
			continue
		}
		candidates = append(candidates, cap)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementCapabilityLinkResults(productID, requirementID, candidates).Render(r.Context(), w)
}

// RequirementCapabilityLink handles POST /products/{id}/requirements/{requirementId}/capabilities/link.
func (c *PageProductController) RequirementCapabilityLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")
	capabilityID := r.FormValue("capability_id")

	if capabilityID == "" {
		http.Error(w, "capability_id is required", http.StatusBadRequest)
		return
	}

	if err := c.component.LinkRequirementToCapability(r.Context(), user, productID, capabilityID, requirementID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeRequirementCapabilitiesTabOOB(w, r, user, productID, requirementID)
}

// RequirementCapabilityCreateModal renders GET /products/{id}/requirements/{requirementId}/capabilities/create-modal.
func (c *PageProductController) RequirementCapabilityCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !user.IsAdmin && role != model.RoleOwner {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityFormModal(productpage.CapabilityFormProps{
		Meta:        templates.PageMeta{CSRFToken: csrfToken},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Components:  components,
		Milestones:  milestones,
		LinkTo:      "requirement:" + requirementID,
	}).Render(r.Context(), w)
}

// RequirementCapabilityCreateAndLink handles POST /products/{id}/requirements/{requirementId}/capabilities.
func (c *PageProductController) RequirementCapabilityCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	componentID := r.FormValue("component_id")
	milestoneID := r.FormValue("milestone_id")

	rerenderForm := func(errMsg string) {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.CapabilityFormModal(productpage.CapabilityFormProps{
			Meta:        templates.PageMeta{CSRFToken: csrfToken},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Components:  components,
			Milestones:  milestones,
			LinkTo:      "requirement:" + requirementID,
			Name:        name,
			Description: description,
			ComponentID: componentID,
			MilestoneID: milestoneID,
			Error:       errMsg,
		}).Render(r.Context(), w)
	}

	cap, err := c.component.CreateCapability(r.Context(), user, service.CreateCapabilityInput{
		ProductID:   productID,
		ComponentID: componentID,
		Name:        name,
		Description: description,
		MilestoneID: milestoneID,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	if err := c.component.LinkRequirementToCapability(r.Context(), user, productID, cap.ID, requirementID); err != nil {
		rerenderForm("created capability but failed to link: " + err.Error())
		return
	}

	c.writeRequirementCapabilitiesTabOOB(w, r, user, productID, requirementID)
}

// RequirementCapabilityUnlink handles DELETE /products/{id}/requirements/{requirementId}/capabilities/{capId}.
func (c *PageProductController) RequirementCapabilityUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")
	capabilityID := r.PathValue("capId")

	if err := c.component.UnlinkRequirementFromCapability(r.Context(), user, productID, capabilityID, requirementID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadRequirementCapabilitiesTabProps(r, user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementCapabilitiesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadRequirementCapabilitiesTabProps(r *http.Request, user *model.User, productID, requirementID string) (productpage.RequirementCapabilitiesTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.RequirementCapabilitiesTabProps{}, err
	}

	capabilities, _ := c.component.ListLinkedCapabilities(r.Context(), user, productID, requirementID)
	components, _ := c.component.ListComponents(r.Context(), user, productID)
	componentIndex := make(map[string]string, len(components))
	for _, comp := range components {
		componentIndex[comp.ID] = comp.Name
	}
	componentNames := make(map[string]string, len(capabilities))
	for _, cap := range capabilities {
		if cap.ComponentID != "" {
			componentNames[cap.ID] = componentIndex[cap.ComponentID]
		}
	}

	return productpage.RequirementCapabilitiesTabProps{
		ProductID:     productID,
		RequirementID: requirementID,
		CSRFToken:     csrfToken,
		CanWrite:      user.IsAdmin || role == model.RoleOwner,
		Capabilities:  capabilities,
		ComponentName: componentNames,
	}, nil
}

func (c *PageProductController) writeRequirementCapabilitiesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, requirementID string) {
	props, err := c.loadRequirementCapabilitiesTabProps(r, user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "capabilities", productpage.RequirementCapabilitiesTabContent(props))
}

func (c *PageProductController) canLinkRequirementCapability(r *http.Request, user *model.User, productID string) bool {
	if user.IsAdmin {
		return true
	}
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return false
	}
	return role == model.RoleOwner
}

// RequirementTestCasesTab renders GET /products/{id}/requirements/{requirementId}/tabs/test-cases.
func (c *PageProductController) RequirementTestCasesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	props, err := c.loadRequirementTestCasesTabProps(r, user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementTestCasesTabContent(props).Render(r.Context(), w)
}

// RequirementTestCaseLinkModal renders GET /products/{id}/requirements/{requirementId}/test-cases/link-modal.
func (c *PageProductController) RequirementTestCaseLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementTestCaseLinkModal(productID, requirementID).Render(r.Context(), w)
}

// RequirementTestCaseLinkSearch renders GET /products/{id}/requirements/{requirementId}/test-cases/link-search.
func (c *PageProductController) RequirementTestCaseLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.testing.ListTestCases(r.Context(), user, productID)
	linked, _ := c.testing.ListLinkedTestCasesForRequirement(r.Context(), user, productID, requirementID)
	linkedIDs := make(map[string]struct{}, len(linked))
	for _, tc := range linked {
		linkedIDs[tc.ID] = struct{}{}
	}

	candidates := make([]*model.TestCase, 0, len(all))
	for _, tc := range all {
		if _, ok := linkedIDs[tc.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(tc.Title), query) {
			continue
		}
		candidates = append(candidates, tc)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementTestCaseLinkResults(productID, requirementID, candidates).Render(r.Context(), w)
}

// RequirementTestCaseLink handles POST /products/{id}/requirements/{requirementId}/test-cases/link.
func (c *PageProductController) RequirementTestCaseLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")
	testCaseID := r.FormValue("test_case_id")

	if testCaseID == "" {
		http.Error(w, "test_case_id is required", http.StatusBadRequest)
		return
	}

	if err := c.testing.LinkRequirement(r.Context(), user, productID, testCaseID, requirementID, "requirement"); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeRequirementTestCasesTabOOB(w, r, user, productID, requirementID)
}

// RequirementTestCaseCreateModal renders GET /products/{id}/requirements/{requirementId}/test-cases/create-modal.
func (c *PageProductController) RequirementTestCaseCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
	pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseFormModal(productpage.TestCaseFormProps{
		Meta:          templates.PageMeta{CSRFToken: csrfToken},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		Components:    components,
		Capabilities:  capabilities,
		PyramidLevels: pyramidLevels,
		LinkTo:        "requirement:" + requirementID,
	}).Render(r.Context(), w)
}

// RequirementTestCaseCreateAndLink handles POST /products/{id}/requirements/{requirementId}/test-cases.
func (c *PageProductController) RequirementTestCaseCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	executionMode := r.FormValue("execution_mode")
	testIdentifier := r.FormValue("test_identifier")
	componentID := r.FormValue("component_id")
	capabilityID := r.FormValue("capability_id")
	pyramidLevelID := r.FormValue("pyramid_level_id")

	rerenderForm := func(errMsg string) {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
		pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.TestCaseFormModal(productpage.TestCaseFormProps{
			Meta:           templates.PageMeta{CSRFToken: csrfToken},
			DisplayName:    user.DisplayName,
			IsAdmin:        user.IsAdmin,
			Product:        p,
			Components:     components,
			Capabilities:   capabilities,
			PyramidLevels:  pyramidLevels,
			LinkTo:         "requirement:" + requirementID,
			Title:          title,
			Description:    description,
			ExecutionMode:  executionMode,
			TestIdentifier: testIdentifier,
			ComponentID:    componentID,
			CapabilityID:   capabilityID,
			PyramidLevelID: pyramidLevelID,
			Error:          errMsg,
		}).Render(r.Context(), w)
	}

	tc, err := c.testing.CreateTestCase(r.Context(), user, service.CreateTestCaseInput{
		ProductID:      productID,
		Title:          title,
		Description:    description,
		ExecutionMode:  executionMode,
		TestIdentifier: testIdentifier,
		ComponentID:    componentID,
		CapabilityID:   capabilityID,
		PyramidLevelID: pyramidLevelID,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	if err := c.testing.LinkRequirement(r.Context(), user, productID, tc.ID, requirementID, "requirement"); err != nil {
		rerenderForm("created test case but failed to link: " + err.Error())
		return
	}

	c.writeRequirementTestCasesTabOOB(w, r, user, productID, requirementID)
}

// RequirementTestCaseUnlink handles DELETE /products/{id}/requirements/{requirementId}/test-cases/{testCaseId}.
func (c *PageProductController) RequirementTestCaseUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")
	testCaseID := r.PathValue("testCaseId")

	if err := c.testing.UnlinkRequirement(r.Context(), user, productID, testCaseID, requirementID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadRequirementTestCasesTabProps(r, user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementTestCasesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadRequirementTestCasesTabProps(r *http.Request, user *model.User, productID, requirementID string) (productpage.RequirementTestCasesTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.RequirementTestCasesTabProps{}, err
	}

	testCases, _ := c.testing.ListLinkedTestCasesForRequirement(r.Context(), user, productID, requirementID)

	return productpage.RequirementTestCasesTabProps{
		ProductID:     productID,
		RequirementID: requirementID,
		CSRFToken:     csrfToken,
		CanWrite:      c.canLinkTestCasesForRole(user, role),
		TestCases:     testCases,
	}, nil
}

func (c *PageProductController) writeRequirementTestCasesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, requirementID string) {
	props, err := c.loadRequirementTestCasesTabProps(r, user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "test-cases", productpage.RequirementTestCasesTabContent(props))
}

func (c *PageProductController) canLinkTestCases(r *http.Request, user *model.User, productID string) bool {
	if user.IsAdmin {
		return true
	}
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return false
	}
	return c.canLinkTestCasesForRole(user, role)
}

func (c *PageProductController) canLinkTestCasesForRole(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner || role == model.RoleDeveloper
}

// RequirementDefectsTab renders GET /products/{id}/requirements/{requirementId}/tabs/defects.
func (c *PageProductController) RequirementDefectsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	props, err := c.loadRequirementDefectsTabProps(r, user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementDefectsTabContent(props).Render(r.Context(), w)
}

// RequirementDefectLinkModal renders GET /products/{id}/requirements/{requirementId}/defects/link-modal.
func (c *PageProductController) RequirementDefectLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementDefectLinkModal(productID, requirementID).Render(r.Context(), w)
}

// RequirementDefectLinkSearch renders GET /products/{id}/requirements/{requirementId}/defects/link-search.
func (c *PageProductController) RequirementDefectLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{})
	linked, _, _ := c.backlog.ListLinkedDefects(r.Context(), user, productID, model.DefectLinkTargetRequirement, requirementID)
	linkedIDs := make(map[string]struct{}, len(linked))
	for _, d := range linked {
		linkedIDs[d.ID] = struct{}{}
	}

	candidates := make([]*model.Defect, 0, len(all))
	for _, d := range all {
		if _, ok := linkedIDs[d.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(d.Title), query) {
			continue
		}
		candidates = append(candidates, d)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementDefectLinkResults(productID, requirementID, candidates).Render(r.Context(), w)
}

// RequirementDefectLink handles POST /products/{id}/requirements/{requirementId}/defects/link.
func (c *PageProductController) RequirementDefectLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")
	defectID := r.FormValue("defect_id")

	if defectID == "" {
		http.Error(w, "defect_id is required", http.StatusBadRequest)
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defectID, model.DefectLinkTargetRequirement, requirementID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeRequirementDefectsTabOOB(w, r, user, productID, requirementID)
}

// RequirementDefectCreateModal renders GET /products/{id}/requirements/{requirementId}/defects/create-modal.
func (c *PageProductController) RequirementDefectCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormModal(productpage.DefectFormProps{
		Meta:        templates.PageMeta{CSRFToken: csrfToken},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Statuses:    statuses,
		Milestones:  milestones,
		Sprints:     sprints,
		LinkTo:      "requirement:" + requirementID,
		Criticality: "medium",
		StatusID:    defaultStatusID(statuses),
	}).Render(r.Context(), w)
}

// RequirementDefectCreateAndLink handles POST /products/{id}/requirements/{requirementId}/defects.
func (c *PageProductController) RequirementDefectCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	criticality := r.FormValue("criticality")
	statusID := r.FormValue("status_id")
	milestoneID := r.FormValue("milestone_id")
	sprintID := r.FormValue("sprint_id")
	estimation := r.FormValue("estimation")
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)

	rerenderForm := func(errMsg string) {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormModal(productpage.DefectFormProps{
			Meta:        templates.PageMeta{CSRFToken: csrfToken},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Statuses:    statuses,
			Milestones:  milestones,
			Sprints:     sprints,
			LinkTo:      "requirement:" + requirementID,
			Title:       title,
			Description: description,
			Criticality: criticality,
			StatusID:    statusID,
			MilestoneID: milestoneID,
			SprintID:    sprintID,
			Estimation:  estimation,
			Priority:    priorityStr,
			Error:       errMsg,
		}).Render(r.Context(), w)
	}

	d, err := c.backlog.CreateDefect(r.Context(), user, service.CreateDefectInput{
		ProductID:   productID,
		Title:       title,
		Description: description,
		Criticality: criticality,
		StatusID:    statusID,
		MilestoneID: milestoneID,
		SprintID:    sprintID,
		Estimation:  estimation,
		Priority:    priority,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, d.ID, model.DefectLinkTargetRequirement, requirementID); err != nil {
		rerenderForm("created defect but failed to link: " + err.Error())
		return
	}

	c.writeRequirementDefectsTabOOB(w, r, user, productID, requirementID)
}

// RequirementDefectUnlink handles DELETE /products/{id}/requirements/{requirementId}/defects/{linkId}.
// Note: the path param is the DefectLink's ID (not the defect's ID), since
// RemoveDefectLink operates on the link row directly.
func (c *PageProductController) RequirementDefectUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")
	linkID := r.PathValue("linkId")

	if err := c.backlog.RemoveDefectLink(r.Context(), user, productID, linkID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadRequirementDefectsTabProps(r, user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementDefectsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadRequirementDefectsTabProps(r *http.Request, user *model.User, productID, requirementID string) (productpage.RequirementDefectsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.RequirementDefectsTabProps{}, err
	}

	defects, linkIDs, _ := c.backlog.ListLinkedDefects(r.Context(), user, productID, model.DefectLinkTargetRequirement, requirementID)

	return productpage.RequirementDefectsTabProps{
		ProductID:     productID,
		RequirementID: requirementID,
		CSRFToken:     csrfToken,
		CanLink:       c.canAddDefectLinksForRole(user, role),
		CanUnlink:     c.canRemoveDefectLinksForRole(user, role),
		Defects:       defects,
		LinkIDs:       linkIDs,
	}, nil
}

func (c *PageProductController) writeRequirementDefectsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, requirementID string) {
	props, err := c.loadRequirementDefectsTabProps(r, user, productID, requirementID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "defects", productpage.RequirementDefectsTabContent(props))
}

func (c *PageProductController) canAddDefectLinks(r *http.Request, user *model.User, productID string) bool {
	if user.IsAdmin {
		return true
	}
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return false
	}
	return c.canAddDefectLinksForRole(user, role)
}

func (c *PageProductController) canAddDefectLinksForRole(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner || role == model.RoleDeveloper || role == model.RoleTester
}

func (c *PageProductController) canRemoveDefectLinksForRole(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner || role == model.RoleDeveloper
}

// RequirementHistoryTab renders GET /products/{id}/requirements/{requirementId}/tabs/history.
func (c *PageProductController) RequirementHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	requirementID := r.PathValue("requirementId")

	entries := c.buildAuditViews(r, user, productID, "requirement", requirementID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.RequirementHistoryTabContent(entries).Render(r.Context(), w)
}

// --- Defect Pages ---

// Defects renders GET /products/{id}/defects.
func (c *PageProductController) Defects(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	defects, _ := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectsPage(productpage.DefectsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Defects",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Defects:     defects,
		Statuses:    c.buildStatusMap(r, user, productID, "defect"),
		CanCreate:   c.canCreateDefects(user, role),
		CanEdit:     c.canEditDefects(user, role),
	}).Render(r.Context(), w)
}

// DefectDetail renders GET /products/{id}/defects/{defectId}.
func (c *PageProductController) DefectDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	d, err := c.backlog.GetDefect(r.Context(), user, productID, defectID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/defects", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectDetailPage(productpage.DefectDetailProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - " + d.Title,
			CSRFToken: csrfToken,
		},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		Defect:        d,
		StatusName:    c.resolveStatusName(r, user, productID, d.StatusID, "defect"),
		MilestoneName: c.resolveMilestoneName(r, user, productID, d.MilestoneID),
		SprintName:    c.resolveSprintName(r, user, productID, d.SprintID),
		CanEdit:       c.canEditDefects(user, role),
	}).Render(r.Context(), w)
}

// DefectCreateForm renders GET /products/{id}/defects/new.
func (c *PageProductController) DefectCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormPage(productpage.DefectFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - New Defect",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Statuses:    statuses,
		Milestones:  milestones,
		Sprints:     sprints,
		Criticality: "medium",
		StatusID:    defaultStatusID(statuses),
		Priority:    "0",
	}).Render(r.Context(), w)
}

// DefectCreate handles POST /products/{id}/defects.
func (c *PageProductController) DefectCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	description := r.FormValue("description")
	criticality := r.FormValue("criticality")
	statusID := r.FormValue("status_id")
	milestoneID := r.FormValue("milestone_id")
	sprintID := r.FormValue("sprint_id")
	estimation := strings.TrimSpace(r.FormValue("estimation"))
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)

	_, createErr := c.backlog.CreateDefect(r.Context(), user, service.CreateDefectInput{
		ProductID:   productID,
		Title:       title,
		Description: description,
		Criticality: criticality,
		StatusID:    statusID,
		MilestoneID: milestoneID,
		SprintID:    sprintID,
		Estimation:  estimation,
		Priority:    priority,
	})
	if createErr != nil {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormPage(productpage.DefectFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - New Defect",
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Statuses:    statuses,
			Milestones:  milestones,
			Sprints:     sprints,
			Error:       createErr.Error(),
			Title:       title,
			Description: description,
			Criticality: criticality,
			StatusID:    statusID,
			MilestoneID: milestoneID,
			SprintID:    sprintID,
			Estimation:  estimation,
			Priority:    priorityStr,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/defects", http.StatusSeeOther)
}

// DefectEditForm renders GET /products/{id}/defects/{defectId}/edit.
func (c *PageProductController) DefectEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	d, err := c.backlog.GetDefect(r.Context(), user, productID, defectID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/defects", http.StatusSeeOther)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormPage(productpage.DefectFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Edit " + d.Title,
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Defect:      d,
		Statuses:    statuses,
		Milestones:  milestones,
		Sprints:     sprints,
		IsEdit:      true,
		ReturnTo:    editReturnTarget(r),
		Title:       d.Title,
		Description: d.Description,
		Criticality: d.Criticality,
		StatusID:    d.StatusID,
		MilestoneID: d.MilestoneID,
		SprintID:    d.SprintID,
		Estimation:  d.Estimation,
		Priority:    fmt.Sprintf("%d", d.Priority),
	}).Render(r.Context(), w)
}

// DefectUpdate handles POST /products/{id}/defects/{defectId}.
func (c *PageProductController) DefectUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	d, err := c.backlog.GetDefect(r.Context(), user, productID, defectID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/defects", http.StatusSeeOther)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	description := r.FormValue("description")
	criticality := r.FormValue("criticality")
	statusID := r.FormValue("status_id")
	milestoneID := r.FormValue("milestone_id")
	sprintID := r.FormValue("sprint_id")
	estimation := strings.TrimSpace(r.FormValue("estimation"))
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)

	_, updateErr := c.backlog.UpdateDefect(r.Context(), user, productID, defectID, service.UpdateDefectInput{
		Title:       &title,
		Description: &description,
		Criticality: &criticality,
		StatusID:    &statusID,
		MilestoneID: &milestoneID,
		SprintID:    &sprintID,
		Estimation:  &estimation,
		Priority:    &priority,
	})
	if updateErr != nil {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormPage(productpage.DefectFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - Edit " + d.Title,
				CSRFToken: csrfToken,
			},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Defect:      d,
			Statuses:    statuses,
			Milestones:  milestones,
			Sprints:     sprints,
			IsEdit:      true,
			ReturnTo:    r.FormValue("return"),
			Error:       updateErr.Error(),
			Title:       title,
			Description: description,
			Criticality: criticality,
			StatusID:    statusID,
			MilestoneID: milestoneID,
			SprintID:    sprintID,
			Estimation:  estimation,
			Priority:    priorityStr,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		"/products/"+productID+"/defects/"+defectID,
		"/products/"+productID+"/defects",
	), http.StatusSeeOther)
}

// DefectDelete handles DELETE /products/{id}/defects/{defectId}.
func (c *PageProductController) DefectDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	_ = c.backlog.DeleteDefect(r.Context(), user, productID, defectID)
	w.Header().Set("HX-Redirect", "/products/"+productID+"/defects")
	w.WriteHeader(http.StatusOK)
}

// DefectDeleteConfirm renders GET /products/{id}/defects/{defectId}/delete/confirm.
func (c *PageProductController) DefectDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	d, err := c.backlog.GetDefect(r.Context(), user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectDeleteConfirmation(d.Title, productID, defectID, middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// DefectOverviewTab renders GET /products/{id}/defects/{defectId}/tabs/overview.
func (c *PageProductController) DefectOverviewTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	d, err := c.backlog.GetDefect(r.Context(), user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectOverviewTabContent(
		productID,
		p.EstimationMode,
		p.WorkflowMode,
		d,
		c.resolveStatusName(r, user, productID, d.StatusID, "defect"),
		c.resolveMilestoneName(r, user, productID, d.MilestoneID),
		c.resolveSprintName(r, user, productID, d.SprintID),
	).Render(r.Context(), w)
}

// --- Defect link tabs (FR-023, FR-501) ---

// DefectComponentsTab renders GET /products/{id}/defects/{defectId}/tabs/components.
func (c *PageProductController) DefectComponentsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	props, err := c.loadDefectComponentsTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectComponentsTabContent(props).Render(r.Context(), w)
}

// DefectComponentLinkModal renders GET /products/{id}/defects/{defectId}/components/link-modal.
func (c *PageProductController) DefectComponentLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectComponentLinkModal(productID, defectID).Render(r.Context(), w)
}

// DefectComponentLinkSearch renders GET /products/{id}/defects/{defectId}/components/link-search.
func (c *PageProductController) DefectComponentLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.component.ListComponents(r.Context(), user, productID)
	linkedIDs := c.defectLinkedTargetIDs(r, user, productID, defectID, model.DefectLinkTargetComponent)

	candidates := make([]*model.Component, 0, len(all))
	for _, comp := range all {
		if _, ok := linkedIDs[comp.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(comp.Name), query) {
			continue
		}
		candidates = append(candidates, comp)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectComponentLinkResults(productID, defectID, candidates).Render(r.Context(), w)
}

// DefectComponentLink handles POST /products/{id}/defects/{defectId}/components/link.
func (c *PageProductController) DefectComponentLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")
	componentID := r.FormValue("component_id")

	if componentID == "" {
		http.Error(w, "component_id is required", http.StatusBadRequest)
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defectID, model.DefectLinkTargetComponent, componentID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeDefectComponentsTabOOB(w, r, user, productID, defectID)
}

// DefectComponentUnlink handles DELETE /products/{id}/defects/{defectId}/components/{linkId}.
// The path parameter is the DefectLink row's ID, not the component's ID.
func (c *PageProductController) DefectComponentUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")
	linkID := r.PathValue("linkId")

	if err := c.backlog.RemoveDefectLink(r.Context(), user, productID, linkID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadDefectComponentsTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectComponentsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadDefectComponentsTabProps(r *http.Request, user *model.User, productID, defectID string) (productpage.DefectComponentsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.DefectComponentsTabProps{}, err
	}

	targetIDs, linkIDs, _ := c.backlog.ListDefectLinkTargets(r.Context(), user, productID, defectID, model.DefectLinkTargetComponent)
	components := make([]*model.Component, 0, len(targetIDs))
	for _, id := range targetIDs {
		comp, err := c.component.GetComponent(r.Context(), user, productID, id)
		if err != nil || comp.ProductID != productID {
			continue
		}
		components = append(components, comp)
	}

	return productpage.DefectComponentsTabProps{
		ProductID:  productID,
		DefectID:   defectID,
		CSRFToken:  csrfToken,
		CanLink:    c.canAddDefectLinksForRole(user, role),
		CanUnlink:  c.canRemoveDefectLinksForRole(user, role),
		Components: components,
		LinkIDs:    linkIDs,
	}, nil
}

func (c *PageProductController) writeDefectComponentsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, defectID string) {
	props, err := c.loadDefectComponentsTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "components", productpage.DefectComponentsTabContent(props))
}

// DefectCapabilitiesTab renders GET /products/{id}/defects/{defectId}/tabs/capabilities.
func (c *PageProductController) DefectCapabilitiesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	props, err := c.loadDefectCapabilitiesTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectCapabilitiesTabContent(props).Render(r.Context(), w)
}

// DefectCapabilityLinkModal renders GET /products/{id}/defects/{defectId}/capabilities/link-modal.
func (c *PageProductController) DefectCapabilityLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectCapabilityLinkModal(productID, defectID).Render(r.Context(), w)
}

// DefectCapabilityLinkSearch renders GET /products/{id}/defects/{defectId}/capabilities/link-search.
func (c *PageProductController) DefectCapabilityLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.component.ListCapabilities(r.Context(), user, productID)
	linkedIDs := c.defectLinkedTargetIDs(r, user, productID, defectID, model.DefectLinkTargetCapability)

	candidates := make([]*model.Capability, 0, len(all))
	for _, cap := range all {
		if _, ok := linkedIDs[cap.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(cap.Name), query) {
			continue
		}
		candidates = append(candidates, cap)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectCapabilityLinkResults(productID, defectID, candidates).Render(r.Context(), w)
}

// DefectCapabilityLink handles POST /products/{id}/defects/{defectId}/capabilities/link.
func (c *PageProductController) DefectCapabilityLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")
	capabilityID := r.FormValue("capability_id")

	if capabilityID == "" {
		http.Error(w, "capability_id is required", http.StatusBadRequest)
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defectID, model.DefectLinkTargetCapability, capabilityID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeDefectCapabilitiesTabOOB(w, r, user, productID, defectID)
}

// DefectCapabilityUnlink handles DELETE /products/{id}/defects/{defectId}/capabilities/{linkId}.
// The path parameter is the DefectLink row's ID, not the capability's ID.
func (c *PageProductController) DefectCapabilityUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")
	linkID := r.PathValue("linkId")

	if err := c.backlog.RemoveDefectLink(r.Context(), user, productID, linkID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadDefectCapabilitiesTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectCapabilitiesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadDefectCapabilitiesTabProps(r *http.Request, user *model.User, productID, defectID string) (productpage.DefectCapabilitiesTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.DefectCapabilitiesTabProps{}, err
	}

	targetIDs, linkIDs, _ := c.backlog.ListDefectLinkTargets(r.Context(), user, productID, defectID, model.DefectLinkTargetCapability)
	capabilities := make([]*model.Capability, 0, len(targetIDs))
	componentNames := make(map[string]string, len(targetIDs))
	for _, id := range targetIDs {
		cap, err := c.component.GetCapability(r.Context(), user, productID, id)
		if err != nil || cap.ProductID != productID {
			continue
		}
		capabilities = append(capabilities, cap)
		if cap.ComponentID != "" {
			if comp, err := c.component.GetComponent(r.Context(), user, productID, cap.ComponentID); err == nil {
				componentNames[cap.ID] = comp.Name
			}
		}
	}

	return productpage.DefectCapabilitiesTabProps{
		ProductID:     productID,
		DefectID:      defectID,
		CSRFToken:     csrfToken,
		CanLink:       c.canAddDefectLinksForRole(user, role),
		CanUnlink:     c.canRemoveDefectLinksForRole(user, role),
		Capabilities:  capabilities,
		ComponentName: componentNames,
		LinkIDs:       linkIDs,
	}, nil
}

func (c *PageProductController) writeDefectCapabilitiesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, defectID string) {
	props, err := c.loadDefectCapabilitiesTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "capabilities", productpage.DefectCapabilitiesTabContent(props))
}

// DefectTestCasesTab renders GET /products/{id}/defects/{defectId}/tabs/test-cases.
func (c *PageProductController) DefectTestCasesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	props, err := c.loadDefectTestCasesTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectTestCasesTabContent(props).Render(r.Context(), w)
}

// DefectTestCaseLinkModal renders GET /products/{id}/defects/{defectId}/test-cases/link-modal.
func (c *PageProductController) DefectTestCaseLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectTestCaseLinkModal(productID, defectID).Render(r.Context(), w)
}

// DefectTestCaseLinkSearch renders GET /products/{id}/defects/{defectId}/test-cases/link-search.
func (c *PageProductController) DefectTestCaseLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.testing.ListTestCases(r.Context(), user, productID)
	linkedIDs := c.defectLinkedTargetIDs(r, user, productID, defectID, model.DefectLinkTargetTestCase)

	candidates := make([]*model.TestCase, 0, len(all))
	for _, tc := range all {
		if _, ok := linkedIDs[tc.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(tc.Title), query) {
			continue
		}
		candidates = append(candidates, tc)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectTestCaseLinkResults(productID, defectID, candidates).Render(r.Context(), w)
}

// DefectTestCaseLink handles POST /products/{id}/defects/{defectId}/test-cases/link.
func (c *PageProductController) DefectTestCaseLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")
	testCaseID := r.FormValue("test_case_id")

	if testCaseID == "" {
		http.Error(w, "test_case_id is required", http.StatusBadRequest)
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defectID, model.DefectLinkTargetTestCase, testCaseID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeDefectTestCasesTabOOB(w, r, user, productID, defectID)
}

// DefectTestCaseUnlink handles DELETE /products/{id}/defects/{defectId}/test-cases/{linkId}.
// The path parameter is the DefectLink row's ID, not the test case's ID.
func (c *PageProductController) DefectTestCaseUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")
	linkID := r.PathValue("linkId")

	if err := c.backlog.RemoveDefectLink(r.Context(), user, productID, linkID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadDefectTestCasesTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectTestCasesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadDefectTestCasesTabProps(r *http.Request, user *model.User, productID, defectID string) (productpage.DefectTestCasesTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.DefectTestCasesTabProps{}, err
	}

	targetIDs, linkIDs, _ := c.backlog.ListDefectLinkTargets(r.Context(), user, productID, defectID, model.DefectLinkTargetTestCase)
	testCases := make([]*model.TestCase, 0, len(targetIDs))
	for _, id := range targetIDs {
		tc, err := c.testing.GetTestCase(r.Context(), user, productID, id)
		if err != nil || tc.ProductID != productID {
			continue
		}
		testCases = append(testCases, tc)
	}

	return productpage.DefectTestCasesTabProps{
		ProductID: productID,
		DefectID:  defectID,
		CSRFToken: csrfToken,
		CanLink:   c.canAddDefectLinksForRole(user, role),
		CanCreate: c.canEditTestCases(user, role),
		CanUnlink: c.canRemoveDefectLinksForRole(user, role),
		TestCases: testCases,
		LinkIDs:   linkIDs,
	}, nil
}

// DefectTestCaseCreateModal renders GET /products/{id}/defects/{defectId}/test-cases/create-modal.
// Creating a test case requires developer role, unlike linking one (tester).
func (c *PageProductController) DefectTestCaseCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canEditTestCases(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
	pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseFormModal(productpage.TestCaseFormProps{
		Meta:          templates.PageMeta{CSRFToken: csrfToken},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		Components:    components,
		Capabilities:  capabilities,
		PyramidLevels: pyramidLevels,
		LinkTo:        "defect:" + defectID,
	}).Render(r.Context(), w)
}

// DefectTestCaseCreateAndLink handles POST /products/{id}/defects/{defectId}/test-cases.
func (c *PageProductController) DefectTestCaseCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canEditTestCases(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	executionMode := r.FormValue("execution_mode")
	testIdentifier := r.FormValue("test_identifier")
	componentID := r.FormValue("component_id")
	capabilityID := r.FormValue("capability_id")
	pyramidLevelID := r.FormValue("pyramid_level_id")

	rerenderForm := func(errMsg string) {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
		pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.TestCaseFormModal(productpage.TestCaseFormProps{
			Meta:           templates.PageMeta{CSRFToken: csrfToken},
			DisplayName:    user.DisplayName,
			IsAdmin:        user.IsAdmin,
			Product:        p,
			Components:     components,
			Capabilities:   capabilities,
			PyramidLevels:  pyramidLevels,
			LinkTo:         "defect:" + defectID,
			Title:          title,
			Description:    description,
			ExecutionMode:  executionMode,
			TestIdentifier: testIdentifier,
			ComponentID:    componentID,
			CapabilityID:   capabilityID,
			PyramidLevelID: pyramidLevelID,
			Error:          errMsg,
		}).Render(r.Context(), w)
	}

	tc, err := c.testing.CreateTestCase(r.Context(), user, service.CreateTestCaseInput{
		ProductID:      productID,
		Title:          title,
		Description:    description,
		ExecutionMode:  executionMode,
		TestIdentifier: testIdentifier,
		ComponentID:    componentID,
		CapabilityID:   capabilityID,
		PyramidLevelID: pyramidLevelID,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defectID, model.DefectLinkTargetTestCase, tc.ID); err != nil {
		rerenderForm("created test case but failed to link: " + err.Error())
		return
	}

	c.writeDefectTestCasesTabOOB(w, r, user, productID, defectID)
}

func (c *PageProductController) writeDefectTestCasesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, defectID string) {
	props, err := c.loadDefectTestCasesTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "test-cases", productpage.DefectTestCasesTabContent(props))
}

// DefectRequirementsTab renders GET /products/{id}/defects/{defectId}/tabs/requirements.
func (c *PageProductController) DefectRequirementsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	props, err := c.loadDefectRequirementsTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectRequirementsTabContent(props).Render(r.Context(), w)
}

// DefectRequirementLinkModal renders GET /products/{id}/defects/{defectId}/requirements/link-modal.
func (c *PageProductController) DefectRequirementLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectRequirementLinkModal(productID, defectID).Render(r.Context(), w)
}

// DefectRequirementLinkSearch renders GET /products/{id}/defects/{defectId}/requirements/link-search.
func (c *PageProductController) DefectRequirementLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.backlog.ListRequirements(r.Context(), user, productID, requirementsOnly)
	linkedIDs := c.defectLinkedTargetIDs(r, user, productID, defectID, model.DefectLinkTargetRequirement)

	candidates := make([]*model.Requirement, 0, len(all))
	for _, req := range all {
		if _, ok := linkedIDs[req.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(req.Title), query) {
			continue
		}
		candidates = append(candidates, req)
	}

	prefixes := c.buildCategoryPrefixMap(r, user, productID, "requirement")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectRequirementLinkResults(productID, defectID, candidates, prefixes).Render(r.Context(), w)
}

// DefectRequirementLink handles POST /products/{id}/defects/{defectId}/requirements/link.
func (c *PageProductController) DefectRequirementLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")
	requirementID := r.FormValue("requirement_id")

	if requirementID == "" {
		http.Error(w, "requirement_id is required", http.StatusBadRequest)
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defectID, model.DefectLinkTargetRequirement, requirementID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeDefectRequirementsTabOOB(w, r, user, productID, defectID)
}

// DefectRequirementUnlink handles DELETE /products/{id}/defects/{defectId}/requirements/{linkId}.
// The path parameter is the DefectLink row's ID, not the requirement's ID.
func (c *PageProductController) DefectRequirementUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")
	linkID := r.PathValue("linkId")

	if err := c.backlog.RemoveDefectLink(r.Context(), user, productID, linkID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadDefectRequirementsTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectRequirementsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadDefectRequirementsTabProps(r *http.Request, user *model.User, productID, defectID string) (productpage.DefectRequirementsTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.DefectRequirementsTabProps{}, err
	}

	targetIDs, linkIDs, _ := c.backlog.ListDefectLinkTargets(r.Context(), user, productID, defectID, model.DefectLinkTargetRequirement)
	requirements := make([]*model.Requirement, 0, len(targetIDs))
	for _, id := range targetIDs {
		req, err := c.backlog.GetRequirement(r.Context(), user, productID, id)
		if err != nil || req.ProductID != productID {
			continue
		}
		requirements = append(requirements, req)
	}

	return productpage.DefectRequirementsTabProps{
		ProductID:        productID,
		DefectID:         defectID,
		CSRFToken:        csrfToken,
		CanLink:          c.canAddDefectLinksForRole(user, role),
		CanUnlink:        c.canRemoveDefectLinksForRole(user, role),
		Requirements:     requirements,
		CategoryPrefixes: c.buildCategoryPrefixMap(r, user, productID, "requirement"),
		LinkIDs:          linkIDs,
	}, nil
}

func (c *PageProductController) writeDefectRequirementsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, defectID string) {
	props, err := c.loadDefectRequirementsTabProps(r, user, productID, defectID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "requirements", productpage.DefectRequirementsTabContent(props))
}

// defectLinkedTargetIDs returns the set of target IDs of the given type already
// linked to the defect, used to exclude them from link-picker candidates.
func (c *PageProductController) defectLinkedTargetIDs(r *http.Request, user *model.User, productID, defectID, targetType string) map[string]struct{} {
	targetIDs, _, _ := c.backlog.ListDefectLinkTargets(r.Context(), user, productID, defectID, targetType)
	set := make(map[string]struct{}, len(targetIDs))
	for _, id := range targetIDs {
		set[id] = struct{}{}
	}
	return set
}

// DefectHistoryTab renders GET /products/{id}/defects/{defectId}/tabs/history.
func (c *PageProductController) DefectHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	entries := c.buildAuditViews(r, user, productID, "defect", defectID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectHistoryTabContent(entries).Render(r.Context(), w)
}

// --- Constraint Pages ---

// canEditConstraints returns true if the user can create/edit/delete constraints (owner or admin).
func (c *PageProductController) canEditConstraints(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner
}

// Constraints renders GET /products/{id}/constraints.
func (c *PageProductController) Constraints(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	constraints, _ := c.constraint.ListConstraints(r.Context(), user, productID, store.ConstraintFilter{})
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "constraint")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintsPage(productpage.ConstraintsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Constraints",
			CSRFToken: csrfToken,
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Constraints:      constraints,
		CategoryPrefixes: prefixes,
		CanEdit:          c.canEditConstraints(user, role),
	}).Render(r.Context(), w)
}

// ConstraintDetail renders GET /products/{id}/constraints/{constraintId}.
func (c *PageProductController) ConstraintDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	con, err := c.constraint.GetConstraint(r.Context(), user, productID, constraintID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/constraints", http.StatusSeeOther)
		return
	}

	prefixes := c.buildCategoryPrefixMap(r, user, productID, "constraint")
	catNames := c.buildCategoryNameMap(r, user, productID, "constraint")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintDetailPage(productpage.ConstraintDetailProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - " + con.Title,
			CSRFToken: csrfToken,
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Constraint:       con,
		CategoryPrefixes: prefixes,
		CategoryNames:    catNames,
		MilestoneName:    c.resolveMilestoneName(r, user, productID, con.MilestoneID),
		CanEdit:          c.canEditConstraints(user, role),
	}).Render(r.Context(), w)
}

// ConstraintCreateForm renders GET /products/{id}/constraints/new.
func (c *PageProductController) ConstraintCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	categories, _ := c.category.ListCategories(r.Context(), user, productID, "constraint")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintFormPage(productpage.ConstraintFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - New Constraint",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Categories:  categories,
		Milestones:  milestones,
		CategoryID:  "",
		Priority:    "0",
	}).Render(r.Context(), w)
}

// ConstraintCreate handles POST /products/{id}/constraints.
func (c *PageProductController) ConstraintCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	description := r.FormValue("description")
	categoryID := r.FormValue("category_id")
	complianceState := r.FormValue("compliance_state")
	milestoneID := r.FormValue("milestone_id")
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)

	_, createErr := c.constraint.CreateConstraint(r.Context(), user, service.CreateConstraintInput{
		ProductID:       productID,
		CategoryID:      categoryID,
		Title:           title,
		Description:     description,
		ComplianceState: complianceState,
		MilestoneID:     milestoneID,
		Priority:        priority,
	})
	if createErr != nil {
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		categories, _ := c.category.ListCategories(r.Context(), user, productID, "constraint")

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.ConstraintFormPage(productpage.ConstraintFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - New Constraint",
				CSRFToken: csrfToken,
			},
			DisplayName:     user.DisplayName,
			IsAdmin:         user.IsAdmin,
			Product:         p,
			Categories:      categories,
			Milestones:      milestones,
			Error:           createErr.Error(),
			Title:           title,
			Description:     description,
			CategoryID:      categoryID,
			ComplianceState: complianceState,
			MilestoneID:     milestoneID,
			Priority:        priorityStr,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/constraints", http.StatusSeeOther)
}

// ConstraintEditForm renders GET /products/{id}/constraints/{constraintId}/edit.
func (c *PageProductController) ConstraintEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	con, err := c.constraint.GetConstraint(r.Context(), user, productID, constraintID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/constraints", http.StatusSeeOther)
		return
	}

	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	categories, _ := c.category.ListCategories(r.Context(), user, productID, "constraint")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintFormPage(productpage.ConstraintFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Edit " + con.Title,
			CSRFToken: csrfToken,
		},
		DisplayName:     user.DisplayName,
		IsAdmin:         user.IsAdmin,
		Product:         p,
		Constraint:      con,
		Categories:      categories,
		Milestones:      milestones,
		IsEdit:          true,
		ReturnTo:        editReturnTarget(r),
		Title:           con.Title,
		Description:     con.Description,
		CategoryID:      con.CategoryID,
		ComplianceState: con.ComplianceState,
		MilestoneID:     con.MilestoneID,
		Priority:        fmt.Sprintf("%d", con.Priority),
	}).Render(r.Context(), w)
}

// ConstraintUpdate handles POST /products/{id}/constraints/{constraintId}.
func (c *PageProductController) ConstraintUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	con, err := c.constraint.GetConstraint(r.Context(), user, productID, constraintID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/constraints", http.StatusSeeOther)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	description := r.FormValue("description")
	categoryID := r.FormValue("category_id")
	complianceState := r.FormValue("compliance_state")
	milestoneID := r.FormValue("milestone_id")
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)

	_, updateErr := c.constraint.UpdateConstraint(r.Context(), user, productID, constraintID, service.UpdateConstraintInput{
		CategoryID:      &categoryID,
		Title:           &title,
		Description:     &description,
		ComplianceState: &complianceState,
		MilestoneID:     &milestoneID,
		Priority:        &priority,
	})
	if updateErr != nil {
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		categories, _ := c.category.ListCategories(r.Context(), user, productID, "constraint")

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.ConstraintFormPage(productpage.ConstraintFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - Edit " + con.Title,
				CSRFToken: csrfToken,
			},
			DisplayName:     user.DisplayName,
			IsAdmin:         user.IsAdmin,
			Product:         p,
			Constraint:      con,
			Categories:      categories,
			Milestones:      milestones,
			IsEdit:          true,
			ReturnTo:        r.FormValue("return"),
			Error:           updateErr.Error(),
			Title:           title,
			Description:     description,
			CategoryID:      categoryID,
			ComplianceState: complianceState,
			MilestoneID:     milestoneID,
			Priority:        priorityStr,
		}).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, postEditRedirect(r,
		"/products/"+productID+"/constraints/"+constraintID,
		"/products/"+productID+"/constraints",
	), http.StatusSeeOther)
}

// ConstraintDelete handles DELETE /products/{id}/constraints/{constraintId}.
func (c *PageProductController) ConstraintDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	if err := c.constraint.DeleteConstraint(r.Context(), user, productID, constraintID); err != nil {
		http.Redirect(w, r, "/products/"+productID+"/constraints", http.StatusSeeOther)
		return
	}

	w.Header().Set("HX-Redirect", "/products/"+productID+"/constraints")
	w.WriteHeader(http.StatusOK)
}

// ConstraintDeleteConfirm renders GET /products/{id}/constraints/{constraintId}/delete/confirm.
func (c *PageProductController) ConstraintDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	con, err := c.constraint.GetConstraint(r.Context(), user, productID, constraintID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintDeleteConfirmation(con.Title, productID, constraintID, middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// ConstraintOverviewTab renders GET /products/{id}/constraints/{constraintId}/tabs/overview.
func (c *PageProductController) ConstraintOverviewTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	con, err := c.constraint.GetConstraint(r.Context(), user, productID, constraintID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	prefixes := c.buildCategoryPrefixMap(r, user, productID, "constraint")
	catNames := c.buildCategoryNameMap(r, user, productID, "constraint")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintOverviewTabContent(productpage.ConstraintDetailProps{
		Product:          p,
		Constraint:       con,
		CategoryPrefixes: prefixes,
		CategoryNames:    catNames,
		MilestoneName:    c.resolveMilestoneName(r, user, productID, con.MilestoneID),
		CanEdit:          c.canEditConstraints(user, role),
	}).Render(r.Context(), w)
}

// --- Constraint → Capability (M:N) ---

func (c *PageProductController) ConstraintCapabilitiesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	props, err := c.loadConstraintCapabilitiesTabProps(r, user, productID, constraintID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintCapabilitiesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) ConstraintCapabilityLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintCapabilityLinkModal(productID, constraintID).Render(r.Context(), w)
}

func (c *PageProductController) ConstraintCapabilityLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.component.ListCapabilities(r.Context(), user, productID)
	links, _ := c.constraint.ListConstraintCapabilities(r.Context(), user, productID, constraintID)
	linkedIDs := make(map[string]struct{}, len(links))
	for _, l := range links {
		linkedIDs[l.CapabilityID] = struct{}{}
	}

	candidates := make([]*model.Capability, 0, len(all))
	for _, cap := range all {
		if _, ok := linkedIDs[cap.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(cap.Name), query) {
			continue
		}
		candidates = append(candidates, cap)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintCapabilityLinkResults(productID, constraintID, candidates).Render(r.Context(), w)
}

func (c *PageProductController) ConstraintCapabilityLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")
	capabilityID := r.FormValue("capability_id")

	if capabilityID == "" {
		http.Error(w, "capability_id is required", http.StatusBadRequest)
		return
	}

	if err := c.constraint.LinkCapability(r.Context(), user, productID, constraintID, capabilityID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	c.writeConstraintCapabilitiesTabOOB(w, r, user, productID, constraintID)
}

func (c *PageProductController) ConstraintCapabilityCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityFormModal(productpage.CapabilityFormProps{
		Meta:        templates.PageMeta{CSRFToken: csrfToken},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Components:  components,
		Milestones:  milestones,
		LinkTo:      "constraint:" + constraintID,
	}).Render(r.Context(), w)
}

func (c *PageProductController) ConstraintCapabilityCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	componentID := r.FormValue("component_id")
	milestoneID := r.FormValue("milestone_id")

	rerenderForm := func(errMsg string) {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.CapabilityFormModal(productpage.CapabilityFormProps{
			Meta:        templates.PageMeta{CSRFToken: csrfToken},
			DisplayName: user.DisplayName,
			IsAdmin:     user.IsAdmin,
			Product:     p,
			Components:  components,
			Milestones:  milestones,
			LinkTo:      "constraint:" + constraintID,
			Name:        name,
			Description: description,
			ComponentID: componentID,
			MilestoneID: milestoneID,
			Error:       errMsg,
		}).Render(r.Context(), w)
	}

	cap, err := c.component.CreateCapability(r.Context(), user, service.CreateCapabilityInput{
		ProductID:   productID,
		ComponentID: componentID,
		Name:        name,
		Description: description,
		MilestoneID: milestoneID,
	})
	if err != nil {
		rerenderForm(err.Error())
		return
	}

	if err := c.constraint.LinkCapability(r.Context(), user, productID, constraintID, cap.ID); err != nil {
		rerenderForm("created capability but failed to link: " + err.Error())
		return
	}

	c.writeConstraintCapabilitiesTabOOB(w, r, user, productID, constraintID)
}

func (c *PageProductController) ConstraintCapabilityUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")
	capabilityID := r.PathValue("capabilityId")

	if err := c.constraint.UnlinkCapability(r.Context(), user, productID, constraintID, capabilityID); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadConstraintCapabilitiesTabProps(r, user, productID, constraintID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintCapabilitiesTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) loadConstraintCapabilitiesTabProps(r *http.Request, user *model.User, productID, constraintID string) (productpage.ConstraintCapabilitiesTabProps, error) {
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.ConstraintCapabilitiesTabProps{}, err
	}

	links, _ := c.constraint.ListConstraintCapabilities(r.Context(), user, productID, constraintID)
	capabilities := make([]*model.Capability, 0, len(links))
	for _, link := range links {
		cap, err := c.component.GetCapability(r.Context(), user, productID, link.CapabilityID)
		if err != nil {
			continue
		}
		capabilities = append(capabilities, cap)
	}

	return productpage.ConstraintCapabilitiesTabProps{
		ProductID:    productID,
		ConstraintID: constraintID,
		CSRFToken:    csrfToken,
		CanWrite:     user.IsAdmin || role == model.RoleOwner,
		Capabilities: capabilities,
	}, nil
}

func (c *PageProductController) writeConstraintCapabilitiesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, constraintID string) {
	props, err := c.loadConstraintCapabilitiesTabProps(r, user, productID, constraintID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "capabilities", productpage.ConstraintCapabilitiesTabContent(props))
}

// ConstraintHistoryTab renders GET /products/{id}/constraints/{constraintId}/tabs/history.
func (c *PageProductController) ConstraintHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")

	entries := c.buildAuditViews(r, user, productID, "constraint", constraintID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintHistoryTabContent(entries).Render(r.Context(), w)
}
