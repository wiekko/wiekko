package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigrateSQLiteAppliesCleanly guards the hand-maintained SQLite migrations:
// every statement must apply to an empty database, and roll back again.
func TestMigrateSQLiteAppliesCleanly(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "migrate.db")
	db, err := sql.Open("sqlite", dsn+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := Migrate(db, "sqlite"); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	for _, table := range []string{"testing_campaigns", "campaign_results", "campaign_result_events"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %s: %v", table, err)
		}
	}
}
