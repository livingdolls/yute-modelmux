package config

import (
	"os"
	"path/filepath"
	"strings"
)

// SecretStorePath returns the encrypted secret store next to the configured
// SQLite database, regardless of the database file name.
func SecretStorePath(storagePath string) string {
	if storagePath == "" {
		storagePath = defaultStoragePath()
	}
	storagePath = expandPathHome(storagePath)
	return filepath.Join(filepath.Dir(storagePath), "secrets.enc")
}

func expandPathHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
