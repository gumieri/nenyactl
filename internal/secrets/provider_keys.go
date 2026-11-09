package secrets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProviderKeyNames returns the provider names that have a non-empty entry
// under provider_keys across the *.json secrets documents in dir, merged in
// ascending file name order (nenya's merge order; a bare `secrets` file is
// deliberately ignored — that name is a $CREDENTIALS_DIRECTORY entry only the
// server resolves). Only NAMES are returned — never values — so callers can
// render set/unset status without handling credentials. Like ClientTokenInDir,
// this is a documented read-only compatibility check, not the source of truth;
// an unreadable directory reports no keys.
func ProviderKeyNames(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		// An unreadable secrets directory reports no keys — the same
		// fail-silent semantics as the client-token shim (see the package
		// doc): a compatibility check, not the source of truth.
		return nil
	}

	names := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s struct {
			ProviderKeys map[string]string `json:"provider_keys"`
		}
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		for name, v := range s.ProviderKeys {
			if strings.TrimSpace(v) != "" {
				names[name] = true
			}
		}
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}
