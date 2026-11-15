package controller

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// DiscussionController handles comment and attachment endpoints.
type DiscussionController struct {
	discussion *service.DiscussionService
	defectRef  refResolver
}

// NewDiscussion creates a DiscussionController.
//
// defectRef resolves defect identifiers given as the owner of comments and
// attachments (FR-043); nil accepts IDs only.
func NewDiscussion(discussion *service.DiscussionService, defectRef refResolver) *DiscussionController {
	return &DiscussionController{discussion: discussion, defectRef: defectRef}
}

// ownerID resolves the owner reference of a comment or attachment: defects
// may be named by identifier.
func (c *DiscussionController) ownerID(r *http.Request, ownerType, ref string) (string, error) {
	if ownerType != model.OwnerTypeDefect {
		return ref, nil
	}
	return resolveDefect(r, c.defectRef, ref)
}

// --- Comments ---

// ListComments handles GET /products/{productId}/comments?owner_type=...&owner_id=...
func (c *DiscussionController) ListComments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	ownerType := r.URL.Query().Get("owner_type")
	ownerID, err := c.ownerID(r, ownerType, r.URL.Query().Get("owner_id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	items, err := c.discussion.ListComments(r.Context(), user, productID, ownerType, ownerID)
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
func (c *DiscussionController) CreateComment(w http.ResponseWriter, r *http.Request) {
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

	ownerID, err := c.ownerID(r, req.OwnerType, req.OwnerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	item, err := c.discussion.CreateComment(r.Context(), user, productID, req.OwnerType, ownerID, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, commentToJSON(item))
}

// GetComment handles GET /products/{productId}/comments/{id}.
func (c *DiscussionController) GetComment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.discussion.GetComment(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, commentToJSON(item))
}

// UpdateComment handles PATCH /products/{productId}/comments/{id}.
func (c *DiscussionController) UpdateComment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	var req struct {
		Body string `json:"body"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	item, err := c.discussion.UpdateComment(r.Context(), user, productID, id, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, commentToJSON(item))
}

// DeleteComment handles DELETE /products/{productId}/comments/{id}.
func (c *DiscussionController) DeleteComment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.discussion.DeleteComment(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Attachments ---

// ListAttachments handles GET /products/{productId}/attachments?owner_type=...&owner_id=...
func (c *DiscussionController) ListAttachments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	ownerType := r.URL.Query().Get("owner_type")
	ownerID, err := c.ownerID(r, ownerType, r.URL.Query().Get("owner_id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	items, err := c.discussion.ListAttachments(r.Context(), user, productID, ownerType, ownerID)
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
// The body is multipart/form-data with fields owner_type, owner_id and file.
func (c *DiscussionController) CreateAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	// Parse explicitly, so only uploadMemoryLimit of the body is held in RAM.
	// Its total size is capped by the LimitBody middleware.
	if err := r.ParseMultipartForm(uploadMemoryLimit); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httputil.Error(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "the upload exceeds the maximum size")
			return
		}
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "a multipart/form-data body with a file field is required")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "a file field is required")
		return
	}
	defer func() { _ = file.Close() }()

	ownerID, err := c.ownerID(r, r.FormValue("owner_type"), r.FormValue("owner_id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	item, err := c.discussion.CreateAttachment(r.Context(), user, productID, service.CreateAttachmentInput{
		OwnerType:   r.FormValue("owner_type"),
		OwnerID:     ownerID,
		Filename:    filepath.Base(header.Filename),
		ContentType: header.Header.Get("Content-Type"),
		Content:     file,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusCreated, attachmentToJSON(item))
}

// DownloadAttachment handles GET /products/{productId}/attachments/{id}/download.
func (c *DiscussionController) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	a, rc, err := c.discussion.OpenAttachment(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer func() { _ = rc.Close() }()

	writeAttachmentDownload(w, a, rc)
}

// GetAttachment handles GET /products/{productId}/attachments/{id}.
func (c *DiscussionController) GetAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	item, err := c.discussion.GetAttachment(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, attachmentToJSON(item))
}

// DeleteAttachment handles DELETE /products/{productId}/attachments/{id}.
func (c *DiscussionController) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	id := r.PathValue("id")

	if err := c.discussion.DeleteAttachment(r.Context(), user, productID, id); err != nil {
		writeServiceError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// authorID renders the author of a comment or attachment: null once the
// author's account is deleted.
func authorID(userID string) any { return nullable(userID) }

// nullable renders an empty ID as JSON null rather than "".
func nullable(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func commentToJSON(c *model.Comment) map[string]any {
	return map[string]any{
		"id":         c.ID,
		"owner_type": c.OwnerType,
		"owner_id":   c.OwnerID,
		"user_id":    authorID(c.UserID),
		"body":       c.Body,
		"created_at": c.CreatedAt,
		"updated_at": c.UpdatedAt,
	}
}

// attachmentToJSON deliberately omits storage_path: it is a server-internal
// detail and must never round-trip through a client.
func attachmentToJSON(a *model.Attachment) map[string]any {
	return map[string]any{
		"id":           a.ID,
		"owner_type":   a.OwnerType,
		"owner_id":     a.OwnerID,
		"user_id":      authorID(a.UserID),
		"filename":     a.Filename,
		"content_type": a.ContentType,
		"size":         a.Size,
		"created_at":   a.CreatedAt,
	}
}

// writeAttachmentDownload streams an attachment to the client.
//
// The response is always forced to download rather than render: an uploaded
// .html or .svg served inline would execute as same-origin script.
func writeAttachmentDownload(w http.ResponseWriter, a *model.Attachment, rc io.Reader) {
	contentType := a.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.FormatInt(a.Size, 10))
	w.Header().Set("Content-Disposition",
		mime.FormatMediaType("attachment", map[string]string{"filename": safeDownloadName(a.Filename)}))

	if _, err := io.Copy(w, rc); err != nil {
		slog.Error("attachment download failed", "attachment_id", a.ID, "err", err)
	}
}

// safeDownloadName strips anything that could break out of the header or
// reintroduce a path. mime.FormatMediaType handles quoting and encoding.
func safeDownloadName(filename string) string {
	name := filepath.Base(strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return -1
		}
		return r
	}, filename))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "download"
	}
	return name
}
