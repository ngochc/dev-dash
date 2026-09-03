package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/ngochc/dev-dash/internal/storage/migrations"
	"github.com/pressly/goose/v3"
)

func migrate(db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}
