package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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
	{label: "Providers", description: "upstream routes"},
	{label: "Models", description: "routable models"},
	{label: "Groups", description: "alias bundles"},
	{label: "Chat", description: "prompt console"},
	{label: "Keys", description: "live key state"},
	{label: "Logs", description: "recent traffic"},
	{label: "Config", description: "edit router"},
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
	cfg              *config.Config
	savedCfg         *config.Config
	configPath       string
	router           inbound.RouterService
	saveConfig       func(*config.Config) error
	reloadRouter     func(*config.Config) (inbound.RouterService, error)
	selected         int
	page             int
	contentFocused   bool
	width            int
	styles           styles
	editor           configEditorState
	chats            []tuiChatSession
	activeChat       int
	chatAnchor       int
	chatInput        string
	chatFilter       string
	chatFiltering    bool
	nextChatID       int
	startedAt        time.Time
	logsFilter       string
	logsSort         string
	keysSort         string
	theme            string
	showHelp         bool
	showThemePicker  bool
	themePickerIndex int
	keyTesting       bool
	keyTestRunning   bool
	keyTestInput     string
	keyTestResult    string
}

type keyTestResultMsg struct {
	keyID string
	err   error
}

type styles struct {
	app        lipgloss.Style
	header     lipgloss.Style
	logo       lipgloss.Style
	title      lipgloss.Style
	subtitle   lipgloss.Style
	section    lipgloss.Style
	badge      lipgloss.Style
	badgeSoft  lipgloss.Style
	badgeWarn  lipgloss.Style
	hint       lipgloss.Style
	statusBar  lipgloss.Style
	splashBox  lipgloss.Style
	sidebar    lipgloss.Style
	nav        lipgloss.Style
	navActive  lipgloss.Style
	navMuted   lipgloss.Style
	panel      lipgloss.Style
	panelTitle lipgloss.Style
	tableFrame lipgloss.Style
	tableHead  lipgloss.Style
	rowEven    lipgloss.Style
	rowOdd     lipgloss.Style
	muted      lipgloss.Style
	good       lipgloss.Style
	bad        lipgloss.Style
	chatMeta   lipgloss.Style
	chatUser   lipgloss.Style
	chatBot    lipgloss.Style
	chatError  lipgloss.Style
	input      lipgloss.Style
	card       lipgloss.Style
	footer     lipgloss.Style
}

type tickMsg time.Time

type chatResponseMsg struct {
	sessionID int
	content   string
	err       error
}

type themePalette struct {
	border        lipgloss.Color
	logo          lipgloss.Color
	title         lipgloss.Color
	subtitle      lipgloss.Color
	section       lipgloss.Color
	badgeBg       lipgloss.Color
	badgeSoftBg   lipgloss.Color
	badgeWarnBg   lipgloss.Color
	hint          lipgloss.Color
	statusBg      lipgloss.Color
	panelTitle    lipgloss.Color
	muted         lipgloss.Color
	good          lipgloss.Color
	bad           lipgloss.Color
	chatBotBorder lipgloss.Color
	chatUserBg    lipgloss.Color
	chatBotBg     lipgloss.Color
	inputBg       lipgloss.Color
	oddRowBg      lipgloss.Color
}

var themeOrder = []string{"blue", "amber", "mint", "violet", "rose", "mono", "matrix"}

const defaultTheme = "blue"

func Run(options Options) error {
	draft := cloneConfig(options.Config)
	m := model{
		cfg:          draft,
		savedCfg:     cloneConfig(options.Config),
		configPath:   options.ConfigPath,
		router:       options.Router,
		saveConfig:   options.SaveConfig,
		reloadRouter: options.ReloadRouter,
		theme:        defaultTheme,
		styles:       defaultStyles(defaultTheme),
		nextChatID:   2,
		startedAt:    time.Now(),
		logsFilter:   "latest",
		logsSort:     "newest",
		keysSort:     "status",
		editor:       newConfigEditorState(draft),
	}
	m.chats = []tuiChatSession{newTUIChatSession(1, defaultChatTarget(draft))}
	prog := tea.NewProgram(m, tea.WithAltScreen())
	_, err := prog.Run()
	return err
}

func defaultStyles(theme string) styles {
	p := paletteForTheme(theme)
	return styles{
		app:        lipgloss.NewStyle().Padding(0, 1),
		header:     lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(p.border),
		logo:       lipgloss.NewStyle().Bold(true).Foreground(p.logo),
		title:      lipgloss.NewStyle().Bold(true).Foreground(p.title),
		subtitle:   lipgloss.NewStyle().Foreground(p.subtitle),
		section:    lipgloss.NewStyle().Bold(true).Foreground(p.section),
		badge:      lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(p.badgeBg).Padding(0, 1),
		badgeSoft:  lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(p.badgeSoftBg).Padding(0, 1),
		badgeWarn:  lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(p.badgeWarnBg).Padding(0, 1),
		hint:       lipgloss.NewStyle().Foreground(p.hint),
		statusBar:  lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(p.statusBg).Padding(0, 1),
		splashBox:  lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(p.section).Padding(1, 2),
		sidebar:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.border).Padding(0, 1),
		nav:        lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Padding(0, 1),
		navActive:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(p.border).Padding(0, 1),
		navMuted:   lipgloss.NewStyle().Foreground(p.muted),
		panel:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.border).Padding(0, 1),
		panelTitle: lipgloss.NewStyle().Bold(true).Foreground(p.panelTitle),
		tableFrame: lipgloss.NewStyle().Foreground(p.border),
		tableHead:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(p.badgeBg),
		rowEven:    lipgloss.NewStyle(),
		rowOdd:     lipgloss.NewStyle().Background(p.oddRowBg),
		muted:      lipgloss.NewStyle().Foreground(p.muted),
		good:       lipgloss.NewStyle().Foreground(p.good),
		bad:        lipgloss.NewStyle().Foreground(p.bad),
		chatMeta:   lipgloss.NewStyle().Foreground(p.hint),
		chatUser:   lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(p.chatUserBg).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(p.border),
		chatBot:    lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(p.chatBotBg).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(p.chatBotBorder),
		chatError:  lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("52")).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("203")),
		input:      lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(p.inputBg).Padding(0, 1),
		card:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.border).Padding(0, 1),
		footer:     lipgloss.NewStyle().Foreground(p.subtitle).Padding(0, 1).Border(lipgloss.NormalBorder(), true, false, false, false).BorderForeground(p.border),
	}
}

