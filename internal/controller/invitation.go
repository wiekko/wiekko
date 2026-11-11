package controller

import (
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// InvitationController handles invitation endpoints.
type InvitationController struct {
	invitation *service.InvitationService
}

// NewInvitation creates an InvitationController.
func NewInvitation(inv *service.InvitationService) *InvitationController {
	return &InvitationController{invitation: inv}
}

// Create handles POST /admin/invitations.
func (c *InvitationController) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email     string `json:"email"`
		ProductID string `json:"product_id"`
		Role      string `json:"role"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Email is required")
		return
	}

	user := middleware.UserFromContext(r.Context())
	result, err := c.invitation.Create(r.Context(), user, req.Email, req.ProductID, req.Role)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVITATION_FAILED", err.Error())
		return
	}

	resp := invitationToJSON(result.Invitation)
	resp["raw_token"] = result.RawToken
	resp["url"] = c.invitation.AcceptURL(result.RawToken, httputil.RequestBaseURL(r))
	httputil.JSON(w, http.StatusCreated, resp)
}

// List handles GET /admin/invitations.
func (c *InvitationController) List(w http.ResponseWriter, r *http.Request) {
	invitations, err := c.invitation.List(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "LIST_FAILED", "Failed to list invitations")
		return
	}

	items := make([]map[string]any, 0, len(invitations))
	for _, inv := range invitations {
		items = append(items, invitationToJSON(inv))
	}
	httputil.JSON(w, http.StatusOK, items)
}

// Delete handles DELETE /admin/invitations/{id}.
func (c *InvitationController) Delete(w http.ResponseWriter, r *http.Request) {
	invID := r.PathValue("id")
	if err := c.invitation.Delete(r.Context(), invID); err != nil {
		httputil.Error(w, http.StatusBadRequest, "DELETE_FAILED", err.Error())
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func invitationToJSON(inv *model.Invitation) map[string]any {
	m := map[string]any{
		"id":         inv.ID,
		"email":      inv.Email,
		"invited_by": inv.InvitedBy,
		"expires_at": inv.ExpiresAt,
		"created_at": inv.CreatedAt,
	}
	if inv.ProductID != "" {
		m["product_id"] = inv.ProductID
	}
	if inv.Role != "" {
		m["role"] = inv.Role
	}
	if inv.AcceptedAt != "" {
		m["accepted_at"] = inv.AcceptedAt
	}
	return m
}
