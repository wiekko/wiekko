// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

const invitationTTL = 7 * 24 * time.Hour // 7 days

// Limits for invitations sent by email. Links the inviter passes on
// themselves are not limited here: they cost nothing and reach nobody unless
// the inviter forwards them. An emailed invitation reaches whatever address
// was typed in, so it must not become a way to flood a mailbox.
const (
	InvitationMailWindow        = time.Hour
	invitationMailsPerInviter   = 20
	InvitationRecipientWindow   = 24 * time.Hour
	invitationMailsPerRecipient = 3
)

// Invitation delivery: a link shown to the inviter, or an email to the
// invited address.
const (
	InvitationDeliveryLink  = "link"
	InvitationDeliveryEmail = "email"
)

// ErrInvitationNotSent is returned when the invitation email could not be
// handed to the mail transport. The invitation is withdrawn again.
var ErrInvitationNotSent = errors.New("the invitation email could not be sent; try again later, or create an invitation link instead")

// InvitationService handles invitation operations.
type InvitationService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx          store.Transactor
	invitations store.InvitationStore
	authz       *Authorizer
	// publicURL is the configured server address (NEXMAP_PUBLIC_URL), or
	// empty to build invitation links from the request.
	publicURL string

	// mail sends invitation emails; nil or disabled: links only.
	mail *Mailer
	// inviterLimiter and recipientLimiter bound invitation emails per
	// inviter and per invited address; nil disables them.
	inviterLimiter   *attemptLimiter
	recipientLimiter *attemptLimiter
}

// MailEnabled reports whether invitations can be sent by email.
func (s *InvitationService) MailEnabled() bool { return s.mail.Enabled() }

// AcceptURL returns the full link an invited person opens to create their
// account. requestBase ("https://host") is used when no public URL is
// configured; it should come from the request the inviter made.
func (s *InvitationService) AcceptURL(rawToken, requestBase string) string {
	base := s.publicURL
	if base == "" {
		base = requestBase
	}
	return strings.TrimRight(base, "/") + "/accept-invite/" + rawToken
}

// CreateInvitationResult contains the raw token (shown once) and the invitation record.
type CreateInvitationResult struct {
	RawToken   string
	Invitation *model.Invitation
}

// Create generates a new invitation whose link the inviter passes on. An
// invitation to a product carries the role the new user gets there and
// requires the owner role in that product (FR-77); an invitation without a
// product requires an administrator.
func (s *InvitationService) Create(ctx context.Context, inviter *model.User, email, productID, role string) (*CreateInvitationResult, error) {
	email, err := s.authorize(ctx, inviter, email, productID, role)
	if err != nil {
		return nil, err
	}
	return s.create(ctx, inviter, email, productID, role)
}

// Send creates an invitation like Create and emails its link to the invited
// address (SC-36). The link is not returned: only the mailbox owner gets it,
// so an account created from it has proven the address. Sending is limited
// per inviter and per address. If the email cannot be sent, the invitation is
// deleted again and ErrInvitationNotSent returned.
func (s *InvitationService) Send(ctx context.Context, inviter *model.User, email, productID, role string) (*model.Invitation, error) {
	if !s.mail.Enabled() {
		return nil, inputError(ErrMailDisabled)
	}
	email, err := s.authorize(ctx, inviter, email, productID, role)
	if err != nil {
		return nil, err
	}

	inviterKey, rcptKey := "inviter:"+inviter.ID, recipientKey(email)
	if err := s.inviterLimiter.check(inviterKey); err != nil {
		return nil, err
	}
	if err := s.recipientLimiter.check(rcptKey); err != nil {
		return nil, err
	}
	s.inviterLimiter.add(inviterKey)
	s.recipientLimiter.add(rcptKey)

	result, err := s.create(ctx, inviter, email, productID, role)
	if err != nil {
		return nil, err
	}
	inv := result.Invitation
	expires, _ := time.Parse(time.RFC3339, inv.ExpiresAt)
	msg := s.mail.invitationMessage(email, result.RawToken, role, expires)
	// Not one transaction with the create: it would stay open for the whole
	// SMTP exchange, and on SQLite (one connection) stall every other request
	// meanwhile. Withdrawing the invitation compensates instead, and a failed
	// withdrawal is reported rather than left silent.
	if err := s.mail.send(ctx, mailPurposeInvitation, msg); err != nil {
		// Do not leave a usable invitation behind that nobody received.
		if delErr := s.invitations.Delete(ctx, inv.ID); delErr != nil {
			return nil, fmt.Errorf("withdraw unsent invitation: %w", delErr)
		}
		return nil, ErrInvitationNotSent
	}
	return inv, nil
}

// authorize checks the address and the inviter's right to invite, and
// returns the trimmed address.
func (s *InvitationService) authorize(ctx context.Context, inviter *model.User, email, productID, role string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", inputError(errors.New("email is required"))
	}
	// The address is shown to administrators and may become an email's
	// recipient; it must be one plain address, nothing else.
	if err := validateEmail(email); err != nil {
		return "", err
	}
	if productID == "" {
		if role != "" {
			return "", inputError(errors.New("a role can only be given with a product"))
		}
		if !inviter.IsAdmin {
			return "", fmt.Errorf("%w: only administrators can invite users without a product", ErrForbidden)
		}
		return email, nil
	}
	if roleRank(role) < 0 {
		return "", inputError(fmt.Errorf("invalid role %q", role))
	}
	if _, err := s.authz.RequireRole(ctx, inviter, productID, model.RoleOwner); err != nil {
		return "", err
	}
	return email, nil
}

func (s *InvitationService) create(ctx context.Context, inviter *model.User, email, productID, role string) (*CreateInvitationResult, error) {
	rawToken, err := generateToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	now := time.Now().UTC()
	inv := &model.Invitation{
		ID:        uuid.New().String(),
		Email:     email,
		TokenHash: hashPATToken(rawToken),
		InvitedBy: inviter.ID,
		ProductID: productID,
		Role:      role,
		ExpiresAt: now.Add(invitationTTL).Format(time.RFC3339),
		CreatedAt: now.Format(time.RFC3339),
	}

	if err := s.invitations.Create(ctx, inv); err != nil {
		return nil, fmt.Errorf("store invitation: %w", err)
	}
	return &CreateInvitationResult{RawToken: rawToken, Invitation: inv}, nil
}

// List returns all invitations.
func (s *InvitationService) List(ctx context.Context) ([]*model.Invitation, error) {
	return s.invitations.List(ctx)
}

// Delete removes an invitation.
func (s *InvitationService) Delete(ctx context.Context, id string) error {
	return s.invitations.Delete(ctx, id)
}