func paletteForTheme(theme string) themePalette {
	c := func(value string) lipgloss.Color { return lipgloss.Color(value) }
	switch theme {
	case "amber":
		return themePalette{border: c("136"), logo: c("214"), title: c("222"), subtitle: c("180"), section: c("214"), badgeBg: c("94"), badgeSoftBg: c("236"), badgeWarnBg: c("130"), hint: c("179"), statusBg: c("236"), panelTitle: c("221"), muted: c("246"), good: c("150"), bad: c("203"), chatBotBorder: c("136"), chatUserBg: c("94"), chatBotBg: c("236"), inputBg: c("237"), oddRowBg: c("235")}
	case "mint":
		return themePalette{border: c("43"), logo: c("121"), title: c("158"), subtitle: c("115"), section: c("86"), badgeBg: c("29"), badgeSoftBg: c("238"), badgeWarnBg: c("71"), hint: c("108"), statusBg: c("235"), panelTitle: c("157"), muted: c("244"), good: c("121"), bad: c("203"), chatBotBorder: c("79"), chatUserBg: c("29"), chatBotBg: c("236"), inputBg: c("236"), oddRowBg: c("235")}
	case "violet":
		return themePalette{border: c("99"), logo: c("177"), title: c("183"), subtitle: c("146"), section: c("141"), badgeBg: c("55"), badgeSoftBg: c("238"), badgeWarnBg: c("98"), hint: c("147"), statusBg: c("236"), panelTitle: c("219"), muted: c("244"), good: c("114"), bad: c("203"), chatBotBorder: c("141"), chatUserBg: c("55"), chatBotBg: c("236"), inputBg: c("236"), oddRowBg: c("235")}
	case "rose":
		return themePalette{border: c("168"), logo: c("204"), title: c("217"), subtitle: c("175"), section: c("211"), badgeBg: c("89"), badgeSoftBg: c("238"), badgeWarnBg: c("167"), hint: c("181"), statusBg: c("236"), panelTitle: c("218"), muted: c("245"), good: c("150"), bad: c("203"), chatBotBorder: c("168"), chatUserBg: c("89"), chatBotBg: c("236"), inputBg: c("236"), oddRowBg: c("235")}
	case "mono":
		return themePalette{border: c("245"), logo: c("255"), title: c("255"), subtitle: c("250"), section: c("252"), badgeBg: c("240"), badgeSoftBg: c("237"), badgeWarnBg: c("243"), hint: c("248"), statusBg: c("236"), panelTitle: c("255"), muted: c("246"), good: c("255"), bad: c("255"), chatBotBorder: c("245"), chatUserBg: c("240"), chatBotBg: c("236"), inputBg: c("237"), oddRowBg: c("235")}
	case "matrix":
		return themePalette{border: c("28"), logo: c("46"), title: c("46"), subtitle: c("40"), section: c("41"), badgeBg: c("22"), badgeSoftBg: c("236"), badgeWarnBg: c("28"), hint: c("34"), statusBg: c("234"), panelTitle: c("47"), muted: c("240"), good: c("46"), bad: c("82"), chatBotBorder: c("34"), chatUserBg: c("22"), chatBotBg: c("234"), inputBg: c("234"), oddRowBg: c("233")}
	default:
		return themePalette{border: c("31"), logo: c("51"), title: c("87"), subtitle: c("109"), section: c("45"), badgeBg: c("24"), badgeSoftBg: c("238"), badgeWarnBg: c("166"), hint: c("110"), statusBg: c("235"), panelTitle: c("219"), muted: c("244"), good: c("84"), bad: c("203"), chatBotBorder: c("63"), chatUserBg: c("24"), chatBotBg: c("236"), inputBg: c("236"), oddRowBg: c("235")}
	}
}

func (m model) Init() tea.Cmd { return tickCmd() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.showHelp {
			if key == "?" || key == "esc" {
				m.showHelp = false
			}
			return m, nil
		}
		if m.showThemePicker {
			return m.updateThemePicker(msg)
		}
		if m.keyTesting {
			return m.updateKeyTest(msg)
		}
		if key == "?" && !m.isTextEntryMode() {
			m.showHelp = !m.showHelp
			return m, nil
		}
		if key == "T" && !m.isTextEntryMode() {
			m.openThemePicker()
			return m, nil
		}
		if key == "t" && !m.isTextEntryMode() {
			m.toggleTheme()
			return m, nil
		}
		if key == "q" && !m.isTextEntryMode() {
			return m, tea.Quit
		}
		if key == "esc" && m.contentFocused && !m.shouldHandleEscInContent() {
			m.contentFocused = false
			m.selected = m.page
			return m, nil
		}
		if m.contentFocused && m.page == pageChat {
			return m.updateChat(msg)
		}
		if m.contentFocused && m.page == pageConfig {
			return m.updateConfigEditor(msg)
		}
		if m.contentFocused && m.page == pageKeys && m.applyKeysSortKey(key) {
			return m, nil
		}
		if m.contentFocused && m.page == pageLogs && m.applyLogFilterKey(key) {
			return m, nil
		}
		if m.contentFocused && m.page == pageKeys && key == "x" {
			m.keyTesting = true
			m.keyTestInput = ""
			m.keyTestResult = ""
			return m, nil
		}
		switch key {
		case "up", "shift+tab":
			m.selected = previousIndex(m.selected, len(navItems))
		case "down", "tab":
			m.selected = nextIndex(m.selected, len(navItems))
		case "enter", " ":
			m.page = m.selected
			m.contentFocused = true
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tickMsg:
		return m, tickCmd()
	case chatResponseMsg:
		m.applyChatResponse(msg)
	case keyTestResultMsg:
		m.keyTestRunning = false
		m.keyTesting = false
		m.keyTestInput = ""
		if msg.err != nil {
			m.keyTestResult = "key " + msg.keyID + " FAIL: " + msg.err.Error()
		} else {
			m.keyTestResult = "key " + msg.keyID + " OK"
		}
	}
	return m, nil
}

func (m model) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.chatFiltering {
		return m.updateChatFilter(msg)
	}
	switch key {
	case "tab":
		m.selected = nextIndex(m.selected, len(navItems))
		return m, nil
	case "shift+tab":
		m.selected = previousIndex(m.selected, len(navItems))
		return m, nil
	case "up":
		m.moveActiveChat(-1)
		return m, nil
	case "down":
		m.moveActiveChat(1)
		return m, nil
	case "ctrl+n":
		m.addChatSession()
		return m, nil
	case "ctrl+t":
		m.cycleActiveChatTarget()
		return m, nil
	case "ctrl+f":
		m.chatFiltering = true
		return m, nil
	case "esc":
		m.chatInput = ""
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

func (m model) updateChatFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "enter", "ctrl+f":
		m.chatFiltering = false
		m.ensureActiveChatVisible()
		return m, nil
	case "backspace", "ctrl+h":
		m.chatFilter = dropLastRune(m.chatFilter)
		m.ensureActiveChatVisible()
		return m, nil
	case "ctrl+u":
		m.chatFilter = ""
		m.ensureActiveChatVisible()
		return m, nil
	}
	if len(msg.Runes) > 0 {
		m.chatFilter += string(msg.Runes)
		m.ensureActiveChatVisible()
	}
	return m, nil
}

func (m model) View() string {
	width := m.width
	if width == 0 {
		width = 100
	}
	if m.showSplash() {
		return m.renderSplash(width)
	}
	sidebarWidth := m.sidebarWidth()
	contentWidth := m.pageBodyWidth()

	header := m.renderHeader(width - 2)
	statusBar := m.renderStatusBar(width - 2)
	sidebar := m.renderSidebar(sidebarWidth)
	content := m.styles.panel.Width(contentWidth).Render(m.renderPage())
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, strings.Repeat(" ", m.bodyGap()), content)
	footerText := "MENU tab/shift+tab:move  enter:open  ?:help  q:quit"
	if m.contentFocused && m.page == pageChat {
		footerText = "CHAT enter:send  up/down:session  ctrl+n:new  ctrl+t:target  ctrl+f:filter  esc:menu"
	} else if m.contentFocused && m.page == pageKeys {
		footerText = "KEYS 1:status  2:cooldown  3:errors  x:test  esc:menu  ?:help"
		if m.keyTesting {
			footerText = fmt.Sprintf("TEST KEY ID: %s", m.keyTestInput)
		} else if m.keyTestResult != "" {
			footerText = m.keyTestResult
		}
	} else if m.contentFocused && m.page == pageLogs {
		footerText = "LOGS 1:latest  2:errors  3:slow  4:rate-limit  5:newest  6:slowest  esc:menu"
	} else if m.contentFocused && m.page == pageConfig {
		footerText = "CFG up/down:row  left/right:section  enter:edit  ctrl+s:save  esc:menu"
	}
	footerText += "  AUTO:" + strings.ToUpper(m.layoutMode()) + "  THEME:" + strings.ToUpper(m.theme)
	footer := m.styles.footer.Width(width - 2).Render(footerText)

	base := m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, header, statusBar, body, footer))
	if m.showHelp {
		return m.renderHelpOverlay(width, base)
	}
	if m.showThemePicker {
		return m.renderThemePicker(width, base)
	}
	return base
}

