package controller

import (
	"net/http"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	admin "nexmap.net/web/templates/pages/admin"
)

// PageAdminController handles admin page routes.
type PageAdminController struct {
	user       *service.UserService
	setting    *service.SettingService
	invitation *service.InvitationService
}

// NewPageAdmin creates a PageAdminController.
func NewPageAdmin(user *service.UserService, setting *service.SettingService, invitation *service.InvitationService) *PageAdminController {
	return &PageAdminController{user: user, setting: setting, invitation: invitation}
}

// ListUsers renders GET /admin/users.
func (c *PageAdminController) ListUsers(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	statusFilter := r.URL.Query().Get("status")
	users, err := c.user.List(r.Context(), store.UserFilter{Status: statusFilter})
	if err != nil {
		users = nil
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = admin.UsersPage(admin.UsersProps{
		Meta: templates.PageMeta{
			Title:     "Users",
			CSRFToken: csrfToken,
		},
		DisplayName:  user.DisplayName,
		IsAdmin:      user.IsAdmin,
		Users:        users,
		StatusFilter: statusFilter,
	}).Render(r.Context(), w)
}

// GetUser renders GET /admin/users/{id}.
func (c *PageAdminController) GetUser(w http.ResponseWriter, r *http.Request) {
	currentUser := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	targetID := r.PathValue("id")
	target, err := c.user.GetByID(r.Context(), targetID)
	if err != nil {
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = admin.UserDetailPage(admin.UserDetailProps{
		Meta: templates.PageMeta{
			Title:     target.Username,
			CSRFToken: csrfToken,
		},
		DisplayName: currentUser.DisplayName,
		IsAdmin:     currentUser.IsAdmin,
		IsSelf:      currentUser.ID == target.ID,
		Target:      target,
	}).Render(r.Context(), w)
}

// Settings renders GET /admin/settings.
func (c *PageAdminController) Settings(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	settings, err := c.setting.GetAll(r.Context())
	if err != nil {
		settings = make(map[string]string)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = admin.SettingsPage(admin.SettingsProps{
		Meta: templates.PageMeta{
			Title:     "Instance Settings",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Settings:    settings,
	}).Render(r.Context(), w)
}

// Invitations renders GET /admin/invitations.
func (c *PageAdminController) Invitations(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	invitations, err := c.invitation.List(r.Context())
	if err != nil {
		invitations = nil
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = admin.InvitationsPage(admin.InvitationsProps{
		Meta: templates.PageMeta{
			Title:     "Invitations",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Invitations: invitations,
	}).Render(r.Context(), w)
}

func userStatusBadge(u *model.User) (string, string) {
	switch u.Status {
	case model.UserStatusActive:
		return "Active", "green"
	case model.UserStatusPendingApproval:
		return "Pending", "yellow"
	case model.UserStatusLocked:
		return "Locked", "red"
	case model.UserStatusDeletionRequested:
		return "Deletion Requested", "red"
	default:
		return u.Status, "gray"
	}
}
