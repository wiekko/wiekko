package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"nexmap.net/internal/config"
)

// Open returns a *sql.DB for the configured driver and DSN.
// It validates the connection before returning.
func Open(cfg *config.Config) (*sql.DB, error) {
	var db *sql.DB
	var err error

	switch cfg.DBDriver {
	case "sqlite":
		db, err = openSQLite(cfg.DBDsn)
	case "postgres":
		db, err = openPostgres(cfg.DBDsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %q (expected \"sqlite\" or \"postgres\")", cfg.DBDriver)
	}
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		errInner := db.Close()
		if errInner != nil {
			return nil, errInner
		}
		return nil, fmt.Errorf("database ping: %w", err)
	}

	return db, nil
}
