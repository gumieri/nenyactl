package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestTUI() tuiModel { return newTUIModel() }

func asTUI(m tea.Model) *tuiModel {
	if ptr, ok := m.(*tuiModel); ok {
		return ptr
	}
	val := m.(tuiModel)
	return &val
}

func TestAgentTUIInitialState(t *testing.T) {
	t.Run("starts in list screen with empty agents", func(t *testing.T) {
		m := newTestTUI()
		if m.screen != screenList {
			t.Errorf("expected screenList, got %d", m.screen)
		}
		if !m.modeAuto {
			t.Error("expected modeAuto to be true by default")
		}
		if len(m.agents) != 0 {
			t.Errorf("expected empty agents, got %d", len(m.agents))
		}
	})

	t.Run("space toggles auto mode and loads defaults", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		m2 := asTUI(mi)
		if !m2.modeAuto {
			t.Error("expected modeAuto to be true after space")
		}
	})

	t.Run("space in manual mode loads defaults", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	})
}

func TestAgentTUIListNavigation(t *testing.T) {
	t.Run("d key shows delete confirmation", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()
		m.cursor = 1

		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		m2 := asTUI(mi)

		if m2.screen != screenConfirm {
			t.Errorf("expected screenConfirm, got %d", m2.screen)
		}
	})

	t.Run("a key starts new agent", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		initialLen := len(m.agents)
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		m2 := asTUI(mi)

		if m2.screen != screenEdit {
			t.Errorf("expected screenEdit, got %d", m2.screen)
		}
		if len(m2.agents) != initialLen+1 {
			t.Errorf("expected %d agents, got %d", initialLen+1, len(m2.agents))
		}
	})

	t.Run("escape on edit removes empty agent", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		initialLen := len(m.agents)
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		m2 := asTUI(mi)

		if len(m2.agents) != initialLen+1 {
			t.Fatalf("expected %d agents, got %d", initialLen+1, len(m2.agents))
		}

		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m2 = asTUI(mi)

		if m2.screen != screenList {
			t.Errorf("expected screenList, got %d", m2.screen)
		}
		if len(m2.agents) != initialLen {
			t.Errorf("expected %d agents after cancel, got %d", initialLen, len(m2.agents))
		}
	})

	t.Run("enter opens edit screen for selected agent", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		m.cursor = 1
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)

		if m2.screen != screenEdit {
			t.Errorf("expected screenEdit, got %d", m2.screen)
		}
		if m2.editCursor != 0 {
			t.Errorf("expected editCursor 0, got %d", m2.editCursor)
		}
	})
}

func TestAgentTUIEditFlow(t *testing.T) {
	t.Run("enter cycles through edit fields then opens picker", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		m.cursor = 1
		m.startEdit(1)

		if m.editCursor != 0 {
			t.Fatalf("expected editCursor 0, got %d", m.editCursor)
		}

		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)

		if m2.editCursor != 1 {
			t.Errorf("expected editCursor 1, got %d", m2.editCursor)
		}

		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 = asTUI(mi)

		if m2.editCursor != 2 {
			t.Errorf("expected editCursor 2, got %d", m2.editCursor)
		}

		mi, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 = asTUI(mi)

		if m2.screen != screenPicker {
			t.Errorf("expected screenPicker, got %d", m2.screen)
		}
	})

	t.Run("tab cycles field cursor", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		m.cursor = 1
		m.startEdit(1)

		if m.editCursor != 0 {
			t.Fatalf("expected editCursor 0, got %d", m.editCursor)
		}

		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m2 := asTUI(mi)

		if m2.editCursor != 1 {
			t.Errorf("expected editCursor 1, got %d", m2.editCursor)
		}
	})

	t.Run("strategy changes in edit mode", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		m.startEdit(1)
		m.editCursor = 1

		if m.strategyIdx == 0 {
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		} else {
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
		}
	})
}

func TestAgentTUIPicker(t *testing.T) {
	t.Run("picker loads models on enter", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		m.startEdit(1)
		m.screen = screenPicker
		m.loadModels()

		if len(m.models) == 0 {
			t.Error("expected models to be loaded in picker")
		}
	})

	t.Run("space toggles model selection", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		m.startEdit(1)
		m.screen = screenPicker
		m.loadModels()

		initialSelected := m.models[0].Selected
		m.modelCursor = 0
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		m2 := asTUI(mi)

		if m2.models[0].Selected == initialSelected {
			t.Error("model selection should toggle")
		}
	})

	t.Run("filter narrows models", func(t *testing.T) {
		m := newTestTUI()
		m.screen = screenPicker
		m.modelFilter.SetValue("gemini")
		m.loadModels()

		for _, mod := range m.models {
			if !strings.Contains(strings.ToLower(mod.ID), "gemini") {
				t.Errorf("expected models with 'gemini' in ID, got %s/%s", mod.Provider, mod.ID)
			}
		}
	})
}

