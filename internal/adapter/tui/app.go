package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
	"github.com/livingdolls/yute-modelmux/internal/core/port/inbound"
)

const (
	pageProviders = iota
	pageModels
	pageGroups
	pageChat
	pageKeys
	pageLogs
	pageConfig
)

type navItem struct {
	label       string
	description string
}

var navItems = []navItem{
	{label: "Providers", description: "Upstream endpoints"},
	{label: "Models", description: "Routable models"},
	{label: "Groups", description: "Model aliases"},
	{label: "Chat", description: "TUI sessions"},
	{label: "Keys", description: "Live key state"},
	{label: "Logs", description: "Recent requests"},
	{label: "Config", description: "Edit routing"},
}

type Options struct {
	ConfigPath   string
	Config       *config.Config
	Router       inbound.RouterService
	SaveConfig   func(*config.Config) error
	ReloadRouter func(*config.Config) (inbound.RouterService, error)
}

type tuiChatMessage struct {
	Role      string
	Content   string
	CreatedAt time.Time
}

type tuiChatSession struct {
	ID       int
	Title    string
	Target   string
	Messages []tuiChatMessage
	Pending  bool
	Error    string
}

type model struct {
	cfg          *config.Config
	savedCfg     *config.Config
	configPath   string
	router       inbound.RouterService
	saveConfig   func(*config.Config) error
	reloadRouter func(*config.Config) (inbound.RouterService, error)
	selected     int
	page         int
	width        int
	styles       styles
	editor       configEditorState
	chats        []tuiChatSession
	activeChat   int
	chatInput    string
	nextChatID   int
}

type styles struct {
	app        lipgloss.Style
	header     lipgloss.Style
	title      lipgloss.Style
	subtitle   lipgloss.Style
	sidebar    lipgloss.Style
	nav        lipgloss.Style
	navActive  lipgloss.Style
	navMuted   lipgloss.Style
	panel      lipgloss.Style
	panelTitle lipgloss.Style
	tableHead  lipgloss.Style
	muted      lipgloss.Style
	good       lipgloss.Style
	bad        lipgloss.Style
	footer     lipgloss.Style
}

type tickMsg time.Time

type chatResponseMsg struct {
	sessionID int
	content   string
	err       error
}

func Run(options Options) error {
	draft := cloneConfig(options.Config)
	m := model{
		cfg:          draft,
		savedCfg:     cloneConfig(options.Config),
		configPath:   options.ConfigPath,
		router:       options.Router,
		saveConfig:   options.SaveConfig,
		reloadRouter: options.ReloadRouter,
		styles:       defaultStyles(),
		nextChatID:   2,
		editor:       newConfigEditorState(draft),
	}
	m.chats = []tuiChatSession{newTUIChatSession(1, defaultChatTarget(draft))}
	prog := tea.NewProgram(m, tea.WithAltScreen())
	_, err := prog.Run()
	return err
}

func defaultStyles() styles {
	return styles{
		app:        lipgloss.NewStyle().Padding(1, 2),
		header:     lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("238")),
		title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
		subtitle:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		sidebar:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(1, 1),
		nav:        lipgloss.NewStyle().Padding(0, 1),
		navActive:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1),
		navMuted:   lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		panel:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(1, 2),
		panelTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")),
		tableHead:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111")),
		muted:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		good:       lipgloss.NewStyle().Foreground(lipgloss.Color("82")),
		bad:        lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		footer:     lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1),
	}
}

func (m model) Init() tea.Cmd { return tickCmd() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		if key == "q" || key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.page == pageChat {
			return m.updateChat(msg)
		}
		if m.page == pageConfig {
			return m.updateConfigEditor(msg)
		}
		switch key {
		case "up", "k", "shift+tab":
			m.selected = previousIndex(m.selected, len(navItems))
		case "down", "j", "tab":
			m.selected = nextIndex(m.selected, len(navItems))
		case "enter", " ":
			m.page = m.selected
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tickMsg:
		return m, tickCmd()
	case chatResponseMsg:
		m.applyChatResponse(msg)
	}
	return m, nil
}

