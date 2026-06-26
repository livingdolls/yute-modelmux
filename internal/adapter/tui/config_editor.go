package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/livingdolls/yute-modelmux/internal/config"
)

type configSection int

const (
	configSectionProviders configSection = iota
	configSectionModels
	configSectionGroups
	configSectionKeys
)

type editorMode int

const (
	editorModeBrowse editorMode = iota
	editorModeForm
	editorModeDelete
)

type configEditorState struct {
	section         configSection
	selected        int
	sectionSelected [4]int
	mode            editorMode
	dirty           bool
	message         string
	filter          string
	filterOn        bool
	form            configFormState
	confirm         deleteConfirmState
}

type configFormState struct {
	title string
	kind  configSection
	index int
	field int
	items []formField
}

type formField struct {
	label string
	value string
	mask  bool
}

type deleteConfirmState struct {
	title  string
	impact string
	kind   configSection
	index  int
	input  string
}

func configSectionName(section configSection) string {
	switch section {
	case configSectionModels:
		return "models"
	case configSectionGroups:
		return "groups"
	case configSectionKeys:
		return "keys"
	default:
		return "providers"
	}
}

func newConfigEditorState(cfg *config.Config) configEditorState {
	return configEditorState{section: configSectionProviders, mode: editorModeBrowse}
}

func (m model) updateConfigEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.editor.mode == editorModeForm {
		return m.updateConfigForm(msg)
	}
	if m.editor.mode == editorModeDelete {
		return m.updateDeleteConfirm(msg)
	}
	if m.editor.filterOn {
		return m.updateConfigFilter(msg)
	}

	switch key {
	case "tab":
		m.selected = nextIndex(m.selected, len(navItems))
		return m, nil
	case "shift+tab":
		m.selected = previousIndex(m.selected, len(navItems))
		return m, nil
	case "up":
		m.selected = previousIndex(m.selected, len(navItems))
		return m, nil
	case "down":
		m.selected = nextIndex(m.selected, len(navItems))
		return m, nil
	case "enter":
		if m.selected != m.page {
			m.page = m.selected
			return m, nil
		}
		m.startEditConfigItem()
	case "left", "h":
		m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
		m.editor.section = configSection(previousIndex(int(m.editor.section), 4))
		m.editor.selected = m.editor.sectionSelected[int(m.editor.section)]
		m.ensureConfigSelectionVisible()
	case "right", "l":
		m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
		m.editor.section = configSection(nextIndex(int(m.editor.section), 4))
		m.editor.selected = m.editor.sectionSelected[int(m.editor.section)]
		m.ensureConfigSelectionVisible()
	case "k":
		m.moveConfigSelection(-1)
	case "j":
		m.moveConfigSelection(1)
	case "a":
		m.startAddConfigItem()
	case "e":
		m.startEditConfigItem()
	case "d":
		m.startDeleteConfigItem()
	case " ":
		m.toggleConfigItem()
	case "s":
		m.saveDraftConfig()
	case "r":
		m.reloadDraftConfig()
	case "/", "ctrl+f":
		m.editor.filterOn = true
	}
	return m, nil
}

func (m model) updateConfigFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "enter", "/", "ctrl+f":
		m.editor.filterOn = false
		m.ensureConfigSelectionVisible()
		return m, nil
	case "backspace", "ctrl+h":
		m.editor.filter = dropLastRune(m.editor.filter)
		m.ensureConfigSelectionVisible()
		return m, nil
	case "ctrl+u":
		m.editor.filter = ""
		m.ensureConfigSelectionVisible()
		return m, nil
	}
	if len(msg.Runes) > 0 {
		m.editor.filter += string(msg.Runes)
		m.ensureConfigSelectionVisible()
	}
	return m, nil
}

