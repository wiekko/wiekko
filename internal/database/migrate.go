// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package database

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"regexp"
	"slices"
	"strconv"

	"nexmap.net/internal/database/migrations"
)

// Migrations are plain SQL files named NNNNN_name.sql in
// internal/database/migrations/<driver>/. Each file is one forward migration;
// there are no down migrations. A file must never change once released: a
// schema change is always a new file, in both driver directories.
//
// Migrate applies every pending migration in version order, each in its own
// transaction together with its row in schema_migrations, so a failing
// migration leaves the database at the previous version. Concurrent starts are
// serialised: PostgreSQL takes a transaction-scoped advisory lock, SQLite an
// immediate (write) transaction, and each migration is re-checked under that
// lock before it runs.
//
// Migrate refuses to run when the database has migrations this binary does not
// know (it was migrated by a newer nexmapd) or when a known migration is
// missing below newer applied ones.
//
// Because migrations run inside a transaction, statements PostgreSQL forbids
// in one (such as CREATE INDEX CONCURRENTLY) cannot be used.

// migrationTable records the applied migrations.
const migrationTable = "schema_migrations"

// gooseTable is the bookkeeping table of the goose library that ran the
// migrations before; Migrate converts it once and drops it.
const gooseTable = "goose_db_version"

// pgLockKey is the advisory lock key serialising migrations on PostgreSQL
// ("nexmap" in ASCII).
const pgLockKey = 0x6e65786d6170

type migration struct {
	version int64
	name    string
	sql     string
}

// dialect holds the driver-specific SQL; everything else is shared.
type dialect struct {
	begin       string // starts a write transaction
	lock        string // run first in every transaction; "" for none
	createTable string
	insert      string // version, name
	isApplied   string // version -> count
	hasGoose    string // -> count
}

