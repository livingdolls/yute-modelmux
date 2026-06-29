package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestValidateRejectsModelGroupIDConflict(t *testing.T) {
	cfg := Default()
	cfg.ModelGroups[0].ID = cfg.Models[0].ID
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected model group id conflict error")
	}
}

func TestValidateRejectsUnknownModelGroupMember(t *testing.T) {
	cfg := Default()
	cfg.ModelGroups[0].Members[0].ModelID = "missing-model"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unknown model group member error")
	}
}

func TestValidateAllowsKeyModelGroupMember(t *testing.T) {
	cfg := Default()
	cfg.ModelGroups[0].Members = []ModelGroupMemberConfig{{KeyID: "mimo-key-1", Priority: 1, Weight: 1, Enabled: true}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("key group member should be valid: %v", err)
	}
}

func TestValidateAllowsMixedModelAndKeyGroupMembers(t *testing.T) {
	cfg := Default()
	cfg.Keys = append(cfg.Keys, KeyConfig{ID: "mimo-key-2", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "key-2", Status: "active", Priority: 2})
	cfg.ModelGroups[0].Members = []ModelGroupMemberConfig{
		{ModelID: "mimo-v2.5-pro", Priority: 1, Weight: 1, Enabled: true},
		{KeyID: "mimo-key-2", Priority: 2, Weight: 1, Enabled: true},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("mixed model/key group members should be valid: %v", err)
	}
}

func TestValidateRejectsAmbiguousModelGroupMember(t *testing.T) {
	cfg := Default()
	cfg.ModelGroups[0].Members = []ModelGroupMemberConfig{{ModelID: "mimo-v2.5-pro", KeyID: "mimo-key-1", Priority: 1, Weight: 1, Enabled: true}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected ambiguous group member error")
	}
}

func TestValidateRejectsUnknownKeyGroupMember(t *testing.T) {
	cfg := Default()
	cfg.ModelGroups[0].Members = []ModelGroupMemberConfig{{KeyID: "missing-key", Priority: 1, Weight: 1, Enabled: true}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unknown key group member error")
	}
}

func TestValidateAllowsDisabledEmptyModelGroup(t *testing.T) {
	cfg := Default()
	cfg.ModelGroups[0].Enabled = false
	cfg.ModelGroups[0].Members = nil
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled empty group should be valid: %v", err)
	}
}

func TestValidateRejectsKeyWithoutValue(t *testing.T) {
	cfg := Default()
	cfg.Keys[0].Value = ""
	cfg.Keys[0].ValueEnv = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing key value error")
	}
}

func TestSaveDoesNotPersistResolvedEnvSecret(t *testing.T) {
	cfg := Default()
	cfg.Keys[0].Value = ""
	cfg.Keys[0].ValueEnv = "MUX_TEST_SECRET"
	t.Setenv("MUX_TEST_SECRET", "super-secret-token")

	if err := cfg.ResolveSecrets(); err != nil {
		t.Fatalf("resolve secrets failed: %v", err)
	}
	if cfg.Keys[0].Value != "" {
		t.Fatal("expected Value to remain empty after ResolveSecrets")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config failed: %v", err)
	}
	saved := string(data)
	if saved == "" {
		t.Fatal("saved config is empty")
	}
	if strings.Contains(saved, "super-secret-token") {
		t.Fatal("saved YAML contains env secret, secret leaked")
	}
	if !strings.Contains(saved, "value_env: MUX_TEST_SECRET") {
		t.Fatal("saved YAML should preserve value_env")
	}
}

func TestResolveSecretsFailsWhenEnvVarMissing(t *testing.T) {
	cfg := Default()
	cfg.Keys[0].Value = ""
	cfg.Keys[0].ValueEnv = "MUX_MISSING_VAR"
	if err := cfg.ResolveSecrets(); err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestValidateAllowsEmptyValueWithValueEnv(t *testing.T) {
	cfg := Default()
	cfg.Keys[0].Value = ""
	cfg.Keys[0].ValueEnv = "MUX_EXISTS"
	t.Setenv("MUX_EXISTS", "token")

	if err := cfg.ResolveSecrets(); err != nil {
		t.Fatalf("resolve secrets failed: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate should allow key with value_env: %v", err)
	}
}

func TestValidateRejectsKeyProviderMismatchWithModel(t *testing.T) {
	cfg := Default()
	cfg.Providers = append(cfg.Providers, ProviderConfig{ID: "other", Name: "Other", Type: "openai-compatible", BaseURL: "https://other.example.com/v1", AuthType: "bearer", TimeoutSeconds: 120, Enabled: true})
	cfg.Keys[0].ProviderID = "other"

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected key provider/model provider mismatch error")
	}
}