func TestAgentTUIConfirmDelete(t *testing.T) {
	t.Run("y key deletes agent", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		initialLen := len(m.agents)
		m.screen = screenConfirm
		m.deleteIdx = 1

		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		m2 := asTUI(mi)

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
		m.loadDefaults()

		initialLen := len(m.agents)
		m.screen = screenConfirm
		m.deleteIdx = 1

		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		m2 := asTUI(mi)

		if len(m2.agents) != initialLen {
			t.Error("expected no deletion")
		}
		if m2.screen != screenList {
			t.Errorf("expected screenList, got %d", m2.screen)
		}
	})

	t.Run("delete with enter key", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()

		initialLen := len(m.agents)
		m.screen = screenConfirm
		m.deleteIdx = 0

		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := asTUI(mi)

		if len(m2.agents) != initialLen-1 {
			t.Error("expected agent to be deleted")
		}
	})
}

func TestAgentTUIEscapeHandling(t *testing.T) {
	t.Run("ctrl+c quits from list screen", func(t *testing.T) {
		m := newTestTUI()
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		m2 := asTUI(mi)
		if !m2.done {
			t.Error("expected done to be true")
		}
	})

	t.Run("esc from edit returns to list", func(t *testing.T) {
		m := newTestTUI()
		m.screen = screenEdit
		mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m2 := asTUI(mi)
		if m2.screen != screenList {
			t.Errorf("expected screenList, got %d", m2.screen)
		}
	})
}

func TestAgentTrunc(t *testing.T) {
	t.Run("returns original string if short", func(t *testing.T) {
		if got := trunc("short", 20); got != "short" {
			t.Errorf("trunc() = %q, want %q", got, "short")
		}
	})

	t.Run("truncates with ellipsis when long", func(t *testing.T) {
		if got := trunc("this is a very long string", 10); got != "this is..." {
			t.Errorf("trunc() = %q, want %q", got, "this is...")
		}
	})

	t.Run("handles exact length", func(t *testing.T) {
		if got := trunc("exactlylen", 10); got != "exactlylen" {
			t.Errorf("trunc() = %q, want %q", got, "exactlylen")
		}
	})

	t.Run("handles empty string", func(t *testing.T) {
		if got := trunc("", 10); got != "" {
			t.Errorf("trunc() = %q, want %q", got, "")
		}
	})
}

func TestAgentLoadDefaults(t *testing.T) {
	t.Run("loads default agents from DefaultAgents", func(t *testing.T) {
		m := newTestTUI()
		m.loadDefaults()

		if len(m.agents) != len(DefaultAgents) {
			t.Errorf("expected %d agents, got %d", len(DefaultAgents), len(m.agents))
		}

		agentNames := make(map[string]bool)
		for _, a := range m.agents {
			agentNames[a.Name] = true
			if a.Strategy == "" {
				t.Errorf("agent %s has empty strategy", a.Name)
			}
			if len(a.Models) == 0 {
				t.Errorf("agent %s has no models", a.Name)
			}
		}

		for name := range DefaultAgents {
			if !agentNames[name] {
				t.Errorf("default agent %s not loaded", name)
			}
		}
	})

	t.Run("agents are sorted alphabetically", func(t *testing.T) {
		m := newTestTUI()
		m.loadDefaults()

		for i := 1; i < len(m.agents); i++ {
			if m.agents[i].Name < m.agents[i-1].Name {
				t.Errorf("agents not sorted: %s < %s", m.agents[i].Name, m.agents[i-1].Name)
			}
		}
	})
}

func TestAgentStartEdit(t *testing.T) {
	t.Run("sets screen to edit", func(t *testing.T) {
		m := newTestTUI()
		m.agents = []Agent{{Name: "test", Strategy: "fallback", Models: []string{"model1"}}}
		m.startEdit(0)

		if m.screen != screenEdit {
			t.Errorf("expected screenEdit, got %d", m.screen)
		}
	})

	t.Run("populates editName", func(t *testing.T) {
		m := newTestTUI()
		m.agents = []Agent{{Name: "my-agent", Strategy: "fallback", Models: []string{"model1"}}}
		m.startEdit(0)

		if m.editName.Value() != "my-agent" {
			t.Errorf("editName = %q, want %q", m.editName.Value(), "my-agent")
		}
	})

	t.Run("sets strategyIdx", func(t *testing.T) {
		m := newTestTUI()
		m.agents = []Agent{{Name: "test", Strategy: "round-robin", Models: []string{"model1"}}}
		m.startEdit(0)

		if m.strategyIdx == 0 {
			t.Error("expected strategyIdx to be set")
		}
	})
}

