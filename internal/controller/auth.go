// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// logFailedLogin records a rejected login attempt. The access log alone does not
// identify these: the page form answers with 200 so htmx swaps the error back
// into the page, and neither log line carries the username or the reason.
func logFailedLogin(r *http.Request, username, ip string, err error) {
	slog.Warn("failed login",
		"username", username,
		"ip", ip,
		"user_agent", r.UserAgent(),
		"reason", err.Error(),
		"request_id", middleware.RequestIDFromContext(r.Context()),
	)
}

// loginBlocked reports whether err means the client has to wait before the
// next login attempt, and for how long (rounded up to whole seconds).
func loginBlocked(err error) (time.Duration, bool) {
	var blocked *service.LoginBlockedError
	if !errors.As(err, &blocked) {
		return 0, false
	}
	return blocked.RetryAfter.Round(time.Second) + time.Second, true
}

// attemptsBlocked reports whether err means the second factor or account
// creation is blocked for now, and for how long (rounded up to whole seconds).
func attemptsBlocked(err error) (time.Duration, bool) {
	var blocked *service.AttemptsBlockedError
	if !errors.As(err, &blocked) {
		return 0, false
	}
	return blocked.RetryAfter.Round(time.Second) + time.Second, true
}

// writeAccountsBlocked answers 429 if err means the client has created too
// many accounts for now, and reports whether it did.
func writeAccountsBlocked(w http.ResponseWriter, err error) bool {
	wait, blocked := attemptsBlocked(err)
	if !blocked {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
	httputil.Error(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", tooManyAccountsMessage(wait))
	return true
}

func tooManyCodesMessage(wait time.Duration) string {
	return fmt.Sprintf("Too many wrong authentication codes. Try again in %s.", waitMinutes(wait))
}

func tooManyAccountsMessage(wait time.Duration) string {
	return fmt.Sprintf("Too many accounts were created from your network. Try again in %s.", waitMinutes(wait))
}

func waitMinutes(wait time.Duration) string {
	minutes := int(math.Ceil(wait.Minutes()))
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

func tooManyAttemptsMessage(wait time.Duration) string {
	minutes := int(math.Ceil(wait.Minutes()))
	unit := "minutes"
	if minutes == 1 {
		unit = "minute"
	}
	return fmt.Sprintf("Too many failed sign-in attempts. Try again in %d %s.", minutes, unit)
}

// currentSessionID returns the web session the request was made with, or ""
// for requests authenticated otherwise (access tokens).
func currentSessionID(r *http.Request) string {
	if middleware.AuthMethodFromContext(r.Context()) != "session" {
		return ""
	}
	cookie, err := r.Cookie("nexmap_session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

// AuthController handles authentication endpoints.
type AuthController struct {
	auth         *service.AuthService
	user         *service.UserService
	cookieSecure bool
}

// NewAuth creates an AuthController.
func NewAuth(auth *service.AuthService, user *service.UserService, cookieSecure bool) *AuthController {
	return &AuthController{auth: auth, user: user, cookieSecure: cookieSecure}
}

// Login handles POST /auth/login.
func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		// TOTPCode lets clients send the second factor with the password in
		// one request. Without it, users with 2FA get a TOTP_REQUIRED answer
		// carrying a challenge for POST /auth/login/totp.
		TOTPCode string `json:"totp_code"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Username == "" || req.Password == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Username and password are required")
		return
	}

	ip := clientIP(r)

	result, err := c.auth.Login(r.Context(), req.Username, req.Password, ip, r.UserAgent())
	if err != nil {
		logFailedLogin(r, req.Username, ip, err)
		if wait, blocked := loginBlocked(err); blocked {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
			httputil.Error(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", tooManyAttemptsMessage(wait))
			return
		}
		httputil.Error(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password")
		return
	}

	if result.TOTPRequired {
		if req.TOTPCode == "" {
			httputil.ErrorWithDetails(w, http.StatusUnauthorized, "TOTP_REQUIRED",
				"Two-factor authentication code required", map[string]any{
					"challenge":            result.ChallengeToken,
					"challenge_expires_in": int(service.LoginChallengeTTL.Seconds()),
				})
			return
		}
		c.completeTOTP(w, r, result.ChallengeToken, req.TOTPCode, req.Username, ip)
		return
	}

	c.writeSession(w, result)
}

// LoginTOTP handles POST /auth/login/totp, the second step of a login for
// users with two-factor authentication.
func (c *AuthController) LoginTOTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Challenge == "" || req.Code == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Challenge and code are required")
		return
	}
	c.completeTOTP(w, r, req.Challenge, req.Code, "", clientIP(r))
}

func (c *AuthController) completeTOTP(w http.ResponseWriter, r *http.Request, challenge, code, username, ip string) {
	result, err := c.auth.CompleteTOTPLogin(r.Context(), challenge, code, ip, r.UserAgent())
	if err != nil {
		logFailedLogin(r, username, ip, fmt.Errorf("second factor: %w", err))
		if wait, blocked := attemptsBlocked(err); blocked {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
			httputil.Error(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", tooManyCodesMessage(wait))
			return
		}
		if errors.Is(err, service.ErrLoginChallengeInvalid) {
			httputil.Error(w, http.StatusUnauthorized, "CHALLENGE_INVALID", errorText(err))
			return
		}
		httputil.Error(w, http.StatusUnauthorized, "INVALID_TOTP_CODE", "Invalid authentication code")
		return
	}
	c.writeSession(w, result)
}

func (c *AuthController) writeSession(w http.ResponseWriter, result *service.LoginResult) {
	http.SetCookie(w, &http.Cookie{
		Name:     "nexmap_session",
		Value:    result.SessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})

	httputil.JSON(w, http.StatusOK, map[string]any{
		"csrf_token": result.CSRFToken,
		"user":       userToJSON(result.User),
	})
}

// Logout handles POST /auth/logout.
func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("nexmap_session")
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "NO_SESSION", "No active session")
		return
	}
	_ = c.auth.Logout(r.Context(), cookie.Value)

	http.SetCookie(w, &http.Cookie{
		Name:     "nexmap_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// Register handles POST /auth/register.
func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Username, email, and password are required")
		return
	}

	user, err := c.user.Register(r.Context(), service.RegisterInput{
		Username:    req.Username,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		ClientIP:    clientIP(r),
	})
	if err != nil {
		if writeAccountsBlocked(w, err) {
			return
		}
		writeAccountError(w, "REGISTRATION_FAILED", err)
		return
	}

	httputil.JSON(w, http.StatusCreated, userToJSON(user))
}

// AcceptInvite handles POST /auth/accept-invite.
func (c *AuthController) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		Username    string `json:"username"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	// Email may be left out: the account then gets the invited address.
	if req.Token == "" || req.Username == "" || req.Password == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Token, username and password are required")
		return
	}

	user, err := c.user.AcceptInvitation(r.Context(), req.Token, service.RegisterInput{
		Username:    req.Username,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		ClientIP:    clientIP(r),
	})
	if err != nil {
		if writeAccountsBlocked(w, err) {
			return
		}
		writeAccountError(w, "INVITATION_FAILED", err)
		return
	}

	httputil.JSON(w, http.StatusCreated, userToJSON(user))
}

func userToJSON(u *model.User) map[string]any {
	return map[string]any{
		"id":           u.ID,
		"username":     u.Username,
		"email":        u.Email,
		"display_name": u.DisplayName,
		"is_admin":     u.IsAdmin,
		"status":       u.Status,
	}
}
