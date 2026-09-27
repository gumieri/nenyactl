package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	var s struct {
		ClientToken string `json:"client_token"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return false
	}
	return s.ClientToken != ""
}
