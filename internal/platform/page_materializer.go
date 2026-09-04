package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ngochc/dev-dash/internal/wiki"
)

// PageMaterializer performs safe filesystem operations for generated wiki pages.
type PageMaterializer struct{}

func (PageMaterializer) Inspect(path string) (wiki.FileInspection, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return wiki.FileInspection{}, nil
	}
	if err != nil {
		return wiki.FileInspection{}, fmt.Errorf("inspect page path %q: %w", path, err)
	}
	return wiki.FileInspection{Exists: true, Regular: info.Mode().IsRegular()}, nil
}

func (PageMaterializer) EnsureRoot(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, 0o755); err != nil {
			return fmt.Errorf("create wiki directory %q: %w", path, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect wiki directory %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("wiki path %q is not a real directory", path)
	}
	return nil
}

func (PageMaterializer) WriteAtomic(path string, content []byte) error {
	inspection, err := (PageMaterializer{}).Inspect(path)
	if err != nil {
		return err
	}
	if inspection.Exists && !inspection.Regular {
		return fmt.Errorf("wiki page path %q is not a regular file", path)
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), ".devdash-wiki-*")
	if err != nil {
		return fmt.Errorf("create temporary wiki page: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set temporary wiki page permissions: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary wiki page: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary wiki page: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary wiki page: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace wiki page %q: %w", path, err)
	}
	removeTemporary = false
	return nil
}

func (PageMaterializer) Remove(path string) error {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("remove wiki page %q: %w", path, err)
	}
	return nil
}
