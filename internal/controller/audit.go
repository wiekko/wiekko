// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"errors"
	"net/http"
	"strconv"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// AuditController handles audit trail endpoints.
type AuditController struct {
	audit *service.AuditService
}

// NewAudit creates an AuditController.
func NewAudit(audit *service.AuditService) *AuditController {
	return &AuditController{audit: audit}
}

// List handles GET /products/{productId}/audit?entity_type=...&entity_id=...
func (c *AuditController) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	entityType := r.URL.Query().Get("entity_type")
	entityID := r.URL.Query().Get("entity_id")

	items, err := c.audit.ListByEntity(r.Context(), user, productID, entityType, entityID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, auditEntryToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

func auditEntryToJSON(e *model.AuditEntry) map[string]any {
	return map[string]any{
		"id":          e.ID,
		"product_id":  nullable(e.ProductID),
		"entity_type": e.EntityType,
		"entity_id":   e.EntityID,
		"user_id":     e.UserID,
		"action":      e.Action,
		"field_name":  e.FieldName,
		"old_value":   e.OldValue,
		"new_value":   e.NewValue,
		"created_at":  e.CreatedAt,
	}
}

// AdminList handles GET /admin/audit: the whole trail, filtered and paged by
// limit (default 50, at most 500) and offset.
func (c *AuditController) AdminList(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = auditPageSize
	}
	if limit > 500 {
		limit = 500
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	entries, total, err := c.audit.AdminList(r.Context(), user, auditFilterFromQuery(r), limit, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		items = append(items, auditEntryToJSON(e))
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"entries": items, "total": total, "limit": limit, "offset": offset})
}

// AdminExport handles GET /admin/audit/export?format=jsonl|csv.
func (c *AuditController) AdminExport(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	writeAuditExport(w, r, c.audit, user, func(err error) { writeServiceError(w, err) })
}

// AdminDeletedProducts handles GET /admin/audit/deleted-products.
func (c *AuditController) AdminDeletedProducts(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	products, err := c.audit.AdminDeletedProducts(r.Context(), user)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(products))
	for _, p := range products {
		items = append(items, map[string]any{
			"id": p.ID, "name": p.Name, "deleted_at": p.DeletedAt, "entries": p.Entries,
		})
	}
	httputil.JSON(w, http.StatusOK, items)
}

// AdminPurge handles DELETE /admin/audit/products/{productId}.
func (c *AuditController) AdminPurge(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	n, err := c.audit.AdminPurgeProduct(r.Context(), user, productID)
	if errors.Is(err, service.ErrProductExists) {
		httputil.Error(w, http.StatusConflict, "PRODUCT_EXISTS", errorText(err))
		return
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	logAdminAction(r, "purge audit trail", "", "product_id", productID, "entries", n)
	httputil.JSON(w, http.StatusOK, map[string]any{"purged": n})
}
