package containers

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func update(m *tuiModel, msg tea.Msg) *tuiModel {
	result, _ := m.Update(msg)
	return result.(*tuiModel)
}

func TestContainersTUI_Init(t *testing.T) {
	t.Run("Init returns nil", func(t *testing.T) {
		m := newTUIModel()
		cmd := m.Init()
		if cmd != nil {
			t.Error("Init() should return nil")
		}
	})
}

func TestContainersTUI_NewModel(t *testing.T) {
	t.Run("creates model with default values", func(t *testing.T) {
		m := newTUIModel()

		if m.screen != screenSelect {
			t.Errorf("screen = %d, want %d", m.screen, screenSelect)
		}
		if len(m.providers) == 0 {
			t.Error("providers should not be empty")
		}
		if m.selected == nil {
			t.Error("selected map should not be nil")
		}
	})

	t.Run("table has rows", func(t *testing.T) {
		m := newTUIModel()
		rows := m.table.Rows()
		if len(rows) == 0 {
			t.Error("table should have rows")
		}
	})

	t.Run("providers include built-in providers", func(t *testing.T) {
		m := newTUIModel()
		foundGemini := false
		for _, p := range m.providers {
			if p.Name == "gemini" {
				foundGemini = true
				break
			}
		}
		if !foundGemini {
			t.Error("expected gemini provider in list")
		}
	})
}

func TestContainersTUI_WindowSize(t *testing.T) {
	t.Run("updates dimensions", func(t *testing.T) {
		m := newTUIModel()
		msg := tea.WindowSizeMsg{Width: 100, Height: 50}

		updated := update(&m, msg)

		if updated.width != 100 {
			t.Errorf("width = %d, want 100", updated.width)
		}
	})
}

func TestContainersTUI_Quit(t *testing.T) {
	t.Run("ctrl+c quits", func(t *testing.T) {
		m := newTUIModel()
		updated := update(&m, tea.KeyMsg{Type: tea.KeyCtrlC})
		if !updated.quitting {
			t.Error("expected quitting flag to be set")
		}
	})

	t.Run("q quits", func(t *testing.T) {
		m := newTUIModel()
		updated := update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		if !updated.quitting {
			t.Error("expected quitting flag to be set")
		}
	})
}

func TestContainersTUI_ProviderDef(t *testing.T) {
	t.Run("BuiltinProviders contains expected values", func(t *testing.T) {
		var ollama ProviderDef
		for _, p := range BuiltinProviders {
			if p.Name == "ollama" {
				ollama = p
				break
			}
		}
		if ollama.Name != "ollama" {
			t.Error("expected ollama to be in BuiltinProviders")
		}
		if ollama.NeedsKey {
			t.Error("ollama should not need a key")
		}
	})

	t.Run("contract catalog providers are used without mutating the shim", func(t *testing.T) {
		before := append([]ProviderDef{}, BuiltinProviders...)
		m := newTUIModelWithProviders([]ProviderDef{
			{Name: "anthropic", Help: "Anthropic", NeedsKey: true},
		})
		if len(m.providers) != 1 || m.providers[0].Name != "anthropic" {
			t.Errorf("providers = %+v, want the contract catalog", m.providers)
		}
		if len(BuiltinProviders) != len(before) {
			t.Errorf("BuiltinProviders was mutated: %d -> %d", len(before), len(BuiltinProviders))
		}
	})
}

func TestContainersTUI_ScreenNavigation(t *testing.T) {
	t.Run("table cursor is at 0 on init", func(t *testing.T) {
		m := newTUIModel()
		if m.table.Cursor() != 0 {
			t.Errorf("table cursor = %d, want 0", m.table.Cursor())
		}
	})

	t.Run("down and j move the provider cursor", func(t *testing.T) {
		m := newTUIModel()
		updated := update(&m, tea.KeyMsg{Type: tea.KeyDown})
		if updated.table.Cursor() != 1 {
			t.Errorf("after down, cursor = %d, want 1", updated.table.Cursor())
		}
		updated = update(updated, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if updated.table.Cursor() != 2 {
			t.Errorf("after j, cursor = %d, want 2", updated.table.Cursor())
		}
		updated = update(updated, tea.KeyMsg{Type: tea.KeyUp})
		if updated.table.Cursor() != 1 {
			t.Errorf("after up, cursor = %d, want 1", updated.table.Cursor())
		}
	})

	t.Run("help map varies per screen and drops navigation on inputs", func(t *testing.T) {
		m := newTUIModel()
		selectKeys := m.helpKeyMap()
		if selectKeys.Toggle.Help().Key == "" {
			t.Error("select screen should advertise the toggle key")
		}
		m.screen = screenKeys
		keysScreen := m.helpKeyMap()
		if keysScreen.Up.Help().Key != "" {
			t.Error("keys screen must not advertise navigation")
		}
	})
}

func TestContainersTUI_KeysScreen(t *testing.T) {
	t.Run("esc returns to select screen", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenKeys
		updated := update(&m, tea.KeyMsg{Type: tea.KeyEsc})
		if updated.screen != screenSelect {
			t.Errorf("screen = %d, want %d", updated.screen, screenSelect)
		}
	})

	t.Run("ctrl+c quits from keys screen", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenKeys
		updated := update(&m, tea.KeyMsg{Type: tea.KeyCtrlC})
		if !updated.quitting {
			t.Error("expected quitting flag to be set")
		}
	})
}

func TestContainersTUI_CustomScreen(t *testing.T) {
	t.Run("esc returns to select screen", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenCustom
		updated := update(&m, tea.KeyMsg{Type: tea.KeyEsc})
		if updated.screen != screenSelect {
			t.Errorf("screen = %d, want %d", updated.screen, screenSelect)
		}
	})

	t.Run("ctrl+c quits from custom screen", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenCustom
		updated := update(&m, tea.KeyMsg{Type: tea.KeyCtrlC})
		if !updated.quitting {
			t.Error("expected quitting flag to be set")
		}
	})
}
