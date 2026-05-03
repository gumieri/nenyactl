package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	screenMode screen = iota
	screenList
	screenEdit
	screenPicker
	screenConfirm
)

type Agent struct {
	Name     string
	Strategy string
	Models   []string
}

type model struct {
	ID       string
	Provider string
	Selected bool
}

type tuiModel struct {
	screen       screen
	modeAuto     bool
	agents       []Agent
	cursor       int
	fieldCursor  int
	nameInput    textinput.Model
	strategyIdx  int
	models       []model
	filter       string
	filterInput  textinput.Model
	deleteIdx    int
	quitConfirm  bool
	done         bool
}

func newTUIModel() tuiModel {
	m := tuiModel{
		modeAuto: true,
		agents:   nil,
		cursor:   0,
	}
	m.filterInput = textinput.New()
	m.filterInput.Placeholder = "Filter models..."
	m.filterInput.CharLimit = 50
	m.filterInput.Width = 40
	return m
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.screen {
		case screenMode:
			return m.updateMode(msg)
		case screenList:
			return m.updateList(msg)
		case screenEdit:
			return m.updateEdit(msg)
		case screenPicker:
			return m.updatePicker(msg)
		case screenConfirm:
			return m.updateConfirm(msg)
		}
	}
	return m, nil
}

func (m *tuiModel) updateMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.done = true
		return m, tea.Quit
	case "enter":
		if m.modeAuto {
			m.done = true
			return m, tea.Quit
		}
		m.screen = screenList
		m.loadDefaults()
		return m, nil
	case " ":
		m.modeAuto = !m.modeAuto
		return m, nil
	case "up", "k":
		m.cursor = 0
		return m, nil
	case "down", "j":
		m.cursor = 1
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.done = true
		return m, tea.Quit
	case "enter":
		if m.cursor < len(m.agents) {
			m.startEdit(m.cursor)
		} else {
			m.startNew()
		}
		return m, nil
	case "d":
		if m.cursor < len(m.agents) {
			m.screen = screenConfirm
			m.deleteIdx = m.cursor
			m.quitConfirm = false
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.agents) {
			m.cursor++
		}
		return m, nil
	case "a":
		m.startNew()
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.screen = screenList
		return m, nil
	case "enter":
		if m.fieldCursor == 0 {
			m.fieldCursor = 1
			return m, nil
		} else if m.fieldCursor == 1 {
			m.fieldCursor = 2
			return m, nil
		} else {
			m.screen = screenPicker
			m.filter = ""
			m.filterInput.Reset()
			m.loadModels()
			return m, m.filterInput.Focus()
		}
	case "tab":
		m.fieldCursor = (m.fieldCursor + 1) % 3
		if m.fieldCursor == 2 {
			return m, nil
		}
		return m, nil
	case "shift+tab":
		m.fieldCursor = (m.fieldCursor - 1 + 3) % 3
		if m.fieldCursor == 2 {
			return m, nil
		}
		return m, nil
	case "up", "down":
		if m.fieldCursor == 1 {
			if msg.String() == "up" && m.strategyIdx > 0 {
				m.strategyIdx--
			}
			if msg.String() == "down" && m.strategyIdx < len(Strategies)-1 {
				m.strategyIdx++
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m *tuiModel) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.screen = screenEdit
		return m, nil
	case "enter":
		if m.filter != "" {
			customModel := strings.TrimSpace(m.filter)
			if customModel != "" {
				m.agents[m.cursor].Models = append(m.agents[m.cursor].Models, customModel)
			}
		}
		m.screen = screenEdit
		return m, nil
	case " ":
		if m.cursor < len(m.models) {
			m.models[m.cursor].Selected = !m.models[m.cursor].Selected
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.models) {
			m.cursor++
		}
		return m, nil
	case "backspace":
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
			m.filterInput.Reset()
			m.filterInput.SetValue(m.filter)
			m.loadModels()
		}
		return m, nil
	}

	m.filterInput, _ = m.filterInput.Update(msg)
	if m.filter != m.filterInput.Value() {
		m.filter = m.filterInput.Value()
		m.loadModels()
	}

	var cmd tea.Cmd
	return m, cmd
}

func (m *tuiModel) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		if m.deleteIdx >= 0 && m.deleteIdx < len(m.agents) {
			m.agents = append(m.agents[:m.deleteIdx], m.agents[m.deleteIdx+1:]...)
		}
		m.screen = screenList
		m.deleteIdx = -1
		return m, nil
	case "n", "esc":
		m.screen = screenList
		m.deleteIdx = -1
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) loadDefaults() {
	m.agents = nil
	for name, def := range DefaultAgents {
		m.agents = append(m.agents, Agent{
			Name:     name,
			Strategy: def.Strategy,
			Models:   append([]string{}, def.Models...),
		})
	}
	m.cursor = 0
}