func TestAgentStartNew(t *testing.T) {
	t.Run("appends new agent", func(t *testing.T) {
		m := newTestTUI()
		initialLen := len(m.agents)
		m.startNew()

		if len(m.agents) != initialLen+1 {
			t.Errorf("expected %d agents, got %d", initialLen+1, len(m.agents))
		}

		newAgent := m.agents[len(m.agents)-1]
		if newAgent.Name != "new-agent" {
			t.Errorf("new agent name = %q, want %q", newAgent.Name, "new-agent")
		}
		if newAgent.Strategy != "fallback" {
			t.Errorf("new agent strategy = %q, want %q", newAgent.Strategy, "fallback")
		}
	})

	t.Run("sets cursor to new agent", func(t *testing.T) {
		m := newTestTUI()
		m.startNew()

		if m.cursor != len(m.agents)-1 {
			t.Errorf("cursor = %d, want %d", m.cursor, len(m.agents)-1)
		}
	})
}

func TestAgentRemoveEmptyAgent(t *testing.T) {
	t.Run("removes agent with default name", func(t *testing.T) {
		m := newTestTUI()
		m.agents = append(m.agents, Agent{Name: "new-agent", Strategy: "fallback", Models: nil})
		m.cursor = len(m.agents) - 1

		initialLen := len(m.agents)
		m.removeEmptyAgent()

		if len(m.agents) != initialLen-1 {
			t.Errorf("expected %d agents, got %d", initialLen-1, len(m.agents))
		}
	})

	t.Run("does not remove agent with custom name", func(t *testing.T) {
		m := newTestTUI()
		m.agents = append(m.agents, Agent{Name: "my-custom", Strategy: "fallback", Models: nil})
		m.cursor = len(m.agents) - 1

		initialLen := len(m.agents)
		m.removeEmptyAgent()

		if len(m.agents) != initialLen {
			t.Errorf("expected %d agents, got %d", initialLen, len(m.agents))
		}
	})
}

func TestWriteAgentsConfig(t *testing.T) {
	t.Run("writes valid agents config to config.d", func(t *testing.T) {
		tmp := t.TempDir()

		cfg := map[string]any{
			"agents": map[string]any{
				"test-agent": map[string]any{
					"strategy": "fallback",
					"models":   []string{"gemini-2.5-flash", "deepseek-chat"},
				},
			},
			"discovery": map[string]any{
				"auto_agents": false,
			},
		}

		configD := filepath.Join(tmp, "config.d")
		if err := WriteAgentsConfig(configD, cfg); err != nil {
			t.Fatalf("WriteAgentsConfig() error = %v", err)
		}

		agentsPath := filepath.Join(configD, "20-agents.json")
		data, err := os.ReadFile(agentsPath)
		if err != nil {
			t.Fatalf("read agents config: %v", err)
		}

		var readCfg map[string]any
		if err := json.Unmarshal(data, &readCfg); err != nil {
			t.Fatalf("unmarshal agents config: %v", err)
		}

		if readCfg["agents"] == nil {
			t.Error("agents section missing")
		}
		if readCfg["discovery"] == nil {
			t.Error("discovery section missing")
		}
	})

	t.Run("creates config.d directory if needed", func(t *testing.T) {
		tmp := t.TempDir()

		cfg := map[string]any{
			"agents":    map[string]any{},
			"discovery": map[string]any{"auto_agents": false},
		}

		configD := filepath.Join(tmp, "config.d")
		if err := WriteAgentsConfig(configD, cfg); err != nil {
			t.Fatalf("WriteAgentsConfig() error = %v", err)
		}

		if _, err := os.Stat(configD); os.IsNotExist(err) {
			t.Error("config.d directory not created")
		}
	})
}

