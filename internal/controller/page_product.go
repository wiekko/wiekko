package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
	"nexmap.net/web/templates/patterns"
	"nexmap.net/web/templates/ui"
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
	document   *service.DocumentService
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

// ProductSwitcher renders GET /products/switcher, the header/sidebar product
// switcher panel. It is fetched lazily on first open, so ordinary page renders
// never pay for the product list.
//
// The section query parameter is the caller's ActiveSection; SwitchTargetURL
// resolves it against each target product so switching preserves the user's
// place. Unrecognised sections fall back to the product overview there.
func (c *PageProductController) ProductSwitcher(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	products, err := c.product.ListWithRoles(r.Context(), user)
	if err != nil {
		http.Error(w, "failed to load products", http.StatusInternalServerError)
		return
	}

	sort.Slice(products, func(i, j int) bool {
		return strings.ToLower(products[i].Product.Name) < strings.ToLower(products[j].Product.Name)
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = patterns.ProductSwitcherPanel(patterns.ProductSwitcherProps{
		Products:  products,
		CurrentID: r.URL.Query().Get("current"),
		Section:   r.URL.Query().Get("section"),
		CanCreate: c.canCreateProduct(r, user),
	}).Render(r.Context(), w)
}

// CreateForm renders GET /products/new.
func (c *PageProductController) CreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	if !c.canCreateProduct(r, user) {
		redirectWithNotice(w, r, "/", noticeProductCreationOff)
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
		redirectWithNotice(w, r, "/", noticeProductCreationOff)
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
			Error:          errorText(err),
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
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}

	reqStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}

	var defStatuses []*model.Status
	if !product.StatusSync {
		defStatuses, err = c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		if err != nil {
			http.Error(w, errorText(err), http.StatusInternalServerError)
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
		http.Error(w, errorText(err), http.StatusInternalServerError)
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
		http.Error(w, errorText(err), http.StatusInternalServerError)
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
	isActive := r.FormValue("is_active") == "on"

	_, err := c.milestone.CreateStatus(r.Context(), user, service.CreateStatusInput{
		ProductID:  productID,
		EntityType: entityType,
		Name:       name,
		Position:   position,
		IsDefault:  isDefault,
		IsClosed:   isClosed,
		IsActive:   isActive,
	})
	if err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		if _, ok := r.Form["is_active"]; ok {
			val := r.FormValue("is_active") == "on"
			input.IsActive = &val
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusInternalServerError)
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
			http.Error(w, errorText(err), http.StatusUnprocessableEntity)
			return
		}
		if _, err := c.milestone.UpdateStatus(r.Context(), user, productID, neighbor.ID, service.UpdateStatusInput{Position: &target.Position}); err != nil {
			http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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

// SettingsCategories renders the Categories tab content fragment.
func (c *PageProductController) SettingsCategories(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	// Listing categories only needs viewer, so gate the tab on owner here the way
	// the Settings page handler does — everything on it is owner-only.
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !user.IsAdmin && role != model.RoleOwner {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	reqCats, err := c.category.ListCategories(r.Context(), user, productID, model.RequirementKindRequirement)
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}
	taskCats, err := c.category.ListCategories(r.Context(), user, productID, model.RequirementKindTask)
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}
	conCats, err := c.category.ListCategories(r.Context(), user, productID, "constraint")
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}

	usage, err := c.categoryUsage(r, user, productID)
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CategoriesTabContent(productID, reqCats, taskCats, conCats, usage).Render(r.Context(), w)
}

// CreateCategory handles POST /products/{id}/settings/categories.
func (c *PageProductController) CreateCategory(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	entityType := r.FormValue("entity_type")
	name := r.FormValue("name")
	prefix := r.FormValue("prefix")

	if name == "" || prefix == "" || entityType == "" {
		http.Error(w, "name, prefix and entity_type are required", http.StatusBadRequest)
		return
	}

	// Append to the end of the list.
	existing, _ := c.category.ListCategories(r.Context(), user, productID, entityType)
	position := len(existing) + 1

	_, err := c.category.CreateCategory(r.Context(), user, service.CreateCategoryInput{
		ProductID:  productID,
		EntityType: entityType,
		Name:       name,
		Prefix:     prefix,
		Position:   position,
	})
	if err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	c.renderCategoryList(w, r, user, productID, entityType)
}

// UpdateCategory handles PATCH /products/{id}/settings/categories/{categoryId}.
func (c *PageProductController) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	categoryID := r.PathValue("categoryId")

	// Get the category first to know its entity type for re-rendering.
	cat, err := c.category.GetCategory(r.Context(), user, productID, categoryID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	input := service.UpdateCategoryInput{}
	if v := r.FormValue("name"); v != "" {
		input.Name = &v
	}
	if v := r.FormValue("prefix"); v != "" {
		input.Prefix = &v
	}

	if _, err := c.category.UpdateCategory(r.Context(), user, productID, categoryID, input); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	c.renderCategoryList(w, r, user, productID, cat.EntityType)
}

// DeleteCategory handles DELETE /products/{id}/settings/categories/{categoryId}.
func (c *PageProductController) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	categoryID := r.PathValue("categoryId")

	cat, err := c.category.GetCategory(r.Context(), user, productID, categoryID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := c.category.DeleteCategory(r.Context(), user, productID, categoryID); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	c.renderCategoryList(w, r, user, productID, cat.EntityType)
}

// MoveCategory handles POST /products/{id}/settings/categories/{categoryId}/move.
// Swaps the category with its neighbor based on form field `direction` ("up" or "down").
func (c *PageProductController) MoveCategory(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	categoryID := r.PathValue("categoryId")
	direction := r.FormValue("direction")

	cat, err := c.category.GetCategory(r.Context(), user, productID, categoryID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	cats, err := c.category.ListCategories(r.Context(), user, productID, cat.EntityType)
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}

	idx := -1
	for i, x := range cats {
		if x.ID == categoryID {
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
		if idx >= 0 && idx < len(cats)-1 {
			neighborIdx = idx + 1
		}
	}

	if neighborIdx >= 0 {
		target := cats[idx]
		neighbor := cats[neighborIdx]
		if _, err := c.category.UpdateCategory(r.Context(), user, productID, target.ID, service.UpdateCategoryInput{Position: &neighbor.Position}); err != nil {
			http.Error(w, errorText(err), http.StatusUnprocessableEntity)
			return
		}
		if _, err := c.category.UpdateCategory(r.Context(), user, productID, neighbor.ID, service.UpdateCategoryInput{Position: &target.Position}); err != nil {
			http.Error(w, errorText(err), http.StatusUnprocessableEntity)
			return
		}
	}

	c.renderCategoryList(w, r, user, productID, cat.EntityType)
}

// renderCategoryList re-renders the category list fragment for a given entity type.
func (c *PageProductController) renderCategoryList(w http.ResponseWriter, r *http.Request, user *model.User, productID, entityType string) {
	cats, err := c.category.ListCategories(r.Context(), user, productID, entityType)
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}
	usage, err := c.category.UsageCounts(r.Context(), user, productID, entityType)
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CategoryListFragment(productID, entityType, cats, usage).Render(r.Context(), w)
}

// categoryUsage merges the item counts of all three category sets into one map,
// which is what the tab template expects.
func (c *PageProductController) categoryUsage(r *http.Request, user *model.User, productID string) (map[string]int, error) {
	merged := map[string]int{}
	for _, entityType := range []string{model.RequirementKindRequirement, model.RequirementKindTask, "constraint"} {
		counts, err := c.category.UsageCounts(r.Context(), user, productID, entityType)
		if err != nil {
			return nil, err
		}
		for id, n := range counts {
			merged[id] = n
		}
	}
	return merged, nil
}

// renderStatusList re-renders the status list fragment for a given entity type.
func (c *PageProductController) renderStatusList(w http.ResponseWriter, r *http.Request, user *model.User, productID, entityType string) {
	statuses, err := c.milestone.ListStatuses(r.Context(), user, productID, entityType)
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.StatusListFragment(productID, entityType, statuses).Render(r.Context(), w)
}

// renderSyncedStatusList re-renders the synced status list fragment.
func (c *PageProductController) renderSyncedStatusList(w http.ResponseWriter, r *http.Request, user *model.User, productID string) {
	statuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	// Re-render the full statuses tab content in split mode.
	reqStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
		return
	}
	defStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	c.renderEnvironmentList(w, r, user, productID)
}

