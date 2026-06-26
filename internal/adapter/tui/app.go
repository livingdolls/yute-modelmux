package tui

import (
	"fmt"
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
	pageSessions
	pageKeys
	pageLogs
)

type navItem struct {
	label       string
	description string
}

var navItems = []navItem{
	{label: "Providers", description: "Upstream endpoints"},
	{label: "Models", description: "Routable models"},
	{label: "Groups", description: "Model aliases"},
	{label: "Sessions", description: "Chat routing"},
	{label: "Keys", description: "Live key state"},
	{label: "Logs", description: "Recent requests"},
}

type model struct {
	cfg      *config.Config
	router   inbound.RouterService
	selected int
	page     int
	width    int
	styles   styles
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

func Run(cfg *config.Config, router inbound.RouterService) error {
	m := model{cfg: cfg, router: router, styles: defaultStyles()}
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
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
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
	footer := m.styles.footer.Render("Navigate: up/down or tab  Select: enter  Quit: q")

	return m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer))
}

func (m model) renderHeader(width int) string {
	providers, models, groups, sessions, keys := m.counts()
	left := m.styles.title.Render("ModelMux") + " " + m.styles.subtitle.Render("LLM key router")
	right := fmt.Sprintf("providers=%d  models=%d  groups=%d  sessions=%d  keys=%d", providers, models, groups, sessions, keys)
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
	case pageSessions:
		body = m.renderConfigSessions(m.cfg.ChatSessions)
	case pageKeys:
		if m.router != nil {
			body = m.renderDomainKeys(m.router.ListKeys())
		} else {
			body = m.renderConfigKeys(m.cfg.Keys)
		}
	case pageLogs:
		body = m.renderLogs()
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

func (m model) renderConfigSessions(items []config.ChatSessionConfig) string {
	rows := [][]string{}
	for _, item := range items {
		rows = append(rows, []string{item.ID, item.Name, item.Target, m.statusText(item.Enabled)})
	}
	return renderTable(m.styles, []string{"ID", "Name", "Target", "Status"}, rows, []int{24, 24, 24, 10})
}

func (m model) renderDomainSessions(items []domain.ChatSession) string {
	rows := [][]string{}
	for _, item := range items {
		rows = append(rows, []string{item.ID, item.Name, item.Target, m.statusText(item.Enabled)})
	}
	return renderTable(m.styles, []string{"ID", "Name", "Target", "Status"}, rows, []int{24, 24, 24, 10})
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
		session := defaultText(item.SessionID, "-")
		group := defaultText(item.GroupID, "-")
		errorText := defaultText(item.Error, "-")
		rows = append(rows, []string{item.CreatedAt.Format("15:04:05"), session, group, item.ModelID, item.KeyID, fmt.Sprint(item.StatusCode), fmt.Sprintf("%dms", item.LatencyMs), truncate(errorText, 24)})
	}
	return renderTable(m.styles, []string{"Time", "Session", "Group", "Model", "Key", "Status", "Latency", "Error"}, rows, []int{10, 18, 18, 20, 20, 8, 10, 26})
}

func (m model) counts() (int, int, int, int, int) {
	providers := len(m.cfg.Providers)
	models := len(m.cfg.Models)
	groups := len(m.cfg.ModelGroups)
	sessions := len(m.cfg.ChatSessions)
	keys := len(m.cfg.Keys)
	if m.router != nil {
		providers = len(m.router.ListProviders())
		models = len(m.router.ListModels())
		groups = len(m.router.ListModelGroups())
		keys = len(m.router.ListKeys())
	}
	return providers, models, groups, sessions, keys
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
