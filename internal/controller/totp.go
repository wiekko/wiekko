package controller

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

// TOTPController handles the two-factor authentication API endpoints.
type TOTPController struct {
	twoFactor *service.TwoFactorService
}

// NewTOTP creates a TOTPController.
func NewTOTP(twoFactor *service.TwoFactorService) *TOTPController {
	return &TOTPController{twoFactor: twoFactor}
}

// Status handles GET /user/me/totp.
func (c *TOTPController) Status(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	resp := map[string]any{
		"enabled":  user.TOTPEnabled,
		"enforced": c.twoFactor.Enforced(r.Context()),
	}
	if user.TOTPEnabled {
		n, err := c.twoFactor.RecoveryCodesRemaining(r.Context(), user.ID)
		if err != nil {
			httputil.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read recovery codes")
			return
		}
		resp["recovery_codes_remaining"] = n
	}
	httputil.JSON(w, http.StatusOK, resp)
}

// Begin handles POST /user/me/totp: it generates a pending secret.
func (c *TOTPController) Begin(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	setup, err := c.twoFactor.BeginSetup(r.Context(), user.ID)
	if err != nil {
		writeTOTPError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{
		"secret":           setup.Secret,
		"provisioning_uri": setup.URI,
	})
}

// Confirm handles POST /user/me/totp/confirm: it activates TOTP and returns
// the recovery codes (shown only this once).
func (c *TOTPController) Confirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	user := middleware.UserFromContext(r.Context())
	codes, err := c.twoFactor.ConfirmSetup(r.Context(), user.ID, req.Code)
	if err != nil {
		writeTOTPError(w, err)
		return
	}
	slog.Info("two-factor authentication enabled", "user_id", user.ID)
	httputil.JSON(w, http.StatusOK, map[string]any{"enabled": true, "recovery_codes": codes})
}

// Regenerate handles POST /user/me/totp/recovery-codes.
func (c *TOTPController) Regenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	user := middleware.UserFromContext(r.Context())
	codes, err := c.twoFactor.RegenerateRecoveryCodes(r.Context(), user.ID, req.Code)
	if err != nil {
		writeTOTPError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// Disable handles DELETE /user/me/totp.
func (c *TOTPController) Disable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	user := middleware.UserFromContext(r.Context())
	if err := c.twoFactor.Disable(r.Context(), user.ID, req.Password, req.Code); err != nil {
		writeTOTPError(w, err)
		return
	}
	slog.Info("two-factor authentication disabled", "user_id", user.ID)
	httputil.JSON(w, http.StatusOK, map[string]bool{"enabled": false})
}

// AdminReset handles DELETE /admin/users/{id}/totp.
func (c *TOTPController) AdminReset(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	if err := c.twoFactor.AdminReset(r.Context(), targetID); err != nil {
		httputil.Error(w, http.StatusNotFound, "NOT_FOUND", "User not found")
		return
	}
	actor := middleware.UserFromContext(r.Context())
	slog.Warn("two-factor authentication reset by administrator", "user_id", targetID, "admin_id", actor.ID)
	httputil.JSON(w, http.StatusOK, map[string]bool{"enabled": false})
}

func writeTOTPError(w http.ResponseWriter, err error) {
	if wait, blocked := attemptsBlocked(err); blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
		httputil.Error(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", tooManyCodesMessage(wait))
		return
	}
	switch {
	case errors.Is(err, service.ErrIncorrectPassword):
		httputil.Error(w, http.StatusBadRequest, "INVALID_PASSWORD", errorText(err))
	case errors.Is(err, service.ErrInvalidTOTPCode):
		httputil.Error(w, http.StatusBadRequest, "INVALID_TOTP_CODE", errorText(err))
	case errors.Is(err, service.ErrTOTPEnforced):
		httputil.Error(w, http.StatusForbidden, "TOTP_ENFORCED", errorText(err))
	case errors.Is(err, service.ErrTOTPAlreadyEnabled),
		errors.Is(err, service.ErrTOTPNotEnabled),
		errors.Is(err, service.ErrTOTPNoPendingSetup):
		httputil.Error(w, http.StatusConflict, "TOTP_STATE", errorText(err))
	default:
		slog.Error("two-factor operation failed", "err", err)
		httputil.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Two-factor operation failed")
	}
}
