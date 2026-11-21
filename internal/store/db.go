// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Dialect names the SQL engine behind a DB. Its values match the configured
// driver names (NEXMAP_DB_DRIVER).
type Dialect string

// Supported dialects.
const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// ParseDialect maps a configured driver name to a Dialect.
func ParseDialect(driver string) (Dialect, error) {
	switch Dialect(driver) {
	case SQLite, Postgres:
		return Dialect(driver), nil
	default:
		return "", fmt.Errorf("unsupported database dialect: %q", driver)
	}
}

// DB is the only door from the stores to the database. Queries are written
// once, in the subset of SQL both engines understand, using `?` placeholders;
// DB papers over the two differences that cannot be avoided in the text:
//
//   - Placeholders: `?` is rewritten to `$1…$n` for Postgres. The rewrite skips
//     string literals, quoted identifiers and `--` comments, so a literal `?`
//     inside quotes is left alone.
//   - Timestamps: a TIMESTAMPTZ column comes back from Postgres as time.Time
//     while SQLite hands back the stored TEXT. Scanning into *string or
//     *sql.NullString normalises time.Time values to UTC RFC 3339 (see
//     FormatTimestamp), so models see the same string on both engines.
//
// Everything else crosses the driver boundary as a native Go value: bools are
// passed as bool (SQLite stores 0/1 and scans them back into bool), and
// timestamps are generated in Go with Now rather than by SQL functions such as
// datetime('now') or NOW().
type DB struct {
	db      *sql.DB
	dialect Dialect
}

// NewDB wraps db for the given dialect.
func NewDB(db *sql.DB, dialect Dialect) *DB {
	return &DB{db: db, dialect: dialect}
}

// Dialect reports the engine behind d.
func (d *DB) Dialect() Dialect { return d.dialect }

// Raw returns the underlying connection pool.
func (d *DB) Raw() *sql.DB { return d.db }

// ExecContext runs a statement that returns no rows.
func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if tx := txFrom(ctx); tx != nil {
		return tx.ExecContext(ctx, query, args...)
	}
	return d.db.ExecContext(ctx, rebind(d.dialect, query), args...)
}

// QueryContext runs a query that returns rows.
func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*Rows, error) {
	if tx := txFrom(ctx); tx != nil {
		return tx.QueryContext(ctx, query, args...)
	}
	rows, err := d.db.QueryContext(ctx, rebind(d.dialect, query), args...)
	if err != nil {
		return nil, err
	}
	return &Rows{Rows: rows}, nil
}

// QueryRowContext runs a query expected to return at most one row.
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *Row {
	if tx := txFrom(ctx); tx != nil {
		return tx.QueryRowContext(ctx, query, args...)
	}
	return &Row{row: d.db.QueryRowContext(ctx, rebind(d.dialect, query), args...)}
}

// Exec is ExecContext with a background context (for tests and seeding).
func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.ExecContext(context.Background(), query, args...)
}

// QueryRow is QueryRowContext with a background context (for tests and seeding).
func (d *DB) QueryRow(query string, args ...any) *Row {
	return d.QueryRowContext(context.Background(), query, args...)
}

// BeginTx starts a transaction. When ctx already carries one (see InTx), the
// returned Tx joins it: its Commit and Rollback do nothing, and the outer
// transaction decides the outcome.
func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	if outer := txFrom(ctx); outer != nil {
		return &Tx{tx: outer.tx, dialect: d.dialect, joined: true}, nil
	}
	tx, err := d.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx, dialect: d.dialect}, nil
}

// Tx is a transaction with the same rewriting as DB.
type Tx struct {
	tx      *sql.Tx
	dialect Dialect
	// joined marks a Tx that runs inside an outer transaction it does not own.
	joined bool
}

// ExecContext runs a statement that returns no rows.
func (t *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, rebind(t.dialect, query), args...)
}

// QueryContext runs a query that returns rows.
func (t *Tx) QueryContext(ctx context.Context, query string, args ...any) (*Rows, error) {
	rows, err := t.tx.QueryContext(ctx, rebind(t.dialect, query), args...)
	if err != nil {
		return nil, err
	}
	return &Rows{Rows: rows}, nil
}

