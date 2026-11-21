// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"

	"wiekko.net/internal/httputil"
	"wiekko.net/internal/middleware"
	"wiekko.net/internal/model"
	"wiekko.net/internal/service"
	"wiekko.net/internal/store"
)

// ConstraintController handles constraint endpoints.
type ConstraintController struct {
	constraint *service.ConstraintService
	documents  *service.DocumentService
}

// UseDocuments makes single-item responses name the documents linked to the
// item (FR-141).
func (c *ConstraintController) UseDocuments(docs *service.DocumentService) { c.documents = docs }

// NewConstraint creates a ConstraintController.
func NewConstraint(constraint *service.ConstraintService) *ConstraintController {
	return &ConstraintController{constraint: constraint}
}

// ListConstraints handles GET /products/{productId}/constraints.
func (c *ConstraintController) ListConstraints(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	filter := store.ConstraintFilter{
		CategoryID: r.URL.Query().Get("category_id"),
	}

	items, err := c.constraint.ListConstraints(r.Context(), user, productID, filter)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	prefixes := c.prefixes(r, user, productID)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, constraintToJSON(item, prefixes))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateConstraint handles POST /products/{productId}/constraints.
func (c *ConstraintController) CreateConstraint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		CategoryID      string `json:"category_id"`
		Title           string `json:"title"`
		Description     string `json:"description"`
		ComplianceState string `json:"compliance_state"`
		MilestoneID     string `json:"milestone_id"`
		Priority        int    `json:"priority"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.constraint.CreateConstraint(r.Context(), user, service.CreateConstraintInput{
		ProductID:       productID,
		CategoryID:      req.CategoryID,
		Title:           req.Title,
		Description:     req.Description,
		ComplianceState: req.ComplianceState,
		MilestoneID:     req.MilestoneID,
		Priority:        req.Priority,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, constraintToJSON(item, c.prefixes(r, user, productID)))
}

// GetConstraint handles GET /products/{productId}/constraints/{id}.
func (c *ConstraintController) GetConstraint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.constraint.GetConstraint(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, withLinkedDocuments(r, c.documents, model.DocumentLinkConstraint, item.ID,
		constraintToJSON(item, c.prefixes(r, user, productID))))
}

// UpdateConstraint handles PATCH /products/{productId}/constraints/{id}.
func (c *ConstraintController) UpdateConstraint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		// IfUpdatedAt is the updated_at the client edited: a change made
		// meanwhile turns the update into 409 CONFLICT.
		IfUpdatedAt     string  `json:"if_updated_at"`
		CategoryID      *string `json:"category_id"`
		Title           *string `json:"title"`
		Description     *string `json:"description"`
		ComplianceState *string `json:"compliance_state"`
		MilestoneID     *string `json:"milestone_id"`
		Priority        *int    `json:"priority"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.constraint.UpdateConstraint(r.Context(), user, productID, id, service.UpdateConstraintInput{
		IfUpdatedAt:     req.IfUpdatedAt,
		CategoryID:      req.CategoryID,
		Title:           req.Title,
		Description:     req.Description,
		ComplianceState: req.ComplianceState,
		MilestoneID:     req.MilestoneID,
		Priority:        req.Priority,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, constraintToJSON(item, c.prefixes(r, user, productID)))
}

// DeleteConstraint handles DELETE /products/{productId}/constraints/{id}.
func (c *ConstraintController) DeleteConstraint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.constraint.DeleteConstraint(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListConstraintCapabilities handles GET /products/{productId}/constraints/{id}/capabilities.
func (c *ConstraintController) ListConstraintCapabilities(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	links, err := c.constraint.ListConstraintCapabilities(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(links))
	for _, link := range links {
		result = append(result, map[string]any{
			"id":            link.ID,
			"constraint_id": link.ConstraintID,
			"capability_id": link.CapabilityID,
			"created_at":    link.CreatedAt,
		})
	}
	httputil.JSON(w, http.StatusOK, result)
}

// ListCapabilityConstraints handles GET /products/{productId}/capabilities/{id}/constraints,
// the reverse of ListConstraintCapabilities (DF-1).
func (c *ConstraintController) ListCapabilityConstraints(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.constraint.ListCapabilityConstraints(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	prefixes := c.prefixes(r, user, productID)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, constraintToJSON(item, prefixes))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// LinkConstraintCapability handles POST /products/{productId}/constraints/{id}/capabilities.
func (c *ConstraintController) LinkConstraintCapability(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		CapabilityID string `json:"capability_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.constraint.LinkCapability(r.Context(), user, productID, id, req.CapabilityID); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UnlinkConstraintCapability handles DELETE /products/{productId}/constraints/{id}/capabilities/{capId}.
func (c *ConstraintController) UnlinkConstraintCapability(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")
	capID := r.PathValue("capId")

	if err := c.constraint.UnlinkCapability(r.Context(), user, productID, id, capID); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// prefixes returns the constraint category prefix map used to render
// identifiers; see BacklogController.prefixes.
func (c *ConstraintController) prefixes(r *http.Request, user *model.User, productID string) map[string]string {
	m, err := c.constraint.CategoryPrefixes(r.Context(), user, productID)
	if err != nil {
		return nil
	}
	return m
}

// constraintToJSON renders a constraint. identifier ("AC-42") is empty when the
// category prefix is unknown.
func constraintToJSON(c *model.Constraint, prefixes map[string]string) map[string]any {
	identifier := ""
	if p, ok := prefixes[c.CategoryID]; ok {
		identifier = model.EntityIdentifier(p, c.SequenceNumber)
	}
	return map[string]any{
		"id":               c.ID,
		"identifier":       identifier,
		"product_id":       c.ProductID,
		"category_id":      c.CategoryID,
		"sequence_number":  c.SequenceNumber,
		"title":            c.Title,
		"description":      c.Description,
		"compliance_state": c.ComplianceState,
		"milestone_id":     c.MilestoneID,
		"priority":         c.Priority,
		"created_at":       c.CreatedAt,
		"updated_at":       c.UpdatedAt,
	}
}
