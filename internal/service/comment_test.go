package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/testutil"
)

func newCommentService(comments *testutil.MockCommentStore, attachments *testutil.MockAttachmentStore, audit *testutil.MockAuditStore, roles *testutil.MockProductRoleStore) *CommentService {
	return newCommentServiceWithBlobs(comments, attachments, audit, roles, memBlobs())
}

func newCommentServiceWithBlobs(comments *testutil.MockCommentStore, attachments *testutil.MockAttachmentStore, audit *testutil.MockAuditStore, roles *testutil.MockProductRoleStore, blobs storage.Store) *CommentService {
	return &CommentService{
		comments:    comments,
		attachments: attachments,
		audit:       audit,
		blobs:       blobs,
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
				ListByOwnerFunc: func(_ context.Context, _, _, _ string) ([]*model.Comment, error) {
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

		result, err := svc.CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", CreateAttachmentInput{
			OwnerType:   model.OwnerTypeRequirement,
			OwnerID:     "req-001",
			Filename:    "spec.pdf",
			ContentType: "application/pdf",
			Content:     strings.NewReader("hello world"),
		})
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
		_, err := svc.CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", CreateAttachmentInput{
			OwnerType: model.OwnerTypeRequirement, OwnerID: "req-001", Content: strings.NewReader("x"),
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newCommentService(nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", CreateAttachmentInput{
			OwnerType: model.OwnerTypeRequirement, OwnerID: "req-001", Filename: "x.txt", Content: strings.NewReader("x"),
		})
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
				ListByOwnerFunc: func(_ context.Context, _, _, _ string) ([]*model.Attachment, error) {
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

// memBlobs is an in-memory storage.Store for tests.
type memBlobStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

func memBlobs() *memBlobStore {
	return &memBlobStore{files: map[string][]byte{}}
}

func (m *memBlobStore) Save(_ context.Context, productID, attachmentID string, r io.Reader) (string, int64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", 0, err
	}
	path := productID + "/" + attachmentID
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = b
	return path, int64(len(b)), nil
}

func (m *memBlobStore) Open(_ context.Context, path string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[path]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(strings.NewReader(string(b))), nil
}

func (m *memBlobStore) Delete(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, path)
	return nil
}

// --- Product scoping and owner-type validation ---

