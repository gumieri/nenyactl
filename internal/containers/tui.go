package containers

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gumieri/nenyactl/internal/tui"
)

type ProviderDef struct {
	Name     string
	Help     string
	Auth     string
	NeedsKey bool
}

var BuiltinProviders = []ProviderDef{
	{Name: "gemini", Help: "Google AI", Auth: "AIza...", NeedsKey: true},
	{Name: "deepseek", Help: "DeepSeek", Auth: "sk-...", NeedsKey: true},
	{Name: "zai", Help: "Z.AI (GLM)", Auth: "sk-...", NeedsKey: true},
	{Name: "zai-coding-plan", Help: "Z.AI Coding Plan", Auth: "sk-...", NeedsKey: true},
	{Name: "groq", Help: "Groq", Auth: "gsk_...", NeedsKey: true},
	{Name: "together", Help: "Together AI", Auth: "sk-...", NeedsKey: true},
	{Name: "anthropic", Help: "Anthropic", Auth: "sk-ant-...", NeedsKey: true},
	{Name: "mistral", Help: "Mistral AI", Auth: "sk-...", NeedsKey: true},
	{Name: "xai", Help: "xAI (Grok)", Auth: "sk-...", NeedsKey: true},
	{Name: "perplexity", Help: "Perplexity", Auth: "pplx-...", NeedsKey: true},
	{Name: "cohere", Help: "Cohere", Auth: "sk-...", NeedsKey: true},
	{Name: "deepinfra", Help: "DeepInfra", Auth: "sk-...", NeedsKey: true},
	{Name: "openrouter", Help: "OpenRouter", Auth: "sk-or-...", NeedsKey: true},
	{Name: "nvidia_free", Help: "NVIDIA (free tier)", Auth: "nvapi-...", NeedsKey: true},
	{Name: "qwen_free", Help: "Qwen (free tier)", Auth: "sk-...", NeedsKey: true},
	{Name: "minimax_free", Help: "MiniMax (free tier)", Auth: "sk-...", NeedsKey: true},
	{Name: "sambanova", Help: "SambaNova", Auth: "sk-...", NeedsKey: true},
	{Name: "cerebras", Help: "Cerebras", Auth: "sk-...", NeedsKey: true},
	{Name: "github", Help: "GitHub Models", Auth: "ghp_...", NeedsKey: true},
	{Name: "nvidia", Help: "NVIDIA (full)", Auth: "nvapi-...", NeedsKey: true},
	{Name: "zen", Help: "Zen (OpenCode)", Auth: "sk-...", NeedsKey: true},
	{Name: "ollama", Help: "Ollama (local)", Auth: "none", NeedsKey: false},
}

type screen int

const (
	screenSelect screen = iota
	screenKeys
	screenCustom
)

type tuiModel struct {
	screen        screen
	providers     []ProviderDef
	selected      map[int]bool
	table         table.Model
	customName    textinput.Model
	customKey     textinput.Model
	customFocus   int
	customResults []customResult
	keys          []keyField
	keyCursor     int
	quitting      bool
	width, height int
	helpModel     help.Model
	helpKM        tui.KeyMap
}

type keyField struct {
	name  string
	input textinput.Model
}

type customResult struct {
	Name  string
	Value string
}