func (m model) updateConfigForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.editor.mode = editorModeBrowse
		m.editor.message = "cancelled"
		return m, nil
	case "ctrl+s":
		m.applyConfigForm()
		return m, nil
	case "enter", "down":
		if m.editor.form.field >= len(m.editor.form.items)-1 {
			m.applyConfigForm()
			return m, nil
		}
		m.editor.form.field++
		return m, nil
	case "up":
		m.editor.form.field = previousIndex(m.editor.form.field, len(m.editor.form.items))
		return m, nil
	case "backspace", "ctrl+h":
		field := &m.editor.form.items[m.editor.form.field]
		field.value = dropLastRune(field.value)
		return m, nil
	case "ctrl+u":
		m.editor.form.items[m.editor.form.field].value = ""
		return m, nil
	case " ":
		m.editor.form.items[m.editor.form.field].value += " "
		return m, nil
	}
	if len(msg.Runes) > 0 {
		m.editor.form.items[m.editor.form.field].value += string(msg.Runes)
	}
	return m, nil
}

func (m model) updateDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.editor.mode = editorModeBrowse
		m.editor.message = "delete cancelled"
	case "enter":
		if m.editor.confirm.input == "delete" {
			m.applyDeleteConfigItem()
			return m, nil
		}
		m.editor.message = "type delete to confirm"
	case "backspace", "ctrl+h":
		m.editor.confirm.input = dropLastRune(m.editor.confirm.input)
	case "ctrl+u":
		m.editor.confirm.input = ""
	case " ":
		m.editor.confirm.input += " "
	default:
		if len(msg.Runes) > 0 {
			m.editor.confirm.input += string(msg.Runes)
		}
	}
	return m, nil
}

func (m model) renderConfigEditor() string {
	switch m.editor.mode {
	case editorModeForm:
		return m.renderConfigForm()
	case editorModeDelete:
		return m.renderDeleteConfirm()
	}
	sections := []string{"Providers", "Models", "Groups", "Keys"}
	var tabs []string
	for i, section := range sections {
		label := section
		if configSection(i) == m.editor.section {
			label = m.styles.navActive.Render(label)
		} else {
			label = m.styles.nav.Render(label)
		}
		tabs = append(tabs, label)
	}
	var b strings.Builder
	b.WriteString(m.styles.section.Render(".:: CONFIG DECK ::."))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("edit the live draft, then save to reload the router"))
	b.WriteString("\n\n")
	b.WriteString(strings.Join(tabs, " "))
	b.WriteString("\n")
	b.WriteString(m.styles.input.Render("/ " + defaultText(m.editor.filter, "all rows")))
	b.WriteString("\n\n")
	b.WriteString(m.renderConfigSectionTable())
	status := "saved"
	if m.editor.dirty {
		status = "dirty"
	}
	b.WriteString("\n\n")
	b.WriteString(m.styles.badge.Render(strings.ToUpper(status)))
	b.WriteString(" ")
	b.WriteString(m.styles.hint.Render(defaultText(m.configPath, "default")))
	if m.editor.message != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.hint.Render(m.editor.message))
	}
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("up/down:menu  j/k:row  <- ->:section  a:add  e:edit  d:del  /:filter  s:save"))
	return b.String()
}