var dialects = map[string]dialect{
	"sqlite": {
		// IMMEDIATE takes the write lock up front, so two processes cannot
		// both see a migration as pending.
		begin: "BEGIN IMMEDIATE",
		createTable: `CREATE TABLE IF NOT EXISTS ` + migrationTable + ` (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		)`,
		insert:    `INSERT INTO ` + migrationTable + ` (version, name) VALUES (?, ?)`,
		isApplied: `SELECT COUNT(*) FROM ` + migrationTable + ` WHERE version = ?`,
		hasGoose:  `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = '` + gooseTable + `'`,
	},
	"postgres": {
		begin: "BEGIN",
		lock:  "SELECT pg_advisory_xact_lock(" + strconv.FormatInt(pgLockKey, 10) + ")",
		createTable: `CREATE TABLE IF NOT EXISTS ` + migrationTable + ` (
			version    BIGINT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		insert:    `INSERT INTO ` + migrationTable + ` (version, name) VALUES ($1, $2)`,
		isApplied: `SELECT COUNT(*) FROM ` + migrationTable + ` WHERE version = $1`,
		hasGoose:  `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = '` + gooseTable + `'`,
	},
}

// Migrate applies all pending migrations for driver ("sqlite" or "postgres")
// and logs each one it applies to log (nil discards).
func Migrate(db *sql.DB, driver string, log *slog.Logger) error {
	fsys, err := migrationFS(driver)
	if err != nil {
		return err
	}
	return migrateFS(db, driver, fsys, log)
}

func migrateFS(db *sql.DB, driver string, fsys fs.FS, log *slog.Logger) error {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	d, ok := dialects[driver]
	if !ok {
		return fmt.Errorf("unsupported driver for migrations: %q", driver)
	}
	known, err := loadMigrations(fsys)
	if err != nil {
		return err
	}

	ctx := context.Background()
	// One connection for the whole run: transactions are started with plain
	// SQL (database/sql has no BEGIN IMMEDIATE), which must stay on the
	// connection that issued them. SQLite's pool has a single connection, so
	// nothing below may use db directly.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := inTx(ctx, conn, d, func() error { return bootstrap(ctx, conn, d, known, log) }); err != nil {
		return fmt.Errorf("migrate: prepare %s: %w", migrationTable, err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}
	pending, err := plan(known, applied)
	if err != nil {
		return err
	}

	for _, m := range pending {
		ran := false
		err := inTx(ctx, conn, d, func() error {
			// Another process may have applied it since we looked.
			var n int
			if err := conn.QueryRowContext(ctx, d.isApplied, m.version).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return nil
			}
			if _, err := conn.ExecContext(ctx, m.sql); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, d.insert, m.version, m.name); err != nil {
				return err
			}
			ran = true
			return nil
		})
		if err != nil {
			return fmt.Errorf("migrate: %05d_%s: %w", m.version, m.name, err)
		}
		if ran {
			log.Info("applied database migration", "version", m.version, "name", m.name)
		}
	}
	return nil
}

// inTx runs fn in a write transaction on conn, holding the dialect's lock.
func inTx(ctx context.Context, conn *sql.Conn, d dialect, fn func() error) (err error) {
	if _, err := conn.ExecContext(ctx, d.begin); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	if d.lock != "" {
		if _, err := conn.ExecContext(ctx, d.lock); err != nil {
			return err
		}
	}
	if err := fn(); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

// bootstrap creates the migration table and, on a database migrated by goose,
// carries goose's applied versions over and drops its table.
func bootstrap(ctx context.Context, conn *sql.Conn, d dialect, known []migration, log *slog.Logger) error {
	if _, err := conn.ExecContext(ctx, d.createTable); err != nil {
		return err
	}
	var n int
	if err := conn.QueryRowContext(ctx, d.hasGoose).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}

	// goose appends a row per up or down step; a version is applied when its
	// latest row says so. Version 0 is goose's own initial row.
	rows, err := conn.QueryContext(ctx, `SELECT version_id, is_applied FROM `+gooseTable+` ORDER BY id`)
	if err != nil {
		return err
	}
	state := map[int64]bool{}
	for rows.Next() {
		var v int64
		var ok bool
		if err := rows.Scan(&v, &ok); err != nil {
			_ = rows.Close()
			return err
		}
		if v > 0 {
			state[v] = ok
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}

	names := map[int64]string{}
	for _, m := range known {
		names[m.version] = m.name
	}
	var adopted []int64
	for v, ok := range state {
		if !ok {
			continue
		}
		var exists int
		if err := conn.QueryRowContext(ctx, d.isApplied, v).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		name := names[v]
		if name == "" {
			name = "unknown"
		}
		if _, err := conn.ExecContext(ctx, d.insert, v, name); err != nil {
			return err
		}
		adopted = append(adopted, v)
	}
	if _, err := conn.ExecContext(ctx, `DROP TABLE `+gooseTable); err != nil {
		return err
	}
	slices.Sort(adopted)
	log.Info("converted goose migration history", "table", gooseTable, "versions", adopted)
	return nil
}

func appliedVersions(ctx context.Context, conn *sql.Conn) ([]int64, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM `+migrationTable+` ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("migrate: read %s: %w", migrationTable, err)
	}
	defer func() { _ = rows.Close() }()
	var vs []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		vs = append(vs, v)
	}
	return vs, rows.Err()
}

// plan returns the migrations to apply, or an error when the database does
// not fit this binary's migrations. known and applied are sorted ascending.
func plan(known []migration, applied []int64) ([]migration, error) {
	isKnown := map[int64]bool{}
	for _, m := range known {
		isKnown[m.version] = true
	}
	var latest int64
	for _, v := range applied {
		if !isKnown[v] {
			return nil, fmt.Errorf("migrate: the database has migration %d, which this nexmapd does not know; it was migrated by a newer version", v)
		}
		latest = v
	}
	isApplied := map[int64]bool{}
	for _, v := range applied {
		isApplied[v] = true
	}
	var pending []migration
	for _, m := range known {
		if isApplied[m.version] {
			continue
		}
		if m.version < latest {
			return nil, fmt.Errorf("migrate: migration %05d_%s is not applied, but the later migration %d is", m.version, m.name, latest)
		}
		pending = append(pending, m)
	}
	return pending, nil
}

var migrationName = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.sql$`)

// loadMigrations reads the migrations in fsys's root, sorted by version.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: read migrations: %w", err)
	}
	var ms []migration
	seen := map[int64]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		parts := migrationName.FindStringSubmatch(e.Name())
		if parts == nil {
			return nil, fmt.Errorf("migrate: %s: not a migration file name (want NNNNN_name.sql)", e.Name())
		}
		v, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("migrate: %s: invalid version", e.Name())
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("migrate: %s and %s have the same version", prev, e.Name())
		}
		seen[v] = e.Name()
		body, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
		ms = append(ms, migration{version: v, name: parts[2], sql: string(body)})
	}
	if len(ms) == 0 {
		return nil, errors.New("migrate: no migrations found")
	}
	slices.SortFunc(ms, func(a, b migration) int { return cmp.Compare(a.version, b.version) })
	return ms, nil
}

func migrationFS(driver string) (fs.FS, error) {
	switch driver {
	case "sqlite":
		return fs.Sub(migrations.SQLiteFS, "sqlite")
	case "postgres":
		return fs.Sub(migrations.PostgresFS, "postgres")
	default:
		return nil, fmt.Errorf("unsupported driver for migrations: %q", driver)
	}
}
