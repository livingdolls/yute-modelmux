package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/config"
)

func writeOpenCodeTestConfig(t *testing.T, cfg *config.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return path
}

func TestGenerateOpenCodeConfigWritesEnabledGroupsAndModels(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Admin.RequireAuth = boolPtr(false)
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 8787
	cfg.Server.RequireAuth = true
	cfg.Server.AuthTokenEnv = "MODELMUX_AUTH_TOKEN"
	cfg.Keys[0].Value = "test-key"
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID:         "disabled-model",
		ProviderID: cfg.Providers[0].ID,
		ModelName:  "disabled-model",
		Strategy:   "failover",
		Enabled:    false,
	})
	cfg.ModelGroups = []config.ModelGroupConfig{
		{
			ID:              "coding-balanced",
			Name:            "Coding — Balanced",
			Strategy:        "failover",
			Enabled:         true,
			ContextWindow:   128000,
			MaxOutputTokens: 32000,
			Members: []config.ModelGroupMemberConfig{{
				ModelID: cfg.Models[0].ID,
				Priority: 1,
				Weight:   1,
				Enabled:  true,
			}},
		},
		{
			ID:       "disabled-group",
			Name:     "Disabled",
			Strategy: "failover",
			Enabled:  false,
		},
	}

	configPath := writeOpenCodeTestConfig(t, cfg)
	outputPath := filepath.Join(t.TempDir(), "nested", "opencode.json")
	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", configPath, "config", "generate-opencode", "--output", outputPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate command failed: %v\nstderr: %s", err, stderr.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	var generated openCodeConfig
	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatalf("decode generated config: %v\n%s", err, data)
	}
	if generated.Model != "modelmux/coding-balanced" {
		t.Fatalf("unexpected default model: %s", generated.Model)
	}
	provider, ok := generated.Provider["modelmux"]
	if !ok {
		t.Fatalf("modelmux provider missing: %+v", generated.Provider)
	}
	if provider.Options.BaseURL != "http://127.0.0.1:8787/v1" {
		t.Fatalf("unexpected base URL: %s", provider.Options.BaseURL)
	}
	if provider.Options.APIKey != "{env:MODELMUX_AUTH_TOKEN}" {
		t.Fatalf("unexpected API key reference: %s", provider.Options.APIKey)
	}
	group, ok := provider.Models["coding-balanced"]
	if !ok || group.Limit == nil {
		t.Fatalf("group or limits missing: %+v", provider.Models)
	}
	if group.Limit.Context != 128000 || group.Limit.Output != 32000 {
		t.Fatalf("unexpected group limits: %+v", group.Limit)
	}
	if _, ok := provider.Models[cfg.Models[0].ID]; !ok {
		t.Fatalf("enabled physical model missing: %+v", provider.Models)
	}
	if _, ok := provider.Models["disabled-model"]; ok {
		t.Fatal("disabled physical model should not be generated")
	}
	if _, ok := provider.Models["disabled-group"]; ok {
		t.Fatal("disabled group should not be generated")
	}
	if !strings.Contains(stdout.String(), "created "+outputPath) {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestGenerateOpenCodeConfigToStdoutWithoutAuth(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Admin.RequireAuth = boolPtr(false)
	cfg.Server.RequireAuth = false
	cfg.Keys[0].Value = "test-key"
	configPath := writeOpenCodeTestConfig(t, cfg)

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"--config", configPath,
		"config", "opencode",
		"--output", "-",
		"--provider-id", "localmux",
		"--base-url", "https://modelmux.example.com/v1/",
		"--default-model", cfg.Models[0].ID,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate command failed: %v\nstderr: %s", err, stderr.String())
	}
	var generated openCodeConfig
	if err := json.Unmarshal(stdout.Bytes(), &generated); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if generated.Model != "localmux/"+cfg.Models[0].ID {
		t.Fatalf("unexpected selected model: %s", generated.Model)
	}
	provider := generated.Provider["localmux"]
	if provider.Options.BaseURL != "https://modelmux.example.com/v1" {
		t.Fatalf("unexpected overridden base URL: %s", provider.Options.BaseURL)
	}
	if provider.Options.APIKey != "" {
		t.Fatalf("API key should be omitted when auth is disabled: %s", provider.Options.APIKey)
	}
}

func TestGenerateOpenCodeConfigRequiresForceToOverwrite(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Admin.RequireAuth = boolPtr(false)
	cfg.Keys[0].Value = "test-key"
	configPath := writeOpenCodeTestConfig(t, cfg)
	outputPath := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(outputPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}

	cmd := newRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", configPath, "config", "generate-opencode", "--output", outputPath})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected overwrite error, got %v", err)
	}

	cmd = newRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", configPath, "config", "generate-opencode", "--output", outputPath, "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("force overwrite failed: %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read overwritten output: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("overwritten output is not JSON: %s", data)
	}
}