// renderEnvironmentList re-renders the environment list fragment.
func (c *PageProductController) renderEnvironmentList(w http.ResponseWriter, r *http.Request, user *model.User, productID string) {
	envs, err := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	c.renderPyramidList(w, r, user, productID)
}

// renderPyramidList re-renders the pyramid level list fragment.
func (c *PageProductController) renderPyramidList(w http.ResponseWriter, r *http.Request, user *model.User, productID string) {
	levels, err := c.milestone.ListPyramidLevels(r.Context(), user, productID)
	if err != nil {
		http.Error(w, errorText(err), http.StatusInternalServerError)
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

	input := service.UpdateProductInput{IfUpdatedAt: r.FormValue("updated_at")}
	switch field {
	case "name":
		input.Name = &value
	case "description":
		input.Description = &value
	case "estimation_mode":
		input.EstimationMode = &value
	case "workflow_mode":
		input.WorkflowMode = &value
	case "defect_prefix":
		input.DefectPrefix = &value
	case "document_prefix":
		input.DocumentPrefix = &value
	case "two_factor_required":
		on := value == "true"
		input.TwoFactorRequired = &on
	default:
		http.Error(w, "unknown field", http.StatusBadRequest)
		return
	}

	updated, err := c.product.Update(r.Context(), user, productID, input)
	if err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		MailEnabled:    c.invitation.MailEnabled(),
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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

	// The search reaches every account on the instance, so it is only for
	// those who may add members: the product's owners and administrators.
	// Without this check any signed-in user could list all users.
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil || !(user.IsAdmin || role == model.RoleOwner) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if len(query) < 2 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return
	}

	// Get current members to exclude.
	members, err := c.product.ListMembers(r.Context(), user, productID)
	if err != nil {
		writePageServiceError(w, err)
		return
	}
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
	mailEnabled := c.invitation.MailEnabled()

	email := r.FormValue("email")
	role := r.FormValue("role")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if email == "" || role == "" {
		_ = productpage.InviteFormOOB(productID, "Email and role are required.", mailEnabled).Render(r.Context(), w)
		return
	}
	delivery, ok := invitationDelivery(r.FormValue("delivery"))
	if !ok {
		_ = productpage.InviteFormOOB(productID, "Choose how to deliver the invitation.", mailEnabled).Render(r.Context(), w)
		return
	}

	if delivery == service.InvitationDeliveryEmail {
		if _, err := c.invitation.Send(r.Context(), user, email, productID, role); err != nil {
			_ = productpage.InviteFormOOB(productID, invitationErrorText(err), mailEnabled).Render(r.Context(), w)
			return
		}
		_ = productpage.InviteEmailSentOOB(productID, strings.TrimSpace(email)).Render(r.Context(), w)
		return
	}

	result, err := c.invitation.Create(r.Context(), user, email, productID, role)
	if err != nil {
		_ = productpage.InviteFormOOB(productID, errorText(err), mailEnabled).Render(r.Context(), w)
		return
	}
	inviteURL := c.invitation.AcceptURL(result.RawToken, httputil.RequestBaseURL(r))
	_ = productpage.InviteSuccessOOB(productID, inviteURL).Render(r.Context(), w)
}

// InviteFormReset handles GET /products/{id}/team/invite-form (resets the invite form).
func (c *PageProductController) InviteFormReset(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.InviteForm(productID, "", c.invitation.MailEnabled()).Render(r.Context(), w)
}

