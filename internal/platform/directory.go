package platform

import (
	"fmt"
	"os"
)

type DirectoryManager struct{}

func (DirectoryManager) Ensure(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", path, err)
	}
	return nil
}
