package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/access"
	"nexmap.net/internal/model"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

// DiscussionService handles the discussion attached to any entity: its comment
// thread and its file attachments. The two travel together — same polymorphic
// owner columns, same lifecycle — and this is the only service holding the blob
// store, so attachment bytes are created and unlinked here and nowhere else.
type DiscussionService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx          store.Transactor
	comments    store.CommentStore
	attachments store.AttachmentStore
	// defectLinks removes the defect links pointing at a deleted entity.
	defectLinks defectLinkRemover
	audit       store.AuditStore
	blobs       storage.Store
	policy      storage.Policy
	authz       *Authorizer
	owns        *ownership
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

// requireOwnerScope checks that a token may read the item a comment or
// attachment belongs to (SC-405). The discussion scope itself is checked per
// route. An empty owner type stands for items of every type.
func requireOwnerScope(ctx context.Context, ownerType string) error {
	r := access.FromContext(ctx)
	scopes := []access.Scope{access.ScopeForOwnerType(ownerType)}
	if ownerType == "" {
		scopes = []access.Scope{access.Structure, access.Backlog, access.Documents, access.Defects, access.Tests}
	}
	for _, sc := range scopes {
		if !r.Allows(sc, access.Read) {
			return fmt.Errorf("%w: token lacks the %s scope", ErrForbidden, sc)
		}
	}
	return nil
}

// CreateComment creates a new comment. Minimum role depends on owner type.
func (s *DiscussionService) CreateComment(ctx context.Context, user *model.User, productID, ownerType, ownerID, body string) (*model.Comment, error) {
	if !model.ValidOwnerType(ownerType) {
		return nil, fmt.Errorf("invalid comment owner type: %q", ownerType)
	}
	if _, err := s.authz.RequireRole(ctx, user, productID, minRoleForOwnerType(ownerType)); err != nil {
		return nil, err
	}
	if err := requireOwnerScope(ctx, ownerType); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{ownerType, ownerID}); err != nil {
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
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.holdOwner(ctx, productID, ownerType, ownerID); err != nil {
			return err
		}
		if err := s.comments.Create(ctx, c); err != nil {
			return fmt.Errorf("create comment: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "comment", EntityID: c.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

	return c, nil
}

// GetComment returns a comment by ID. Requires viewer role.
func (s *DiscussionService) GetComment(ctx context.Context, user *model.User, productID, id string) (*model.Comment, error) {
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
	if err := requireOwnerScope(ctx, c.OwnerType); err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateComment modifies a comment body. Only the author can update.
func (s *DiscussionService) UpdateComment(ctx context.Context, user *model.User, productID, id, body string) (*model.Comment, error) {
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

	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.comments.Update(ctx, c); err != nil {
			return fmt.Errorf("update comment: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "comment", EntityID: c.ID,
			UserID: user.ID, Action: "update", CreatedAt: now,
		})
	}); err != nil {
		return nil, err
	}

	return c, nil
}

// DeleteComment removes a comment. Author or product owner can delete.
func (s *DiscussionService) DeleteComment(ctx context.Context, user *model.User, productID, id string) error {
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
	if err := requireOwnerScope(ctx, c.OwnerType); err != nil {
		return err
	}

	if c.UserID != user.ID && roleRank(role) < roleRank(model.RoleOwner) {
		return fmt.Errorf("%w: only the author or product owner can delete a comment", ErrForbidden)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.comments.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete comment: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "comment", EntityID: id,
			UserID: user.ID, Action: "delete", CreatedAt: now,
		})
	}); err != nil {
		return err
	}

	return nil
}

