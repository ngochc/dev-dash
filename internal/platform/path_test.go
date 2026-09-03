package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDirectoryExplicitPath(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "workspace")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	t.Chdir(parent)

	got, err := ResolveDirectory(filepath.Join("workspace", "."))
	if err != nil {
		t.Fatalf("ResolveDirectory() error = %v", err)
	}
	if got != directory {
		t.Errorf("ResolveDirectory() = %q, want %q", got, directory)
	}
}

func TestResolveDirectoryCurrentDirectory(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)

	got, err := ResolveDirectory("")
	if err != nil {
		t.Fatalf("ResolveDirectory() error = %v", err)
	}
	if got != directory {
		t.Errorf("ResolveDirectory() = %q, want %q", got, directory)
	}
}

func TestResolveDirectoryRejectsMissingPath(t *testing.T) {
	_, err := ResolveDirectory(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("ResolveDirectory() error = nil, want missing path error")
	}
}

func TestResolveDirectoryRejectsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("content"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	_, err := ResolveDirectory(file)
	if err == nil {
		t.Fatal("ResolveDirectory() error = nil, want non-directory error")
	}
}
