package controller

import (
	"errors"
	"net/http"
	"strings"

	"nexmap.net/internal/access"
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
//
// Without "scopes" the token gets write access to all product data;
// "products" restricts it to the listed products, and "role_cap" caps the
// role it acts with (SC-401 – SC-406).
func (c *PATController) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	var req struct {
		Name      string    `json:"name"`
		ExpiresAt *string   `json:"expires_at"`
		Scopes    *[]string `json:"scopes"`
		RoleCap   string    `json:"role_cap"`
		Products  *[]string `json:"products"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Token name is required")
		return
	}

	in := service.CreatePATInput{Name: req.Name, RoleCap: req.RoleCap}
	if req.ExpiresAt != nil {
		in.ExpiresAt = *req.ExpiresAt
	}
	if req.Scopes != nil {
		grants, err := access.ParseGrants(strings.Join(*req.Scopes, " "))
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, "INVALID_SCOPES", err.Error())
			return
		}
		in.Grants = grants
	}
	if req.Products != nil {
		in.ProductsRestricted = true
		in.ProductIDs = *req.Products
	}

	result, err := c.pat.Create(r.Context(), user, in)
	if errors.Is(err, service.ErrForbidden) {
		httputil.Error(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}
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
	if r, err := access.FromPAT(p); err == nil {
		for k, v := range restrictionJSON(r) {
			m[k] = v
		}
	}
	return m
}

// restrictionJSON renders a token's restriction: "scopes" as a list of
// "scope:level" grants, "role_cap" (null if none) and "products" (null if the
// token reaches every product of its user).
func restrictionJSON(r *access.Restriction) map[string]any {
	m := map[string]any{"scopes": strings.Fields(r.Grants.String()), "role_cap": nil, "products": nil}
	if r.RoleCap != "" {
		m["role_cap"] = r.RoleCap
	}
	if r.ProductsRestricted {
		products := r.Products
		if products == nil {
			products = []string{}
		}
		m["products"] = products
	}
	return m
}
