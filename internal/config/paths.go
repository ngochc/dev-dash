package config

import (
	"os"
	"path/filepath"
)

const envDatabasePath = "DEVDASH_DB"

func DatabasePath() (string, error) {
	if path := os.Getenv(envDatabasePath); path != "" {
		return filepath.Clean(path), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".devdash", "devdash.db"), nil
}