func (m model) renderHeader(width int) string {
	providers, models, groups, chats, keys := m.counts()
	brand := m.renderBrand(width)
	stats := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderStatBadge("PROV", providers),
		" ",
		m.renderStatBadge("MOD", models),
		" ",
		m.renderStatBadge("GRP", groups),
		" ",
		m.renderStatBadge("CHAT", chats),
		" ",
		m.renderStatBadge("KEY", keys),
	)
	if width < 90 {
		body := lipgloss.JoinVertical(lipgloss.Left, brand, m.styles.subtitle.Render("llm key router // local failover dashboard"), stats)
		return m.styles.header.Width(width).Render(body)
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, brand, "  ", lipgloss.JoinVertical(lipgloss.Left, m.styles.subtitle.Render("llm key router // local failover dashboard"), "", stats))
	return m.styles.header.Width(width).Render(body)
}

func (m model) renderSidebar(width int) string {
	var b strings.Builder
	b.WriteString(m.styles.section.Render(".:: MENU ::."))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("router control deck"))
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
		b.WriteString(m.styles.navMuted.Render("   " + item.description))
		if i < len(navItems)-1 {
			b.WriteString("\n\n")
		}
	}
	b.WriteString("\n\n")
	b.WriteString(m.styles.section.Render(".:: ACTIVE ::."))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render(pageGlyph(m.page) + " " + navItems[m.page].label + " // " + navItems[m.page].description))
	b.WriteString("\n\n")
	b.WriteString(m.styles.section.Render(".:: HEALTH ::."))
	b.WriteString("\n")
	b.WriteString(m.renderSidebarHealth(width - 2))
	b.WriteString("\n\n")
	if m.cfg.Server.Host == "0.0.0.0" && !m.cfg.Server.RequireAuth {
		b.WriteString(m.styles.section.Render(".:: SECURITY ::."))
		b.WriteString("\n")
		b.WriteString(m.styles.bad.Render("Server bound to 0.0.0.0 without authentication. Enable server.require_auth!"))
		b.WriteString("\n\n")
	}
	b.WriteString(m.styles.section.Render(".:: KEYS ::."))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("tab move"))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("enter open"))
	return m.styles.sidebar.Width(width).Render(b.String())
}

func (m model) renderPage() string {
	title := m.styles.panelTitle.Render(pageGlyph(m.page) + " :: " + strings.ToUpper(navItems[m.page].label) + " ::")
	subtitle := m.styles.subtitle.Render(m.pageSubtitle())
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
	return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, m.styles.muted.Render(strings.Repeat("-", 58)), body)
}

func (m *model) toggleTheme() {
	idx := 0
	for i, theme := range themeOrder {
		if theme == m.theme {
			idx = i
			break
		}
	}
	m.theme = themeOrder[(idx+1)%len(themeOrder)]
	m.styles = defaultStyles(m.theme)
}

func (m *model) openThemePicker() {
	m.showThemePicker = true
	m.themePickerIndex = themeIndex(m.theme)
}

func themeIndex(theme string) int {
	for i, item := range themeOrder {
		if item == theme {
			return i
		}
	}
	return 0
}

func (m model) updateThemePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "T":
		m.showThemePicker = false
		return m, nil
	case "enter", " ":
		m.theme = themeOrder[m.themePickerIndex]
		m.styles = defaultStyles(m.theme)
		m.showThemePicker = false
		return m, nil
	case "up":
		m.themePickerIndex = previousIndex(m.themePickerIndex, len(themeOrder))
		return m, nil
	case "down":
		m.themePickerIndex = nextIndex(m.themePickerIndex, len(themeOrder))
		return m, nil
	}
	return m, nil
}

func (m model) updateKeyTest(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.keyTesting = false
		m.keyTestRunning = false
		m.keyTestInput = ""
		return m, nil
	case "backspace", "ctrl+h":
		if m.keyTestRunning {
			return m, nil
		}
		m.keyTestInput = dropLastRune(m.keyTestInput)
		return m, nil
	case "enter":
		if m.keyTestRunning {
			return m, nil
		}
		if strings.TrimSpace(m.keyTestInput) == "" {
			m.keyTesting = false
			return m, nil
		}
		if m.router == nil {
			m.keyTestResult = "error: no router"
			m.keyTesting = false
			m.keyTestInput = ""
			return m, nil
		}
		keyID := strings.TrimSpace(m.keyTestInput)
		m.keyTestRunning = true
		m.keyTestResult = "testing key " + keyID + "..."
		return m, keyTestCmd(m.router, keyID)
	}
	if !m.keyTestRunning && len(msg.Runes) > 0 {
		m.keyTestInput += string(msg.Runes)
	}
	return m, nil
}

func keyTestCmd(router inbound.RouterService, keyID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return keyTestResultMsg{keyID: keyID, err: router.TestKey(ctx, keyID)}
	}
}

func pageGlyph(page int) string {
	switch page {
	case pageProviders:
		return "[P]"
	case pageModels:
		return "[M]"
	case pageGroups:
		return "[G]"
	case pageChat:
		return "[C]"
	case pageKeys:
		return "[K]"
	case pageLogs:
		return "[L]"
	case pageConfig:
		return "[CFG]"
	default:
		return "[*]"
	}
}

func (m model) renderHelpOverlay(width int, base string) string {
	_ = base
	boxWidth := minInt(maxInt(58, width-14), 92)
	help := m.styles.card.Width(boxWidth).Render(lipgloss.JoinVertical(lipgloss.Left,
		m.styles.panelTitle.Render("HELP // "+strings.ToUpper(navItems[m.page].label)),
		m.styles.subtitle.Render("press ? to close"),
		m.styles.muted.Render(strings.Repeat("-", boxWidth-4)),
		m.renderHelpText(),
	))
	return m.styles.app.Render(lipgloss.Place(width-2, 22, lipgloss.Center, lipgloss.Center, help))
}

func (m model) renderThemePicker(width int, base string) string {
	_ = base
	boxWidth := minInt(maxInt(42, width-18), 68)
	var items []string
	for i, theme := range themeOrder {
		line := fmt.Sprintf("%d. %s", i+1, strings.ToUpper(theme))
		if i == m.themePickerIndex {
			line = m.styles.navActive.Render(line)
		} else if theme == m.theme {
			line = m.styles.nav.Render(line + "  (current)")
		} else {
			line = m.styles.nav.Render(line)
		}
		items = append(items, line)
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.panelTitle.Render("THEME PICKER"),
		m.styles.subtitle.Render("up/down choose, enter apply, esc cancel"),
		m.styles.muted.Render(strings.Repeat("-", boxWidth-4)),
		strings.Join(items, "\n"),
	)
	box := m.styles.card.Width(boxWidth).Render(content)
	return m.styles.app.Render(lipgloss.Place(width-2, 18, lipgloss.Center, lipgloss.Center, box))
}

func (m model) renderHelpText() string {
	lines := []string{
		"global: esc back/cancel, q quit, ctrl+c force quit, ? toggle help",
		"global: t cycle theme, T open picker when not typing",
		"themes: " + strings.Join(themeOrder, ", "),
		"menu: tab/shift+tab move, enter open selected page",
	}
	switch m.page {
	case pageChat:
		lines = append(lines,
			"chat: enter send, ctrl+n new session, ctrl+t cycle target",
			"chat: up/down change active session, ctrl+f filter sessions",
			"chat: esc clears typed text; esc again returns to menu",
		)
	case pageKeys:
		lines = append(lines,
			"keys: 1 sort by status, 2 by cooldown, 3 by errors",
		)
	case pageLogs:
		lines = append(lines,
			"logs: 1 latest, 2 errors, 3 slow, 4 rate-limit, 5 newest, 6 slowest",
		)
	case pageConfig:
		lines = append(lines,
			"config: enter edit, a add, delete remove, ctrl+s save, ctrl+r reload",
			"config: left/right switch section, up/down move row, tab move menu",
			"config: esc cancels forms/filters or returns to menu",
		)
	}
	return strings.Join(lines, "\n")
}

