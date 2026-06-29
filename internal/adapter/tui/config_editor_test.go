package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestEditKeyPreservesSecretRefAndDailyLimits(t *testing.T) {
	cfg := config.Default()
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Name: "Test Key", Value: "sk-abc", ValueEnv: "", SecretRef: "secret://store/key1", Status: "active", Priority: 5, DailyRequestLimit: 100, DailyTokenLimit: 50000},
	}

	m := model{cfg: cfg}
	m.editor.section = configSectionKeys
	m.editor.selected = 0
	m.editor.mode = editorModeForm
	m.editor.form = newConfigForm(m.editor.section, 0, &m)
	m.applyConfigForm()

	edited := m.cfg.Keys[0]
	if edited.SecretRef != "secret://store/key1" {
		t.Fatalf("expected secret_ref to be preserved, got %q", edited.SecretRef)
	}
	if edited.DailyRequestLimit != 100 {
		t.Fatalf("expected daily_request_limit 100, got %d", edited.DailyRequestLimit)
	}
	if edited.DailyTokenLimit != 50000 {
		t.Fatalf("expected daily_token_limit 50000, got %d", edited.DailyTokenLimit)
	}
	if edited.Priority != 5 {
		t.Fatalf("expected priority 5, got %d", edited.Priority)
	}
	if edited.ID != "key-1" {
		t.Fatalf("expected id key-1, got %q", edited.ID)
	}
}

func TestModelProviderIDCanBeSelectedFromProviders(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{ID: "second", Name: "Second", Type: "openai-compatible", BaseURL: "https://second.example.com/v1", AuthType: "bearer", TimeoutSeconds: 120, Enabled: true})
	m := model{cfg: cfg}
	m.editor.section = configSectionModels
	m.editor.mode = editorModeForm
	m.editor.form = newConfigForm(m.editor.section, -1, &m)
	m.editor.form.field = 1

	updated, cmd := m.updateConfigForm(tea.KeyMsg{Type: tea.KeyRight})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when opening provider id select")
	}
	if !got.editor.form.selectOpen {
		t.Fatal("expected provider id select popup to open")
	}
	if !strings.Contains(got.renderConfigForm(), "Select Provider ID") || !strings.Contains(got.renderConfigForm(), "second") {
		t.Fatal("expected select popup to render provider id options")
	}

	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(model)
	if got.editor.form.selectOpen {
		t.Fatal("expected provider id select popup to close after apply")
	}
	if got.editor.form.items[1].value != "mimo" {
		t.Fatalf("expected first provider id to be selected, got %q", got.editor.form.items[1].value)
	}

	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyRight})
	got = updated.(model)
	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(model)
	if got.editor.form.items[1].value != "second" {
		t.Fatalf("expected second provider id to be selected, got %q", got.editor.form.items[1].value)
	}
}

func TestKeyModelIDCanBeSelectedFromModels(t *testing.T) {
	cfg := config.Default()
	cfg.Models = append(cfg.Models, config.ModelConfig{ID: "extra-model", ProviderID: "mimo", ModelName: "extra", Strategy: "failover", Enabled: true})
	m := model{cfg: cfg}
	m.editor.section = configSectionKeys
	m.editor.mode = editorModeForm
	m.editor.form = newConfigForm(m.editor.section, -1, &m)
	m.editor.form.field = 2

	updated, _ := m.updateConfigForm(tea.KeyMsg{Type: tea.KeyRight})
	got := updated.(model)
	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(model)

	if got.editor.form.items[2].value != "extra-model" {
		t.Fatalf("expected extra model id to be selected, got %q", got.editor.form.items[2].value)
	}
}

func TestGroupMembersCanSelectModelIDs(t *testing.T) {
	cfg := config.Default()
	cfg.Models = append(cfg.Models, config.ModelConfig{ID: "extra-model", ProviderID: "mimo", ModelName: "extra", Strategy: "failover", Enabled: true})
	m := model{cfg: cfg}
	m.editor.section = configSectionGroups
	m.editor.mode = editorModeForm
	m.editor.form = newConfigForm(m.editor.section, -1, &m)
	m.editor.form.field = 3

	updated, _ := m.updateConfigForm(tea.KeyMsg{Type: tea.KeyRight})
	got := updated.(model)
	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(model)

	if got.editor.form.items[3].value != "extra-model" {
		t.Fatalf("expected selected group member model id, got %q", got.editor.form.items[3].value)
	}

	got.editor.form.items[3].value += ","
	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyRight})
	got = updated.(model)
	updated, _ = got.updateConfigForm(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(model)
	if got.editor.form.items[3].value != "extra-model,mimo-v2.5-pro" {
		t.Fatalf("expected select to fill the trailing member token, got %q", got.editor.form.items[3].value)
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
