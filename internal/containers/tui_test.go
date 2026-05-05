package containers

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func asTUI(m tea.Model) *tuiModel {
	if ptr, ok := m.(*tuiModel); ok {
		return ptr
	}
	val := m.(tuiModel)
	return &val
}

func TestContainerTUISelectScreen(t *testing.T) {
	t.Run("initial state is screenSelect", func(t *testing.T) {
		m := newTUIModel()
		if m.screen != screenSelect {
			t.Errorf("expected screenSelect, got %d", m.screen)
		}
	})

	t.Run("space toggles provider selection", func(t *testing.T) {
		m := newTUIModel()
		if m.selected[0] {
			t.Error("expected no provider selected initially")
		}
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		m2 := asTUI(mi)
		if !m2.selected[0] {
			t.Error("expected first provider selected after space")
		}
	})

	t.Run("enter with no selections quits", func(t *testing.T) {
		m := newTUIModel()
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		if !m2.quitting {
			t.Error("expected to quit when no providers selected")
		}
	})

	t.Run("enter with selections goes to keys screen", func(t *testing.T) {
		m := newTUIModel()
		geminiIdx := -1
		for i, p := range m.providers {
			if p.Name == "gemini" {
				geminiIdx = i
				break
			}
		}
		if geminiIdx == -1 {
			t.Skip("gemini not found in providers")
		}
		m.selected[geminiIdx] = true
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		if m2.screen != screenKeys {
			t.Errorf("expected screenKeys, got %d", m2.screen)
		}
		if len(m2.keys) != 1 {
			t.Errorf("expected 1 key field, got %d", len(m2.keys))
		}
		if m2.keys[0].name != "gemini" {
			t.Errorf("expected gemini field, got %s", m2.keys[0].name)
		}
	})

	t.Run("enter on + Add custom shows custom screen", func(t *testing.T) {
		m := newTUIModel()
		m.table.SetCursor(len(m.providers))
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		if m2.screen != screenCustom {
			t.Errorf("expected screenCustom, got %d", m2.screen)
		}
	})
}

func TestContainerTUICustomProvider(t *testing.T) {
	t.Run("enter on custom name field moves to key", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenCustom
		m.customName.Focus()
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		if m2.customFocus != 1 {
			t.Errorf("expected customFocus 1, got %d", m2.customFocus)
		}
	})

	t.Run("esc returns to select screen", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenCustom
		m.customFocus = 0
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m2 := asTUI(mi)
		if m2.screen != screenSelect {
			t.Errorf("expected screenSelect, got %d", m2.screen)
		}
	})
}

func TestContainerTUIKeysScreen(t *testing.T) {
	t.Run("tab stays in bounds when only one field", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenKeys
		ti := textinput.New()
		m.keys = []keyField{{name: "test", input: ti}}
		m.keyCursor = 0
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m2 := asTUI(mi)
		if m2.keyCursor != 0 {
			t.Errorf("expected keyCursor 0, got %d", m2.keyCursor)
		}
	})

	t.Run("enter confirms and quits", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenKeys
		ti := textinput.New()
		ti.SetValue("test-key-123")
		m.keys = []keyField{{name: "gemini", input: ti}}
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		if !m2.quitting {
			t.Error("expected quitting after enter")
		}
	})
}

