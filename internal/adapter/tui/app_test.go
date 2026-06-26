package tui

import (
	"context"
	"net/http"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
	"github.com/livingdolls/yute-modelmux/internal/core/port/inbound"
)

type stubRouter struct{}

func (stubRouter) HandleChatCompletion(context.Context, *http.Request) (*http.Response, error) {
	return nil, nil
}

func (stubRouter) HandleCompletion(context.Context, *http.Request) (*http.Response, error) {
	return nil, nil
}

func (stubRouter) SelectKey(context.Context, string) (*domain.APIKey, error) { return nil, nil }

func (stubRouter) MarkKeyResult(context.Context, string, inbound.KeyResult) error { return nil }

func (stubRouter) ListProviders() []domain.Provider { return nil }

func (stubRouter) ListModels() []domain.Model { return nil }

func (stubRouter) ListModelGroups() []domain.ModelGroup { return nil }

func (stubRouter) ListKeys() []domain.APIKey { return nil }

func (stubRouter) Logs() []domain.RequestLog { return nil }

func (stubRouter) TestKey(context.Context, string) error { return nil }

func TestUpdateTypesQInChatInsteadOfQuitting(t *testing.T) {
	m := model{
		page:     pageChat,
		selected: pageChat,
		chats:    []tuiChatSession{newTUIChatSession(1, "gpt")},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no quit command while typing in chat")
	}
	if got.chatInput != "q" {
		t.Fatalf("expected chat input to contain q, got %q", got.chatInput)
	}
}

func TestUpdateTypesQInConfigFormInsteadOfQuitting(t *testing.T) {
	m := model{
		page: pageConfig,
		editor: configEditorState{
			mode: editorModeForm,
			form: configFormState{
				items: []formField{{label: "ID"}},
			},
		},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no quit command while editing config form")
	}
	if got.editor.form.items[0].value != "q" {
		t.Fatalf("expected form field to contain q, got %q", got.editor.form.items[0].value)
	}
}

func TestUpdateQuitsWithQOutsideTextEntry(t *testing.T) {
	m := model{page: pageProviders}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command outside text entry mode")
	}
}

func TestThemePickerAppliesSelectedTheme(t *testing.T) {
	m := model{page: pageProviders, theme: defaultTheme, styles: defaultStyles(defaultTheme)}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	got := updated.(model)
	if cmd != nil {
		t.Fatal("expected no command when opening theme picker")
	}
	if !got.showThemePicker {
		t.Fatal("expected theme picker to open")
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(model)

	if got.showThemePicker {
		t.Fatal("expected theme picker to close after selection")
	}
	if got.theme == defaultTheme {
		t.Fatal("expected theme to change after selecting another preset")
	}
}

func TestSaveDraftConfigPreservesThemeAcrossRouterReload(t *testing.T) {
	cfg := cloneConfig(config.Default())
	m := model{
		cfg:        cfg,
		savedCfg:   cloneConfig(cfg),
		theme:      "matrix",
		styles:     defaultStyles("matrix"),
		saveConfig: func(*config.Config) error { return nil },
		reloadRouter: func(*config.Config) (inbound.RouterService, error) {
			return stubRouter{}, nil
		},
	}

	m.saveDraftConfig()

	if m.theme != "matrix" {
		t.Fatalf("expected theme to persist after router reload, got %q", m.theme)
	}
	if m.router == nil {
		t.Fatal("expected router to be reloaded")
	}
}

func TestConfigPageUpDownMovesMainMenu(t *testing.T) {
	m := model{
		page:     pageConfig,
		selected: pageConfig,
		cfg:      cloneConfig(config.Default()),
		editor:   newConfigEditorState(config.Default()),
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when moving menu from config page")
	}
	if got.selected != pageLogs {
		t.Fatalf("expected selected menu to move to logs, got %d", got.selected)
	}
	if got.editor.selected != 0 {
		t.Fatalf("expected config row selection to stay unchanged, got %d", got.editor.selected)
	}
}

func TestConfigPageJKMovesRows(t *testing.T) {
	cfg := cloneConfig(config.Default())
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{ID: "extra", Name: "Extra", Type: "openai-compatible", BaseURL: "https://example.org/v1", AuthType: "bearer", TimeoutSeconds: 60, Enabled: true})

	m := model{
		page:     pageConfig,
		selected: pageConfig,
		cfg:      cfg,
		editor:   newConfigEditorState(cfg),
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when moving config rows")
	}
	if got.selected != pageConfig {
		t.Fatalf("expected main menu selection to stay on config, got %d", got.selected)
	}
	if got.editor.selected != 1 {
		t.Fatalf("expected row selection to move to next row, got %d", got.editor.selected)
	}
}

func TestKeyTestEnterReturnsAsyncCommand(t *testing.T) {
	m := model{
		page:        pageKeys,
		selected:    pageKeys,
		router:      stubRouter{},
		keyTesting:  true,
		keyTestInput: "test-key-1",
	}

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := m.updateKeyTest(msg)
	got := updated.(model)

	if cmd == nil {
		t.Fatal("expected async command for key test, got nil")
	}
	if !got.keyTestRunning {
		t.Fatal("expected keyTestRunning to be true after enter")
	}
	if got.keyTestResult != "testing key test-key-1..." {
		t.Fatalf("expected 'testing key...' result, got %q", got.keyTestResult)
	}
}

func TestKeyTestEscExitsMode(t *testing.T) {
	m := model{
		page:        pageKeys,
		selected:    pageKeys,
		keyTesting:  true,
		keyTestInput: "some-key",
	}

	msg := tea.KeyMsg{Type: tea.KeyEscape}
	updated, cmd := m.updateKeyTest(msg)
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command for esc")
	}
	if got.keyTesting {
		t.Fatal("expected keyTesting to be false after esc")
	}
}
