package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gumieri/nenyactl/internal/agents"
	"github.com/gumieri/nenyactl/internal/jsonc"
	"github.com/gumieri/nenyactl/internal/tui"
	"github.com/tailscale/hujson"
)

type configScreen int

const (
	screenSections configScreen = iota
	screenKeys
	screenEdit
	screenAgents
)

type sectionInfo struct {
	name    string
	isAgent bool
	source  string // file origin, empty for agent section
}

type configEntry struct {
	Key   string
	Value *hujson.Value
}

type agentEntry struct {
	Name     string
	Strategy string
	Models   []string
}

type configModel struct {
	screen        configScreen
	config        *hujson.Value
	sections      []sectionInfo
	cursor        int
	entries       []configEntry
	editKey       string
	editInput     textinput.Model
	activeSection string

	// multi-file tracking
	sources map[string]sourceInfo
	dirty   map[string]bool

	// agents section state
	agents          []agentEntry
	agentsModeAuto  bool
	agentsFile      string
	agentsDirty     bool
	agentCursor     int
	editName        textinput.Model
	modelFilter     textinput.Model
	strategyIdx     int
	editCursor      int
	modelFilterFocused bool

	sectionsView  viewport.Model
	keysView      viewport.Model
	agentsView    viewport.Model
	pickerView    viewport.Model
	width, height int
	helpModel     help.Model
	helpKM        tui.KeyMap

	saved bool
	quit  bool
}

func newConfigModel(cfg *hujson.Value, configFile, configD string) configModel {
	sections := jsonc.TopLevelKeys(cfg)

	// Build sources map for all top-level keys
	sources := make(map[string]sourceInfo)
	for _, key := range sections {
		if v, ok := jsonc.GetField(cfg, key); ok {
			sources[key] = sourceInfo{
				filePath: configFile,
				value:    v,
			}
		}
	}

	m := configModel{
		screen:       screenSections,
		config:       cfg,
		sections:     make([]sectionInfo, len(sections)+1), // +1 for agents section
		cursor:       0,
		editInput:    textinput.New(),
		sectionsView: viewport.New(0, 0),
		keysView:     viewport.New(0, 0),
		agentsView:   viewport.New(0, 0),
		pickerView:   viewport.New(0, 0),
		helpModel:    tui.NewHelpModel(),
		helpKM:       tui.ListKeyMap,

		// agents state
		agentsFile: filepath.Join(configD, "20-agents.json"),

		sources: sources,
		dirty:   make(map[string]bool),
	}
	m.editInput.CharLimit = 256
	m.editInput.Width = 50

	// Fill sections array
	for i, key := range sections {
		m.sections[i] = sectionInfo{
			name:    key,
			isAgent: false,
			source:  configFile,
		}
	}
	// Add agents section at the end
	m.sections[len(sections)] = sectionInfo{
		name:    "agents",
		isAgent: true,
		source:  "", // special section
	}

	// Initialize agents UI state
	m.editName = textinput.New()
	m.editName.Placeholder = "agent-name"
	m.editName.CharLimit = 64
	m.editName.Width = 40

	m.modelFilter = textinput.New()
	m.modelFilter.Placeholder = "Filter models..."
	m.modelFilter.CharLimit = 50
	m.modelFilter.Width = 40

	// Load agents from file if exists
	if data, err := os.ReadFile(m.agentsFile); err == nil {
		var agentsMap map[string]any
		if err := json.Unmarshal(data, &agentsMap); err == nil {
			if auto, ok := agentsMap["auto_agents"].(bool); ok {
				m.agentsModeAuto = auto
			}
			if agentsList, ok := agentsMap["agents"].(map[string]any); ok {
				for name, agentAny := range agentsList {
					if agentMap, ok := agentAny.(map[string]any); ok {
						strategy := "fallback"
						if s, ok := agentMap["strategy"].(string); ok {
							strategy = s
						}
						models := []string{}
						if m, ok := agentMap["models"].([]any); ok {
							for _, modelAny := range m {
								if modelStr, ok := modelAny.(string); ok {
									models = append(models, modelStr)
								}
							}
						}
						m.agents = append(m.agents, agentEntry{
							Name:     name,
							Strategy: strategy,
							Models:   models,
						})
					}
				}
			}
		}
	}

	// Initialize models list for picker
	m.loadModelsFromAgents()

	return m
}