// DeleteProduct handles DELETE /products/{id}.
func (c *PageProductController) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	if err := c.product.Delete(r.Context(), user, productID); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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

	requires2FA := false
	if p, err := c.product.GetByID(r.Context(), user, productID); err == nil {
		requires2FA = p.TwoFactorRequired
	}

	views := make([]productpage.TeamMemberView, 0, len(members))
	for _, m := range members {
		displayName := m.UserID
		username := ""
		missing2FA := false
		if u, ok := userMap[m.UserID]; ok {
			displayName = u.DisplayName
			username = u.Username
			missing2FA = requires2FA && !u.TOTPEnabled
		}
		views = append(views, productpage.TeamMemberView{
			UserID:      m.UserID,
			DisplayName: displayName,
			Username:    username,
			Role:        m.Role,
			IsOwner:     m.Role == model.RoleOwner,

			MissingTwoFactor: missing2FA,
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
		TabCounts:      c.milestoneTabCounts(r, user, p.ID, m.ID),
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
		redirectWithNotice(w, r, "/products/"+productID+"/milestones", noticeNeedsOwner)
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
			Error:       errorText(err),
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
		redirectWithNotice(w, r, fmt.Sprintf("/products/%s/milestones/%s", productID, milestoneID), noticeNeedsOwner)
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
		IfUpdatedAt: r.FormValue("updated_at"),
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
			Error:       errorText(err),
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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

// --- Milestone link tabs (FR-19, FR-31, FR-35) ---
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
	_, affected := c.milestoneAffected(r, user, productID, milestoneID)

	return productpage.MilestoneComponentsTabProps{
		ProductID:   productID,
		MilestoneID: milestoneID,
		CSRFToken:   csrfToken,
		CanWrite:    c.canEditMilestoneLinks(user, role),
		Components:  items,
		Affected:    affected,
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
	affected, _ := c.milestoneAffected(r, user, productID, milestoneID)

	return productpage.MilestoneCapabilitiesTabProps{
		ProductID:    productID,
		MilestoneID:  milestoneID,
		CSRFToken:    csrfToken,
		CanWrite:     c.canEditMilestoneLinks(user, role),
		Capabilities: items,
		Affected:     affected,
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		if query != "" && !strings.Contains(strings.ToLower(d.Title), query) && !strings.Contains(strings.ToLower(d.Identifier), query) {
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &mid,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		MilestoneID: &empty,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		TabCounts:      c.componentTabCounts(r, user, productID, comp.ID),
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
		redirectWithNotice(w, r, "/products/"+productID+"/components", noticeNeedsOwner)
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
			Error:       errorText(err),
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
		redirectWithNotice(w, r, fmt.Sprintf("/products/%s/components/%s", productID, componentID), noticeNeedsOwner)
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
		IfUpdatedAt: r.FormValue("updated_at"),
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
			Error:       errorText(err),
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
	_ = productpage.ComponentOverviewTabContent(productID, comp, milestoneName).Render(r.Context(), w)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		ComponentID: &newCompID,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		rerenderForm(errorText(err))
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
		IfUpdatedAt: r.FormValue("updated_at"),
		ComponentID: &empty,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt: r.FormValue("updated_at"),
		ComponentID: &newCompID,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		rerenderForm(errorText(err))
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
		IfUpdatedAt: r.FormValue("updated_at"),
		ComponentID: &empty,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		TabCounts:      c.capabilityTabCounts(r, user, productID, f.ID),
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
		redirectWithNotice(w, r, "/products/"+productID+"/capabilities", noticeNeedsOwner)
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
			Error:       errorText(err),
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
		redirectWithNotice(w, r, fmt.Sprintf("/products/%s/capabilities/%s", productID, capabilityID), noticeNeedsOwner)
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
		IfUpdatedAt: r.FormValue("updated_at"),
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
			Error:       errorText(err),
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		rerenderForm(errorText(err))
		return
	}

	if err := c.component.LinkRequirementToCapability(r.Context(), user, productID, capabilityID, req.ID); err != nil {
		rerenderForm("created requirement but failed to link: " + errorText(err))
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		IfUpdatedAt:  r.FormValue("updated_at"),
		CapabilityID: &newCapID,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		rerenderForm(errorText(err))
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
		IfUpdatedAt:  r.FormValue("updated_at"),
		CapabilityID: &empty,
	}); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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

// --- Capability and component → Defect (polymorphic via DefectLink) ---

// defectLinkHost is a structure item whose page has a Defects tab: a
// capability, or a component (UR-159). The handlers below serve both.
type defectLinkHost struct {
	target  string // model.DefectLinkTarget*
	param   string // path value naming the item
	segment string // URL segment, e.g. "capabilities"
	noun    string // "capability" or "component"; also the defect form's link_to prefix
}

var (
	capabilityDefectHost = defectLinkHost{model.DefectLinkTargetCapability, "capabilityId", "capabilities", "capability"}
	componentDefectHost  = defectLinkHost{model.DefectLinkTargetComponent, "componentId", "components", "component"}
)

func (h defectLinkHost) baseURL(productID, id string) string {
	return fmt.Sprintf("/products/%s/%s/%s", productID, h.segment, id)
}

// CapabilityDefectsTab renders GET /products/{id}/capabilities/{capabilityId}/tabs/defects.
func (c *PageProductController) CapabilityDefectsTab(w http.ResponseWriter, r *http.Request) {
	c.hostDefectsTab(w, r, capabilityDefectHost)
}
func (c *PageProductController) CapabilityDefectLinkModal(w http.ResponseWriter, r *http.Request) {
	c.hostDefectLinkModal(w, r, capabilityDefectHost)
}
func (c *PageProductController) CapabilityDefectLinkSearch(w http.ResponseWriter, r *http.Request) {
	c.hostDefectLinkSearch(w, r, capabilityDefectHost)
}
func (c *PageProductController) CapabilityDefectLink(w http.ResponseWriter, r *http.Request) {
	c.hostDefectLink(w, r, capabilityDefectHost)
}
func (c *PageProductController) CapabilityDefectCreateModal(w http.ResponseWriter, r *http.Request) {
	c.hostDefectCreateModal(w, r, capabilityDefectHost)
}
func (c *PageProductController) CapabilityDefectCreateAndLink(w http.ResponseWriter, r *http.Request) {
	c.hostDefectCreateAndLink(w, r, capabilityDefectHost)
}
func (c *PageProductController) CapabilityDefectUnlink(w http.ResponseWriter, r *http.Request) {
	c.hostDefectUnlink(w, r, capabilityDefectHost)
}

// ComponentDefectsTab renders GET /products/{id}/components/{componentId}/tabs/defects:
// the defects linked to the component itself (UR-159).
func (c *PageProductController) ComponentDefectsTab(w http.ResponseWriter, r *http.Request) {
	c.hostDefectsTab(w, r, componentDefectHost)
}
func (c *PageProductController) ComponentDefectLinkModal(w http.ResponseWriter, r *http.Request) {
	c.hostDefectLinkModal(w, r, componentDefectHost)
}
func (c *PageProductController) ComponentDefectLinkSearch(w http.ResponseWriter, r *http.Request) {
	c.hostDefectLinkSearch(w, r, componentDefectHost)
}
func (c *PageProductController) ComponentDefectLink(w http.ResponseWriter, r *http.Request) {
	c.hostDefectLink(w, r, componentDefectHost)
}
func (c *PageProductController) ComponentDefectCreateModal(w http.ResponseWriter, r *http.Request) {
	c.hostDefectCreateModal(w, r, componentDefectHost)
}
func (c *PageProductController) ComponentDefectCreateAndLink(w http.ResponseWriter, r *http.Request) {
	c.hostDefectCreateAndLink(w, r, componentDefectHost)
}
func (c *PageProductController) ComponentDefectUnlink(w http.ResponseWriter, r *http.Request) {
	c.hostDefectUnlink(w, r, componentDefectHost)
}

func (c *PageProductController) hostDefectsTab(w http.ResponseWriter, r *http.Request, h defectLinkHost) {
	user := middleware.UserFromContext(r.Context())
	props, err := c.loadHostDefectsTabProps(r, user, r.PathValue("id"), r.PathValue(h.param), h)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.HostDefectsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) hostDefectLinkModal(w http.ResponseWriter, r *http.Request, h defectLinkHost) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.HostDefectLinkModal(h.baseURL(productID, r.PathValue(h.param))).Render(r.Context(), w)
}

func (c *PageProductController) hostDefectLinkSearch(w http.ResponseWriter, r *http.Request, h defectLinkHost) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	hostID := r.PathValue(h.param)

	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{})
	linked, _, _ := c.backlog.ListLinkedDefects(r.Context(), user, productID, h.target, hostID)
	linkedIDs := make(map[string]struct{}, len(linked))
	for _, d := range linked {
		linkedIDs[d.ID] = struct{}{}
	}

	candidates := make([]*model.Defect, 0, len(all))
	for _, d := range all {
		if _, ok := linkedIDs[d.ID]; ok {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(d.Title), query) && !strings.Contains(strings.ToLower(d.Identifier), query) {
			continue
		}
		candidates = append(candidates, d)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.HostDefectLinkResults(h.baseURL(productID, hostID), candidates).Render(r.Context(), w)
}

func (c *PageProductController) hostDefectLink(w http.ResponseWriter, r *http.Request, h defectLinkHost) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	hostID := r.PathValue(h.param)
	defectID := r.FormValue("defect_id")

	if defectID == "" {
		http.Error(w, "defect_id is required", http.StatusBadRequest)
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defectID, h.target, hostID); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	c.writeHostDefectsTabOOB(w, r, user, productID, hostID, h)
}

func (c *PageProductController) hostDefectFormProps(r *http.Request, user *model.User, p *model.Product, h defectLinkHost, hostID string) productpage.DefectFormProps {
	productID := p.ID
	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
	environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
	return productpage.DefectFormProps{
		Meta:         templates.PageMeta{CSRFToken: middleware.CSRFTokenFromContext(r.Context())},
		DisplayName:  user.DisplayName,
		IsAdmin:      user.IsAdmin,
		Product:      p,
		Statuses:     statuses,
		Milestones:   milestones,
		Sprints:      sprints,
		Environments: environments,
		LinkTo:       h.noun + ":" + hostID,
	}
}

func (c *PageProductController) hostDefectCreateModal(w http.ResponseWriter, r *http.Request, h defectLinkHost) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canAddDefectLinks(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	props := c.hostDefectFormProps(r, user, p, h, r.PathValue(h.param))
	props.Criticality = "medium"
	props.StatusID = defaultStatusID(props.Statuses)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormModal(props).Render(r.Context(), w)
}

func (c *PageProductController) hostDefectCreateAndLink(w http.ResponseWriter, r *http.Request, h defectLinkHost) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	hostID := r.PathValue(h.param)

	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)
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

	rerenderForm := func(errMsg string) {
		props := c.hostDefectFormProps(r, user, p, h, hostID)
		props.Title = input.Title
		props.Description = input.Description
		props.Criticality = input.Criticality
		props.StatusID = input.StatusID
		props.MilestoneID = input.MilestoneID
		props.SprintID = input.SprintID
		props.EnvironmentID = input.EnvironmentID
		props.Estimation = input.Estimation
		props.Priority = priorityStr
		props.Error = errMsg
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormModal(props).Render(r.Context(), w)
	}

	d, err := c.backlog.CreateDefect(r.Context(), user, input)
	if err != nil {
		rerenderForm(errorText(err))
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, d.ID, h.target, hostID); err != nil {
		rerenderForm("created defect but failed to link: " + errorText(err))
		return
	}

	c.writeHostDefectsTabOOB(w, r, user, productID, hostID, h)
}

