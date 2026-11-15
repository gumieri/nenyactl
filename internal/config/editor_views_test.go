package config

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gumieri/nenyactl/internal/tui"
)

// Tests for the config editor's view rendering, scroll helpers, help-key map,
// and navigation paths not covered elsewhere (NCTL-50).

func seedViews(t *testing.T, cfg string) *configModel {
	t.Helper()
	m := setupModel(t, cfg)
	m.width, m.height = 80, 24
	m.sectionsView.Height = 10
	m.keysView.Height = 10
	m.agentsView.Height = 10
	m.providersView.Height = 10
	return &m
}

const viewsCfg = `{
	"server": {"listen_addr": ":8080"},
	"providers": {"openai": {"api_key": "[REDACTED]"}},
	"agents": {"mode": "manual"}
}`

func TestViewsRender(t *testing.T) {
	t.Run("viewSections lists the config sections", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		m.updateSectionsContent()
		view := m.viewSections()
		if !strings.Contains(view, "server") {
			t.Errorf("viewSections = %q, want the server section", view)
		}
	})

	t.Run("viewKeys lists the section entries", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		m.loadSection("server")
		m.updateKeysContent()
		view := m.viewKeys()
		if !strings.Contains(view, "listen_addr") {
			t.Errorf("viewKeys = %q, want the listen_addr entry", view)
		}
	})

	t.Run("viewEdit shows the key being edited", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		m.loadSection("server")
		m.screen = screenKeys // startEdit records this as the return screen
		m.startEdit(0)        // production sequence: keys → startEdit → edit
		m.screen = screenEdit // updateKeys sets this after startEdit
		m.editInput.SetValue(":9999")
		view := m.viewEdit()
		if !strings.Contains(view, "listen_addr") {
			t.Errorf("viewEdit = %q, want the key name", view)
		}
	})

	t.Run("viewConfirmDelete names a pending agent delete", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		m.screen = screenAgents
		m.agents = []agentEntry{{}}
		m.confirmKind = confirmDeleteAgent
		m.confirmIndex = 0
		m.updateAgentsContent()
		view := m.viewConfirmDelete(tui.Current())
		if !strings.Contains(view, "Delete") {
			t.Errorf("viewConfirmDelete = %q, want a delete prompt", view)
		}
	})

	t.Run("viewConfirmDelete names a pending discard-all", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		m.confirmKind = confirmDiscardAll
		view := m.viewConfirmDelete(tui.Current())
		if !strings.Contains(view, "Discard") {
			t.Errorf("viewConfirmDelete = %q, want a discard prompt", view)
		}
	})

	t.Run("viewProviderEdit shows the provider name", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		m.screen = screenProviderEdit
		m.editingProvider = "openai"
		view := m.viewProviderEdit(tui.Current())
		if !strings.Contains(view, "openai") {
			t.Errorf("viewProviderEdit = %q, want the provider name", view)
		}
	})

	t.Run("viewAgents lists agents", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		m.screen = screenAgents
		m.agents = []agentEntry{{Name: "alpha", Strategy: "fallback"}}
		m.updateAgentsContent()
		view := m.viewAgents()
		if !strings.Contains(view, "alpha") {
			t.Errorf("viewAgents = %q, want the agent name", view)
		}
	})

	t.Run("View dispatches per screen and renders empty when done", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		for _, s := range []configScreen{screenSections, screenKeys, screenEdit, screenAgents, screenProviders, screenProviderEdit, screenReview, screenConfirmDelete} {
			m.screen = s
			if m.View() == "" {
				t.Errorf("View() for screen %v = empty", s)
			}
		}
		m.saved = true
		if m.View() != "" {
			t.Error("saved model must render empty")
		}
	})
}

func TestHelpKeyMapPerScreen(t *testing.T) {
	m := setupModel(t, viewsCfg)
	for _, s := range []configScreen{screenSections, screenKeys, screenEdit, screenAgents, screenProviders, screenProviderEdit, screenConfirmDelete, screenReview} {
		m.screen = s
		km := m.helpKeyMap()
		if km.ShortHelp() == nil {
			t.Errorf("helpKeyMap(%v) returned no bindings", s)
		}
	}
	// default: branch (out-of-range screen value) falls back to the sections
	// map — assert a known binding rather than a vacuous non-nil check.
	m.screen = configScreen(99)
	if m.helpKeyMap().Select.Help().Key != tui.SectionsKeyMap.Select.Help().Key {
		t.Error("default key map should fall back to the sections map")
	}
}

