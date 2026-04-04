package router

import (
	"net/http"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

// New creates the top-level HTTP handler with all routes and middleware applied.
func New(svc *service.Services, cookieSecure, devMode bool) http.Handler {
	mux := http.NewServeMux()

	mountStatic(mux)
	mountPages(mux, svc, cookieSecure)
	mountAPIv1(mux, svc, cookieSecure)

	if devMode {
		dev := controller.NewDevController()
		mux.HandleFunc("GET /dev/catalogue", dev.Catalogue)
	}

	stack := middleware.Chain(
		middleware.RequestID,
		middleware.Logging,
		middleware.Recovery,
		middleware.SecurityHeaders,
		middleware.Authenticate(svc.Auth, svc.Auth),
	)

	return stack(mux)
}
