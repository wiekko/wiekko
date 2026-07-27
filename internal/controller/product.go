package controller

import (
	"errors"
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
)

// ProductController handles product CRUD and member management endpoints.
type ProductController struct {
	product *service.ProductService
}

// NewProduct creates a ProductController.
func NewProduct(product *service.ProductService) *ProductController {
	return &ProductController{product: product}
}

// List handles GET /products.
func (c *ProductController) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	products, err := c.product.List(r.Context(), user)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "LIST_FAILED", "Failed to list products")
		return
	}

	items := make([]map[string]any, 0, len(products))
	for _, p := range products {
		items = append(items, productToJSON(p))
	}
	httputil.JSON(w, http.StatusOK, items)
}

// Create handles POST /products.
func (c *ProductController) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	var req struct {
		Name           string `json:"name"`
		Description    string `json:"description"`
		EstimationMode string `json:"estimation_mode"`
		WorkflowMode   string `json:"workflow_mode"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	product, err := c.product.Create(r.Context(), user, service.CreateProductInput{
		Name:           req.Name,
		Description:    req.Description,
		EstimationMode: req.EstimationMode,
		WorkflowMode:   req.WorkflowMode,
	})
	if err != nil {
		if errors.Is(err, service.ErrForbidden) {
			httputil.Error(w, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		httputil.Error(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
		return
	}

	httputil.JSON(w, http.StatusCreated, productToJSON(product))
}

// Get handles GET /products/{productId}.
func (c *ProductController) Get(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	product, err := c.product.GetByID(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, productToJSON(product))
}

// Update handles PATCH /products/{productId}.
func (c *ProductController) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		Name           *string `json:"name"`
		Description    *string `json:"description"`
		EstimationMode *string `json:"estimation_mode"`
		WorkflowMode   *string `json:"workflow_mode"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	product, err := c.product.Update(r.Context(), user, productID, service.UpdateProductInput{
		Name:           req.Name,
		Description:    req.Description,
		EstimationMode: req.EstimationMode,
		WorkflowMode:   req.WorkflowMode,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, productToJSON(product))
}

// Delete handles DELETE /products/{productId}.
func (c *ProductController) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	if err := c.product.Delete(r.Context(), user, productID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListMembers handles GET /products/{productId}/members.
func (c *ProductController) ListMembers(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	members, err := c.product.ListMembers(r.Context(), user, productID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(members))
	for _, m := range members {
		items = append(items, productRoleToJSON(m))
	}
	httputil.JSON(w, http.StatusOK, items)
}

// AddMember handles POST /products/{productId}/members.
func (c *ProductController) AddMember(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.product.AddMember(r.Context(), user, productID, req.UserID, req.Role); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "member_added"})
}

// UpdateMember handles PATCH /products/{productId}/members/{userId}.
func (c *ProductController) UpdateMember(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	targetUserID := r.PathValue("userId")

	var req struct {
		Role string `json:"role"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.product.UpdateMember(r.Context(), user, productID, targetUserID, req.Role); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "member_updated"})
}

// RemoveMember handles DELETE /products/{productId}/members/{userId}.
func (c *ProductController) RemoveMember(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	targetUserID := r.PathValue("userId")

	if err := c.product.RemoveMember(r.Context(), user, productID, targetUserID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "member_removed"})
}

// TransferOwnership handles POST /products/{productId}/transfer-ownership.
func (c *ProductController) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		NewOwnerID string `json:"new_owner_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	if err := c.product.TransferOwnership(r.Context(), user, productID, req.NewOwnerID); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "ownership_transferred"})
}

// writeServiceError maps service-layer errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrForbidden) {
		httputil.Error(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}
	if errors.Is(err, service.ErrNotFound) || errors.Is(err, storage.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	if errors.Is(err, storage.ErrTooLarge) {
		httputil.Error(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", err.Error())
		return
	}
	if errors.Is(err, storage.ErrTypeNotAllowed) {
		httputil.Error(w, http.StatusUnsupportedMediaType, "FILE_TYPE_NOT_ALLOWED", err.Error())
		return
	}
	httputil.Error(w, http.StatusBadRequest, "REQUEST_FAILED", err.Error())
}

// productToJSON converts a Product to a JSON-safe map.
func productToJSON(p *model.Product) map[string]any {
	return map[string]any{
		"id":              p.ID,
		"name":            p.Name,
		"description":     p.Description,
		"estimation_mode": p.EstimationMode,
		"workflow_mode":   p.WorkflowMode,
		"created_at":      p.CreatedAt,
		"updated_at":      p.UpdatedAt,
	}
}

// productRoleToJSON converts a ProductRole to a JSON-safe map.
func productRoleToJSON(r *model.ProductRole) map[string]any {
	return map[string]any{
		"id":         r.ID,
		"user_id":    r.UserID,
		"product_id": r.ProductID,
		"role":       r.Role,
		"created_at": r.CreatedAt,
	}
}