func (m model) renderConfigSectionTable() string {
	indexes := m.filteredConfigIndexes()
	switch m.editor.section {
	case configSectionModels:
		rows := make([][]string, 0, len(indexes))
		for _, i := range indexes {
			item := m.cfg.Models[i]
			rows = append(rows, m.markSelectedRow(i, []string{item.ID, item.ProviderID, item.ModelName, defaultText(item.Strategy, "failover"), boolString(item.Enabled)}))
		}
		return m.renderAdaptiveTable([]string{"ID", "Provider", "Provider Model", "Strategy", "Enabled"}, rows, []int{20, 14, 24, 12, 8}, []int{10, 8, 12, 8, 7})
	case configSectionGroups:
		rows := make([][]string, 0, len(indexes))
		for _, i := range indexes {
			item := m.cfg.ModelGroups[i]
			rows = append(rows, m.markSelectedRow(i, []string{item.ID, item.Name, defaultText(item.Strategy, "failover"), fmt.Sprint(len(item.Members)), boolString(item.Enabled)}))
		}
		return m.renderAdaptiveTable([]string{"ID", "Name", "Strategy", "Members", "Enabled"}, rows, []int{20, 18, 12, 8, 8}, []int{10, 10, 8, 6, 7})
	case configSectionKeys:
		rows := make([][]string, 0, len(indexes))
		for _, i := range indexes {
			item := m.cfg.Keys[i]
			rows = append(rows, m.markSelectedRow(i, []string{item.ID, item.ProviderID, item.ModelID, item.Name, item.ValueEnv, defaultText(item.Status, "active"), fmt.Sprint(item.Priority)}))
		}
		return m.renderAdaptiveTable([]string{"ID", "Provider", "Model", "Name", "Value Env", "Status", "Priority"}, rows, []int{16, 14, 18, 14, 14, 10, 8}, []int{8, 8, 10, 8, 10, 8, 6})
	default:
		rows := make([][]string, 0, len(indexes))
		for _, i := range indexes {
			item := m.cfg.Providers[i]
			rows = append(rows, m.markSelectedRow(i, []string{item.ID, item.Name, item.Type, truncate(item.BaseURL, 30), item.AuthType, boolString(item.Enabled)}))
		}
		return m.renderAdaptiveTable([]string{"ID", "Name", "Type", "Base URL", "Auth", "Enabled"}, rows, []int{14, 18, 16, 28, 8, 8}, []int{8, 10, 8, 12, 6, 7})
	}
}

func (m model) markSelectedRow(index int, row []string) []string {
	out := append([]string(nil), row...)
	if index == m.editor.selected && len(out) > 0 {
		out[0] = "> " + out[0]
	}
	return out
}

func (m model) renderConfigForm() string {
	form := m.editor.form
	var b strings.Builder
	b.WriteString(m.styles.panelTitle.Render(":: " + strings.ToUpper(form.title) + " ::"))
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("enter:next/apply  ctrl+s:save  esc:cancel"))
	b.WriteString("\n")
	b.WriteString(m.styles.muted.Render(strings.Repeat("-", 46)))
	b.WriteString("\n")
	for i, field := range form.items {
		prefix := "  "
		if i == form.field {
			prefix = "> "
		}
		value := field.value
		if field.mask && value != "" {
			value = strings.Repeat("*", minInt(12, len([]rune(value))))
		}
		b.WriteString(fmt.Sprintf("%s%-14s %s\n", prefix, field.label+":", value))
	}
	if m.editor.message != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.bad.Render(m.editor.message))
	}
	return b.String()
}

func (m model) renderDeleteConfirm() string {
	confirm := m.editor.confirm
	var b strings.Builder
	b.WriteString(m.styles.bad.Render(":: DELETE CONFIRM ::"))
	b.WriteString("\n")
	b.WriteString(m.styles.panelTitle.Render(confirm.title))
	b.WriteString("\n")
	b.WriteString(m.styles.muted.Render(strings.Repeat("-", 46)))
	b.WriteString("\n")
	b.WriteString(confirm.impact)
	b.WriteString("\n")
	b.WriteString(m.styles.hint.Render("Type delete to confirm, esc to cancel"))
	b.WriteString("\n")
	b.WriteString(m.styles.input.Render("> " + confirm.input))
	return b.String()
}

func (m model) configSectionLen() int {
	switch m.editor.section {
	case configSectionModels:
		return len(m.cfg.Models)
	case configSectionGroups:
		return len(m.cfg.ModelGroups)
	case configSectionKeys:
		return len(m.cfg.Keys)
	default:
		return len(m.cfg.Providers)
	}
}

func (m model) filteredConfigIndexes() []int {
	query := strings.ToLower(strings.TrimSpace(m.editor.filter))
	out := make([]int, 0, m.configSectionLen())
	appendIfMatch := func(index int, values ...string) {
		if query == "" {
			out = append(out, index)
			return
		}
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), query) {
				out = append(out, index)
				return
			}
		}
	}
	switch m.editor.section {
	case configSectionModels:
		for i, item := range m.cfg.Models {
			appendIfMatch(i, item.ID, item.ProviderID, item.ModelName, string(item.Strategy))
		}
	case configSectionGroups:
		for i, item := range m.cfg.ModelGroups {
			appendIfMatch(i, item.ID, item.Name, item.Strategy, groupMembersText(item.Members))
		}
	case configSectionKeys:
		for i, item := range m.cfg.Keys {
			appendIfMatch(i, item.ID, item.ProviderID, item.ModelID, item.Name, item.Status)
		}
	default:
		for i, item := range m.cfg.Providers {
			appendIfMatch(i, item.ID, item.Name, item.Type, item.BaseURL, item.AuthType)
		}
	}
	return out
}

