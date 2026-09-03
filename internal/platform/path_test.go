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

func TestResolveWorkspaceDirectoryCreatesDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := ResolveWorkspaceDirectory("devdash", "")
	if err != nil {
		t.Fatalf("ResolveWorkspaceDirectory() error = %v", err)
	}
	want := filepath.Join(home, "devdash", "devdash")
	if got != want {
		t.Errorf("ResolveWorkspaceDirectory() = %q, want %q", got, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat created directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("created path %q is not a directory", want)
	}
}

func TestResolveWorkspaceDirectoryStoresAbsoluteDefault(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatalf("create home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Chdir(parent)

	got, err := ResolveWorkspaceDirectory("workspace", "")
	if err != nil {
		t.Fatalf("ResolveWorkspaceDirectory() error = %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("ResolveWorkspaceDirectory() = %q, want absolute path", got)
	}
}

func TestResolveWorkspaceDirectoryRejectsInvalidDefaultNames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"", ".", "..", "nested/workspace", `nested\\workspace`} {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveWorkspaceDirectory(name, ""); err == nil {
				t.Fatalf("ResolveWorkspaceDirectory(%q, %q) error = nil, want invalid name error", name, "")
			}
		})
	}
}

func TestResolveWorkspaceDirectoryReportsHomeLookupFailure(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := ResolveWorkspaceDirectory("workspace", ""); err == nil {
		t.Fatal("ResolveWorkspaceDirectory() error = nil, want home lookup error")
	}
}

func TestResolveWorkspaceDirectoryRetainsExplicitBehavior(t *testing.T) {
	directory := t.TempDir()
	got, err := ResolveWorkspaceDirectory("invalid/name", directory)
	if err != nil {
		t.Fatalf("ResolveWorkspaceDirectory() error = %v", err)
	}
	if got != directory {
		t.Errorf("ResolveWorkspaceDirectory() = %q, want %q", got, directory)
	}
}
