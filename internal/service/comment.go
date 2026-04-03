package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// CommentService handles comments and attachments on any entity.
type CommentService struct {
	comments    store.CommentStore
	attachments store.AttachmentStore
	audit       store.AuditStore
	authz       *Authorizer
}

// minRoleForOwnerType returns the minimum role required to comment on an entity type.
func minRoleForOwnerType(ownerType string) string {
	switch ownerType {
	case "defect", "test_case":
		return model.RoleTester
	default:
		return model.RoleDeveloper
	}
}

// CreateComment creates a new comment. Minimum role depends on owner type.
func (s *CommentService) CreateComment(ctx context.Context, user *model.User, productID, ownerType, ownerID, body string) (*model.Comment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, minRoleForOwnerType(ownerType)); err != nil {
		return nil, err
	}

	if body == "" {
		return nil, errors.New("comment body is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Comment{
		ID:        uuid.New().String(),
		OwnerType: ownerType,
		OwnerID:   ownerID,
		UserID:    user.ID,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.comments.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "comment", EntityID: c.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return c, nil
}

// GetComment returns a comment by ID. Requires viewer role.
func (s *CommentService) GetComment(ctx context.Context, user *model.User, productID, id string) (*model.Comment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.comments.GetByID(ctx, id)
}

// UpdateComment modifies a comment body. Only the author can update.
func (s *CommentService) UpdateComment(ctx context.Context, user *model.User, productID, id, body string) (*model.Comment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}

	c, err := s.comments.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if c.UserID != user.ID && !user.IsAdmin {
		return nil, fmt.Errorf("%w: only the author can update a comment", ErrForbidden)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c.Body = body
	c.UpdatedAt = now

	if err := s.comments.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("update comment: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "comment", EntityID: c.ID,
		UserID: user.ID, Action: "update", CreatedAt: now,
	})

	return c, nil
}

// DeleteComment removes a comment. Author or product owner can delete.
func (s *CommentService) DeleteComment(ctx context.Context, user *model.User, productID, id string) error {
	role, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer)
	if err != nil {
		return err
	}

	c, err := s.comments.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if c.UserID != user.ID && roleRank(role) < roleRank(model.RoleOwner) {
		return fmt.Errorf("%w: only the author or product owner can delete a comment", ErrForbidden)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.comments.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "comment", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListComments returns comments for an entity. Requires viewer role.
func (s *CommentService) ListComments(ctx context.Context, user *model.User, productID, ownerType, ownerID string) ([]*model.Comment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.comments.ListByOwner(ctx, ownerType, ownerID)
}

// CreateAttachment creates a new attachment record. Minimum role depends on owner type.
func (s *CommentService) CreateAttachment(ctx context.Context, user *model.User, productID string, a *model.Attachment) (*model.Attachment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, minRoleForOwnerType(a.OwnerType)); err != nil {
		return nil, err
	}

	if a.Filename == "" {
		return nil, errors.New("filename is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	a.ID = uuid.New().String()
	a.UserID = user.ID
	a.CreatedAt = now

	if err := s.attachments.Create(ctx, a); err != nil {
		return nil, fmt.Errorf("create attachment: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "attachment", EntityID: a.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return a, nil
}

// GetAttachment returns an attachment by ID. Requires viewer role.
func (s *CommentService) GetAttachment(ctx context.Context, user *model.User, productID, id string) (*model.Attachment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.attachments.GetByID(ctx, id)
}

// DeleteAttachment removes an attachment. Author or product owner can delete.
func (s *CommentService) DeleteAttachment(ctx context.Context, user *model.User, productID, id string) error {
	role, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer)
	if err != nil {
		return err
	}

	a, err := s.attachments.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if a.UserID != user.ID && roleRank(role) < roleRank(model.RoleOwner) {
		return fmt.Errorf("%w: only the author or product owner can delete an attachment", ErrForbidden)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.attachments.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete attachment: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "attachment", EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: now,
	})

	return nil
}

// ListAttachments returns attachments for an entity. Requires viewer role.
func (s *CommentService) ListAttachments(ctx context.Context, user *model.User, productID, ownerType, ownerID string) ([]*model.Attachment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.attachments.ListByOwner(ctx, ownerType, ownerID)
}