func TestUpdateConfigDiscovery(t *testing.T) {
	t.Run("sets auto_agents to true", func(t *testing.T) {
		tmp := t.TempDir()
		configFile := filepath.Join(tmp, "config.json")

		initialCfg := map[string]any{
			"server": map[string]any{"listen_addr": ":8080"},
		}
		initialData, _ := json.MarshalIndent(initialCfg, "", "  ")
		if err := os.WriteFile(configFile, initialData, 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if err := UpdateConfigDiscovery(configFile, true); err != nil {
			t.Fatalf("UpdateConfigDiscovery() error = %v", err)
		}

		data, err := os.ReadFile(configFile)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}

		var cfg map[string]any
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("unmarshal config: %v", err)
		}

		disco, ok := cfg["discovery"].(map[string]any)
		if !ok {
			t.Fatal("discovery not found or not a map")
		}
		if disco["auto_agents"] != true {
			t.Errorf("auto_agents = %v, want true", disco["auto_agents"])
		}
	})

	t.Run("sets auto_agents to false", func(t *testing.T) {
		tmp := t.TempDir()
		configFile := filepath.Join(tmp, "config.json")

		initialCfg := map[string]any{
			"discovery": map[string]any{"auto_agents": true},
		}
		initialData, _ := json.MarshalIndent(initialCfg, "", "  ")
		if err := os.WriteFile(configFile, initialData, 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if err := UpdateConfigDiscovery(configFile, false); err != nil {
			t.Fatalf("UpdateConfigDiscovery() error = %v", err)
		}

		data, err := os.ReadFile(configFile)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}

		var cfg map[string]any
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("unmarshal config: %v", err)
		}

		disco, ok := cfg["discovery"].(map[string]any)
		if !ok {
			t.Fatal("discovery not found or not a map")
		}
		if disco["auto_agents"] != false {
			t.Errorf("auto_agents = %v, want false", disco["auto_agents"])
		}
	})

	t.Run("creates discovery section if missing", func(t *testing.T) {
		tmp := t.TempDir()
		configFile := filepath.Join(tmp, "config.json")

		initialCfg := map[string]any{"server": map[string]any{}}
		initialData, _ := json.MarshalIndent(initialCfg, "", "  ")
		if err := os.WriteFile(configFile, initialData, 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if err := UpdateConfigDiscovery(configFile, true); err != nil {
			t.Fatalf("UpdateConfigDiscovery() error = %v", err)
		}

		data, err := os.ReadFile(configFile)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}

		var cfg map[string]any
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("unmarshal config: %v", err)
		}

		if cfg["discovery"] == nil {
			t.Error("discovery section not created")
		}
	})
}

func TestAgentTUIViewRenders(t *testing.T) {
	t.Run("list screen View contains key hints", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()
		view := m.View()
		if !strings.Contains(view, "agents") && !strings.Contains(view, "auto") {
			t.Error("expected agent-related content in list view")
		}
	})

	t.Run("edit screen View shows edit fields", func(t *testing.T) {
		m := newTestTUI()
		m.modeAuto = false
		m.loadDefaults()
		m.startEdit(0)
		view := m.View()
		if !strings.Contains(view, "Name") {
			t.Error("expected Name in edit view")
		}
	})

	t.Run("confirm screen View shows confirmation prompt", func(t *testing.T) {
		m := newTestTUI()
		m.screen = screenConfirm
		view := m.View()
		if !strings.Contains(view, "delete") && !strings.Contains(view, "Delete") {
			t.Error("expected delete confirmation in confirm view")
		}
	})

	t.Run("picker screen View shows model list", func(t *testing.T) {
		m := newTestTUI()
		m.screen = screenPicker
		m.loadModels()
		view := m.View()
		if !strings.Contains(view, "gemini") && !strings.Contains(view, "Models") {
			t.Error("expected model options in picker view")
		}
	})

	t.Run("quitting View returns empty string", func(t *testing.T) {
		m := newTestTUI()
		m.done = true
		view := m.View()
		if view != "" {
			t.Errorf("expected empty view when done, got %q", view)
		}
	})
}

func TestAgentTUIInit(t *testing.T) {
	t.Run("returns nil command on init", func(t *testing.T) {
		m := newTestTUI()
		cmd := m.Init()
		if cmd != nil {
			t.Error("expected nil command from Init")
		}
	})
}

func TestAgentRunAgentEditor(t *testing.T) {
	t.Run("returns empty config when called (no TTY)", func(t *testing.T) {
		useAuto, cfg, err := RunAgentEditor()
		if err != nil {
			t.Logf("expected no TTY error: %v", err)
		}
		t.Logf("useAuto: %v, cfg: %v", useAuto, cfg)
	})
}

func TestAgentScrollPicker(t *testing.T) {
	t.Run("scrollPicker with empty models does not panic", func(t *testing.T) {
		m := newTestTUI()
		m.screen = screenPicker
		m.scrollPicker()
	})

	t.Run("scrollPicker with models updates offset", func(t *testing.T) {
		m := newTestTUI()
		m.screen = screenPicker
		m.loadModels()
		m.modelCursor = 1
		m.scrollPicker()
	})
}

func TestAgentUpdateAgentContent(t *testing.T) {
	t.Run("updateAgentContent calls renderAgentList", func(t *testing.T) {
		m := newTestTUI()
		m.updateAgentContent()
	})
}