// ListComments returns comments for an entity. Requires viewer role.
func (s *DiscussionService) ListComments(ctx context.Context, user *model.User, productID, ownerType, ownerID string) ([]*model.Comment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := requireOwnerScope(ctx, ownerType); err != nil {
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
func (s *DiscussionService) CreateAttachment(ctx context.Context, user *model.User, productID string, input CreateAttachmentInput) (*model.Attachment, error) {
	if !model.ValidOwnerType(input.OwnerType) {
		return nil, fmt.Errorf("invalid attachment owner type: %q", input.OwnerType)
	}
	if _, err := s.authz.RequireRole(ctx, user, productID, minRoleForOwnerType(input.OwnerType)); err != nil {
		return nil, err
	}
	if err := requireOwnerScope(ctx, input.OwnerType); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{input.OwnerType, input.OwnerID}); err != nil {
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

	// Peek the leading bytes so the type policy can inspect real content
	// rather than the client-declared MIME type, then hand the whole stream
	// (peeked bytes first) to storage.
	head := make([]byte, storage.SniffLen)
	n, err := io.ReadFull(input.Content, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	head = head[:n]

	if err := s.policy.Check(input.Filename, head); err != nil {
		return nil, err
	}

	content := io.MultiReader(bytes.NewReader(head), input.Content)

	id := uuid.New().String()
	path, size, err := s.blobs.Save(ctx, productID, id, content)
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

	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.holdOwner(ctx, productID, input.OwnerType, input.OwnerID); err != nil {
			return err
		}
		if err := s.attachments.Create(ctx, a); err != nil {
			return fmt.Errorf("create attachment: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "attachment", EntityID: a.ID,
			UserID: user.ID, Action: "create", CreatedAt: now,
		})
	})
	if err != nil {
		// Don't leave an unreferenced blob behind.
		_ = s.blobs.Delete(context.WithoutCancel(ctx), path)
		return nil, err
	}

	return a, nil
}

// GetAttachment returns attachment metadata by ID. Requires viewer role.
func (s *DiscussionService) GetAttachment(ctx context.Context, user *model.User, productID, id string) (*model.Attachment, error) {
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
	if err := requireOwnerScope(ctx, a.OwnerType); err != nil {
		return nil, err
	}
	return a, nil
}

// OpenAttachment returns the metadata plus a reader over the stored bytes.
// The caller must close the reader. Requires viewer role.
func (s *DiscussionService) OpenAttachment(ctx context.Context, user *model.User, productID, id string) (*model.Attachment, io.ReadCloser, error) {
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
func (s *DiscussionService) DeleteAttachment(ctx context.Context, user *model.User, productID, id string) error {
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
	if err := requireOwnerScope(ctx, a.OwnerType); err != nil {
		return err
	}

	if a.UserID != user.ID && roleRank(role) < roleRank(model.RoleOwner) {
		return fmt.Errorf("%w: only the author or product owner can delete an attachment", ErrForbidden)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.attachments.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete attachment: %w", err)
		}
		return s.audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: "attachment", EntityID: id,
			UserID: user.ID, Action: "delete", CreatedAt: now,
		})
	}); err != nil {
		return err
	}
	// Only after the commit, and best effort: the row is authoritative, so a
	// failed unlink leaves an orphan blob rather than a dangling row.
	_ = s.blobs.Delete(context.WithoutCancel(ctx), a.StoragePath)

	return nil
}

// holdOwner takes the owner's lock for the current transaction and checks
// that the owner still exists. PurgeOwner takes the same lock, so a comment or
// attachment cannot be added to an entity whose delete is under way and end
// up pointing at nothing.
func (s *DiscussionService) holdOwner(ctx context.Context, productID, ownerType, ownerID string) error {
	if err := lock(ctx, s.tx, lockOwner(ownerID)); err != nil {
		return err
	}
	return s.owns.require(ctx, productID, ref{ownerType, ownerID})
}

// ownerPurger discards what hangs off an entity that is being deleted.
// Entity services depend on this narrow view of DiscussionService.
type ownerPurger interface {
	PurgeOwner(ctx context.Context, productID, ownerType, ownerID string) (afterCommit func(), err error)
}

