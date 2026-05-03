package containers

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
)

type keyField struct {
	name  string
	input textinput.Model
}

type keyResult struct {
	Name  string
	Value string
}

type tuiModel struct {
	state         screen
	providers     []ProviderDef
	selected      map[int]bool
	cursor        int
	fields        []keyField
	fieldCursor   int
	done          bool
	quitting      bool
	customName    string
	customResults []keyResult
	showCustom    int
	customInputs  []textinput.Model
	customCursor  int
}

func newTUIModel() tuiModel {
	return tuiModel{
		state:     screenSelect,
		providers: BuiltinProviders,
		selected:  make(map[int]bool),
	}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.state == screenSelect {
			return m.updateSelect(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m tuiModel) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalItems := len(m.providers) + 1 // +1 for custom provider entry

	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		m.done = true
		return m, tea.Quit

	case "enter":
		if m.cursor == len(m.providers) {
			m.showCustom = 0
			ci := textinput.New()
			ci.Placeholder = "my-custom-provider"
			ci.CharLimit = 128
			ci.Width = 40
			m.customInputs = []textinput.Model{ci}

			ki := textinput.New()
			ki.Placeholder = "sk-..."
			ki.CharLimit = 256
			ki.Width = 50
			m.customInputs = append(m.customInputs, ki)
			m.customCursor = 0
			return m, ci.Focus()
		}

		selectedCount := 0
		for i := range m.providers {
			if m.selected[i] {
				selectedCount++
			}
		}
		if selectedCount == 0 {
			return m, nil
		}

		m.state = screenKeys
		m.fields = nil
		m.fieldCursor = 0
		for i, p := range m.providers {
			if m.selected[i] && p.NeedsKey {
				ti := textinput.New()
				ti.Placeholder = p.Auth
				ti.CharLimit = 256
				ti.Width = 60
				m.fields = append(m.fields, keyField{name: p.Name, input: ti})
			}
		}
		if len(m.fields) > 0 {
			return m, m.fields[0].input.Focus()
		}
		m.done = true
		return m, tea.Quit

	case " ":
		if m.cursor < len(m.providers) {
			m.selected[m.cursor] = !m.selected[m.cursor]
		}
		return m, nil

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < totalItems-1 {
			m.cursor++
		}
		return m, nil
	}

	if m.showCustom >= 0 && m.customCursor >= 0 {
		return m.updateCustom(msg)
	}

	return m, nil
}

func (m *tuiModel) updateCustom(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.customCursor == 1 {
			name := m.customInputs[0].Value()
			key := m.customInputs[1].Value()
			if name != "" && key != "" {
				m.customResults = append(m.customResults, keyResult{Name: name, Value: key})
			}
			m.showCustom++
			ci := textinput.New()
			ci.Placeholder = "my-custom-provider"
			ci.CharLimit = 128
			ci.Width = 40
			m.customInputs = []textinput.Model{ci}
			ki := textinput.New()
			ki.Placeholder = "sk-..."
			ki.CharLimit = 256
			ki.Width = 50
			m.customInputs = append(m.customInputs, ki)
			m.customCursor = 0
			return m, ci.Focus()
		}
		m.showCustom = -1
		m.customCursor = -1
		return m, nil

	case "esc":
		m.showCustom = -1
		m.customCursor = -1
		return m, nil

	case "tab":
		m.customCursor = (m.customCursor + 1) % len(m.customInputs)
		return m, m.customInputs[m.customCursor].Focus()

	case "shift+tab":
		m.customCursor = (m.customCursor - 1 + len(m.customInputs)) % len(m.customInputs)
		return m, m.customInputs[m.customCursor].Focus()
	}

	var cmd tea.Cmd
	m.customInputs[m.customCursor], cmd = m.customInputs[m.customCursor].Update(msg)
	return m, cmd
}

func (m tuiModel) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.done = true
		return m, tea.Quit

	case "enter":
		m.done = true
		return m, tea.Quit

	case "tab", "down":
		if m.fieldCursor < len(m.fields)-1 {
			m.fields[m.fieldCursor].input.Blur()
			m.fieldCursor++
			return m, m.fields[m.fieldCursor].input.Focus()
		}
		return m, nil

	case "shift+tab", "up":
		if m.fieldCursor > 0 {
			m.fields[m.fieldCursor].input.Blur()
			m.fieldCursor--
			return m, m.fields[m.fieldCursor].input.Focus()
		}
		return m, nil
	}

	if m.fieldCursor < len(m.fields) {
		m.fields[m.fieldCursor].input, _ = m.fields[m.fieldCursor].input.Update(msg)
	}
	return m, nil
}

func (m tuiModel) View() string {
	if m.quitting || (m.done && m.state == screenSelect && len(m.fields) == 0) {
		return ""
	}

	if m.state == screenKeys {
		return m.renderKeys()
	}
	return m.renderSelect()
}

func (m tuiModel) renderSelect() string {
	s := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("Select providers to configure") + "\n"
	s += lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  Space to toggle · ↑/↓ to navigate · Enter to continue · Esc to skip\n") + "\n"

	for i, p := range m.providers {
		check := " "
		if m.selected[i] {
			check = "x"
		}
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		style := lipgloss.NewStyle()
		if !m.selected[i] {
			style = style.Foreground(lipgloss.Color("7"))
		} else {
			style = style.Foreground(lipgloss.Color("10"))
		}

		auth := ""
		if !p.NeedsKey {
			auth = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(" (no key needed)")
		}

		s += fmt.Sprintf("%s[%s] %s%s%s\n",
			cursor,
			style.Render(check),
			style.Render(p.Name),
			lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(" ("+p.Help+")"),
			auth,
		)
	}

	cursor := "  "
	if m.cursor == len(m.providers) {
		cursor = "> "
	}
	s += fmt.Sprintf("%s%s\n", cursor,
		lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render("+ Add custom provider"))

	return s
}

func (m tuiModel) renderKeys() string {
	s := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("Enter API keys") + "\n"
	s += lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  Tab to navigate · Enter to confirm\n") + "\n"

	for _, p := range m.customResults {
		s += fmt.Sprintf("  %s: [set]\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(p.Name))
	}

	for i, f := range m.fields {
		prefix := "  "
		if i == m.fieldCursor {
			prefix = "> "
		}
		s += prefix + lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(f.name) + "\n"
		s += "    " + f.input.View() + "\n"
	}

	return s
}

func (m tuiModel) Results() map[string]string {
	result := make(map[string]string)
	for _, f := range m.fields {
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
	p := tea.NewProgram(newTUIModel())
	m, err := p.Run()
	if err != nil {
		return nil, err
	}
	if model, ok := m.(tuiModel); ok {
		return model.Results(), nil
	}
	return nil, nil
}

func init() {
	sort.Slice(BuiltinProviders, func(i, j int) bool {
		if BuiltinProviders[i].NeedsKey != BuiltinProviders[j].NeedsKey {
			return BuiltinProviders[i].NeedsKey
		}
		return BuiltinProviders[i].Name < BuiltinProviders[j].Name
	})
}