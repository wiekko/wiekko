// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package database_test

import (
	"database/sql"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"wiekko.net/internal/database"
	"wiekko.net/internal/database/dbtest"
)

// eachEmpty runs fn per engine on an empty, unmigrated database.
func eachEmpty(t *testing.T, fn func(t *testing.T, driver string, db *sql.DB)) {
	t.Helper()
	for _, driver := range dbtest.Engines() {
		t.Run(driver, func(t *testing.T) {
			fn(t, driver, dbtest.OpenEmpty(t, driver))
		})
	}
}

func appliedVersions(t *testing.T, db *sql.DB) []int64 {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var vs []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		vs = append(vs, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return vs
}

// latestVersions are the versions of all embedded migrations.
var latestVersions = []int64{1, 2, 3, 4, 5, 6}

func wantVersions(t *testing.T, db *sql.DB, want ...int64) {
	t.Helper()
	got := appliedVersions(t, db)
	if len(got) != len(want) {
		t.Fatalf("applied versions = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("applied versions = %v, want %v", got, want)
		}
	}
}

func tableExists(db *sql.DB, table string) bool {
	_, err := db.Exec(`SELECT 1 FROM ` + table + ` WHERE 1 = 0`)
	return err == nil
}

func wantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("err = %v, want one mentioning %q", err, substr)
	}
}

func TestMigrateFreshAndIdempotent(t *testing.T) {
	eachEmpty(t, func(t *testing.T, driver string, db *sql.DB) {
		if err := database.Migrate(db, driver, nil); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		wantVersions(t, db, latestVersions...)

		var name string
		if err := db.QueryRow(`SELECT name FROM schema_migrations WHERE version = 3`).Scan(&name); err != nil || name != "products" {
			t.Fatalf("name of version 3 = %q, %v; want products", name, err)
		}
		// Migration 1 inserts a row with a fixed key: running it twice fails,
		// so a second Migrate must apply nothing.
		if err := database.Migrate(db, driver, nil); err != nil {
			t.Fatalf("second migrate: %v", err)
		}
		wantVersions(t, db, latestVersions...)
	})
}

func TestMigrateAppliesOnlyNewMigrations(t *testing.T) {
	eachEmpty(t, func(t *testing.T, driver string, db *sql.DB) {
		v1 := fstest.MapFS{"00001_a.sql": {Data: []byte(`CREATE TABLE a (x INTEGER);`)}}
		if err := database.MigrateFS(db, driver, v1, nil); err != nil {
			t.Fatalf("migrate v1: %v", err)
		}
		v2 := fstest.MapFS{
			"00001_a.sql": v1["00001_a.sql"],
			"00002_b.sql": {Data: []byte("CREATE TABLE b (x INTEGER);\nINSERT INTO a (x) VALUES (1);")},
		}
		if err := database.MigrateFS(db, driver, v2, nil); err != nil {
			t.Fatalf("migrate v2: %v", err)
		}
		wantVersions(t, db, 1, 2)
		if !tableExists(db, "b") {
			t.Fatal("table b missing")
		}
	})
}

func TestMigrateFailureRollsBack(t *testing.T) {
	eachEmpty(t, func(t *testing.T, driver string, db *sql.DB) {
		fsys := fstest.MapFS{
			"00001_a.sql": {Data: []byte(`CREATE TABLE a (x INTEGER);`)},
			"00002_b.sql": {Data: []byte("CREATE TABLE b (x INTEGER);\nINSERT INTO no_such_table (x) VALUES (1);")},
		}
		wantErr(t, database.MigrateFS(db, driver, fsys, nil), "00002_b")
		wantVersions(t, db, 1)
		if !tableExists(db, "a") {
			t.Fatal("table a from the successful migration is missing")
		}
		if tableExists(db, "b") {
			t.Fatal("table b from the failed migration was not rolled back")
		}
		// The connection is usable again (no transaction left open).
		fixed := fstest.MapFS{
			"00001_a.sql": fsys["00001_a.sql"],
			"00002_b.sql": {Data: []byte(`CREATE TABLE b (x INTEGER);`)},
		}
		if err := database.MigrateFS(db, driver, fixed, nil); err != nil {
			t.Fatalf("migrate after fix: %v", err)
		}
		wantVersions(t, db, 1, 2)
	})
}

