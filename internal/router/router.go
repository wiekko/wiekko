package router

import (
	"net/http"

	"nexmap.net/internal/middleware"
)

// New creates the top-level HTTP handler with all routes and middleware applied.
func New() http.Handler {
	mux := http.NewServeMux()

	mountStatic(mux)
	mountAPIv1(mux)

	stack := middleware.Chain(
		middleware.RequestID,
		middleware.Logging,
		middleware.Recovery,
		middleware.SecurityHeaders,
	)

	return stack(mux)
}