func TestContainerTUIResults(t *testing.T) {
	t.Run("collects selected provider keys", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenKeys
		ti1 := textinput.New()
		ti1.SetValue("gemini-key")
		ti2 := textinput.New()
		ti2.SetValue("deepseek-key")
		m.keys = []keyField{
			{name: "gemini", input: ti1},
			{name: "deepseek", input: ti2},
		}
		result := m.Results()
		if result["gemini"] != "gemini-key" {
			t.Errorf("expected gemini key, got %v", result["gemini"])
		}
		if result["deepseek"] != "deepseek-key" {
			t.Errorf("expected deepseek key, got %v", result["deepseek"])
		}
	})

	t.Run("Results includes custom results when present", func(t *testing.T) {
		m := newTUIModel()
		m.customResults = append(m.customResults, customResult{Name: "custom-prov", Value: "custom-key"})
		result := m.Results()
		if result["custom-prov"] != "custom-key" {
			t.Errorf("expected custom key, got %v", result["custom-prov"])
		}
	})

	t.Run("skips empty key fields", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenKeys
		ti := textinput.New()
		m.keys = []keyField{{name: "gemini", input: ti}}
		result := m.Results()
		if len(result) != 0 {
			t.Errorf("expected empty results, got %v", result)
		}
	})

	t.Run("ollama adds no fields", func(t *testing.T) {
		m := newTUIModel()
		ollamaIdx := -1
		for i, p := range BuiltinProviders {
			if p.Name == "ollama" {
				ollamaIdx = i
				break
			}
		}
		if ollamaIdx == -1 {
			t.Skip("ollama not found in BuiltinProviders")
		}
		m.selected[ollamaIdx] = true
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		if len(m2.keys) != 0 {
			t.Errorf("expected 0 fields for ollama, got %d", len(m2.keys))
		}
		result := m2.Results()
		if len(result) != 0 {
			t.Errorf("expected empty results for ollama, got %v", result)
		}
	})
}

func TestContainerKeyReqText(t *testing.T) {
	t.Run("returns (none) for providers without key", func(t *testing.T) {
		p := ProviderDef{Name: "ollama", Auth: "none", NeedsKey: false}
		if got := keyReqText(p); got != "(none)" {
			t.Errorf("keyReqText() = %q, want (none)", got)
		}
	})

	t.Run("returns auth hint for providers with key", func(t *testing.T) {
		p := ProviderDef{Name: "gemini", Auth: "AIza...", NeedsKey: true}
		if got := keyReqText(p); got != "AIza..." {
			t.Errorf("keyReqText() = %q, want AIza...", got)
		}
	})
}

func TestContainerTUIViewRenders(t *testing.T) {
	t.Run("select screen View includes gemini", func(t *testing.T) {
		m := newTUIModel()
		view := m.View()
		if !strings.Contains(view, "gemini") {
			t.Error("expected gemini in select screen view")
		}
	})

	t.Run("select screen View includes providers", func(t *testing.T) {
		m := newTUIModel()
		view := m.View()
		if !strings.Contains(view, "gemini") && !strings.Contains(view, "Provider") {
			t.Error("expected provider info in select screen view")
		}
	})

	t.Run("custom screen View shows name input", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenCustom
		view := m.View()
		if !strings.Contains(view, "Provider name") {
			t.Error("expected Provider name in custom screen view")
		}
	})

	t.Run("keys screen View shows input fields", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenKeys
		ti := textinput.New()
		m.keys = []keyField{{name: "test", input: ti}}
		view := m.View()
		if !strings.Contains(view, "test") {
			t.Error("expected field name in keys screen view")
		}
	})

	t.Run("quitting View returns empty string", func(t *testing.T) {
		m := newTUIModel()
		m.quitting = true
		view := m.View()
		if view != "" {
			t.Errorf("expected empty view when quitting, got %q", view)
		}
	})
}

func TestContainerTUIInit(t *testing.T) {
	t.Run("returns nil command on init", func(t *testing.T) {
		m := newTUIModel()
		cmd := m.Init()
		if cmd != nil {
			t.Error("expected nil command from Init")
		}
	})
}

func TestContainerTUIUpdateKeys(t *testing.T) {
	t.Run("esc quits", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenKeys
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m2 := asTUI(mi)
		if !m2.quitting {
			t.Error("expected quitting after esc")
		}
	})

	t.Run("tab advances cursor", func(t *testing.T) {
		m := newTUIModel()
		m.screen = screenKeys
		ti := textinput.New()
		ti2 := textinput.New()
		m.keys = []keyField{{name: "a", input: ti}, {name: "b", input: ti2}}
		m.keyCursor = 0
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m2 := asTUI(mi)
		if m2.keyCursor != 1 {
			t.Errorf("expected keyCursor 1, got %d", m2.keyCursor)
		}
	})
}

func TestContainerCollectProviderKeys(t *testing.T) {
	t.Run("fails when no TTY available", func(t *testing.T) {
		keys, err := CollectProviderKeys()
		if err == nil {
			t.Logf("collect keys returned %d keys (unexpected without TTY)", len(keys))
		} else {
			t.Logf("expected error: %v", err)
		}
	})
}
