package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestFS(t *testing.T, maxBytes int64) *FS {
	t.Helper()
	fs, err := NewFS(t.TempDir(), maxBytes)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	return fs
}

func TestFSRoundTrip(t *testing.T) {
	fs := newTestFS(t, 1<<20)
	ctx := context.Background()

	path, size, err := fs.Save(ctx, "product-001", "attachment-001", strings.NewReader("hello world"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if size != 11 {
		t.Fatalf("size = %d, want 11", size)
	}
	if path != "product-001/attachment-001" {
		t.Fatalf("path = %q, want product-001/attachment-001", path)
	}

	rc, err := fs.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("content = %q, want %q", got, "hello world")
	}

	if err := fs.Delete(ctx, path); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := fs.Open(ctx, path); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open after delete: err = %v, want ErrNotFound", err)
	}
}

func TestFSDeleteIsIdempotent(t *testing.T) {
	fs := newTestFS(t, 1<<20)
	if err := fs.Delete(context.Background(), "product-001/missing"); err != nil {
		t.Fatalf("Delete of a missing object should succeed, got %v", err)
	}
}

// Deleting a product must leave nothing behind — not the files, and not the
// directory that held them (regression: the emptied directory stayed under
// Root after the product was gone).
func TestFSDeleteProduct(t *testing.T) {
	fs := newTestFS(t, 1<<20)
	ctx := context.Background()

	for _, id := range []string{"attachment-001", "attachment-002"} {
		if _, _, err := fs.Save(ctx, "product-001", id, strings.NewReader("x")); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	if _, _, err := fs.Save(ctx, "product-002", "attachment-003", strings.NewReader("y")); err != nil {
		t.Fatalf("Save into the second product: %v", err)
	}

	if err := fs.DeleteProduct(ctx, "product-001"); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fs.Root, "product-001")); !os.IsNotExist(err) {
		t.Fatal("the product directory survived DeleteProduct")
	}
	if _, err := os.Stat(filepath.Join(fs.Root, "product-002", "attachment-003")); err != nil {
		t.Fatalf("another product's files were touched: %v", err)
	}
}

func TestFSDeleteProductIsIdempotent(t *testing.T) {
	fs := newTestFS(t, 1<<20)
	if err := fs.DeleteProduct(context.Background(), "product-001"); err != nil {
		t.Fatalf("DeleteProduct of a product that stored nothing should succeed, got %v", err)
	}
}

// A hostile product identifier must not turn a purge into an arbitrary
// recursive delete.
func TestFSDeleteProductRejectsTraversal(t *testing.T) {
	fs := newTestFS(t, 1<<20)
	ctx := context.Background()

	for _, bad := range []string{"..", "../evil", "a/b", `a\b`, "", ".", "..\x00", "/etc"} {
		if err := fs.DeleteProduct(ctx, bad); err == nil {
			t.Fatalf("DeleteProduct accepted a hostile productID %q", bad)
		}
	}
}

func TestFSRejectsOversizedUpload(t *testing.T) {
	fs := newTestFS(t, 8)
	ctx := context.Background()

	_, _, err := fs.Save(ctx, "product-001", "attachment-001", strings.NewReader("123456789"))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}

	// The partial write must not be left behind.
	if _, err := os.Stat(filepath.Join(fs.Root, "product-001", "attachment-001")); !os.IsNotExist(err) {
		t.Fatal("an oversized upload left a file behind")
	}
}

func TestFSAcceptsExactlyMaxBytes(t *testing.T) {
	fs := newTestFS(t, 8)
	_, size, err := fs.Save(context.Background(), "product-001", "attachment-001", strings.NewReader("12345678"))
	if err != nil {
		t.Fatalf("Save at exactly the limit: %v", err)
	}
	if size != 8 {
		t.Fatalf("size = %d, want 8", size)
	}
}

// A hostile identifier must not be able to escape the storage root.
func TestFSRejectsTraversal(t *testing.T) {
	fs := newTestFS(t, 1<<20)
	ctx := context.Background()

	hostile := []string{"..", "../evil", "a/b", `a\b`, "", ".", "..\x00"}

	for _, bad := range hostile {
		if _, _, err := fs.Save(ctx, bad, "attachment-001", strings.NewReader("x")); err == nil {
			t.Fatalf("Save accepted a hostile productID %q", bad)
		}
		if _, _, err := fs.Save(ctx, "product-001", bad, strings.NewReader("x")); err == nil {
			t.Fatalf("Save accepted a hostile attachmentID %q", bad)
		}
	}

	for _, bad := range []string{"../etc/passwd", "product-001/../../etc/passwd", "/etc/passwd", "single", "a/b/c"} {
		if _, err := fs.Open(ctx, bad); err == nil {
			t.Fatalf("Open accepted a hostile path %q", bad)
		}
		if err := fs.Delete(ctx, bad); err == nil {
			t.Fatalf("Delete accepted a hostile path %q", bad)
		}
	}
}

func TestFSSaveRefusesToOverwrite(t *testing.T) {
	fs := newTestFS(t, 1<<20)
	ctx := context.Background()

	if _, _, err := fs.Save(ctx, "product-001", "attachment-001", strings.NewReader("first")); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if _, _, err := fs.Save(ctx, "product-001", "attachment-001", strings.NewReader("second")); err == nil {
		t.Fatal("Save overwrote an existing object")
	}
}
