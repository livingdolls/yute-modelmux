package config

import (
	"path/filepath"
	"testing"
)

func TestSecretStorePathUsesDatabaseDirectory(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"default filename", filepath.Join("tmp", "modelmux.db"), filepath.Join("tmp", "secrets.enc")},
		{"custom filename", filepath.Join("tmp", "custom.db"), filepath.Join("tmp", "secrets.enc")},
		{"relative filename", "custom.db", "secrets.enc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SecretStorePath(tt.path); got != tt.want {
				t.Fatalf("SecretStorePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