func (m model) renderBrand(width int) string {
	if width < 70 {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			m.styles.logo.Render("MODELMUX"),
			m.styles.title.Render("retro router deck"),
		)
	}
	logo := strings.Join([]string{
		" __  __           _      __  __            ",
		"|  \\/  | ___   __| | ___|  \\/  |_   ___  __",
		"| |\\/| |/ _ \\ / _` |/ _ \\ |\\/| | | | \\/ /",
		"| |  | | (_) | (_| |  __/ |  | | |_| |>  < ",
		"|_|  |_|\\___/ \\__,_|\\___|_|  |_|\\__,_/_/\\_\\",
	}, "\n")
	return m.styles.logo.Render(logo)
}

func (m model) renderStatBadge(label string, value int) string {
	return m.styles.badge.Render(fmt.Sprintf("%s %d", label, value))
}

func (m model) pageSubtitle() string {
	switch m.page {
	case pageProviders:
		return "Upstream endpoints, auth mode, and routing status."
	case pageModels:
		return "Model ids exposed by the router and their failover strategy."
	case pageGroups:
		return "Alias groups that bundle models for fallback and balancing."
	case pageChat:
		return "Prompt directly through the local router without leaving the terminal."
	case pageKeys:
		return "Live key counters, cooldown windows, and error state."
	case pageLogs:
		return "Recent request trail as a live activity feed with health hints."
	case pageConfig:
		return "Edit providers, models, groups, and keys from one control surface."
	default:
		return "Local router dashboard."
	}
}

func (m model) renderSidebarHealth(width int) string {
	active, cooldown, limited, invalid, disabled := m.keyStatusCounts()
	var b strings.Builder
	b.WriteString(m.routerHealthText())
	b.WriteString("\n")
	b.WriteString(m.styles.navMuted.Render(fmt.Sprintf("a:%d c:%d l:%d x:%d o:%d", active, cooldown, limited, invalid, disabled)))
	if summary := m.lastRequestSummary(); summary != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.hint.Render(truncate(summary, maxInt(10, width))))
	}
	return b.String()
}

func (m model) routerHealthText() string {
	if m.router == nil {
		return m.styles.bad.Render("offline")
	}
	_, cooldown, limited, invalid, _ := m.keyStatusCounts()
	if invalid > 0 || limited > 0 || cooldown > 0 {
		return m.styles.bad.Render("degraded")
	}
	return m.styles.good.Render("healthy")
}

func (m model) layoutMode() string {
	width := m.width
	if width == 0 {
		width = 100
	}
	if width >= 132 {
		return "wide"
	}
	return "compact"
}

func (m model) sidebarWidth() int {
	if m.layoutMode() == "wide" {
		return 31
	}
	return 26
}

func (m model) chatSidebarWidth() int {
	if m.layoutMode() == "wide" {
		return 29
	}
	return 24
}

func (m model) bodyGap() int {
	if m.layoutMode() == "wide" {
		return 2
	}
	return 1
}

func (m model) pageBodyWidth() int {
	width := m.width
	if width == 0 {
		width = 100
	}
	return maxInt(48, width-m.sidebarWidth()-m.bodyGap()-5)
}

func (m model) renderCardGrid(cards []string) string {
	if len(cards) == 0 {
		return m.emptyState("No data")
	}
	if m.layoutMode() != "wide" || len(cards) == 1 {
		return strings.Join(cards, "\n\n")
	}
	gap := strings.Repeat(" ", m.bodyGap())
	cardWidth := maxInt(20, (m.pageBodyWidth()-m.bodyGap())/2)
	rows := make([]string, 0, (len(cards)+1)/2)
	for i := 0; i < len(cards); i += 2 {
		left := lipgloss.NewStyle().Width(cardWidth).Render(cards[i])
		if i+1 >= len(cards) {
			rows = append(rows, left)
			continue
		}
		right := lipgloss.NewStyle().Width(cardWidth).Render(cards[i+1])
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right))
	}
	return strings.Join(rows, "\n\n")
}

func (m model) renderProviderCard(item config.ProviderConfig) string {
	status := m.styles.badgeSoft.Render("DISABLED")
	if item.Enabled {
		status = m.styles.badge.Render("ENABLED")
	}
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, m.styles.panelTitle.Render(defaultText(item.Name, item.ID)), " ", status),
		m.styles.navMuted.Render("id="+item.ID),
		m.styles.badgeSoft.Render(strings.ToUpper(defaultText(item.Type, "unknown"))),
		m.styles.hint.Render("base  "+truncate(defaultText(item.BaseURL, "-"), maxInt(16, m.pageBodyWidth()/2))),
		m.styles.hint.Render("auth  "+defaultText(item.AuthType, "bearer")),
		m.styles.hint.Render(fmt.Sprintf("timeout %ds", defaultInt(item.TimeoutSeconds, 120))),
	)
	return m.styles.card.Render(content)
}

func (m model) renderModelCard(item config.ModelConfig) string {
	status := m.styles.badgeSoft.Render("DISABLED")
	if item.Enabled {
		status = m.styles.badge.Render("ENABLED")
	}
	strategy := strings.ToUpper(defaultText(item.Strategy, "failover"))
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, m.styles.panelTitle.Render(item.ID), " ", status),
		m.styles.navMuted.Render("provider="+defaultText(item.ProviderID, "-")),
		m.styles.badgeSoft.Render(strategy),
		m.styles.hint.Render("upstream  "+truncate(defaultText(item.ModelName, "-"), maxInt(16, m.pageBodyWidth()/2))),
	)
	return m.styles.card.Render(content)
}

func (m model) renderConfigGroupCard(item config.ModelGroupConfig) string {
	status := m.styles.badgeSoft.Render("DISABLED")
	if item.Enabled {
		status = m.styles.badge.Render("ENABLED")
	}
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, m.styles.panelTitle.Render(defaultText(item.Name, item.ID)), " ", status),
		m.styles.navMuted.Render("id="+item.ID),
		m.styles.badgeSoft.Render(strings.ToUpper(defaultText(item.Strategy, "failover"))),
		m.styles.hint.Render(fmt.Sprintf("members  %d", len(item.Members))),
		m.styles.hint.Render("models   "+truncate(groupMembersText(item.Members), maxInt(16, m.pageBodyWidth()/2))),
	)
	return m.styles.card.Render(content)
}

func (m model) renderDomainGroupCard(item domain.ModelGroup) string {
	status := m.styles.badgeSoft.Render("DISABLED")
	if item.Enabled {
		status = m.styles.badge.Render("ENABLED")
	}
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, m.styles.panelTitle.Render(defaultText(item.Name, item.ID)), " ", status),
		m.styles.navMuted.Render("id="+item.ID),
		m.styles.badgeSoft.Render(strings.ToUpper(string(item.Strategy))),
		m.styles.hint.Render(fmt.Sprintf("members  %d", len(item.Members))),
		m.styles.hint.Render("models   "+truncate(domainGroupMembersText(item.Members), maxInt(16, m.pageBodyWidth()/2))),
	)
	return m.styles.card.Render(content)
}

func (m model) showSplash() bool {
	return time.Since(m.startedAt) < 1200*time.Millisecond
}

func (m model) renderSplash(width int) string {
	elapsed := time.Since(m.startedAt)
	frames := []string{"[    ]", "[=   ]", "[==  ]", "[=== ]", "[====]"}
	stages := []string{"load config", "sync routes", "warm status bus", "prime ui", "ready"}
	idx := minInt(len(frames)-1, int(elapsed/(250*time.Millisecond)))
	logo := m.renderBrand(width)
	lines := []string{
		m.styles.subtitle.Render("booting local failover deck"),
		"",
		lipgloss.JoinHorizontal(lipgloss.Top, m.styles.badge.Render("BOOT"), " ", m.styles.badgeSoft.Render(strings.ToUpper(stages[idx]))),
		m.styles.hint.Render(frames[idx] + "  spin up sequence"),
		"",
		m.renderSplashTimeline(idx),
		"",
		m.styles.hint.Render("press ctrl+c to abort startup"),
	}
	box := m.styles.splashBox.Render(lipgloss.JoinVertical(lipgloss.Left, append([]string{logo}, lines...)...))
	height := 14
	if m.width > 0 {
		height = 18
	}
	return m.styles.app.Render(lipgloss.Place(width-2, height, lipgloss.Center, lipgloss.Center, box))
}

