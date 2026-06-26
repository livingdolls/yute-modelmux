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

func TestValidateRejectsKeyWithoutValue(t *testing.T) {
	cfg := Default()
	cfg.Keys[0].Value = ""
	cfg.Keys[0].ValueEnv = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing key value error")
	}
}

func TestValidateRejectsChatSessionIDConflict(t *testing.T) {
	cfg := Default()
	cfg.ChatSessions[0].ID = cfg.ModelGroups[0].ID
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected chat session id conflict error")
	}
}

func TestValidateRejectsUnknownChatSessionTarget(t *testing.T) {
	cfg := Default()
	cfg.ChatSessions[0].Target = "missing-target"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unknown chat session target error")
	}
}
