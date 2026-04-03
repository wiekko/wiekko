package controller

import (
	"net/http"

	templates "nexmap.net/web/templates/pages/dev"
)

// DevController handles development-only routes.
type DevController struct{}

// NewDevController creates a new DevController.
func NewDevController() *DevController {
	return &DevController{}
}

// Catalogue renders the component catalogue page.
func (c *DevController) Catalogue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = templates.CataloguePage().Render(r.Context(), w)
}
