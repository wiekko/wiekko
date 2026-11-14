package controller

import (
	"io"
	"net/http"
	"strconv"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

// SpecificationController serves the specification export (FR-905).
type SpecificationController struct {
	spec *service.SpecificationService
}

// NewSpecification creates a SpecificationController.
func NewSpecification(spec *service.SpecificationService) *SpecificationController {
	return &SpecificationController{spec: spec}
}

// Export handles GET /products/{productId}/specification[?tasks=true]
// [&drafts=true]. Documents are listed when the token may read them; their
// bodies are fetched from the documents endpoints (FR-907). The
// response is the Markdown document itself, not a JSON envelope; errors are
// JSON like everywhere else.
func (c *SpecificationController) Export(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	var opts service.ExportOptions
	if v := r.URL.Query().Get("tasks"); v != "" {
		tasks, err := strconv.ParseBool(v)
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, "INVALID_PARAMETER", "tasks must be true or false")
			return
		}
		opts.Tasks = tasks
	}
	if v := r.URL.Query().Get("drafts"); v != "" {
		drafts, err := strconv.ParseBool(v)
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, "INVALID_PARAMETER", "drafts must be true or false")
			return
		}
		opts.Drafts = drafts
	}

	doc, err := c.spec.Export(r.Context(), user, productID, opts)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, doc)
}
