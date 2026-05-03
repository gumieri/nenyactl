package agents

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
)

func newTestTUI() tuiModel { return newTUIModel() }

func asTUI(m tea.Model) *tuiModel {
	if ptr, ok := m.(*tuiModel); ok {
		return ptr
	}
	val := m.(tuiModel)
	return &val
}

func TestAgentTUIModeSelect(t *testing.T) {
	t.Run("auto mode selected by default", func(t *testing.T) {
		m := newTestTUI()
		if !m.modeAuto {
			t.Error("expected modeAuto to be true by default")
		}
		if m.screen != screenMode {
			t.Errorf("expected screenMode, got %d", m.screen)
		}
	})

	t.Run("space toggles auto mode", func(t *testing.T) {
		m := newTestTUI()
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		m2 := asTUI(mi)
		if m2.modeAuto {
			t.Error("expected modeAuto to be false after space")
		}
	})

	t.Run("enter with manual mode goes to list screen", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		if m2.screen != screenList {
			t.Errorf("expected screenList, got %d", m2.screen)
		}
		if len(m2.agents) != 3 {
			t.Errorf("expected 3 agents loaded, got %d", len(m2.agents))
		}
	})
}

func TestAgentTUIListNavigation(t *testing.T) {
	t.Run("d key shows delete confirmation", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		m2.loadDefaults()

		m2.cursor = 0
		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		m2 = asTUI(mi)

		if m2.screen != screenConfirm {
			t.Errorf("expected screenConfirm, got %d", m2.screen)
		}
		if m2.deleteIdx != 0 {
			t.Errorf("expected deleteIdx 0, got %d", m2.deleteIdx)
		}
	})

	t.Run("a key starts new agent", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		m2.loadDefaults()

		initialLen := len(m2.agents)
		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		m2 = asTUI(mi)

		if m2.screen != screenEdit {
			t.Errorf("expected screenEdit, got %d", m2.screen)
		}
		if len(m2.agents) != initialLen+1 {
			t.Error("expected new agent to be added")
		}
	})
}

func TestAgentTUIEditFlow(t *testing.T) {
	t.Run("enter opens model picker from edit screen", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		m2.loadDefaults()

		m2.cursor = 0
		m2.startEdit(0)

		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 = asTUI(mi)
		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 = asTUI(mi)
		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 = asTUI(mi)

		if m2.screen != screenPicker {
			t.Errorf("expected screenPicker, got %d", m2.screen)
		}
		if len(m2.models) == 0 {
			t.Error("expected models to be loaded in picker")
		}
	})

	t.Run("tab cycles field cursor", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		m2.loadDefaults()

		m2.cursor = 0
		m2.startEdit(0)

		if m2.fieldCursor != 0 {
			t.Errorf("expected fieldCursor 0, got %d", m2.fieldCursor)
		}

		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab})
		m2 = asTUI(mi)
		if m2.fieldCursor != 1 {
			t.Errorf("expected fieldCursor 1, got %d", m2.fieldCursor)
		}
	})
}

func TestAgentTUIPicker(t *testing.T) {
	t.Run("space toggles model selection", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		m2.loadDefaults()

		m2.cursor = 0
		m2.startEdit(0)
		m2.screen = screenPicker
		m2.loadModels()

		initialSelected := m2.models[0].Selected
		m2.cursor = 0
		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		m2 = asTUI(mi)

		if m2.models[0].Selected == initialSelected {
			t.Error("model selection should toggle")
		}
	})

	t.Run("filter loads matching models", func(t *testing.T) {
		m := newTestTUI()
		m.screen = screenPicker
		m.loadModels()

		m.filter = "gemini"
		m.loadModels()

		for _, mod := range m.models {
			if mod.Provider != "gemini" {
				t.Errorf("expected only gemini models, got %s", mod.Provider)
			}
		}
	})
}

func TestAgentTUIConfirmDelete(t *testing.T) {
	t.Run("y key deletes agent", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		m2.loadDefaults()

		initialLen := len(m2.agents)
		m2.cursor = 1
		m2.screen = screenConfirm
		m2.deleteIdx = 1

		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		m2 = asTUI(mi)

		if len(m2.agents) != initialLen-1 {
			t.Error("expected agent to be deleted")
		}
		if m2.screen != screenList {
			t.Errorf("expected screenList, got %d", m2.screen)
		}
	})

	t.Run("n key cancels deletion", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)
		m2.loadDefaults()

		initialLen := len(m2.agents)
		m2.cursor = 1
		m2.screen = screenConfirm
		m2.deleteIdx = 1

		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		m2 = asTUI(mi)

		if len(m2.agents) != initialLen {
			t.Error("expected no deletion")
		}
		if m2.screen != screenList {
			t.Errorf("expected screenList, got %d", m2.screen)
		}
	})
}

func TestAgentTUIEscapeHandling(t *testing.T) {
	t.Run("ctrl+c quits from mode screen", func(t *testing.T) {
		m := newTestTUI()
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		m2 := asTUI(mi)
		if !m2.done {
			t.Error("expected done to be true")
		}
	})

	t.Run("esc returns to previous screen", func(t *testing.T) {
		m := newTestTUI()
		m.screen = screenEdit
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m2 := asTUI(mi)
		if m2.screen != screenList {
			t.Errorf("expected screenList, got %d", m2.screen)
		}
	})
}
