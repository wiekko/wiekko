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
}

// NewPageProduct creates a PageProductController.
func NewPageProduct(product *service.ProductService, dashboard *service.DashboardService, settings *service.SettingService, user *service.UserService, invitation *service.InvitationService) *PageProductController {
	return &PageProductController{product: product, dashboard: dashboard, settings: settings, user: user, invitation: invitation}
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

// SettingsStatuses renders the stub Statuses tab content.
func (c *PageProductController) SettingsStatuses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.StubTabContent("Statuses").Render(r.Context(), w)
}

// SettingsEnvironments renders the stub Environments tab content.
func (c *PageProductController) SettingsEnvironments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.StubTabContent("Environments").Render(r.Context(), w)
}

// SettingsPyramid renders the stub Pyramid Levels tab content.
func (c *PageProductController) SettingsPyramid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.StubTabContent("Pyramid Levels").Render(r.Context(), w)
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
