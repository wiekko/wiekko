package router

import (
	"net/http"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

func mountPages(mux *http.ServeMux, svc *service.Services) {
	loginCtrl := controller.NewPageLogin(svc.Setting)
	dashCtrl := controller.NewPageDashboard(svc.Dashboard, svc.Setting)

	// Public pages.
	mux.HandleFunc("GET /login", loginCtrl.Show)

	// Authenticated pages.
	mux.Handle("GET /{$}", pageAuthed(http.HandlerFunc(dashCtrl.Show)))
}

// pageAuthed wraps a handler with RequireAuthPage middleware (redirect to /login).
func pageAuthed(h http.Handler) http.Handler {
	return middleware.RequireAuthPage(h)
}
