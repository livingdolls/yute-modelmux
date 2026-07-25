package config

import (
	"strings"
	"testing"
)

func TestValidateModelGroupMetadataAndConsistentHash(t *testing.T) {
	cfg := Default()
	cfg.ModelGroups[0].Strategy = "consistent_hash"
	cfg.ModelGroups[0].Description = "Stable agent routing"
	cfg.ModelGroups[0].RequiredCapabilities = []string{"chat", "streaming", "tools"}
	cfg.ModelGroups[0].ContextWindow = 128000
	cfg.ModelGroups[0].MaxOutputTokens = 32000
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid group metadata: %v", err)
	}
}

func TestValidateRejectsInvalidModelGroupMetadata(t *testing.T) {
	cfg := Default()
	cfg.ModelGroups[0].Strategy = "random"
	cfg.ModelGroups[0].RequiredCapabilities = []string{"telepathy"}
	cfg.ModelGroups[0].ContextWindow = -1
	cfg.ModelGroups[0].MaxOutputTokens = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"consistent_hash", "required capability telepathy", "context_window must be non-negative", "max_output_tokens must be non-negative"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in %q", want, err.Error())
		}
	}
}
