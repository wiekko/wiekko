package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

// CommentService handles comments and attachments on any entity.
type CommentService struct {
	comments    store.CommentStore
	attachments store.AttachmentStore
	audit       store.AuditStore
	blobs       storage.Store
	authz       *Authorizer
}

// minRoleForOwnerType returns the minimum role required to comment on an entity type.
func minRoleForOwnerType(ownerType string) string {
	switch ownerType {
	case model.OwnerTypeDefect, model.OwnerTypeTestCase:
		return model.RoleTester
	default:
		return model.RoleDeveloper
	}
}

// CreateComment creates a new comment. Minimum role depends on owner type.
func (s *CommentService) CreateComment(ctx context.Context, user *model.User, productID, ownerType, ownerID, body string) (*model.Comment, error) {
	if !model.ValidOwnerType(ownerType) {
		return nil, fmt.Errorf("invalid comment owner type: %q", ownerType)
	}
	if _, err := s.authz.RequireRole(ctx, user, productID, minRoleForOwnerType(ownerType)); err != nil {
		return nil, err
	}

	if body == "" {
		return nil, errors.New("comment body is required")
	}
	if ownerID == "" {
		return nil, errors.New("comment owner id is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	c := &model.Comment{
		ID:        uuid.New().String(),
		ProductID: productID,
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
	c, err := s.comments.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// A comment reached through the wrong product must look absent, not forbidden.
	if c.ProductID != productID {
		return nil, fmt.Errorf("%w: comment", ErrNotFound)
	}
	return c, nil
}

// UpdateComment modifies a comment body. Only the author can update.
func (s *CommentService) UpdateComment(ctx context.Context, user *model.User, productID, id, body string) (*model.Comment, error) {
	if body == "" {
		return nil, errors.New("comment body is required")
	}

	c, err := s.GetComment(ctx, user, productID, id)
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
	if c.ProductID != productID {
		return fmt.Errorf("%w: comment", ErrNotFound)
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
	return s.comments.ListByOwner(ctx, productID, ownerType, ownerID)
}

// CreateAttachmentInput describes an upload. The storage path is derived by the
// service; callers cannot influence where bytes land.
type CreateAttachmentInput struct {
	OwnerType   string
	OwnerID     string
	Filename    string
	ContentType string
	Content     io.Reader
}

// CreateAttachment stores the uploaded bytes and records the metadata.
// Minimum role depends on owner type.
func (s *CommentService) CreateAttachment(ctx context.Context, user *model.User, productID string, input CreateAttachmentInput) (*model.Attachment, error) {
	if !model.ValidOwnerType(input.OwnerType) {
		return nil, fmt.Errorf("invalid attachment owner type: %q", input.OwnerType)
	}
	if _, err := s.authz.RequireRole(ctx, user, productID, minRoleForOwnerType(input.OwnerType)); err != nil {
		return nil, err
	}

	if input.Filename == "" {
		return nil, errors.New("filename is required")
	}
	if input.OwnerID == "" {
		return nil, errors.New("attachment owner id is required")
	}
	if input.Content == nil {
		return nil, errors.New("file content is required")
	}

	id := uuid.New().String()
	path, size, err := s.blobs.Save(ctx, productID, id, input.Content)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	a := &model.Attachment{
		ID:          id,
		ProductID:   productID,
		OwnerType:   input.OwnerType,
		OwnerID:     input.OwnerID,
		UserID:      user.ID,
		Filename:    input.Filename,
		ContentType: input.ContentType,
		Size:        size,
		StoragePath: path,
		CreatedAt:   now,
	}

	if err := s.attachments.Create(ctx, a); err != nil {
		// Don't leave an unreferenced blob behind.
		_ = s.blobs.Delete(ctx, path)
		return nil, fmt.Errorf("create attachment: %w", err)
	}

	_ = s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "attachment", EntityID: a.ID,
		UserID: user.ID, Action: "create", CreatedAt: now,
	})

	return a, nil
}

// GetAttachment returns attachment metadata by ID. Requires viewer role.
func (s *CommentService) GetAttachment(ctx context.Context, user *model.User, productID, id string) (*model.Attachment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	a, err := s.attachments.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.ProductID != productID {
		return nil, fmt.Errorf("%w: attachment", ErrNotFound)
	}
	return a, nil
}

// OpenAttachment returns the metadata plus a reader over the stored bytes.
// The caller must close the reader. Requires viewer role.
func (s *CommentService) OpenAttachment(ctx context.Context, user *model.User, productID, id string) (*model.Attachment, io.ReadCloser, error) {
	a, err := s.GetAttachment(ctx, user, productID, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.blobs.Open(ctx, a.StoragePath)
	if err != nil {
		return nil, nil, err
	}
	return a, rc, nil
}

// DeleteAttachment removes an attachment row and its stored bytes.
// Author or product owner can delete.
func (s *CommentService) DeleteAttachment(ctx context.Context, user *model.User, productID, id string) error {
	role, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer)
	if err != nil {
		return err
	}

	a, err := s.attachments.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if a.ProductID != productID {
		return fmt.Errorf("%w: attachment", ErrNotFound)
	}

	if a.UserID != user.ID && roleRank(role) < roleRank(model.RoleOwner) {
		return fmt.Errorf("%w: only the author or product owner can delete an attachment", ErrForbidden)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.attachments.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete attachment: %w", err)
	}
	// Best effort: the row is authoritative, so a failed unlink leaves an
	// orphan blob rather than a dangling row.
	_ = s.blobs.Delete(ctx, a.StoragePath)

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
	return s.attachments.ListByOwner(ctx, productID, ownerType, ownerID)
}
