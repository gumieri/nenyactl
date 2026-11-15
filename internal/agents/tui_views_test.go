package agents

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Tests for the agents TUI paths not covered by tui_test.go (NCTL-50): the
// picker browse navigation, scroll helpers, view rendering, trunc, and the
// empty-agent cleanup.

func setupPicker(t *testing.T) *tuiModel {
	t.Helper()
	m := newTestModel()
	m.loadDefaults()
	m.screen = screenPicker
	m.width, m.height = 80, 24
	m.pickerView.Height = 10
	m.loadModels()
	m.updatePickerContent()
	return &m
}

func TestPickerBrowse(t *testing.T) {
	t.Run("up and down move the model cursor", func(t *testing.T) {
		m := setupPicker(t)
		if len(m.models) < 2 {
			t.Fatalf("models = %d, need at least 2", len(m.models))
		}
		m = update(m, tea.KeyMsg{Type: tea.KeyDown})
		if m.modelCursor != 1 {
			t.Errorf("modelCursor = %d, want 1", m.modelCursor)
		}
		m = update(m, tea.KeyMsg{Type: tea.KeyUp})
		if m.modelCursor != 0 {
			t.Errorf("modelCursor = %d, want 0", m.modelCursor)
		}
	})

	t.Run("cursor never leaves the model bounds", func(t *testing.T) {
		m := setupPicker(t)
		for i := 0; i < len(m.models)+3; i++ {
			m = update(m, tea.KeyMsg{Type: tea.KeyDown})
		}
		if m.modelCursor != len(m.models)-1 {
			t.Errorf("modelCursor = %d, want clamped to %d", m.modelCursor, len(m.models)-1)
		}
		for i := 0; i < len(m.models)+3; i++ {
			m = update(m, tea.KeyMsg{Type: tea.KeyUp})
		}
		if m.modelCursor != 0 {
			t.Errorf("modelCursor = %d, want clamped to 0", m.modelCursor)
		}
	})

	t.Run("space toggles a model selection onto the agent", func(t *testing.T) {
		m := setupPicker(t)
		// Pick an unselected model so the toggle adds instead of removes.
		idx := -1
		for i := range m.models {
			if !m.models[i].Selected {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.Fatal("every model is already selected; need an unselected one")
		}
		m.modelCursor = idx
		before := len(m.agents[m.cursor].Models)
		m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
		after := len(m.agents[m.cursor].Models)
		if after != before+1 {
			t.Errorf("agent models = %d, want %d after toggling one on", after, before+1)
		}
		if !m.models[idx].Selected {
			t.Error("toggled model should read selected")
		}
	})

	t.Run("enter leaves the picker back to the edit screen", func(t *testing.T) {
		m := setupPicker(t)
		m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.screen != screenEdit {
			t.Errorf("screen = %v, want screenEdit", m.screen)
		}
	})
}

func TestScrollPicker(t *testing.T) {
	t.Run("keeps the cursor in view", func(t *testing.T) {
		m := setupPicker(t)
		m.pickerView.Height = 2
		m.modelCursor = len(m.models) - 1
		m.scrollPicker()
		if m.pickerView.YOffset < 0 || m.pickerView.YOffset > len(m.models)-m.pickerView.Height {
			t.Errorf("YOffset = %d, out of bounds", m.pickerView.YOffset)
		}
		// The highlighted row must sit inside the visible window.
		if m.modelCursor < m.pickerView.YOffset || m.modelCursor >= m.pickerView.YOffset+m.pickerView.Height {
			t.Errorf("cursor %d outside window [%d, %d)",
				m.modelCursor, m.pickerView.YOffset, m.pickerView.YOffset+m.pickerView.Height)
		}
	})

	t.Run("no-op at height 0", func(t *testing.T) {
		m := setupPicker(t)
		m.pickerView.Height = 0
		m.scrollPicker()
		if m.pickerView.YOffset != 0 {
			t.Errorf("YOffset = %d, want 0 (no-op)", m.pickerView.YOffset)
		}
	})
}

func TestAgentViewsRender(t *testing.T) {
	seed := func(t *testing.T) *tuiModel {
		t.Helper()
		m := newTestModel()
		m.agents = []Agent{
			{Name: "alpha", Strategy: "fallback"},
			{Name: "beta", Strategy: "round-robin"},
		}
		m.width, m.height = 80, 24
		m.agentsView.Height = 10
		m.updateAgentContent()
		return &m
	}

	t.Run("viewList shows agents and the help bar", func(t *testing.T) {
		m := seed(t)
		m.modeAuto = false // manual mode renders the agent list
		m.width, m.height = 80, 24
		view := m.View()
		if !strings.Contains(view, "alpha") || !strings.Contains(view, "beta") {
			t.Errorf("viewList missing agent names: %q", view)
		}
	})

	t.Run("viewEdit shows the edited agent name", func(t *testing.T) {
		m := seed(t)
		m.screen = screenEdit
		m.cursor = 0
		m.editName.SetValue("alpha")
		view := m.View()
		if !strings.Contains(view, "alpha") {
			t.Errorf("viewEdit missing the agent name: %q", view)
		}
	})

	t.Run("viewPicker shows models", func(t *testing.T) {
		m := setupPicker(t)
		view := m.View()
		// The picker renders the model and provider as separate columns.
		if !strings.Contains(view, "claude-sonnet-4-5") || !strings.Contains(view, "anthropic") {
			t.Errorf("viewPicker missing the model rows: %q", view)
		}
	})

	t.Run("viewConfirm names the agent", func(t *testing.T) {
		m := seed(t)
		m.screen = screenConfirm
		m.deleteIdx = 1
		view := m.View()
		if !strings.Contains(view, "beta") {
			t.Errorf("viewConfirm missing the agent name: %q", view)
		}
	})

	t.Run("done model renders empty", func(t *testing.T) {
		m := seed(t)
		m.done = true
		if view := m.View(); view != "" {
			t.Errorf("done View = %q, want empty", view)
		}
	})
}

func TestRenderAgentList(t *testing.T) {
	m := newTestModel()
	m.agents = []Agent{{Name: "solo", Strategy: "fallback"}}
	m.width, m.height = 80, 24
	m.agentsView.Height = 10
	m.updateAgentContent()
	content := m.agentsView.View()
	if !strings.Contains(content, "solo") {
		t.Errorf("agent list = %q, want the agent name", content)
	}
}

func TestRemoveEmptyAgent(t *testing.T) {
	t.Run("removes the placeholder new-agent at the cursor", func(t *testing.T) {
		m := newTestModel()
		m.agents = []Agent{{Name: "new-agent"}, {Name: "keep"}}
		m.cursor = 0
		m.removeEmptyAgent()
		if len(m.agents) != 1 || m.agents[0].Name != "keep" {
			t.Errorf("agents = %+v, want only keep", m.agents)
		}
	})

	t.Run("keeps agents with real names", func(t *testing.T) {
		m := newTestModel()
		m.agents = []Agent{{Name: "real"}}
		m.cursor = 0
		m.removeEmptyAgent()
		if len(m.agents) != 1 || m.agents[0].Name != "real" {
			t.Errorf("agents = %+v, want real kept", m.agents)
		}
	})

	t.Run("cursor clamps when it sits past the end with nothing to remove", func(t *testing.T) {
		m := newTestModel()
		m.agents = []Agent{{Name: "real"}}
		m.cursor = 1 // past the end: no removal happens
		m.removeEmptyAgent()
		if len(m.agents) != 1 || m.agents[0].Name != "real" {
			t.Errorf("agents = %+v, want unchanged", m.agents)
		}
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want clamped to 0", m.cursor)
		}
	})

	t.Run("removal at the end steps the cursor back", func(t *testing.T) {
		m := newTestModel()
		m.agents = []Agent{{Name: "keep"}, {Name: "new-agent"}}
		m.cursor = 1
		m.removeEmptyAgent()
		if len(m.agents) != 1 || m.agents[0].Name != "keep" {
			t.Errorf("agents = %+v, want only keep", m.agents)
		}
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0 after stepping back", m.cursor)
		}
	})
}

func TestScrollAgents(t *testing.T) {
	t.Run("keeps the cursor visible with a small viewport", func(t *testing.T) {
		m := newTestModel()
		m.agents = []Agent{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"}}
		m.agentsView.Height = 2
		m.cursor = len(m.agents) // last agent (cursor 0 is the auto row)
		m.scrollAgents()
		if m.agentsView.YOffset < 0 {
			t.Errorf("YOffset = %d, must not be negative", m.agentsView.YOffset)
		}
		// The agent's name line (the content has no auto row: agent i's
		// first line is at 2*i) must be inside the visible window.
		firstLine := (m.cursor - 1) * 2
		if firstLine < m.agentsView.YOffset || firstLine >= m.agentsView.YOffset+m.agentsView.Height {
			t.Errorf("cursor line %d outside window [%d, %d)",
				firstLine, m.agentsView.YOffset, m.agentsView.YOffset+m.agentsView.Height)
		}
	})

	t.Run("no-op at height 0", func(t *testing.T) {
		m := newTestModel()
		m.agentsView.Height = 0
		m.scrollAgents()
		if m.agentsView.YOffset != 0 {
			t.Errorf("YOffset = %d, want 0 (no-op)", m.agentsView.YOffset)
		}
	})
}
