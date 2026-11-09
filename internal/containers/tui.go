package containers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
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

// BuiltinProviders is a documented scaffold-time shim: `containers setup`
// creates a deployment before a nenya binary exists on the machine, so it
// cannot call `nenya describe --json` (the authoritative provider catalog,
// CONTRACT.md §4.3). Callers that do have a binary pass the contract catalog to
// CollectProviderKeysFrom instead; this list is pruned when the container flow
// can assume an installed binary.
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
	// validationMsg is the submit-gate error shown on the keys and custom
	// screens when required fields are still empty or invalid (NCTL-51).
	validationMsg string
	// submitted is true when the user finished deliberately via enter (the
	// gated keys submit, or a select-screen finish); ctrl+c/esc/q aborts
	// never set it (NCTL-51 review).
	submitted     bool
	quitting      bool
	width, height int
	helpModel     help.Model
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
	return newTUIModelWithProviders(BuiltinProviders)
}

// newTUIModelWithProviders builds the provider picker from the given providers.
// Pass the contract catalog when a nenya binary is available; BuiltinProviders
// is the scaffold-time shim.
func newTUIModelWithProviders(providers []ProviderDef) tuiModel {
	// dedupeProviders allocates a fresh slice, so the caller's input (and the
	// package-level BuiltinProviders) is never reordered by the sort below.
	providers = dedupeProviders(providers)
	sort.Slice(providers, func(i, j int) bool {
		if providers[i].NeedsKey != providers[j].NeedsKey {
			return providers[i].NeedsKey
		}
		return providers[i].Name < providers[j].Name
	})

	cols := []table.Column{
		{Title: " ", Width: 3},
		{Title: "Provider", Width: 18},
		{Title: "Description", Width: 25},
		{Title: "Key Required", Width: 14},
	}

	rows := []table.Row{}
	for _, p := range providers {
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

	customKey := newKeyInput("sk-...")
	customKey.Width = 50

	return tuiModel{
		screen:      screenSelect,
		providers:   providers,
		selected:    make(map[int]bool),
		table:       t,
		customName:  customName,
		customKey:   customKey,
		customFocus: 0,
		helpModel:   tui.NewHelpModel(),
	}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

// helpKeyMap returns the key map for the active screen so the help bar only
// advertises handled keys.
func (m tuiModel) helpKeyMap() tui.KeyMap {
	switch m.screen {
	case screenSelect:
		return tui.KeyMap{
			Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
			Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
			Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "continue")),
			Toggle: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
			Quit:   key.NewBinding(key.WithKeys("ctrl+c", "q", "esc"), key.WithHelp("ctrl+c/q/esc", "quit")),
		}
	case screenKeys:
		return tui.KeyMap{
			Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
			Toggle: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
			Prev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
			Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			Quit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		}
	default: // screenCustom
		return tui.KeyMap{
			Toggle: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
			Prev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
			Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
			Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			Quit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		}
	}
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.screen {
		case screenSelect:
			// Command keys (enter/space/quit) are handled here; navigation is
			// forwarded to the table below.
			if model, selCmd, handled := m.selectKey(msg); handled {
				return model, selCmd
			}
		case screenKeys:
			// Command keys (ctrl+c/esc/enter/tab/shift+tab) are handled here;
			// everything else (typing, backspace, arrows) falls through to
			// the focused input below. Routing unhandled keys onward is what
			// makes the fields typeable at all.
			if model, keysCmd, handled := m.updateKeys(msg); handled {
				return model, keysCmd
			}
		case screenCustom:
			if model, customCmd, handled := m.updateCustom(msg); handled {
				return model, customCmd
			}
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

	switch m.screen {
	case screenSelect:
		m.table, cmd = m.table.Update(msg)
	case screenKeys:
		if m.keyCursor < len(m.keys) {
			m.keys[m.keyCursor].input, cmd = m.keys[m.keyCursor].input.Update(msg)
		}
	case screenCustom:
		if m.customFocus == 0 {
			m.customName, cmd = m.customName.Update(msg)
		} else {
			m.customKey, cmd = m.customKey.Update(msg)
		}
	}

	return m, cmd
}

// selectKey handles the command keys on the provider-select screen. It returns
// handled=false for navigation keys (up/down/page), which the caller forwards to
// the table so the cursor can move.
func (m *tuiModel) selectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		m.quitting = true
		return m, tea.Quit, true

	case "enter":
		if m.table.Cursor() == len(m.providers) {
			m.screen = screenCustom
			m.customFocus = 0
			m.validationMsg = ""
			// Defensive: any input left focused by an earlier visit must not
			// render a second cursor next to the name field.
			m.customName.Blur()
			m.customKey.Blur()
			return m, m.customName.Focus(), true
		}

		selectedCount := 0
		for i := range m.providers {
			if m.selected[i] {
				selectedCount++
			}
		}

		if selectedCount == 0 {
			// Enter-driven finish (nothing to configure); custom entries
			// saved on the custom screen must still be returned.
			m.quitting = true
			m.submitted = true
			return m, tea.Quit, true
		}

		m.screen = screenKeys
		m.keys = syncKeyFields(m.providers, m.selected, m.keys)
		// Preserve the working position across back-navigation; only clamp
		// against a shrunken list (NCTL-51 review).
		if m.keyCursor >= len(m.keys) {
			m.keyCursor = len(m.keys) - 1
		}
		if m.keyCursor < 0 {
			m.keyCursor = 0
		}
		m.validationMsg = ""

		if len(m.keys) > 0 {
			return m, m.keys[m.keyCursor].input.Focus(), true
		}
		// Enter-driven finish: only no-key providers are selected, but custom
		// entries saved on the custom screen must still be returned.
		m.quitting = true
		m.submitted = true
		return m, tea.Quit, true

	case " ":
		idx := m.table.Cursor()
		if idx < len(m.providers) {
			m.selected[idx] = !m.selected[idx]
			m.updateTableRow(idx)
		}
		return m, nil, true
	}

	return m, nil, false
}

