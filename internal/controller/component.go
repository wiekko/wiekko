package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// ComponentController handles component and functionality endpoints.
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

// --- Functionalities ---

// ListFunctionalities handles GET /products/{productId}/functionalities.
func (c *ComponentController) ListFunctionalities(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	items, err := c.component.ListFunctionalities(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, functionalityToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateFunctionality handles POST /products/{productId}/functionalities.
func (c *ComponentController) CreateFunctionality(w http.ResponseWriter, r *http.Request) {
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

	item, err := c.component.CreateFunctionality(r.Context(), user, service.CreateFunctionalityInput{
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

	httputil.JSON(w, http.StatusCreated, functionalityToJSON(item))
}

// GetFunctionality handles GET /products/{productId}/functionalities/{id}.
func (c *ComponentController) GetFunctionality(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.component.GetFunctionality(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, functionalityToJSON(item))
}

// UpdateFunctionality handles PATCH /products/{productId}/functionalities/{id}.
func (c *ComponentController) UpdateFunctionality(w http.ResponseWriter, r *http.Request) {
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

	item, err := c.component.UpdateFunctionality(r.Context(), user, productID, id, service.UpdateFunctionalityInput{
		Name:        req.Name,
		Description: req.Description,
		ComponentID: req.ComponentID,
		MilestoneID: req.MilestoneID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, functionalityToJSON(item))
}

// DeleteFunctionality handles DELETE /products/{productId}/functionalities/{id}.
func (c *ComponentController) DeleteFunctionality(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.component.DeleteFunctionality(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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

func functionalityToJSON(f *model.Functionality) map[string]any {
	return map[string]any{
		"id":           f.ID,
		"product_id":   f.ProductID,
		"component_id": f.ComponentID,
		"name":         f.Name,
		"description":  f.Description,
		"milestone_id": f.MilestoneID,
		"created_at":   f.CreatedAt,
		"updated_at":   f.UpdatedAt,
	}
}
