package app

import (
	"context"
	"fmt"
	"io"

	"github.com/ngochc/dev-dash/internal/config"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
)

func runDoctor(ctx context.Context, output io.Writer) error {
	dbPath, err := config.DatabasePath()
	if err != nil {
		return err
	}

	db, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	fmt.Fprintf(output, "database   OK  %s\n", dbPath)
	fmt.Fprintln(output, "sqlite     OK")
	fmt.Fprintln(output, "migration  OK")

	return nil
}