// QueryRowContext runs a query expected to return at most one row.
func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *Row {
	return &Row{row: t.tx.QueryRowContext(ctx, rebind(t.dialect, query), args...)}
}

// PrepareContext prepares a statement for repeated execution. The returned
// statement is meant for Exec; queries through it bypass timestamp
// normalisation.
func (t *Tx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.tx.PrepareContext(ctx, rebind(t.dialect, query))
}

// Commit commits the transaction.
func (t *Tx) Commit() error {
	if t.joined {
		return nil
	}
	return t.tx.Commit()
}

// Rollback aborts the transaction.
func (t *Tx) Rollback() error {
	if t.joined {
		return nil
	}
	return t.tx.Rollback()
}

// Rows wraps *sql.Rows so Scan normalises timestamps.
type Rows struct {
	*sql.Rows
}

// Scan copies the current row into dest; see DB for timestamp handling.
func (r *Rows) Scan(dest ...any) error {
	return r.Rows.Scan(wrapDest(dest)...)
}

// Row wraps *sql.Row so Scan normalises timestamps.
type Row struct {
	row *sql.Row
}

// Scan copies the row into dest; see DB for timestamp handling.
func (r *Row) Scan(dest ...any) error {
	return r.row.Scan(wrapDest(dest)...)
}

// Err reports a deferred query error.
func (r *Row) Err() error { return r.row.Err() }

// Now returns the current time in the canonical stored format.
func Now() string {
	return FormatTimestamp(time.Now())
}

// FormatTimestamp renders t as the canonical stored timestamp: UTC, RFC 3339,
// with fractional seconds only when present. Both engines round-trip this
// exact string (Postgres keeps microseconds).
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// wrapDest replaces string destinations with timestamp-aware scanners. Other
// destinations are passed through untouched.
func wrapDest(dest []any) []any {
	out := make([]any, len(dest))
	for i, d := range dest {
		switch p := d.(type) {
		case *string:
			out[i] = &textDest{dst: p}
		case *sql.NullString:
			out[i] = &nullTextDest{dst: p}
		default:
			out[i] = d
		}
	}
	return out
}

// textDest scans into a *string, formatting time.Time canonically.
type textDest struct{ dst *string }

func (t *textDest) Scan(src any) error {
	if ts, ok := src.(time.Time); ok {
		*t.dst = FormatTimestamp(ts)
		return nil
	}
	var ns sql.NullString
	if err := ns.Scan(src); err != nil {
		return err
	}
	if !ns.Valid {
		return fmt.Errorf("converting NULL to string is unsupported")
	}
	*t.dst = ns.String
	return nil
}

// nullTextDest scans into a *sql.NullString, formatting time.Time canonically.
type nullTextDest struct{ dst *sql.NullString }

func (n *nullTextDest) Scan(src any) error {
	if ts, ok := src.(time.Time); ok {
		*n.dst = sql.NullString{String: FormatTimestamp(ts), Valid: true}
		return nil
	}
	return n.dst.Scan(src)
}

// rebind rewrites `?` placeholders to `$1…$n` for Postgres. Placeholders inside
// single-quoted strings, double-quoted identifiers and `--` line comments are
// not touched. SQLite queries are returned unchanged.
func rebind(dialect Dialect, query string) string {
	if dialect != Postgres || !strings.Contains(query, "?") {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 16)
	n := 0
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch {
		case c == '\'' || c == '"':
			// Copy through the closing quote. A doubled quote ('' or "")
			// closes and reopens, which this loop handles naturally.
			j := strings.IndexByte(query[i+1:], c)
			if j < 0 {
				b.WriteString(query[i:])
				return b.String()
			}
			b.WriteString(query[i : i+j+2])
			i += j + 1
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			j := strings.IndexByte(query[i:], '\n')
			if j < 0 {
				b.WriteString(query[i:])
				return b.String()
			}
			b.WriteString(query[i : i+j+1])
			i += j
		case c == '?':
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
