// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"net/http"
	"net/netip"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

// Options are the settings the HTTP layer needs besides the services.
type Options struct {
	// CookieSecure says the server is reached over HTTPS: cookies are marked
	// Secure and HSTS is sent.
	CookieSecure bool
	// DevMode mounts the component catalogue.
	DevMode bool
	// MaxUploadBytes is the largest accepted attachment.
	MaxUploadBytes int64
	// TrustedProxies are the addresses whose X-Forwarded-For header is
	// believed (see middleware.ClientIP).
	TrustedProxies []netip.Prefix
	// PublicURL is the address users reach the server at, if configured.
	PublicURL string
}

// New creates the top-level HTTP handler with all routes and middleware applied.
func New(svc *service.Services, opts Options) (http.Handler, error) {
	mux := http.NewServeMux()

	crossOrigin, err := middleware.CrossOrigin(opts.PublicURL)
	if err != nil {
		return nil, err
	}

	mountStatic(mux)
	mountPages(mux, svc, opts.CookieSecure, opts.MaxUploadBytes)
	mountAPIv1(mux, svc, opts.CookieSecure)

	if opts.DevMode {
		dev := controller.NewDevController()
		mux.HandleFunc("GET /dev/catalogue", dev.Catalogue)
	}

	stack := middleware.Chain(
		middleware.RequestID,
		middleware.ClientIP(opts.TrustedProxies),
		middleware.Logging,
		middleware.Recovery,
		middleware.SecurityHeaders(opts.CookieSecure),
		crossOrigin,
		middleware.LimitBody(opts.MaxUploadBytes),
		middleware.Authenticate(svc.Auth, svc.Auth),
		middleware.RequireTwoFactorEnrollment(svc.TwoFactor),
	)

	return stack(mux), nil
}
