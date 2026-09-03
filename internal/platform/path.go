package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultWorkspaceParent = "devdash"

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

// ResolveWorkspaceDirectory resolves an explicit workspace path or creates the
// default directory at ~/devdash/<name> when path is empty.
func ResolveWorkspaceDirectory(name, path string) (string, error) {
	if path != "" {
		return ResolveDirectory(path)
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || filepath.IsAbs(name) || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("workspace name must be a single path component: %q", name)
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	root, err := filepath.Abs(filepath.Join(homeDirectory, defaultWorkspaceParent))
	if err != nil {
		return "", fmt.Errorf("resolve default workspace root: %w", err)
	}
	root = filepath.Clean(root)
	target := filepath.Clean(filepath.Join(root, name))
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("validate default workspace path: %w", err)
	}
	if relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace name escapes default root: %q", name)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", fmt.Errorf("create workspace directory %q: %w", target, err)
	}
	return target, nil
}
