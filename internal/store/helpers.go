package store

import "log/slog"

// scanner is satisfied by both *Row and *Rows.
type scanner interface {
	Scan(dest ...any) error
}

// closeRows closes rows and logs any error.
func closeRows(rows *Rows) {
	if err := rows.Close(); err != nil {
		slog.Error("failed to close rows", "err", err)
	}
}

// nullableText returns nil for empty strings, enabling SQL NULL storage.
func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}
