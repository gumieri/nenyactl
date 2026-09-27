package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAgentsIntoConfig(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configFile, []byte("{\n  // keep me\n  \"server\": {\"listen_addr\": \":8080\"}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := map[string]any{
		"agents": map[string]any{
			"build": map[string]any{"strategy": "fallback", "models": []string{"m"}},
		},
	}
	if err := WriteAgentsIntoConfig(configFile, cfg); err != nil {
		t.Fatalf("WriteAgentsIntoConfig: %v", err)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "keep me") || !strings.Contains(s, "listen_addr") {
		t.Errorf("unrelated keys/comment not preserved:\n%s", s)
	}
	if !strings.Contains(s, "\"agents\"") || !strings.Contains(s, "\"build\"") {
		t.Errorf("agents not written:\n%s", s)
	}
}

func TestWriteAgentsIntoConfigRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentsIntoConfig(configFile, map[string]any{"other": 1}); err == nil {
		t.Fatal("expected error when cfg has no agents key")
	}
}
