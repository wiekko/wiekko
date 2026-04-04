package controller

import (
	"net/http"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
	login "nexmap.net/web/templates/pages/login"
)

// PageLoginController handles the login page.
type PageLoginController struct {
	settings *service.SettingService
}

// NewPageLogin creates a PageLoginController.
func NewPageLogin(settings *service.SettingService) *PageLoginController {
	return &PageLoginController{settings: settings}
}

// Show renders the login page. Already-authenticated users are redirected to /.
func (c *PageLoginController) Show(w http.ResponseWriter, r *http.Request) {
	if middleware.UserFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	selfReg, _ := c.settings.Get(r.Context(), "self_registration_enabled")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = login.LoginPage(selfReg == "true").Render(r.Context(), w)
}