func (m *model) moveConfigSelection(step int) {
	indexes := m.filteredConfigIndexes()
	if len(indexes) == 0 {
		return
	}
	pos := 0
	for i, idx := range indexes {
		if idx == m.editor.selected {
			pos = i
			break
		}
	}
	if step < 0 {
		pos = previousIndex(pos, len(indexes))
	} else {
		pos = nextIndex(pos, len(indexes))
	}
	m.editor.selected = indexes[pos]
	m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
}

func (m *model) ensureConfigSelectionVisible() {
	indexes := m.filteredConfigIndexes()
	if len(indexes) == 0 {
		m.editor.selected = m.editor.sectionSelected[int(m.editor.section)]
		return
	}
	saved := m.editor.sectionSelected[int(m.editor.section)]
	for _, idx := range indexes {
		if idx == saved {
			m.editor.selected = idx
			return
		}
	}
	for _, idx := range indexes {
		if idx == m.editor.selected {
			m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
			return
		}
	}
	m.editor.selected = indexes[0]
	m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
}

func (m model) hasVisibleConfigSelection() bool {
	for _, idx := range m.filteredConfigIndexes() {
		if idx == m.editor.selected {
			return true
		}
	}
	return false
}

func (m *model) startAddConfigItem() {
	m.editor.mode = editorModeForm
	m.editor.form = newConfigForm(m.editor.section, -1, nil)
	m.editor.message = ""
}

func (m *model) startEditConfigItem() {
	if !m.hasVisibleConfigSelection() {
		m.editor.message = "nothing selected"
		return
	}
	idx := m.editor.selected
	if idx < 0 || idx >= m.configSectionLen() {
		m.editor.message = "nothing selected"
		return
	}
	m.editor.mode = editorModeForm
	m.editor.form = newConfigForm(m.editor.section, idx, m)
	m.editor.message = ""
}

func newConfigForm(section configSection, index int, m *model) configFormState {
	form := configFormState{kind: section, index: index}
	suffix := "Add"
	if index >= 0 {
		suffix = "Edit"
	}
	switch section {
	case configSectionModels:
		form.title = suffix + " Model"
		item := config.ModelConfig{Strategy: "failover", Enabled: true}
		if m != nil && index >= 0 {
			item = m.cfg.Models[index]
		}
		form.items = []formField{{"ID", item.ID, false}, {"Provider ID", item.ProviderID, false}, {"Model Name", item.ModelName, false}, {"Strategy", defaultText(item.Strategy, "failover"), false}, {"Enabled", boolString(item.Enabled), false}}
	case configSectionGroups:
		form.title = suffix + " Group"
		item := config.ModelGroupConfig{Strategy: "failover", Enabled: true}
		if m != nil && index >= 0 {
			item = m.cfg.ModelGroups[index]
		}
		form.items = []formField{{"ID", item.ID, false}, {"Name", item.Name, false}, {"Strategy", defaultText(item.Strategy, "failover"), false}, {"Members", groupMembersText(item.Members), false}, {"Enabled", boolString(item.Enabled), false}}
	case configSectionKeys:
		form.title = suffix + " API Key"
		item := config.KeyConfig{Status: "active", Priority: 1}
		if m != nil && index >= 0 {
			item = m.cfg.Keys[index]
		}
		form.items = []formField{{"ID", item.ID, false}, {"Provider ID", item.ProviderID, false}, {"Model ID", item.ModelID, false}, {"Name", item.Name, false}, {"Value", item.Value, true}, {"Value Env", item.ValueEnv, false}, {"Status", defaultText(item.Status, "active"), false}, {"Priority", fmt.Sprint(defaultInt(item.Priority, 1)), false}}
	default:
		form.title = suffix + " Provider"
		item := config.ProviderConfig{Type: "openai-compatible", AuthType: "bearer", TimeoutSeconds: 120, Enabled: true}
		if m != nil && index >= 0 {
			item = m.cfg.Providers[index]
		}
		form.items = []formField{{"ID", item.ID, false}, {"Name", item.Name, false}, {"Type", defaultText(item.Type, "openai-compatible"), false}, {"Base URL", item.BaseURL, false}, {"Auth Type", defaultText(item.AuthType, "bearer"), false}, {"Auth Header", item.AuthHeaderName, false}, {"Timeout", fmt.Sprint(defaultInt(item.TimeoutSeconds, 120)), false}, {"Enabled", boolString(item.Enabled), false}}
	}
	return form
}