func (c *PageProductController) hostDefectUnlink(w http.ResponseWriter, r *http.Request, h defectLinkHost) {
	user := middleware.UserFromContext(r.Context())
	if err := c.backlog.RemoveDefectLink(r.Context(), user, r.PathValue("id"), r.PathValue("linkId")); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	c.hostDefectsTab(w, r, h)
}

func (c *PageProductController) loadHostDefectsTabProps(r *http.Request, user *model.User, productID, hostID string, h defectLinkHost) (productpage.HostDefectsTabProps, error) {
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.HostDefectsTabProps{}, err
	}

	defects, linkIDs, _ := c.backlog.ListLinkedDefects(r.Context(), user, productID, h.target, hostID)
	var derived []productpage.DerivedDefect
	if h.target == model.DefectLinkTargetComponent {
		derived = c.componentDefectsViaCapabilities(r, user, productID, hostID, defects)
	}

	return productpage.HostDefectsTabProps{
		BaseURL:   h.baseURL(productID, hostID),
		ProductID: productID,
		Noun:      h.noun,
		CanLink:   c.canAddDefectLinksForRole(user, role),
		CanUnlink: c.canRemoveDefectLinksForRole(user, role),
		Defects:   defects,
		LinkIDs:   linkIDs,
		Derived:   derived,
	}, nil
}

func (c *PageProductController) writeHostDefectsTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, hostID string, h defectLinkHost) {
	props, err := c.loadHostDefectsTabProps(r, user, productID, hostID, h)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "defects", productpage.HostDefectsTabContent(props))
}

// --- Capability → Constraint (M:N reverse, UR-159) ---

func (c *PageProductController) CapabilityConstraintsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	props, err := c.loadCapabilityConstraintsTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityConstraintsTabContent(props).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityConstraintLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityConstraintLinkModal(productID, capabilityID).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityConstraintLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")

	if !c.canLinkRequirementCapability(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "constraint")
	all, _ := c.constraint.ListConstraints(r.Context(), user, productID, store.ConstraintFilter{})
	linked, _ := c.constraint.ListCapabilityConstraints(r.Context(), user, productID, capabilityID)
	linkedIDs := make(map[string]struct{}, len(linked))
	for _, con := range linked {
		linkedIDs[con.ID] = struct{}{}
	}

	candidates := make([]*model.Constraint, 0, len(all))
	for _, con := range all {
		if _, ok := linkedIDs[con.ID]; ok {
			continue
		}
		identifier := strings.ToLower(model.EntityIdentifier(prefixes[con.CategoryID], con.SequenceNumber))
		if query != "" && !strings.Contains(strings.ToLower(con.Title), query) && !strings.Contains(identifier, query) {
			continue
		}
		candidates = append(candidates, con)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.CapabilityConstraintLinkResults(productID, capabilityID, candidates, prefixes).Render(r.Context(), w)
}

func (c *PageProductController) CapabilityConstraintLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")
	constraintID := r.FormValue("constraint_id")

	if constraintID == "" {
		http.Error(w, "constraint_id is required", http.StatusBadRequest)
		return
	}

	if err := c.constraint.LinkCapability(r.Context(), user, productID, constraintID, capabilityID); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	props, err := c.loadCapabilityConstraintsTabProps(r, user, productID, capabilityID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "constraints", productpage.CapabilityConstraintsTabContent(props))
}

func (c *PageProductController) CapabilityConstraintUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	capabilityID := r.PathValue("capabilityId")
	constraintID := r.PathValue("constraintId")

	if err := c.constraint.UnlinkCapability(r.Context(), user, productID, constraintID, capabilityID); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}

	c.CapabilityConstraintsTab(w, r)
}

func (c *PageProductController) loadCapabilityConstraintsTabProps(r *http.Request, user *model.User, productID, capabilityID string) (productpage.CapabilityConstraintsTabProps, error) {
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.CapabilityConstraintsTabProps{}, err
	}
	constraints, err := c.constraint.ListCapabilityConstraints(r.Context(), user, productID, capabilityID)
	if err != nil {
		return productpage.CapabilityConstraintsTabProps{}, err
	}
	return productpage.CapabilityConstraintsTabProps{
		ProductID:        productID,
		CapabilityID:     capabilityID,
		CSRFToken:        middleware.CSRFTokenFromContext(r.Context()),
		CanWrite:         user.IsAdmin || role == model.RoleOwner,
		Constraints:      constraints,
		CategoryPrefixes: c.buildCategoryPrefixMap(r, user, productID, "constraint"),
	}, nil
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
		name := userName(userMap, e.UserID)
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
		Capabilities:  c.buildCapabilityMap(r, user, productID),
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
		EnvironmentNames: c.resolveTestCaseEnvironmentNames(r, user, productID, testCaseID),
		CanEdit:          c.canEditTestCases(user, role),
		TabCounts:        c.testCaseTabCounts(r, user, productID, testCaseID),
	}).Render(r.Context(), w)
}

