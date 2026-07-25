package secret

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func writeFileAtomic(path string, data []byte, mode fs.FileMode, validate func(string) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create secret directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".secrets-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary secret store: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set temporary secret store mode: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary secret store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary secret store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary secret store: %w", err)
	}
	if validate != nil {
		if err := validate(tmpPath); err != nil {
			return fmt.Errorf("validate secret store: %w", err)
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace secret store: %w", err)
	}
	removeTemp = false
	if dirHandle, err := os.Open(dir); err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return nil
}
