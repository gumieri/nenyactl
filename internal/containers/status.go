package containers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultPort is the container-internal port and the fallback published port.
const DefaultPort = "8080"

// PublishedPort returns the host port a deployment publishes, read from the
// compose port mapping (the source of truth written by `containers setup`). It
// falls back to DefaultPort when no mapping is present.
func PublishedPort(dir string) string {
	if compose, err := os.ReadFile(filepath.Join(dir, "compose.yml")); err == nil {
		if p, ok := parsePublishedPort(string(compose)); ok {
			return p
		}
	}
	return DefaultPort
}

// parsePublishedPort extracts the published (host) port from a compose file's
// ports block. It only reads lines under `ports:` so volume mappings are not
// mistaken for ports.
func parsePublishedPort(compose string) (string, bool) {
	inPorts := false
	portsIndent := 0
	for _, line := range strings.Split(compose, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "ports:":
			inPorts = true
			portsIndent = leadingSpaces(line)
			continue
		case !inPorts, trimmed == "":
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), `"'`)
			parts := strings.Split(value, ":")
			if len(parts) >= 2 {
				return parts[len(parts)-2], true
			}
			continue
		}
		if leadingSpaces(line) <= portsIndent {
			inPorts = false
		}
	}
	return "", false
}

func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " \t"))
}

// ClientToken resolves the effective client_token for a container deployment.
// It merges secrets/*.json in name order (last non-empty wins), matching
// nenya's secrets merge (CONTRACT.md §6.2), and falls back to a single
// secrets.json when present or when the directory merge yields nothing.
func ClientToken(dir string) string {
	token := ""
	if entries, err := os.ReadDir(filepath.Join(dir, "secrets")); err == nil {
		var files []string
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
				files = append(files, filepath.Join(dir, "secrets", entry.Name()))
			}
		}
		sort.Strings(files)
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			if t := tokenFromJSON(data); t != "" {
				token = t
			}
		}
	}
	if token == "" {
		if data, err := os.ReadFile(filepath.Join(dir, "secrets.json")); err == nil {
			token = tokenFromJSON(data)
		}
	}
	return token
}

func tokenFromJSON(data []byte) string {
	var s struct {
		ClientToken string `json:"client_token"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return ""
	}
	return s.ClientToken
}
