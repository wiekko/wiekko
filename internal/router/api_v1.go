package router

import (
	"net/http"

	"nexmap.net/internal/controller"
)

func mountAPIv1(mux *http.ServeMux) {
	apiV1 := http.NewServeMux()

	// Health
	apiV1.HandleFunc("GET /health", controller.Health)

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiV1))
}