func (m model) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "tab":
		m.selected = nextIndex(m.selected, len(navItems))
		return m, nil
	case "shift+tab":
		m.selected = previousIndex(m.selected, len(navItems))
		return m, nil
	case "up", "k", "[":
		m.activeChat = previousIndex(m.activeChat, len(m.chats))
		return m, nil
	case "down", "j", "]":
		m.activeChat = nextIndex(m.activeChat, len(m.chats))
		return m, nil
	case "ctrl+n":
		m.addChatSession()
		return m, nil
	case "ctrl+t":
		m.cycleActiveChatTarget()
		return m, nil
	case "ctrl+u":
		m.chatInput = ""
		return m, nil
	case "backspace", "ctrl+h":
		m.chatInput = dropLastRune(m.chatInput)
		return m, nil
	case "enter":
		if m.selected != m.page {
			m.page = m.selected
			return m, nil
		}
		return m.sendActiveChatMessage()
	case " ":
		m.chatInput += " "
		return m, nil
	}
	if len(msg.Runes) > 0 {
		m.chatInput += string(msg.Runes)
	}
	return m, nil
}

func (m model) View() string {
	width := m.width
	if width == 0 {
		width = 100
	}
	contentWidth := maxInt(48, width-34)

	header := m.renderHeader(width - 4)
	sidebar := m.renderSidebar(25)
	content := m.styles.panel.Width(contentWidth).Render(m.renderPage())
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, "  ", content)
	footerText := "Navigate: up/down or tab  Select: enter  Quit: q"
	if m.page == pageChat {
		footerText = "Chat: type message, enter send, ctrl+n new, ctrl+t change target, up/down switch session, tab menu, q quit"
	} else if m.page == pageConfig {
		footerText = "Config: left/right section, up/down row, a add, e edit, d delete, space toggle, s save, r reload"
	}
	footer := m.styles.footer.Render(footerText)

	return m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer))
}

func (m model) renderHeader(width int) string {
	providers, models, groups, chats, keys := m.counts()
	left := m.styles.title.Render("ModelMux") + " " + m.styles.subtitle.Render("LLM key router")
	right := fmt.Sprintf("providers=%d  models=%d  groups=%d  chats=%d  keys=%d", providers, models, groups, chats, keys)
	line := left + strings.Repeat(" ", maxInt(1, width-lipgloss.Width(left)-lipgloss.Width(right))) + m.styles.subtitle.Render(right)
	return m.styles.header.Width(width).Render(line)
}

func (m model) renderSidebar(width int) string {
	var b strings.Builder
	b.WriteString(m.styles.muted.Render("SELECT MENU"))
	b.WriteString("\n\n")
	for i, item := range navItems {
		cursor := "  "
		style := m.styles.nav.Width(width - 4)
		if i == m.selected {
			cursor = "> "
			style = m.styles.navActive.Width(width - 4)
		}
		label := style.Render(cursor + item.label)
		b.WriteString(label)
		b.WriteString("\n")
		desc := "  " + item.description
		b.WriteString(m.styles.navMuted.Width(width - 2).Render(desc))
		if i < len(navItems)-1 {
			b.WriteString("\n\n")
		}
	}
	b.WriteString("\n\n")
	b.WriteString(m.styles.muted.Render("Active page: "))
	b.WriteString(navItems[m.page].label)
	return m.styles.sidebar.Width(width).Render(b.String())
}

func (m model) renderPage() string {
	title := m.styles.panelTitle.Render(navItems[m.page].label)
	var body string
	switch m.page {
	case pageModels:
		body = m.renderModels(m.cfg.Models)
	case pageGroups:
		if m.router != nil {
			body = m.renderDomainGroups(m.router.ListModelGroups())
		} else {
			body = m.renderConfigGroups(m.cfg.ModelGroups)
		}
	case pageChat:
		body = m.renderChat()
	case pageKeys:
		if m.router != nil {
			body = m.renderDomainKeys(m.router.ListKeys())
		} else {
			body = m.renderConfigKeys(m.cfg.Keys)
		}
	case pageLogs:
		body = m.renderLogs()
	case pageConfig:
		body = m.renderConfigEditor()
	case pageProviders:
		body = m.renderProviders(m.cfg.Providers)
	default:
		body = m.renderProviders(m.cfg.Providers)
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, m.styles.muted.Render(strings.Repeat("-", 72)), body)
}

func (m model) renderProviders(items []config.ProviderConfig) string {
	rows := [][]string{}
	for _, item := range items {
		rows = append(rows, []string{item.ID, item.Name, item.Type, truncate(item.BaseURL, 32), m.statusText(item.Enabled)})
	}
	return renderTable(m.styles, []string{"ID", "Name", "Type", "Base URL", "Status"}, rows, []int{16, 24, 20, 34, 10})
}

