package jsonc

import (
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

func WriteFile(path string, v *hujson.Value, perm os.FileMode) error {
	data := v.Pack()
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func memberName(m *hujson.ObjectMember) string {
	return m.Name.Value.(hujson.Literal).String()
}

func MemberName(m *hujson.ObjectMember) string {
	return memberName(m)
}

func AsObject(v *hujson.Value) (*hujson.Object, bool) {
	return GetObject(v)
}

func GetObject(v *hujson.Value) (*hujson.Object, bool) {
	obj, ok := v.Value.(*hujson.Object)
	return obj, ok
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

func SetField(v *hujson.Value, key string, literal hujson.Literal) {
	obj, ok := v.Value.(*hujson.Object)
	if !ok {
		return
	}
	for i := range obj.Members {
		if memberName(&obj.Members[i]) == key {
			obj.Members[i].Value.Value = literal
			return
		}
	}
	obj.Members = append(obj.Members, hujson.ObjectMember{
		Name:  hujson.Value{Value: hujson.Literal(fmt.Sprintf("%q", key))},
		Value: hujson.Value{Value: literal},
	})
}

func EnsureNestedObject(v *hujson.Value, path []string) (*hujson.Object, bool) {
	current := v
	for _, key := range path {
		field, ok := GetField(current, key)
		if !ok {
			return nil, false
		}
		if _, ok := field.Value.(*hujson.Object); !ok {
			return nil, false
		}
		current = field
	}
	return current.Value.(*hujson.Object), true
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

func SetNestedField(v *hujson.Value, path []string, literal hujson.Literal) bool {
	if len(path) == 0 {
		return false
	}

	obj, ok := v.Value.(*hujson.Object)
	if !ok {
		return false
	}

	for _, key := range path[:len(path)-1] {
		field, ok := getField(obj, key)
		if !ok {
			newObj := &hujson.Object{}
			obj.Members = append(obj.Members, hujson.ObjectMember{
				Name:  hujson.Value{Value: hujson.Literal(fmt.Sprintf("%q", key))},
				Value: hujson.Value{Value: newObj},
			})
			obj = newObj
			continue
		}
		nested, ok := field.Value.(*hujson.Object)
		if !ok {
			return false
		}
		obj = nested
	}

	lastKey := path[len(path)-1]
	for i := range obj.Members {
		if memberName(&obj.Members[i]) == lastKey {
			obj.Members[i].Value.Value = literal
			return true
		}
	}

	obj.Members = append(obj.Members, hujson.ObjectMember{
		Name:  hujson.Value{Value: hujson.Literal(fmt.Sprintf("%q", lastKey))},
		Value: hujson.Value{Value: literal},
	})
	return true
}

func getField(obj *hujson.Object, key string) (*hujson.Value, bool) {
	for i := range obj.Members {
		if memberName(&obj.Members[i]) == key {
			return &obj.Members[i].Value, true
		}
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
