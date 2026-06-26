package tui

import (
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/config"
)

func TestDeleteModelRemovesKeysAndGroupMembers(t *testing.T) {
	cfg := config.Default()
	cfg.Models = append(cfg.Models, config.ModelConfig{ID: "extra", ProviderID: "mimo", ModelName: "extra", Strategy: "failover", Enabled: true})
	cfg.Keys = append(cfg.Keys, config.KeyConfig{ID: "extra-key", ProviderID: "mimo", ModelID: "extra", Value: "key", Status: "active", Priority: 1})
	cfg.ModelGroups[0].Members = append(cfg.ModelGroups[0].Members, config.ModelGroupMemberConfig{ModelID: "extra", Priority: 2, Weight: 1, Enabled: true})
	m := model{cfg: cfg, editor: configEditorState{section: configSectionModels, selected: 1, confirm: deleteConfirmState{kind: configSectionModels, index: 1}}}

	m.applyDeleteConfigItem()

	for _, item := range m.cfg.Keys {
		if item.ModelID == "extra" {
			t.Fatalf("expected extra model keys to be removed: %+v", m.cfg.Keys)
		}
	}
	for _, member := range m.cfg.ModelGroups[0].Members {
		if member.ModelID == "extra" {
			t.Fatalf("expected extra model to be removed from group: %+v", m.cfg.ModelGroups[0].Members)
		}
	}
}

func TestDeleteProviderDisablesEmptyGroup(t *testing.T) {
	cfg := config.Default()
	m := model{cfg: cfg, editor: configEditorState{section: configSectionProviders, selected: 0, confirm: deleteConfirmState{kind: configSectionProviders, index: 0}}}

	m.applyDeleteConfigItem()

	if len(m.cfg.Providers) != 0 || len(m.cfg.Models) != 0 || len(m.cfg.Keys) != 0 {
		t.Fatalf("expected provider cascade delete, got providers=%d models=%d keys=%d", len(m.cfg.Providers), len(m.cfg.Models), len(m.cfg.Keys))
	}
	if len(m.cfg.ModelGroups) != 1 || m.cfg.ModelGroups[0].Enabled || len(m.cfg.ModelGroups[0].Members) != 0 {
		t.Fatalf("expected empty group disabled, got %+v", m.cfg.ModelGroups)
	}
	if err := m.cfg.Validate(); err != nil {
		t.Fatalf("cascade-deleted config should validate: %v", err)
	}
}
