package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
)

// ConstraintController handles constraint endpoints.
type ConstraintController struct {
	constraint *service.ConstraintService
}

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

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, constraintToJSON(item))
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

	httputil.JSON(w, http.StatusCreated, constraintToJSON(item))
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

	httputil.JSON(w, http.StatusOK, constraintToJSON(item))
}

// UpdateConstraint handles PATCH /products/{productId}/constraints/{id}.
func (c *ConstraintController) UpdateConstraint(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
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

	httputil.JSON(w, http.StatusOK, constraintToJSON(item))
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

func constraintToJSON(c *model.Constraint) map[string]any {
	return map[string]any{
		"id":               c.ID,
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
