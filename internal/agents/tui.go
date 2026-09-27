package agents

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gumieri/nenyactl/internal/tui"
)

type screen int

const (
	screenList screen = iota
	screenEdit
	screenPicker
	screenConfirm
)

// defaultStrategy is the strategy a new agent starts with.
const defaultStrategy = "fallback"

// Strategies are the agent routing strategies nenya accepts.
var Strategies = []string{"fallback", "round-robin"}

type Agent struct {
	Name     string
	Strategy string
	Models   []string
}

type modelState struct {
	ID        string
	Provider  string
	Qualified string // provider/model, the selection key
	Selected  bool
}

type tuiModel struct {
	screen   screen
	modeAuto bool
	agents   []Agent
	cursor   int

	// catalog is the authoritative model/provider catalog from nenya describe.
	catalog Catalog

	editName    textinput.Model
	strategyIdx int
	editCursor  int

	models      []modelState
	modelCursor int
	modelFilter textinput.Model

	deleteIdx int

	agentsView    viewport.Model
	pickerView    viewport.Model
	width, height int
	helpModel     help.Model

	saved bool
	done  bool
}

func newTUIModel(catalog Catalog) tuiModel {
	m := tuiModel{
		modeAuto:    true,
		agents:      nil,
		cursor:      0,
		catalog:     catalog,
		editName:    textinput.New(),
		modelFilter: textinput.New(),
		agentsView:  viewport.New(0, 0),
		pickerView:  viewport.New(0, 0),
		helpModel:   tui.NewHelpModel(),
	}

	m.editName.Placeholder = "agent-name"
	m.editName.CharLimit = 64
	m.editName.Width = 40

	m.modelFilter.Placeholder = "Filter models..."
	m.modelFilter.CharLimit = 50
	m.modelFilter.Width = 40

	// Seed the viewport content so the first render is populated before any
	// WindowSizeMsg arrives (the scroll helpers early-return at height 0).
	m.updateAgentContent()

	return m
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

// helpKeyMap returns the key map for the active screen. Only the list screen
// handles "s", so only it advertises save; elsewhere "s" is input text.
func (m tuiModel) helpKeyMap() tui.KeyMap {
	switch m.screen {
	case screenList:
		return tui.AgentsKeyMap
	case screenEdit:
		return tui.FormKeyMap
	case screenPicker:
		return tui.PickerKeyMap
	case screenConfirm:
		return tui.ConfirmKeyMap
	default:
		return tui.AgentsKeyMap
	}
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		h := msg.Height - 6
		if h < 8 {
			h = 8
		}
		m.agentsView.Width = msg.Width - 6
		m.agentsView.Height = h
		m.pickerView.Width = msg.Width - 6
		m.pickerView.Height = h

	case tea.KeyMsg:
		switch m.screen {
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

	switch m.screen {
	case screenEdit:
		m.editName, _ = m.editName.Update(msg)
	case screenPicker:
		m.modelFilter, _ = m.modelFilter.Update(msg)
		if m.modelFilter.Focused() {
			m.loadModels()
		}
	}

	return m, cmd
}

func (m *tuiModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.done = true
		return m, tea.Quit
	case "esc":
		m.done = true
		return m, tea.Quit
	case "s":
		m.saved = true
		return m, tea.Quit
	case "enter":
		if m.cursor < len(m.agents) {
			m.startEdit(m.cursor)
		} else {
			m.startNew()
		}
		return m, nil
	case " ":
		if m.cursor == 0 {
			m.modeAuto = !m.modeAuto
			if m.modeAuto {
				// Turning auto on is itself the save; auto-agents need no
				// custom list.
				m.saved = true
				return m, tea.Quit
			}
			m.loadDefaults()
		}
		return m, nil
	case "d":
		if m.cursor > 0 && m.cursor-1 < len(m.agents) {
			m.screen = screenConfirm
			m.deleteIdx = m.cursor - 1
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.scrollAgents()
		}
		return m, nil
	case "down", "j":
		total := len(m.agents) + 1
		if m.cursor < total-1 {
			m.cursor++
			m.scrollAgents()
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
		m.removeEmptyAgent()
		m.scrollAgents()
		return m, nil
	case "enter":
		switch m.editCursor {
		case 0:
			m.editCursor = 1
			return m, nil
		case 1:
			m.editCursor = 2
			return m, nil
		default:
			m.screen = screenPicker
			m.modelFilter.Reset()
			m.modelCursor = 0
			m.loadModels()
			m.pickerView.GotoTop()
			return m, nil
		}
	case "tab":
		m.editCursor = (m.editCursor + 1) % 3
		return m, nil
	case "shift+tab":
		m.editCursor = (m.editCursor - 1 + 3) % 3
		return m, nil
	case "up", "k":
		if m.editCursor == 1 {
			if m.strategyIdx > 0 {
				m.strategyIdx--
				m.agents[m.cursor].Strategy = Strategies[m.strategyIdx]
			}
		}
		return m, nil
	case "down", "j":
		if m.editCursor == 1 {
			if m.strategyIdx < len(Strategies)-1 {
				m.strategyIdx++
				m.agents[m.cursor].Strategy = Strategies[m.strategyIdx]
			}
		}
		return m, nil
	}

	m.editName, _ = m.editName.Update(msg)
	if m.editName.Value() != "" && m.cursor < len(m.agents) {
		m.agents[m.cursor].Name = m.editName.Value()
	}
	return m, nil
}

func (m *tuiModel) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.screen = screenEdit
		return m, nil
	case "enter":
		m.screen = screenEdit
		return m, nil
	case " ":
		if m.modelCursor < len(m.models) {
			m.models[m.modelCursor].Selected = !m.models[m.modelCursor].Selected
			m.syncAgentModels()
			m.updatePickerContent()
		}
		return m, nil
	case "up", "k":
		if m.modelCursor > 0 {
			m.modelCursor--
			m.scrollPicker()
		}
		return m, nil
	case "down", "j":
		if m.modelCursor < len(m.models)-1 {
			m.modelCursor++
			m.scrollPicker()
		}
		return m, nil
	case "/":
		m.modelFilter.Focus()
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		if m.deleteIdx >= 0 && m.deleteIdx < len(m.agents) {
			m.agents = append(m.agents[:m.deleteIdx], m.agents[m.deleteIdx+1:]...)
		}
		m.screen = screenList
		m.deleteIdx = -1
		m.cursor = 0
		m.scrollAgents()
		return m, nil
	case "n", "esc":
		m.screen = screenList
		m.deleteIdx = -1
		m.scrollAgents()
		return m, nil
	}
	return m, nil
}

