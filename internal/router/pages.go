package router

import (
	"net/http"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

func mountPages(mux *http.ServeMux, svc *service.Services, cookieSecure bool) {
	loginCtrl := controller.NewPageLogin(svc.Auth, svc.Setting, cookieSecure)
	dashCtrl := controller.NewPageDashboard(svc.Dashboard, svc.Setting)
	adminCtrl := controller.NewPageAdmin(svc.User, svc.Setting, svc.Invitation)

	// Public pages.
	mux.HandleFunc("GET /login", loginCtrl.Show)

	// Authenticated pages.
	mux.Handle("GET /{$}", pageAuthed(http.HandlerFunc(dashCtrl.Show)))
	mux.Handle("POST /logout", pageAuthed(http.HandlerFunc(loginCtrl.Logout)))

	// Admin pages.
	mux.Handle("GET /admin/users", pageAdmin(http.HandlerFunc(adminCtrl.ListUsers)))
	mux.Handle("GET /admin/users/{id}", pageAdmin(http.HandlerFunc(adminCtrl.GetUser)))
	mux.Handle("GET /admin/settings", pageAdmin(http.HandlerFunc(adminCtrl.Settings)))
	mux.Handle("GET /admin/invitations", pageAdmin(http.HandlerFunc(adminCtrl.Invitations)))
}

// pageAuthed wraps a handler with RequireAuthPage middleware (redirect to /login).
func pageAuthed(h http.Handler) http.Handler {
	return middleware.RequireAuthPage(h)
}

// pageAdmin wraps a handler with RequireAdminPage middleware (redirect to / for non-admins).
func pageAdmin(h http.Handler) http.Handler {
	return middleware.RequireAdminPage(h)
}
