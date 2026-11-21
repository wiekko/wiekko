// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"database/sql"
	"testing"
	"time"
)

func TestRebind(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"no placeholders", `SELECT 1`, `SELECT 1`},
		{"numbered in order", `SELECT * FROM t WHERE a = ? AND b IN (?, ?)`, `SELECT * FROM t WHERE a = $1 AND b IN ($2, $3)`},
		{"string literal", `SELECT '?', ? FROM t WHERE x LIKE ? ESCAPE '\'`, `SELECT '?', $1 FROM t WHERE x LIKE $2 ESCAPE '\'`},
		{"doubled quote", `SELECT 'it''s ?', ?`, `SELECT 'it''s ?', $1`},
		{"quoted identifier", `SELECT "odd?col" FROM t WHERE id = ?`, `SELECT "odd?col" FROM t WHERE id = $1`},
		{"line comment", "SELECT ? -- why?\nFROM t WHERE id = ?", "SELECT $1 -- why?\nFROM t WHERE id = $2"},
		{"trailing comment", "SELECT ? -- why?", "SELECT $1 -- why?"},
		{"unterminated literal", `SELECT ?, 'oops ?`, `SELECT $1, 'oops ?`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rebind(Postgres, tt.in); got != tt.want {
				t.Errorf("rebind(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
			if got := rebind(SQLite, tt.in); got != tt.in {
				t.Errorf("sqlite rebind changed the query: %q", got)
			}
		})
	}
}

func TestTextDestNormalisesTimestamps(t *testing.T) {
	berlin := time.FixedZone("CET", 3600)
	at := time.Date(2026, 11, 7, 19, 30, 5, 0, berlin)

	var s string
	if err := (&textDest{dst: &s}).Scan(at); err != nil || s != "2026-11-07T18:30:05Z" {
		t.Errorf("time.Time -> %q, %v", s, err)
	}
	if err := (&textDest{dst: &s}).Scan("2026-01-01T00:00:00Z"); err != nil || s != "2026-01-01T00:00:00Z" {
		t.Errorf("string -> %q, %v", s, err)
	}
	if err := (&textDest{dst: &s}).Scan([]byte("abc")); err != nil || s != "abc" {
		t.Errorf("[]byte -> %q, %v", s, err)
	}
	if err := (&textDest{dst: &s}).Scan(nil); err == nil {
		t.Error("NULL into string should fail")
	}

	var ns sql.NullString
	if err := (&nullTextDest{dst: &ns}).Scan(at.Add(500 * time.Millisecond)); err != nil || ns.String != "2026-11-07T18:30:05.5Z" || !ns.Valid {
		t.Errorf("time.Time -> %+v, %v", ns, err)
	}
	if err := (&nullTextDest{dst: &ns}).Scan(nil); err != nil || ns.Valid {
		t.Errorf("NULL -> %+v, %v", ns, err)
	}
}