func newTUIModel() tuiModel {
	sort.Slice(BuiltinProviders, func(i, j int) bool {
		if BuiltinProviders[i].NeedsKey != BuiltinProviders[j].NeedsKey {
			return BuiltinProviders[i].NeedsKey
		}
		return BuiltinProviders[i].Name < BuiltinProviders[j].Name
	})

	cols := []table.Column{
		{Title: " ", Width: 3},
		{Title: "Provider", Width: 18},
		{Title: "Description", Width: 25},
		{Title: "Key Required", Width: 14},
	}

	rows := []table.Row{}
	for _, p := range BuiltinProviders {
		rows = append(rows, table.Row{"  ", p.Name, p.Help, keyReqText(p)})
	}
	rows = append(rows, table.Row{"  ", "+ Add custom provider...", "", ""})

	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(15),
	)

	theme := tui.Current()
	s := table.DefaultStyles()
	s.Header = s.Header.
		Foreground(theme.Title.GetForeground()).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground())
	s.Selected = theme.SelectedRow
	s.Cell = theme.Body
	t.SetStyles(s)

	customName := textinput.New()
	customName.Placeholder = "my-custom-provider"
	customName.CharLimit = 128
	customName.Width = 40

	customKey := textinput.New()
	customKey.Placeholder = "sk-..."
	customKey.CharLimit = 256
	customKey.Width = 50

	return tuiModel{
		screen:    screenSelect,
		providers: BuiltinProviders,
		selected:  make(map[int]bool),
		table:     t,
		customName:  customName,
		customKey:   customKey,
		customFocus: 0,
		helpModel:   tui.NewHelpModel(),
		helpKM:      tui.ListKeyMap,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.screen {
		case screenSelect:
			return m.updateSelect(msg)
		case screenKeys:
			return m.updateKeys(msg)
		case screenCustom:
			return m.updateCustom(msg)
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		h := msg.Height - 6
		if h < 5 {
			h = 5
		}
		m.table.SetHeight(h)
		m.table.SetWidth(msg.Width - 4)
	}

	if m.screen == screenSelect {
		m.table, cmd = m.table.Update(msg)
	} else if m.screen == screenKeys {
		if m.keyCursor < len(m.keys) {
			m.keys[m.keyCursor].input, cmd = m.keys[m.keyCursor].input.Update(msg)
		}
	} else if m.screen == screenCustom {
		if m.customFocus == 0 {
			m.customName, cmd = m.customName.Update(msg)
		} else {
			m.customKey, cmd = m.customKey.Update(msg)
		}
	}

	return m, cmd
}

func (m *tuiModel) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return m, tea.Quit

	case "q":
		m.quitting = true
		return m, tea.Quit

	case "enter":
		if m.table.Cursor() == len(m.providers) {
			m.screen = screenCustom
			m.customFocus = 0
			return m, m.customName.Focus()
		}

		selectedCount := 0
		for i := range m.providers {
			if m.selected[i] {
				selectedCount++
			}
		}

		if selectedCount == 0 {
			m.quitting = true
			return m, tea.Quit
		}

		m.screen = screenKeys
		m.keys = nil
		m.keyCursor = 0

		for i, p := range m.providers {
			if m.selected[i] && p.NeedsKey {
				ti := textinput.New()
				ti.Placeholder = p.Auth
				ti.CharLimit = 256
				ti.Width = 60
				m.keys = append(m.keys, keyField{name: p.Name, input: ti})
			}
		}

		if len(m.keys) > 0 {
			m.keys[0].input.Focus()
		} else {
			m.quitting = true
			return m, tea.Quit
		}

	case " ":
		idx := m.table.Cursor()
		if idx < len(m.providers) {
			m.selected[idx] = !m.selected[idx]
			m.updateTableRow(idx)
		}
	}

	return m, nil
}

func (m *tuiModel) updateCustom(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenSelect
		return m, nil

	case "enter":
		if m.customFocus == 0 {
			m.customFocus = 1
			return m, m.customKey.Focus()
		}

		name := m.customName.Value()
		key := m.customKey.Value()
		if name != "" && key != "" {
			m.customResults = append(m.customResults, customResult{Name: name, Value: key})

			m.customName.Reset()
			m.customKey.Reset()
			m.customFocus = 0
			return m, m.customName.Focus()
		}

	case "tab":
		m.customFocus = (m.customFocus + 1) % 2
		if m.customFocus == 0 {
			return m, m.customName.Focus()
		}
		return m, m.customKey.Focus()
	}

	return m, nil
}

func (m *tuiModel) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return m, tea.Quit

	case "enter":
		m.quitting = true
		return m, tea.Quit

	case "tab":
		if m.keyCursor < len(m.keys)-1 {
			m.keys[m.keyCursor].input.Blur()
			m.keyCursor++
			return m, m.keys[m.keyCursor].input.Focus()
		}
	}

	return m, nil
}

