package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/config"
	"gopkg.in/yaml.v3"
)

func boolPtr(v bool) *bool { return &v }

func setupTestConfig(t *testing.T, baseURL string) string {
	t.Helper()

	cfg := config.Default()
	cfg.Server.Admin.RequireAuth = boolPtr(false)
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

func TestConfigValidateValid(t *testing.T) {
	configPath := setupTestConfig(t, "https://api.example.com/v1")

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", configPath, "config", "validate"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected valid config, got error: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid") {
		t.Fatalf("expected 'valid' in stdout, got: %s", stdout.String())
	}
}

func TestConfigValidateJSON(t *testing.T) {
	configPath := setupTestConfig(t, "https://api.example.com/v1")

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", configPath, "config", "validate", "--json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected valid config, got error: %v\nstderr: %s", err, stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if !strings.Contains(out, `"valid"`) || !strings.Contains(out, `true`) {
		t.Fatalf("expected valid in json output, got: %s", out)
	}
}

func TestConfigValidateInvalid(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Host = ""

	dir := t.TempDir()
	configPath := filepath.Join(dir, "badconfig.yaml")
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(configPath, data, 0o600)

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", configPath, "config", "validate"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	stderrOut := stderr.String()
	if !strings.Contains(stderrOut, "error") && !strings.Contains(stderrOut, "error(s)") {
		t.Fatalf("expected error output on stderr, got: %s", stderrOut)
	}
}

func TestConfigValidateBadYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "badyaml.yaml")
	os.WriteFile(configPath, []byte("{ this is not valid yaml!!"), 0o600)

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", configPath, "config", "validate"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "YAML") {
		t.Fatalf("expected YAML error, got: %v", err)
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := newRootCommand()

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected version command to succeed, got error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"modelmux", "commit:", "built:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got: %s", want, out)
		}
	}
}

func TestGroupsCLIJSONIncludesKeyMembers(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Admin.RequireAuth = boolPtr(false)
	cfg.Providers[0].BaseURL = "https://api.example.com/v1"
	cfg.Keys[0].Value = "test-key-value"
	cfg.ModelGroups = []config.ModelGroupConfig{{
		ID:       "mixed",
		Name:     "Mixed",
		Strategy: "failover",
		Enabled:  true,
		Members: []config.ModelGroupMemberConfig{
			{ModelID: "mimo-v2.5-pro", Priority: 1, Weight: 1, Enabled: true},
			{KeyID: "mimo-key-1", Priority: 2, Weight: 1, Enabled: true},
		},
	}}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", configPath, "groups", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("groups --json failed: %v\nstderr: %s", err, stderr.String())
	}

	var groups []struct {
		ID      string `json:"id"`
		Members []struct {
			ModelID string `json:"model_id"`
			KeyID   string `json:"key_id"`
		} `json:"members"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &groups); err != nil {
		t.Fatalf("decode groups json: %v\n%s", err, stdout.String())
	}
	if len(groups) != 1 || groups[0].ID != "mixed" || len(groups[0].Members) != 2 {
		t.Fatalf("unexpected groups output: %+v", groups)
	}
	if groups[0].Members[0].ModelID != "mimo-v2.5-pro" {
		t.Fatalf("expected model member, got %+v", groups[0].Members[0])
	}
	if groups[0].Members[1].KeyID != "mimo-key-1" {
		t.Fatalf("expected key member, got %+v", groups[0].Members[1])
	}
}
