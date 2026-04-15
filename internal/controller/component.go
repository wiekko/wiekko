package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// ComponentController handles component and capability endpoints.
type ComponentController struct {
	component *service.ComponentService
}

// NewComponent creates a ComponentController.
func NewComponent(component *service.ComponentService) *ComponentController {
	return &ComponentController{component: component}
}

// --- Components ---

// ListComponents handles GET /products/{productId}/components.
func (c *ComponentController) ListComponents(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.component.ListComponents(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, componentToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateComponent handles POST /products/{productId}/components.
func (c *ComponentController) CreateComponent(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		MilestoneID string `json:"milestone_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.component.CreateComponent(r.Context(), user, service.CreateComponentInput{
		ProductID:   productID,
		Name:        req.Name,
		Description: req.Description,
		MilestoneID: req.MilestoneID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, componentToJSON(item))
}

// GetComponent handles GET /products/{productId}/components/{id}.
func (c *ComponentController) GetComponent(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.component.GetComponent(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, componentToJSON(item))
}

// UpdateComponent handles PATCH /products/{productId}/components/{id}.
func (c *ComponentController) UpdateComponent(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		MilestoneID *string `json:"milestone_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.component.UpdateComponent(r.Context(), user, productID, id, service.UpdateComponentInput{
		Name:        req.Name,
		Description: req.Description,
		MilestoneID: req.MilestoneID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, componentToJSON(item))
}

// DeleteComponent handles DELETE /products/{productId}/components/{id}.
func (c *ComponentController) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.component.DeleteComponent(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Capabilities ---

// ListCapabilities handles GET /products/{productId}/capabilities.
func (c *ComponentController) ListCapabilities(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.component.ListCapabilities(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, capabilityToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateCapability handles POST /products/{productId}/capabilities.
func (c *ComponentController) CreateCapability(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		ComponentID string `json:"component_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		MilestoneID string `json:"milestone_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.component.CreateCapability(r.Context(), user, service.CreateCapabilityInput{
		ProductID:   productID,
		ComponentID: req.ComponentID,
		Name:        req.Name,
		Description: req.Description,
		MilestoneID: req.MilestoneID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, capabilityToJSON(item))
}

// GetCapability handles GET /products/{productId}/capabilities/{id}.
func (c *ComponentController) GetCapability(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.component.GetCapability(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, capabilityToJSON(item))
}

// UpdateCapability handles PATCH /products/{productId}/capabilities/{id}.
func (c *ComponentController) UpdateCapability(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		ComponentID *string `json:"component_id"`
		MilestoneID *string `json:"milestone_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.component.UpdateCapability(r.Context(), user, productID, id, service.UpdateCapabilityInput{
		Name:        req.Name,
		Description: req.Description,
		ComponentID: req.ComponentID,
		MilestoneID: req.MilestoneID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, capabilityToJSON(item))
}

// DeleteCapability handles DELETE /products/{productId}/capabilities/{id}.
func (c *ComponentController) DeleteCapability(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.component.DeleteCapability(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Capability-Requirement Links ---

// ListCapabilityRequirements handles GET /products/{productId}/capabilities/{id}/requirements.
func (c *ComponentController) ListCapabilityRequirements(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.component.ListCapabilityRequirements(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, requirementCapabilityToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// LinkCapabilityRequirement handles POST /products/{productId}/capabilities/{id}/requirements.
func (c *ComponentController) LinkCapabilityRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		RequirementID string `json:"requirement_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.component.LinkRequirementToCapability(r.Context(), user, productID, id, req.RequirementID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "linked"})
}

// UnlinkCapabilityRequirement handles DELETE /products/{productId}/capabilities/{id}/requirements/{reqId}.
func (c *ComponentController) UnlinkCapabilityRequirement(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")
	reqID := r.PathValue("reqId")

	if err := c.component.UnlinkRequirementFromCapability(r.Context(), user, productID, id, reqID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

// ListRequirementCapabilities handles GET /products/{productId}/requirements/{id}/capabilities.
func (c *ComponentController) ListRequirementCapabilities(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	items, err := c.component.ListRequirementCapabilities(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, requirementCapabilityToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// LinkRequirementCapability handles POST /products/{productId}/requirements/{id}/capabilities.
func (c *ComponentController) LinkRequirementCapability(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	reqID := r.PathValue("id")

	var req struct {
		CapabilityID string `json:"capability_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.component.LinkRequirementToCapability(r.Context(), user, productID, req.CapabilityID, reqID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "linked"})
}

// UnlinkRequirementCapability handles DELETE /products/{productId}/requirements/{id}/capabilities/{capId}.
func (c *ComponentController) UnlinkRequirementCapability(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	reqID := r.PathValue("id")
	capID := r.PathValue("capId")

	if err := c.component.UnlinkRequirementFromCapability(r.Context(), user, productID, capID, reqID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

func componentToJSON(c *model.Component) map[string]any {
	return map[string]any{
		"id":           c.ID,
		"product_id":   c.ProductID,
		"name":         c.Name,
		"description":  c.Description,
		"milestone_id": c.MilestoneID,
		"created_at":   c.CreatedAt,
		"updated_at":   c.UpdatedAt,
	}
}

func capabilityToJSON(cap *model.Capability) map[string]any {
	return map[string]any{
		"id":           cap.ID,
		"product_id":   cap.ProductID,
		"component_id": cap.ComponentID,
		"name":         cap.Name,
		"description":  cap.Description,
		"milestone_id": cap.MilestoneID,
		"created_at":   cap.CreatedAt,
		"updated_at":   cap.UpdatedAt,
	}
}

func requirementCapabilityToJSON(rc *model.RequirementCapability) map[string]any {
	return map[string]any{
		"id":             rc.ID,
		"requirement_id": rc.RequirementID,
		"capability_id":  rc.CapabilityID,
		"created_at":     rc.CreatedAt,
	}
}