// resolveTestCaseEnvironmentNames names a test case's linked environments. It
// walks the product's environment list rather than the link list so the result
// comes out in the same order the pickers and matrix columns use.
func (c *PageProductController) resolveTestCaseEnvironmentNames(r *http.Request, user *model.User, productID, testCaseID string) []string {
	linked, err := c.testing.ListTestCaseEnvironments(r.Context(), user, productID, testCaseID)
	if err != nil || len(linked) == 0 {
		return nil
	}

	isLinked := make(map[string]bool, len(linked))
	for _, id := range linked {
		isLinked[id] = true
	}

	envs, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
	names := make([]string, 0, len(linked))
	for _, env := range envs {
		if isLinked[env.ID] {
			names = append(names, env.Name)
		}
	}
	return names
}

// TestCaseCreateForm renders GET /products/{id}/test-cases/new.
func (c *PageProductController) TestCaseCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !c.canEditTestCases(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/test-cases", noticeNeedsDeveloper)
		return
	}

	components, _ := c.component.ListComponents(r.Context(), user, productID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
	pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
	environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

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
		Environments:  environments,
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

	// The environment picker submits repeated values, which needs the parsed form
	// rather than FormValue.
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	executionMode := r.FormValue("execution_mode")
	testIdentifier := r.FormValue("test_identifier")
	componentID := r.FormValue("component_id")
	capabilityID := r.FormValue("capability_id")
	pyramidLevelID := r.FormValue("pyramid_level_id")
	environmentIDs := r.Form["environment_ids"]

	rerender := func(message string) {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
		pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
		environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

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
			Environments:   environments,
			Error:          message,
			Title:          title,
			Description:    description,
			ExecutionMode:  executionMode,
			TestIdentifier: testIdentifier,
			ComponentID:    componentID,
			CapabilityID:   capabilityID,
			PyramidLevelID: pyramidLevelID,
			EnvironmentIDs: environmentIDs,
		}).Render(r.Context(), w)
	}

	tc, err := c.testing.CreateTestCase(r.Context(), user, service.CreateTestCaseInput{
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
		rerender(errorText(err))
		return
	}

	if err := c.syncTestCaseEnvironments(r, user, productID, tc.ID, environmentIDs); err != nil {
		rerender("created the test case but failed to link its environments: " + errorText(err))
		return
	}

	http.Redirect(w, r, "/products/"+productID+"/test-cases", http.StatusSeeOther)
}

// syncTestCaseEnvironments reconciles a test case's environment links with the
// submitted selection, adding and removing only what changed. The links decide
// how many matrix columns a campaign seeds for this test case (FR-25).
func (c *PageProductController) syncTestCaseEnvironments(r *http.Request, user *model.User, productID, testCaseID string, want []string) error {
	current, err := c.testing.ListTestCaseEnvironments(r.Context(), user, productID, testCaseID)
	if err != nil {
		return err
	}

	wanted := make(map[string]bool, len(want))
	for _, id := range want {
		wanted[id] = true
	}
	existing := make(map[string]bool, len(current))
	for _, id := range current {
		existing[id] = true
	}

	for id := range wanted {
		if existing[id] {
			continue
		}
		if err := c.testing.AddTestCaseEnvironment(r.Context(), user, productID, testCaseID, id); err != nil {
			return err
		}
	}
	for id := range existing {
		if wanted[id] {
			continue
		}
		if err := c.testing.RemoveTestCaseEnvironment(r.Context(), user, productID, testCaseID, id); err != nil {
			return err
		}
	}
	return nil
}

// TestCaseEditForm renders GET /products/{id}/test-cases/{testCaseId}/edit.
func (c *PageProductController) TestCaseEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !c.canEditTestCases(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/test-cases/"+testCaseID, noticeNeedsDeveloper)
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
	environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
	environmentIDs, _ := c.testing.ListTestCaseEnvironments(r.Context(), user, productID, testCaseID)

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
		Environments:   environments,
		IsEdit:         true,
		ReturnTo:       editReturnTarget(r),
		Title:          tc.Title,
		Description:    tc.Description,
		ExecutionMode:  tc.ExecutionMode,
		TestIdentifier: tc.TestIdentifier,
		ComponentID:    tc.ComponentID,
		CapabilityID:   tc.CapabilityID,
		PyramidLevelID: tc.PyramidLevelID,
		EnvironmentIDs: environmentIDs,
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

	// The environment picker submits repeated values, which needs the parsed form
	// rather than FormValue.
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	executionMode := r.FormValue("execution_mode")
	testIdentifier := r.FormValue("test_identifier")
	componentID := r.FormValue("component_id")
	capabilityID := r.FormValue("capability_id")
	pyramidLevelID := r.FormValue("pyramid_level_id")
	environmentIDs := r.Form["environment_ids"]

	rerender := func(message string) {
		components, _ := c.component.ListComponents(r.Context(), user, productID)
		capabilities, _ := c.component.ListCapabilities(r.Context(), user, productID)
		pyramidLevels, _ := c.milestone.ListPyramidLevels(r.Context(), user, productID)
		environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

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
			Environments:   environments,
			IsEdit:         true,
			ReturnTo:       r.FormValue("return"),
			Error:          message,
			Title:          title,
			Description:    description,
			ExecutionMode:  executionMode,
			TestIdentifier: testIdentifier,
			ComponentID:    componentID,
			CapabilityID:   capabilityID,
			PyramidLevelID: pyramidLevelID,
			EnvironmentIDs: environmentIDs,
		}).Render(r.Context(), w)
	}

	_, err = c.testing.UpdateTestCase(r.Context(), user, productID, testCaseID, service.UpdateTestCaseInput{
		IfUpdatedAt:    r.FormValue("updated_at"),
		Title:          &title,
		Description:    &description,
		ExecutionMode:  &executionMode,
		TestIdentifier: &testIdentifier,
		ComponentID:    &componentID,
		CapabilityID:   &capabilityID,
		PyramidLevelID: &pyramidLevelID,
	})
	if err != nil {
		rerender(errorText(err))
		return
	}

	if err := c.syncTestCaseEnvironments(r, user, productID, testCaseID, environmentIDs); err != nil {
		rerender("saved the test case but failed to update its environments: " + errorText(err))
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
	environmentNames := c.resolveTestCaseEnvironmentNames(r, user, productID, testCaseID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseOverviewTabContent(productID, tc, componentName, capabilityName,
		pyramidLevelName, environmentNames).Render(r.Context(), w)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		rerenderForm(errorText(err))
		return
	}

	if err := c.testing.LinkRequirement(r.Context(), user, productID, testCaseID, req.ID, "requirement"); err != nil {
		rerenderForm("created requirement but failed to link: " + errorText(err))
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
			Environment:  resultEnvironment(result, envNames),
			ExecutedBy:   c.resolveUserDisplayName(r, user, result.ExecutedBy),
		}
		if result.DefectID != "" {
			if d, err := c.backlog.GetDefect(r.Context(), user, productID, result.DefectID); err == nil {
				view.DefectTitle = defectLabel(d)
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

// buildStatusMap returns a map of status ID → status for the given entity type.
// The list pages need more than the name: they colour the status badge from its
// place in the workflow, which only the status itself knows.
func (c *PageProductController) buildStatusMap(r *http.Request, user *model.User, productID, entityType string) map[string]*model.Status {
	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, entityType)
	m := make(map[string]*model.Status, len(statuses))
	for _, s := range statuses {
		m[s.ID] = s
	}
	return m
}

// resolveStatus returns the status with the given ID for the entity type, or
// nil when it is unset or no longer configured. Detail pages badge the status
// by its place in the workflow, so they need the status and not just its name.
func (c *PageProductController) resolveStatus(r *http.Request, user *model.User, productID, statusID, entityType string) *model.Status {
	if statusID == "" {
		return nil
	}
	statuses, err := c.milestone.ListStatuses(r.Context(), user, productID, entityType)
	if err != nil {
		return nil
	}
	for _, s := range statuses {
		if s.ID == statusID {
			return s
		}
	}
	return nil
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

// deleteChildren lists the items below a requirement or task for its delete
// confirmation, and names where kept children would move: the item's parent
// or the top level.
func (c *PageProductController) deleteChildren(r *http.Request, user *model.User, productID string, item *model.Requirement) ([]patterns.ChildItem, string) {
	descendants, err := c.backlog.ListDescendants(r.Context(), user, productID, item.ID)
	if err != nil || len(descendants) == 0 {
		return nil, ""
	}
	prefixes := c.buildBacklogPrefixMap(r, user, productID)
	children := make([]patterns.ChildItem, 0, len(descendants))
	// Deepest first from the service; list them top-down instead.
	for i := len(descendants) - 1; i >= 0; i-- {
		d := descendants[i]
		children = append(children, patterns.ChildItem{
			Identifier: model.EntityIdentifier(prefixes[d.CategoryID], d.SequenceNumber), Title: d.Title,
		})
	}
	keepTarget := "the top level"
	if item.ParentID != "" {
		if parent, err := c.backlog.GetRequirement(r.Context(), user, productID, item.ParentID); err == nil {
			keepTarget = model.EntityIdentifier(prefixes[parent.CategoryID], parent.SequenceNumber) + " " + parent.Title
		}
	}
	return children, keepTarget
}

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
	// (FR-100), so the children table has to resolve task prefixes too.
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
		Status:           c.resolveStatus(r, user, productID, req.StatusID, "requirement"),
		MilestoneName:    c.resolveMilestoneName(r, user, productID, req.MilestoneID),
		SprintName:       c.resolveSprintName(r, user, productID, req.SprintID),
		ParentName:       parentName,
		ParentIdentifier: parentIdentifier,
		Children:         children,
		CanEdit:          c.canEditRequirements(user, role),
		TabCounts:        c.requirementTabCounts(r, user, productID, req.ID),
	}).Render(r.Context(), w)
}

