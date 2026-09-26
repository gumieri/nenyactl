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
}

func TestContainersTUI_ScreenNavigation(t *testing.T) {
	t.Run("table cursor is at 0 on init", func(t *testing.T) {
		m := newTUIModel()
		if m.table.Cursor() != 0 {
			t.Errorf("table cursor = %d, want 0", m.table.Cursor())
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

