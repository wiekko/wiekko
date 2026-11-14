package controller

import (
	"mime"
	"net/http"
	"strconv"

	"nexmap.net/internal/access"
	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/internal/textdiff"
)

// DocumentController handles document endpoints (FR-044..FR-052).
type DocumentController struct {
	documents *service.DocumentService
}

// NewDocument creates a DocumentController.
func NewDocument(documents *service.DocumentService) *DocumentController {
	return &DocumentController{documents: documents}
}

// documentToJSON converts a document; the body only when withBody is set.
func documentToJSON(d *model.Document, withBody bool) map[string]any {
	m := map[string]any{
		"id":               d.ID,
		"identifier":       d.Identifier,
		"product_id":       d.ProductID,
		"sequence_number":  d.SequenceNumber,
		"title":            d.Title,
		"format":           d.Format,
		"state":            d.State,
		"superseded_by_id": d.SupersededByID,
		"component_id":     d.ComponentID,
		"capability_id":    d.CapabilityID,
		"milestone_id":     d.MilestoneID,
		"revision":         d.Revision,
		"file_name":        model.DocumentFileName(d.Identifier, d.Format),
		"created_at":       d.CreatedAt,
		"updated_at":       d.UpdatedAt,
	}
	if withBody {
		m["body"] = d.Body
	}
	return m
}

func revisionToJSON(r *model.DocumentRevision, withBody bool) map[string]any {
	m := map[string]any{
		"revision":   r.Revision,
		"title":      r.Title,
		"format":     r.Format,
		"user_id":    r.UserID,
		"created_at": r.CreatedAt,
	}
	if withBody {
		m["body"] = r.Body
	}
	return m
}

// resolveDocument turns a document reference from a request body into its ID.
func (c *DocumentController) resolveDocument(r *http.Request, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	return c.documents.ResolveDocumentRef(r.Context(), middleware.UserFromContext(r.Context()), r.PathValue("productId"), ref)
}

// ListDocuments handles GET /products/{productId}/documents. Bodies are left
// out unless ?body=true.
func (c *DocumentController) ListDocuments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	q := r.URL.Query()
	withBody, err := parseOptionalBool(q.Get("body"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_PARAMETER", "body must be true or false")
		return
	}
	filter := store.DocumentFilter{
		State: q.Get("state"), Format: q.Get("format"), ComponentID: q.Get("component_id"),
		CapabilityID: q.Get("capability_id"), MilestoneID: q.Get("milestone_id"), WithBody: withBody,
	}
	items, err := c.documents.ListDocuments(r.Context(), user, r.PathValue("productId"), filter,
		store.ListParams{Search: q.Get("q"), Sort: q.Get("sort"), Dir: q.Get("dir")})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, d := range items {
		out = append(out, documentToJSON(d, withBody))
	}
	httputil.JSON(w, http.StatusOK, out)
}

func parseOptionalBool(s string) (bool, error) {
	if s == "" {
		return false, nil
	}
	return strconv.ParseBool(s)
}

