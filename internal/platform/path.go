package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveDirectory returns an absolute, cleaned path to an existing directory.
// An empty path resolves to the current working directory.
func ResolveDirectory(path string) (string, error) {
	if path == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get current working directory: %w", err)
		}
		path = workingDirectory
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %q: %w", path, err)
	}
	absolutePath = filepath.Clean(absolutePath)

	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("stat workspace directory %q: %w", absolutePath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace path is not a directory: %q", absolutePath)
	}

	return absolutePath, nil
}