func (m configModel) Init() tea.Cmd { return nil }

func (m configModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		h := msg.Height - 6
		if h < 8 {
			h = 8
		}
		m.sectionsView.Width = msg.Width - 6
		m.sectionsView.Height = h
		m.keysView.Width = msg.Width - 6
		m.keysView.Height = h

	case tea.KeyMsg:
		switch m.screen {
		case screenSections:
			return m.updateSections(msg)
		case screenKeys:
			return m.updateKeys(msg)
		case screenEdit:
			return m.updateEdit(msg)
		case screenAgents:
			return m.updateAgents(msg)
		}
	}

	switch m.screen {
	case screenEdit:
		m.editInput, _ = m.editInput.Update(msg)
	case screenAgents:
		m.editName, _ = m.editName.Update(msg)
		if m.modelFilterFocused {
			m.modelFilter, _ = m.modelFilter.Update(msg)
		}
	}

	return m, nil
}

func (m *configModel) updateSections(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.quit = true
		return m, tea.Quit
	case "enter":
		if m.cursor < len(m.sections) {
			section := m.sections[m.cursor]
			if section.isAgent {
				m.screen = screenAgents
				m.agentCursor = 0
				m.updateAgentsContent()
			} else {
				m.loadSection(section.name)
				m.screen = screenKeys
			}
		}
		return m, nil
	case "s":
		m.saved = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.scrollSections()
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.sections)-1 {
			m.cursor++
			m.scrollSections()
		}
		return m, nil
	}
	return m, nil
}

func (m *configModel) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.screen = screenSections
		m.cursor = 0
		m.scrollSections()
		return m, nil
	case "enter":
		if m.cursor < len(m.entries) {
			m.startEdit(m.cursor)
			m.screen = screenEdit
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.scrollKeys()
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			m.scrollKeys()
		}
		return m, nil
	}
	return m, nil
}

func (m *configModel) loadModelsFromAgents() {
	sort.Slice(agents.Models, func(i, j int) bool {
		if agents.Models[i].Provider != agents.Models[j].Provider {
			return agents.Models[i].Provider < agents.Models[j].Provider
		}
		return agents.Models[i].ID < agents.Models[j].ID
	})
}

func (m *configModel) updateAgents(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.screen = screenSections
		m.cursor = 0
		m.scrollSections()
		return m, nil
	case "s":
		m.saved = true
		m.agentsDirty = false
		return m, tea.Quit
	case "enter":
		m.startEditAgent()
		return m, nil
	case "a":
		m.agents = append(m.agents, agentEntry{Name: "new-agent", Strategy: "fallback"})
		m.agentCursor = len(m.agents) - 1
		m.startEditAgent()
		return m, nil
	case "d":
		if m.agentCursor >= 0 && m.agentCursor < len(m.agents) {
			m.agents = append(m.agents[:m.agentCursor], m.agents[m.agentCursor+1:]...)
			m.agentsDirty = true
			if m.agentCursor >= len(m.agents) && m.agentCursor > 0 {
				m.agentCursor--
			}
			m.updateAgentsContent()
		}
		return m, nil
	case "up", "k":
		if m.agentCursor > 0 {
			m.agentCursor--
			m.scrollAgents()
		}
		return m, nil
	case "down", "j":
		if m.agentCursor < len(m.agents) {
			m.agentCursor++
			m.scrollAgents()
		}
		return m, nil
	}
	return m, nil
}

