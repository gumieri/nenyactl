package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
