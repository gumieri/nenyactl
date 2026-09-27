package config

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gumieri/nenyactl/internal/jsonc"
)

func setupModel(t *testing.T, configJSON string) configModel {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := jsonc.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := newConfigModel(cfg, []byte(configJSON), path)
	return m
}

func update(m *configModel, msg tea.Msg) *configModel {
	result, _ := m.Update(msg)
	return result.(*configModel)
}

func TestEditorTUI_Init(t *testing.T) {
	t.Run("Init returns nil", func(t *testing.T) {
		m := setupModel(t, `{"server": {"listen_addr": ":8080"}}`)
		cmd := m.Init()
		if cmd != nil {
			t.Error("Init() should return nil")
		}
	})
}

func TestEditorTUI_Quit(t *testing.T) {
	cfg := `{"server": {"listen_addr": ":8080"}, "discovery": {"enabled": true}}`

	t.Run("ctrl+c quits", func(t *testing.T) {
		m := setupModel(t, cfg)
		updated := update(&m, tea.KeyMsg{Type: tea.KeyCtrlC})
		if !updated.quit {
			t.Error("expected quit flag to be set")
		}
	})

	t.Run("q quits", func(t *testing.T) {
		m := setupModel(t, cfg)
		updated := update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		if !updated.quit {
			t.Error("expected quit flag to be set")
		}
	})

	t.Run("esc quits", func(t *testing.T) {
		m := setupModel(t, cfg)
		updated := update(&m, tea.KeyMsg{Type: tea.KeyEsc})
		if !updated.quit {
			t.Error("expected quit flag to be set")
		}
	})
}

func TestEditorTUI_CursorMovement(t *testing.T) {
	cfg := `{"server": {"listen_addr": ":8080"}, "discovery": {"enabled": true}, "governance": {"ratelimit_max_tpm": 250000}}`

	t.Run("down key moves cursor down", func(t *testing.T) {
		m := setupModel(t, cfg)
		updated := update(&m, tea.KeyMsg{Type: tea.KeyDown})
		if updated.cursor != 1 {
			t.Errorf("cursor = %d, want 1", updated.cursor)
		}
	})

	t.Run("j key moves cursor down", func(t *testing.T) {
		m := setupModel(t, cfg)
		updated := update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if updated.cursor != 1 {
			t.Errorf("cursor = %d, want 1", updated.cursor)
		}
	})

	t.Run("up key moves cursor up", func(t *testing.T) {
		m := setupModel(t, cfg)
		m.cursor = 1
		updated := update(&m, tea.KeyMsg{Type: tea.KeyUp})
		if updated.cursor != 0 {
			t.Errorf("cursor = %d, want 0", updated.cursor)
		}
	})

	t.Run("k key moves cursor up", func(t *testing.T) {
		m := setupModel(t, cfg)
		m.cursor = 1
		updated := update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		if updated.cursor != 0 {
			t.Errorf("cursor = %d, want 0", updated.cursor)
		}
	})
}

func TestEditorTUI_EnterSection(t *testing.T) {
	cfg := `{"server": {"listen_addr": ":8080"}, "discovery": {"enabled": true}}`

	t.Run("enter on object section switches to keys screen", func(t *testing.T) {
		m := setupModel(t, cfg)
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.screen != screenKeys {
			t.Errorf("screen = %d, want %d", m.screen, screenKeys)
		}
		if len(m.entries) == 0 {
			t.Error("expected entries to be loaded")
		}
	})

	t.Run("enter on agents section switches to agents screen", func(t *testing.T) {
		m := setupModel(t, cfg)
		for i := 0; i < len(m.sections)-1; i++ {
			update(&m, tea.KeyMsg{Type: tea.KeyDown})
		}
		update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.screen != screenAgents {
			t.Errorf("screen = %d, want %d", m.screen, screenAgents)
		}
	})
}

func TestEditorTUI_Save(t *testing.T) {
	cfg := `{"server": {"listen_addr": ":8080"}}`

	t.Run("s key saves and quits", func(t *testing.T) {
		m := setupModel(t, cfg)
		updated := update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
		if !updated.saved {
			t.Error("expected saved flag to be set")
		}
	})
}

func TestEditorTUI_KeysScreen(t *testing.T) {
	setup := func(t *testing.T, configJSON string) *configModel {
		m := setupModel(t, configJSON)
		m.loadSection("server")
		m.screen = screenKeys
		return &m
	}
	cfg := `{"server": {"listen_addr": ":8080", "port": "443"}}`

	t.Run("up key moves cursor up", func(t *testing.T) {
		m := setup(t, cfg)
		m.cursor = 1
		update(m, tea.KeyMsg{Type: tea.KeyUp})
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m.cursor)
		}
	})

	t.Run("down key moves cursor down", func(t *testing.T) {
		m := setup(t, cfg)
		m.cursor = 0
		update(m, tea.KeyMsg{Type: tea.KeyDown})
		if m.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m.cursor)
		}
	})

	t.Run("esc returns to sections screen", func(t *testing.T) {
		m := setup(t, cfg)
		update(m, tea.KeyMsg{Type: tea.KeyEsc})
		if m.screen != screenSections {
			t.Errorf("screen = %d, want %d", m.screen, screenSections)
		}
	})

	t.Run("enter on a key opens edit screen", func(t *testing.T) {
		m := setup(t, cfg)
		update(m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.screen != screenEdit {
			t.Errorf("screen = %d, want %d", m.screen, screenEdit)
		}
	})
}

func TestEditorTUI_EditScreen(t *testing.T) {
	setup := func(t *testing.T) *configModel {
		m := setupModel(t, `{"server": {"listen_addr": ":8080"}}`)
		m.loadSection("server")
		m.screen = screenEdit
		m.startEdit(0)
		return &m
	}

	t.Run("enter applies edit and returns to keys", func(t *testing.T) {
		m := setup(t)
		m.editInput.SetValue("newvalue")
		updated := update(m, tea.KeyMsg{Type: tea.KeyEnter})
		if updated.screen != screenKeys {
			t.Errorf("screen = %d, want %d", updated.screen, screenKeys)
		}
	})

	t.Run("esc cancels edit and returns to keys", func(t *testing.T) {
		m := setup(t)
		updated := update(m, tea.KeyMsg{Type: tea.KeyEsc})
		if updated.screen != screenKeys {
			t.Errorf("screen = %d, want %d", updated.screen, screenKeys)
		}
	})
}

func TestEditorTUI_WindowSize(t *testing.T) {
	t.Run("updates viewports and dimensions", func(t *testing.T) {
		m := setupModel(t, `{"server": {"listen_addr": ":8080"}}`)
		msg := tea.WindowSizeMsg{Width: 120, Height: 40}
		update(&m, msg)

		if m.width != 120 {
			t.Errorf("width = %d, want 120", m.width)
		}
		if m.height != 40 {
			t.Errorf("height = %d, want 40", m.height)
		}
	})
}

func TestEditorTUI_AgentsScreenNavigation(t *testing.T) {
	t.Run("esc returns to sections", func(t *testing.T) {
		m := setupModel(t, `{"server": {"listen_addr": ":8080"}}`)
		m.screen = screenAgents
		update(&m, tea.KeyMsg{Type: tea.KeyEsc})
		if m.screen != screenSections {
			t.Errorf("screen = %d, want %d", m.screen, screenSections)
		}
	})
}