func (m model) renderModels(items []config.ModelConfig) string {
	rows := [][]string{}
	for _, item := range items {
		rows = append(rows, []string{item.ID, item.ProviderID, item.ModelName, defaultText(item.Strategy, "failover"), m.statusText(item.Enabled)})
	}
	return renderTable(m.styles, []string{"ID", "Provider", "Provider Model", "Strategy", "Status"}, rows, []int{22, 14, 28, 14, 10})
}

func (m model) renderConfigGroups(items []config.ModelGroupConfig) string {
	rows := [][]string{}
	for _, item := range items {
		rows = append(rows, []string{item.ID, defaultText(item.Strategy, "failover"), fmt.Sprint(len(item.Members)), m.statusText(item.Enabled)})
	}
	return renderTable(m.styles, []string{"ID", "Strategy", "Members", "Status"}, rows, []int{24, 16, 10, 10})
}

func (m model) renderDomainGroups(items []domain.ModelGroup) string {
	if len(items) == 0 {
		return m.emptyState("No model groups configured")
	}
	var b strings.Builder
	for _, item := range items {
		header := fmt.Sprintf("%s  strategy=%s  members=%d  %s", item.ID, item.Strategy, len(item.Members), m.statusText(item.Enabled))
		b.WriteString(m.styles.tableHead.Render(header))
		b.WriteString("\n")
		rows := [][]string{}
		for _, member := range item.Members {
			rows = append(rows, []string{member.ModelID, fmt.Sprint(member.Priority), fmt.Sprint(member.Weight), m.statusText(member.Enabled)})
		}
		b.WriteString(renderTable(m.styles, []string{"Model", "Priority", "Weight", "Status"}, rows, []int{28, 10, 8, 10}))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m model) renderConfigKeys(items []config.KeyConfig) string {
	rows := [][]string{}
	for _, item := range items {
		rows = append(rows, []string{item.ID, item.ModelID, defaultText(item.Status, "active"), fmt.Sprint(item.Priority)})
	}
	return renderTable(m.styles, []string{"ID", "Model", "Status", "Priority"}, rows, []int{24, 24, 12, 10})
}

func (m model) renderDomainKeys(items []domain.APIKey) string {
	rows := [][]string{}
	for _, item := range items {
		cooldown := "-"
		if item.CooldownEnd != nil && item.CooldownEnd.After(time.Now()) {
			cooldown = time.Until(*item.CooldownEnd).Round(time.Second).String()
		}
		rows = append(rows, []string{item.ID, item.ModelID, string(item.Status), fmt.Sprint(item.UsedCount), fmt.Sprint(item.ErrorCount), cooldown})
	}
	return renderTable(m.styles, []string{"ID", "Model", "Status", "Used", "Errors", "Cooldown"}, rows, []int{24, 24, 12, 8, 8, 12})
}

func (m model) renderChat() string {
	if len(m.chats) == 0 {
		return m.emptyState("No chat sessions")
	}
	active := m.chats[m.activeChat]
	left := m.renderChatSessionList(24)
	right := m.renderChatConversation(active)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m model) renderChatSessionList(width int) string {
	var b strings.Builder
	b.WriteString(m.styles.tableHead.Render("SESSIONS"))
	b.WriteString("\n\n")
	for i, chat := range m.chats {
		label := fmt.Sprintf("%d. %s", chat.ID, chat.Title)
		if chat.Pending {
			label += " ..."
		}
		style := m.styles.nav.Width(width - 4)
		prefix := "  "
		if i == m.activeChat {
			style = m.styles.navActive.Width(width - 4)
			prefix = "> "
		}
		b.WriteString(style.Render(prefix + truncate(label, width-6)))
		b.WriteString("\n")
		b.WriteString(m.styles.navMuted.Render("  target: " + truncate(chat.Target, width-12)))
		if i < len(m.chats)-1 {
			b.WriteString("\n\n")
		}
	}
	b.WriteString("\n\n")
	b.WriteString(m.styles.muted.Render("ctrl+n new\nctrl+t target\nup/down switch"))
	return m.styles.sidebar.Width(width).Render(b.String())
}

func (m model) renderChatConversation(chat tuiChatSession) string {
	var b strings.Builder
	header := fmt.Sprintf("%s  target=%s", chat.Title, chat.Target)
	if chat.Pending {
		header += "  sending..."
	}
	b.WriteString(m.styles.panelTitle.Render(header))
	b.WriteString("\n")
	b.WriteString(m.styles.muted.Render(strings.Repeat("-", 74)))
	b.WriteString("\n")

	if len(chat.Messages) == 0 {
		b.WriteString(m.styles.muted.Render("Start typing below, then press enter to send."))
		b.WriteString("\n")
	} else {
		start := 0
		if len(chat.Messages) > 12 {
			start = len(chat.Messages) - 12
		}
		for _, message := range chat.Messages[start:] {
			role := message.Role
			style := m.styles.muted
			if role == "user" {
				style = m.styles.good
			}
			if role == "assistant" {
				style = m.styles.tableHead
			}
			if role == "error" {
				style = m.styles.bad
			}
			b.WriteString(style.Render(strings.ToUpper(role)))
			b.WriteString(" ")
			b.WriteString(m.styles.muted.Render(message.CreatedAt.Format("15:04:05")))
			b.WriteString("\n")
			b.WriteString(wrapText(message.Content, 78))
			b.WriteString("\n\n")
		}
	}

	if chat.Error != "" {
		b.WriteString(m.styles.bad.Render("Error: " + chat.Error))
		b.WriteString("\n")
	}
	b.WriteString(m.styles.muted.Render(strings.Repeat("-", 74)))
	b.WriteString("\n")
	b.WriteString(m.styles.tableHead.Render("Input"))
	b.WriteString("\n")
	b.WriteString("> ")
	b.WriteString(defaultText(m.chatInput, m.styles.muted.Render("type message...")))
	return b.String()
}

func (m *model) addChatSession() {
	target := defaultChatTarget(m.cfg)
	m.chats = append(m.chats, newTUIChatSession(m.nextChatID, target))
	m.activeChat = len(m.chats) - 1
	m.nextChatID++
	m.chatInput = ""
}

func (m *model) cycleActiveChatTarget() {
	if len(m.chats) == 0 {
		return
	}
	targets := chatTargets(m.cfg)
	if len(targets) == 0 {
		return
	}
	current := m.chats[m.activeChat].Target
	idx := 0
	for i, target := range targets {
		if target == current {
			idx = (i + 1) % len(targets)
			break
		}
	}
	m.chats[m.activeChat].Target = targets[idx]
}

func (m model) sendActiveChatMessage() (tea.Model, tea.Cmd) {
	if len(m.chats) == 0 {
		return m, nil
	}
	input := strings.TrimSpace(m.chatInput)
	if input == "" || m.chats[m.activeChat].Pending {
		return m, nil
	}
	if m.router == nil {
		m.chats[m.activeChat].Messages = append(m.chats[m.activeChat].Messages, tuiChatMessage{Role: "error", Content: "router is not available", CreatedAt: time.Now()})
		m.chatInput = ""
		return m, nil
	}
	m.chats[m.activeChat].Messages = append(m.chats[m.activeChat].Messages, tuiChatMessage{Role: "user", Content: input, CreatedAt: time.Now()})
	m.chats[m.activeChat].Pending = true
	m.chats[m.activeChat].Error = ""
	m.chatInput = ""
	chat := m.chats[m.activeChat]
	return m, sendChatCmd(m.router, chat)
}

func (m *model) applyChatResponse(msg chatResponseMsg) {
	for i := range m.chats {
		if m.chats[i].ID != msg.sessionID {
			continue
		}
		m.chats[i].Pending = false
		if msg.err != nil {
			m.chats[i].Error = msg.err.Error()
			m.chats[i].Messages = append(m.chats[i].Messages, tuiChatMessage{Role: "error", Content: msg.err.Error(), CreatedAt: time.Now()})
			return
		}
		m.chats[i].Messages = append(m.chats[i].Messages, tuiChatMessage{Role: "assistant", Content: msg.content, CreatedAt: time.Now()})
		return
	}
}

func sendChatCmd(router inbound.RouterService, chat tuiChatSession) tea.Cmd {
	return func() tea.Msg {
		payload := struct {
			Model    string           `json:"model"`
			Messages []chatAPIMessage `json:"messages"`
		}{Model: chat.Target}
		for _, message := range chat.Messages {
			if message.Role != "user" && message.Role != "assistant" {
				continue
			}
			payload.Messages = append(payload.Messages, chatAPIMessage{Role: message.Role, Content: message.Content})
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return chatResponseMsg{sessionID: chat.ID, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://modelmux.local/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return chatResponseMsg{sessionID: chat.ID, err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := router.HandleChatCompletion(ctx, req)
		if err != nil {
			return chatResponseMsg{sessionID: chat.ID, err: err}
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return chatResponseMsg{sessionID: chat.ID, err: err}
		}
		if resp.StatusCode >= 400 {
			return chatResponseMsg{sessionID: chat.ID, err: fmt.Errorf("upstream returned %s: %s", resp.Status, truncate(string(respBody), 300))}
		}
		return chatResponseMsg{sessionID: chat.ID, content: extractAssistantText(respBody)}
	}
}

type chatAPIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func extractAssistantText(body []byte) string {
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && len(payload.Choices) > 0 {
		if payload.Choices[0].Message.Content != "" {
			return payload.Choices[0].Message.Content
		}
		if payload.Choices[0].Text != "" {
			return payload.Choices[0].Text
		}
	}
	return string(body)
}

func newTUIChatSession(id int, target string) tuiChatSession {
	return tuiChatSession{ID: id, Title: fmt.Sprintf("Chat %d", id), Target: target}
}

func defaultChatTarget(cfg *config.Config) string {
	targets := chatTargets(cfg)
	if len(targets) == 0 {
		return ""
	}
	return targets[0]
}

func chatTargets(cfg *config.Config) []string {
	var targets []string
	for _, group := range cfg.ModelGroups {
		if group.Enabled {
			targets = append(targets, group.ID)
		}
	}
	for _, model := range cfg.Models {
		if model.Enabled {
			targets = append(targets, model.ID)
		}
	}
	return targets
}

func dropLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes[:len(runes)-1])
}

func wrapText(text string, width int) string {
	if width <= 0 || lipgloss.Width(text) <= width {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}
	var b strings.Builder
	line := ""
	for _, word := range words {
		candidate := strings.TrimSpace(line + " " + word)
		if lipgloss.Width(candidate) > width && line != "" {
			b.WriteString(line)
			b.WriteString("\n")
			line = word
			continue
		}
		line = candidate
	}
	if line != "" {
		b.WriteString(line)
	}
	return b.String()
}

func (m model) renderLogs() string {
	if m.router == nil {
		return m.emptyState("No logs yet")
	}
	items := m.router.Logs()
	if len(items) == 0 {
		return m.emptyState("No logs yet")
	}
	start := 0
	if len(items) > 16 {
		start = len(items) - 16
	}
	rows := [][]string{}
	for _, item := range items[start:] {
		group := defaultText(item.GroupID, "-")
		errorText := defaultText(item.Error, "-")
		rows = append(rows, []string{item.CreatedAt.Format("15:04:05"), group, item.ModelID, item.KeyID, fmt.Sprint(item.StatusCode), fmt.Sprintf("%dms", item.LatencyMs), truncate(errorText, 28)})
	}
	return renderTable(m.styles, []string{"Time", "Group", "Model", "Key", "Status", "Latency", "Error"}, rows, []int{10, 18, 22, 22, 8, 10, 30})
}

func (m model) counts() (int, int, int, int, int) {
	providers := len(m.cfg.Providers)
	models := len(m.cfg.Models)
	groups := len(m.cfg.ModelGroups)
	chats := len(m.chats)
	keys := len(m.cfg.Keys)
	if m.router != nil {
		providers = len(m.router.ListProviders())
		models = len(m.router.ListModels())
		groups = len(m.router.ListModelGroups())
		keys = len(m.router.ListKeys())
	}
	return providers, models, groups, chats, keys
}

func (m model) statusText(enabled bool) string {
	if enabled {
		return m.styles.good.Render("active")
	}
	return m.styles.bad.Render("disabled")
}

func (m model) emptyState(text string) string {
	return m.styles.muted.Render(text)
}

func renderTable(styles styles, headers []string, rows [][]string, widths []int) string {
	if len(rows) == 0 {
		return styles.muted.Render("No data")
	}
	var b strings.Builder
	for i, header := range headers {
		b.WriteString(styles.tableHead.Render(padRight(truncate(header, widths[i]), widths[i])))
		if i < len(headers)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n")
	for i, width := range widths {
		b.WriteString(styles.muted.Render(strings.Repeat("-", width)))
		if i < len(widths)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n")
	for _, row := range rows {
		for i, cell := range row {
			b.WriteString(padRight(truncate(cell, widths[i]), widths[i]))
			if i < len(row)-1 {
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func previousIndex(current, length int) int {
	if length == 0 {
		return 0
	}
	if current <= 0 {
		return length - 1
	}
	return current - 1
}

func nextIndex(current, length int) int {
	if length == 0 {
		return 0
	}
	return (current + 1) % length
}

func padRight(text string, width int) string {
	padding := width - lipgloss.Width(text)
	if padding <= 0 {
		return text
	}
	return text + strings.Repeat(" ", padding)
}

func truncate(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	if width <= 1 {
		runes := []rune(text)
		if len(runes) == 0 {
			return text
		}
		return string(runes[:1])
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "~"
}

func defaultText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}
