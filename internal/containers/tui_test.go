package containers

import (
	"strings"
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

func TestContainersTUI_WizardNavigation(t *testing.T) {
	providers := []ProviderDef{
		{Name: "gemini", Help: "Gemini", Auth: "sk-...", NeedsKey: true},
		{Name: "ollama", Help: "Ollama (local)", Auth: "none", NeedsKey: false},
	}

	seed := func(t *testing.T) *tuiModel {
		t.Helper()
		m := newTUIModelWithProviders(providers)
		m.selected[0] = true
		updated := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		return updated
	}

	t.Run("back navigation preserves entered key values", func(t *testing.T) {
		m := seed(t)
		m.keys[0].input.SetValue("typed-key")
		back := update(m, tea.KeyMsg{Type: tea.KeyEsc})
		if back.screen != screenSelect {
			t.Fatalf("screen = %v, want screenSelect", back.screen)
		}
		again := update(back, tea.KeyMsg{Type: tea.KeyEnter})
		if len(again.keys) != 1 {
			t.Fatalf("keys = %d, want 1", len(again.keys))
		}
		if got := again.keys[0].input.Value(); got != "typed-key" {
			t.Errorf("key value = %q, want typed-key (back-nav must not lose input)", got)
		}
	})

	t.Run("submit blocked with a clear message while required keys are empty", func(t *testing.T) {
		m := seed(t)
		blocked := update(m, tea.KeyMsg{Type: tea.KeyEnter})
		if blocked.quitting {
			t.Fatal("submit must be blocked while a required key is empty")
		}
		if blocked.validationMsg == "" {
			t.Fatal("expected a validation message naming the missing key")
		}
		if !strings.Contains(blocked.validationMsg, "gemini") {
			t.Errorf("validation message %q must name the missing provider", blocked.validationMsg)
		}
		if got := blocked.keys[0].input.Value(); got != "" {
			t.Errorf("validation must not fill the field, got %q", got)
		}
	})

	t.Run("whitespace-only key does not pass the gate", func(t *testing.T) {
		m := seed(t)
		m.keys[0].input.SetValue("   ")
		blocked := update(m, tea.KeyMsg{Type: tea.KeyEnter})
		if blocked.quitting {
			t.Fatal("whitespace-only key must not count as filled")
		}
		if !strings.Contains(blocked.validationMsg, "gemini") {
			t.Errorf("validation message %q must name the missing provider", blocked.validationMsg)
		}
	})

	t.Run("tab moves focus forward and back with shift+tab", func(t *testing.T) {
		multi := newTUIModelWithProviders([]ProviderDef{
			{Name: "gemini", Help: "Gemini", Auth: "sk-...", NeedsKey: true},
			{Name: "openai", Help: "OpenAI", Auth: "sk-...", NeedsKey: true},
		})
		multi.selected[0] = true
		multi.selected[1] = true
		m := update(&multi, tea.KeyMsg{Type: tea.KeyEnter})
		if m.keyCursor != 0 {
			t.Fatalf("keyCursor = %d, want 0", m.keyCursor)
		}
		m = update(m, tea.KeyMsg{Type: tea.KeyTab})
		if m.keyCursor != 1 {
			t.Errorf("tab: keyCursor = %d, want 1", m.keyCursor)
		}
		m = update(m, tea.KeyMsg{Type: tea.KeyShiftTab})
		if m.keyCursor != 0 {
			t.Errorf("shift+tab: keyCursor = %d, want 0", m.keyCursor)
		}
	})

	t.Run("tab wraps from the last field to the first", func(t *testing.T) {
		multi := newTUIModelWithProviders([]ProviderDef{
			{Name: "gemini", Help: "Gemini", Auth: "sk-...", NeedsKey: true},
			{Name: "openai", Help: "OpenAI", Auth: "sk-...", NeedsKey: true},
		})
		multi.selected[0] = true
		multi.selected[1] = true
		m := update(&multi, tea.KeyMsg{Type: tea.KeyEnter})
		m.keyCursor = 1
		m = update(m, tea.KeyMsg{Type: tea.KeyTab})
		if m.keyCursor != 0 {
			t.Errorf("tab wrap: keyCursor = %d, want 0", m.keyCursor)
		}
	})

	t.Run("validation message survives a no-op tab but clears on esc", func(t *testing.T) {
		m := seed(t)
		m = update(m, tea.KeyMsg{Type: tea.KeyEnter}) // arms the message
		m = update(m, tea.KeyMsg{Type: tea.KeyTab})   // single field: no move
		if m.validationMsg == "" {
			t.Error("no-op tab must not erase the validation message")
		}
		back := update(m, tea.KeyMsg{Type: tea.KeyEsc})
		if back.validationMsg != "" {
			t.Error("esc must clear the validation message")
		}
		reentered := update(back, tea.KeyMsg{Type: tea.KeyEnter})
		if reentered.validationMsg != "" {
			t.Error("re-entering the keys screen must start without a stale message")
		}
	})

	t.Run("unselecting the provider drops its typed key from results", func(t *testing.T) {
		m := seed(t)
		m.keys[0].input.SetValue("sk-typed")
		back := update(m, tea.KeyMsg{Type: tea.KeyEsc})
		back.selected[0] = false
		back.updateTableRow(0)
		again := update(back, tea.KeyMsg{Type: tea.KeyEnter})
		if !again.quitting {
			t.Fatal("no providers selected: enter should finish the wizard")
		}
		if got := again.Results(); len(got) != 0 {
			t.Errorf("Results() = %v, want empty (unselected provider's typed key must not leak)", got)
		}
	})

	t.Run("filled required keys allow submit", func(t *testing.T) {
		m := seed(t)
		m.keys[0].input.SetValue("sk-real")
		done := update(m, tea.KeyMsg{Type: tea.KeyEnter})
		if !done.quitting {
			t.Error("enter with all required keys filled should quit/finish")
		}
		if got := done.Results()["gemini"]; got != "sk-real" {
			t.Errorf("Results()[gemini] = %q, want sk-real", got)
		}
	})
}
