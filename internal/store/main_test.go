package store

import (
	"database/sql"
	"os"
	"testing"

	"nexmap.net/internal/database/dbtest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

// forEachDB runs fn against a freshly migrated database on every supported
// engine. These tests exercise the real SQL rather than mocks, so schema,
// query and dialect mistakes surface here.
func forEachDB(t *testing.T, fn func(t *testing.T, db *DB)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		fn(t, NewDB(conn, Dialect(driver)))
	})
}
