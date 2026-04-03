package database

import (
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	"nexmap.net/internal/database/migrations"
)

// Migrate applies all pending database migrations.
func Migrate(db *sql.DB, driver string) error {
	baseFS, dir, err := migrationFS(driver)
	if err != nil {
		return err
	}

	goose.SetBaseFS(baseFS)

	dialect, err := gooseDialect(driver)
	if err != nil {
		return err
	}

	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}

	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	return nil
}

func migrationFS(driver string) (fs.FS, string, error) {
	switch driver {
	case "sqlite":
		return migrations.SQLiteFS, "sqlite", nil
	case "postgres":
		return migrations.PostgresFS, "postgres", nil
	default:
		return nil, "", fmt.Errorf("unsupported driver for migrations: %q", driver)
	}
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
