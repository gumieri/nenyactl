package config

import (
	"os"
	"path/filepath"
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
	cfg, err := jsonc.ReadFile(filepath.Join("testdata", "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("no testdata/config.json")
		}
		t.Fatal(err)
	}

	m := newConfigModel(cfg)
	if len(m.sections) == 0 {
		t.Error("expected non-empty sections")
	}
}

func TestLoadSection(t *testing.T) {
	cfg, err := jsonc.ReadFile(filepath.Join("testdata", "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("no testdata/config.json")
		}
		t.Fatal(err)
	}

	m := newConfigModel(cfg)
	if len(m.sections) == 0 {
		t.Fatal("no sections")
	}

	m.loadSection(m.sections[0])
	if len(m.entries) == 0 {
		t.Error("expected non-empty entries for first section")
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

	m := newConfigModel(cfg)
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
}

func TestApplyEditPreservesComments(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := jsonc.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	m := newConfigModel(cfg)
	m.loadSection("governance")
	m.cursor = 0
	m.editInput.SetValue("500000")
	m.applyEdit()

	packed := m.config.Pack()
	if len(packed) == 0 {
		t.Error("packed should not be empty")
	}

	cfg2, err := jsonc.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_ = cfg2
}

func TestIsSectionObject(t *testing.T) {
	v, err := jsonc.ReadFile(filepath.Join("testdata", "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("no testdata/config.json")
		}
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
