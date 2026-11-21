// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Package dbtest gives integration tests a migrated database on every
// supported engine: SQLite in a temp file, and PostgreSQL on a server named by
// NEXMAP_TEST_POSTGRES_DSN.
//
// A package opts in with a TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }
//
// and runs each test against both engines with Each.
//
// The PostgreSQL runs exist to catch dialect differences that SQLite hides, but
// not every machine has a server, so they need one to be named:
//
//	NEXMAP_TEST_POSTGRES_DSN=postgres://user:pass@host:port/postgres
//
// `just test-pg` starts a throwaway server in a container and sets it. The
// DSN's database is only used to create and drop the test databases, so the
// role needs CREATEDB. Without a DSN (or with -short, or with
// NEXMAP_TEST_POSTGRES=off) the PostgreSQL subtests are skipped and say so;
// NEXMAP_TEST_POSTGRES=required turns a missing DSN into a failure instead,
// for runs that must cover PostgreSQL.
package dbtest

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"nexmap.net/internal/config"
	"nexmap.net/internal/database"
)

// Engine driver names, as accepted by database.Open and database.Migrate.
const (
	SQLite   = "sqlite"
	Postgres = "postgres"
)

const (
	// EnvPostgresDSN names the PostgreSQL server the tests use.
	EnvPostgresDSN = "NEXMAP_TEST_POSTGRES_DSN"
	// EnvPostgres is "off" (or "0", "false") to skip the PostgreSQL runs even
	// with a DSN, or "required" to fail when there is no DSN.
	EnvPostgres = "NEXMAP_TEST_POSTGRES"
)

var (
	pgServer   *url.URL // the DSN, nil when PostgreSQL runs are off
	pgAdminDB  string   // the DSN's database
	pgSkipWhy  string   // why PostgreSQL runs are skipped
	dbPrefix   string   // unique per test binary: binaries share the server
	templateDB string
	dbCounter  atomic.Int64
	mainCalled bool
)

// Main sets up the PostgreSQL template database (unless PostgreSQL runs are
// off), runs the tests and drops it again. It returns the exit code for
// os.Exit.
func Main(m *testing.M) int {
	flag.Parse()
	mainCalled = true

	dsn := os.Getenv(EnvPostgresDSN)
	switch mode := strings.ToLower(os.Getenv(EnvPostgres)); {
	case testing.Short():
		pgSkipWhy = "PostgreSQL tests skipped in -short mode"
	case mode == "off" || mode == "0" || mode == "false":
		pgSkipWhy = "PostgreSQL tests disabled by " + EnvPostgres + "=" + mode
	case dsn == "" && mode == "required":
		fmt.Fprintf(os.Stderr, "dbtest: %s=required, but %s is not set (`just test-pg` provides a server)\n", EnvPostgres, EnvPostgresDSN)
		return 1
	case dsn == "":
		pgSkipWhy = "PostgreSQL tests skipped: " + EnvPostgresDSN + " is not set (run `just test-pg`)"
	}
	if pgSkipWhy != "" {
		return m.Run()
	}

	if err := setupPostgres(dsn); err != nil {
		fmt.Fprintf(os.Stderr, "dbtest: %v\n(unset %s or set %s=off to skip the PostgreSQL runs)\n", err, EnvPostgresDSN, EnvPostgres)
		return 1
	}
	code := m.Run()
	if err := dropDatabase(templateDB); err != nil {
		fmt.Fprintf(os.Stderr, "dbtest: drop template: %v\n", err)
	}
	return code
}

func setupPostgres(dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return fmt.Errorf("%s must be a postgres:// URL", EnvPostgresDSN)
	}
	pgServer = u
	pgAdminDB = strings.TrimPrefix(u.Path, "/")
	if pgAdminDB == "" {
		pgAdminDB = "postgres"
	}

	var b [4]byte
	_, _ = rand.Read(b[:])
	dbPrefix = fmt.Sprintf("nexmap_test_%d_%s", os.Getpid(), hex.EncodeToString(b[:]))
	templateDB = dbPrefix + "_template"
	if err := createTemplate(); err != nil {
		_ = dropDatabase(templateDB)
		return err
	}
	return nil
}

