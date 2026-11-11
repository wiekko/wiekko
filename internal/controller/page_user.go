package controller

import (
	"net/http"
	"time"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
	"nexmap.net/web/templates"
	userpage "nexmap.net/web/templates/pages/user"
)

// PageUserController handles user profile and token pages.
type PageUserController struct {
	user      *service.UserService
	pat       *service.PATService
	twoFactor *service.TwoFactorService
}

// NewPageUser creates a PageUserController.
func NewPageUser(user *service.UserService, pat *service.PATService, twoFactor *service.TwoFactorService) *PageUserController {
	return &PageUserController{user: user, pat: pat, twoFactor: twoFactor}
}

// Profile renders GET /user/profile.
func (c *PageUserController) Profile(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.ProfilePage(userpage.ProfileProps{
		Meta: templates.PageMeta{
			Title:     "Profile",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		User:        user,
		TwoFactor:   c.twoFactorCard(r, user, false),
	}).Render(r.Context(), w)
}

// UpdateProfile handles POST /user/profile.
func (c *PageUserController) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	displayName := r.FormValue("display_name")
	email := r.FormValue("email")

	updated, err := c.user.UpdateProfile(r.Context(), user.ID, service.UpdateProfileInput{
		Email:       &email,
		DisplayName: &displayName,
	})
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = userpage.ProfileCard(user, err.Error()).Render(r.Context(), w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.ProfileCard(updated, "Profile updated successfully.").Render(r.Context(), w)
	// OOB swap to update the header display name.
	_ = userpage.HeaderUserInfoOOB(updated.DisplayName).Render(r.Context(), w)
}

// ChangePassword handles POST /user/profile/password.
func (c *PageUserController) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	oldPassword := r.FormValue("old_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if oldPassword == "" || newPassword == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = userpage.PasswordCard("All password fields are required.", true).Render(r.Context(), w)
		return
	}
	if newPassword != confirmPassword {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = userpage.PasswordCard("New passwords do not match.", true).Render(r.Context(), w)
		return
	}

	if err := c.user.ChangePassword(r.Context(), user.ID, oldPassword, newPassword, currentSessionID(r)); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = userpage.PasswordCard(err.Error(), true).Render(r.Context(), w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.PasswordCard("Password changed successfully. You were signed out on all other devices.", false).Render(r.Context(), w)
}

// RequestDeletion handles POST /user/profile/delete.
func (c *PageUserController) RequestDeletion(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	if err := c.user.RequestDeletion(r.Context(), user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("HX-Redirect", "/login")
	w.WriteHeader(http.StatusOK)
}

// Tokens renders GET /user/tokens.
func (c *PageUserController) Tokens(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	pats, err := c.pat.List(r.Context(), user.ID)
	if err != nil {
		pats = nil
	}

	now := time.Now().UTC()
	views := make([]userpage.TokenView, 0, len(pats))
	for _, p := range pats {
		expired := false
		if p.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil {
				expired = now.After(t)
			}
		}
		views = append(views, userpage.TokenView{
			Token:     p,
			IsExpired: expired,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.TokensPage(userpage.TokensProps{
		Meta: templates.PageMeta{
			Title:     "API Tokens",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Tokens:      views,
	}).Render(r.Context(), w)
}

// CreateToken handles POST /user/tokens.
func (c *PageUserController) CreateToken(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	name := r.FormValue("name")
	if name == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = userpage.CreateTokenForm("Token name is required.").Render(r.Context(), w)
		return
	}

	var expiresAt *string
	if v := r.FormValue("expires_at"); v != "" {
		exp := v + "T23:59:59Z"
		expiresAt = &exp
	}

	result, err := c.pat.Create(r.Context(), user.ID, name, expiresAt)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = userpage.CreateTokenForm(err.Error()).Render(r.Context(), w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.TokenCreated(result.RawToken).Render(r.Context(), w)
}

// RevokeToken handles DELETE /user/tokens/{id}.
func (c *PageUserController) RevokeToken(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	tokenID := r.PathValue("id")

	if err := c.pat.Revoke(r.Context(), user.ID, tokenID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return updated token table.
	pats, err := c.pat.List(r.Context(), user.ID)
	if err != nil {
		pats = nil
	}

	now := time.Now().UTC()
	views := make([]userpage.TokenView, 0, len(pats))
	for _, p := range pats {
		expired := false
		if p.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil {
				expired = now.After(t)
			}
		}
		views = append(views, userpage.TokenView{
			Token:     p,
			IsExpired: expired,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.TokenTableBody(views).Render(r.Context(), w)
}
