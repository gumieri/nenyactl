package containers

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
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
		if m.state != screenSelect {
			t.Errorf("expected screenSelect, got %d", m.state)
		}
	})

	t.Run("space toggles provider selection", func(t *testing.T) {
		m := newTUIModel()
		if m.selected[0] {
			t.Error("gemini should not be selected initially")
		}
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		m2 := asTUI(mi)
		if !m2.selected[0] {
			t.Error("gemini should be selected after space")
		}
	})

	t.Run("enter with no selections does nothing", func(t *testing.T) {
		m := newTUIModel()
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		if m2.done {
			t.Error("should not quit when no providers selected")
		}
	})

	t.Run("enter with selections goes to keys screen", func(t *testing.T) {
		m := newTUIModel()
		// Find gemini index (sorted alphabetically)
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
		if m2.state != screenKeys {
			t.Errorf("expected screenKeys, got %d", m2.state)
		}
		if len(m2.fields) != 1 {
			t.Errorf("expected 1 key field, got %d", len(m2.fields))
		}
		if m2.fields[0].name != "gemini" {
			t.Errorf("expected gemini field, got %s", m2.fields[0].name)
		}
	})

	t.Run("enter on + Add custom shows custom entry", func(t *testing.T) {
		m := newTUIModel()
		m.cursor = len(m.providers)
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		if m2.showCustom != 0 {
			t.Errorf("expected showCustom 0, got %d", m2.showCustom)
		}
		if len(m2.customInputs) != 2 {
			t.Errorf("expected 2 custom inputs, got %d", len(m2.customInputs))
		}
	})
}

func TestContainerTUICustomProvider(t *testing.T) {
	t.Run("tab cycles between name and key fields", func(t *testing.T) {
		m := newTUIModel()
		m.cursor = len(m.providers)
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)

		if m2.customCursor != 0 {
			t.Error("expected customCursor 0")
		}
		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab})
		m2 = asTUI(mi)
		if m2.customCursor != 1 {
			t.Error("expected customCursor 1 after tab")
		}
	})
}

func TestContainerTUIKeysScreen(t *testing.T) {
	t.Run("tab cycles between fields", func(t *testing.T) {
		m := newTUIModel()
		geminiIdx := -1
		for i, p := range m.providers {
			if p.Name == "gemini" {
				geminiIdx = i
				break
			}
		}
		if geminiIdx == -1 {
			t.Skip("gemini not found")
		}
		m.selected[geminiIdx] = true
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)

		if m2.fieldCursor != 0 {
			t.Error("expected fieldCursor 0")
		}
		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab})
		m2 = asTUI(mi)
		if m2.fieldCursor != 1 && len(m2.fields) > 1 {
			t.Error("expected fieldCursor to increment")
		}
	})

	t.Run("enter confirms and quits", func(t *testing.T) {
		m := newTUIModel()
		geminiIdx := -1
		for i, p := range m.providers {
			if p.Name == "gemini" {
				geminiIdx = i
				break
			}
		}
		if geminiIdx == -1 {
			t.Skip("gemini not found")
		}
		m.selected[geminiIdx] = true
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)

		// Set value on whatever field is first
		if len(m2.fields) > 0 {
			m2.fields[0].input.SetValue("test-key-123")
		}
		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 = asTUI(mi)

		if !m2.done {
			t.Error("expected done to be true")
		}
	})
}

func TestContainerTUIResults(t *testing.T) {
	t.Run("collects selected provider keys", func(t *testing.T) {
		m := newTUIModel()
		geminiIdx, deepseekIdx := -1, -1
		for i, p := range m.providers {
			if p.Name == "gemini" {
				geminiIdx = i
			}
			if p.Name == "deepseek" {
				deepseekIdx = i
			}
		}
		if geminiIdx == -1 || deepseekIdx == -1 {
			t.Skip("gemini or deepseek not found")
		}
		m.selected[geminiIdx] = true
		m.selected[deepseekIdx] = true
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)

		for i := range m2.fields {
			if m2.fields[i].name == "gemini" {
				m2.fields[i].input.SetValue("gemini-key")
			}
			if m2.fields[i].name == "deepseek" {
				m2.fields[i].input.SetValue("deepseek-key")
			}
		}
		result := m2.Results()

		if result["gemini"] != "gemini-key" {
			t.Errorf("expected gemini key, got %v", result["gemini"])
		}
		if result["deepseek"] != "deepseek-key" {
			t.Errorf("expected deepseek key, got %v", result["deepseek"])
		}
	})

	t.Run("Results includes custom results when present", func(t *testing.T) {
		m := newTUIModel()
		// Directly add custom results to the model
		m.customResults = append(m.customResults, keyResult{Name: "custom-prov", Value: "custom-key"})

		result := m.Results()
		if result["custom-prov"] != "custom-key" {
			t.Errorf("expected custom key, got %v", result["custom-prov"])
		}
	})

	t.Run("skips providers that don't need keys", func(t *testing.T) {
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

		m := newTUIModel()
		m.selected[ollamaIdx] = true
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)

		if len(m2.fields) != 0 {
			t.Errorf("expected 0 fields for ollama, got %d", len(m2.fields))
		}
		result := m2.Results()
		if len(result) != 0 {
			t.Errorf("expected empty results for ollama, got %v", result)
		}
	})
}
