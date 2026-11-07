// Package dbtest gives integration tests a migrated database on every
// supported engine: SQLite in a temp file, and PostgreSQL via an embedded
// server started once per test binary.
//
// A package opts in with a TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }
//
// and runs each test against both engines with Each. The Postgres server is
// downloaded on first use (cached in ~/.embedded-postgres-go) and runs by
// default; it is skipped with -short or NEXMAP_TEST_POSTGRES=off.
package dbtest

import (
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/pressly/goose/v3"

	"nexmap.net/internal/config"
	"nexmap.net/internal/database"
)

// Engine driver names, as accepted by database.Open and database.Migrate.
const (
	SQLite   = "sqlite"
	Postgres = "postgres"
)

// EnvPostgres disables the Postgres runs when set to "off", "0" or "false".
const EnvPostgres = "NEXMAP_TEST_POSTGRES"

const templateDB = "nexmap_template"

var (
	pgBaseURL  string // server URL without database, "" when Postgres is off
	pgSkipWhy  string // why Postgres runs are skipped
	dbCounter  atomic.Int64
	mainCalled bool
)

// Main starts the shared Postgres server (unless disabled), runs the tests
// and stops the server again. It returns the exit code for os.Exit.
func Main(m *testing.M) int {
	flag.Parse()
	mainCalled = true
	// Every test migrates a database; goose's per-file log lines drown the
	// test output.
	goose.SetLogger(goose.NopLogger())

	switch v := strings.ToLower(os.Getenv(EnvPostgres)); {
	case testing.Short():
		pgSkipWhy = "PostgreSQL tests skipped in -short mode"
	case v == "off" || v == "0" || v == "false":
		pgSkipWhy = "PostgreSQL tests disabled by " + EnvPostgres + "=" + v
	}
	if pgSkipWhy != "" {
		return m.Run()
	}

	stop, err := startPostgres()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbtest: %v\n(set %s=off or use -short to skip the PostgreSQL runs)\n", err, EnvPostgres)
		return 1
	}
	code := m.Run()
	if err := stop(); err != nil {
		fmt.Fprintf(os.Stderr, "dbtest: stop postgres: %v\n", err)
	}
	return code
}

func startPostgres() (func() error, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	base, err := os.MkdirTemp("", "nexmap-pg-")
	if err != nil {
		return nil, err
	}
	runtime := filepath.Join(base, "runtime")

	var log bytes.Buffer
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V18).
		Port(port).
		RuntimePath(runtime).
		// Throwaway data: durability only costs time.
		StartParameters(map[string]string{
			"fsync":              "off",
			"synchronous_commit": "off",
			"full_page_writes":   "off",
		}).
		Logger(&log))
	// The library writes its log file to the system temp dir and never removes
	// it (and wipes runtime on start); point the temp dir at base for the start
	// so our cleanup takes the log file too.
	restore := setTempDir(base)
	err = pg.Start()
	restore()
	if err != nil {
		_ = os.RemoveAll(base)
		return nil, fmt.Errorf("start embedded postgres: %w\n%s", err, log.String())
	}
	stop := func() error {
		err := pg.Stop()
		_ = os.RemoveAll(base)
		return err
	}

	pgBaseURL = fmt.Sprintf("postgres://postgres:postgres@localhost:%d", port)
	if err := createTemplate(); err != nil {
		_ = stop()
		return nil, err
	}
	return stop, nil
}

// createTemplate migrates a template database once; every test then gets a
// cheap copy of it.
func createTemplate() error {
	admin, err := openPostgres("postgres")
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
	if err := database.Migrate(tmpl, Postgres); err != nil {
		return fmt.Errorf("migrate template: %w", err)
	}
	return nil
}

func openPostgres(name string) (*sql.DB, error) {
	return database.Open(&config.Config{
		DBDriver: Postgres,
		DBDsn:    pgBaseURL + "/" + name + "?sslmode=disable",
	})
}

func setTempDir(dir string) (restore func()) {
	vars := []string{"TMPDIR", "TMP", "TEMP"}
	old := make(map[string]*string, len(vars))
	for _, k := range vars {
		if v, ok := os.LookupEnv(k); ok {
			old[k] = &v
		} else {
			old[k] = nil
		}
		_ = os.Setenv(k, dir)
	}
	return func() {
		for k, v := range old {
			if v == nil {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, *v)
			}
		}
	}
}

func freePort() (uint32, error) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, fmt.Errorf("find free port: %w", err)
	}
	defer func() { _ = l.Close() }()
	return uint32(l.Addr().(*net.TCPAddr).Port), nil
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
		return openSQLite(t)
	case Postgres:
		return openPostgresTest(t)
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

func openSQLite(t testing.TB) *sql.DB {
	t.Helper()
	db, err := database.Open(&config.Config{
		DBDriver: SQLite,
		DBDsn:    filepath.Join(t.TempDir(), "test.db"),
	})
	if err != nil {
		t.Fatalf("dbtest: open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(db, SQLite); err != nil {
		t.Fatalf("dbtest: migrate sqlite: %v", err)
	}
	return db
}

func openPostgresTest(t testing.TB) *sql.DB {
	t.Helper()
	if !mainCalled {
		t.Fatalf("dbtest: PostgreSQL requested but the package has no TestMain calling dbtest.Main")
	}
	if pgBaseURL == "" {
		t.Skip(pgSkipWhy)
	}

	name := fmt.Sprintf("test_%d_%d", os.Getpid(), dbCounter.Add(1))
	admin, err := openPostgres("postgres")
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if _, err := admin.Exec(`CREATE DATABASE ` + name + ` TEMPLATE ` + templateDB); err != nil {
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
