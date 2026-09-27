package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

type KeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Back   key.Binding
	Quit   key.Binding
	Delete key.Binding
	Add    key.Binding
	Toggle key.Binding
	Save   key.Binding
}

func NewHelpModel() help.Model {
	m := help.New()
	theme := Current()
	m.Styles = help.Styles{
		ShortKey:       theme.Dimmed,
		ShortDesc:      theme.Body,
		ShortSeparator: theme.Dimmed,
		FullKey:        theme.Dimmed,
		FullDesc:       theme.Body,
		FullSeparator:  theme.Dimmed,
		Ellipsis:       theme.Dimmed,
	}
	return m
}

func (k KeyMap) ShortHelp() []key.Binding {
	b := []key.Binding{}
	for _, x := range []key.Binding{k.Up, k.Down, k.Select, k.Back, k.Quit, k.Delete, k.Add, k.Toggle, k.Save} {
		if x.Help().Key != "" || x.Help().Desc != "" {
			b = append(b, x)
		}
	}
	return b
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

// SectionsKeyMap is the config-editor sections screen: navigation, expand, and
// save, but no delete/add/toggle (which belong to other screens).
var SectionsKeyMap = KeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "quit")),
	Quit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Save:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save")),
}

// KeysKeyMap is the config-editor keys screen: navigation and edit only.
var KeysKeyMap = KeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

// AgentsKeyMap is the agents screen: navigation, save, add, delete, toggle.
var AgentsKeyMap = KeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:   key.NewBinding(key.WithKeys("ctrl+c", "q"), key.WithHelp("ctrl+c", "quit")),
	Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	Add:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
	Toggle: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle auto")),
	Save:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save")),
}

var FormKeyMap = KeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "next")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Toggle: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
}

var ConfirmKeyMap = KeyMap{
	Select: key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y/enter", "confirm")),
	Back:   key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "cancel")),
}

var PickerKeyMap = KeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "done")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Toggle: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
}