// loadDefaults seeds a single neutral agent with the first catalog model when
// the user turns auto-agents off. It deliberately does not fabricate role names
// or a model distribution: nenya owns the auto-agent defaults, and inventing
// roles from the provider list would be arbitrary.
func (m *tuiModel) loadDefaults() {
	m.agents = nil
	if len(m.catalog.Models) == 0 {
		m.cursor = 0
		m.scrollAgents()
		return
	}
	first := m.catalog.Models[0]
	m.agents = []Agent{{
		Name:     "default",
		Strategy: defaultStrategy,
		Models:   []string{first.Provider + "/" + first.Model},
	}}
	m.cursor = 0
	m.scrollAgents()
}

func (m *tuiModel) startEdit(idx int) {
	m.screen = screenEdit
	m.cursor = idx
	m.editName.SetValue(m.agents[idx].Name)
	m.editName.Focus()

	for i, s := range Strategies {
		if m.agents[idx].Strategy == s {
			m.strategyIdx = i
			break
		}
	}
	m.editCursor = 0
}

func (m *tuiModel) startNew() {
	m.screen = screenEdit
	m.cursor = len(m.agents)
	m.agents = append(m.agents, Agent{Name: "new-agent", Strategy: defaultStrategy, Models: nil})
	m.editName.SetValue("new-agent")
	m.editName.Focus()
	m.strategyIdx = 0
	m.editCursor = 0
}

