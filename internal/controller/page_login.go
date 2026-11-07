package controller

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
	login "nexmap.net/web/templates/pages/login"
)

// PageLoginController handles the login and logout pages.
type PageLoginController struct {
	auth         *service.AuthService
	settings     *service.SettingService
	cookieSecure bool
}

// NewPageLogin creates a PageLoginController.
func NewPageLogin(auth *service.AuthService, settings *service.SettingService, cookieSecure bool) *PageLoginController {
	return &PageLoginController{auth: auth, settings: settings, cookieSecure: cookieSecure}
}

// Show renders the login page. Already-authenticated users are redirected to /.
func (c *PageLoginController) Show(w http.ResponseWriter, r *http.Request) {
	if middleware.UserFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	selfReg, _ := c.settings.Get(r.Context(), "self_registration_enabled")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = login.LoginPage(selfReg == "true", "").Render(r.Context(), w)
}

// Login handles POST /login. Authenticates the user and redirects to /.
func (c *PageLoginController) Login(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		c.renderLoginError(w, r, "Username and password are required")
		return
	}

	ip := clientIP(r)

	result, err := c.auth.Login(r.Context(), username, password, ip, r.UserAgent())
	if err != nil {
		logFailedLogin(r, username, ip, err)
		c.renderLoginError(w, r, "Invalid username or password")
		return
	}

	if result.TOTPRequired {
		c.setChallengeCookie(w, result.ChallengeToken)
		c.renderTOTPForm(w, r, "")
		return
	}

	c.finishLogin(w, r, result.SessionID)
}

// LoginTOTP handles POST /login/totp, the second login step for users with
// two-factor authentication.
func (c *PageLoginController) LoginTOTP(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(challengeCookieName)
	if err != nil || cookie.Value == "" {
		c.clearChallengeCookie(w)
		c.renderLoginError(w, r, service.ErrLoginChallengeInvalid.Error())
		return
	}

	code := r.FormValue("code")
	if code == "" {
		c.renderTOTPForm(w, r, "Enter the code from your authenticator app.")
		return
	}

	ip := clientIP(r)
	result, err := c.auth.CompleteTOTPLogin(r.Context(), cookie.Value, code, ip, r.UserAgent())
	if err != nil {
		logFailedLogin(r, "", ip, fmt.Errorf("second factor: %w", err))
		if errors.Is(err, service.ErrLoginChallengeInvalid) {
			c.clearChallengeCookie(w)
			c.renderLoginError(w, r, err.Error())
			return
		}
		if errors.Is(err, service.ErrTOTPCodeReused) {
			c.renderTOTPForm(w, r, "That code was already used. Wait for your app to show the next code.")
			return
		}
		c.renderTOTPForm(w, r, "Invalid authentication code")
		return
	}

	c.clearChallengeCookie(w)
	c.finishLogin(w, r, result.SessionID)
}

// challengeCookieName holds the pending second-step token between the
// password and the code form. Scoped to /login so nothing else sees it.
const challengeCookieName = "nexmap_login_challenge"

func (c *PageLoginController) setChallengeCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     challengeCookieName,
		Value:    token,
		Path:     "/login",
		MaxAge:   int(service.LoginChallengeTTL.Seconds()),
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (c *PageLoginController) clearChallengeCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     challengeCookieName,
		Value:    "",
		Path:     "/login",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// finishLogin sets the session cookie and sends the browser to the dashboard.
func (c *PageLoginController) finishLogin(w http.ResponseWriter, r *http.Request, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "nexmap_session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (c *PageLoginController) renderTOTPForm(w http.ResponseWriter, r *http.Request, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		_ = login.TOTPForm(message).Render(r.Context(), w)
		return
	}
	if message != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	_ = login.TOTPPage(message).Render(r.Context(), w)
}

// clientIP returns the address recorded for sessions and failed-login logs.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	return r.RemoteAddr
}

func (c *PageLoginController) renderLoginError(w http.ResponseWriter, r *http.Request, message string) {
	selfReg, _ := c.settings.Get(r.Context(), "self_registration_enabled")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		// Return just the form fragment for htmx swap. It must be 2xx: htmx's
		// default responseHandling drops the body of any 4xx, which would leave
		// the user with no error message at all.
		_ = login.LoginForm(selfReg == "true", message).Render(r.Context(), w)
		return
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = login.LoginPage(selfReg == "true", message).Render(r.Context(), w)
}

// Logout handles POST /logout. Validates the CSRF token from the form,
// destroys the session, clears the cookie, and redirects to /login.
func (c *PageLoginController) Logout(w http.ResponseWriter, r *http.Request) {
	// Validate CSRF token from form body.
	expected := middleware.CSRFTokenFromContext(r.Context())
	provided := r.FormValue("csrf_token")
	if expected == "" || provided == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Destroy session.
	cookie, err := r.Cookie("nexmap_session")
	if err == nil {
		_ = c.auth.Logout(r.Context(), cookie.Value)
	}

	// Clear session cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     "nexmap_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