func (m *model) applyConfigForm() {
	form := m.editor.form
	values := formValues(form.items)
	switch form.kind {
	case configSectionModels:
		item := config.ModelConfig{ID: values[0], ProviderID: values[1], ModelName: values[2], Strategy: defaultText(values[3], "failover"), Enabled: parseBool(values[4])}
		if form.index >= 0 {
			m.cfg.Models[form.index] = item
		} else {
			m.cfg.Models = append(m.cfg.Models, item)
			m.editor.selected = len(m.cfg.Models) - 1
			m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
		}
	case configSectionGroups:
		item := config.ModelGroupConfig{ID: values[0], Name: values[1], Strategy: defaultText(values[2], "failover"), Members: parseGroupMembers(values[3]), Enabled: parseBool(values[4])}
		if form.index >= 0 {
			m.cfg.ModelGroups[form.index] = item
		} else {
			m.cfg.ModelGroups = append(m.cfg.ModelGroups, item)
			m.editor.selected = len(m.cfg.ModelGroups) - 1
			m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
		}
	case configSectionKeys:
		priority, err := strconv.Atoi(defaultText(values[7], "1"))
		if err != nil || priority <= 0 {
			m.editor.message = "priority must be a positive number"
			return
		}
		item := config.KeyConfig{ID: values[0], ProviderID: values[1], ModelID: values[2], Name: values[3], Value: values[4], ValueEnv: values[5], Status: defaultText(values[6], "active"), Priority: priority}
		if form.index >= 0 {
			m.cfg.Keys[form.index] = item
		} else {
			m.cfg.Keys = append(m.cfg.Keys, item)
			m.editor.selected = len(m.cfg.Keys) - 1
			m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
		}
	default:
		timeout, err := strconv.Atoi(defaultText(values[6], "120"))
		if err != nil || timeout <= 0 {
			m.editor.message = "timeout must be a positive number"
			return
		}
		item := config.ProviderConfig{ID: values[0], Name: values[1], Type: defaultText(values[2], "openai-compatible"), BaseURL: values[3], AuthType: defaultText(values[4], "bearer"), AuthHeaderName: values[5], TimeoutSeconds: timeout, Enabled: parseBool(values[7])}
		if form.index >= 0 {
			m.cfg.Providers[form.index] = item
		} else {
			m.cfg.Providers = append(m.cfg.Providers, item)
			m.editor.selected = len(m.cfg.Providers) - 1
			m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
		}
	}
	m.editor.mode = editorModeBrowse
	m.editor.dirty = true
	m.editor.message = "changed; press s to save"
}

func (m *model) startDeleteConfigItem() {
	if !m.hasVisibleConfigSelection() {
		m.editor.message = "nothing selected"
		return
	}
	idx := m.editor.selected
	if idx < 0 || idx >= m.configSectionLen() {
		m.editor.message = "nothing selected"
		return
	}
	title, impact := m.deleteImpact(m.editor.section, idx)
	m.editor.mode = editorModeDelete
	m.editor.confirm = deleteConfirmState{title: title, impact: impact, kind: m.editor.section, index: idx}
}

