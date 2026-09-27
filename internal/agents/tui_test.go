package agents

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func update(m *tuiModel, msg tea.Msg) *tuiModel {
	result, _ := m.Update(msg)
	return result.(*tuiModel)
}

// testCatalog is a small catalog so the TUI tests exercise real provider/model
// rows without a nenya binary.
func testCatalog() Catalog {
	return CatalogFromDescribe([]CatalogModel{
		{Provider: "anthropic", Model: "claude-sonnet-4-5"},
		{Provider: "gemini", Model: "gemini-2.5-flash"},
		{Provider: "openai", Model: "gpt-5"},
	})
}

func newTestModel() tuiModel { return newTUIModel(testCatalog()) }

func TestAgentsTUI_Init(t *testing.T) {
	t.Run("Init returns nil", func(t *testing.T) {
		m := newTestModel()
		cmd := m.Init()
		if cmd != nil {
			t.Error("Init() should return nil")
		}
	})
}

func TestAgentsTUI_NewModel(t *testing.T) {
	t.Run("creates model with auto mode on", func(t *testing.T) {
		m := newTestModel()
		if !m.modeAuto {
			t.Error("expected modeAuto to be true")
		}
		if m.screen != screenList {
			t.Errorf("screen = %d, want %d", m.screen, screenList)
		}
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m.cursor)
		}
	})
}

func TestAgentsTUI_Quit(t *testing.T) {
	t.Run("ctrl+c quits", func(t *testing.T) {
		m := newTestModel()
		updated := update(&m, tea.KeyMsg{Type: tea.KeyCtrlC})
		if !updated.done {
			t.Error("expected done flag to be set")
		}
	})

	t.Run("q quits", func(t *testing.T) {
		m := newTestModel()
		updated := update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		if !updated.done {
			t.Error("expected done flag to be set")
		}
	})

	t.Run("esc quits", func(t *testing.T) {
		m := newTestModel()
		updated := update(&m, tea.KeyMsg{Type: tea.KeyEsc})
		if !updated.done {
			t.Error("expected done flag to be set")
		}
	})
}

func TestAgentsTUI_AutoMode(t *testing.T) {
	t.Run("space toggles auto mode off when on", func(t *testing.T) {
		m := newTestModel()
		m.modeAuto = true
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
		if m.modeAuto {
			t.Error("expected modeAuto to be false after space toggle")
		}
	})

	t.Run("space on auto mode with no agents toggles and saves", func(t *testing.T) {
		m := newTestModel()
		m.modeAuto = false
		m.agents = nil
		update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
		if !m.modeAuto {
			t.Error("expected modeAuto to be true after toggle")
		}
		if !m.saved {
			t.Error("expected saved flag when toggling auto on with no agents")
		}
	})
}

func TestAgentsTUI_CursorMovement(t *testing.T) {
	t.Run("down key moves cursor down", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyDown})
		if m.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m.cursor)
		}
	})

	t.Run("up key moves cursor up", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		m.cursor = 1
		update(&m, tea.KeyMsg{Type: tea.KeyUp})
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m.cursor)
		}
	})

	t.Run("cursor does not go below 0", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyUp})
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m.cursor)
		}
	})
}

func TestAgentsTUI_AddAgent(t *testing.T) {
	t.Run("a key starts new agent", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		count := len(m.agents)
		update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
		if m.screen != screenEdit {
			t.Errorf("screen = %d, want %d", m.screen, screenEdit)
		}
		if len(m.agents) != count+1 {
			t.Errorf("agents = %d, want %d", len(m.agents), count+1)
		}
	})

	t.Run("enter on add new agent line starts new agent", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		m.cursor = len(m.agents) // cursor at "add new" line
		count := len(m.agents)
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.screen != screenEdit {
			t.Errorf("screen = %d, want %d", m.screen, screenEdit)
		}
		if len(m.agents) != count+1 {
			t.Errorf("agents = %d, want %d", len(m.agents), count+1)
		}
	})

	t.Run("enter on agent enters edit screen", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.screen != screenEdit {
			t.Errorf("screen = %d, want %d", m.screen, screenEdit)
		}
	})
}

