// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// CategoryController handles category endpoints.
type CategoryController struct {
	category *service.CategoryService
}

// NewCategory creates a CategoryController.
func NewCategory(category *service.CategoryService) *CategoryController {
	return &CategoryController{category: category}
}

// ListCategories handles GET /products/{productId}/categories.
func (c *CategoryController) ListCategories(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	entityType := r.URL.Query().Get("entity_type")

	items, err := c.category.ListCategories(r.Context(), user, productID, entityType)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, categoryToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateCategory handles POST /products/{productId}/categories.
func (c *CategoryController) CreateCategory(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		EntityType string `json:"entity_type"`
		Name       string `json:"name"`
		Prefix     string `json:"prefix"`
		Position   int    `json:"position"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.category.CreateCategory(r.Context(), user, service.CreateCategoryInput{
		ProductID:  productID,
		EntityType: req.EntityType,
		Name:       req.Name,
		Prefix:     req.Prefix,
		Position:   req.Position,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, categoryToJSON(item))
}

// GetCategory handles GET /products/{productId}/categories/{id}.
func (c *CategoryController) GetCategory(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.category.GetCategory(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, categoryToJSON(item))
}

// UpdateCategory handles PATCH /products/{productId}/categories/{id}.
func (c *CategoryController) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		Name     *string `json:"name"`
		Prefix   *string `json:"prefix"`
		Position *int    `json:"position"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.category.UpdateCategory(r.Context(), user, productID, id, service.UpdateCategoryInput{
		Name:     req.Name,
		Prefix:   req.Prefix,
		Position: req.Position,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, categoryToJSON(item))
}

// DeleteCategory handles DELETE /products/{productId}/categories/{id}.
func (c *CategoryController) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.category.DeleteCategory(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func categoryToJSON(cat *model.Category) map[string]any {
	return map[string]any{
		"id":          cat.ID,
		"product_id":  cat.ProductID,
		"entity_type": cat.EntityType,
		"name":        cat.Name,
		"prefix":      cat.Prefix,
		"position":    cat.Position,
		"created_at":  cat.CreatedAt,
	}
}