func TestValidateRejectsDuplicateKeyIDWithValueEnv(t *testing.T) {
	t.Setenv("MUX_DUP_A", "token-a")
	t.Setenv("MUX_DUP_B", "token-b")

	cfg := Default()
	cfg.Keys = []KeyConfig{
		{ID: "dup", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", ValueEnv: "MUX_DUP_A", Status: "active", Priority: 1},
		{ID: "dup", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", ValueEnv: "MUX_DUP_B", Status: "active", Priority: 2},
	}

	if err := cfg.ResolveSecrets(); err != nil {
		t.Fatalf("resolve secrets failed: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate key id error for value_env keys")
	}
}

func TestValidateRejectsDuplicateKeyIDMixedValueAndValueEnv(t *testing.T) {
	t.Setenv("MUX_DUP_C", "token-c")

	cfg := Default()
	cfg.Keys = []KeyConfig{
		{ID: "dup", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", ValueEnv: "MUX_DUP_C", Status: "active", Priority: 1},
		{ID: "dup", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "direct-token", Status: "active", Priority: 2},
	}

	if err := cfg.ResolveSecrets(); err != nil {
		t.Fatalf("resolve secrets failed: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate key id error for mixed value/value_env keys")
	}
}

func TestReloadFlowFailsWhenValueEnvMissing(t *testing.T) {
	cfg := Default()
	cfg.Keys[0].Value = ""
	cfg.Keys[0].ValueEnv = "MUX_RELOAD_MISSING"

	if err := cfg.ResolveSecrets(); err == nil {
		t.Fatal("expected resolve secrets error for missing env var during reload")
	}
}

func TestReloadFlowSucceedsWhenValueEnvAvailable(t *testing.T) {
	t.Setenv("MUX_RELOAD_EXISTS", "reload-token")

	cfg := Default()
	cfg.Keys[0].Value = ""
	cfg.Keys[0].ValueEnv = "MUX_RELOAD_EXISTS"

	if err := cfg.ResolveSecrets(); err != nil {
		t.Fatalf("resolve secrets failed: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
}

func TestResolveSecretsFailsWhenAuthTokenEnvMissing(t *testing.T) {
	cfg := Default()
	cfg.Server.RequireAuth = true
	cfg.Server.AuthTokenEnv = "MUX_AUTH_MISSING"

	if err := cfg.ResolveSecrets(); err == nil {
		t.Fatal("expected resolve secrets error for missing auth token env")
	}
}

func TestResolveSecretsAllowsConfiguredAuthTokenEnv(t *testing.T) {
	t.Setenv("MUX_AUTH_EXISTS", "local-token")

	cfg := Default()
	cfg.Server.RequireAuth = true
	cfg.Server.AuthTokenEnv = "MUX_AUTH_EXISTS"

	if err := cfg.ResolveSecrets(); err != nil {
		t.Fatalf("resolve secrets failed: %v", err)
	}
}

func TestValidateRejectsInvalidProviderType(t *testing.T) {
	cfg := Default()
	cfg.Providers[0].Type = "unknown-type"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid provider type error")
	}
}

func TestValidateRejectsMissingBaseURL(t *testing.T) {
	cfg := Default()
	cfg.Providers[0].BaseURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing base_url error")
	}
}

func TestValidateRejectsBadBaseURLScheme(t *testing.T) {
	cfg := Default()
	cfg.Providers[0].BaseURL = "ftp://example.com/v1"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected bad base_url scheme error")
	}
}

func TestValidateRejectsInvalidAuthType(t *testing.T) {
	cfg := Default()
	cfg.Providers[0].AuthType = "oauth2"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid auth_type error")
	}
}

func TestValidateRejectsHeaderAuthWithoutHeaderName(t *testing.T) {
	cfg := Default()
	cfg.Providers[0].AuthType = "header"
	cfg.Providers[0].AuthHeaderName = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing auth_header_name error")
	}
}

func TestValidateRejectsTimeoutOutOfRange(t *testing.T) {
	cfg := Default()
	cfg.Providers[0].TimeoutSeconds = 9999
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected timeout out of range error")
	}
}

func TestValidateRejectsInvalidModelStrategy(t *testing.T) {
	cfg := Default()
	cfg.Models[0].Strategy = "random"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid model strategy error")
	}
}

func TestValidateRejectsInvalidGroupStrategy(t *testing.T) {
	cfg := Default()
	cfg.ModelGroups[0].Strategy = "random"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid group strategy error")
	}
}

func TestValidateAllowsDisabledProviderWithoutFullValidation(t *testing.T) {
	cfg := Default()
	cfg.Providers[0].Enabled = false
	cfg.Providers[0].BaseURL = ""
	cfg.Providers[0].TimeoutSeconds = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled provider should skip strict validation: %v", err)
	}
}
