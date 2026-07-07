package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func (stubRouter) ListRouteTraces() []domain.RouteTrace { return nil }

func TestUpdateTypesQInChatInsteadOfQuitting(t *testing.T) {
	m := model{
		page:           pageChat,
		selected:       pageChat,
		contentFocused: true,
		chatOpen:       true,
		chats:          []tuiChatSession{newTUIChatSession(1, "gpt")},
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

func TestUpdateTypesJKInChatInsteadOfMovingSession(t *testing.T) {
	m := model{
		page:           pageChat,
		selected:       pageChat,
		contentFocused: true,
		chatOpen:       true,
		chats:          []tuiChatSession{newTUIChatSession(1, "gpt"), newTUIChatSession(2, "gpt")},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j', 'k'}})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command while typing in chat")
	}
	if got.chatInput != "jk" {
		t.Fatalf("expected chat input to contain jk, got %q", got.chatInput)
	}
	if got.activeChat != 0 {
		t.Fatalf("expected active chat to stay unchanged, got %d", got.activeChat)
	}
}

func TestUpdateTypesQInConfigFormInsteadOfQuitting(t *testing.T) {
	m := model{
		page:           pageConfig,
		contentFocused: true,
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

func TestEnterFocusesSelectedPageFromMenu(t *testing.T) {
	m := model{page: pageProviders, selected: pageChat}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when opening selected page")
	}
	if got.page != pageChat {
		t.Fatalf("expected page chat, got %d", got.page)
	}
	if !got.contentFocused {
		t.Fatal("expected content to be focused after enter")
	}
	if got.chatOpen {
		t.Fatal("expected chat to open on session picker")
	}
}

func TestEscReturnsFromChatSessionsToMenu(t *testing.T) {
	m := model{page: pageChat, selected: pageChat, contentFocused: true, chats: []tuiChatSession{newTUIChatSession(1, "gpt")}}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when returning to menu")
	}
	if got.contentFocused {
		t.Fatal("expected content focus to be false after esc")
	}
	if got.selected != pageChat {
		t.Fatalf("expected selected menu to stay on chat, got %d", got.selected)
	}
}

func TestEscClearsChatInputBeforeReturningToSessions(t *testing.T) {
	m := model{page: pageChat, selected: pageChat, contentFocused: true, chatOpen: true, chatInput: "hello", chats: []tuiChatSession{newTUIChatSession(1, "gpt")}}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when clearing chat input")
	}
	if got.chatInput != "" {
		t.Fatalf("expected chat input to be cleared, got %q", got.chatInput)
	}
	if !got.contentFocused {
		t.Fatal("expected chat to stay focused after clearing input")
	}
	if !got.chatOpen {
		t.Fatal("expected chat conversation to stay open after clearing input")
	}
}

func TestEnterOpensSelectedChatSession(t *testing.T) {
	m := model{
		page:           pageChat,
		selected:       pageChat,
		contentFocused: true,
		chats:          []tuiChatSession{newTUIChatSession(1, "gpt")},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when opening chat session")
	}
	if !got.chatOpen {
		t.Fatal("expected selected chat session to open")
	}
}

func TestEscFromChatConversationReturnsToSessions(t *testing.T) {
	m := model{
		page:           pageChat,
		selected:       pageChat,
		contentFocused: true,
		chatOpen:       true,
		chats:          []tuiChatSession{newTUIChatSession(1, "gpt")},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when returning to chat sessions")
	}
	if !got.contentFocused {
		t.Fatal("expected chat page to stay focused")
	}
	if got.chatOpen {
		t.Fatal("expected chat conversation to close")
	}
}

func TestEscReturnsFromConfigBrowseToMenu(t *testing.T) {
	m := model{page: pageConfig, selected: pageConfig, contentFocused: true, cfg: cloneConfig(config.Default()), editor: newConfigEditorState(config.Default())}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when returning to menu")
	}
	if got.contentFocused {
		t.Fatal("expected content focus to be false after esc")
	}
	if got.selected != pageConfig {
		t.Fatalf("expected selected menu to stay on config, got %d", got.selected)
	}
}

func TestChatViewDoesNotExceedTerminalHeightWithLongConversation(t *testing.T) {
	cfg := cloneConfig(config.Default())
	chat := newTUIChatSession(1, "mimo-v2.5-pro")
	for i := 0; i < 40; i++ {
		chat.Messages = append(chat.Messages, tuiChatMessage{
			Role:      "assistant",
			Content:   "this is a long response that should wrap across multiple lines and must not push the menu or header out of the terminal viewport",
			CreatedAt: time.Now(),
		})
	}
	m := model{
		cfg:            cfg,
		page:           pageChat,
		selected:       pageChat,
		contentFocused: true,
		chatOpen:       true,
		width:          120,
		height:         28,
		styles:         defaultStyles(defaultTheme),
		chats:          []tuiChatSession{chat},
	}

	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("expected view height <= %d, got %d", m.height, got)
	}
}

