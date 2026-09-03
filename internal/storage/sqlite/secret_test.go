package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ngochc/dev-dash/internal/secret"
)

func TestSecretRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	repository := NewSecretRepository(db)
	const (
		key              = "github.pat"
		initialValue     = "initial-test-value"
		replacementValue = "replacement-test-value"
	)
	if err := repository.Set(ctx, key, initialValue); err != nil {
		t.Fatalf("Set(initial) error = %v", err)
	}

	item, err := repository.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if item.Key != key {
		t.Errorf("Get() key = %q, want %q", item.Key, key)
	}
	if item.Value != initialValue {
		t.Error("Get() returned an unexpected value")
	}
	if item.Description != "" {
		t.Errorf("Get() description = %q, want empty", item.Description)
	}

	fixedTimestamp := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := db.ExecContext(ctx, `
		UPDATE secrets
		SET created_at = ?, updated_at = ?
		WHERE key = ?
	`, fixedTimestamp, fixedTimestamp, key); err != nil {
		t.Fatalf("set deterministic timestamps: %v", err)
	}
	if err := repository.Set(ctx, key, replacementValue); err != nil {
		t.Fatalf("Set(replacement) error = %v", err)
	}

	item, err = repository.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() after replacement error = %v", err)
	}
	if item.Value != replacementValue {
		t.Error("Get() did not return the replacement value")
	}
	if !item.CreatedAt.Equal(fixedTimestamp) {
		t.Errorf("created_at changed: got %v, want %v", item.CreatedAt, fixedTimestamp)
	}
	if !item.UpdatedAt.After(fixedTimestamp) {
		t.Errorf("updated_at = %v, want later than %v", item.UpdatedAt, fixedTimestamp)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM secrets WHERE key = ?", key).Scan(&count); err != nil {
		t.Fatalf("count upserted rows: %v", err)
	}
	if count != 1 {
		t.Errorf("upserted row count = %d, want 1", count)
	}

	if err := repository.Set(ctx, "alpha", "alpha-test-value"); err != nil {
		t.Fatalf("Set(alpha) error = %v", err)
	}
	items, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("List() returned %d secrets, want 2", len(items))
	}
	if items[0].Key != "alpha" || items[1].Key != key {
		t.Errorf("List() keys = %q, %q; want alpha, %s", items[0].Key, items[1].Key, key)
	}
	for _, listed := range items {
		if listed.Value != "" {
			t.Error("List() unexpectedly returned a secret value")
		}
	}

	if err := repository.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := repository.Get(ctx, key); !errors.Is(err, secret.ErrNotFound) {
		t.Errorf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestSecretRepositoryMissing(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "devdash.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	repository := NewSecretRepository(db)
	if _, err := repository.Get(ctx, "missing"); !errors.Is(err, secret.ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
	if err := repository.Delete(ctx, "missing"); !errors.Is(err, secret.ErrNotFound) {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}