func (m *tuiModel) removeEmptyAgent() {
	if m.cursor < len(m.agents) && m.agents[m.cursor].Name == "new-agent" {
		m.agents = append(m.agents[:m.cursor], m.agents[m.cursor+1:]...)
	}
	if m.cursor >= len(m.agents) && m.cursor > 0 {
		m.cursor--
	}
}

func (m *tuiModel) loadModels() {
	filter := strings.ToLower(m.modelFilter.Value())
	m.models = nil
	for _, def := range m.catalog.Models {
		name := def.Model
		if filter != "" && !strings.Contains(strings.ToLower(name), filter) &&
			!strings.Contains(strings.ToLower(def.Provider), filter) {
			continue
		}
		// Selection is keyed by the (provider, model) pair so two providers
		// offering the same model name do not both read as selected.
		qualified := def.Provider + "/" + name
		sel := false
		if m.cursor < len(m.agents) {
			for _, mdl := range m.agents[m.cursor].Models {
				if mdl == qualified || mdl == name {
					sel = true
					break
				}
			}
		}
		m.models = append(m.models, modelState{ID: name, Provider: def.Provider, Qualified: qualified, Selected: sel})
	}
	m.modelCursor = 0
	m.pickerView.GotoTop()
	m.updatePickerContent()
}

func (m *tuiModel) syncAgentModels() {
	if m.cursor >= len(m.agents) {
		return
	}
	var selected []string
	for _, mod := range m.models {
		if mod.Selected {
			// Store the provider-qualified id so two providers offering the
			// same model name cannot collapse into one entry.
			selected = append(selected, mod.Qualified)
		}
	}
	m.agents[m.cursor].Models = selected
}

func (m *tuiModel) scrollAgents() {
	total := len(m.agents) + 1
	visible := m.agentsView.Height
	if visible <= 0 {
		return
	}
	half := visible / 2
	if m.cursor < half {
		m.agentsView.YOffset = 0
	} else if m.cursor > total-1-half {
		m.agentsView.YOffset = total - visible
		if m.agentsView.YOffset < 0 {
			m.agentsView.YOffset = 0
		}
	} else {
		m.agentsView.YOffset = m.cursor - half
	}
	m.updateAgentContent()
}

func (m *tuiModel) scrollPicker() {
	visible := m.pickerView.Height
	if visible <= 0 {
		return
	}
	half := visible / 2
	if m.modelCursor < half {
		m.pickerView.YOffset = 0
	} else if m.modelCursor > len(m.models)-1-half {
		m.pickerView.YOffset = len(m.models) - visible
		if m.pickerView.YOffset < 0 {
			m.pickerView.YOffset = 0
		}
	} else {
		m.pickerView.YOffset = m.modelCursor - half
	}
	m.updatePickerContent()
}

func (m *tuiModel) updateAgentContent() {
	content := m.renderAgentList()
	m.agentsView.SetContent(content)
}

func (m *tuiModel) updatePickerContent() {
	content := m.renderPickerList()
	m.pickerView.SetContent(content)
}

func (m tuiModel) View() string {
	theme := tui.Current()

	if m.done {
		return ""
	}

	var content string
	switch m.screen {
	case screenList:
		content = m.viewList()
	case screenEdit:
		content = m.viewEdit()
	case screenPicker:
		content = m.viewPicker()
	case screenConfirm:
		content = m.viewConfirm()
	}

	helpView := m.helpModel.View(m.helpKeyMap())

	out := lipgloss.JoinVertical(lipgloss.Top,
		content,
		"",
		helpView,
	)

	return theme.App.Render(out)
}