// RequirementCreateForm renders GET /products/{id}/requirements/new.
func (c *PageProductController) RequirementCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !c.canEditRequirements(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/requirements", noticeNeedsOwner)
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
			Error:            errorText(createErr),
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

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !c.canEditRequirements(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/requirements/"+requirementID, noticeNeedsOwner)
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
		IfUpdatedAt:      r.FormValue("updated_at"),
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
			Error:            errorText(updateErr),
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

	policy, _ := service.ParseChildPolicy(r.FormValue("children"))
	_ = c.backlog.DeleteRequirementWith(r.Context(), user, productID, requirementID, policy)
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
	children, keepTarget := c.deleteChildren(r, user, productID, req)
	_ = productpage.RequirementDeleteConfirmation(req.Title, productID, requirementID, middleware.CSRFTokenFromContext(r.Context()), children, keepTarget).Render(r.Context(), w)
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
	// (FR-100), so the children table has to resolve task prefixes too.
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
		Status:           c.resolveStatus(r, user, productID, req.StatusID, "requirement"),
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		rerenderForm(errorText(err))
		return
	}

	if err := c.component.LinkRequirementToCapability(r.Context(), user, productID, cap.ID, requirementID); err != nil {
		rerenderForm("created capability but failed to link: " + errorText(err))
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		rerenderForm(errorText(err))
		return
	}

	if err := c.testing.LinkRequirement(r.Context(), user, productID, tc.ID, requirementID, "requirement"); err != nil {
		rerenderForm("created test case but failed to link: " + errorText(err))
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		if query != "" && !strings.Contains(strings.ToLower(d.Title), query) && !strings.Contains(strings.ToLower(d.Identifier), query) {
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
	environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormModal(productpage.DefectFormProps{
		Meta:         templates.PageMeta{CSRFToken: csrfToken},
		DisplayName:  user.DisplayName,
		IsAdmin:      user.IsAdmin,
		Product:      p,
		Statuses:     statuses,
		Milestones:   milestones,
		Sprints:      sprints,
		Environments: environments,
		LinkTo:       "requirement:" + requirementID,
		Criticality:  "medium",
		StatusID:     defaultStatusID(statuses),
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
	environmentID := r.FormValue("environment_id")
	estimation := r.FormValue("estimation")
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)

	rerenderForm := func(errMsg string) {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormModal(productpage.DefectFormProps{
			Meta:          templates.PageMeta{CSRFToken: csrfToken},
			DisplayName:   user.DisplayName,
			IsAdmin:       user.IsAdmin,
			Product:       p,
			Statuses:      statuses,
			Milestones:    milestones,
			Sprints:       sprints,
			Environments:  environments,
			LinkTo:        "requirement:" + requirementID,
			Title:         title,
			Description:   description,
			Criticality:   criticality,
			StatusID:      statusID,
			MilestoneID:   milestoneID,
			SprintID:      sprintID,
			EnvironmentID: environmentID,
			Estimation:    estimation,
			Priority:      priorityStr,
			Error:         errMsg,
		}).Render(r.Context(), w)
	}

	d, err := c.backlog.CreateDefect(r.Context(), user, service.CreateDefectInput{
		ProductID:     productID,
		Title:         title,
		Description:   description,
		Criticality:   criticality,
		StatusID:      statusID,
		MilestoneID:   milestoneID,
		SprintID:      sprintID,
		EnvironmentID: environmentID,
		Estimation:    estimation,
		Priority:      priority,
	})
	if err != nil {
		rerenderForm(errorText(err))
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, d.ID, model.DefectLinkTargetRequirement, requirementID); err != nil {
		rerenderForm("created defect but failed to link: " + errorText(err))
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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

// defectsPageSize is the number of defects shown per page (UC-77). Fixed
// rather than user-selectable: another dimension of URL state buys little when
// sorting and filtering already narrow the list.
const defectsPageSize = 50

// Query parameter names for the defect list facets. They are also the names of
// the filter <select> elements, which is what lets htmx's hx-include ship the
// whole facet set back as the next request's query string.
const (
	defectFilterStatus      = "status_id"
	defectFilterCriticality = "criticality"
	defectFilterEnvironment = "environment_id"
	defectFilterSprint      = "sprint_id"
)

// parseDefectListState reads sort, search, facet and page state out of the
// request. Nothing here can fail: a stale bookmark or a hand-edited URL should
// render the default view, not an error page, so unknown sort keys and
// out-of-range pages are corrected rather than rejected.
//
// Facet values are passed through unvalidated, exactly as the JSON API does —
// they reach SQL only as bound parameters, and an unknown ID simply matches
// nothing.
func parseDefectListState(r *http.Request, baseURL string) (store.DefectFilter, store.ListParams, ui.ListState) {
	q := r.URL.Query()

	filter := store.DefectFilter{
		Criticality:   q.Get(defectFilterCriticality),
		StatusID:      q.Get(defectFilterStatus),
		SprintID:      q.Get(defectFilterSprint),
		EnvironmentID: q.Get(defectFilterEnvironment),
	}

	sortKey := q.Get(ui.ParamSort)
	if _, ok := store.DefectSortColumns[sortKey]; !ok {
		sortKey = ""
	}
	dir := ui.SortAsc
	if q.Get(ui.ParamDir) == ui.SortDesc {
		dir = ui.SortDesc
	}

	page, err := strconv.Atoi(q.Get(ui.ParamPage))
	if err != nil || page < 1 {
		page = 1
	}

	search := strings.TrimSpace(q.Get(ui.ParamSearch))

	state := ui.ListState{
		BaseURL: baseURL,
		Sort:    sortKey,
		Dir:     dir,
		Search:  search,
		Page:    page,
		Filters: map[string]string{
			defectFilterCriticality: filter.Criticality,
			defectFilterStatus:      filter.StatusID,
			defectFilterSprint:      filter.SprintID,
			defectFilterEnvironment: filter.EnvironmentID,
		},
	}

	params := store.ListParams{
		Sort:   sortKey,
		Dir:    dir,
		Search: search,
		Limit:  defectsPageSize,
		Offset: (page - 1) * defectsPageSize,
	}

	return filter, params, state
}

// loadDefectsProps assembles everything the defects list needs, for both the
// full page and the htmx fragment, so the two cannot render different views of
// the same URL.
func (c *PageProductController) loadDefectsProps(r *http.Request, user *model.User, productID string) (productpage.DefectsProps, error) {
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.DefectsProps{}, err
	}

	filter, params, state := parseDefectListState(r, "/products/"+productID+"/defects")

	defects, total, err := c.backlog.ListDefectsPage(r.Context(), user, productID, filter, params)
	if err != nil {
		return productpage.DefectsProps{}, err
	}

	// A page beyond the end of the result set renders as empty, which looks
	// like "no matches" rather than "you have scrolled too far". Clamp and
	// re-query so the last page is what the user actually lands on.
	totalPages := (total + defectsPageSize - 1) / defectsPageSize
	if totalPages > 0 && state.Page > totalPages {
		state.Page = totalPages
		params.Offset = (totalPages - 1) * defectsPageSize
		defects, _, err = c.backlog.ListDefectsPage(r.Context(), user, productID, filter, params)
		if err != nil {
			return productpage.DefectsProps{}, err
		}
	}

	// The unfiltered count separates "this product has no defects yet" from
	// "nothing matches these filters" — the first invites you to create one,
	// the second would be wrong to.
	productTotal := total
	if state.Filtered() {
		productTotal, _ = c.backlog.CountDefects(r.Context(), user, productID)
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)

	return productpage.DefectsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Defects",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		Defects:       defects,
		Statuses:      c.buildStatusMap(r, user, productID, "defect"),
		Environments:  c.buildEnvironmentMap(r, user, productID),
		CanCreate:     c.canCreateDefects(user, role),
		CanEdit:       c.canEditDefects(user, role),
		State:         state,
		Total:         total,
		ProductTotal:  productTotal,
		TotalPages:    totalPages,
		StatusOptions: statuses,
		EnvOptions:    environments,
		SprintOptions: sprints,
	}, nil
}

// Defects renders GET /products/{id}/defects — the full page for a normal
// request, and just the table fragment for an htmx one.
//
// The tab fragments elsewhere in this file each get their own route, but a list
// view cannot: sort, filter and page state has to be bookmarkable
// (DOC-1, "URL = state"), so the controls push their request URL
// into the address bar. A separate fragment route would push that route's URL
// instead, and the search box — whose URL depends on what the user has typed —
// cannot precompute a page URL to push in its place. Serving both shapes from
// the one URL keeps the address bar honest.
func (c *PageProductController) Defects(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	props, err := c.loadDefectsProps(r, user, r.PathValue("id"))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		_ = productpage.DefectsTableContent(props).Render(r.Context(), w)
		return
	}
	_ = productpage.DefectsPage(props).Render(r.Context(), w)
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
			Title:     p.Name + " - " + d.Identifier + " " + d.Title,
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Defect:      d,
		Overview:    c.buildDefectOverview(r, user, p, d),
		CanEdit:     c.canEditDefects(user, role),
		TabCounts:   c.defectTabCounts(r, user, p.ID, d.ID),
	}).Render(r.Context(), w)
}

