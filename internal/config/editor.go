package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gumieri/nenyactl/internal/jsonc"
	"github.com/gumieri/nenyactl/internal/tui"
	"github.com/tailscale/hujson"
)

type configScreen int

const (
	screenSections configScreen = iota
	screenKeys
	screenEdit
)

type configEntry struct {
	Key   string
	Value *hujson.Value
}

type configModel struct {
	screen        configScreen
	config        *hujson.Value
	sections      []string
	cursor        int
	entries       []configEntry
	editKey       string
	editInput     textinput.Model
	activeSection string

	sectionsView  viewport.Model
	keysView      viewport.Model
	width, height int
	helpModel     help.Model
	helpKM        tui.KeyMap

	saved bool
	quit  bool
}

func newConfigModel(cfg *hujson.Value) configModel {
	sections := jsonc.TopLevelKeys(cfg)
	m := configModel{
		screen:       screenSections,
		config:       cfg,
		sections:     sections,
		editInput:    textinput.New(),
		sectionsView: viewport.New(0, 0),
		keysView:     viewport.New(0, 0),
		helpModel:    tui.NewHelpModel(),
		helpKM:       tui.ListKeyMap,
	}
	m.editInput.CharLimit = 256
	m.editInput.Width = 50
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
		}
	}

	if m.screen == screenEdit {
		m.editInput, _ = m.editInput.Update(msg)
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
			m.loadSection(m.sections[m.cursor])
			m.screen = screenKeys
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
	fileInfo := theme.Dimmed.Render("Enter to expand section · s to save · q to quit")

	body := lipgloss.JoinVertical(lipgloss.Top, title, "", fileInfo, "", m.sectionsView.View())

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

	for i, name := range m.sections {
		var prefix string
		style := theme.Body
		if i == m.cursor {
			prefix = "● "
			style = theme.Highlight
		} else {
			prefix = "  "
		}

		field, ok := jsonc.GetField(m.config, name)
		typeHint := ""
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

		nameStyle := theme.Accent
		if i == m.cursor {
			nameStyle = nameStyle.Bold(true)
		}

		line := fmt.Sprintf("%s%s%s", style.Render(prefix), nameStyle.Render(name), typeHint)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m configModel) viewKeys() string {
	theme := tui.Current()

	title := theme.Title.Render(fmt.Sprintf("Section: %s", m.activeSection))
	subtitle := theme.Dimmed.Render("Enter to edit value · Esc to go back")

	body := lipgloss.JoinVertical(lipgloss.Top, title, "", subtitle, "", m.keysView.View())

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
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

func RunConfigEditor(configFile string) (*hujson.Value, bool, error) {
	cfg, err := jsonc.ReadFile(configFile)
	if err != nil {
		return nil, false, err
	}

	m := newConfigModel(cfg)
	m.loadDefaults()
	m.updateSectionsContent()

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return cfg, false, err
	}

	tm, ok := result.(configModel)
	if !ok {
		return cfg, false, nil
	}

	if tm.quit || !tm.saved {
		return cfg, false, nil
	}

	return tm.config, true, nil
}

func (m *configModel) loadDefaults() {
	m.cursor = 0
	m.scrollSections()
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
