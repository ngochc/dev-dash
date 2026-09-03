package config

import (
	"path/filepath"
	"testing"
)

func TestDatabasePathFromEnvironment(t *testing.T) {
	t.Setenv(envDatabasePath, filepath.Join("relative", "..", "devdash.db"))

	got, err := DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if got != "devdash.db" {
		t.Errorf("DatabasePath() = %q, want %q", got, "devdash.db")
	}
}

func TestDatabasePathFromHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envDatabasePath, "")
	t.Setenv("HOME", home)

	got, err := DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	want := filepath.Join(home, ".devdash", "devdash.db")
	if got != want {
		t.Errorf("DatabasePath() = %q, want %q", got, want)
	}
}
