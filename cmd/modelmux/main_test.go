package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/config"
)

func setupTestConfig(t *testing.T, baseURL string) string {
	t.Helper()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = baseURL + "/v1"
	cfg.Providers[0].TimeoutSeconds = 5
	cfg.Models[0].ModelName = cfg.Models[0].ID
	cfg.Keys[0].Value = "test-key-value"

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save test config: %v", err)
	}
	return path
}

func TestKeyTestCLISuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	configPath := setupTestConfig(t, server.URL)
	cmd := newRootCommand()

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", configPath, "key", "test", "--id", "mimo-key-1"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if stdout.String() != "OK: mimo-key-1\n" {
		t.Fatalf("expected 'OK: mimo-key-1', got %q", stdout.String())
	}
}

func TestKeyTestCLIFailsOnUnknownKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	configPath := setupTestConfig(t, server.URL)
	cmd := newRootCommand()

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", configPath, "key", "test", "--id", "unknown-key"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestKeyTestCLIFailsOn401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	configPath := setupTestConfig(t, server.URL)
	cmd := newRootCommand()

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", configPath, "key", "test", "--id", "mimo-key-1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 401 Unauthorized")
	}
	stderrOutput := stderr.String()
	if stderrOutput == "" {
		t.Fatal("expected FAIL message on stderr for 401")
	}
}

func TestKeyTestCLIRequiresIDFlag(t *testing.T) {
	cmd := newRootCommand()

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", "/tmp/nonexistent.yaml", "key", "test"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --id flag")
	}
}