// createTemplate migrates a template database once; every test then gets a
// cheap copy of it.
func createTemplate() error {
	admin, err := openPostgres(pgAdminDB)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.Exec(`CREATE DATABASE ` + templateDB); err != nil {
		return fmt.Errorf("create template: %w", err)
	}

	tmpl, err := openPostgres(templateDB)
	if err != nil {
		return err
	}
	defer func() { _ = tmpl.Close() }()
	if err := database.Migrate(tmpl, Postgres, nil); err != nil {
		return fmt.Errorf("migrate template: %w", err)
	}
	return nil
}

func dropDatabase(name string) error {
	admin, err := openPostgres(pgAdminDB)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close() }()
	_, err = admin.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`)
	return err
}

// openPostgres connects to database name on the test server.
func openPostgres(name string) (*sql.DB, error) {
	u := *pgServer
	u.Path = "/" + name
	return database.Open(&config.Config{DBDriver: Postgres, DBDsn: u.String()})
}

// Engines lists the engines every integration test runs against.
func Engines() []string { return []string{SQLite, Postgres} }

// Open returns a freshly migrated, empty database for driver. It is closed
// (and, for Postgres, dropped) when the test ends. A Postgres request skips
// the test when Postgres runs are disabled.
func Open(t testing.TB, driver string) *sql.DB {
	t.Helper()
	switch driver {
	case SQLite:
		return openSQLite(t, true)
	case Postgres:
		return openPostgresTest(t, true)
	default:
		t.Fatalf("dbtest: unknown driver %q", driver)
		return nil
	}
}

// Each runs fn once per engine as a subtest named after the driver.
func Each(t *testing.T, fn func(t *testing.T, driver string, db *sql.DB)) {
	t.Helper()
	for _, driver := range Engines() {
		t.Run(driver, func(t *testing.T) {
			fn(t, driver, Open(t, driver))
		})
	}
}

func openSQLite(t testing.TB, migrate bool) *sql.DB {
	t.Helper()
	db, err := database.Open(&config.Config{
		DBDriver: SQLite,
		DBDsn:    filepath.Join(t.TempDir(), "test.db"),
	})
	if err != nil {
		t.Fatalf("dbtest: open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if !migrate {
		return db
	}
	if err := database.Migrate(db, SQLite, nil); err != nil {
		t.Fatalf("dbtest: migrate sqlite: %v", err)
	}
	return db
}

func openPostgresTest(t testing.TB, migrate bool) *sql.DB {
	t.Helper()
	if !mainCalled {
		t.Fatalf("dbtest: PostgreSQL requested but the package has no TestMain calling dbtest.Main")
	}
	if pgServer == nil {
		t.Skip(pgSkipWhy)
	}

	name := fmt.Sprintf("%s_%d", dbPrefix, dbCounter.Add(1))
	admin, err := openPostgres(pgAdminDB)
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	create := `CREATE DATABASE ` + name
	if migrate {
		create += ` TEMPLATE ` + templateDB
	}
	if _, err := admin.Exec(create); err != nil {
		t.Fatalf("dbtest: create database: %v", err)
	}

	db, err := openPostgres(name)
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`)
	})
	return db
}

// OpenEmpty is Open without the migrations: an empty database for tests of
// the migration runner itself.
func OpenEmpty(t testing.TB, driver string) *sql.DB {
	t.Helper()
	switch driver {
	case SQLite:
		return openSQLite(t, false)
	case Postgres:
		return openPostgresTest(t, false)
	default:
		t.Fatalf("dbtest: unknown driver %q", driver)
		return nil
	}
}