func (m *configModel) loadSection(sectionName string) {
	m.activeSection = sectionName
	field, ok := jsonc.GetField(m.config, sectionName)
	if !ok {
		m.entries = nil
		return
	}

	obj, ok := field.Value.(*hujson.Object)
	if !ok {
		m.entries = []configEntry{{Key: sectionName, Value: field}}
		return
	}

	m.entries = make([]configEntry, len(obj.Members))
	for i := range obj.Members {
		m.entries[i] = configEntry{
			Key:   obj.Members[i].Name.Value.(hujson.Literal).String(),
			Value: &obj.Members[i].Value,
		}
	}
	m.cursor = 0
	m.scrollKeys()
}

func (m *configModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.screen = screenKeys
		return m, nil
	case "enter":
		m.applyEdit()
		m.screen = screenKeys
		return m, nil
	}
	return m, nil
}

func (m *configModel) startEdit(idx int) {
	if idx >= len(m.entries) {
		return
	}
	entry := m.entries[idx]
	m.editKey = entry.Key
	m.editInput.SetValue(jsonc.FieldValueString(entry.Value))
	m.editInput.CursorEnd()
	m.editInput.Focus()
}

func (m *configModel) applyEdit() {
	if len(m.entries) == 0 {
		return
	}

	entry := m.entries[m.cursor]
	raw := m.editInput.Value()

	if _, ok := entry.Value.Value.(*hujson.Object); ok {
		return
	}
	if _, ok := entry.Value.Value.(*hujson.Array); ok {
		return
	}

	entry.Value.Value = parseLiteralValue(raw)

	// Mark the source file as dirty
	if src, ok := m.sources[entry.Key]; ok {
		m.dirty[src.filePath] = true
	}
}

func (m *configModel) scrollSections() {
	visible := m.sectionsView.Height
	if visible <= 0 {
		return
	}
	half := visible / 2
	if m.cursor < half {
		m.sectionsView.YOffset = 0
	} else if m.cursor > len(m.sections)-1-half {
		m.sectionsView.YOffset = len(m.sections) - visible
		if m.sectionsView.YOffset < 0 {
			m.sectionsView.YOffset = 0
		}
	} else {
		m.sectionsView.YOffset = m.cursor - half
	}
	m.updateSectionsContent()
}

func (m *configModel) scrollKeys() {
	visible := m.keysView.Height
	if visible <= 0 {
		return
	}
	half := visible / 2
	if m.cursor < half {
		m.keysView.YOffset = 0
	} else if m.cursor > len(m.entries)-1-half {
		m.keysView.YOffset = len(m.entries) - visible
		if m.keysView.YOffset < 0 {
			m.keysView.YOffset = 0
		}
	} else {
		m.keysView.YOffset = m.cursor - half
	}
	m.updateKeysContent()
}

func (m *configModel) scrollAgents() {
	visible := m.agentsView.Height
	if visible <= 0 {
		return
	}
	half := visible / 2
	total := len(m.agents) + 1
	if m.agentCursor < half {
		m.agentsView.YOffset = 0
	} else if m.agentCursor > total-1-half {
		m.agentsView.YOffset = total - visible
		if m.agentsView.YOffset < 0 {
			m.agentsView.YOffset = 0
		}
	} else {
		m.agentsView.YOffset = m.agentCursor - half
	}
	m.updateAgentsContent()
}

func (m configModel) View() string {
	theme := tui.Current()

	if m.quit || m.saved {
		return ""
	}

	var content string
	switch m.screen {
	case screenSections:
		content = m.viewSections()
	case screenKeys:
		content = m.viewKeys()
	case screenEdit:
		content = m.viewEdit()
	case screenAgents:
		content = m.viewAgents()
	}

	helpView := m.helpModel.View(m.helpKM)

	out := lipgloss.JoinVertical(lipgloss.Top,
		content,
		"",
		helpView,
	)

	return theme.App.Render(out)
}