func TestChatPageUpScrollsHistoryWithoutMovingSession(t *testing.T) {
	activeChat := newTUIChatSession(2, "gpt")
	for i := 0; i < 8; i++ {
		activeChat.Messages = append(activeChat.Messages, tuiChatMessage{
			Role:      "assistant",
			Content:   fmt.Sprintf("message %d", i),
			CreatedAt: time.Now(),
		})
	}
	m := model{
		page:           pageChat,
		selected:       pageChat,
		contentFocused: true,
		chatOpen:       true,
		height:         30,
		styles:         defaultStyles(defaultTheme),
		chats:          []tuiChatSession{newTUIChatSession(1, "gpt"), activeChat},
		activeChat:     1,
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when scrolling chat history")
	}
	if got.activeChat != 1 {
		t.Fatalf("expected active chat to stay unchanged, got %d", got.activeChat)
	}
	if got.chatScroll == 0 {
		t.Fatal("expected chat scroll to increase")
	}
}

func TestChatConversationCanRenderOlderHistoryWhenScrolled(t *testing.T) {
	chat := newTUIChatSession(1, "gpt")
	for i := 0; i < 24; i++ {
		chat.Messages = append(chat.Messages, tuiChatMessage{
			Role:      "assistant",
			Content:   fmt.Sprintf("history marker %02d", i),
			CreatedAt: time.Now(),
		})
	}
	m := model{
		styles:     defaultStyles(defaultTheme),
		chatScroll: 1 << 20,
	}

	view := m.renderChatConversation(chat, 80, 18)

	if !strings.Contains(view, "history marker 00") {
		t.Fatal("expected scrolled chat view to include oldest message")
	}
	if strings.Contains(view, "history marker 23") {
		t.Fatal("expected scrolled chat view to hide latest message")
	}
}

func TestOpenChatConversationDoesNotRenderSessionPicker(t *testing.T) {
	m := model{
		cfg:            cloneConfig(config.Default()),
		page:           pageChat,
		selected:       pageChat,
		contentFocused: true,
		chatOpen:       true,
		width:          120,
		height:         28,
		styles:         defaultStyles(defaultTheme),
		chats:          []tuiChatSession{newTUIChatSession(1, "gpt")},
	}

	view := m.View()

	if strings.Contains(view, ".:: SESSIONS ::.") {
		t.Fatal("expected open chat conversation not to render session picker")
	}
	if !strings.Contains(view, "type your prompt") {
		t.Fatal("expected open chat conversation to render prompt input")
	}
}

func TestChatSessionPickerKeepsLongPreviewCompact(t *testing.T) {
	chat := newTUIChatSession(1, "gpt")
	chat.Title = "A very long generated title that should not take over the entire session list row"
	chat.Messages = append(chat.Messages, tuiChatMessage{
		Role:      "assistant",
		Content:   "first line of a very long assistant response\nsecond line should stay hidden " + strings.Repeat("tail ", 40),
		CreatedAt: time.Now(),
	})
	m := model{
		styles: defaultStyles(defaultTheme),
		chats:  []tuiChatSession{chat},
	}

	view := m.renderChatSessionPicker(80)
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 80 {
			t.Fatalf("expected session picker line width <= 80, got %d for %q", lipgloss.Width(line), line)
		}
	}
	if strings.Contains(view, "second line should stay hidden") {
		t.Fatal("expected multiline assistant response to be collapsed in session preview")
	}
	if strings.Count(view, "tail") > 8 {
		t.Fatal("expected long assistant response preview to be truncated")
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

func TestConfigPageTabMovesMainMenu(t *testing.T) {
	m := model{
		page:     pageConfig,
		selected: pageConfig,
		cfg:      cloneConfig(config.Default()),
		editor:   newConfigEditorState(config.Default()),
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := updated.(model)

	if cmd != nil {
		t.Fatal("expected no command when moving menu from config page")
	}
	if got.selected != nextIndex(pageConfig, len(navItems)) {
		t.Fatalf("expected menu to move to next page, got %d", got.selected)
	}
	if got.editor.selected != 0 {
		t.Fatalf("expected config row selection to stay unchanged, got %d", got.editor.selected)
	}
}

func TestConfigPageDownMovesRows(t *testing.T) {
	cfg := cloneConfig(config.Default())
	cfg.Providers = append(cfg.Providers, config.ProviderConfig{ID: "extra", Name: "Extra", Type: "openai-compatible", BaseURL: "https://example.org/v1", AuthType: "bearer", TimeoutSeconds: 60, Enabled: true})

	m := model{
		page:           pageConfig,
		selected:       pageConfig,
		contentFocused: true,
		cfg:            cfg,
		editor:         newConfigEditorState(cfg),
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
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
		page:         pageKeys,
		selected:     pageKeys,
		router:       stubRouter{},
		keyTesting:   true,
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
		page:         pageKeys,
		selected:     pageKeys,
		keyTesting:   true,
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
