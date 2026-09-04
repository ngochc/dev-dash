package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/ngochc/dev-dash/internal/config"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
	"github.com/ngochc/dev-dash/internal/ui/progress"
)

func runDoctor(ctx context.Context, output, feedback io.Writer) error {
	dbPath, err := config.DatabasePath()
	if err != nil {
		return err
	}

	var db *sql.DB
	err = progress.Run(feedback, "Checking database and migrations", func() error {
		var openErr error
		db, openErr = sqlite.Open(ctx, dbPath)
		return openErr
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	fmt.Fprintf(output, "database   OK  %s\n", dbPath)
	fmt.Fprintln(output, "sqlite     OK")
	fmt.Fprintln(output, "migration  OK")

	return nil
}
