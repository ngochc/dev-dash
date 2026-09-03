package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesPrivateDatabaseFiles(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(parent, "devdash.db")

	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	assertPrivatePermissions(t, parent)
	assertPrivatePermissions(t, path)
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := path + suffix
		if _, err := os.Stat(sidecar); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			t.Fatalf("stat SQLite sidecar: %v", err)
		}
		assertPrivatePermissions(t, sidecar)
	}
}

func TestOpenPreservesExistingParentAndTightensDatabase(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatalf("Chmod(parent) error = %v", err)
	}
	path := filepath.Join(parent, "devdash.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod(database) error = %v", err)
	}

	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	parentInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("stat parent directory: %v", err)
	}
	if got := parentInfo.Mode().Perm(); got != 0o755 {
		t.Errorf("parent permissions = %04o, want 0755", got)
	}

	databaseInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if got := databaseInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("database permissions = %04o, want 0600", got)
	}
}

func assertPrivatePermissions(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat private path: %v", err)
	}
	if got := info.Mode().Perm(); got&0o077 != 0 {
		t.Errorf("group/other permissions = %04o, want none", got&0o077)
	}
}
