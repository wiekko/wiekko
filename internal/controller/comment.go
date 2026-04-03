package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// CommentController handles comment and attachment endpoints.
type CommentController struct {
	comment *service.CommentService
}

// NewComment creates a CommentController.
func NewComment(comment *service.CommentService) *CommentController {
	return &CommentController{comment: comment}
}

// --- Comments ---

// ListComments handles GET /products/{productId}/comments?owner_type=...&owner_id=...
func (c *CommentController) ListComments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	ownerType := r.URL.Query().Get("owner_type")
	ownerID := r.URL.Query().Get("owner_id")

	items, err := c.comment.ListComments(r.Context(), user, productID, ownerType, ownerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, commentToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateComment handles POST /products/{productId}/comments.
func (c *CommentController) CreateComment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		OwnerType string `json:"owner_type"`
		OwnerID   string `json:"owner_id"`
		Body      string `json:"body"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.comment.CreateComment(r.Context(), user, productID, req.OwnerType, req.OwnerID, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, commentToJSON(item))
}

// GetComment handles GET /products/{productId}/comments/{id}.
func (c *CommentController) GetComment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.comment.GetComment(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, commentToJSON(item))
}

// UpdateComment handles PATCH /products/{productId}/comments/{id}.
func (c *CommentController) UpdateComment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		Body string `json:"body"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.comment.UpdateComment(r.Context(), user, productID, id, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, commentToJSON(item))
}

// DeleteComment handles DELETE /products/{productId}/comments/{id}.
func (c *CommentController) DeleteComment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.comment.DeleteComment(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Attachments ---

// ListAttachments handles GET /products/{productId}/attachments?owner_type=...&owner_id=...
func (c *CommentController) ListAttachments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	ownerType := r.URL.Query().Get("owner_type")
	ownerID := r.URL.Query().Get("owner_id")

	items, err := c.comment.ListAttachments(r.Context(), user, productID, ownerType, ownerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, attachmentToJSON(item))
	}
	httputil.JSON(w, http.StatusOK, result)
}

// CreateAttachment handles POST /products/{productId}/attachments.
func (c *CommentController) CreateAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var req struct {
		OwnerType   string `json:"owner_type"`
		OwnerID     string `json:"owner_id"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
		StoragePath string `json:"storage_path"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.comment.CreateAttachment(r.Context(), user, productID, &model.Attachment{
		OwnerType:   req.OwnerType,
		OwnerID:     req.OwnerID,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		Size:        req.Size,
		StoragePath: req.StoragePath,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, attachmentToJSON(item))
}

// GetAttachment handles GET /products/{productId}/attachments/{id}.
func (c *CommentController) GetAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.comment.GetAttachment(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, attachmentToJSON(item))
}

// DeleteAttachment handles DELETE /products/{productId}/attachments/{id}.
func (c *CommentController) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.comment.DeleteAttachment(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func commentToJSON(c *model.Comment) map[string]any {
	return map[string]any{
		"id":         c.ID,
		"owner_type": c.OwnerType,
		"owner_id":   c.OwnerID,
		"user_id":    c.UserID,
		"body":       c.Body,
		"created_at": c.CreatedAt,
		"updated_at": c.UpdatedAt,
	}
}

func attachmentToJSON(a *model.Attachment) map[string]any {
	return map[string]any{
		"id":           a.ID,
		"owner_type":   a.OwnerType,
		"owner_id":     a.OwnerID,
		"user_id":      a.UserID,
		"filename":     a.Filename,
		"content_type": a.ContentType,
		"size":         a.Size,
		"storage_path": a.StoragePath,
		"created_at":   a.CreatedAt,
	}
}
