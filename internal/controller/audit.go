package controller

import (
	"net/http"

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