func (m *tuiModel) updateTableRow(idx int) {
	rows := m.table.Rows()
	for i := range m.providers {
		check := "  "
		if m.selected[i] {
			check = "✓ "
		}
		if i < len(rows) {
			rows[i] = table.Row{check, m.providers[i].Name, m.providers[i].Help, keyReqText(m.providers[i])}
		}
	}
	m.table.SetRows(rows)
}

func keyReqText(p ProviderDef) string {
	if !p.NeedsKey {
		return "(none)"
	}
	return p.Auth
}

func (m tuiModel) View() string {
	theme := tui.Current()

	if m.quitting {
		return ""
	}

	var content string
	switch m.screen {
	case screenSelect:
		content = m.renderSelect()
	case screenKeys:
		content = m.renderKeys()
	case screenCustom:
		content = m.renderCustom()
	}

	helpView := m.helpModel.View(m.helpKM)

	h := lipgloss.JoinVertical(lipgloss.Top,
		content,
		"",
		helpView,
	)

	return theme.App.Render(h)
}

func (m tuiModel) renderSelect() string {
	theme := tui.Current()

	title := theme.Title.Render("Select Providers to Configure")
	subtitle := theme.Dimmed.Render("Choose providers to add API keys for")

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	tbl := m.table.View()
	wrapped := lipgloss.NewStyle().Width(m.width - 6).Render(tbl)

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		subtitle,
		"",
		wrapped,
	)

	return baseStyle.Render(body)
}

func (m tuiModel) renderCustom() string {
	theme := tui.Current()

	title := theme.Title.Render("Add Custom Provider")

	nameFocus := ""
	keyFocus := ""
	if m.customFocus == 0 {
		nameFocus = "●"
	} else {
		keyFocus = "●"
	}

	nameLabel := theme.Body.Render(nameFocus + " Provider name:")
	keyLabel := theme.Body.Render(keyFocus + " API key:")

	nameStyle := theme.InputBlurred
	keyStyle := theme.InputBlurred
	if m.customFocus == 0 {
		nameStyle = theme.InputFocused
	} else {
		keyStyle = theme.InputFocused
	}

	nameView := nameStyle.Render(m.customName.View())
	keyView := keyStyle.Render(m.customKey.View())

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		nameLabel,
		"",
		nameView,
		"",
		keyLabel,
		"",
		keyView,
	)

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

func (m tuiModel) renderKeys() string {
	theme := tui.Current()

	title := theme.Title.Render("Enter API Keys")
	subtitle := theme.Dimmed.Render(fmt.Sprintf("Configure %d provider(s)", len(m.keys)))

	var rows []string
	for i, f := range m.keys {
		prefix := "  "
		if i == m.keyCursor {
			prefix = "●"
		}
		label := theme.Body.Render(prefix + " " + f.name)

		inputStyle := theme.InputBlurred
		if i == m.keyCursor {
			inputStyle = theme.InputFocused
		}
		inputView := inputStyle.Render(f.input.View())

		rows = append(rows, label)
		rows = append(rows, "")
		rows = append(rows, inputView)
		if i < len(m.keys)-1 {
			rows = append(rows, "")
		}
	}

	for _, cr := range m.customResults {
		rows = append(rows, "")
		rows = append(rows, theme.Success.Render("✓ "+cr.Name))
	}

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		subtitle,
		"",
		lipgloss.JoinVertical(lipgloss.Top, rows...),
	)

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

func (m tuiModel) Results() map[string]string {
	result := make(map[string]string)
	for _, f := range m.keys {
		if f.input.Value() != "" {
			result[f.name] = f.input.Value()
		}
	}
	for _, c := range m.customResults {
		result[c.Name] = c.Value
	}
	return result
}

func CollectProviderKeys() (map[string]string, error) {
	p := tea.NewProgram(newTUIModel(), tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		return nil, err
	}
	if model, ok := m.(tuiModel); ok {
		return model.Results(), nil
	}
	return nil, nil
}
