// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"

	"wiekko.net/internal/access"
	"wiekko.net/internal/httputil"
	"wiekko.net/internal/middleware"
	"wiekko.net/internal/service"
)

// UserController handles current-user endpoints.
type UserController struct {
	user *service.UserService
}

// NewUser creates a UserController.
func NewUser(user *service.UserService) *UserController {
	return &UserController{user: user}
}

// GetMe handles GET /user/me.
//
// For token-authenticated requests the response carries a "token" object
// with the token's restriction (SC-51 – SC-56).
func (c *UserController) GetMe(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	resp := userToJSON(user)
	if restriction := access.FromContext(r.Context()); restriction != nil {
		resp["token"] = restrictionJSON(restriction)
	}
	httputil.JSON(w, http.StatusOK, resp)
}

// UpdateMe handles PATCH /user/me.
func (c *UserController) UpdateMe(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	var req struct {
		Email       *string `json:"email"`
		DisplayName *string `json:"display_name"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	updated, err := c.user.UpdateProfile(r.Context(), user.ID, service.UpdateProfileInput{
		Email:       req.Email,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		writeAccountError(w, "UPDATE_FAILED", err)
		return
	}

	httputil.JSON(w, http.StatusOK, userToJSON(updated))
}

// ChangePassword handles POST /user/me/password.
func (c *UserController) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Old and new passwords are required")
		return
	}

	if err := c.user.ChangePassword(r.Context(), user.ID, req.OldPassword, req.NewPassword, currentSessionID(r)); err != nil {
		writeAccountError(w, "PASSWORD_CHANGE_FAILED", err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
}

// RequestDeletion handles POST /user/me/delete.
func (c *UserController) RequestDeletion(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	// Session requests confirm with the password; token requests send no body.
	var req struct {
		Password string `json:"password"`
	}
	if r.ContentLength != 0 && !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if err := c.user.RequestDeletion(r.Context(), user.ID, req.Password); err != nil {
		writeAccountError(w, "DELETION_FAILED", err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deletion_requested"})
}