// A comment reached through the wrong product must not be readable, even
// though the caller holds a legitimate role on the product they named.
func TestCommentsAreScopedToTheirProduct(t *testing.T) {
	foreign := testutil.MakeComment(func(c *model.Comment) { c.ProductID = "other-product" })
	store := &testutil.MockCommentStore{
		GetByIDFunc: func(_ context.Context, _ string) (*model.Comment, error) { return foreign, nil },
		DeleteFunc:  func(_ context.Context, _ string) error { t.Fatal("delete reached the store"); return nil },
		UpdateFunc:  func(_ context.Context, _ *model.Comment) error { t.Fatal("update reached the store"); return nil },
	}

	t.Run("get", func(t *testing.T) {
		svc := newCommentService(store, nil, noopAudit(), ownerRoles())
		_, err := svc.GetComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		svc := newCommentService(store, nil, noopAudit(), ownerRoles())
		err := svc.DeleteComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("update", func(t *testing.T) {
		svc := newCommentService(store, nil, noopAudit(), ownerRoles())
		_, err := svc.UpdateComment(context.Background(), testutil.MakeUser(), "product-001", "comment-001", "edited")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestAttachmentsAreScopedToTheirProduct(t *testing.T) {
	foreign := testutil.MakeAttachment(func(a *model.Attachment) { a.ProductID = "other-product" })
	store := &testutil.MockAttachmentStore{
		GetByIDFunc: func(_ context.Context, _ string) (*model.Attachment, error) { return foreign, nil },
		DeleteFunc:  func(_ context.Context, _ string) error { t.Fatal("delete reached the store"); return nil },
	}

	svc := newCommentService(nil, store, noopAudit(), ownerRoles())

	if _, err := svc.GetAttachment(context.Background(), testutil.MakeUser(), "product-001", "attachment-001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAttachment err = %v, want ErrNotFound", err)
	}
	if err := svc.DeleteAttachment(context.Background(), testutil.MakeUser(), "product-001", "attachment-001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteAttachment err = %v, want ErrNotFound", err)
	}
}

func TestCreateStampsTheAuthorizingProduct(t *testing.T) {
	var created *model.Comment
	svc := newCommentService(
		&testutil.MockCommentStore{
			CreateFunc: func(_ context.Context, c *model.Comment) error { created = c; return nil },
		},
		nil, noopAudit(), developerRoles(),
	)

	if _, err := svc.CreateComment(context.Background(), testutil.MakeUser(), "product-001", model.OwnerTypeRequirement, "req-001", "hi"); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if created.ProductID != "product-001" {
		t.Fatalf("ProductID = %q, want product-001", created.ProductID)
	}
}

func TestOwnerTypeIsValidated(t *testing.T) {
	svc := newCommentService(
		&testutil.MockCommentStore{
			CreateFunc: func(_ context.Context, _ *model.Comment) error { t.Fatal("create reached the store"); return nil },
		},
		&testutil.MockAttachmentStore{
			CreateFunc: func(_ context.Context, _ *model.Attachment) error { t.Fatal("create reached the store"); return nil },
		},
		noopAudit(), ownerRoles(),
	)

	if _, err := svc.CreateComment(context.Background(), testutil.MakeUser(), "product-001", "sprint", "sprint-001", "hi"); err == nil {
		t.Fatal("expected an error for an unsupported comment owner type")
	}
	if _, err := svc.CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", CreateAttachmentInput{
		OwnerType: "sprint", OwnerID: "sprint-001", Filename: "x.txt", Content: strings.NewReader("x"),
	}); err == nil {
		t.Fatal("expected an error for an unsupported attachment owner type")
	}
}

func TestAllSevenOwnerTypesAreAccepted(t *testing.T) {
	for _, ot := range []string{
		model.OwnerTypeRequirement, model.OwnerTypeConstraint, model.OwnerTypeTestCase,
		model.OwnerTypeComponent, model.OwnerTypeCapability, model.OwnerTypeMilestone,
		model.OwnerTypeDefect,
	} {
		svc := newCommentService(
			&testutil.MockCommentStore{
				CreateFunc: func(_ context.Context, _ *model.Comment) error { return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)
		if _, err := svc.CreateComment(context.Background(), testutil.MakeUser(), "product-001", ot, "owner-001", "hi"); err != nil {
			t.Fatalf("CreateComment(%q): %v", ot, err)
		}
	}
}

// Uploading stores the bytes and removes them again on delete.
func TestAttachmentBytesRoundTripAndAreCleanedUp(t *testing.T) {
	blobs := memBlobs()
	var stored *model.Attachment
	svc := newCommentServiceWithBlobs(nil,
		&testutil.MockAttachmentStore{
			CreateFunc:  func(_ context.Context, a *model.Attachment) error { stored = a; return nil },
			GetByIDFunc: func(_ context.Context, _ string) (*model.Attachment, error) { return stored, nil },
			DeleteFunc:  func(_ context.Context, _ string) error { return nil },
		},
		noopAudit(), ownerRoles(), blobs,
	)

	user := testutil.MakeUser()
	created, err := svc.CreateAttachment(context.Background(), user, "product-001", CreateAttachmentInput{
		OwnerType: model.OwnerTypeDefect, OwnerID: "defect-001",
		Filename: "notes.txt", ContentType: "text/plain", Content: strings.NewReader("payload"),
	})
	if err != nil {
		t.Fatalf("CreateAttachment: %v", err)
	}
	if created.Size != 7 {
		t.Fatalf("Size = %d, want 7", created.Size)
	}
	if created.StoragePath == "" {
		t.Fatal("StoragePath was not set by the service")
	}

	_, rc, err := svc.OpenAttachment(context.Background(), user, "product-001", created.ID)
	if err != nil {
		t.Fatalf("OpenAttachment: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "payload" {
		t.Fatalf("content = %q, want %q", got, "payload")
	}

	if err := svc.DeleteAttachment(context.Background(), user, "product-001", created.ID); err != nil {
		t.Fatalf("DeleteAttachment: %v", err)
	}
	if _, err := blobs.Open(context.Background(), created.StoragePath); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("delete left the stored bytes behind")
	}
}

// A failed metadata write must not leave an unreferenced blob on disk.
func TestFailedMetadataWriteRemovesTheBlob(t *testing.T) {
	blobs := memBlobs()
	svc := newCommentServiceWithBlobs(nil,
		&testutil.MockAttachmentStore{
			CreateFunc: func(_ context.Context, _ *model.Attachment) error { return errors.New("db down") },
		},
		noopAudit(), ownerRoles(), blobs,
	)

	_, err := svc.CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", CreateAttachmentInput{
		OwnerType: model.OwnerTypeDefect, OwnerID: "defect-001",
		Filename: "notes.txt", Content: strings.NewReader("payload"),
	})
	if err == nil {
		t.Fatal("expected the store error to surface")
	}
	if len(blobs.files) != 0 {
		t.Fatalf("orphan blob left behind: %v", blobs.files)
	}
}

// The upload policy inspects real bytes, not the client-declared MIME type.
func TestCreateAttachmentEnforcesTheUploadPolicy(t *testing.T) {
	zipMagic := string([]byte{'P', 'K', 0x03, 0x04, 0, 0, 0, 0})

	newSvc := func(blobs *memBlobStore) *CommentService {
		svc := newCommentServiceWithBlobs(nil,
			&testutil.MockAttachmentStore{
				CreateFunc: func(_ context.Context, _ *model.Attachment) error { return nil },
			},
			noopAudit(), ownerRoles(), blobs,
		)
		svc.policy = storage.NewPolicy("images,logs")
		return svc
	}

	t.Run("rejects a disallowed extension", func(t *testing.T) {
		blobs := memBlobs()
		_, err := newSvc(blobs).CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", CreateAttachmentInput{
			OwnerType: model.OwnerTypeDefect, OwnerID: "defect-001",
			Filename: "payload.zip", Content: strings.NewReader(zipMagic),
		})
		if !errors.Is(err, storage.ErrTypeNotAllowed) {
			t.Fatalf("err = %v, want ErrTypeNotAllowed", err)
		}
		if len(blobs.files) != 0 {
			t.Fatal("a rejected upload was written to storage")
		}
	})

	t.Run("rejects a lying content type", func(t *testing.T) {
		blobs := memBlobs()
		// Client claims text/plain; the bytes are a zip.
		_, err := newSvc(blobs).CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", CreateAttachmentInput{
			OwnerType: model.OwnerTypeDefect, OwnerID: "defect-001",
			Filename: "notes.log", ContentType: "text/plain", Content: strings.NewReader(zipMagic),
		})
		if !errors.Is(err, storage.ErrTypeNotAllowed) {
			t.Fatalf("err = %v, want ErrTypeNotAllowed", err)
		}
		if len(blobs.files) != 0 {
			t.Fatal("a rejected upload was written to storage")
		}
	})

	t.Run("accepts an allowed file and stores every byte", func(t *testing.T) {
		blobs := memBlobs()
		// Longer than the 512-byte sniff window, so the peeked prefix must be
		// stitched back onto the rest of the stream.
		body := strings.Repeat("log line\n", 200)
		created, err := newSvc(blobs).CreateAttachment(context.Background(), testutil.MakeUser(), "product-001", CreateAttachmentInput{
			OwnerType: model.OwnerTypeDefect, OwnerID: "defect-001",
			Filename: "app.log", Content: strings.NewReader(body),
		})
		if err != nil {
			t.Fatalf("CreateAttachment: %v", err)
		}
		if created.Size != int64(len(body)) {
			t.Fatalf("Size = %d, want %d — the sniff prefix was lost", created.Size, len(body))
		}
		stored := string(blobs.files[created.StoragePath])
		if stored != body {
			t.Fatalf("stored content differs from the upload (len %d vs %d)", len(stored), len(body))
		}
	})
}
