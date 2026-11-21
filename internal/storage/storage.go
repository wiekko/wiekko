// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Package storage persists attachment payloads outside the database.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrTooLarge is returned when an upload exceeds the configured size limit.
var ErrTooLarge = errors.New("file exceeds the maximum upload size")

// ErrNotFound is returned when a stored object is missing.
var ErrNotFound = errors.New("stored file not found")

// Store persists and retrieves attachment payloads.
//
// Save returns an opaque path that callers must persist alongside the
// attachment row and pass back to Open and Delete. Callers never construct
// paths themselves: the identifiers given to Save are validated, so a
// hostile value cannot escape the storage root.
// DeleteProduct discards a product's whole storage — every object it holds and
// whatever container they live in. It is idempotent: a product that never
// stored anything is not an error.
type Store interface {
	Save(ctx context.Context, productID, attachmentID string, r io.Reader) (path string, size int64, err error)
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	DeleteProduct(ctx context.Context, productID string) error
}