func (m model) renderSplashTimeline(active int) string {
	stages := []string{"CONFIG", "ROUTES", "STATUS", "UI"}
	parts := make([]string, 0, len(stages))
	for i, stage := range stages {
		style := m.styles.badgeSoft
		if i < active {
			style = m.styles.badge
		} else if i == active {
			style = m.styles.badgeWarn
		}
		parts = append(parts, style.Render(stage))
	}
	return strings.Join(parts, " ")
}

func (m model) renderStatusBar(width int) string {
	parts := append([]string{m.renderRouterHealthBadge()}, m.pageStatusParts()...)
	line := strings.Join(parts, "  ")
	return m.styles.statusBar.Width(width).Render(truncate(line, width-2))
}

func (m model) renderRouterHealthBadge() string {
	if m.router == nil {
		return m.styles.bad.Render("ROUTER OFFLINE")
	}
	_, cooldown, limited, invalid, _ := m.keyStatusCounts()
	if invalid > 0 || limited > 0 || cooldown > 0 {
		return m.styles.badgeWarn.Render("ROUTER DEGRADED")
	}
	return m.styles.badge.Render("ROUTER HEALTHY")
}

func (m model) keyStatusCounts() (active, cooldown, limited, invalid, disabled int) {
	keys := m.cfgKeys()
	for _, key := range keys {
		switch key.Status {
		case domain.KeyStatusCooldown:
			cooldown++
		case domain.KeyStatusLimited:
			limited++
		case domain.KeyStatusInvalid:
			invalid++
		case domain.KeyStatusDisabled:
			disabled++
		default:
			active++
		}
	}
	return
}

func (m model) cfgKeys() []domain.APIKey {
	if m.router != nil {
		return m.router.ListKeys()
	}
	keys := make([]domain.APIKey, 0, len(m.cfg.Keys))
	for _, item := range m.cfg.Keys {
		status := domain.APIKeyStatus(defaultText(item.Status, "active"))
		keys = append(keys, domain.APIKey{ID: item.ID, ModelID: item.ModelID, Status: status})
	}
	return keys
}

func (m model) lastRequestSummary() string {
	if m.router == nil {
		return ""
	}
	logs := m.router.Logs()
	if len(logs) == 0 {
		return "no recent traffic"
	}
	last := logs[len(logs)-1]
	status := fmt.Sprintf("last %s %s %dms", defaultText(last.GroupID, last.ModelID), fmt.Sprint(last.StatusCode), last.LatencyMs)
	if last.Error != "" {
		status += " err=" + truncate(last.Error, 24)
	}
	return status
}

func (m model) pageStatusParts() []string {
	active, cooldown, limited, invalid, disabled := m.keyStatusCounts()
	common := []string{
		m.styles.badgeSoft.Render("THEME " + strings.ToUpper(m.theme)),
		m.styles.badgeSoft.Render(fmt.Sprintf("ACTIVE %d", active)),
		m.styles.badgeSoft.Render(fmt.Sprintf("COOLDOWN %d", cooldown)),
	}
	switch m.page {
	case pageChat:
		if len(m.chats) == 0 {
			return append(common, m.styles.hint.Render("no chat session"))
		}
		chat := m.chats[m.activeChat]
		state := "idle"
		if chat.Pending {
			state = "sending"
		}
		parts := append(common,
			m.styles.badgeSoft.Render("TARGET "+defaultText(chat.Target, "none")),
			m.styles.badgeSoft.Render(fmt.Sprintf("MSG %d", len(chat.Messages))),
			m.styles.badgeSoft.Render("STATE "+strings.ToUpper(state)),
		)
		if m.chatInput != "" {
			parts = append(parts, m.styles.hint.Render(fmt.Sprintf("draft %d chars", len([]rune(m.chatInput)))))
		}
		if strings.TrimSpace(m.chatFilter) != "" {
			parts = append(parts, m.styles.badgeSoft.Render("FIND "+strings.ToUpper(m.chatFilter)))
		}
		return parts
	case pageKeys:
		parts := append(common,
			m.styles.badgeSoft.Render(fmt.Sprintf("LIMITED %d", limited)),
			m.styles.badgeSoft.Render(fmt.Sprintf("INVALID %d", invalid)),
			m.styles.badgeSoft.Render(fmt.Sprintf("OFF %d", disabled)),
			m.styles.badgeSoft.Render("SORT "+strings.ToUpper(defaultText(m.keysSort, "status"))),
		)
		if soonest := m.nextCooldownSummary(); soonest != "" {
			parts = append(parts, m.styles.hint.Render(soonest))
		}
		return parts
	case pageLogs:
		parts := append(common,
			m.styles.badgeSoft.Render(fmt.Sprintf("LIMITED %d", limited)),
			m.styles.badgeSoft.Render("FILTER "+strings.ToUpper(defaultText(m.logsFilter, "latest"))),
			m.styles.badgeSoft.Render("SORT "+strings.ToUpper(defaultText(m.logsSort, "newest"))),
		)
		if summary := m.lastRequestSummary(); summary != "" {
			parts = append(parts, m.styles.hint.Render(summary))
		}
		return parts
	case pageConfig:
		state := "SAVED"
		if m.editor.dirty {
			state = "DIRTY"
		}
		parts := append(common,
			m.styles.badgeSoft.Render("CFG "+state),
			m.styles.badgeSoft.Render("SECTION "+strings.ToUpper(configSectionName(m.editor.section))),
		)
		if strings.TrimSpace(m.editor.filter) != "" {
			parts = append(parts, m.styles.badgeSoft.Render("FIND "+strings.ToUpper(m.editor.filter)))
		}
		if m.editor.message != "" {
			parts = append(parts, m.styles.hint.Render(truncate(m.editor.message, 40)))
		}
		return parts
	case pageProviders:
		return append(common, m.styles.badgeSoft.Render(fmt.Sprintf("PROV %d", len(m.cfg.Providers))))
	case pageModels:
		return append(common, m.styles.badgeSoft.Render(fmt.Sprintf("MODELS %d", len(m.cfg.Models))))
	case pageGroups:
		count := len(m.cfg.ModelGroups)
		if m.router != nil {
			count = len(m.router.ListModelGroups())
		}
		return append(common, m.styles.badgeSoft.Render(fmt.Sprintf("GROUPS %d", count)))
	default:
		return common
	}
}

func (m model) nextCooldownSummary() string {
	var soonest time.Duration
	found := false
	for _, key := range m.cfgKeys() {
		if key.CooldownEnd == nil || !key.CooldownEnd.After(time.Now()) {
			continue
		}
		remaining := time.Until(*key.CooldownEnd).Round(time.Second)
		if !found || remaining < soonest {
			soonest = remaining
			found = true
		}
	}
	if !found {
		return "no active cooldown window"
	}
	return "next cooldown clears in " + soonest.String()
}

func (m *model) applyLogFilterKey(key string) bool {
	switch key {
	case "1":
		m.logsFilter = "latest"
		return true
	case "2":
		m.logsFilter = "errors"
		return true
	case "3":
		m.logsFilter = "slow"
		return true
	case "4":
		m.logsFilter = "rate-limit"
		return true
	case "5":
		m.logsSort = "newest"
		return true
	case "6":
		m.logsSort = "slowest"
		return true
	default:
		return false
	}
}

func (m *model) applyKeysSortKey(key string) bool {
	switch key {
	case "1":
		m.keysSort = "status"
		return true
	case "2":
		m.keysSort = "cooldown"
		return true
	case "3":
		m.keysSort = "errors"
		return true
	default:
		return false
	}
}

func (m model) renderProviders(items []config.ProviderConfig) string {
	enabled := 0
	rows := make([]string, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			enabled++
		}
		rows = append(rows, m.renderProviderCard(item))
	}
	summary := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.styles.badge.Render(fmt.Sprintf("TOTAL %d", len(items))),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("ENABLED %d", enabled)),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("DISABLED %d", len(items)-enabled)),
	)
	return lipgloss.JoinVertical(lipgloss.Left, summary, "", m.renderCardGrid(rows))
}

