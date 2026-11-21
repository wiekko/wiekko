// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"nexmap.net/internal/access"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/web/templates"
	userpage "nexmap.net/web/templates/pages/user"
)

// PageUserController handles user profile and token pages.
type PageUserController struct {
	user      *service.UserService
	pat       *service.PATService
	twoFactor *service.TwoFactorService
	product   *service.ProductService
}

// NewPageUser creates a PageUserController.
func NewPageUser(user *service.UserService, pat *service.PATService, twoFactor *service.TwoFactorService, product *service.ProductService) *PageUserController {
	return &PageUserController{user: user, pat: pat, twoFactor: twoFactor, product: product}
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
		_ = userpage.ProfileCard(user, errorText(err)).Render(r.Context(), w)
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
		_ = userpage.PasswordCard(errorText(err), true).Render(r.Context(), w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.PasswordCard("Password changed successfully. You were signed out on all other devices.", false).Render(r.Context(), w)
}

// RequestDeletion handles POST /user/profile/delete.
func (c *PageUserController) RequestDeletion(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	if err := c.user.RequestDeletion(r.Context(), user.ID, r.FormValue("password")); err != nil {
		// Plain text with 422: htmx-errors.js shows it as a toast.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(errorText(err)))
		return
	}

	w.Header().Set("HX-Redirect", "/login")
	w.WriteHeader(http.StatusOK)
}

// Tokens renders GET /user/tokens.
func (c *PageUserController) Tokens(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	form := c.tokenForm(r, user)
	form.Grants = access.FullAccess()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.TokensPage(userpage.TokensProps{
		Meta: templates.PageMeta{
			Title:     "API Tokens",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Tokens:      c.tokenViews(r, user),
		Form:        form,
	}).Render(r.Context(), w)
}

// tokenViews lists the user's tokens for display.
func (c *PageUserController) tokenViews(r *http.Request, user *model.User) []userpage.TokenView {
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
		views = append(views, userpage.TokenView{Token: p, IsExpired: expired})
	}
	return views
}

// tokenForm prepares the create token form: the products the user can
// restrict a token to and the instance's lifetime limit.
func (c *PageUserController) tokenForm(r *http.Request, user *model.User) userpage.CreateTokenFormProps {
	form := userpage.CreateTokenFormProps{IsAdmin: user.IsAdmin, Grants: access.Grants{}}
	if products, err := c.product.ListWithRoles(r.Context(), user); err == nil {
		for _, p := range products {
			form.Products = append(form.Products, userpage.ProductOption{ID: p.Product.ID, Name: p.Product.Name})
		}
	}
	if days := c.pat.MaxLifetimeDays(r.Context()); days > 0 {
		form.MaxLifetimeDays = days
		form.MaxExpiry = time.Now().UTC().AddDate(0, 0, days).Format(time.DateOnly)
	}
	return form
}

// CreateToken handles POST /user/tokens.
func (c *PageUserController) CreateToken(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	form := c.tokenForm(r, user)
	form.Name = r.FormValue("name")
	form.ExpiresAt = r.FormValue("expires_at")
	form.RoleCap = r.FormValue("role_cap")
	form.RestrictProducts = r.FormValue("products_mode") == "selected"
	selected := r.Form["product_ids"]
	for i := range form.Products {
		form.Products[i].Selected = slices.Contains(selected, form.Products[i].ID)
	}

	in := service.CreatePATInput{Name: form.Name, RoleCap: form.RoleCap, Grants: access.Grants{},
		Password: r.FormValue("password")}
	var formErr string
	for _, sc := range access.AllScopes {
		level, err := access.ParseLevel(r.FormValue("scope_" + string(sc)))
		if err != nil {
			formErr = errorText(err)
			continue
		}
		if level > access.None {
			in.Grants[sc] = level
		}
	}
	form.Grants = in.Grants
	if form.ExpiresAt != "" {
		in.ExpiresAt = form.ExpiresAt + "T23:59:59Z"
	}
	if form.RestrictProducts {
		in.ProductsRestricted = true
		in.ProductIDs = selected
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	switch {
	case form.Name == "":
		formErr = "Token name is required."
	case formErr == "" && len(in.Grants) == 0:
		formErr = "Grant the token at least one scope."
	}
	if formErr != "" {
		form.Error = formErr
		_ = userpage.CreateTokenForm(form).Render(r.Context(), w)
		return
	}

	result, err := c.pat.Create(r.Context(), user, in)
	if err != nil {
		msg := strings.TrimPrefix(errorText(err), service.ErrForbidden.Error()+": ")
		if msg != "" {
			msg = strings.ToUpper(msg[:1]) + msg[1:] + "."
		}
		form.Error = msg
		_ = userpage.CreateTokenForm(form).Render(r.Context(), w)
		return
	}

	_ = userpage.TokenCreated(result.RawToken).Render(r.Context(), w)
}

// RevokeToken handles DELETE /user/tokens/{id}.
func (c *PageUserController) RevokeToken(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	tokenID := r.PathValue("id")

	if err := c.pat.Revoke(r.Context(), user.ID, tokenID); err != nil {
		http.Error(w, errorText(err), http.StatusBadRequest)
		return
	}

	// Return updated token table.
	views := c.tokenViews(r, user)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.TokenTableBody(views).Render(r.Context(), w)
}