// buildDefectOverview assembles the defect overview, including the campaign
// result trail that shows whether the fix was re-tested.
func (c *PageProductController) buildDefectOverview(r *http.Request, user *model.User, p *model.Product, d *model.Defect) productpage.DefectOverviewProps {
	return productpage.DefectOverviewProps{
		ProductID:       p.ID,
		EstimationMode:  p.EstimationMode,
		WorkflowMode:    p.WorkflowMode,
		Defect:          d,
		Status:          c.resolveStatus(r, user, p.ID, d.StatusID, "defect"),
		MilestoneName:   c.resolveMilestoneName(r, user, p.ID, d.MilestoneID),
		SprintName:      c.resolveSprintName(r, user, p.ID, d.SprintID),
		EnvironmentName: c.buildEnvironmentMap(r, user, p.ID)[d.EnvironmentID],
		Verification:    c.buildDefectVerification(r, user, p.ID, d.ID),
	}
}

// buildDefectVerification groups the defect's result trail by result, so each
// block reads as one test case's journey: the failure that raised the defect,
// the re-test once it was closed, and the verdict that followed.
//
// The trail deliberately includes events that no longer name the defect — a
// passing verdict clears the link, and that pass is the whole point of the
// section.
func (c *PageProductController) buildDefectVerification(r *http.Request, user *model.User, productID, defectID string) []productpage.DefectVerificationGroup {
	events, _ := c.testing.ListVerificationEvents(r.Context(), user, productID, defectID)
	if len(events) == 0 {
		return nil
	}

	names := c.displayNames(r, user)
	envs := c.buildEnvironmentMap(r, user, productID)
	campaignNames := make(map[string]string)
	for _, campaign := range c.listCampaignsQuietly(r, user, productID) {
		campaignNames[campaign.ID] = campaign.Name
	}
	// Events arrive oldest first; keep that order within each group and keep the
	// groups in the order their first event appeared.
	order := make([]string, 0)
	byResult := make(map[string][]*model.CampaignResultEvent)
	seenCampaign := make(map[string]bool)
	results := make(map[string]*model.CampaignResult)
	for _, ev := range events {
		if _, seen := byResult[ev.ResultID]; !seen {
			order = append(order, ev.ResultID)
		}
		byResult[ev.ResultID] = append(byResult[ev.ResultID], ev)

		// A defect usually spans one campaign, so fetch each campaign's results
		// once rather than looking up each result on its own.
		if !seenCampaign[ev.CampaignID] {
			seenCampaign[ev.CampaignID] = true
			campaignResults, _ := c.testing.ListCampaignResults(r.Context(), user, productID, ev.CampaignID)
			for _, result := range campaignResults {
				results[result.ID] = result
			}
		}
	}

	groups := make([]productpage.DefectVerificationGroup, 0, len(order))
	for _, resultID := range order {
		resultEvents := byResult[resultID]
		group := productpage.DefectVerificationGroup{
			CampaignName: campaignNames[resultEvents[0].CampaignID],
			Verified:     resultEvents[len(resultEvents)-1].ToStatus == model.CampaignResultStatusPassed,
		}
		if result, ok := results[resultID]; ok {
			group.TestCase = result.TestTitle
			if result.TestCaseID == "" {
				group.TestCase += " (deleted)"
			}
			group.Environment = resultEnvironment(result, envs)
		}

		previousUser := ""
		for _, ev := range resultEvents {
			group.Events = append(group.Events, productpage.ResultEventView{
				UserName:   eventActorName(names, ev.UserID),
				FromStatus: ev.FromStatus,
				ToStatus:   ev.ToStatus,
				At:         formatTimestamp(ev.CreatedAt),
				IsOverride: isOverride(previousUser, ev.UserID),
			})
			previousUser = ev.UserID
		}
		groups = append(groups, group)
	}
	return groups
}

