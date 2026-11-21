// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"errors"
	"log/slog"
	"net/http"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/web/templates"
	userpage "nexmap.net/web/templates/pages/user"
)

// twoFactorCard builds the card state for a user who is not in the middle of
// an action: pending setup, enabled, or not configured.
func (c *PageUserController) twoFactorCard(r *http.Request, user *model.User, standalone bool) userpage.TwoFactorCardProps {
	props := userpage.TwoFactorCardProps{
		Enabled:    user.TOTPEnabled,
		Standalone: standalone,
	}
	req, err := c.twoFactor.Requirement(r.Context(), user)
	if err != nil {
		slog.Error("cannot read the 2FA requirement", "user_id", user.ID, "err", err)
	}
	props.Enforced = req.Required()
	props.EnforcedBy = req.String()
	// A product the setup page was opened for (see RequireTwoFactorEnrollment):
	// the card returns there once setup is done.
	if standalone {
		if id := r.FormValue("product"); safeProductID(id) {
			props.ProductID = id
		}
	}
	if user.TOTPEnabled {
		if n, err := c.twoFactor.RecoveryCodesRemaining(r.Context(), user.ID); err == nil {
			props.RecoveryRemaining = n
		}
	}
	return props
}

func setupView(setup *service.TOTPSetup) *userpage.TOTPSetupView {
	if setup == nil {
		return nil
	}
	return &userpage.TOTPSetupView{QRCodeSVG: setup.QRCodeSVG, FormattedSecret: setup.FormattedSecret}
}

func renderTwoFactorCard(w http.ResponseWriter, r *http.Request, props userpage.TwoFactorCardProps) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.TwoFactorCard(props).Render(r.Context(), w)
}

// safeProductID reports whether id can be put into a /products/{id} link
// without leading anywhere else.
func safeProductID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, ch := range id {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '-') {
			return false
		}
	}
	return true
}

// TwoFactorSetupPage renders GET /user/2fa/setup. Users land here when the
// instance requires 2FA of them, or when they open a product that requires it
// (?product=); it starts (or resumes) the setup right away.
func (c *PageUserController) TwoFactorSetupPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user.TOTPEnabled {
		http.Redirect(w, r, "/user/profile", http.StatusSeeOther)
		return
	}

	card := c.twoFactorCard(r, user, true)
	var productName string
	if card.ProductID != "" {
		if p := c.twoFactor.LockedProduct(r.Context(), user, card.ProductID); p != nil {
			productName = p.Name
		} else {
			card.ProductID = ""
		}
	}
	setup, err := c.twoFactor.PendingSetup(user)
	if err == nil && setup == nil {
		setup, err = c.twoFactor.BeginSetup(r.Context(), user.ID)
	}
	if err != nil {
		slog.Error("two-factor setup failed", "user_id", user.ID, "err", err)
		card.Feedback, card.IsError = "Could not start two-factor setup. Please try again.", true
	}
	card.Setup = setupView(setup)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = userpage.TwoFactorSetupPage(userpage.TwoFactorSetupProps{
		Meta: templates.PageMeta{
			Title:     "Two-Factor Authentication",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:  user.DisplayName,
		IsAdmin:      user.IsAdmin,
		InstanceRule: c.twoFactor.RequiredFor(r.Context(), user),
		EnforcedBy:   card.EnforcedBy,
		ProductName:  productName,
		Card:         card,
	}).Render(r.Context(), w)
}

// TwoFactorBegin handles POST /user/2fa/setup: it generates a new secret and
// shows the QR code.
func (c *PageUserController) TwoFactorBegin(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	card := c.twoFactorCard(r, user, r.FormValue("standalone") != "")

	setup, err := c.twoFactor.BeginSetup(r.Context(), user.ID)
	if err != nil {
		card.Feedback, card.IsError = twoFactorMessage(err), true
		renderTwoFactorCard(w, r, card)
		return
	}
	card.Setup = setupView(setup)
	renderTwoFactorCard(w, r, card)
}

// TwoFactorConfirm handles POST /user/2fa/confirm.
func (c *PageUserController) TwoFactorConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	card := c.twoFactorCard(r, user, r.FormValue("standalone") != "")

	codes, err := c.twoFactor.ConfirmSetup(r.Context(), user.ID, r.FormValue("code"))
	if err != nil {
		// Keep showing the same QR code so the user can simply retry.
		if setup, serr := c.twoFactor.PendingSetup(user); serr == nil {
			card.Setup = setupView(setup)
		}
		card.Feedback, card.IsError = twoFactorMessage(err), true
		renderTwoFactorCard(w, r, card)
		return
	}

	slog.Info("two-factor authentication enabled", "user_id", user.ID)
	card.Enabled = true
	card.RecoveryCodes = codes
	card.Feedback = "Two-factor authentication is now enabled."
	renderTwoFactorCard(w, r, card)
}

// TwoFactorRegenerate handles POST /user/2fa/recovery-codes.
func (c *PageUserController) TwoFactorRegenerate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	card := c.twoFactorCard(r, user, false)

	codes, err := c.twoFactor.RegenerateRecoveryCodes(r.Context(), user.ID, r.FormValue("code"))
	if err != nil {
		card.Feedback, card.IsError = twoFactorMessage(err), true
		card.Section = "regenerate"
		renderTwoFactorCard(w, r, card)
		return
	}
	card.RecoveryCodes = codes
	card.Feedback = "New recovery codes generated. The old ones no longer work."
	renderTwoFactorCard(w, r, card)
}

// TwoFactorDisable handles POST /user/2fa/disable.
func (c *PageUserController) TwoFactorDisable(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	err := c.twoFactor.Disable(r.Context(), user.ID, r.FormValue("password"), r.FormValue("code"))
	if err != nil {
		card := c.twoFactorCard(r, user, false)
		card.Feedback, card.IsError = twoFactorMessage(err), true
		card.Section = "disable"
		renderTwoFactorCard(w, r, card)
		return
	}

	slog.Info("two-factor authentication disabled", "user_id", user.ID)
	card := c.twoFactorCard(r, user, false)
	card.Enabled = false
	card.Feedback = "Two-factor authentication turned off."
	renderTwoFactorCard(w, r, card)
}

// twoFactorMessage maps service errors to messages fit for the page.
func twoFactorMessage(err error) string {
	if wait, blocked := attemptsBlocked(err); blocked {
		return tooManyCodesMessage(wait)
	}
	switch {
	case errors.Is(err, service.ErrTOTPCodeReused):
		return "That code was already used. Wait for your app to show the next code."
	case errors.Is(err, service.ErrInvalidTOTPCode):
		return "That code is not valid. Check the time on your device and try the next code."
	case errors.Is(err, service.ErrIncorrectPassword),
		errors.Is(err, service.ErrTOTPEnforced),
		errors.Is(err, service.ErrTOTPAlreadyEnabled),
		errors.Is(err, service.ErrTOTPNotEnabled),
		errors.Is(err, service.ErrTOTPNoPendingSetup):
		return errorText(err)
	default:
		slog.Error("two-factor operation failed", "err", err)
		return "Something went wrong. Please try again."
	}
}
