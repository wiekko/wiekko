package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// PATController handles personal access token endpoints.
type PATController struct {
	pat *service.PATService
}

// NewPAT creates a PATController.
func NewPAT(pat *service.PATService) *PATController {
	return &PATController{pat: pat}
}

// List handles GET /user/me/tokens.
func (c *PATController) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	pats, err := c.pat.List(r.Context(), user.ID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "LIST_FAILED", "Failed to list tokens")
		return
	}

	items := make([]map[string]any, 0, len(pats))
	for _, p := range pats {
		items = append(items, patToJSON(p))
	}
	httputil.JSON(w, http.StatusOK, items)
}

// Create handles POST /user/me/tokens.
func (c *PATController) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	var req struct {
		Name      string  `json:"name"`
		ExpiresAt *string `json:"expires_at"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Token name is required")
		return
	}

	result, err := c.pat.Create(r.Context(), user.ID, req.Name, req.ExpiresAt)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
		return
	}

	resp := patToJSON(result.PAT)
	resp["raw_token"] = result.RawToken
	httputil.JSON(w, http.StatusCreated, resp)
}

// Revoke handles DELETE /user/me/tokens/{id}.
func (c *PATController) Revoke(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	patID := r.PathValue("id")
	if patID == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_ID", "Token ID is required")
		return
	}

	if err := c.pat.Revoke(r.Context(), user.ID, patID); err != nil {
		httputil.Error(w, http.StatusBadRequest, "REVOKE_FAILED", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func patToJSON(p *model.PAT) map[string]any {
	m := map[string]any{
		"id":           p.ID,
		"name":         p.Name,
		"token_prefix": p.TokenPrefix,
		"created_at":   p.CreatedAt,
	}
	if p.LastUsedAt != "" {
		m["last_used_at"] = p.LastUsedAt
	}
	if p.ExpiresAt != "" {
		m["expires_at"] = p.ExpiresAt
	}
	if p.RevokedAt != "" {
		m["revoked_at"] = p.RevokedAt
	}
	return m
}