// DefectCreateForm renders GET /products/{id}/defects/new.
func (c *PageProductController) DefectCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !c.canCreateDefects(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/defects", noticeNeedsTester)
		return
	}

	statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
	environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormPage(productpage.DefectFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - New Defect",
			CSRFToken: csrfToken,
		},
		DisplayName:  user.DisplayName,
		IsAdmin:      user.IsAdmin,
		Product:      p,
		Statuses:     statuses,
		Milestones:   milestones,
		Sprints:      sprints,
		Environments: environments,
		Criticality:  "medium",
		StatusID:     defaultStatusID(statuses),
		Priority:     "0",
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
	environmentID := r.FormValue("environment_id")
	estimation := strings.TrimSpace(r.FormValue("estimation"))
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)

	_, createErr := c.backlog.CreateDefect(r.Context(), user, service.CreateDefectInput{
		ProductID:     productID,
		Title:         title,
		Description:   description,
		Criticality:   criticality,
		StatusID:      statusID,
		MilestoneID:   milestoneID,
		SprintID:      sprintID,
		EnvironmentID: environmentID,
		Estimation:    estimation,
		Priority:      priority,
	})
	if createErr != nil {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormPage(productpage.DefectFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - New Defect",
				CSRFToken: csrfToken,
			},
			DisplayName:   user.DisplayName,
			IsAdmin:       user.IsAdmin,
			Product:       p,
			Statuses:      statuses,
			Milestones:    milestones,
			Sprints:       sprints,
			Environments:  environments,
			Error:         errorText(createErr),
			Title:         title,
			Description:   description,
			Criticality:   criticality,
			StatusID:      statusID,
			MilestoneID:   milestoneID,
			SprintID:      sprintID,
			EnvironmentID: environmentID,
			Estimation:    estimation,
			Priority:      priorityStr,
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

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !c.canEditDefects(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/defects/"+defectID, noticeNeedsOwner)
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
	environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectFormPage(productpage.DefectFormProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Edit " + d.Title,
			CSRFToken: csrfToken,
		},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		Defect:        d,
		Statuses:      statuses,
		Milestones:    milestones,
		Sprints:       sprints,
		Environments:  environments,
		IsEdit:        true,
		ReturnTo:      editReturnTarget(r),
		Title:         d.Title,
		Description:   d.Description,
		Criticality:   d.Criticality,
		StatusID:      d.StatusID,
		MilestoneID:   d.MilestoneID,
		SprintID:      d.SprintID,
		EnvironmentID: d.EnvironmentID,
		Estimation:    d.Estimation,
		Priority:      fmt.Sprintf("%d", d.Priority),
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
	environmentID := r.FormValue("environment_id")
	estimation := strings.TrimSpace(r.FormValue("estimation"))
	priorityStr := r.FormValue("priority")
	priority, _ := strconv.Atoi(priorityStr)

	_, updateErr := c.backlog.UpdateDefect(r.Context(), user, productID, defectID, service.UpdateDefectInput{
		IfUpdatedAt:   r.FormValue("updated_at"),
		Title:         &title,
		Description:   &description,
		Criticality:   &criticality,
		StatusID:      &statusID,
		MilestoneID:   &milestoneID,
		SprintID:      &sprintID,
		EnvironmentID: &environmentID,
		Estimation:    &estimation,
		Priority:      &priority,
	})
	if updateErr != nil {
		statuses, _ := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
		milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
		sprints, _ := c.milestone.ListSprints(r.Context(), user, productID)
		environments, _ := c.milestone.ListRuntimeEnvironments(r.Context(), user, productID)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = productpage.DefectFormPage(productpage.DefectFormProps{
			Meta: templates.PageMeta{
				Title:     p.Name + " - Edit " + d.Title,
				CSRFToken: csrfToken,
			},
			DisplayName:   user.DisplayName,
			IsAdmin:       user.IsAdmin,
			Product:       p,
			Defect:        d,
			Statuses:      statuses,
			Milestones:    milestones,
			Sprints:       sprints,
			Environments:  environments,
			IsEdit:        true,
			ReturnTo:      r.FormValue("return"),
			Error:         errorText(updateErr),
			Title:         title,
			Description:   description,
			Criticality:   criticality,
			StatusID:      statusID,
			MilestoneID:   milestoneID,
			SprintID:      sprintID,
			EnvironmentID: environmentID,
			Estimation:    estimation,
			Priority:      priorityStr,
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
	_ = productpage.DefectOverviewTabContent(c.buildDefectOverview(r, user, p, d)).Render(r.Context(), w)
}

// --- Defect link tabs (FR-29, FR-120) ---

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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		rerenderForm(errorText(err))
		return
	}

	if err := c.backlog.AddDefectLink(r.Context(), user, productID, defectID, model.DefectLinkTargetTestCase, tc.ID); err != nil {
		rerenderForm("created test case but failed to link: " + errorText(err))
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		TabCounts:        c.constraintTabCounts(r, user, productID, con.ID),
	}).Render(r.Context(), w)
}

// ConstraintCreateForm renders GET /products/{id}/constraints/new.
func (c *PageProductController) ConstraintCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !c.canEditConstraints(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/constraints", noticeNeedsOwner)
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
			Error:           errorText(createErr),
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

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !c.canEditConstraints(user, role) {
		redirectWithNotice(w, r, "/products/"+productID+"/constraints/"+constraintID, noticeNeedsOwner)
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
		IfUpdatedAt:     r.FormValue("updated_at"),
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
			Error:           errorText(updateErr),
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
		rerenderForm(errorText(err))
		return
	}

	if err := c.constraint.LinkCapability(r.Context(), user, productID, constraintID, cap.ID); err != nil {
		rerenderForm("created capability but failed to link: " + errorText(err))
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
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
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