func (m *model) deleteImpact(section configSection, index int) (string, string) {
	switch section {
	case configSectionModels:
		item := m.cfg.Models[index]
		keys := countKeysForModel(m.cfg, item.ID)
		groups := countGroupsForModel(m.cfg, item.ID)
		return "Delete model: " + item.ID, fmt.Sprintf("Impact:\n- %d API keys will be deleted\n- removed from %d groups", keys, groups)
	case configSectionGroups:
		item := m.cfg.ModelGroups[index]
		return "Delete group: " + item.ID, "Impact:\n- group will be deleted\n- models and keys remain"
	case configSectionKeys:
		item := m.cfg.Keys[index]
		return "Delete key: " + item.ID, "Impact:\n- key will be deleted"
	default:
		item := m.cfg.Providers[index]
		models := modelsForProvider(m.cfg, item.ID)
		keys := 0
		for _, modelID := range models {
			keys += countKeysForModel(m.cfg, modelID)
		}
		return "Delete provider: " + item.ID, fmt.Sprintf("Impact:\n- %d models will be deleted\n- %d API keys will be deleted\n- related group members will be removed", len(models), keys)
	}
}

func (m *model) applyDeleteConfigItem() {
	section := m.editor.confirm.kind
	index := m.editor.confirm.index
	switch section {
	case configSectionModels:
		modelID := m.cfg.Models[index].ID
		m.cfg.Models = append(m.cfg.Models[:index], m.cfg.Models[index+1:]...)
		removeKeysForModels(m.cfg, map[string]struct{}{modelID: {}})
		removeModelsFromGroups(m.cfg, map[string]struct{}{modelID: {}})
	case configSectionGroups:
		m.cfg.ModelGroups = append(m.cfg.ModelGroups[:index], m.cfg.ModelGroups[index+1:]...)
	case configSectionKeys:
		m.cfg.Keys = append(m.cfg.Keys[:index], m.cfg.Keys[index+1:]...)
	default:
		providerID := m.cfg.Providers[index].ID
		modelIDs := map[string]struct{}{}
		for _, model := range m.cfg.Models {
			if model.ProviderID == providerID {
				modelIDs[model.ID] = struct{}{}
			}
		}
		m.cfg.Providers = append(m.cfg.Providers[:index], m.cfg.Providers[index+1:]...)
		filterModelsByProvider(m.cfg, providerID)
		removeKeysForModels(m.cfg, modelIDs)
		removeModelsFromGroups(m.cfg, modelIDs)
	}
	m.editor.selected = clampIndex(m.editor.selected, m.configSectionLen())
	m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
	m.editor.mode = editorModeBrowse
	m.editor.dirty = true
	m.editor.message = "deleted; press s to save"
}

func (m *model) toggleConfigItem() {
	if !m.hasVisibleConfigSelection() {
		return
	}
	idx := m.editor.selected
	if idx < 0 || idx >= m.configSectionLen() {
		return
	}
	switch m.editor.section {
	case configSectionModels:
		m.cfg.Models[idx].Enabled = !m.cfg.Models[idx].Enabled
	case configSectionGroups:
		m.cfg.ModelGroups[idx].Enabled = !m.cfg.ModelGroups[idx].Enabled
	case configSectionKeys:
		if m.cfg.Keys[idx].Status == "disabled" {
			m.cfg.Keys[idx].Status = "active"
		} else {
			m.cfg.Keys[idx].Status = "disabled"
		}
	default:
		m.cfg.Providers[idx].Enabled = !m.cfg.Providers[idx].Enabled
	}
	m.editor.dirty = true
	m.editor.sectionSelected[int(m.editor.section)] = m.editor.selected
	m.editor.message = "changed; press s to save"
}

func (m *model) saveDraftConfig() {
	if err := m.cfg.Validate(); err != nil {
		m.editor.message = "save failed: " + err.Error()
		return
	}
	if m.saveConfig != nil {
		if err := m.saveConfig(m.cfg); err != nil {
			m.editor.message = "save failed: " + err.Error()
			return
		}
	}
	if m.reloadRouter != nil {
		router, err := m.reloadRouter(m.cfg)
		if err != nil {
			m.editor.message = "saved, reload failed: " + err.Error()
			return
		}
		m.router = router
	}
	m.savedCfg = cloneConfig(m.cfg)
	m.editor.dirty = false
	m.editor.message = "saved and reloaded"
}

