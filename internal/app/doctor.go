package app

import (
	"context"
	"fmt"

	"github.com/ngochc/dev-dash/internal/config"
	"github.com/ngochc/dev-dash/internal/storage/sqlite"
)

func runDoctor(ctx context.Context) error {
	dbPath, err := config.DatabasePath()
	if err != nil {
		return err
	}

	db, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	fmt.Printf("database   OK  %s\n", dbPath)
	fmt.Println("sqlite     OK")
	fmt.Println("migration  OK")

	return nil
}
