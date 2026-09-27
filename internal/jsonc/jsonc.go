package jsonc

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tailscale/hujson"
)

func ReadFile(path string) (*hujson.Value, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	v, err := hujson.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &v, nil
}

// ParseDoc parses JSONC text into a hujson value. Empty input yields an empty
// object so callers can seed a new document.
func ParseDoc(data []byte) (*hujson.Value, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		v, _ := hujson.Parse([]byte("{}"))
		return &v, nil
	}
	v, err := hujson.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse JSONC: %w", err)
	}
	return &v, nil
}

// Render serializes a hujson value back to JSONC bytes. Pack preserves the
// original document's formatting and comments; it does not introduce the
// trailing commas hujson.Format emits for non-standard JSON.
func Render(v *hujson.Value) ([]byte, error) {
	return v.Pack(), nil
}

func memberName(m *hujson.ObjectMember) string {
	return m.Name.Value.(hujson.Literal).String()
}

func GetObject(v *hujson.Value) (*hujson.Object, bool) {
	obj, ok := v.Value.(*hujson.Object)
	return obj, ok
}

// EnsureObject returns the object member named key, creating it when absent.
// It returns false when v is not an object or the existing member is present
// but not an object (so callers never silently discard user data).
func EnsureObject(v *hujson.Value, key string) (*hujson.Object, bool) {
	obj, ok := v.Value.(*hujson.Object)
	if !ok {
		return nil, false
	}
	for i := range obj.Members {
		if memberName(&obj.Members[i]) == key {
			child, isObj := obj.Members[i].Value.Value.(*hujson.Object)
			return child, isObj
		}
	}
	child := &hujson.Object{}
	obj.Members = append(obj.Members, hujson.ObjectMember{
		Name:  hujson.Value{Value: hujson.Literal(fmt.Sprintf("%q", key))},
		Value: hujson.Value{Value: child},
	})
	return child, true
}

// SetValue sets a member on an object from a Go value, replacing any existing
// member with the same name.
func SetValue(obj *hujson.Object, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	parsed, err := hujson.Parse(encoded)
	if err != nil {
		return err
	}
	for i := range obj.Members {
		if memberName(&obj.Members[i]) == key {
			obj.Members[i].Value = parsed
			return nil
		}
	}
	obj.Members = append(obj.Members, hujson.ObjectMember{
		Name:  hujson.Value{Value: hujson.Literal(fmt.Sprintf("%q", key))},
		Value: parsed,
	})
	return nil
}

// DeleteMember removes the member named key from obj, reporting whether it
// existed.
func DeleteMember(obj *hujson.Object, key string) bool {
	for i := range obj.Members {
		if memberName(&obj.Members[i]) == key {
			obj.Members = append(obj.Members[:i], obj.Members[i+1:]...)
			return true
		}
	}
	return false
}

func GetField(v *hujson.Value, key string) (*hujson.Value, bool) {
	obj, ok := v.Value.(*hujson.Object)
	if !ok {
		return nil, false
	}
	for i := range obj.Members {
		if memberName(&obj.Members[i]) == key {
			return &obj.Members[i].Value, true
		}
	}
	return nil, false
}

func GetNestedField(v *hujson.Value, path []string) (*hujson.Value, bool) {
	current := v
	for i, key := range path {
		field, ok := GetField(current, key)
		if !ok {
			return nil, false
		}
		if i == len(path)-1 {
			return field, true
		}
		current = field
	}
	return nil, false
}

func TopLevelKeys(v *hujson.Value) []string {
	obj, ok := v.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	keys := make([]string, len(obj.Members))
	for i := range obj.Members {
		keys[i] = memberName(&obj.Members[i])
	}
	return keys
}

func FieldValueString(v *hujson.Value) string {
	switch lit := v.Value.(type) {
	case hujson.Literal:
		return strings.TrimSpace(string(lit))
	default:
		return strings.TrimSpace(v.String())
	}
}
