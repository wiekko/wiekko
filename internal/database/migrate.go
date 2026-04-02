package database

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"nexmap.net/internal/database/migrations"
)

// Migrate applies all pending database migrations.
func Migrate(db *sql.DB, driver string) error {
	goose.SetBaseFS(migrations.FS)

	dialect, err := gooseDialect(driver)
	if err != nil {
		return err
	}

	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	return nil
}

func gooseDialect(driver string) (string, error) {
	switch driver {
	case "sqlite":
		return "sqlite3", nil
	case "postgres":
		return "postgres", nil
	default:
		return "", fmt.Errorf("unsupported driver for migrations: %q", driver)
	}
}