// defectLinkRemover is the part of store.DefectStore PurgeOwner needs.
type defectLinkRemover interface {
	RemoveLinksToTarget(ctx context.Context, targetID string) error
}

// productPurger discards the stored attachment bytes of a product that is being
// deleted. ProductService depends on this narrow view of DiscussionService.
type productPurger interface {
	PurgeProduct(ctx context.Context, productID string) error
}

// PurgeOwner removes every row that points at one entity through a
// polymorphic column — its comments, its attachments and the defect links
// targeting it — so no foreign key can cascade them away with the entity.
//
// Call it with the context of the transaction that deletes the entity: the
// rows then go with the entity or not at all. The attachment bytes cannot
// join a transaction, so they are left to afterCommit, which the caller runs
// once the transaction has committed (and drops if it rolled back).
//
// It takes the owner's lock first, as do the inserts that attach something to
// an owner (comments, attachments, links), so nothing can be attached between
// this purge and the commit of the delete.
//
// It authorizes nothing itself: comments and attachments have no independent
// existence, so the caller deleting the owning entity has already established
// the right to discard them. Author-only rules deliberately do not apply here —
// deleting an entity takes its whole thread with it, whoever wrote it.
func (s *DiscussionService) PurgeOwner(ctx context.Context, productID, ownerType, ownerID string) (func(), error) {
	if err := lock(ctx, s.tx, lockOwner(ownerID)); err != nil {
		return nil, err
	}
	// Read the storage paths before the rows go away.
	attachments, err := s.attachments.ListByOwner(ctx, productID, ownerType, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list attachments of %s %s: %w", ownerType, ownerID, err)
	}

	if err := s.attachments.DeleteByOwner(ctx, productID, ownerType, ownerID); err != nil {
		return nil, fmt.Errorf("delete attachments of %s %s: %w", ownerType, ownerID, err)
	}
	if err := s.comments.DeleteByOwner(ctx, productID, ownerType, ownerID); err != nil {
		return nil, fmt.Errorf("delete comments of %s %s: %w", ownerType, ownerID, err)
	}
	if s.defectLinks != nil {
		if err := s.defectLinks.RemoveLinksToTarget(ctx, ownerID); err != nil {
			return nil, fmt.Errorf("delete defect links to %s %s: %w", ownerType, ownerID, err)
		}
	}

	// Detached from ctx: the request may be over by the time this runs, and
	// the unlink should still happen.
	blobCtx := context.WithoutCancel(ctx)
	return func() { s.purgeBlobs(blobCtx, attachments) }, nil
}

// PurgeProduct removes the attachment storage of a product. The rows themselves
// are cascaded away by the products foreign key, so this covers only what SQL
// cannot reach: the files on disk and the directory that holds them.
//
// It discards the product's storage wholesale rather than walking the rows,
// which also collects blobs whose row-driven unlink failed at some earlier
// point — nothing under a deleted product is worth keeping.
func (s *DiscussionService) PurgeProduct(ctx context.Context, productID string) error {
	if err := s.blobs.DeleteProduct(ctx, productID); err != nil {
		return fmt.Errorf("purge attachment storage of product %s: %w", productID, err)
	}
	return nil
}

// purgeBlobs unlinks stored bytes best-effort, matching DeleteAttachment: the
// row is authoritative, so a failed unlink leaves an orphan blob rather than
// blocking a delete that has already happened.
func (s *DiscussionService) purgeBlobs(ctx context.Context, attachments []*model.Attachment) {
	for _, a := range attachments {
		_ = s.blobs.Delete(ctx, a.StoragePath)
	}
}

// ListAttachments returns attachments for an entity. Requires viewer role.
func (s *DiscussionService) ListAttachments(ctx context.Context, user *model.User, productID, ownerType, ownerID string) ([]*model.Attachment, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := requireOwnerScope(ctx, ownerType); err != nil {
		return nil, err
	}
	return s.attachments.ListByOwner(ctx, productID, ownerType, ownerID)
}
