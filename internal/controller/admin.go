// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"log/slog"
	"net/http"

	"wiekko.net/internal/httputil"
	"wiekko.net/internal/middleware"
	"wiekko.net/internal/service"
	"wiekko.net/internal/store"
)

// AdminController handles admin-only endpoints.
type AdminController struct {
	user    *service.UserService
	setting *service.SettingService
}

// NewAdmin creates an AdminController.
func NewAdmin(user *service.UserService, setting *service.SettingService) *AdminController {
	return &AdminController{user: user, setting: setting}
}

// ListUsers handles GET /admin/users.
func (c *AdminController) ListUsers(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	users, err := c.user.List(r.Context(), store.UserFilter{Status: status})
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "LIST_FAILED", "Failed to list users")
		return
	}

	items := make([]map[string]any, 0, len(users))
	for _, u := range users {
		items = append(items, userToJSON(u))
	}
	httputil.JSON(w, http.StatusOK, items)
}

// GetUser handles GET /admin/users/{id}.
func (c *AdminController) GetUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	user, err := c.user.GetByID(r.Context(), userID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "NOT_FOUND", "User not found")
		return
	}
	httputil.JSON(w, http.StatusOK, userToJSON(user))
}

// LockUser handles POST /admin/users/{id}/lock.
func (c *AdminController) LockUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if err := c.user.AdminLockUser(r.Context(), userID); err != nil {
		writeAccountError(w, "LOCK_FAILED", err)
		return
	}
	logAdminAction(r, "lock user", userID)
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "locked"})
}

// UnlockUser handles POST /admin/users/{id}/unlock.
func (c *AdminController) UnlockUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if err := c.user.AdminUnlockUser(r.Context(), userID); err != nil {
		writeAccountError(w, "UNLOCK_FAILED", err)
		return
	}
	logAdminAction(r, "unlock user", userID)
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "unlocked"})
}

// SetPassword handles POST /admin/users/{id}/password.
func (c *AdminController) SetPassword(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	var req struct {
		Password string `json:"password"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Password == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Password is required")
		return
	}

	if err := c.user.AdminSetPassword(r.Context(), userID, req.Password, currentSessionID(r)); err != nil {
		writeAccountError(w, "PASSWORD_SET_FAILED", err)
		return
	}
	logAdminAction(r, "set password", userID)
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "password_set"})
}

// ApproveUser handles POST /admin/users/{id}/approve.
func (c *AdminController) ApproveUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if err := c.user.AdminApproveUser(r.Context(), userID); err != nil {
		writeAccountError(w, "APPROVE_FAILED", err)
		return
	}
	logAdminAction(r, "approve user", userID)
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// DeleteUser handles DELETE /admin/users/{id}.
func (c *AdminController) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if err := c.user.AdminDeleteUser(r.Context(), userID); err != nil {
		writeAccountError(w, "DELETE_FAILED", err)
		return
	}
	logAdminAction(r, "delete user", userID)
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GetSettings handles GET /admin/settings.
func (c *AdminController) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := c.setting.GetAll(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "SETTINGS_FAILED", "Failed to load settings")
		return
	}
	httputil.JSON(w, http.StatusOK, settings)
}

// UpdateSettings handles PATCH /admin/settings.
func (c *AdminController) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if err := c.setting.Update(r.Context(), req); err != nil {
		writeAccountError(w, "SETTINGS_UPDATE_FAILED", err)
		return
	}
	for key, value := range req {
		logAdminAction(r, "change setting", "", "setting", key, "value", value)
	}

	settings, err := c.setting.GetAll(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "SETTINGS_FAILED", "Failed to load settings")
		return
	}
	httputil.JSON(w, http.StatusOK, settings)
}

// logAdminAction records an administrative change in the server log: who did
// what to whom, and from where. These actions have no place in a product's
// audit trail, and without a record a misused or stolen administrator account
// would leave no trace. targetID is the affected user, or "" if there is none;
// more attributes may follow as key-value pairs.
func logAdminAction(r *http.Request, action, targetID string, attrs ...any) {
	args := []any{"action", action}
	if targetID != "" {
		args = append(args, "user_id", targetID)
	}
	if actor := middleware.UserFromContext(r.Context()); actor != nil {
		args = append(args, "admin_id", actor.ID)
	}
	args = append(args, "ip", clientIP(r), "request_id", middleware.RequestIDFromContext(r.Context()))
	slog.Warn("administrator action", append(args, attrs...)...)
}
