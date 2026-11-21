// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"
	"strings"

	"wiekko.net/internal/middleware"
	"wiekko.net/internal/model"
	"wiekko.net/internal/service"
	"wiekko.net/internal/store"
	"wiekko.net/web/templates"
	productpage "wiekko.net/web/templates/pages/product"
)

// Board renders GET /products/{id}/board — the kanban board for a product.
// Scrum-mode products are redirected to /sprints (matches the sidebar).
func (c *PageProductController) Board(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if p.WorkflowMode == model.WorkflowModeScrum {
		http.Redirect(w, r, "/products/"+productID+"/sprints", http.StatusSeeOther)
		return
	}

	reqStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "requirement")
	if err != nil {
		http.Error(w, "failed to load statuses", http.StatusInternalServerError)
		return
	}
	defStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	if err != nil {
		http.Error(w, "failed to load statuses", http.StatusInternalServerError)
		return
	}
	// Requirements and tasks share the requirement status set, so they share
	// the board columns too; an empty kind filter returns both.
	// Archived items are done with (FR-155); the board leaves them out.
	backlogItems, err := c.backlog.ListRequirements(r.Context(), user, productID, store.RequirementFilter{Archived: store.ArchivedExclude})
	if err != nil {
		http.Error(w, "failed to load requirements", http.StatusInternalServerError)
		return
	}
	defects, err := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{Archived: store.ArchivedExclude})
	if err != nil {
		http.Error(w, "failed to load defects", http.StatusInternalServerError)
		return
	}

	prefixes := c.buildBacklogPrefixMap(r, user, productID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.BoardPage(productpage.BoardPageProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Board",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Surface: buildBoardSurface(productID, p, canMoveCards(user, role), canMoveInactive(user, role),
			reqStatuses, defStatuses, backlogItems, defects, prefixes, ""),
	}).Render(r.Context(), w)
}

// DefectsBoard renders GET /products/{id}/defects/board — the defect-only board.
// It exists in both workflow modes: defects are triaged continuously whether or
// not the backlog runs in sprints.
func (c *PageProductController) DefectsBoard(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	csrfToken := middleware.CSRFTokenFromContext(r.Context())
	productID := r.PathValue("id")

	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	defStatuses, err := c.milestone.ListStatuses(r.Context(), user, productID, "defect")
	if err != nil {
		http.Error(w, "failed to load statuses", http.StatusInternalServerError)
		return
	}
	defects, err := c.backlog.ListDefects(r.Context(), user, productID, store.DefectFilter{Archived: store.ArchivedExclude})
	if err != nil {
		http.Error(w, "failed to load defects", http.StatusInternalServerError)
		return
	}

	defCards, strandedDef := buildDefectCards(productID, defects, canMoveInactive(user, role))
	defColumns := entityBoardColumns(defStatuses, defCards, false)
	if p.WorkflowMode == model.WorkflowModeKanban {
		showActiveColumns(defColumns)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DefectsBoardPage(productpage.DefectsBoardPageProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Defects Board",
			CSRFToken: csrfToken,
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Surface: templates.BoardSurfaceProps{
			ProductID:   productID,
			CanMove:     canMoveCards(user, role),
			Columns:     defColumns,
			StrandedDef: strandedDef,
			GroupPrefix: "defects",
		},
	}).Render(r.Context(), w)
}

// BoardRequirementStatus handles PATCH /products/{id}/board/requirements/{reqId}/status.
// Expects a form-encoded `status_id`. Returns 204 on success.
func (c *PageProductController) BoardRequirementStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	reqID := r.PathValue("reqId")

	statusID := strings.TrimSpace(r.FormValue("status_id"))
	_, err := c.backlog.UpdateRequirement(r.Context(), user, productID, reqID, service.UpdateRequirementInput{
		IfUpdatedAt: r.FormValue("updated_at"),
		StatusID:    &statusID,
	})
	if err != nil {
		http.Error(w, errorText(err), http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BoardTaskStatus handles PATCH /products/{id}/board/tasks/{taskId}/status.
// Tasks live in the requirements table and use the requirement status set, so
// this is the requirement move with a URL that reads honestly.
func (c *PageProductController) BoardTaskStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	taskID := r.PathValue("taskId")

	statusID := strings.TrimSpace(r.FormValue("status_id"))
	_, err := c.backlog.UpdateRequirement(r.Context(), user, productID, taskID, service.UpdateRequirementInput{
		IfUpdatedAt: r.FormValue("updated_at"),
		StatusID:    &statusID,
	})
	if err != nil {
		http.Error(w, errorText(err), http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BoardDefectStatus handles PATCH /products/{id}/board/defects/{defectId}/status.
func (c *PageProductController) BoardDefectStatus(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	defectID := r.PathValue("defectId")

	statusID := strings.TrimSpace(r.FormValue("status_id"))
	_, err := c.backlog.UpdateDefect(r.Context(), user, productID, defectID, service.UpdateDefectInput{
		IfUpdatedAt: r.FormValue("updated_at"),
		StatusID:    &statusID,
	})
	if err != nil {
		http.Error(w, errorText(err), http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
