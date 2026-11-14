package controller

import (
	"database/sql"
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
	users   *service.UserService
}

// NewProduct creates a ProductController. users resolves member usernames.
func NewProduct(product *service.ProductService, users *service.UserService) *ProductController {
	return &ProductController{product: product, users: users}
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
			httputil.Error(w, http.StatusForbidden, "FORBIDDEN", errorText(err))
			return
		}
		httputil.Error(w, http.StatusBadRequest, "CREATE_FAILED", errorText(err))
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
		DefectPrefix   *string `json:"defect_prefix"`
		DocumentPrefix *string `json:"document_prefix"`
		// TwoFactorRequired keeps members without 2FA out (SC-107).
		TwoFactorRequired *bool `json:"two_factor_required"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	product, err := c.product.Update(r.Context(), user, productID, service.UpdateProductInput{
		Name:           req.Name,
		Description:    req.Description,
		EstimationMode: req.EstimationMode,
		WorkflowMode:   req.WorkflowMode,
		DefectPrefix:   req.DefectPrefix,
		DocumentPrefix: req.DocumentPrefix,

		TwoFactorRequired: req.TwoFactorRequired,
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

	// Usernames are pseudonyms (SC-306) and visible to every product member
	// in the web UI; display names and emails stay out of the API listing.
	items := make([]map[string]any, 0, len(members))
	for _, m := range members {
		item := productRoleToJSON(m)
		item["username"] = ""
		if c.users != nil {
			if u, err := c.users.GetByID(r.Context(), m.UserID); err == nil {
				item["username"] = u.Username
			}
		}
		items = append(items, item)
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

// SplitStatuses handles POST /products/{productId}/statuses/split: it
// separates the requirement and defect statuses for good, as the settings
// page does.
func (c *ProductController) SplitStatuses(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if err := c.product.SplitStatuses(r.Context(), user, r.PathValue("productId")); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "statuses_split"})
}

// writeServiceError maps service-layer errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrTwoFactorRequired) {
		httputil.Error(w, http.StatusForbidden, "TOTP_SETUP_REQUIRED",
			"This product requires two-factor authentication; set it up via /api/v1/user/me/totp")
		return
	}
	if errors.Is(err, service.ErrForbidden) {
		httputil.Error(w, http.StatusForbidden, "FORBIDDEN", errorText(err))
		return
	}
	if errors.Is(err, service.ErrNotFound) || errors.Is(err, storage.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "NOT_FOUND", errorText(err))
		return
	}
	// Stores report a missing row as a wrapped sql.ErrNoRows. That is a 404 for
	// the client, and the driver's wording must not leak into the response.
	if errors.Is(err, sql.ErrNoRows) {
		httputil.Error(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if errors.Is(err, storage.ErrTooLarge) {
		httputil.Error(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", errorText(err))
		return
	}
	if errors.Is(err, storage.ErrTypeNotAllowed) {
		httputil.Error(w, http.StatusUnsupportedMediaType, "FILE_TYPE_NOT_ALLOWED", errorText(err))
		return
	}
	message, internal := publicError(err)
	if internal {
		httputil.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", message)
		return
	}
	httputil.Error(w, http.StatusBadRequest, "REQUEST_FAILED", message)
}

// writeAccountError answers a failed account operation (registration, profile,
// password, administration) with the service's message under the given code;
// database errors are logged and answered with a generic 500 instead.
func writeAccountError(w http.ResponseWriter, code string, err error) {
	message, internal := publicError(err)
	if internal {
		httputil.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", message)
		return
	}
	httputil.Error(w, http.StatusBadRequest, code, message)
}

// productToJSON converts a Product to a JSON-safe map.
func productToJSON(p *model.Product) map[string]any {
	return map[string]any{
		"id":                  p.ID,
		"name":                p.Name,
		"description":         p.Description,
		"estimation_mode":     p.EstimationMode,
		"workflow_mode":       p.WorkflowMode,
		"defect_prefix":       p.DefectPrefix,
		"document_prefix":     p.DocumentPrefix,
		"two_factor_required": p.TwoFactorRequired,
		"created_at":          p.CreatedAt,
		"updated_at":          p.UpdatedAt,
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
