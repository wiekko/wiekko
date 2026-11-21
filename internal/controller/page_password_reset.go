// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"wiekko.net/internal/middleware"
	"wiekko.net/internal/service"
	login "wiekko.net/web/templates/pages/login"
)

// PagePasswordResetController serves the password reset pages (SC-37):
// asking for a link, and choosing a new password through it. Both exist only
// when the installation sends email; otherwise they answer 404 and the login
// page keeps telling people to ask an administrator.
type PagePasswordResetController struct {
	resets *service.PasswordResetService
}

// NewPagePasswordReset creates a PagePasswordResetController.
func NewPagePasswordReset(resets *service.PasswordResetService) *PagePasswordResetController {
	return &PagePasswordResetController{resets: resets}
}

// ShowForgot handles GET /forgot-password.
func (c *PagePasswordResetController) ShowForgot(w http.ResponseWriter, r *http.Request) {
	if !c.resets.Enabled() {
		http.NotFound(w, r)
		return
	}
	if middleware.UserFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	c.renderForgot(w, r, http.StatusOK, login.ForgotPasswordProps{})
}

// Forgot handles POST /forgot-password. Whatever was entered, the answer is
// the same confirmation (unless the client has asked too often), so the page
// cannot be used to find out which accounts exist.
func (c *PagePasswordResetController) Forgot(w http.ResponseWriter, r *http.Request) {
	if !c.resets.Enabled() {
		http.NotFound(w, r)
		return
	}
	props := login.ForgotPasswordProps{Identifier: r.FormValue("identifier")}
	err := c.resets.Request(r.Context(), props.Identifier, clientIP(r))
	var inputErr *service.AccountInputError
	switch {
	case err == nil:
		c.renderForgot(w, r, http.StatusOK, login.ForgotPasswordProps{Sent: true})
	case errors.As(err, &inputErr):
		props.Error = inputErr.Error()
		c.renderForgot(w, r, http.StatusUnprocessableEntity, props)
	default:
		if wait, blocked := attemptsBlocked(err); blocked {
			props.Error = fmt.Sprintf("Too many reset requests from your network. Try again in %s.", waitMinutes(wait))
			c.renderForgot(w, r, http.StatusTooManyRequests, props)
			return
		}
		slog.ErrorContext(r.Context(), "password reset request failed", "err", err)
		props.Error = "Your request could not be handled. Please try again later."
		c.renderForgot(w, r, http.StatusInternalServerError, props)
	}
}

// ShowReset handles GET /reset-password/{token}.
func (c *PagePasswordResetController) ShowReset(w http.ResponseWriter, r *http.Request) {
	if !c.resets.Enabled() {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	protectTokenPage(w)
	if err := c.resets.Check(r.Context(), token); err != nil {
		c.renderInvalid(w, r, err)
		return
	}
	c.renderReset(w, r, http.StatusOK, token, "")
}

// Reset handles POST /reset-password/{token}.
func (c *PagePasswordResetController) Reset(w http.ResponseWriter, r *http.Request) {
	if !c.resets.Enabled() {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	protectTokenPage(w)
	password := r.FormValue("password")
	if password != r.FormValue("password_confirm") {
		c.renderReset(w, r, http.StatusUnprocessableEntity, token, "The passwords do not match.")
		return
	}
	err := c.resets.Reset(r.Context(), token, password)
	var inputErr *service.AccountInputError
	switch {
	case err == nil:
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Retarget", "body")
		}
		c.renderMessage(w, http.StatusOK, r, "Password changed",
			"Your password has been changed and you have been signed out everywhere. Sign in with your new password.")
	case errors.Is(err, service.ErrPasswordResetInvalid):
		c.renderInvalid(w, r, err)
	case errors.As(err, &inputErr):
		c.renderReset(w, r, http.StatusUnprocessableEntity, token, inputErr.Error())
	default:
		slog.ErrorContext(r.Context(), "password reset failed", "err", err)
		c.renderReset(w, r, http.StatusInternalServerError, token, "Your password could not be changed. Please try again later.")
	}
}

// protectTokenPage keeps the token in the page's address out of Referer
// headers and caches.
func protectTokenPage(w http.ResponseWriter) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
}

func (c *PagePasswordResetController) renderForgot(w http.ResponseWriter, r *http.Request, status int, props login.ForgotPasswordProps) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		// htmx swaps the response into the form; a 4xx would be discarded.
		_ = login.ForgotPasswordForm(props).Render(r.Context(), w)
		return
	}
	w.WriteHeader(status)
	_ = login.ForgotPasswordPage(props).Render(r.Context(), w)
}

func (c *PagePasswordResetController) renderReset(w http.ResponseWriter, r *http.Request, status int, token, message string) {
	action := "/reset-password/" + token
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		_ = login.ResetPasswordForm(action, message).Render(r.Context(), w)
		return
	}
	w.WriteHeader(status)
	_ = login.ResetPasswordPage(action, message).Render(r.Context(), w)
}

func (c *PagePasswordResetController) renderInvalid(w http.ResponseWriter, r *http.Request, err error) {
	if !errors.Is(err, service.ErrPasswordResetInvalid) {
		slog.ErrorContext(r.Context(), "password reset link check failed", "err", err)
	}
	if r.Header.Get("HX-Request") == "true" {
		// Replace the whole page: the form is of no use any more.
		w.Header().Set("HX-Retarget", "body")
	}
	c.renderMessage(w, http.StatusNotFound, r, "Link not usable",
		"This password reset link is not valid any more: it was used already, it has expired, or a newer one was sent. Ask for a new link on the sign-in page.")
}

func (c *PagePasswordResetController) renderMessage(w http.ResponseWriter, status int, r *http.Request, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = login.AccountMessagePage(title, message).Render(r.Context(), w)
}