func (m *tuiModel) startEdit(idx int) {
	m.screen = screenEdit
	m.cursor = idx
	m.nameInput = textinput.New()
	m.nameInput.Placeholder = "agent-name"
	m.nameInput.CharLimit = 64
	m.nameInput.Width = 40
	m.nameInput.SetValue(m.agents[idx].Name)
	m.nameInput.Focus()

	for i, s := range Strategies {
		if m.agents[idx].Strategy == s {
			m.strategyIdx = i
			break
		}
	}
	m.fieldCursor = 0
}

func (m *tuiModel) startNew() {
	m.screen = screenEdit
	m.cursor = len(m.agents)
	m.agents = append(m.agents, Agent{Name: "new-agent", Strategy: "fallback", Models: nil})
	m.nameInput = textinput.New()
	m.nameInput.Placeholder = "agent-name"
	m.nameInput.CharLimit = 64
	m.nameInput.Width = 40
	m.nameInput.SetValue("new-agent")
	m.nameInput.Focus()
	m.strategyIdx = 0
	m.fieldCursor = 0
}

func (m *tuiModel) loadModels() {
	m.models = nil
	for _, def := range Models {
		name := def.ID
		if m.filter != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(m.filter)) {
			continue
		}
		sel := false
		if m.screen == screenPicker && m.cursor < len(m.agents) {
			for _, mdl := range m.agents[m.cursor].Models {
				if mdl == name || mdl == def.Provider+"/"+name {
					sel = true
					break
				}
			}
		}
		m.models = append(m.models, model{ID: name, Provider: def.Provider, Selected: sel})
	}
	m.cursor = 0
}

func (m tuiModel) View() string {
	if m.done {
		return ""
	}

	switch m.screen {
	case screenMode:
		return m.viewMode()
	case screenList:
		return m.viewList()
	case screenEdit:
		return m.viewEdit()
	case screenPicker:
		return m.viewPicker()
	case screenConfirm:
		return m.viewConfirm()
	}
	return ""
}

func (m tuiModel) viewMode() string {
	s := header("Configure Agents") + "\n"
	s += dim("How would you like to configure agents?\n\n")

	cursor := "  "
	autoSel := "[ ]"
	manualSel := "[ ]"
	if m.cursor == 0 {
		cursor = "> "
		autoSel = "[x]"
	}
	if m.cursor == 1 {
		cursor = "  "
		manualSel = "[x]"
	}
	if m.modeAuto {
		autoSel = "[x]"
		cursor = "  "
	} else {
		manualSel = "[x]"
	}

	s += fmt.Sprintf("%s%s Enable auto-agents (recommended)\n", cursor, autoSel)
	s += dim("    auto_fast, auto_reasoning, auto_vision, auto_coding, auto_large, auto_balanced\n")
	if !m.modeAuto {
		s += "\n"
		s += fmt.Sprintf("  %s Configure manually (defaults: small, build, plan)\n", manualSel)
	}

	s += "\n" + dim("↑/↓ Navigate · Space toggle · Enter confirm")
	return s
}

func (m tuiModel) viewList() string {
	s := header("Configure Agents") + "\n"
	s += dim("Agents: use ↑/↓ to select · Enter to edit · d to delete · a to add new\n\n")

	for i, a := range m.agents {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		modelsStr := strings.Join(a.Models, ", ")
		if modelsStr == "" {
			modelsStr = dim("(no models)")
		}
		s += fmt.Sprintf("%s%s  %s  [%s]\n    %s\n",
			cursor,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render(a.Name),
			lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(a.Strategy),
			modelsStr,
			dim(fmt.Sprintf("%d models", len(a.Models))),
		)
	}

	cursor := "  "
	if m.cursor == len(m.agents) {
		cursor = "> "
	}
	s += fmt.Sprintf("%s%s\n", cursor, lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render("+ Add new agent"))

	s += "\n" + dim("Esc to go back · Ctrl+C to exit and save")
	return s
}

