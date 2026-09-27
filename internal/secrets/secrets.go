package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func GenerateClientToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return "nk-" + hex.EncodeToString(b)
}

func GenerateAPIKey() (string, string) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	token := "nk-" + hex.EncodeToString(b)
	id := hex.EncodeToString(b[:4])
	return id, token
}

// ExistingTokenFile reports the path of a file in dir that already holds a
// client_token, or "" when none is found. It is a compatibility pre-check for
// callers that want to avoid overwriting a token; nenya itself (via
// `secret set`) remains the authority on the secrets layout, so this must not
// be treated as the source of truth.
func ExistingTokenFile(dir string) string {
	if dir == "" {
		return ""
	}
	// A secrets directory may hold the shipped unit's `secrets.json`, a single
	// `secrets` file, or arbitrarily named `*.json` files merged by name.
	for _, name := range []string{"secrets.json", "secrets"} {
		if p := filepath.Join(dir, name); fileHasClientToken(p) {
			return p
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if fileHasClientToken(p) {
			return p
		}
	}
	return ""
}

func fileHasClientToken(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return HasClientToken(data)
}

// HasClientToken reports whether data is a secrets document with a non-empty
// client_token. Unparseable data is reported as "no token".
func HasClientToken(data []byte) bool {
	return ClientTokenFromJSON(data) != ""
}

// ClientTokenFromJSON returns the client_token in a secrets document, or ""
// when the document has none or is unparseable. It is the single parser for the
// secrets JSON shape, shared by the read-only compatibility checks in this
// package and by the container token reader.
func ClientTokenFromJSON(data []byte) string {
	var s struct {
		ClientToken string `json:"client_token"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return ""
	}
	return s.ClientToken
}

// ClientTokenInDir resolves the effective client_token in a secrets directory,
// merging *.json files in ascending name order (last non-empty wins) with a
// fallback to a single "<dir>/secrets.json". This mirrors nenya's secrets merge
// order; it is a documented read-only compatibility check for the window before
// a contract read surface exists (NENYA-103), not the source of truth.
func ClientTokenInDir(dir string) string {
	token := ""
	if entries, err := os.ReadDir(dir); err == nil {
		var files []string
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
				files = append(files, filepath.Join(dir, entry.Name()))
			}
		}
		sort.Strings(files)
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			if t := ClientTokenFromJSON(data); t != "" {
				token = t
			}
		}
	}
	if token == "" {
		if data, err := os.ReadFile(filepath.Join(dir, "secrets.json")); err == nil {
			token = ClientTokenFromJSON(data)
		}
	}
	return token
}
