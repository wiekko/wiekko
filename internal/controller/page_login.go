package controller

import (
	"crypto/subtle"
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

	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = fwd
	}

	result, err := c.auth.Login(r.Context(), username, password, ip, r.UserAgent())
	if err != nil {
		c.renderLoginError(w, r, "Invalid username or password")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "nexmap_session",
		Value:    result.SessionID,
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
