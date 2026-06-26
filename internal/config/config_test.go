package config

import "testing"

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
