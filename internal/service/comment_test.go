package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newCommentService(comments *testutil.MockCommentStore, attachments *testutil.MockAttachmentStore, audit *testutil.MockAuditStore, roles *testutil.MockProductRoleStore) *CommentService {
	return &CommentService{
		comments:    comments,
		attachments: attachments,
		audit:       audit,
		authz:       &Authorizer{productRoles: roles},
	}
}

// --- Comments ---

func TestCreateComment(t *testing.T) {
	t.Run("success on defect (tester role)", func(t *testing.T) {
		var created *model.Comment
		svc := newCommentService(
			&testutil.MockCommentStore{
				CreateFunc: func(_ context.Context, c *model.Comment) error { created = c; return nil },
			},
			nil, noopAudit(), testerRoles(),
		)

		result, err := svc.CreateComment(context.Background(), testutil.MakeUser(), "product-001", "defect", "defect-001", "Found a bug")
		if err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
		if result.Body != "Found a bug" {
			t.Fatalf("Body = %q, want %q", result.Body, "Found a bug")
		}
		if created == nil {
			t.Fatal("comment was not created")
		}
	})

	t.Run("success on requirement (developer role)", func(t *testing.T) {
		svc := newCommentService(
			&testutil.MockCommentStore{
				CreateFunc: func(_ context.Context, _ *model.Comment) error { return nil },
			},
			nil, noopAudit(), developerRoles(),
		)

		_, err := svc.CreateComment(context.Background(), testutil.MakeUser(), "product-001", "requirement", "req-001", "Comment")
		if err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, developerRoles())
		_, err := svc.CreateComment(context.Background(), testutil.MakeUser(), "product-001", "requirement", "req-001", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateComment(context.Background(), testutil.MakeUser(), "product-001", "defect", "defect-001", "text")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetComment(t *testing.T) {
	comment := testutil.MakeComment()

	t.Run("success", func(t *testing.T) {
		svc := newCommentService(
			&testutil.MockCommentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Comment, error) { return comment, nil },
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.GetComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001")
		if err != nil {
			t.Fatalf("GetComment: %v", err)
		}
		if result.ID != comment.ID {
			t.Fatalf("ID = %q, want %q", result.ID, comment.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.GetComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateComment(t *testing.T) {
	t.Run("author can update", func(t *testing.T) {
		comment := testutil.MakeComment()
		var updated *model.Comment
		svc := newCommentService(
			&testutil.MockCommentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Comment, error) { return comment, nil },
				UpdateFunc:  func(_ context.Context, c *model.Comment) error { updated = c; return nil },
			},
			nil, noopAudit(), viewerRoles(),
		)

		result, err := svc.UpdateComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001", "Updated body")
		if err != nil {
			t.Fatalf("UpdateComment: %v", err)
		}
		if result.Body != "Updated body" {
			t.Fatalf("Body = %q, want %q", result.Body, "Updated body")
		}
		if updated == nil {
			t.Fatal("comment was not updated")
		}
	})

	t.Run("admin can update others comment", func(t *testing.T) {
		comment := testutil.MakeComment(func(c *model.Comment) { c.UserID = "user-002" })
		svc := newCommentService(
			&testutil.MockCommentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Comment, error) { return comment, nil },
				UpdateFunc:  func(_ context.Context, _ *model.Comment) error { return nil },
			},
			nil, noopAudit(), viewerRoles(),
		)

		admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })
		_, err := svc.UpdateComment(context.Background(), admin, "product-001", "comment-001", "Admin edit")
		if err != nil {
			t.Fatalf("UpdateComment: %v", err)
		}
	})

	t.Run("non-author non-admin cannot update", func(t *testing.T) {
		comment := testutil.MakeComment(func(c *model.Comment) { c.UserID = "user-002" })
		svc := newCommentService(
			&testutil.MockCommentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Comment, error) { return comment, nil },
			},
			nil, nil, viewerRoles(),
		)

		_, err := svc.UpdateComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001", "Nope")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("forbidden (no product access)", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdateComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001", "text")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteComment(t *testing.T) {
	t.Run("author can delete", func(t *testing.T) {
		comment := testutil.MakeComment()
		deleted := false
		svc := newCommentService(
			&testutil.MockCommentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Comment, error) { return comment, nil },
				DeleteFunc:  func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, noopAudit(), viewerRoles(),
		)

		err := svc.DeleteComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001")
		if err != nil {
			t.Fatalf("DeleteComment: %v", err)
		}
		if !deleted {
			t.Fatal("comment was not deleted")
		}
	})

	t.Run("product owner can delete others comment", func(t *testing.T) {
		comment := testutil.MakeComment(func(c *model.Comment) { c.UserID = "user-002" })
		deleted := false
		svc := newCommentService(
			&testutil.MockCommentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Comment, error) { return comment, nil },
				DeleteFunc:  func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		err := svc.DeleteComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001")
		if err != nil {
			t.Fatalf("DeleteComment: %v", err)
		}
		if !deleted {
			t.Fatal("comment was not deleted")
		}
	})

	t.Run("non-author non-owner cannot delete", func(t *testing.T) {
		comment := testutil.MakeComment(func(c *model.Comment) { c.UserID = "user-002" })
		svc := newCommentService(
			&testutil.MockCommentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Comment, error) { return comment, nil },
			},
			nil, nil, viewerRoles(),
		)

		err := svc.DeleteComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListComments(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newCommentService(
			&testutil.MockCommentStore{
				ListByOwnerFunc: func(_ context.Context, _, _ string) ([]*model.Comment, error) {
					return []*model.Comment{testutil.MakeComment()}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListComments(context.Background(), testutil.MakeUser(), "product-001", "requirement", "req-001")
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListComments(context.Background(), testutil.MakeUser(), "product-001", "requirement", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Attachments ---

func TestCreateAttachment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Attachment
		svc := newCommentService(nil,
			&testutil.MockAttachmentStore{
				CreateFunc: func(_ context.Context, a *model.Attachment) error { created = a; return nil },
			},
			noopAudit(), developerRoles(),
		)

		a := &model.Attachment{
			OwnerType:   "requirement",
			OwnerID:     "req-001",
			Filename:    "spec.pdf",
			ContentType: "application/pdf",
			Size:        2048,
			StoragePath: "/tmp/spec.pdf",
		}
		result, err := svc.CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", a)
		if err != nil {
			t.Fatalf("CreateAttachment: %v", err)
		}
		if result.Filename != "spec.pdf" {
			t.Fatalf("Filename = %q, want %q", result.Filename, "spec.pdf")
		}
		if created == nil {
			t.Fatal("attachment was not created")
		}
		if created.UserID != "user-001" {
			t.Fatalf("UserID = %q, want %q", created.UserID, "user-001")
		}
	})

	t.Run("missing filename", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, developerRoles())
		a := &model.Attachment{OwnerType: "requirement", OwnerID: "req-001"}
		_, err := svc.CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", a)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, forbiddenRoles())
		a := &model.Attachment{OwnerType: "requirement", OwnerID: "req-001", Filename: "x.txt"}
		_, err := svc.CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", a)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetAttachment(t *testing.T) {
	attachment := testutil.MakeAttachment()

	t.Run("success", func(t *testing.T) {
		svc := newCommentService(nil,
			&testutil.MockAttachmentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Attachment, error) { return attachment, nil },
			},
			nil, viewerRoles(),
		)

		result, err := svc.GetAttachment(context.Background(), testutil.MakeUser(), "product-001", "attachment-001")
		if err != nil {
			t.Fatalf("GetAttachment: %v", err)
		}
		if result.ID != attachment.ID {
			t.Fatalf("ID = %q, want %q", result.ID, attachment.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.GetAttachment(context.Background(), testutil.MakeUser(), "product-001", "attachment-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteAttachment(t *testing.T) {
	t.Run("author can delete", func(t *testing.T) {
		attachment := testutil.MakeAttachment()
		deleted := false
		svc := newCommentService(nil,
			&testutil.MockAttachmentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Attachment, error) { return attachment, nil },
				DeleteFunc:  func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			noopAudit(), viewerRoles(),
		)

		err := svc.DeleteAttachment(context.Background(), testutil.MakeUser(), "product-001", "attachment-001")
		if err != nil {
			t.Fatalf("DeleteAttachment: %v", err)
		}
		if !deleted {
			t.Fatal("attachment was not deleted")
		}
	})

	t.Run("product owner can delete others attachment", func(t *testing.T) {
		attachment := testutil.MakeAttachment(func(a *model.Attachment) { a.UserID = "user-002" })
		deleted := false
		svc := newCommentService(nil,
			&testutil.MockAttachmentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Attachment, error) { return attachment, nil },
				DeleteFunc:  func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		err := svc.DeleteAttachment(context.Background(), testutil.MakeUser(), "product-001", "attachment-001")
		if err != nil {
			t.Fatalf("DeleteAttachment: %v", err)
		}
		if !deleted {
			t.Fatal("attachment was not deleted")
		}
	})

	t.Run("non-author non-owner cannot delete", func(t *testing.T) {
		attachment := testutil.MakeAttachment(func(a *model.Attachment) { a.UserID = "user-002" })
		svc := newCommentService(nil,
			&testutil.MockAttachmentStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Attachment, error) { return attachment, nil },
			},
			nil, viewerRoles(),
		)

		err := svc.DeleteAttachment(context.Background(), testutil.MakeUser(), "product-001", "attachment-001")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListAttachments(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newCommentService(nil,
			&testutil.MockAttachmentStore{
				ListByOwnerFunc: func(_ context.Context, _, _ string) ([]*model.Attachment, error) {
					return []*model.Attachment{testutil.MakeAttachment()}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListAttachments(context.Background(), testutil.MakeUser(), "product-001", "requirement", "req-001")
		if err != nil {
			t.Fatalf("ListAttachments: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.ListAttachments(context.Background(), testutil.MakeUser(), "product-001", "requirement", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
