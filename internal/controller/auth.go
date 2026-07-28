package controller

import (
	"log/slog"
	"net/http"

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
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Username == "" || req.Password == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Username and password are required")
		return
	}

	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = fwd
	}

	result, err := c.auth.Login(r.Context(), req.Username, req.Password, ip, r.UserAgent())
	if err != nil {
		logFailedLogin(r, req.Username, ip, err)
		httputil.Error(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password")
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
	})
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "REGISTRATION_FAILED", err.Error())
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
	if req.Token == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Token, username, email, and password are required")
		return
	}

	user, err := c.user.AcceptInvitation(r.Context(), req.Token, service.RegisterInput{
		Username:    req.Username,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
	})
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVITATION_FAILED", err.Error())
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