func (m model) renderModels(items []config.ModelConfig) string {
	enabled := 0
	rows := make([]string, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			enabled++
		}
		rows = append(rows, m.renderModelCard(item))
	}
	summary := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.styles.badge.Render(fmt.Sprintf("TOTAL %d", len(items))),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("ENABLED %d", enabled)),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("DISABLED %d", len(items)-enabled)),
	)
	return lipgloss.JoinVertical(lipgloss.Left, summary, "", m.renderCardGrid(rows))
}

func (m model) renderConfigGroups(items []config.ModelGroupConfig) string {
	enabled := 0
	rows := make([]string, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			enabled++
		}
		rows = append(rows, m.renderConfigGroupCard(item))
	}
	memberCount := 0
	for _, item := range items {
		memberCount += len(item.Members)
	}
	summary := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.styles.badge.Render(fmt.Sprintf("TOTAL %d", len(items))),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("ENABLED %d", enabled)),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("MEMBERS %d", memberCount)),
	)
	return lipgloss.JoinVertical(lipgloss.Left, summary, "", m.renderCardGrid(rows))
}

func (m model) renderDomainGroups(items []domain.ModelGroup) string {
	if len(items) == 0 {
		return m.emptyState("No model groups configured")
	}
	enabled := 0
	memberCount := 0
	rows := make([]string, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			enabled++
		}
		memberCount += len(item.Members)
		rows = append(rows, m.renderDomainGroupCard(item))
	}
	summary := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.styles.badge.Render(fmt.Sprintf("TOTAL %d", len(items))),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("ENABLED %d", enabled)),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("MEMBERS %d", memberCount)),
	)
	return lipgloss.JoinVertical(lipgloss.Left, summary, "", m.renderCardGrid(rows))
}

func (m model) renderConfigKeys(items []config.KeyConfig) string {
	items = m.sortedConfigKeys(items)
	statusCounts := map[string]int{}
	rows := [][]string{}
	for _, item := range items {
		statusCounts[defaultText(item.Status, "active")]++
		rows = append(rows, []string{item.ID, item.ModelID, defaultText(item.Status, "active"), fmt.Sprint(item.Priority)})
	}
	summary := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.styles.badgeSoft.Render(fmt.Sprintf("ACTIVE %d", statusCounts["active"])),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("DISABLED %d", statusCounts["disabled"])),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("TOTAL %d", len(items))),
	)
	table := m.renderAdaptiveTable([]string{"ID", "Model", "Status", "Priority"}, rows, []int{20, 20, 10, 8}, []int{10, 10, 8, 6})
	return lipgloss.JoinVertical(lipgloss.Left, summary, "", table)
}

func (m model) renderDomainKeys(items []domain.APIKey) string {
	items = m.sortedDomainKeys(items)
	if len(items) == 0 {
		return m.emptyState("No keys available")
	}
	active, cooldown, limited, invalid, disabled := m.keyStatusCounts()
	summary := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.styles.badge.Render(fmt.Sprintf("ACTIVE %d", active)),
		" ",
		m.styles.badgeWarn.Render(fmt.Sprintf("COOLDOWN %d", cooldown)),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("LIMITED %d", limited)),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("INVALID %d", invalid)),
		" ",
		m.styles.badgeSoft.Render(fmt.Sprintf("OFF %d", disabled)),
		" ",
		m.styles.badgeSoft.Render("SORT "+strings.ToUpper(defaultText(m.keysSort, "status"))),
	)
	feedWidth := maxInt(36, m.pageBodyWidth()-4)
	var entries []string
	for _, item := range items {
		entries = append(entries, m.renderKeyEntry(item, feedWidth))
	}
	return lipgloss.JoinVertical(lipgloss.Left, summary, "", strings.Join(entries, "\n\n"))
}

func (m model) renderChat() string {
	if len(m.chats) == 0 {
		return m.emptyState("No chat sessions")
	}
	active := m.chats[m.activeChat]
	leftWidth := m.chatSidebarWidth()
	rightWidth := maxInt(34, m.pageBodyWidth()-leftWidth-m.bodyGap())
	left := m.renderChatSessionList(leftWidth)
	right := m.renderChatConversation(active, rightWidth)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", m.bodyGap()), right)
}

func (m model) renderChatSessionList(width int) string {
	indexes := m.filteredChatIndexes()
	var b strings.Builder
	b.WriteString(m.styles.section.Render(".:: SESSIONS ::."))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("ctrl+n new  ctrl+t target  ctrl+f filter"))
	b.WriteString("\n")
	b.WriteString(m.styles.input.Width(width - 2).Render("/ " + defaultText(m.chatFilter, "all sessions")))
	b.WriteString("\n\n")
	if len(indexes) == 0 {
		b.WriteString(m.styles.muted.Render("no matching sessions"))
		b.WriteString("\n\n")
	}
	for _, idx := range indexes {
		chat := m.chats[idx]
		label := fmt.Sprintf("%d. %s", chat.ID, chat.Title)
		style := m.styles.nav.Width(width - 4)
		prefix := "  "
		if idx == m.activeChat {
			style = m.styles.navActive.Width(width - 4)
			prefix = "> "
		}
		preview := "no messages yet"
		if len(chat.Messages) > 0 {
			preview = truncate(chat.Messages[len(chat.Messages)-1].Content, width-8)
		}
		stateBadge := m.styles.badgeSoft.Render("READY")
		if chat.Pending {
			stateBadge = m.styles.badgeWarn.Render("SENDING")
		}
		b.WriteString(style.Render(prefix + truncate(label, width-6)))
		b.WriteString("\n")
		b.WriteString(m.styles.navMuted.Render("   " + truncate(chat.Target, width-6)))
		b.WriteString("\n")
		b.WriteString("   ")
		b.WriteString(stateBadge)
		b.WriteString("\n")
		b.WriteString(m.styles.hint.Render("   " + preview))
		if idx != indexes[len(indexes)-1] {
			b.WriteString("\n\n")
		}
	}
	b.WriteString("\n\n")
	b.WriteString(m.styles.section.Render(".:: FLOW ::."))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("up/down switch"))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("enter send"))
	return m.styles.sidebar.Width(width).Render(b.String())
}

func (m model) renderChatConversation(chat tuiChatSession, width int) string {
	var b strings.Builder
	header := m.styles.panelTitle.Render(chat.Title)
	target := m.styles.badge.Render(defaultText(chat.Target, "no-target"))
	if chat.Pending {
		target += " " + m.styles.hint.Render("sending")
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, header, "  ", target))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("assistant traffic is routed through the currently selected model or group"))
	b.WriteString("\n")
	b.WriteString(m.styles.muted.Render(strings.Repeat("-", width)))
	b.WriteString("\n")

	if len(chat.Messages) == 0 {
		b.WriteString(m.styles.muted.Render("No messages yet. Start typing below and press enter to send."))
		b.WriteString("\n")
	} else {
		start := 0
		if len(chat.Messages) > 10 {
			start = len(chat.Messages) - 10
		}
		for _, message := range chat.Messages[start:] {
			b.WriteString(m.renderChatMessage(message, width))
			b.WriteString("\n")
		}
	}

	if chat.Error != "" {
		b.WriteString(m.styles.bad.Render("Error: " + chat.Error))
		b.WriteString("\n")
	}
	b.WriteString(m.styles.muted.Render(strings.Repeat("-", width)))
	b.WriteString("\n")
	b.WriteString(m.styles.section.Render("INPUT"))
	b.WriteString("\n")
	b.WriteString(m.styles.input.Width(width).Render("> " + defaultText(m.chatInput, "type your prompt...")))
	return b.String()
}

