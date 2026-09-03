package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devdash.db")

	db, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow(`
		SELECT MAX(version_id)
		FROM goose_db_version
		WHERE is_applied = 1
	`).Scan(&version); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if version != 2 {
		t.Fatalf("migration version = %d, want 2", version)
	}

	var resourceTypes, relationTypes int
	if err := db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM resource_types WHERE name = 'repository'),
			(SELECT COUNT(*) FROM relation_types WHERE name = 'related_to')
	`).Scan(&resourceTypes, &relationTypes); err != nil {
		t.Fatalf("query seeded metadata: %v", err)
	}
	if resourceTypes != 1 {
		t.Errorf("repository resource type count = %d, want 1", resourceTypes)
	}
	if relationTypes != 1 {
		t.Errorf("related_to relation type count = %d, want 1", relationTypes)
	}
}