func (m *tuiModel) updateCustom(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit, true
	case "esc":
		m.validationMsg = ""
		// Blur the active input so re-entry doesn't render two cursors
		// (selectKey only focuses the name field).
		if m.customFocus == 0 {
			m.customName.Blur()
		} else {
			m.customKey.Blur()
		}
		m.screen = screenSelect
		return m, nil, true

	case "enter":
		if m.customFocus == 0 {
			m.customFocus = 1
			m.customName.Blur()
			m.validationMsg = ""
			return m, m.customKey.Focus(), true
		}

		name := strings.TrimSpace(m.customName.Value())
		key := strings.TrimSpace(m.customKey.Value())
		if name != "" && key != "" {
			// A custom provider shadowing a builtin (or an earlier custom
			// entry) would silently overwrite its key in Results — reject the
			// duplicate instead (NCTL-51 review).
			if m.customNameTaken(name) {
				m.validationMsg = fmt.Sprintf("provider name %q is already in use", name)
				return m, nil, true
			}
			m.validationMsg = ""
			m.customResults = append(m.customResults, customResult{Name: name, Value: key})

			m.customName.Reset()
			m.customKey.Reset()
			m.customKey.Blur()
			m.customFocus = 0
			return m, m.customName.Focus(), true
		}
		m.validationMsg = "provider name and API key are both required"
		return m, nil, true

	case "tab", "shift+tab":
		m.validationMsg = ""
		m.customFocus = (m.customFocus + 1) % 2
		if m.customFocus == 0 {
			m.customKey.Blur()
			return m, m.customName.Focus(), true
		}
		m.customName.Blur()
		return m, m.customKey.Focus(), true
	}

	// Unhandled keys fall through to the focused input.
	return m, nil, false
}

// customNameTaken reports whether name collides with a builtin provider or a
// custom entry already saved in this session (NCTL-51 review).
func (m *tuiModel) customNameTaken(name string) bool {
	for _, p := range m.providers {
		if p.Name == name {
			return true
		}
	}
	for _, c := range m.customResults {
		if c.Name == name {
			return true
		}
	}
	return false
}

func (m *tuiModel) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit, true

	case "esc":
		// NCTL-51: back navigation; entered values survive the round trip
		// (the keys list is synced, not rebuilt, on re-entry).
		m.screen = screenSelect
		m.validationMsg = ""
		return m, nil, true

	case "enter":
		// NCTL-51: submit is gated until every required key field is filled;
		// the user sees which fields are missing instead of a silent quit.
		if missing := missingKeyFields(m.keys); len(missing) > 0 {
			m.validationMsg = missingKeysMessage(missing)
			return m, nil, true
		}
		m.quitting = true
		m.submitted = true
		return m, tea.Quit, true

	case "tab", "shift+tab":
		if len(m.keys) == 0 {
			return m, nil, true
		}
		next := m.keyCursor + 1
		if msg.String() == "shift+tab" {
			next = m.keyCursor - 1
		}
		next = (next + len(m.keys)) % len(m.keys)
		if next == m.keyCursor {
			// Single field: no movement, keep the validation message.
			return m, nil, true
		}
		// Clear the gate message only on an actual move; a no-op key press
		// must not erase the "which fields are missing" hint (NCTL-51 review).
		m.validationMsg = ""
		m.keys[m.keyCursor].input.Blur()
		m.keyCursor = next
		return m, m.keys[m.keyCursor].input.Focus(), true
	}

	// Unhandled keys (typing, backspace, arrows) fall through so the focused
	// input receives them.
	return m, nil, false
}

// missingKeysMessage renders the submit-gate error. Long lists are capped so
// the line survives narrow terminals (NCTL-51 review).
func missingKeysMessage(missing []string) string {
	const maxNamed = 5
	if len(missing) > maxNamed {
		shown := missing[:maxNamed]
		return fmt.Sprintf(
			"required key(s) still empty: %s …and %d more — fill them, or go back (esc) and unselect the provider",
			strings.Join(shown, ", "), len(missing)-maxNamed,
		)
	}
	return fmt.Sprintf(
		"required key(s) still empty: %s — fill them, or go back (esc) and unselect the provider",
		strings.Join(missing, ", "),
	)
}