func TestAgentsTUI_EditScreen(t *testing.T) {
	t.Run("esc cancels edit", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEsc})
		if m.screen != screenList {
			t.Errorf("screen = %d, want %d", m.screen, screenList)
		}
	})

	t.Run("enter cycles through edit cursors", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.editCursor != 1 {
			t.Errorf("editCursor = %d, want 1", m.editCursor)
		}
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.editCursor != 2 {
			t.Errorf("editCursor = %d, want 2", m.editCursor)
		}
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.screen != screenPicker {
			t.Errorf("screen = %d, want %d", m.screen, screenPicker)
		}
	})

	t.Run("tab cycles edit cursor", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyTab})
		if m.editCursor != 1 {
			t.Errorf("editCursor = %d, want 1", m.editCursor)
		}
		update(&m, tea.KeyMsg{Type: tea.KeyTab})
		if m.editCursor != 2 {
			t.Errorf("editCursor = %d, want 2", m.editCursor)
		}
	})
}

func TestAgentsTUI_StrategyNavigation(t *testing.T) {
	t.Run("down key changes strategy down", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEnter}) // advance to strategy field
		agent := m.agents[m.cursor]
		initialStrategy := agent.Strategy
		updated := update(&m, tea.KeyMsg{Type: tea.KeyDown})
		if updated.agents[updated.cursor].Strategy == initialStrategy {
			t.Error("expected strategy to change after down key")
		}
	})
}

func TestAgentsTUI_DeleteAgent(t *testing.T) {
	t.Run("d key initiates delete", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		m.cursor = 1
		update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		if m.screen != screenConfirm {
			t.Errorf("screen = %d, want %d", m.screen, screenConfirm)
		}
	})

	t.Run("y confirms delete", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		count := len(m.agents)
		m.cursor = 1
		update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
		if m.screen != screenList {
			t.Errorf("screen = %d, want %d", m.screen, screenList)
		}
		if len(m.agents) != count-1 {
			t.Errorf("agents = %d, want %d", len(m.agents), count-1)
		}
	})

	t.Run("n cancels delete", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		count := len(m.agents)
		m.cursor = 1
		update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		if m.screen != screenList {
			t.Errorf("screen = %d, want %d", m.screen, screenList)
		}
		if len(m.agents) != count {
			t.Errorf("agents = %d, want %d", len(m.agents), count)
		}
	})
}

func TestAgentsTUI_PickerScreen(t *testing.T) {
	t.Run("esc returns to edit from picker", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEsc})
		if m.screen != screenEdit {
			t.Errorf("screen = %d, want %d", m.screen, screenEdit)
		}
	})

	t.Run("enter returns to edit from picker", func(t *testing.T) {
		m := newTestModel()
		m.loadDefaults()
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.screen != screenEdit {
			t.Errorf("screen = %d, want %d", m.screen, screenEdit)
		}
	})
}

func TestAgentsTUI_WindowSize(t *testing.T) {
	t.Run("updates dimensions", func(t *testing.T) {
		m := newTestModel()
		msg := tea.WindowSizeMsg{Width: 100, Height: 50}
		update(&m, msg)
		if m.width != 100 {
			t.Errorf("width = %d, want 100", m.width)
		}
		if m.height != 50 {
			t.Errorf("height = %d, want 50", m.height)
		}
	})
}

