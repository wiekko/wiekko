package store

import (
	"database/sql"
	"log/slog"
	"time"
)

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// closeRows closes sql.Rows and logs any error.
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Error("failed to close rows", "err", err)
	}
}

// boolToInt converts a bool to 0 or 1 for SQLite storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullableText returns nil for empty strings, enabling SQL NULL storage.
func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// scanTimestamp normalises a scanned timestamp value to an RFC 3339 string.
// SQLite returns string; PostgreSQL (TIMESTAMPTZ) returns time.Time.
func scanTimestamp(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case time.Time:
		if v.IsZero() {
			return ""
		}
		return v.Format(time.RFC3339)
	case []byte:
		return string(v)
	default:
		return ""
	}
}

// scanBool normalises a scanned boolean value.
// SQLite returns int64; PostgreSQL (BOOLEAN) returns bool.
func scanBool(val any) bool {
	switch v := val.(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}