func (m model) renderChatMessage(message tuiChatMessage, width int) string {
	meta := m.styles.chatMeta.Render(strings.ToUpper(message.Role) + " // " + message.CreatedAt.Format("15:04:05"))
	bubbleWidth := maxInt(20, width-10)
	content := wrapText(message.Content, bubbleWidth-4)
	if content == "" {
		content = " "
	}
	bubble := m.styles.chatBot.Render(content)
	align := lipgloss.Left
	switch message.Role {
	case "user":
		bubble = m.styles.chatUser.Render(content)
		align = lipgloss.Right
	case "error":
		bubble = m.styles.chatError.Render(content)
	}
	block := lipgloss.JoinVertical(lipgloss.Left, meta, bubble)
	return lipgloss.PlaceHorizontal(width, align, block)
}

func (m model) filteredChatIndexes() []int {
	if strings.TrimSpace(m.chatFilter) == "" {
		out := make([]int, len(m.chats))
		for i := range m.chats {
			out[i] = i
		}
		return out
	}
	query := strings.ToLower(strings.TrimSpace(m.chatFilter))
	out := make([]int, 0, len(m.chats))
	for i, chat := range m.chats {
		if strings.Contains(strings.ToLower(chat.Title), query) || strings.Contains(strings.ToLower(chat.Target), query) || strings.Contains(strings.ToLower(chatPreview(chat)), query) {
			out = append(out, i)
		}
	}
	return out
}

func chatPreview(chat tuiChatSession) string {
	if len(chat.Messages) == 0 {
		return ""
	}
	return chat.Messages[len(chat.Messages)-1].Content
}

func (m *model) moveActiveChat(step int) {
	indexes := m.filteredChatIndexes()
	if len(indexes) == 0 {
		return
	}
	pos := 0
	for i, idx := range indexes {
		if idx == m.activeChat {
			pos = i
			break
		}
	}
	if step < 0 {
		pos = previousIndex(pos, len(indexes))
	} else {
		pos = nextIndex(pos, len(indexes))
	}
	if pos >= 0 && pos < len(indexes) {
		m.activeChat = indexes[pos]
		m.chatAnchor = m.activeChat
	}
}

func (m *model) ensureActiveChatVisible() {
	indexes := m.filteredChatIndexes()
	if len(indexes) == 0 {
		return
	}
	if strings.TrimSpace(m.chatFilter) == "" {
		if m.chatAnchor >= 0 && m.chatAnchor < len(m.chats) {
			m.activeChat = m.chatAnchor
			return
		}
	}
	for _, idx := range indexes {
		if idx == m.chatAnchor {
			m.activeChat = idx
			return
		}
	}
	for _, idx := range indexes {
		if idx == m.activeChat {
			m.chatAnchor = idx
			return
		}
	}
	m.activeChat = indexes[0]
	m.chatAnchor = m.activeChat
}

func (m *model) addChatSession() {
	target := defaultChatTarget(m.cfg)
	m.chats = append(m.chats, newTUIChatSession(m.nextChatID, target))
	m.activeChat = len(m.chats) - 1
	m.chatAnchor = m.activeChat
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
	m.chatAnchor = m.activeChat
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
	items := m.sortedLogs(m.filteredLogs(m.router.Logs()))
	if len(items) == 0 {
		return m.emptyState("No log entries match the current filter")
	}
	if len(items) > 10 {
		items = items[:10]
	}
	feedWidth := maxInt(36, m.pageBodyWidth()-4)
	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.styles.badgeSoft.Render("FILTER "+strings.ToUpper(defaultText(m.logsFilter, "latest"))),
		" ",
		m.styles.badgeSoft.Render("SORT "+strings.ToUpper(defaultText(m.logsSort, "newest"))),
		" ",
		m.styles.hint.Render("1 latest  2 errors  3 slow"),
		" ",
		m.styles.hint.Render("5 newest  6 slowest"),
	)
	var sections []string
	for _, item := range items {
		sections = append(sections, m.renderLogEntry(item, feedWidth))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, "", strings.Join(sections, "\n\n"))
}

func (m model) filteredLogs(items []domain.RequestLog) []domain.RequestLog {
	filter := defaultText(m.logsFilter, "latest")
	if filter == "latest" {
		return items
	}
	filtered := make([]domain.RequestLog, 0, len(items))
	for _, item := range items {
		switch filter {
		case "errors":
			if item.Error != "" || item.StatusCode >= 400 {
				filtered = append(filtered, item)
			}
		case "slow":
			if item.LatencyMs >= 3000 {
				filtered = append(filtered, item)
			}
		case "rate-limit":
			if item.StatusCode == 429 {
				filtered = append(filtered, item)
			}
		default:
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m model) sortedLogs(items []domain.RequestLog) []domain.RequestLog {
	out := append([]domain.RequestLog(nil), items...)
	switch m.logsSort {
	case "slowest":
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].LatencyMs == out[j].LatencyMs {
				return out[i].CreatedAt.After(out[j].CreatedAt)
			}
			return out[i].LatencyMs > out[j].LatencyMs
		})
	default:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		})
	}
	return out
}

func (m model) renderLogEntry(item domain.RequestLog, width int) string {
	badge := m.logStatusBadge(item)
	target := defaultText(item.GroupID, item.ModelID)
	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		badge,
		" ",
		m.styles.panelTitle.Render(target),
		" ",
		m.styles.hint.Render(item.CreatedAt.Format("15:04:05")),
	)
	metaText := truncate(fmt.Sprintf("model=%s  provider=%s  key=%s", defaultText(item.ModelID, "-"), defaultText(item.ProviderID, "-"), defaultText(item.KeyID, "-")), width)
	metricsText := truncate(fmt.Sprintf("latency=%dms  tokens=%d/%d", item.LatencyMs, item.TokenInput, item.TokenOutput), width)
	parts := []string{header, m.styles.navMuted.Render(metaText), m.styles.hint.Render(metricsText)}
	if item.Error != "" {
		parts = append(parts, m.styles.bad.Render(wrapText("error: "+item.Error, width)))
	}
	divider := m.styles.tableFrame.Render(strings.Repeat("─", width))
	parts = append(parts, divider)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) logStatusBadge(item domain.RequestLog) string {
	if item.Error != "" || item.StatusCode >= 500 {
		return m.styles.badgeWarn.Render(fmt.Sprintf("ERR %d", item.StatusCode))
	}
	if item.StatusCode >= 400 {
		return m.styles.badgeSoft.Render(fmt.Sprintf("FAIL %d", item.StatusCode))
	}
	return m.styles.badge.Render(fmt.Sprintf("OK %d", item.StatusCode))
}

func (m model) renderKeyEntry(item domain.APIKey, width int) string {
	badge := m.keyStatusBadge(item)
	name := defaultText(item.Name, item.ID)
	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		badge,
		" ",
		m.styles.panelTitle.Render(truncate(name, maxInt(10, width-24))),
	)
	meta := m.styles.navMuted.Render(truncate(fmt.Sprintf("model=%s  provider=%s  priority=%d", defaultText(item.ModelID, "-"), defaultText(item.ProviderID, "-"), item.Priority), width))
	metrics := m.styles.hint.Render(truncate(fmt.Sprintf("used=%d  errors=%d  updated=%s", item.UsedCount, item.ErrorCount, keyRecentText(item.LastUsedAt)), width))
	parts := []string{header, meta, metrics}
	if item.DailyRequestLimit > 0 || item.DailyTokenLimit > 0 {
		quotaText := ""
		if item.DailyRequestLimit > 0 {
			quotaText = fmt.Sprintf("req=%d/%d", item.DailyRequestCount, item.DailyRequestLimit)
		}
		if item.DailyTokenLimit > 0 {
			if quotaText != "" {
				quotaText += "  "
			}
			quotaText += fmt.Sprintf("tok=%d/%d", item.DailyTokenCount, item.DailyTokenLimit)
		}
		parts = append(parts, m.styles.hint.Render(quotaText))
	}
	if item.CooldownEnd != nil && item.CooldownEnd.After(time.Now()) {
		parts = append(parts, m.styles.bad.Render("cooldown clears in "+time.Until(*item.CooldownEnd).Round(time.Second).String()))
	}
	parts = append(parts, m.styles.tableFrame.Render(strings.Repeat("─", width)))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) sortedDomainKeys(items []domain.APIKey) []domain.APIKey {
	out := append([]domain.APIKey(nil), items...)
	switch m.keysSort {
	case "cooldown":
		sort.SliceStable(out, func(i, j int) bool {
			ci, cji := cooldownPriority(out[i]), cooldownPriority(out[j])
			if ci == cji {
				return out[i].ErrorCount > out[j].ErrorCount
			}
			return ci < cji
		})
	case "errors":
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].ErrorCount == out[j].ErrorCount {
				return out[i].UsedCount > out[j].UsedCount
			}
			return out[i].ErrorCount > out[j].ErrorCount
		})
	default:
		sort.SliceStable(out, func(i, j int) bool {
			pi, pj := keyStatusPriority(out[i].Status), keyStatusPriority(out[j].Status)
			if pi == pj {
				return out[i].Priority < out[j].Priority
			}
			return pi < pj
		})
	}
	return out
}

