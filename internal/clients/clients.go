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
		return openCode(base, ep.Token)
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

// openCode renders an OpenCode V2 provider entry. OpenCode V2 uses the
// `providers` map keyed by provider ID, with `package` selecting the runtime
// and `settings` carrying package-specific options (baseURL/apiKey). The old V1
// `provider`/`npm`/`options` shape is not valid in V2.
func openCode(base, token string) (Snippet, error) {
	doc := map[string]any{
		"providers": map[string]any{
			"nenya": map[string]any{
				"name":    "Nenya Gateway",
				"package": "@opencode/ai/providers/openai-compatible",
				"settings": map[string]any{
					"baseURL": base + "/v1",
					"apiKey":  token,
				},
				"models": map[string]any{
					"build": map[string]any{"name": "build"},
				},
			},
		},
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return Snippet{}, fmt.Errorf("render opencode snippet: %w", err)
	}
	return Snippet{
		Name:        OpenCode,
		Description: "Add the providers block to ~/.config/opencode/opencode.json",
		Body:        string(body),
		ConfigPath:  "~/.config/opencode/opencode.json",
		Mergeable:   true,
	}, nil
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

// MergeOpenCodeProvider regenerates the providers.nenya object in an existing
// OpenCode config, preserving every other key and any JSONC comments. It returns
// the full merged document and whether a legacy V1 provider.nenya was removed.
// providers.nenya itself is replaced wholesale, since it is generated from the
// resolved endpoint. Existing config that is not a JSON object is rejected.
func MergeOpenCodeProvider(existing []byte, ep Endpoint) ([]byte, bool, error) {
	base := strings.TrimRight(ep.BaseURL, "/")
	snippet, err := openCode(base, ep.Token)
	if err != nil {
		return nil, false, err
	}

	v, err := jsonc.ParseDoc(existing)
	if err != nil {
		return nil, false, err
	}
	root, ok := jsonc.GetObject(v)
	if !ok {
		return nil, false, fmt.Errorf("existing opencode.json is not a JSON object")
	}

	var block map[string]any
	if err := json.Unmarshal([]byte(snippet.Body), &block); err != nil {
		return nil, false, fmt.Errorf("parse opencode snippet: %w", err)
	}
	providers, ok := block["providers"].(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("internal error: opencode snippet has no providers object")
	}
	built, ok := providers["nenya"].(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("internal error: opencode snippet is not a provider object")
	}

	providersObj, ok := jsonc.EnsureObject(v, "providers")
	if !ok {
		return nil, false, fmt.Errorf("existing opencode.json has a non-object \"providers\" value")
	}
	if err := jsonc.SetValue(providersObj, "nenya", built); err != nil {
		return nil, false, err
	}

	// Migrate away from the V1 shape: drop a legacy top-level provider.nenya so
	// an upgrading user does not end up with both blocks, and drop the now-empty
	// provider object entirely.
	removedLegacy := false
	if legacyProvider, ok := jsonc.GetNestedField(v, []string{"provider"}); ok {
		if legacyObj, ok := jsonc.GetObject(legacyProvider); ok {
			removedLegacy = jsonc.DeleteMember(legacyObj, "nenya")
			if removedLegacy && len(legacyObj.Members) == 0 {
				jsonc.DeleteMember(root, "provider")
			}
		}
	}

	merged, err := jsonc.Render(v)
	if err != nil {
		return nil, false, err
	}
	return merged, removedLegacy, nil
}
