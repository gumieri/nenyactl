package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gumieri/nenyactl/internal/jsonc"
)

func TestLoadEffectiveConfig_MultiFileMerge(t *testing.T) {
	tmp := t.TempDir()
	configD := filepath.Join(tmp, "config.d")
	configFile := filepath.Join(tmp, "config.json")

	// Create test config.d directory
	if err := os.MkdirAll(configD, 0755); err != nil {
		t.Fatal(err)
	}

	// Write 00-first.json with base config
	firstFile := filepath.Join(configD, "00-first.json")
	if err := os.WriteFile(firstFile, []byte(`{
  "key1": "from-first",
  "key2": "from-first",
  "nested": {
    "sub1": "from-first"
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Write 20-second.json with overrides (should win for key2)
	secondFile := filepath.Join(configD, "20-second.json")
	if err := os.WriteFile(secondFile, []byte(`{
  "key2": "from-second",
  "key3": "from-second"
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Write config.json (should win for everything)
	if err := os.WriteFile(configFile, []byte(`{
  "key1": "from-config",
  "key4": "from-config"
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Load and test
	merged, sources, err := LoadEffectiveConfig(configFile, configD)
	if err != nil {
		t.Fatal(err)
	}

	// Verify expectations
	tests := []struct {
		key     string
		wantVal string
		wantSrc string
	}{
		{"key1", `"from-config"`, configFile}, // config.json wins
		{"key2", `"from-second"`, secondFile}, // Last config.d wins
		{"key3", `"from-second"`, secondFile},
		{"key4", `"from-config"`, configFile},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			field, ok := jsonc.GetField(merged, tt.key)
			if !ok {
				t.Fatalf("key %s not found in merged config", tt.key)
			}
			val := jsonc.FieldValueString(field)
			src := sources[tt.key]

			if val != tt.wantVal {
				t.Errorf("value = %q, want %q", val, tt.wantVal)
			}
			if src.filePath != tt.wantSrc {
				t.Errorf("source = %q, want %q", src.filePath, tt.wantSrc)
			}
		})
	}
}

func TestLoadEffectiveConfig_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	configD := filepath.Join(tmp, "config.d")
	configFile := filepath.Join(tmp, "config.json")

	if err := os.MkdirAll(configD, 0755); err != nil {
		t.Fatal(err)
	}

	// Invalid JSON
	invalidFile := filepath.Join(configD, "00-invalid.json")
	if err := os.WriteFile(invalidFile, []byte(`{invalid json}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadEffectiveConfig(configFile, configD)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestLoadEffectiveConfig_EmptyConfigD(t *testing.T) {
	tmp := t.TempDir()
	configD := filepath.Join(tmp, "config.d")
	configFile := filepath.Join(tmp, "config.json")

	if err := os.MkdirAll(configD, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(configFile, []byte(`{"key": "value"}`), 0644); err != nil {
		t.Fatal(err)
	}

	merged, sources, err := LoadEffectiveConfig(configFile, configD)
	if err != nil {
		t.Fatal(err)
	}

	field, ok := jsonc.GetField(merged, "key")
	if !ok {
		t.Fatal("key not found")
	}
	if val := jsonc.FieldValueString(field); val != `"value"` {
		t.Errorf("got %q, want \"value\"", val)
	}

	src, ok := sources["key"]
	if !ok {
		t.Fatal("source not found for key")
	}
	if src.filePath != configFile {
		t.Errorf("source = %q, want %q", src.filePath, configFile)
	}
}

func TestLoadEffectiveConfig_NonObjectFile(t *testing.T) {
	tmp := t.TempDir()
	configD := filepath.Join(tmp, "config.d")
	configFile := filepath.Join(tmp, "config.json")

	if err := os.MkdirAll(configD, 0755); err != nil {
		t.Fatal(err)
	}

	// Valid JSON but not an object
	nonObjFile := filepath.Join(configD, "00-array.json")
	if err := os.WriteFile(nonObjFile, []byte(`["not", "an", "object"]`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(configFile, []byte(`{"key": "value"}`), 0644); err != nil {
		t.Fatal(err)
	}

	merged, _, err := LoadEffectiveConfig(configFile, configD)
	if err != nil {
		t.Fatal(err)
	}

	// Should only have config.json's key
	field, ok := jsonc.GetField(merged, "key")
	if !ok {
		t.Fatal("key not found")
	}
	if val := jsonc.FieldValueString(field); val != `"value"` {
		t.Errorf("got %q, want \"value\"", val)
	}
}

func TestLoadEffectiveConfig_ParsingOrder(t *testing.T) {
	tmp := t.TempDir()
	configD := filepath.Join(tmp, "config.d")
	configFile := filepath.Join(tmp, "config.json")

	if err := os.MkdirAll(configD, 0755); err != nil {
		t.Fatal(err)
	}

	// Create files out of order
	files := map[string]string{
		"30-zzz.json": `{"key": "zzz"}`,
		"10-aaa.json": `{"key": "aaa"}`,
		"20-mmm.json": `{"key": "mmm"}`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(configD, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(configFile, []byte(`{"other": "value"}`), 0644); err != nil {
		t.Fatal(err)
	}

	merged, sources, err := LoadEffectiveConfig(configFile, configD)
	if err != nil {
		t.Fatal(err)
	}

	// Last file (30-zzz.json) should win for key
	field, ok := jsonc.GetField(merged, "key")
	if !ok {
		t.Fatal("key not found")
	}
	if val := jsonc.FieldValueString(field); val != `"zzz"` {
		t.Errorf("got %q, want \"zzz\" (last file should win)", val)
	}

	// Verify source is 30-zzz.json
	src := sources["key"]
	if !strings.HasSuffix(src.filePath, "30-zzz.json") {
		t.Errorf("source = %q, want 30-zzz.json", src.filePath)
	}
}
