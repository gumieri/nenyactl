package containers

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gumieri/nenyactl/internal/secrets"
)

// DefaultPort is the container-internal port and the fallback published port.
const DefaultPort = "8080"

// PublishedPort returns the host port a deployment publishes, read from the
// compose port mapping (the source of truth written by `containers setup`). It
// falls back to DefaultPort when no mapping is present.
func PublishedPort(dir string) string {
	if p, ok := HostPort(dir); ok {
		return p
	}
	return DefaultPort
}

// HostPort returns the host port a deployment publishes and whether an explicit
// mapping was found. Unlike PublishedPort it does not fall back, so callers can
// decide what to do when a deployment (e.g. host-network) has no mapping.
func HostPort(dir string) (string, bool) {
	if compose, err := os.ReadFile(filepath.Join(dir, "compose.yml")); err == nil {
		if p, ok := parsePublishedPort(string(compose)); ok {
			return p, true
		}
	}
	return "", false
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
//
// This is a documented read-only compatibility check for the window before a
// contract read surface exists (NENYA-103); the merge itself lives in
// internal/secrets so there is one implementation.
func ClientToken(dir string) string {
	if token := secrets.ClientTokenInDir(filepath.Join(dir, "secrets")); token != "" {
		return token
	}
	return secrets.ClientTokenInDir(dir)
}