func TestMigrateRefusesNewerDatabase(t *testing.T) {
	eachEmpty(t, func(t *testing.T, driver string, db *sql.DB) {
		if err := database.Migrate(db, driver, nil); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version, name) VALUES (99999, 'future')`); err != nil {
			t.Fatal(err)
		}
		wantErr(t, database.Migrate(db, driver, nil), "newer version")
	})
}

func TestMigrateRefusesGap(t *testing.T) {
	eachEmpty(t, func(t *testing.T, driver string, db *sql.DB) {
		fsys := fstest.MapFS{
			"00001_a.sql": {Data: []byte(`CREATE TABLE a (x INTEGER);`)},
			"00003_c.sql": {Data: []byte(`CREATE TABLE c (x INTEGER);`)},
		}
		if err := database.MigrateFS(db, driver, fsys, nil); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		// A migration slotted in below an applied one (e.g. from a merged
		// branch) must not run out of order.
		fsys["00002_b.sql"] = &fstest.MapFile{Data: []byte(`CREATE TABLE b (x INTEGER);`)}
		wantErr(t, database.MigrateFS(db, driver, fsys, nil), "00002_b is not applied")
	})
}

func TestMigrateRejectsBadFiles(t *testing.T) {
	db := dbtest.OpenEmpty(t, dbtest.SQLite)
	for name, fsys := range map[string]fstest.MapFS{
		"bad name":  {"init.sql": {Data: []byte(`SELECT 1;`)}},
		"version 0": {"00000_zero.sql": {Data: []byte(`SELECT 1;`)}},
		"duplicate": {"00001_a.sql": {Data: []byte(`SELECT 1;`)}, "1_b.sql": {Data: []byte(`SELECT 1;`)}},
		"empty":     {},
	} {
		if err := database.MigrateFS(db, dbtest.SQLite, fsys, nil); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestMigrateConcurrent(t *testing.T) {
	eachEmpty(t, func(t *testing.T, driver string, db *sql.DB) {
		var wg sync.WaitGroup
		errs := make(chan error, 4)
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- database.Migrate(db, driver, nil)
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Errorf("concurrent migrate: %v", err)
			}
		}
		wantVersions(t, db, latestVersions...)
	})
}

// gooseSchema is the bookkeeping table goose created, per engine.
var gooseSchema = map[string]string{
	dbtest.SQLite: `CREATE TABLE goose_db_version (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		is_applied INTEGER NOT NULL,
		tstamp TIMESTAMP DEFAULT (datetime('now')))`,
	dbtest.Postgres: `CREATE TABLE goose_db_version (
		id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,
		version_id bigint NOT NULL,
		is_applied boolean NOT NULL,
		tstamp timestamp NOT NULL DEFAULT now())`,
}

func TestMigrateAdoptsGooseHistory(t *testing.T) {
	eachEmpty(t, func(t *testing.T, driver string, db *sql.DB) {
		// A database goose migrated to version 3: the schema is there, the
		// history is in goose_db_version only. Goose never ran a later
		// migration, so the schema is that of version 3.
		upTo3, err := database.MigrationsUpTo(driver, 3)
		if err != nil {
			t.Fatal(err)
		}
		if err := database.MigrateFS(db, driver, upTo3, nil); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE schema_migrations`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(gooseSchema[driver]); err != nil {
			t.Fatal(err)
		}
		// Version 3 was rolled back once and re-applied; the latest row counts.
		for _, row := range []struct {
			v  int
			ok bool
		}{{0, true}, {1, true}, {2, true}, {3, true}, {3, false}, {3, true}} {
			if _, err := db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (` +
				strconv.Itoa(row.v) + `, ` + boolLit(driver, row.ok) + `)`); err != nil {
				t.Fatal(err)
			}
		}

		// Re-running migration 1 would fail on its fixed-key insert, so
		// success means the history was taken over, not replayed.
		if err := database.Migrate(db, driver, nil); err != nil {
			t.Fatalf("migrate goose database: %v", err)
		}
		wantVersions(t, db, latestVersions...)
		if tableExists(db, "goose_db_version") {
			t.Fatal("goose_db_version was not dropped")
		}
	})
}

func boolLit(driver string, b bool) string {
	switch {
	case driver == dbtest.Postgres && b:
		return "true"
	case driver == dbtest.Postgres:
		return "false"
	case b:
		return "1"
	default:
		return "0"
	}
}
