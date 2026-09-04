package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPageMaterializerCreatesRealRootAndWritesAtomically(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "wiki")
	page := filepath.Join(root, "page.md")
	materializer := PageMaterializer{}
	if err := materializer.EnsureRoot(root); err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	if err := materializer.WriteAtomic(page, []byte("first")); err != nil {
		t.Fatalf("first WriteAtomic() error = %v", err)
	}
	if err := materializer.WriteAtomic(page, []byte("second")); err != nil {
		t.Fatalf("second WriteAtomic() error = %v", err)
	}
	content, err := os.ReadFile(page)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "second" {
		t.Fatalf("content = %q, want second", content)
	}
	info, err := os.Stat(page)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o, want 644", info.Mode().Perm())
	}
	inspection, err := materializer.Inspect(page)
	if err != nil || !inspection.Exists || !inspection.Regular {
		t.Fatalf("Inspect() = %#v, %v", inspection, err)
	}
}

func TestPageMaterializerRejectsUnsafeRootsAndDestinations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	rootLink := filepath.Join(workspace, "wiki")
	if err := os.Symlink(target, rootLink); err != nil {
		t.Fatal(err)
	}
	materializer := PageMaterializer{}
	if err := materializer.EnsureRoot(rootLink); err == nil {
		t.Fatal("EnsureRoot(symlink) error = nil")
	}

	realRoot := filepath.Join(workspace, "real-wiki")
	if err := materializer.EnsureRoot(realRoot); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(workspace, "target.md")
	if err := os.WriteFile(targetFile, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	pageLink := filepath.Join(realRoot, "page.md")
	if err := os.Symlink(targetFile, pageLink); err != nil {
		t.Fatal(err)
	}
	inspection, err := materializer.Inspect(pageLink)
	if err != nil || !inspection.Exists || inspection.Regular {
		t.Fatalf("Inspect(symlink) = %#v, %v", inspection, err)
	}
	if err := materializer.WriteAtomic(pageLink, []byte("changed")); err == nil {
		t.Fatal("WriteAtomic(symlink) error = nil")
	}
	content, err := os.ReadFile(targetFile)
	if err != nil || string(content) != "unchanged" {
		t.Fatalf("target content = %q, %v", content, err)
	}
}

func TestPageMaterializerRemoveIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.md")
	if err := os.WriteFile(path, []byte("page"), 0o644); err != nil {
		t.Fatal(err)
	}
	materializer := PageMaterializer{}
	if err := materializer.Remove(path); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := materializer.Remove(path); err != nil {
		t.Fatalf("second Remove() error = %v", err)
	}
}
