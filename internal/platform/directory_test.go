package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryManagerExists(t *testing.T) {
	manager := DirectoryManager{}
	directory := t.TempDir()
	file := filepath.Join(directory, "file")
	if err := os.WriteFile(file, []byte("content"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	for _, test := range []struct {
		name string
		path string
		want bool
	}{
		{name: "directory", path: directory, want: true},
		{name: "missing", path: filepath.Join(directory, "missing")},
		{name: "file", path: file},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := manager.Exists(test.path)
			if err != nil {
				t.Fatalf("Exists() error = %v", err)
			}
			if got != test.want {
				t.Errorf("Exists() = %v, want %v", got, test.want)
			}
		})
	}
}
