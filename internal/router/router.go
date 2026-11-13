package router

import (
	"net/http"
	"net/netip"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

// New creates the top-level HTTP handler with all routes and middleware applied.
//
// trustedProxies are the addresses whose X-Forwarded-For header is believed
// (see middleware.ClientIP). cookieSecure says the server is reached over
// HTTPS, which also turns on HSTS.
func New(svc *service.Services, cookieSecure, devMode bool, maxUploadBytes int64, trustedProxies []netip.Prefix) http.Handler {
	mux := http.NewServeMux()

	mountStatic(mux)
	mountPages(mux, svc, cookieSecure, maxUploadBytes)
	mountAPIv1(mux, svc, cookieSecure)

	if devMode {
		dev := controller.NewDevController()
		mux.HandleFunc("GET /dev/catalogue", dev.Catalogue)
	}

	stack := middleware.Chain(
		middleware.RequestID,
		middleware.ClientIP(trustedProxies),
		middleware.Logging,
		middleware.Recovery,
		middleware.SecurityHeaders(cookieSecure),
		middleware.LimitBody(maxUploadBytes),
		middleware.Authenticate(svc.Auth, svc.Auth),
		middleware.RequireTwoFactorEnrollment(svc.TwoFactor),
	)

	return stack(mux)
}
