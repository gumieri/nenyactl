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

func TestGetObject(t *testing.T) {
	t.Run("extracts object value", func(t *testing.T) {
		v := hujson.Value{Value: &hujson.Object{}}
		obj, ok := GetObject(&v)
		if !ok {
			t.Fatal("expected true for object")
		}
		if obj == nil {
			t.Fatal("expected non-nil object")
		}
	})

	t.Run("returns false for non-object", func(t *testing.T) {
		v := hujson.Value{Value: hujson.Literal("string")}
		_, ok := GetObject(&v)
		if ok {
			t.Error("expected false for non-object")
		}
	})
}

func TestEnsureObject(t *testing.T) {
	v, err := hujson.Parse([]byte(`{"provider": {"existing": true}}`))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("returns an existing object", func(t *testing.T) {
		obj, ok := EnsureObject(&v, "provider")
		if !ok {
			t.Fatal("expected true for object member")
		}
		if obj == nil {
			t.Fatal("expected non-nil object")
		}
	})

	t.Run("creates a missing object", func(t *testing.T) {
		obj, ok := EnsureObject(&v, "created")
		if !ok {
			t.Fatal("expected true when creating")
		}
		if obj == nil {
			t.Fatal("expected non-nil object")
		}
	})

	t.Run("returns false for a non-object member", func(t *testing.T) {
		s, _ := hujson.Parse([]byte(`{"scalar": 1}`))
		if _, ok := EnsureObject(&s, "scalar"); ok {
			t.Error("expected false for scalar member")
		}
	})
}
