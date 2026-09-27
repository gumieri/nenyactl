// Package clients renders ready-to-paste configuration for gateway clients
// (OpenCode, Cursor, Claude Code, Aider) from a resolved endpoint and token.
//
// The endpoint and token are supplied by the caller (resolved from the local
// deployment); this package only formats them, so it works regardless of how
// the deployment was created and never reimplements nenya's path/secret
// resolution.
package clients

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Endpoint is the resolved gateway base URL and client token a client needs.
type Endpoint struct {
	// BaseURL is the gateway origin, e.g. http://localhost:8080 (no trailing
	// slash).
	BaseURL string
	// Token is the client_token used for Authorization: Bearer.
	Token string
}

// Name identifies a supported client.
type Name string

const (
	OpenCode Name = "opencode"
	Cursor   Name = "cursor"
	Claude   Name = "claude"
	Aider    Name = "aider"
)

// Supported lists the client names nenyactl can render, in display order.
func Supported() []Name { return []Name{OpenCode, Cursor, Claude, Aider} }

// ParseName validates a user-supplied client name.
func ParseName(s string) (Name, error) {
	for _, n := range Supported() {
		if string(n) == strings.ToLower(strings.TrimSpace(s)) {
			return n, nil
		}
	}
	return "", fmt.Errorf("unknown client %q (supported: %s)", s, namesList())
}

func namesList() string {
	parts := make([]string, 0, len(Supported()))
	for _, n := range Supported() {
		parts = append(parts, string(n))
	}
	return strings.Join(parts, ", ")
}

// Snippet is a rendered client configuration.
type Snippet struct {
	Name Name
	// Description is a one-line human summary.
	Description string
	// Body is the snippet to print or write (JSON or key=value text).
	Body string
	// ConfigPath is the conventional config file location, when the client
	// uses one (empty for env-var-only clients).
	ConfigPath string
	// MergeKey is the top-level JSON key a merge-style writer must preserve
	// when updating an existing config file (empty for non-JSON clients).
	MergeKey string
}

// Render produces the snippet for a client at an endpoint.
func Render(name Name, ep Endpoint) (Snippet, error) {
	base := strings.TrimRight(ep.BaseURL, "/")
	switch name {
	case OpenCode:
		return openCode(base, ep.Token), nil
	case Cursor:
		return cursor(base, ep.Token), nil
	case Claude:
		return claude(base, ep.Token), nil
	case Aider:
		return aider(base, ep.Token), nil
	default:
		return Snippet{}, fmt.Errorf("unsupported client %q", name)
	}
}

// RenderAll returns every snippet, in Supported order.
func RenderAll(ep Endpoint) ([]Snippet, error) {
	out := make([]Snippet, 0, len(Supported()))
	for _, n := range Supported() {
		s, err := Render(n, ep)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func openCode(base, token string) Snippet {
	provider := map[string]any{
		"nenya": map[string]any{
			"npm":  "@ai-sdk/openai-compatible",
			"name": "Nenya Gateway",
			"options": map[string]any{
				"baseURL": base + "/v1",
				"apiKey":  token,
			},
			"models": map[string]any{
				"build": map[string]any{"name": "build"},
			},
		},
	}
	body, _ := json.MarshalIndent(provider, "", "  ")
	return Snippet{
		Name:        OpenCode,
		Description: "Add the provider block to ~/.config/opencode/opencode.json",
		Body:        string(body),
		ConfigPath:  "~/.config/opencode/opencode.json",
		MergeKey:    "provider",
	}
}

func cursor(base, token string) Snippet {
	return Snippet{
		Name:        Cursor,
		Description: "Settings → Models → OpenAI API Key, and Override OpenAI Base URL",
		Body:        fmt.Sprintf("Base URL: %s/v1\nAPI Key:  %s", base, token),
	}
}

func claude(base, token string) Snippet {
	return Snippet{
		Name:        Claude,
		Description: "Environment for Claude Code (ANTHROPIC_BASE_URL)",
		Body:        fmt.Sprintf("export ANTHROPIC_BASE_URL=%q\nexport ANTHROPIC_AUTH_TOKEN=%q", base, token),
	}
}

func aider(base, token string) Snippet {
	return Snippet{
		Name:        Aider,
		Description: "Aider flags for an OpenAI-compatible endpoint",
		Body:        fmt.Sprintf("--openai-api-base %s/v1\n--openai-api-key %s", base, token),
	}
}

// MergeOpenCodeProvider updates an existing OpenCode config's provider.other
// object for the "nenya" key, preserving every other key. It returns the full
// merged JSON document. Existing config that is not a JSON object is rejected.
func MergeOpenCodeProvider(existing []byte, ep Endpoint) ([]byte, error) {
	base := strings.TrimRight(ep.BaseURL, "/")
	snippet := openCode(base, ep.Token)

	var root map[string]any
	if len(strings.TrimSpace(string(existing))) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, fmt.Errorf("existing opencode.json is not a JSON object: %w", err)
		}
	}
	if root == nil {
		root = map[string]any{}
	}

	var provider map[string]any
	if raw, ok := root["provider"].(map[string]any); ok {
		provider = raw
	} else {
		provider = map[string]any{}
	}

	var block map[string]any
	if err := json.Unmarshal([]byte(snippet.Body), &block); err != nil {
		return nil, err
	}
	if built, ok := block["nenya"].(map[string]any); ok {
		provider["nenya"] = built
	}
	root["provider"] = provider

	return json.MarshalIndent(root, "", "  ")
}

// SortedKeys is a small helper for deterministic output in callers.
func SortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