// viewList renders the list screen.
// Content is refreshed by scrollAgents() which is called on cursor
// movement, toggle, deletion, and when returning from the edit screen.
func (m tuiModel) viewList() string {
	theme := tui.Current()

	autoLabel := "OFF"
	autoStyle := theme.Error
	if m.modeAuto {
		autoLabel = "ON"
		autoStyle = theme.Success
	}

	title := theme.Title.Render("Configure Agents")
	autoLine := fmt.Sprintf("Auto-agents: %s  (%s)",
		autoStyle.Bold(true).Render(autoLabel),
		theme.Dimmed.Render("Space to toggle, Enter to configure manually"),
	)

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

func (m tuiModel) renderAgentList() string {
	theme := tui.Current()
	var lines []string

	for i, a := range m.agents {
		var prefix string
		style := theme.Body
		idx := i + 1
		if idx == m.cursor {
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
		if idx == m.cursor {
			nameStyle = nameStyle.Background(theme.Highlight.GetBackground())
		}

		name := nameStyle.Bold(true).Render(a.Name)
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
	if m.cursor == len(m.agents) {
		addPrefix = "● "
	}
	addLabel := theme.Accent.Render(addPrefix + "+ Add new agent")
	lines = append(lines, addLabel)

	return strings.Join(lines, "\n")
}

func (m tuiModel) viewEdit() string {
	theme := tui.Current()

	title := theme.Title.Render(fmt.Sprintf("Edit Agent (%d/%d)", m.cursor+1, len(m.agents)))

	nameLabel := theme.Dimmed.Render("● Name")
	if m.editCursor == 0 {
		nameLabel = theme.Cursor.Render("● Name")
	}

	nameStyle := theme.InputBlurred
	if m.editCursor == 0 {
		nameStyle = theme.InputFocused
	}
	nameView := nameStyle.Render(m.editName.View())

	stratLabel := theme.Dimmed.Render("  Strategy")
	if m.editCursor == 1 {
		stratLabel = theme.Cursor.Render("● Strategy")
	}

	var stratLines []string
	for i, s := range Strategies {
		sel := "○ "
		if i == m.strategyIdx {
			sel = "● "
		}
		style := theme.Dimmed
		if i == m.strategyIdx {
			style = theme.Accent.Bold(true)
		}
		cursor := "  "
		if m.editCursor == 1 && i == m.strategyIdx {
			cursor = theme.Cursor.Render("→")
		}
		stratLines = append(stratLines, fmt.Sprintf("  %s %s%s", cursor, sel, style.Render(s)))
	}

	modelLabel := theme.Dimmed.Render("  Models")
	if m.editCursor == 2 {
		modelLabel = theme.Cursor.Render("● Models")
	}

	modelsStr := "(none)"
	if m.cursor < len(m.agents) && len(m.agents[m.cursor].Models) > 0 {
		modelsStr = strings.Join(m.agents[m.cursor].Models, ", ")
	}
	modelLine := theme.Accent.Bold(true).Render(fmt.Sprintf("[%d selected]", len(m.agents[m.cursor].Models)))

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		nameLabel,
		"",
		nameView,
		"",
		stratLabel,
		strings.Join(stratLines, "\n"),
		"",
		modelLabel,
		"",
		fmt.Sprintf("  %s %s", modelLine, theme.Dimmed.Render(trunc(modelsStr, 50))),
	)

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

func (m tuiModel) viewPicker() string {
	theme := tui.Current()

	name := ""
	if m.cursor < len(m.agents) {
		name = m.agents[m.cursor].Name
	}

	title := theme.Title.Render(fmt.Sprintf("Select Models: %s", name))
	subtitle := theme.Dimmed.Render("Space to toggle · / to filter · Enter to finish · Esc to go back")
	selectedStr := ""
	if m.cursor < len(m.agents) {
		selectedStr = theme.Dimmed.Render(fmt.Sprintf("selected: %d", len(m.agents[m.cursor].Models)))
	}

	filterView := m.modelFilter.View()
	if m.modelFilter.Focused() {
		filterView = theme.InputFocused.Render(filterView)
	} else {
		filterView = theme.InputBlurred.Render(filterView)
	}

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		subtitle,
		"",
		selectedStr,
		"",
		filterView,
		"",
		m.pickerView.View(),
	)

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

// viewPicker renders the model picker screen.
// Viewport content is refreshed by scrollPicker() on cursor movement and
// by updatePicker() on filter changes.

func (m tuiModel) renderPickerList() string {
	theme := tui.Current()
	var lines []string

	for i, mod := range m.models {
		prefix := "  "
		if i == m.modelCursor {
			prefix = "● "
		}
		sel := " "
		if mod.Selected {
			sel = "✓"
		}
		selStyle := theme.Unchecked
		if mod.Selected {
			selStyle = theme.Checked
		}

		modelStyle := theme.Body
		if i == m.modelCursor {
			modelStyle = theme.Highlight
		}

		name := theme.Accent.Render(mod.ID)
		provider := theme.Dimmed.Render(mod.Provider)

		line := fmt.Sprintf("%s[%s] %s  %s",
			selStyle.Render(prefix),
			selStyle.Render(sel),
			modelStyle.Render(name),
			provider,
		)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m tuiModel) viewConfirm() string {
	theme := tui.Current()

	name := ""
	if m.deleteIdx >= 0 && m.deleteIdx < len(m.agents) {
		name = m.agents[m.deleteIdx].Name
	}

	title := theme.Title.Render("Delete Agent")
	prompt := fmt.Sprintf("Delete '%s'?", theme.Error.Bold(true).Render(name))
	confirm := theme.Dimmed.Render("y or Enter to confirm · n or Esc to cancel")

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		prompt,
		"",
		confirm,
	)

	modalStyle := lipgloss.NewStyle().
		Border(theme.Border).
		BorderForeground(theme.Error.GetForeground()).
		Padding(1, 2).
		Width(40)

	modal := modalStyle.Render(body)

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		Padding(4, 2)

	return baseStyle.Render(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal))
}

func trunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// RunAgentEditor runs the agent configuration TUI against the authoritative
// model/provider catalog (from `nenya describe --json`). It returns whether the
// user chose auto-agents and, otherwise, the config changes to apply. The
// returned map carries only the "agents" key; discovery.auto_agents is applied
// by the caller through nenya's single writer, so it is not duplicated here.
func RunAgentEditor(catalog Catalog) (bool, map[string]any, error) {
	m := newTUIModel(catalog)
	p := tea.NewProgram(&m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return false, nil, err
	}

	tm, ok := result.(*tuiModel)
	if !ok {
		return false, nil, nil
	}
	return tm.editorResult()
}

// editorResult assembles the editor's output from its final state. It is
// separate from the TUI run loop so the assembly (not just the interactive
// path) is unit-testable.
func (m *tuiModel) editorResult() (bool, map[string]any, error) {
	// Not saved (q/esc) is a no-op.
	if !m.saved {
		return false, nil, nil
	}

	// Auto-agents is the "use defaults" choice; nothing to write.
	if m.modeAuto {
		return true, nil, nil
	}

	if len(m.agents) == 0 {
		return false, nil, fmt.Errorf("no agents to save; add an agent or leave auto-agents on")
	}

	agentsMap := make(map[string]map[string]any)
	for _, a := range m.agents {
		if a.Name == "" {
			return false, nil, fmt.Errorf("agent name cannot be empty")
		}
		if _, dup := agentsMap[a.Name]; dup {
			return false, nil, fmt.Errorf("duplicate agent name %q", a.Name)
		}
		agentsMap[a.Name] = map[string]any{
			"strategy": a.Strategy,
			"models":   a.Models,
		}
	}

	return false, map[string]any{"agents": agentsMap}, nil
}