func (m configModel) viewSections() string {
	theme := tui.Current()

	title := theme.Title.Render("Edit Configuration")
	var fileInfo string
	if len(m.sections) > 0 {
		fileInfo = theme.Dimmed.Render("Enter to expand section · s to save · q to quit")
	}

	body := lipgloss.JoinVertical(lipgloss.Top, title, "", fileInfo, "", m.sectionsView.View())

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

func (m configModel) viewKeys() string {
	theme := tui.Current()

	title := theme.Title.Render(fmt.Sprintf("Edit: %s", m.activeSection))
	subtitle := theme.Dimmed.Render("Enter to save value · Esc to go back")

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		subtitle,
		"",
		m.keysView.View(),
	)

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

func (m configModel) viewEdit() string {
	theme := tui.Current()

	title := theme.Title.Render(fmt.Sprintf("Edit: %s", m.editKey))

	inputView := m.editInput.View()
	if m.editInput.Focused() {
		inputView = theme.InputFocused.Render(inputView)
	} else {
		inputView = theme.InputBlurred.Render(inputView)
	}

	currentVal := theme.Dimmed.Render("Current: " + jsonc.FieldValueString(m.entries[m.cursor].Value))

	body := lipgloss.JoinVertical(lipgloss.Top, title, "", currentVal, "", inputView, "", theme.Dimmed.Render("Enter to save · Esc to cancel"))

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

func (m configModel) viewAgents() string {
	theme := tui.Current()

	var autoLabel string
	var autoStyle lipgloss.Style
	if m.agentsModeAuto {
		autoLabel = "ON"
		autoStyle = theme.Success
	} else {
		autoLabel = "OFF"
		autoStyle = theme.Error
	}

	title := theme.Title.Render("Configure Agents")
	autoLine := fmt.Sprintf("Auto-agents: %s  (Space to toggle, Enter to configure manually)",
		autoStyle.Bold(true).Render(autoLabel))

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		autoLine,
		"",
		m.agentsView.View(),
	)

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

func (m configModel) renderSections() string {
	theme := tui.Current()
	var lines []string

	for i, section := range m.sections {
		var prefix string
		style := theme.Body
		if i == m.cursor {
			prefix = "● "
			style = theme.Highlight
		} else {
			prefix = "  "
		}

		typeHint := ""
		if section.isAgent {
			count := len(m.agents)
			typeHint = theme.Dimmed.Render(fmt.Sprintf("  %d agent(s)", count))
		} else {
			field, ok := jsonc.GetField(m.config, section.name)
			if ok {
				switch field.Value.(type) {
				case *hujson.Object:
					typeHint = theme.Dimmed.Render("  {object}")
				case *hujson.Array:
					typeHint = theme.Dimmed.Render("  [array]")
				default:
					val := jsonc.FieldValueString(field)
					if len(val) > 50 {
						val = val[:47] + "..."
					}
					typeHint = theme.Dimmed.Render("  " + val)
				}
			}
		}

		nameStyle := theme.Accent
		if i == m.cursor {
			nameStyle = nameStyle.Bold(true)
		}

		name := section.name
		if section.isAgent {
			nameStyle = theme.Success
		}

		line := fmt.Sprintf("%s%s%s", style.Render(prefix), nameStyle.Render(name), typeHint)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m configModel) renderAgents() string {
	theme := tui.Current()
	var lines []string

	for i, a := range m.agents {
		var prefix string
		style := theme.Body
		if i == m.agentCursor {
			prefix = "● "
			style = theme.Highlight
		} else {
			prefix = "  "
		}

		modelsStr := strings.Join(a.Models, ", ")
		if modelsStr == "" {
			modelsStr = "(no models)"
		}

		nameStyle := theme.Accent
		if i == m.agentCursor {
			nameStyle = nameStyle.Bold(true)
		}

		name := nameStyle.Render(a.Name)
		strategy := theme.Dimmed.Render(a.Strategy)
		modelsLine := theme.Dimmed.Render(fmt.Sprintf("  %d models: %s", len(a.Models), trunc(modelsStr, 50)))

		line := fmt.Sprintf("%s%s  %s\n    %s",
			style.Render(prefix),
			name,
			strategy,
			modelsLine,
		)
		lines = append(lines, line)
	}

	addPrefix := "  "
	if m.agentCursor == len(m.agents) {
		addPrefix = "● "
	}
	addLabel := theme.Accent.Render(addPrefix + "+ Add new agent")
	lines = append(lines, addLabel)

	return strings.Join(lines, "\n")
}

func (m *configModel) updateAgentsContent() {
	m.agentsView.SetContent(m.renderAgents())
}

func (m *configModel) updateSectionsContent() {
	m.sectionsView.SetContent(m.renderSections())
}

func (m *configModel) updateKeysContent() {
	m.keysView.SetContent(m.renderKeys())
}

func isSectionObject(v *hujson.Value) bool {
	_, ok := v.Value.(*hujson.Object)
	return ok
}

type EditorResult struct {
	ConfigFile string
	ConfigD    string
	AgentsFile string
	Config     *hujson.Value
	Agents     []agentEntry
	Dirty      map[string]bool
}

func (m *configModel) startEditAgent() {
	m.screen = screenEdit
	m.editCursor = 0
	m.editName.SetValue("")
	if m.agentCursor >= 0 && m.agentCursor < len(m.agents) {
		m.editName.SetValue(m.agents[m.agentCursor].Name)
	}
	m.editName.Focus()

	for i, s := range agents.Strategies {
		if m.agentCursor >= 0 && m.agentCursor < len(m.agents) && m.agents[m.agentCursor].Strategy == s {
			m.strategyIdx = i
			break
		}
	}
}

func (m configModel) renderKeys() string {
	theme := tui.Current()
	var lines []string

	for i, entry := range m.entries {
		var prefix string
		style := theme.Body
		if i == m.cursor {
			prefix = "● "
			style = theme.Highlight
		} else {
			prefix = "  "
		}

		keyStyle := theme.Accent
		if i == m.cursor {
			keyStyle = keyStyle.Bold(true)
		}

		val := jsonc.FieldValueString(entry.Value)
		if len(val) > 60 {
			val = val[:57] + "..."
		}

		line := fmt.Sprintf("%s%s  %s", style.Render(prefix), keyStyle.Render(entry.Key), theme.Dimmed.Render(val))
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func RunConfigEditor(configFile, configD string) (*EditorResult, bool, error) {
	cfg, err := jsonc.ReadFile(configFile)
	if err != nil {
		return nil, false, err
	}

	m := newConfigModel(cfg, configFile, configD)
	m.loadDefaults()
	m.updateSectionsContent()

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, false, err
	}

	tm, ok := result.(configModel)
	if !ok {
		return nil, false, nil
	}

	if tm.quit || !tm.saved {
		return nil, false, nil
	}

	resultData := &EditorResult{
		ConfigFile: configFile,
		ConfigD:    configD,
		AgentsFile: m.agentsFile,
		Config:     tm.config,
		Agents:     tm.agents,
		Dirty:      tm.dirty,
	}

	if tm.agentsDirty {
		resultData.Dirty[tm.agentsFile] = true
	}

	return resultData, true, nil
}

func (m *configModel) loadDefaults() {
	m.cursor = 0
	m.scrollSections()
}

func trunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func parseLiteralValue(raw string) hujson.Literal {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return hujson.Literal(`""`)
	}
	switch strings.ToLower(raw) {
	case "true":
		return hujson.Literal("true")
	case "false":
		return hujson.Literal("false")
	case "null":
		return hujson.Literal("null")
	}
	if raw[0] == '"' || raw[0] == '{' || raw[0] == '[' {
		return hujson.Literal(raw)
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return hujson.Literal(raw)
	}
	if _, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return hujson.Literal(raw)
	}
	return hujson.Literal(fmt.Sprintf("%q", raw))
}

func WriteAgentsFile(path string, agents []agentEntry) error {
	agentsMap := make(map[string]any)
	for _, a := range agents {
		agentsMap[a.Name] = map[string]any{
			"strategy": a.Strategy,
			"models":   a.Models,
		}
	}

	data, err := json.MarshalIndent(agentsMap, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