// newKeyInput builds a masked API-key input. Keys are credentials: they are
// echo-masked so they do not land in terminal scrollback (NCTL-51 review);
// the placeholder still shows the expected format.
func newKeyInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.CharLimit = 512
	ti.Width = 60
	return ti
}

// syncKeyFields rebuilds the key-entry list from the current selection while
// preserving inputs already typed for providers still selected (NCTL-51:
// back-and-forth navigation must not lose entered values). Provider names are
// unique by construction (dedupeProviders), so name-keyed preservation is
// unambiguous.
func syncKeyFields(providers []ProviderDef, selected map[int]bool, existing []keyField) []keyField {
	previousValues := make(map[string]string, len(existing))
	for _, f := range existing {
		previousValues[f.name] = f.input.Value()
	}

	keys := make([]keyField, 0, len(providers))
	for i, p := range providers {
		if !selected[i] || !p.NeedsKey {
			continue
		}
		ti := newKeyInput(p.Auth)
		ti.SetValue(previousValues[p.Name])
		// Defensive: bubbles v1.0.0 already lands the cursor at the end of
		// the restored value on SetValue; pin it explicitly in case that
		// changes.
		ti.CursorEnd()
		keys = append(keys, keyField{name: p.Name, input: ti})
	}
	return keys
}

// dedupeProviders drops later entries with a name already seen, keeping the
// first. Duplicate provider names would silently misroute typed key values in
// syncKeyFields and collapse in Results (NCTL-51 review).
func dedupeProviders(providers []ProviderDef) []ProviderDef {
	seen := make(map[string]bool, len(providers))
	out := make([]ProviderDef, 0, len(providers))
	for _, p := range providers {
		if seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		out = append(out, p)
	}
	return out
}

// isEmptyKeyField reports whether a required key field is still unfilled;
// whitespace-only input counts as empty (NCTL-51 review).
func isEmptyKeyField(f keyField) bool {
	return strings.TrimSpace(f.input.Value()) == ""
}

// missingKeyFields returns the names of required key fields still empty
// (NCTL-51).
func missingKeyFields(keys []keyField) []string {
	var missing []string
	for _, f := range keys {
		if isEmptyKeyField(f) {
			missing = append(missing, f.name)
		}
	}
	return missing
}

func (m *tuiModel) updateTableRow(idx int) {
	// Only the toggled row's content changes; avoid rewriting every row.
	rows := m.table.Rows()
	if idx < 0 || idx >= len(m.providers) || idx >= len(rows) {
		return
	}
	check := "  "
	if m.selected[idx] {
		check = "✓ "
	}
	rows[idx] = table.Row{check, m.providers[idx].Name, m.providers[idx].Help, keyReqText(m.providers[idx])}
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

	helpView := m.helpModel.View(m.helpKeyMap())

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

	rows := []string{
		title,
		"",
		nameLabel,
		"",
		nameView,
		"",
		keyLabel,
		"",
		keyView,
	}
	// NCTL-51 review: surface validation errors (duplicate/missing fields)
	// on the custom screen too.
	if m.validationMsg != "" {
		rows = append(rows, "", theme.Error.Render(m.validationMsg))
	}

	body := lipgloss.JoinVertical(lipgloss.Top, rows...)

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
		// NCTL-51: required fields are marked until filled.
		if isEmptyKeyField(f) {
			label += theme.Error.Render("  (required)")
		}

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

	if m.validationMsg != "" {
		rows = append(rows, "")
		rows = append(rows, theme.Error.Render(m.validationMsg))
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
	// Only keys for still-selected providers leave the wizard: typing a key,
	// going back, and unselecting the provider must not persist the orphan
	// value (NCTL-51 review).
	selectedNames := make(map[string]bool, len(m.providers))
	for i, p := range m.providers {
		if m.selected[i] {
			selectedNames[p.Name] = true
		}
	}
	result := make(map[string]string)
	for _, f := range m.keys {
		value := f.input.Value()
		if selectedNames[f.name] && !isEmptyKeyField(f) {
			result[f.name] = strings.TrimSpace(value)
		}
	}
	for _, c := range m.customResults {
		result[c.Name] = c.Value
	}
	return result
}

// CollectProviderKeysFrom runs the provider-key picker with the given provider
// catalog. Callers with an installed nenya pass the contract catalog so the
// picker matches what the gateway actually supports. It returns (nil, nil)
// when the user aborts (ctrl+c/esc/q) — only a deliberate submit yields the
// collected keys.
func CollectProviderKeysFrom(providers []ProviderDef) (map[string]string, error) {
	m := newTUIModelWithProviders(providers)
	p := tea.NewProgram(&m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, err
	}
	if model, ok := result.(*tuiModel); ok && model.submitted {
		return model.Results(), nil
	}
	// Aborted: nothing to persist.
	return nil, nil
}
