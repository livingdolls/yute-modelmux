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

type model struct {
	cfg    *config.Config
	router inbound.RouterService
	page   int
	width  int
	height int
	styles styles
}

type styles struct {
	title  lipgloss.Style
	box    lipgloss.Style
	muted  lipgloss.Style
	accent lipgloss.Style
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
		title:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
		box:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
		muted:  lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		accent: lipgloss.NewStyle().Foreground(lipgloss.Color("86")),
	}
}

func (m model) Init() tea.Cmd { return tickCmd() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "1":
			m.page = 0
		case "2":
			m.page = 1
		case "3":
			m.page = 2
		case "4":
			m.page = 3
		case "5":
			m.page = 4
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		return m, tickCmd()
	}
	return m, nil
}

func (m model) View() string {
	head := m.styles.title.Render("ModelMux") + " " + m.styles.muted.Render("LLM key router")
	menu := "[1] Providers  [2] Models  [3] Groups  [4] Keys  [5] Logs  [q] Quit"
	content := m.renderPage()
	return lipgloss.JoinVertical(lipgloss.Left, head, m.styles.muted.Render(menu), "", content)
}

func (m model) renderPage() string {
	switch m.page {
	case 1:
		return m.styles.box.Render(renderModels(m.cfg.Models))
	case 2:
		if m.router != nil {
			return m.styles.box.Render(renderDomainGroups(m.router.ListModelGroups()))
		}
		return m.styles.box.Render(renderConfigGroups(m.cfg.ModelGroups))
	case 3:
		if m.router != nil {
			return m.styles.box.Render(renderDomainKeys(m.router.ListKeys()))
		}
		return m.styles.box.Render(renderConfigKeys(m.cfg.Keys))
	case 4:
		return m.styles.box.Render(renderLogs(m.router))
	default:
		return m.styles.box.Render(renderProviders(m.cfg.Providers))
	}
}

func renderProviders(items []config.ProviderConfig) string {
	var b strings.Builder
	b.WriteString("Providers\n\n")
	for _, item := range items {
		fmt.Fprintf(&b, "%s  %s  %s\n", item.ID, item.Name, boolText(item.Enabled))
	}
	return b.String()
}

func renderModels(items []config.ModelConfig) string {
	var b strings.Builder
	b.WriteString("Models\n\n")
	for _, item := range items {
		fmt.Fprintf(&b, "%s  provider=%s  strategy=%s\n", item.ID, item.ProviderID, item.Strategy)
	}
	return b.String()
}

func renderConfigGroups(items []config.ModelGroupConfig) string {
	var b strings.Builder
	b.WriteString("Groups\n\n")
	for _, item := range items {
		fmt.Fprintf(&b, "%s  strategy=%s  members=%d  %s\n", item.ID, item.Strategy, len(item.Members), boolText(item.Enabled))
	}
	return b.String()
}

func renderDomainGroups(items []domain.ModelGroup) string {
	var b strings.Builder
	b.WriteString("Groups\n\n")
	for _, item := range items {
		fmt.Fprintf(&b, "%s  strategy=%s  members=%d  %s\n", item.ID, item.Strategy, len(item.Members), boolText(item.Enabled))
		for _, member := range item.Members {
			fmt.Fprintf(&b, "  - %s  priority=%d  weight=%d  %s\n", member.ModelID, member.Priority, member.Weight, boolText(member.Enabled))
		}
	}
	return b.String()
}

func renderConfigKeys(items []config.KeyConfig) string {
	var b strings.Builder
	b.WriteString("Keys\n\n")
	for _, item := range items {
		fmt.Fprintf(&b, "%s  model=%s  status=%s  priority=%d\n", item.ID, item.ModelID, item.Status, item.Priority)
	}
	return b.String()
}

func renderDomainKeys(items []domain.APIKey) string {
	var b strings.Builder
	b.WriteString("Keys\n\n")
	for _, item := range items {
		cooldown := "-"
		if item.CooldownEnd != nil && item.CooldownEnd.After(time.Now()) {
			cooldown = time.Until(*item.CooldownEnd).Round(time.Second).String()
		}
		fmt.Fprintf(&b, "%s  model=%s  status=%s  used=%d  errors=%d  cooldown=%s\n", item.ID, item.ModelID, item.Status, item.UsedCount, item.ErrorCount, cooldown)
	}
	return b.String()
}

func renderLogs(router inbound.RouterService) string {
	var b strings.Builder
	b.WriteString("Logs\n\n")
	if router == nil {
		b.WriteString("No logs yet\n")
		return b.String()
	}
	items := router.Logs()
	if len(items) == 0 {
		b.WriteString("No logs yet\n")
		return b.String()
	}
	for _, item := range items {
		if item.GroupID != "" {
			fmt.Fprintf(&b, "%s group=%s model=%s key=%s status=%d error=%s\n", item.CreatedAt.Format("15:04:05"), item.GroupID, item.ModelID, item.KeyID, item.StatusCode, item.Error)
			continue
		}
		fmt.Fprintf(&b, "%s model=%s key=%s status=%d error=%s\n", item.CreatedAt.Format("15:04:05"), item.ModelID, item.KeyID, item.StatusCode, item.Error)
	}
	return b.String()
}

func boolText(v bool) string {
	if v {
		return "active"
	}
	return "disabled"
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}
