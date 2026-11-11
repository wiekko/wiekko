package controller

import (
	"log/slog"
	"net/http"

	"nexmap.net/internal/httputil"
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
	twoFactor  *service.TwoFactorService
}

// NewPageAdmin creates a PageAdminController.
func NewPageAdmin(user *service.UserService, setting *service.SettingService, invitation *service.InvitationService, twoFactor *service.TwoFactorService) *PageAdminController {
	return &PageAdminController{user: user, setting: setting, invitation: invitation, twoFactor: twoFactor}
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

// ApproveUser handles POST /admin/users/{id}/approve.
func (c *PageAdminController) ApproveUser(w http.ResponseWriter, r *http.Request) {
	currentUser := middleware.UserFromContext(r.Context())
	targetID := r.PathValue("id")

	if err := c.user.AdminApproveUser(r.Context(), targetID); err != nil {
		c.renderActionsCard(w, r, targetID, currentUser, err.Error())
		return
	}

	c.renderActionsCard(w, r, targetID, currentUser, "")
}

// LockUser handles POST /admin/users/{id}/lock.
func (c *PageAdminController) LockUser(w http.ResponseWriter, r *http.Request) {
	currentUser := middleware.UserFromContext(r.Context())
	targetID := r.PathValue("id")

	if err := c.user.AdminLockUser(r.Context(), targetID); err != nil {
		c.renderActionsCard(w, r, targetID, currentUser, err.Error())
		return
	}

	c.renderActionsCard(w, r, targetID, currentUser, "")
}

// UnlockUser handles POST /admin/users/{id}/unlock.
func (c *PageAdminController) UnlockUser(w http.ResponseWriter, r *http.Request) {
	currentUser := middleware.UserFromContext(r.Context())
	targetID := r.PathValue("id")

	if err := c.user.AdminUnlockUser(r.Context(), targetID); err != nil {
		c.renderActionsCard(w, r, targetID, currentUser, err.Error())
		return
	}

	c.renderActionsCard(w, r, targetID, currentUser, "")
}

// SetPassword handles POST /admin/users/{id}/password.
func (c *PageAdminController) SetPassword(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	password := r.FormValue("password")

	if password == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = admin.PasswordResetForm(targetID, "Password is required.").Render(r.Context(), w)
		return
	}

	if err := c.user.AdminSetPassword(r.Context(), targetID, password, currentSessionID(r)); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = admin.PasswordResetForm(targetID, err.Error()).Render(r.Context(), w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = admin.PasswordResetSuccess().Render(r.Context(), w)
}

// ResetTwoFactor handles POST /admin/users/{id}/2fa/reset: it removes the
// user's TOTP setup and recovery codes, e.g. after a lost phone.
func (c *PageAdminController) ResetTwoFactor(w http.ResponseWriter, r *http.Request) {
	currentUser := middleware.UserFromContext(r.Context())
	targetID := r.PathValue("id")

	if err := c.twoFactor.AdminReset(r.Context(), targetID); err != nil {
		c.renderActionsCard(w, r, targetID, currentUser, err.Error())
		return
	}
	slog.Warn("two-factor authentication reset by administrator", "user_id", targetID, "admin_id", currentUser.ID)
	c.renderActionsCard(w, r, targetID, currentUser, "Two-factor authentication has been reset.")
}

// DeleteUser handles DELETE /admin/users/{id}.
func (c *PageAdminController) DeleteUser(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")

	if err := c.user.AdminDeleteUser(r.Context(), targetID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("HX-Redirect", "/admin/users")
	w.WriteHeader(http.StatusOK)
}

// renderActionsCard re-fetches the target user and renders the actions card fragment.
func (c *PageAdminController) renderActionsCard(w http.ResponseWriter, r *http.Request, targetID string, currentUser *model.User, feedback string) {
	target, err := c.user.GetByID(r.Context(), targetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = admin.ActionsCard(target, currentUser.ID == target.ID, feedback).Render(r.Context(), w)
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

// UpdateSetting handles PATCH /admin/settings.
func (c *PageAdminController) UpdateSetting(w http.ResponseWriter, r *http.Request) {
	key := r.FormValue("key")
	value := r.FormValue("value")

	if key == "" {
		http.Error(w, "Missing key", http.StatusBadRequest)
		return
	}

	if err := c.setting.Update(r.Context(), map[string]string{key: value}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated toggle/input element.
	checked := value == "true"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Determine which kind of setting this is based on value type.
	// If it's "true" or "false", it's a toggle; otherwise it's a number input.
	if value == "true" || value == "false" {
		label, description := settingMeta(key)
		_ = admin.SettingToggle(key, label, description, checked).Render(r.Context(), w)
	} else {
		label, _ := settingMeta(key)
		_ = admin.SettingNumber(key, label, value).Render(r.Context(), w)
	}
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

// CreateInvitation handles POST /admin/invitations.
func (c *PageAdminController) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	currentUser := middleware.UserFromContext(r.Context())
	email := r.FormValue("email")
	if email == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = admin.InviteResult("Email is required.", "", false).Render(r.Context(), w)
		return
	}

	result, err := c.invitation.Create(r.Context(), currentUser, email, "", "")
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = admin.InviteResult(err.Error(), "", false).Render(r.Context(), w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = admin.InviteResult("", c.invitation.AcceptURL(result.RawToken, httputil.RequestBaseURL(r)), true).Render(r.Context(), w)
}

// DeleteInvitation handles DELETE /admin/invitations/{id}.
func (c *PageAdminController) DeleteInvitation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := c.invitation.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return empty response — htmx will delete the row via hx-swap="delete".
	w.WriteHeader(http.StatusOK)
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

func settingMeta(key string) (label, description string) {
	switch key {
	case "self_registration_enabled":
		return "Self-Registration", "Allow users to register without an invitation link."
	case "moderation_enabled":
		return "Moderation", "Require admin approval for self-registered accounts."
	case "product_creation_enabled":
		return "Product Creation", "Allow non-admin users to create new products."
	case "2fa_enforced":
		return "Enforce 2FA", "Require two-factor authentication for all users."
	case "password_min_length":
		return "Minimum Length", ""
	case "password_require_uppercase":
		return "Require Uppercase", "At least one uppercase letter."
	case "password_require_lowercase":
		return "Require Lowercase", "At least one lowercase letter."
	case "password_require_digit":
		return "Require Digit", "At least one numeric digit."
	case "password_require_special":
		return "Require Special Character", "At least one special character."
	default:
		return key, ""
	}
}