func (m tuiModel) viewEdit() string {
	s := header(fmt.Sprintf("Edit Agent (%d/%d)", m.cursor+1, len(m.agents))) + "\n"
	s += dim("Name, Strategy, Models · Tab to navigate · Esc to go back\n\n")

	nameCursor := ""
	stratCursor := ""
	modelCursor := ""
	if m.fieldCursor == 0 {
		nameCursor = ">"
	}
	if m.fieldCursor == 1 {
		stratCursor = ">"
	}
	if m.fieldCursor == 2 {
		modelCursor = ">"
	}

	s += fmt.Sprintf("%s Name: %s\n", nameCursor, m.nameInput.View())

	s += fmt.Sprintf("%s Strategy:\n", stratCursor)
	for _, strat := range Strategies {
		sel := "  "
		if strat == Strategies[m.strategyIdx] {
			sel = "→ "
		}
		s += fmt.Sprintf("    %s%s\n", sel, lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(strat))
	}
	if m.fieldCursor == 1 {
		s += dim("     ↑/↓ to change\n")
	}

	modelsStr := "  (none)"
	if len(m.agents[m.cursor].Models) > 0 {
		modelsStr = strings.Join(m.agents[m.cursor].Models, ", ")
	}
	s += fmt.Sprintf("%s Models: %s\n    %s%s\n",
		modelCursor,
		dim("Enter to select models"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(trunc(modelsStr, 60)),
		dim(fmt.Sprintf(" [%d]", len(m.agents[m.cursor].Models))),
	)

	s += "\n" + dim("Enter to confirm name/strategy · Esc to go back")
	return s
}

func (m tuiModel) viewPicker() string {
	s := header("Select Models") + "\n"
	s += dim("Space to toggle · Type to filter · Enter to finish · Esc to go back\n\n")

	if len(m.agents) > m.cursor {
		s += fmt.Sprintf("%s: %s\n\n",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render(m.agents[m.cursor].Name),
			dim("selected: "+strings.Join(m.agents[m.cursor].Models, ", ")),
		)
	}

	s += m.filterInput.View() + "\n\n"

	shown := 0
	for _, mod := range m.models {
		if shown >= 30 {
			s += dim("  ... (more models, type to filter)\n")
			break
		}
		cursor := "  "
		if m.cursor == shown {
			cursor = "> "
		}
		sel := " "
		if mod.Selected {
			sel = "x"
		}
		s += fmt.Sprintf("%s[%s] %s  %s\n",
			cursor,
			lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(sel),
			lipgloss.NewStyle().Bold(true).Render(mod.ID),
			dim(mod.Provider),
		)
		shown++
	}

	return s
}

func (m tuiModel) viewConfirm() string {
	name := ""
	if m.deleteIdx >= 0 && m.deleteIdx < len(m.agents) {
		name = m.agents[m.deleteIdx].Name
	}
	s := header("Delete Agent") + "\n\n"
	s += fmt.Sprintf("Delete '%s'? [y/N]\n", lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Render(name))
	s += "\n" + dim("y or Enter to confirm · n or Esc to cancel")
	return s
}

func header(s string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render(s)
}

func dim(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(s)
}

func trunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func RunAgentEditor() (bool, map[string]any, error) {
	p := tea.NewProgram(newTUIModel())
	m, err := p.Run()
	if err != nil {
		return false, nil, err
	}

	tm, ok := m.(tuiModel)
	if !ok {
		return false, nil, nil
	}

	if tm.done && tm.modeAuto {
		return true, nil, nil
	}

	if tm.done || len(tm.agents) == 0 {
		return false, nil, nil
	}

	agentsMap := make(map[string]map[string]any)
	for _, a := range tm.agents {
		agentsMap[a.Name] = map[string]any{
			"strategy": a.Strategy,
			"models":   a.Models,
		}
	}

	cfg := map[string]any{
		"agents":    agentsMap,
		"discovery": map[string]any{"auto_agents": false},
	}

	return false, cfg, nil
}

func WriteAgentsConfig(dir string, cfg map[string]any) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	configD := filepath.Join(dir, "config.d")
	if err := os.MkdirAll(configD, 0o755); err != nil {
		return err
	}

	path := filepath.Join(configD, "20-agents.json")
	return os.WriteFile(path, data, 0o644)
}

func UpdateConfigDiscovery(dir string, autoAgents bool) error {
	configPath := filepath.Join(dir, "config", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		var raw map[string]json.RawMessage
		if err2 := json.Unmarshal(data, &raw); err2 != nil {
			return err2
		}
		cfg = make(map[string]any)
		for k, v := range raw {
			cfg[k] = v
		}
	}

	if cfg["discovery"] == nil {
		cfg["discovery"] = make(map[string]any)
	}
	if disco, ok := cfg["discovery"].(map[string]any); ok {
		disco["auto_agents"] = autoAgents
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0o644)
}

func init() {
	sort.Slice(Models, func(i, j int) bool {
		if Models[i].Provider != Models[j].Provider {
			return Models[i].Provider < Models[j].Provider
		}
		return Models[i].ID < Models[j].ID
	})
}