func TestScrollHelpers(t *testing.T) {
	cfg := `{"server": {"a": "1", "b": "2", "c": "3", "d": "4", "e": "5"}}`

	t.Run("scrollSections clamps the offset", func(t *testing.T) {
		m := seedViews(t, cfg)
		m.cursor = 99
		m.scrollSections()
		// One row per section (<= 6 rows) fits height 10: the clamp must
		// land the window at the last full page, offset 0 for this size.
		if m.sectionsView.YOffset != 0 {
			t.Errorf("YOffset = %d, want 0 (all rows fit)", m.sectionsView.YOffset)
		}
	})

	t.Run("scrollKeys clamps the offset", func(t *testing.T) {
		m := seedViews(t, cfg)
		m.loadSection("server")
		m.cursor = 99
		m.scrollKeys()
		// One row per entry (5 entries) fits height 10: offset 0.
		if m.keysView.YOffset != 0 {
			t.Errorf("YOffset = %d, want 0 (all rows fit)", m.keysView.YOffset)
		}
	})

	t.Run("scrollKeys and scrollSections are no-ops at height 0", func(t *testing.T) {
		m := seedViews(t, cfg)
		m.sectionsView.Height = 0
		m.keysView.Height = 0
		m.cursor = 99
		m.scrollSections()
		m.scrollKeys()
		if m.sectionsView.YOffset != 0 || m.keysView.YOffset != 0 {
			t.Error("height-0 scrolls must be no-ops")
		}
	})

	t.Run("scrollProviders clamps the offset", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		m.screen = screenProviders
		m.providerCursor = 50
		m.updateProvidersContent()
		m.scrollProviders()
		// One row per provider (1 row) fits height 10: offset 0.
		if m.providersView.YOffset != 0 {
			t.Errorf("YOffset = %d, want 0 (all rows fit)", m.providersView.YOffset)
		}
	})

	t.Run("scrollProviders is a no-op at height 0", func(t *testing.T) {
		m := seedViews(t, viewsCfg)
		m.providersView.Height = 0
		m.providerCursor = 50
		m.scrollProviders()
		if m.providersView.YOffset != 0 {
			t.Errorf("YOffset = %d, want 0 (no-op)", m.providersView.YOffset)
		}
	})

	t.Run("scrollAgents clamps the offset", func(t *testing.T) {
		m := seedViews(t, cfg)
		m.screen = screenAgents
		m.agents = []agentEntry{
			{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"},
		}
		m.updateAgentsContent()
		m.agentCursor = len(m.agents) - 1 // last agent: window must scroll
		m.scrollAgents()
		if m.agentsView.YOffset < 0 {
			t.Errorf("YOffset = %d, must not be negative", m.agentsView.YOffset)
		}
		// Two physical lines per agent: the cursor's first line must sit in
		// the visible window (regression for the row-math bug).
		firstLine := m.agentCursor * 2
		if firstLine < m.agentsView.YOffset || firstLine >= m.agentsView.YOffset+m.agentsView.Height {
			t.Errorf("cursor line %d outside window [%d, %d)",
				firstLine, m.agentsView.YOffset, m.agentsView.YOffset+m.agentsView.Height)
		}
	})
}

func TestUpdateAgentsNavigation(t *testing.T) {
	m := setupModel(t, viewsCfg)
	m.screen = screenAgents
	m.agents = []agentEntry{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.agentCursor = 0

	mm := update(&m, teaKeyMsgDown())
	if mm.agentCursor != 1 {
		t.Errorf("agentCursor = %d, want 1", mm.agentCursor)
	}
	mm = update(mm, teaKeyMsgUp())
	if mm.agentCursor != 0 {
		t.Errorf("agentCursor = %d, want 0", mm.agentCursor)
	}
	// Clamped at both ends.
	mm = update(mm, teaKeyMsgUp())
	if mm.agentCursor != 0 {
		t.Errorf("agentCursor = %d, want clamped to 0", mm.agentCursor)
	}
}

func TestUpdateProvidersNavigation(t *testing.T) {
	cfg := `{"providers": {"a": {}, "b": {}, "c": {}}}`
	m := providersSetup(t, cfg, nil)
	if len(m.providerRows) != 3 {
		t.Fatalf("providerRows = %d, want 3", len(m.providerRows))
	}
	m = update(m, teaKeyMsgDown())
	if m.providerCursor != 1 {
		t.Errorf("providerCursor = %d, want 1", m.providerCursor)
	}
	m = update(m, teaKeyMsgDown())
	m = update(m, teaKeyMsgDown()) // clamped at the end
	if m.providerCursor != 2 {
		t.Errorf("providerCursor = %d, want clamped to 2", m.providerCursor)
	}
	m = update(m, teaKeyMsgUp())
	if m.providerCursor != 1 {
		t.Errorf("providerCursor = %d, want 1", m.providerCursor)
	}
	m = update(m, teaKeyMsgEsc())
	if m.screen != screenSections {
		t.Errorf("screen = %v, want screenSections after esc", m.screen)
	}
}

func TestStartEditAgent(t *testing.T) {
	m := setupModel(t, viewsCfg)
	m.screen = screenAgents
	m.agents = []agentEntry{{Name: "alpha", Strategy: "fallback"}}
	m.agentCursor = 0
	m.startEditAgent()
	if m.screen != screenEdit {
		t.Errorf("screen = %v, want screenEdit", m.screen)
	}
	if m.editKey != "agent_name" {
		t.Errorf("editKey = %q, want agent_name", m.editKey)
	}
}

func TestUpdateKeysContent(t *testing.T) {
	m := setupModel(t, `{"server": {"listen_addr": ":8080"}}`)
	m.loadSection("server")
	m.keysView.Height = 5
	m.updateKeysContent()
	if !strings.Contains(m.keysView.View(), "listen_addr") {
		t.Errorf("keys content = %q, want the entry", m.keysView.View())
	}
}

func TestRenderChanges(t *testing.T) {
	out := RenderChanges(map[string]string{"z.last": "1", "a.first": "true"})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 || lines[0] != "a.first = true" || lines[1] != "z.last = 1" {
		t.Errorf("RenderChanges = %q, want sorted lines", out)
	}
	if RenderChanges(nil) != "No changes.\n" {
		t.Error("RenderChanges(nil) should report no changes")
	}
}

// teaKeyMsgDown/Up/Esc are local shims (the shared helpers live in other
// files).

func teaKeyMsgDown() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyDown} }

func teaKeyMsgUp() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyUp} }

func teaKeyMsgEsc() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEsc} }