func (m *model) reloadDraftConfig() {
	if m.editor.dirty {
		m.editor.message = "unsaved changes discarded"
	}
	m.cfg = cloneConfig(m.savedCfg)
	m.editor.mode = editorModeBrowse
	m.editor.dirty = false
}

func formValues(fields []formField) []string {
	values := make([]string, len(fields))
	for i, field := range fields {
		values[i] = strings.TrimSpace(field.value)
	}
	return values
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "y", "1", "enabled", "active":
		return true
	default:
		return false
	}
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func parseGroupMembers(value string) []config.ModelGroupMemberConfig {
	parts := strings.Split(value, ",")
	members := make([]config.ModelGroupMemberConfig, 0, len(parts))
	for _, part := range parts {
		modelID := strings.TrimSpace(part)
		if modelID == "" {
			continue
		}
		members = append(members, config.ModelGroupMemberConfig{ModelID: modelID, Priority: len(members) + 1, Weight: 1, Enabled: true})
	}
	return members
}

func groupMembersText(members []config.ModelGroupMemberConfig) string {
	items := make([]string, 0, len(members))
	for _, member := range members {
		items = append(items, member.ModelID)
	}
	return strings.Join(items, ",")
}

func cloneConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return config.Default()
	}
	out := *cfg
	out.Providers = append([]config.ProviderConfig(nil), cfg.Providers...)
	out.Models = append([]config.ModelConfig(nil), cfg.Models...)
	out.Keys = append([]config.KeyConfig(nil), cfg.Keys...)
	out.ModelGroups = make([]config.ModelGroupConfig, len(cfg.ModelGroups))
	for i, group := range cfg.ModelGroups {
		out.ModelGroups[i] = group
		out.ModelGroups[i].Members = append([]config.ModelGroupMemberConfig(nil), group.Members...)
	}
	return &out
}

func modelsForProvider(cfg *config.Config, providerID string) []string {
	var out []string
	for _, model := range cfg.Models {
		if model.ProviderID == providerID {
			out = append(out, model.ID)
		}
	}
	return out
}

func countKeysForModel(cfg *config.Config, modelID string) int {
	count := 0
	for _, key := range cfg.Keys {
		if key.ModelID == modelID {
			count++
		}
	}
	return count
}

func countGroupsForModel(cfg *config.Config, modelID string) int {
	count := 0
	for _, group := range cfg.ModelGroups {
		for _, member := range group.Members {
			if member.ModelID == modelID {
				count++
				break
			}
		}
	}
	return count
}

func removeKeysForModels(cfg *config.Config, modelIDs map[string]struct{}) {
	keys := cfg.Keys[:0]
	for _, key := range cfg.Keys {
		if _, remove := modelIDs[key.ModelID]; remove {
			continue
		}
		keys = append(keys, key)
	}
	cfg.Keys = keys
}

func removeModelsFromGroups(cfg *config.Config, modelIDs map[string]struct{}) {
	for i := range cfg.ModelGroups {
		members := cfg.ModelGroups[i].Members[:0]
		for _, member := range cfg.ModelGroups[i].Members {
			if _, remove := modelIDs[member.ModelID]; remove {
				continue
			}
			member.Priority = len(members) + 1
			members = append(members, member)
		}
		cfg.ModelGroups[i].Members = members
		if len(members) == 0 {
			cfg.ModelGroups[i].Enabled = false
		}
	}
}

func filterModelsByProvider(cfg *config.Config, providerID string) {
	models := cfg.Models[:0]
	for _, model := range cfg.Models {
		if model.ProviderID == providerID {
			continue
		}
		models = append(models, model)
	}
	cfg.Models = models
}

func clampIndex(index, length int) int {
	if length <= 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	if index < 0 {
		return 0
	}
	return index
}

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
