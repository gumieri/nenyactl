package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gumieri/nenyactl/internal/jsonc"
	"github.com/tailscale/hujson"
)

const testConfig = `{
  // Server
  "server": {
    "listen_addr": ":8080"
  },
  // Discovery
  "discovery": {
    "enabled": true,
    "auto_agents": true
  },
  "governance": {
    "ratelimit_max_tpm": 250000
  }
}
`

func TestAgentsValueRejectsBadNames(t *testing.T) {
	if _, err := agentsValue([]agentEntry{{Name: "", Strategy: "fallback"}}); err == nil {
		t.Error("expected an error for an empty agent name")
	}
	if _, err := agentsValue([]agentEntry{
		{Name: "dup", Strategy: "fallback"},
		{Name: "dup", Strategy: "fallback"},
	}); err == nil {
		t.Error("expected an error for a duplicate agent name")
	}

	// A valid set round-trips, and a nil model list becomes [].
	out, err := agentsValue([]agentEntry{{Name: "build", Strategy: "fallback"}})
	if err != nil {
		t.Fatalf("agentsValue: %v", err)
	}
	var got map[string]struct {
		Strategy string   `json:"strategy"`
		Models   []string `json:"models"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("agentsValue output is not JSON: %v", err)
	}
	if got["build"].Models == nil {
		t.Error("models should marshal as [] not null")
	}
}

func TestParseLiteralValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"true", "true"},
		{"false", "false"},
		{"null", "null"},
		{`"hello"`, `"hello"`},
		{"42", "42"},
		{"3.14", "3.14"},
		{"", `""`},
		{"unquoted", `"unquoted"`},
	}
	for _, tt := range tests {
		got := string(parseLiteralValue(tt.input))
		if got != tt.want {
			t.Errorf("parseLiteralValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNewConfigModel(t *testing.T) {
	cfg, err := jsonc.ParseDoc([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}

	m := newConfigModel(cfg, []byte(testConfig))
	if len(m.sections) == 0 {
		t.Error("expected non-empty sections")
	}
	if !m.sections[len(m.sections)-1].isAgent {
		t.Error("expected the final section to be the agents section")
	}
}

func TestNewConfigModelDoesNotDuplicateAgents(t *testing.T) {
	// The effective config from describe already contains "agents" once agents
	// are configured; the editor must not add a second synthetic row.
	effective := `{"server":{"listen_addr":":8080"},"agents":{"build":{"strategy":"fallback","models":["m"]}}}`
	cfg, err := jsonc.ParseDoc([]byte(effective))
	if err != nil {
		t.Fatal(err)
	}

	m := newConfigModel(cfg, []byte(effective))

	count := 0
	for _, s := range m.sections {
		if s.name == "agents" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("agents sections = %d, want 1: %+v", count, m.sections)
	}
}

// updateCfg drives the model Update the way bubbletea does, returning the
// concrete type.
func updateCfg(m *configModel, msg tea.Msg) *configModel {
	result, _ := m.Update(msg)
	return result.(*configModel)
}

func TestConfigEditorValueInput(t *testing.T) {
	cfg, err := jsonc.ParseDoc([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	m := newConfigModel(cfg, []byte(testConfig))
	m.loadSection("governance")
	m.startEdit(0)
	m.screen = screenEdit

	// Typed characters must reach the value input (they did not before, because
	// updateEdit returned before the outer Update forwarded the key).
	updated := updateCfg(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("7")})
	if !strings.HasSuffix(updated.editInput.Value(), "7") {
		t.Errorf("value = %q, want it to end with the typed rune", updated.editInput.Value())
	}
}

func TestApplyEditTopLevelScalarKey(t *testing.T) {
	// A top-level scalar section keys its single entry by the section name, so
	// the recorded dotted key must not become "foo.foo".
	scalar := `{"log_level":"info"}`
	cfg, err := jsonc.ParseDoc([]byte(scalar))
	if err != nil {
		t.Fatal(err)
	}

	m := newConfigModel(cfg, []byte(scalar))
	m.loadSection("log_level")
	if len(m.entries) != 1 || m.entries[0].Key != "log_level" {
		t.Fatalf("entries = %+v, want one log_level entry", m.entries)
	}

	m.cursor = 0
	m.editInput.SetValue("debug")
	m.applyEdit()

	if _, ok := m.changes["log_level.log_level"]; ok {
		t.Errorf("recorded a doubled key: %v", m.changes)
	}
	if got := m.changes["log_level"]; got != `"debug"` {
		t.Errorf("recorded change = %q, want %q (changes: %v)", got, `"debug"`, m.changes)
	}
}

func TestLoadSection(t *testing.T) {
	cfg, err := jsonc.ParseDoc([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}

	m := newConfigModel(cfg, []byte(testConfig))
	if len(m.sections) == 0 {
		t.Fatal("no sections")
	}

	m.loadSection("server")
	if len(m.entries) == 0 {
		t.Error("expected non-empty entries for the server section")
	}
}

func TestApplyEdit(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := jsonc.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	m := newConfigModel(cfg, []byte(testConfig))
	m.loadSection("governance")
	if len(m.entries) == 0 {
		t.Fatal("expected entries")
	}

	m.cursor = 0
	m.editInput.SetValue("500000")
	m.applyEdit()

	field, ok := jsonc.GetNestedField(m.config, []string{"governance", "ratelimit_max_tpm"})
	if !ok {
		t.Fatal("expected nested field")
	}
	got := jsonc.FieldValueString(field)
	if got != "500000" {
		t.Errorf("got %q, want 500000", got)
	}
	if change := m.changes["governance.ratelimit_max_tpm"]; change != "500000" {
		t.Errorf("recorded change = %q, want 500000", change)
	}
}

func TestApplyEditPreservesComments(t *testing.T) {
	cfg, err := jsonc.ParseDoc([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}

	m := newConfigModel(cfg, []byte(testConfig))
	m.loadSection("governance")
	m.cursor = 0
	m.editInput.SetValue("500000")
	m.applyEdit()

	packed := string(m.config.Pack())
	if len(packed) == 0 {
		t.Fatal("packed should not be empty")
	}
	if !strings.Contains(packed, "// Server") {
		t.Errorf("comments were not preserved:\n%s", packed)
	}
}

func TestAgentsModeToggleRecorded(t *testing.T) {
	cfg, err := jsonc.ParseDoc([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}

	m := newConfigModel(cfg, []byte(testConfig))
	m.screen = screenAgents

	// Toggling the mode is not an agents-list edit: it must not mark the
	// agents dirty or emit an agents rewrite.
	update(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if m.agentsDirty {
		t.Error("a mode-only toggle must not mark the agents dirty")
	}

	result, err := m.result()
	if err != nil || result == nil {
		t.Fatalf("result: %v", err)
	}
	want := strconv.FormatBool(m.agentsModeAuto)
	if got := result.Changes["discovery.auto_agents"]; got != want {
		t.Errorf("auto_agents change = %q, want %q", got, want)
	}
	if _, ok := result.Changes["agents"]; ok {
		t.Error("a mode-only toggle must not emit an agents change")
	}
}

func TestResultIncludesDirtyAgents(t *testing.T) {
	cfg, err := jsonc.ParseDoc([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}

	m := newConfigModel(cfg, []byte(testConfig))
	m.agents = []agentEntry{{Name: "build", Strategy: "fallback", Models: []string{"m"}}}
	m.agentsDirty = true

	result, err := m.result()
	if err != nil || result == nil {
		t.Fatalf("result: %v", err)
	}
	if result.Changes["agents"] == "" {
		t.Error("expected an agents change")
	}
}

func TestIsSectionObject(t *testing.T) {
	v, err := jsonc.ParseDoc([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range jsonc.TopLevelKeys(v) {
		field, ok := jsonc.GetField(v, key)
		if !ok {
			continue
		}
		isObj := isSectionObject(field)
		if _, ok := field.Value.(*hujson.Object); ok && !isObj {
			t.Errorf("isSectionObject(%s) = false, want true", key)
		}
	}
}

func TestAgentsFromEffective(t *testing.T) {
	effective := []byte(`{
	  "discovery": {"auto_agents": false},
	  "agents": {
	    "build": {"strategy": "fallback", "models": ["m1", "m2"]},
	    "review": {"strategy": "round_robin"}
	  }
	}`)

	agents, auto := agentsFromEffective(effective)
	if auto {
		t.Error("auto_agents should be false")
	}
	if len(agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(agents))
	}
	if agents[0].Name != "build" || agents[1].Name != "review" {
		t.Errorf("agents not sorted by name: %v", agents)
	}
	if agents[0].Strategy != "fallback" || len(agents[0].Models) != 2 {
		t.Errorf("build agent parsed wrong: %+v", agents[0])
	}
	if agents[1].Strategy != "round_robin" || len(agents[1].Models) != 0 {
		t.Errorf("review agent parsed wrong: %+v", agents[1])
	}
}

func TestAgentsFromEffectiveAutoFlag(t *testing.T) {
	agents, auto := agentsFromEffective([]byte(`{"discovery":{"auto_agents":true}}`))
	if !auto {
		t.Error("auto_agents should be true")
	}
	if len(agents) != 0 {
		t.Errorf("expected no agents, got %v", agents)
	}
}

func TestAgentsFromEffectiveEdgeCases(t *testing.T) {
	t.Run("invalid JSON", func(t *testing.T) {
		agents, auto := agentsFromEffective([]byte(`not json`))
		if agents != nil || auto {
			t.Errorf("got (%v, %v), want (nil, false)", agents, auto)
		}
	})

	t.Run("null document", func(t *testing.T) {
		agents, auto := agentsFromEffective([]byte(`null`))
		if agents != nil || auto {
			t.Errorf("got (%v, %v), want (nil, false)", agents, auto)
		}
	})

	t.Run("discovery without auto_agents", func(t *testing.T) {
		_, auto := agentsFromEffective([]byte(`{"discovery":{"enabled":true}}`))
		if auto {
			t.Error("auto_agents should default to false when absent")
		}
	})

	t.Run("models null", func(t *testing.T) {
		agents, _ := agentsFromEffective([]byte(`{"agents":{"a":{"strategy":"fallback","models":null}}}`))
		if len(agents) != 1 || len(agents[0].Models) != 0 {
			t.Errorf("agents = %+v", agents)
		}
	})

	t.Run("agents not an object", func(t *testing.T) {
		agents, _ := agentsFromEffective([]byte(`{"agents":[]}`))
		if agents != nil {
			t.Errorf("agents = %v, want nil", agents)
		}
	})
}
