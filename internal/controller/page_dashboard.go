// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
	"nexmap.net/web/templates"
	dashboard "nexmap.net/web/templates/pages/dashboard"
)

// PageDashboardController handles the dashboard page.
type PageDashboardController struct {
	dashboard *service.DashboardService
	settings  *service.SettingService
}

// NewPageDashboard creates a PageDashboardController.
func NewPageDashboard(dashboard *service.DashboardService, settings *service.SettingService) *PageDashboardController {
	return &PageDashboardController{dashboard: dashboard, settings: settings}
}

// Show renders the dashboard page with product cards.
func (c *PageDashboardController) Show(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())

	summaries, err := c.dashboard.ProductSummaries(r.Context(), user)
	if err != nil {
		summaries = nil
	}

	productCreation, _ := c.settings.Get(r.Context(), "product_creation_enabled")
	canCreate := user.IsAdmin || productCreation == "true"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dashboard.DashboardPage(dashboard.DashboardProps{
		Meta: templates.PageMeta{
			Title:     "Dashboard",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Products:    summaries,
		CanCreate:   canCreate,
	}).Render(r.Context(), w)
}
