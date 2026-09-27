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

// PublishedPort returns the host port a deployment publishes. It prefers the
// compose port mapping, then PORT in .env, then DefaultPort. It never assumes
// 8080 when the deployment says otherwise.
func PublishedPort(dir string) string {
	if compose, err := os.ReadFile(filepath.Join(dir, "compose.yml")); err == nil {
		if p, ok := parsePublishedPort(string(compose)); ok {
			return p
		}
	}
	if p := envPort(filepath.Join(dir, ".env")); p != "" {
		return p
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

// envPort reads PORT from an env file, tolerating `export PORT=...`.
func envPort(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimPrefix(strings.TrimSpace(line), "export ")
		if strings.HasPrefix(line, "PORT=") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "PORT=")), `"'`)
		}
	}
	return ""
}

// ClientToken resolves the effective client_token for a container deployment.
// It merges secrets/*.json in name order (last non-empty wins), matching
// nenya's secrets merge (CONTRACT.md §6.2), and falls back to a single
// secrets.json when present.
func ClientToken(dir string) string {
	secretsDir := filepath.Join(dir, "secrets")
	entries, err := os.ReadDir(secretsDir)
	if err != nil {
		if data, readErr := os.ReadFile(filepath.Join(dir, "secrets.json")); readErr == nil {
			return tokenFromJSON(data)
		}
		return ""
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			files = append(files, filepath.Join(secretsDir, entry.Name()))
		}
	}
	sort.Strings(files)

	token := ""
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		if t := tokenFromJSON(data); t != "" {
			token = t
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
