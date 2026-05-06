package jsonc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tailscale/hujson"
)

const exampleJSONC = `{
  // Server config
  "server": {
    "listen_addr": ":8080"
  },
  "discovery": {
    "enabled": true,
    "auto_agents": true
  },
  // Trailing comma here
  "governance": {
    "ratelimit_max_tpm": 250000,
  },
}
`

func TestReadFile(t *testing.T) {
	t.Run("parses JSONC with comments", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.json")
		if err := os.WriteFile(path, []byte(exampleJSONC), 0o644); err != nil {
			t.Fatal(err)
		}

		v, err := ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}

		keys := TopLevelKeys(v)
		if len(keys) != 3 {
			t.Fatalf("expected 3 keys, got %d: %v", len(keys), keys)
		}
		if keys[0] != "server" || keys[1] != "discovery" || keys[2] != "governance" {
			t.Errorf("keys = %v, want [server discovery governance]", keys)
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := ReadFile("/nonexistent/config.json")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}

func TestWriteFile(t *testing.T) {
	t.Run("writes and preserves comments", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.json")

		v, err := hujson.Parse([]byte(exampleJSONC))
		if err != nil {
			t.Fatal(err)
		}

		if err := WriteFile(path, &v, 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if len(data) == 0 {
			t.Error("file should not be empty")
		}

		v2, err := ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		keys := TopLevelKeys(v2)
		if len(keys) != 3 {
			t.Errorf("expected 3 keys after round-trip, got %d", len(keys))
		}
	})
}

func TestGetField(t *testing.T) {
	v, err := hujson.Parse([]byte(exampleJSONC))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("existing top-level field", func(t *testing.T) {
		field, ok := GetField(&v, "server")
		if !ok {
			t.Fatal("expected server field")
		}
		str := FieldValueString(field)
		if str == "" {
			t.Error("server field should not be empty")
		}
	})

	t.Run("missing field", func(t *testing.T) {
		_, ok := GetField(&v, "nonexistent")
		if ok {
			t.Error("expected false for missing field")
		}
	})
}

func TestSetField(t *testing.T) {
	v, err := hujson.Parse([]byte(`{"key": "old"}`))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("updates existing field", func(t *testing.T) {
		SetField(&v, "key", hujson.Literal(`"new"`))
		field, ok := GetField(&v, "key")
		if !ok {
			t.Fatal("expected key field")
		}
		if FieldValueString(field) != `"new"` {
			t.Errorf("got %q, want %q", FieldValueString(field), `"new"`)
		}
	})

	t.Run("adds new field", func(t *testing.T) {
		SetField(&v, "new_key", hujson.Literal(`"value"`))
		field, ok := GetField(&v, "new_key")
		if !ok {
			t.Fatal("expected new_key field")
		}
		if FieldValueString(field) != `"value"` {
			t.Errorf("got %q, want %q", FieldValueString(field), `"value"`)
		}
	})
}

func TestSetNestedField(t *testing.T) {
	v, err := hujson.Parse([]byte(`{"discovery": {"auto_agents": true}}`))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("updates nested field", func(t *testing.T) {
		ok := SetNestedField(&v, []string{"discovery", "auto_agents"}, hujson.Literal("false"))
		if !ok {
			t.Fatal("SetNestedField failed")
		}
		field, ok := GetNestedField(&v, []string{"discovery", "auto_agents"})
		if !ok {
			t.Fatal("expected nested field")
		}
		if FieldValueString(field) != "false" {
			t.Errorf("got %q, want false", FieldValueString(field))
		}
	})

	t.Run("returns false for empty path", func(t *testing.T) {
		ok := SetNestedField(&v, nil, hujson.Literal("true"))
		if ok {
			t.Error("expected false for empty path")
		}
	})
}

func TestGetNestedField(t *testing.T) {
	v, err := hujson.Parse([]byte(exampleJSONC))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("gets nested value", func(t *testing.T) {
		field, ok := GetNestedField(&v, []string{"discovery", "auto_agents"})
		if !ok {
			t.Fatal("expected discovery.auto_agents")
		}
		if FieldValueString(field) != "true" {
			t.Errorf("got %q, want true", FieldValueString(field))
		}
	})

	t.Run("returns false for missing path", func(t *testing.T) {
		_, ok := GetNestedField(&v, []string{"nonexistent", "field"})
		if ok {
			t.Error("expected false for missing path")
		}
	})
}

func TestTopLevelKeys(t *testing.T) {
	v, err := hujson.Parse([]byte(exampleJSONC))
	if err != nil {
		t.Fatal(err)
	}

	keys := TopLevelKeys(&v)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "server" {
		t.Errorf("keys[0] = %q, want server", keys[0])
	}
	if keys[1] != "discovery" {
		t.Errorf("keys[1] = %q, want discovery", keys[1])
	}
}

func TestFieldValueString(t *testing.T) {
	t.Run("string literal", func(t *testing.T) {
		v := hujson.Value{Value: hujson.Literal(`"hello" `)}
		if FieldValueString(&v) != `"hello"` {
			t.Errorf("got %q", FieldValueString(&v))
		}
	})

	t.Run("boolean literal", func(t *testing.T) {
		v := hujson.Value{Value: hujson.Literal("true")}
		if FieldValueString(&v) != "true" {
			t.Errorf("got %q", FieldValueString(&v))
		}
	})

	t.Run("number literal", func(t *testing.T) {
		v := hujson.Value{Value: hujson.Literal("42")}
		if FieldValueString(&v) != "42" {
			t.Errorf("got %q", FieldValueString(&v))
		}
	})

	t.Run("object value", func(t *testing.T) {
		v := hujson.Value{Value: &hujson.Object{}}
		s := FieldValueString(&v)
		if s != "{}" {
			t.Errorf("got %q", s)
		}
	})
}
