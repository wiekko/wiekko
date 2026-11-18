package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

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
		Delivery  string `json:"delivery"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Email is required")
		return
	}
	c.create(w, r, req.Email, req.ProductID, req.Role, req.Delivery, func(err error) {
		httputil.Error(w, http.StatusBadRequest, "INVITATION_FAILED", errorText(err))
	})
}

// CreateForProduct handles POST /products/{productId}/invitations: a product
// owner invites a not-yet-registered user with a role (FR-77), as the team
// page does.
func (c *InvitationController) CreateForProduct(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Role     string `json:"role"`
		Delivery string `json:"delivery"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" || req.Role == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "Email and role are required")
		return
	}
	c.create(w, r, req.Email, r.PathValue("productId"), req.Role, req.Delivery, func(err error) {
		writeServiceError(w, err)
	})
}

// create invites by link (the default, as before mail existed) or by email.
// A link invitation answers with the link; an emailed one does not, since
// only the invited mailbox is meant to get it.
func (c *InvitationController) create(w http.ResponseWriter, r *http.Request, email, productID, role, rawDelivery string, fail func(error)) {
	delivery, ok := invitationDelivery(rawDelivery)
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "INVALID_DELIVERY", "delivery must be \"link\" or \"email\"")
		return
	}
	user := middleware.UserFromContext(r.Context())

	if delivery == service.InvitationDeliveryEmail {
		inv, err := c.invitation.Send(r.Context(), user, email, productID, role)
		if err != nil {
			if wait, blocked := attemptsBlocked(err); blocked {
				w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
				httputil.Error(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", invitationErrorText(err))
				return
			}
			if errors.Is(err, service.ErrInvitationNotSent) {
				httputil.Error(w, http.StatusBadGateway, "MAIL_NOT_SENT", errorText(err))
				return
			}
			fail(err)
			return
		}
		resp := invitationToJSON(inv)
		resp["delivery"] = service.InvitationDeliveryEmail
		httputil.JSON(w, http.StatusCreated, resp)
		return
	}

	result, err := c.invitation.Create(r.Context(), user, email, productID, role)
	if err != nil {
		fail(err)
		return
	}
	resp := invitationToJSON(result.Invitation)
	resp["delivery"] = service.InvitationDeliveryLink
	resp["raw_token"] = result.RawToken
	resp["url"] = c.invitation.AcceptURL(result.RawToken, httputil.RequestBaseURL(r))
	httputil.JSON(w, http.StatusCreated, resp)
}

// invitationDelivery parses the requested delivery; empty means a link.
func invitationDelivery(v string) (string, bool) {
	switch v {
	case "", service.InvitationDeliveryLink:
		return service.InvitationDeliveryLink, true
	case service.InvitationDeliveryEmail:
		return service.InvitationDeliveryEmail, true
	}
	return "", false
}

// invitationErrorText is the message for a failed emailed invitation.
func invitationErrorText(err error) string {
	if wait, blocked := attemptsBlocked(err); blocked {
		return fmt.Sprintf("Too many invitation emails for now, from you or to this address. Try again in %s, or create an invitation link instead.", waitMinutes(wait))
	}
	return errorText(err)
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
		httputil.Error(w, http.StatusBadRequest, "DELETE_FAILED", errorText(err))
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