func TestAgentsTUI_LoadDefaultsFromCatalog(t *testing.T) {
	m := newTestModel()
	m.loadDefaults()

	// A single neutral agent seeded from the first catalog model; role names
	// are nenya's domain, so no provider-derived names are fabricated.
	if len(m.agents) != 1 {
		t.Fatalf("agents = %d, want 1: %+v", len(m.agents), m.agents)
	}
	a := m.agents[0]
	if a.Name != "default" {
		t.Errorf("agent name = %q, want default", a.Name)
	}
	if a.Strategy != defaultStrategy {
		t.Errorf("strategy = %q, want %q", a.Strategy, defaultStrategy)
	}
	if len(m.catalog.Models) == 0 {
		t.Fatal("catalog is empty")
	}
	if len(a.Models) != 1 || a.Models[0] != m.catalog.Models[0].Provider+"/"+m.catalog.Models[0].Model {
		t.Errorf("models = %v, want [%s/%s]", a.Models, m.catalog.Models[0].Provider, m.catalog.Models[0].Model)
	}
}

func TestAgentsTUI_LoadDefaultsEmptyCatalog(t *testing.T) {
	m := newTUIModel(Catalog{})
	m.loadDefaults()
	if len(m.agents) != 0 {
		t.Errorf("agents = %+v, want none for an empty catalog", m.agents)
	}
}

func TestAgentsTUI_ModelFilter(t *testing.T) {
	m := newTestModel()

	m.modelFilter.SetValue("gemini")
	m.loadModels()
	if len(m.models) != 1 || m.models[0].Provider != "gemini" {
		t.Errorf("filter by provider = %+v, want the gemini row", m.models)
	}

	m.modelFilter.SetValue("claude")
	m.loadModels()
	if len(m.models) != 1 || m.models[0].ID != "claude-sonnet-4-5" {
		t.Errorf("filter by model = %+v, want claude-sonnet-4-5", m.models)
	}
}

func TestAgentsTUI_ModelSelectionIsProviderQualified(t *testing.T) {
	// Two providers offer the same model name; selecting one must not mark the
	// other, and the saved list carries the provider-qualified id.
	m := newTUIModel(CatalogFromDescribe([]CatalogModel{
		{Provider: "openrouter", Model: "gpt-5"},
		{Provider: "zen", Model: "gpt-5"},
	}))
	m.startNew()

	m.loadModels()
	if len(m.models) != 2 {
		t.Fatalf("models = %+v, want 2", m.models)
	}
	m.models[0].Selected = true
	m.syncAgentModels()

	got := m.agents[m.cursor].Models
	if len(got) != 1 || got[0] != m.models[0].Qualified {
		t.Errorf("models = %v, want [%s]", got, m.models[0].Qualified)
	}

	// Reloading must select only the matching row.
	m.loadModels()
	if !m.models[0].Selected || m.models[1].Selected {
		t.Errorf("selection not provider-scoped: %+v", m.models)
	}
}

func TestAgentsTUI_EditorResult(t *testing.T) {
	t.Run("unsaved is a no-op", func(t *testing.T) {
		m := newTestModel()
		useAuto, cfg, err := m.editorResult()
		if err != nil || useAuto || cfg != nil {
			t.Errorf("editorResult = %v, %v, %v; want false, nil, nil", useAuto, cfg, err)
		}
	})

	t.Run("saved auto returns auto", func(t *testing.T) {
		m := newTestModel()
		m.modeAuto = true
		m.saved = true
		useAuto, cfg, err := m.editorResult()
		if err != nil || !useAuto || cfg != nil {
			t.Errorf("editorResult = %v, %v, %v; want true, nil, nil", useAuto, cfg, err)
		}
	})

	t.Run("saved custom returns the agents map", func(t *testing.T) {
		m := newTestModel()
		m.saved = true
		m.modeAuto = false
		m.loadDefaults()
		useAuto, cfg, err := m.editorResult()
		if err != nil || useAuto {
			t.Fatalf("editorResult = %v, %v, %v", useAuto, cfg, err)
		}
		agentsMap, ok := cfg["agents"].(map[string]map[string]any)
		if !ok {
			t.Fatalf("cfg = %#v, want an agents map", cfg)
		}
		if len(agentsMap) != len(m.agents) {
			t.Errorf("agents map = %d, want %d", len(agentsMap), len(m.agents))
		}
	})
}
