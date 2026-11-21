// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package database

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func openSQLite(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	// SQLite doesn't benefit from multiple writer connections; a single
	// connection avoids SQLITE_BUSY under concurrent writes with WAL mode.
	db.SetMaxOpenConns(1)

	// Enable WAL for concurrent reads during writes, enforce foreign keys,
	// and set a busy timeout to wait on lock contention.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			errInner := db.Close()
			if errInner != nil {
				return nil, errInner
			}
			return nil, fmt.Errorf("sqlite pragma %q: %w", p, err)
		}
	}

	return db, nil
}
