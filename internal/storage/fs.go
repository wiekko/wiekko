package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FS stores attachment payloads on the local filesystem under Root.
//
// Objects live at <Root>/<productID>/<attachmentID>. Neither component may
// contain a path separator or a dot segment, so a caller cannot traverse out
// of Root. The user-supplied filename is never part of the path — it is kept
// in the database and only used for display and Content-Disposition.
type FS struct {
	Root     string
	MaxBytes int64
}

// NewFS returns a filesystem store rooted at dir, creating it if needed.
func NewFS(dir string, maxBytes int64) (*FS, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve upload dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}
	return &FS{Root: abs, MaxBytes: maxBytes}, nil
}

// safeSegment rejects anything that could alter the path structure.
func safeSegment(s string) error {
	if s == "" || s == "." || s == ".." {
		return fmt.Errorf("invalid path segment %q", s)
	}
	if strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") || strings.ContainsRune(s, 0) {
		return fmt.Errorf("invalid path segment %q", s)
	}
	return nil
}

func (f *FS) Save(ctx context.Context, productID, attachmentID string, r io.Reader) (string, int64, error) {
	if err := safeSegment(productID); err != nil {
		return "", 0, err
	}
	if err := safeSegment(attachmentID); err != nil {
		return "", 0, err
	}

	dir := filepath.Join(f.Root, productID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", 0, fmt.Errorf("create product dir: %w", err)
	}

	full := filepath.Join(dir, attachmentID)
	dst, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}

	// Read one byte past the limit so an oversized upload is detected rather
	// than silently truncated.
	limited := io.LimitReader(r, f.MaxBytes+1)
	size, copyErr := io.Copy(dst, limited)
	closeErr := dst.Close()

	switch {
	case copyErr != nil:
		_ = os.Remove(full)
		return "", 0, fmt.Errorf("write file: %w", copyErr)
	case closeErr != nil:
		_ = os.Remove(full)
		return "", 0, fmt.Errorf("close file: %w", closeErr)
	case f.MaxBytes > 0 && size > f.MaxBytes:
		_ = os.Remove(full)
		return "", 0, ErrTooLarge
	}

	// Store a relative path so the root can move between deployments.
	return filepath.ToSlash(filepath.Join(productID, attachmentID)), size, nil
}

// resolve maps a stored path back to an absolute path, refusing anything that
// does not land inside Root.
func (f *FS) resolve(path string) (string, error) {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid storage path %q", path)
	}
	for _, p := range parts {
		if err := safeSegment(p); err != nil {
			return "", err
		}
	}
	return filepath.Join(f.Root, parts[0], parts[1]), nil
}

func (f *FS) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	full, err := f.resolve(path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(full)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return file, nil
}

func (f *FS) Delete(ctx context.Context, path string) error {
	full, err := f.resolve(path)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}

// DeleteProduct removes a product's directory with everything in it. It runs
// once the product and its attachment rows are being deleted, so anything left
// under the directory is garbage — including bytes an earlier unlink failed to
// remove. Without this the emptied directory would linger under Root forever.
func (f *FS) DeleteProduct(ctx context.Context, productID string) error {
	if err := safeSegment(productID); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(f.Root, productID)); err != nil {
		return fmt.Errorf("remove product dir: %w", err)
	}
	return nil
}