// CreateDocument handles POST /products/{productId}/documents.
func (c *DocumentController) CreateDocument(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	var req struct {
		Title          string `json:"title"`
		Body           string `json:"body"`
		Format         string `json:"format"`
		State          string `json:"state"`
		SupersededByID string `json:"superseded_by_id"`
		ComponentID    string `json:"component_id"`
		CapabilityID   string `json:"capability_id"`
		MilestoneID    string `json:"milestone_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	successor, err := c.resolveDocument(r, req.SupersededByID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	d, err := c.documents.CreateDocument(r.Context(), user, service.CreateDocumentInput{
		ProductID: r.PathValue("productId"), Title: req.Title, Body: req.Body, Format: req.Format,
		State: req.State, SupersededByID: successor, ComponentID: req.ComponentID,
		CapabilityID: req.CapabilityID, MilestoneID: req.MilestoneID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusCreated, documentToJSON(d, true))
}

// GetDocument handles GET /products/{productId}/documents/{id}.
func (c *DocumentController) GetDocument(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	d, err := c.documents.GetDocument(r.Context(), user, r.PathValue("productId"), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, documentToJSON(d, true))
}

// UpdateDocument handles PATCH /products/{productId}/documents/{id}.
func (c *DocumentController) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	var req struct {
		Title          *string `json:"title"`
		Body           *string `json:"body"`
		Format         *string `json:"format"`
		State          *string `json:"state"`
		SupersededByID *string `json:"superseded_by_id"`
		ComponentID    *string `json:"component_id"`
		CapabilityID   *string `json:"capability_id"`
		MilestoneID    *string `json:"milestone_id"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.SupersededByID != nil {
		id, err := c.resolveDocument(r, *req.SupersededByID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		req.SupersededByID = &id
	}
	d, err := c.documents.UpdateDocument(r.Context(), user, r.PathValue("productId"), r.PathValue("id"), service.UpdateDocumentInput{
		Title: req.Title, Body: req.Body, Format: req.Format, State: req.State, SupersededByID: req.SupersededByID,
		ComponentID: req.ComponentID, CapabilityID: req.CapabilityID, MilestoneID: req.MilestoneID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, documentToJSON(d, true))
}

// DeleteDocument handles DELETE /products/{productId}/documents/{id}.
func (c *DocumentController) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if err := c.documents.DeleteDocument(r.Context(), user, r.PathValue("productId"), r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListRevisions handles GET /products/{productId}/documents/{id}/revisions.
func (c *DocumentController) ListRevisions(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	revs, err := c.documents.ListRevisions(r.Context(), user, r.PathValue("productId"), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(revs))
	for _, rev := range revs {
		out = append(out, revisionToJSON(rev, false))
	}
	httputil.JSON(w, http.StatusOK, out)
}

func revisionParam(w http.ResponseWriter, value, name string) (int, bool) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		httputil.Error(w, http.StatusBadRequest, "INVALID_PARAMETER", name+" must be a revision number")
		return 0, false
	}
	return n, true
}

// GetRevision handles GET /products/{productId}/documents/{id}/revisions/{rev}.
func (c *DocumentController) GetRevision(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	n, ok := revisionParam(w, r.PathValue("rev"), "rev")
	if !ok {
		return
	}
	rev, err := c.documents.GetRevision(r.Context(), user, r.PathValue("productId"), r.PathValue("id"), n)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, revisionToJSON(rev, true))
}

// Diff handles GET /products/{productId}/documents/{id}/diff?from=&to=. To
// defaults to the latest revision, from to the one before it.
func (c *DocumentController) Diff(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID, id := r.PathValue("productId"), r.PathValue("id")
	d, err := c.documents.GetDocument(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	to, from := d.Revision, d.Revision-1
	if v := r.URL.Query().Get("to"); v != "" {
		n, ok := revisionParam(w, v, "to")
		if !ok {
			return
		}
		to, from = n, n-1
	}
	if v := r.URL.Query().Get("from"); v != "" {
		n, ok := revisionParam(w, v, "from")
		if !ok {
			return
		}
		from = n
	}
	if from < 1 {
		from = 1
	}
	diff, err := c.documents.Diff(r.Context(), user, productID, id, from, to)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	lines := make([]map[string]any, 0, len(diff.Lines))
	for _, l := range diff.Lines {
		lines = append(lines, map[string]any{"op": diffOpName(l.Op), "text": l.Text, "old": l.Old, "new": l.New})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"from": revisionToJSON(diff.From, false), "to": revisionToJSON(diff.To, false),
		"title_changed": diff.TitleChanged, "format_changed": diff.FormatChanged,
		"unified": diff.Unified(d.Identifier), "lines": lines,
	})
}

func diffOpName(op textdiff.Op) string {
	switch op {
	case textdiff.Delete:
		return "delete"
	case textdiff.Insert:
		return "insert"
	}
	return "equal"
}

// Restore handles POST /products/{productId}/documents/{id}/restore.
func (c *DocumentController) Restore(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	var req struct {
		Revision int `json:"revision"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Revision < 1 {
		httputil.Error(w, http.StatusBadRequest, "INVALID_PARAMETER", "revision must be a revision number")
		return
	}
	d, err := c.documents.RestoreRevision(r.Context(), user, r.PathValue("productId"), r.PathValue("id"), req.Revision)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, documentToJSON(d, true))
}

// Raw handles GET /products/{productId}/documents/{id}/raw[?revision=n]: the
// body byte for byte. Whatever the document's format, it is served as a
// text/plain attachment that browsers neither sniff nor render, so a body
// holding markup can never run as active content.
func (c *DocumentController) Raw(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID, id := r.PathValue("productId"), r.PathValue("id")
	d, err := c.documents.GetDocument(r.Context(), user, productID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	body, format := d.Body, d.Format
	if v := r.URL.Query().Get("revision"); v != "" {
		n, ok := revisionParam(w, v, "revision")
		if !ok {
			return
		}
		rev, err := c.documents.GetRevision(r.Context(), user, productID, id, n)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		body, format = rev.Body, rev.Format
	}
	writeDocumentBody(w, model.DocumentFileName(d.Identifier, format), body)
}

// writeDocumentBody serves a document body as a plain-text attachment.
func writeDocumentBody(w http.ResponseWriter, filename, body string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": safeDownloadName(filename)}))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// ListLinked handles GET /products/{productId}/documents/{id}/{requirements,
// constraints,test-cases}: the items of one type the document is linked to.
func (c *DocumentController) ListLinked(targetType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		items, err := c.documents.LinkedItems(r.Context(), user, r.PathValue("productId"), r.PathValue("id"), targetType)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		out := make([]map[string]any, 0, len(items))
		for _, it := range items {
			m := map[string]any{"id": it.ID, "identifier": it.Identifier, "title": it.Title}
			if targetType == model.DocumentLinkTestCase {
				m["test_identifier"] = it.TestIdentifier
			}
			out = append(out, m)
		}
		httputil.JSON(w, http.StatusOK, out)
	})
}

// Link handles POST /products/{productId}/documents/{id}/{requirements,
// constraints,test-cases} with {"target_id": ...}. The target may be given by
// identifier where the type has one; resolve turns it into an ID.
func (c *DocumentController) Link(targetType string, resolve refResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		var req struct {
			TargetID string `json:"target_id"`
		}
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		target := req.TargetID
		if resolve != nil && target != "" {
			id, err := resolve(r.Context(), user, r.PathValue("productId"), target)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			target = id
		}
		if err := c.documents.LinkDocument(r.Context(), user, r.PathValue("productId"), r.PathValue("id"), targetType, target); err != nil {
			writeServiceError(w, err)
			return
		}
		httputil.JSON(w, http.StatusCreated, map[string]string{"status": "linked"})
	})
}

// Unlink handles DELETE /products/{productId}/documents/{id}/{type}/{targetId}.
func (c *DocumentController) Unlink(targetType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		if err := c.documents.UnlinkDocument(r.Context(), user, r.PathValue("productId"), r.PathValue("id"), targetType, r.PathValue("targetId")); err != nil {
			writeServiceError(w, err)
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
	})
}

// DocumentsOf handles GET /products/{productId}/{requirements,constraints,
// test-cases}/{id}/documents.
func (c *DocumentController) DocumentsOf(targetType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		docs, err := c.documents.DocumentsFor(r.Context(), user, r.PathValue("productId"), targetType, r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		out := make([]map[string]any, 0, len(docs))
		for _, d := range docs {
			out = append(out, documentToJSON(d, false))
		}
		httputil.JSON(w, http.StatusOK, out)
	})
}

// linkedDocumentsJSON names the documents linked to an item, for embedding in
// the item's own response (FR-909). A token without documents:read learns
// only their identifiers; a failed lookup leaves the field out rather than
// failing the item.
func linkedDocumentsJSON(r *http.Request, docs *service.DocumentService, targetType, id string) ([]map[string]any, bool) {
	if docs == nil {
		return nil, false
	}
	user := middleware.UserFromContext(r.Context())
	items, err := docs.DocumentsFor(r.Context(), user, r.PathValue("productId"), targetType, id)
	if err != nil {
		return nil, false
	}
	full := access.FromContext(r.Context()).Allows(access.Documents, access.Read)
	out := make([]map[string]any, 0, len(items))
	for _, d := range items {
		m := map[string]any{"identifier": d.Identifier}
		if full {
			m["id"] = d.ID
			m["title"] = d.Title
			m["state"] = d.State
		}
		out = append(out, m)
	}
	return out, true
}

// withLinkedDocuments adds the "documents" field to an item's JSON.
func withLinkedDocuments(r *http.Request, docs *service.DocumentService, targetType, id string, m map[string]any) map[string]any {
	if list, ok := linkedDocumentsJSON(r, docs, targetType, id); ok {
		m["documents"] = list
	}
	return m
}
