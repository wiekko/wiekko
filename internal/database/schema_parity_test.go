// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package database_test

import (
	"database/sql"
	"os"
	"sort"
	"strings"
	"testing"

	"wiekko.net/internal/database/dbtest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

// column is what must match between the two migration sets. Types are allowed
// to differ (UUID vs TEXT, TIMESTAMPTZ vs TEXT, BOOLEAN vs INTEGER); names and
// nullability are not.
type column struct {
	table, name string
	notNull     bool
}

func (c column) String() string {
	s := c.table + "." + c.name
	if c.notNull {
		s += " NOT NULL"
	}
	return s
}

// TestSchemaParity migrates both engines and compares their tables and
// columns, so the hand-maintained SQLite and PostgreSQL migrations cannot
// drift apart. The schema_migrations table is compared like any other.
func TestSchemaParity(t *testing.T) {
	sqliteCols := sqliteColumns(t, dbtest.Open(t, dbtest.SQLite))
	pgCols := postgresColumns(t, dbtest.Open(t, dbtest.Postgres))

	if len(sqliteCols) == 0 {
		t.Fatal("no columns found in the SQLite schema")
	}

	inSQLite := make(map[string]bool, len(sqliteCols))
	for _, c := range sqliteCols {
		inSQLite[c.String()] = true
	}
	inPG := make(map[string]bool, len(pgCols))
	for _, c := range pgCols {
		inPG[c.String()] = true
	}
	for _, c := range sqliteCols {
		if !inPG[c.String()] {
			t.Errorf("only in SQLite: %s", c)
		}
	}
	for _, c := range pgCols {
		if !inSQLite[c.String()] {
			t.Errorf("only in PostgreSQL: %s", c)
		}
	}
}

func sqliteColumns(t *testing.T, db *sql.DB) []column {
	t.Helper()
	rows, err := db.Query(`
		SELECT m.name, p.name, p."notnull", p.pk
		FROM sqlite_master m JOIN pragma_table_info(m.name) p
		WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("sqlite columns: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var cols []column
	for rows.Next() {
		var c column
		var notNull, pk int
		if err := rows.Scan(&c.table, &c.name, &notNull, &pk); err != nil {
			t.Fatal(err)
		}
		// SQLite lets a non-INTEGER primary key hold NULL unless declared
		// otherwise; Postgres never does. Treat both as NOT NULL.
		c.notNull = notNull != 0 || pk != 0
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return sorted(cols)
}

func postgresColumns(t *testing.T, db *sql.DB) []column {
	t.Helper()
	rows, err := db.Query(`
		SELECT table_name, column_name, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'`)
	if err != nil {
		t.Fatalf("postgres columns: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var cols []column
	for rows.Next() {
		var c column
		var nullable string
		if err := rows.Scan(&c.table, &c.name, &nullable); err != nil {
			t.Fatal(err)
		}
		c.notNull = strings.EqualFold(nullable, "NO")
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return sorted(cols)
}

func sorted(cols []column) []column {
	sort.Slice(cols, func(i, j int) bool { return cols[i].String() < cols[j].String() })
	return cols
}