func (m model) sortedConfigKeys(items []config.KeyConfig) []config.KeyConfig {
	out := append([]config.KeyConfig(nil), items...)
	switch m.keysSort {
	case "errors":
		sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	case "cooldown":
		sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	default:
		sort.SliceStable(out, func(i, j int) bool {
			pi := keyStatusPriority(domain.APIKeyStatus(defaultText(out[i].Status, "active")))
			pj := keyStatusPriority(domain.APIKeyStatus(defaultText(out[j].Status, "active")))
			if pi == pj {
				return out[i].Priority < out[j].Priority
			}
			return pi < pj
		})
	}
	return out
}

func keyStatusPriority(status domain.APIKeyStatus) int {
	switch status {
	case domain.KeyStatusActive:
		return 0
	case domain.KeyStatusLimited:
		return 1
	case domain.KeyStatusCooldown:
		return 2
	case domain.KeyStatusDisabled:
		return 3
	case domain.KeyStatusInvalid:
		return 4
	default:
		return 5
	}
}

func cooldownPriority(item domain.APIKey) int64 {
	if item.CooldownEnd != nil && item.CooldownEnd.After(time.Now()) {
		return time.Until(*item.CooldownEnd).Milliseconds()
	}
	return 1 << 62
}

func (m model) keyStatusBadge(item domain.APIKey) string {
	switch item.Status {
	case domain.KeyStatusCooldown:
		return m.styles.badgeWarn.Render("COOLDOWN")
	case domain.KeyStatusLimited:
		return m.styles.badgeSoft.Render("LIMITED")
	case domain.KeyStatusInvalid:
		return m.styles.bad.Render("INVALID")
	case domain.KeyStatusDisabled:
		return m.styles.badgeSoft.Render("OFF")
	default:
		return m.styles.badge.Render("ACTIVE")
	}
}

func keyRecentText(lastUsedAt *time.Time) string {
	if lastUsedAt == nil {
		return "never"
	}
	return lastUsedAt.Format("15:04:05")
}

func domainGroupMembersText(members []domain.ModelGroupMember) string {
	items := make([]string, 0, len(members))
	for _, member := range members {
		items = append(items, member.ModelID)
	}
	return strings.Join(items, ",")
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
		return m.styles.badge.Render("ACTIVE")
	}
	return m.styles.badgeSoft.Render("DISABLED")
}

func (m model) emptyState(text string) string {
	return m.styles.muted.Render(text)
}

func (m model) isTextEntryMode() bool {
	if m.contentFocused && m.page == pageChat {
		return true
	}
	if m.keyTesting {
		return true
	}
	if !m.contentFocused || m.page != pageConfig {
		return false
	}
	return m.editor.mode == editorModeForm || m.editor.mode == editorModeDelete || m.editor.filterOn
}

func (m model) shouldHandleEscInContent() bool {
	switch m.page {
	case pageChat:
		return m.chatFiltering || m.chatInput != ""
	case pageConfig:
		return m.editor.mode == editorModeForm || m.editor.mode == editorModeDelete || m.editor.filterOn
	default:
		return false
	}
}

func (m model) renderAdaptiveTable(headers []string, rows [][]string, widths []int, minWidths []int) string {
	availableWidth := maxInt(24, m.pageBodyWidth()-6)
	return renderTable(m.styles, headers, rows, autoTableWidths(headers, rows, widths, minWidths, availableWidth))
}

func renderTable(styles styles, headers []string, rows [][]string, widths []int) string {
	if len(rows) == 0 {
		return styles.muted.Render("No data")
	}
	sep := styles.tableFrame.Render("│")
	pad := " "

	lineParts := make([]string, len(widths))
	headParts := make([]string, len(widths))
	for i, w := range widths {
		lineParts[i] = strings.Repeat("─", w+2)
		headParts[i] = styles.tableHead.Render(padRight(truncate(headers[i], w), w))
	}
	topLine := styles.tableFrame.Render("╭" + strings.Join(lineParts, "┬") + "╮")

	var b strings.Builder
	b.WriteString(topLine)
	b.WriteString("\n")
	b.WriteString(sep + pad + strings.Join(headParts, pad+sep+pad) + pad + sep)
	b.WriteString("\n")
	midLine := styles.tableFrame.Render("├" + strings.Join(lineParts, "┼") + "┤")
	b.WriteString(midLine)
	b.WriteString("\n")

	for rowIndex, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = padRight(truncate(cell, widths[i]), widths[i])
		}
		rowLine := sep + pad + strings.Join(cells, pad+sep+pad) + pad + sep
		if rowIndex%2 == 0 {
			b.WriteString(styles.rowEven.Render(rowLine))
		} else {
			b.WriteString(styles.rowOdd.Render(rowLine))
		}
		if rowIndex < len(rows)-1 {
			b.WriteString("\n")
			b.WriteString(styles.tableFrame.Render("├" + strings.Join(lineParts, "┼") + "┤"))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	botLine := styles.tableFrame.Render("╰" + strings.Join(lineParts, "┴") + "╯")
	b.WriteString(botLine)

	return b.String()
}

func autoTableWidths(headers []string, rows [][]string, widths []int, minWidths []int, maxWidth int) []int {
	fitted := fitTableWidths(widths, minWidths, maxWidth)
	if renderedTableWidth(fitted) >= maxWidth {
		return fitted
	}
	targets := desiredTableWidths(headers, rows, widths)
	for renderedTableWidth(fitted) < maxWidth {
		idx := -1
		bestNeed := 0
		for i := range fitted {
			need := targets[i] - fitted[i]
			if need > bestNeed {
				bestNeed = need
				idx = i
			}
		}
		if idx == -1 {
			break
		}
		fitted[idx]++
	}
	return fitTableWidths(fitted, minWidths, maxWidth)
}

func fitTableWidths(widths []int, minWidths []int, maxWidth int) []int {
	fitted := append([]int(nil), widths...)
	mins := append([]int(nil), minWidths...)
	for len(mins) < len(fitted) {
		mins = append(mins, 1)
	}
	for renderedTableWidth(fitted) > maxWidth {
		idx := -1
		bestRoom := 0
		bestWidth := 0
		for i := range fitted {
			room := fitted[i] - mins[i]
			if room <= 0 {
				continue
			}
			if room > bestRoom || (room == bestRoom && fitted[i] > bestWidth) {
				idx = i
				bestRoom = room
				bestWidth = fitted[i]
			}
		}
		if idx == -1 {
			break
		}
		fitted[idx]--
	}
	return fitted
}

func desiredTableWidths(headers []string, rows [][]string, widths []int) []int {
	desired := append([]int(nil), widths...)
	for i := range headers {
		desired[i] = maxInt(desired[i], lipgloss.Width(headers[i]))
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(desired) {
				break
			}
			cellWidth := lipgloss.Width(cell)
			if cellWidth > desired[i] {
				desired[i] = minInt(cellWidth, widths[i]+18)
			}
		}
	}
	return desired
}

func renderedTableWidth(widths []int) int {
	total := 1
	for _, width := range widths {
		total += width + 3
	}
	return total
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
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}
