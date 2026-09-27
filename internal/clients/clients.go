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
	"strings"

	"github.com/gumieri/nenyactl/internal/jsonc"
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
	// Mergeable marks snippets nenyactl can merge into an existing JSON config
	// file (only OpenCode today).
	Mergeable bool
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
		Mergeable:   true,
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

// MergeOpenCodeProvider updates an existing OpenCode config's provider.nenya
// object, preserving every other key and any JSONC comments. It returns the
// full merged document. Existing config that is not a JSON object is rejected.
func MergeOpenCodeProvider(existing []byte, ep Endpoint) ([]byte, error) {
	base := strings.TrimRight(ep.BaseURL, "/")
	snippet := openCode(base, ep.Token)

	v, err := jsonc.ParseDoc(existing)
	if err != nil {
		return nil, err
	}
	if _, ok := jsonc.GetObject(v); !ok {
		return nil, fmt.Errorf("existing opencode.json is not a JSON object")
	}

	var block map[string]any
	if err := json.Unmarshal([]byte(snippet.Body), &block); err != nil {
		return nil, err
	}
	built, ok := block["nenya"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("internal error: opencode snippet is not a provider object")
	}

	provider := jsonc.EnsureObject(v, "provider")
	if provider == nil {
		return nil, fmt.Errorf("existing opencode.json is not a JSON object")
	}
	if err := jsonc.SetValue(provider, "nenya", built); err != nil {
		return nil, err
	}

	return jsonc.Render(v)
}
