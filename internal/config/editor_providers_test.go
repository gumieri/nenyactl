package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gumieri/nenyactl/internal/jsonc"
	"github.com/gumieri/nenyactl/internal/tui"
)

// Tests for the providers screen (NCTL-45): key-status listing from the
// effective doc + secrets-file presence, the masked key input, and the
// KeyChanges output the caller applies through nenya's single writer.

func providersSetup(t *testing.T, configJSON string, status []ProviderKeyStatus) *configModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := jsonc.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := newConfigModel(cfg, []byte(configJSON), status)

	// Open the providers section the way the sections screen does.
	for i, s := range m.sections {
		if s.isProviders {
			m.cursor = i
			break
		}
	}
	return update(&m, teaKeyEnter())
}

func typeRunes(m *configModel, s string) *configModel {
	for _, r := range s {
		m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func TestEditorTUI_ProvidersScreen(t *testing.T) {
	// The api_key values mirror what `nenya describe` emits: a set key is
	// always the [REDACTED] sentinel, never a literal or a visible ref.
	cfg := `{
		"providers": {
			"anthropic": {"api_key": "[REDACTED]", "url": "https://api.anthropic.com"},
			"openai": {"url": "https://api.openai.com"},
			"zai": {"api_key": "[REDACTED]"}
		}
	}`

	status := []ProviderKeyStatus{
		{Name: "openai", KeySet: true, Source: "secrets dir"},
		{Name: "zai", KeySet: true, Source: "secrets dir"},
	}

	setup := func(t *testing.T) *configModel {
		return providersSetup(t, cfg, status)
	}

	t.Run("rows report set/unset and source without values", func(t *testing.T) {
		m := setup(t)
		if len(m.providerRows) != 3 {
			t.Fatalf("providerRows = %d, want 3", len(m.providerRows))
		}
		byName := map[string]ProviderKeyStatus{}
		for _, r := range m.providerRows {
			byName[r.Name] = r
		}
		// anthropic: only the redacted config-level key → set via config.
		if a := byName["anthropic"]; !a.KeySet || a.Source != "config" {
			t.Errorf("anthropic = %+v, want set via config (redacted sentinel)", a)
		}
		// zai has both a config-level key and a secrets dir entry: nenya's
		// ResolveProviders uses only secrets, so the label stays "secrets dir".
		if z := byName["zai"]; !z.KeySet || z.Source != "secrets dir" {
			t.Errorf("zai = %+v, want secrets dir label to win", z)
		}
		if o := byName["openai"]; !o.KeySet || o.Source != "secrets dir" {
			t.Errorf("openai = %+v, want set via secrets dir", o)
		}
	})

	t.Run("unset provider renders a visible no-key flag", func(t *testing.T) {
		m := setup(t)
		m.providersView.Height = 10 // viewport renders empty at height 0
		m.providerRows = append(m.providerRows, ProviderKeyStatus{Name: "fresh"})
		m.updateProvidersContent()
		view := m.viewProviders(tui.Current())
		if !strings.Contains(view, "no key set") {
			t.Error("unset provider must be flagged 'no key set' in the view")
		}
	})

	t.Run("enter opens the masked key input for the selected provider", func(t *testing.T) {
		m := setup(t)
		m.providerCursor = 1 // openai
		update(m, teaKeyEnter())
		if m.screen != screenProviderEdit {
			t.Fatalf("screen = %v, want screenProviderEdit", m.screen)
		}
		if m.editingProvider != "openai" {
			t.Errorf("editingProvider = %q, want openai", m.editingProvider)
		}
		if m.keyInput.EchoMode != textinput.EchoPassword {
			t.Error("key input must be echo-masked")
		}
	})

	t.Run("typed keystrokes reach the masked input and record a KeyChange", func(t *testing.T) {
		m := setup(t)
		m.providerCursor = 1
		m = update(m, teaKeyEnter())
		m = typeRunes(m, "sk-typed")
		if got := m.keyInput.Value(); got != "sk-typed" {
			t.Fatalf("input value = %q, want sk-typed (keystrokes must reach the input)", got)
		}
		m = update(m, teaKeyEnter())
		if m.screen != screenProviders {
			t.Fatalf("screen = %v, want screenProviders", m.screen)
		}
		if got := m.keyChanges["openai"]; got != "sk-typed" {
			t.Errorf("keyChanges[openai] = %q, want the typed key", got)
		}
	})

	t.Run("esc on the input cancels without recording", func(t *testing.T) {
		m := setup(t)
		m.providerCursor = 1
		m = update(m, teaKeyEnter())
		m = typeRunes(m, "sk-nope")
		m = update(m, teaKeyEsc())
		if len(m.keyChanges) != 0 {
			t.Errorf("keyChanges = %v, want empty after cancel", m.keyChanges)
		}
	})

	t.Run("empty key input is rejected with a message", func(t *testing.T) {
		m := setup(t)
		m.providerCursor = 1
		m = update(m, teaKeyEnter())
		m = update(m, teaKeyEnter())
		if len(m.keyChanges) != 0 {
			t.Error("empty key must not be recorded")
		}
		if m.validationMsg == "" {
			t.Error("expected a validation message for an empty key")
		}
	})

	t.Run("validation message clears on esc after a rejected submit", func(t *testing.T) {
		m := setup(t)
		m.providerCursor = 1
		m = update(m, teaKeyEnter())
		m = update(m, teaKeyEnter()) // arms the empty-key message
		m = update(m, teaKeyEsc())
		if m.validationMsg != "" {
			t.Errorf("validationMsg = %q, want cleared on esc", m.validationMsg)
		}
	})

	t.Run("review lists provider key ops by name only", func(t *testing.T) {
		m := setup(t)
		m.keyChanges["openai"] = "sk-secret-value"
		view := m.viewReview(tui.Current())
		if !strings.Contains(view, "provider key openai") {
			t.Error("review must name the provider key op")
		}
		if strings.Contains(view, "sk-secret-value") {
			t.Error("review must never render the key value")
		}
	})

	t.Run("result carries KeyChanges for the caller", func(t *testing.T) {
		m := setup(t)
		m.keyChanges["openai"] = "sk-x"
		res, err := m.result()
		if err != nil {
			t.Fatalf("result: %v", err)
		}
		ops := res.ProviderKeys()
		if len(ops) != 1 || ops[0].Provider != "openai" || ops[0].Value != "sk-x" {
			t.Errorf("ProviderKeys() = %+v, want openai/sk-x", ops)
		}
		// The result must own its copy: mutating it must not touch the model.
		res.KeyChanges["openai"] = "mutated"
		if m.keyChanges["openai"] != "sk-x" {
			t.Error("result.KeyChanges must be a copy, not the live map")
		}
	})

	t.Run("discard-all clears pending key changes", func(t *testing.T) {
		m := setup(t)
		m.keyChanges["openai"] = "sk-x"
		m.discardAll()
		if len(m.keyChanges) != 0 {
			t.Errorf("keyChanges = %v, want cleared", m.keyChanges)
		}
	})
}

func TestSanitizeConfigName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"openai", "openai"},
		{"\x1b[31mhax\x1b[0m", "hax"},
		{"]plain]", "]plain]"}, // only CSI (ESC [) sequences are stripped
		{"a\tb", "ab"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := sanitizeConfigName(tc.in); got != tc.want {
			t.Errorf("sanitizeConfigName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildProviderRows(t *testing.T) {
	cfgJSON := []byte(`{"providers": {"b": {"url": "u"}, "a": {"api_key": "[REDACTED]"}, "refd": {"api_key": "env:MY_KEY"}, "filed": {"api_key": "file:/run/secrets/x"}, "nulled": {"api_key": null}, "empty": {"api_key": ""}, "num": {"api_key": 123}}}`)
	cfg, err := jsonc.ParseDoc(cfgJSON)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	rows := buildProviderRows(cfg, []ProviderKeyStatus{{Name: "b", KeySet: true, Source: "secrets dir"}})
	if len(rows) != 7 {
		t.Fatalf("rows = %d, want 7", len(rows))
	}
	byName := map[string]ProviderKeyStatus{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	// a: config-level key, redacted sentinel, no secrets entry → "config".
	if r := byName["a"]; !r.KeySet || r.Source != "config" {
		t.Errorf("a = %+v, want config set (redacted sentinel)", r)
	}
	// refd: a raw (pre-redaction) env reference classifies as env ref.
	if r := byName["refd"]; !r.KeySet || r.Source != "env ref" {
		t.Errorf("refd = %+v, want env ref", r)
	}
	if r := byName["filed"]; !r.KeySet || r.Source != "file ref" {
		t.Errorf("filed = %+v, want file ref", r)
	}
	// nulled/empty: explicit null or "" is unset.
	if r := byName["nulled"]; r.KeySet {
		t.Errorf("nulled = %+v, want unset (null api_key)", r)
	}
	if r := byName["empty"]; r.KeySet {
		t.Errorf("empty = %+v, want unset (empty api_key)", r)
	}
	// num: non-string kinds are not credentials.
	if r := byName["num"]; r.KeySet {
		t.Errorf("num = %+v, want unset (numeric api_key)", r)
	}
	if r := byName["b"]; !r.KeySet || r.Source != "secrets dir" {
		t.Errorf("b = %+v, want b/secrets dir set", r)
	}

	if got := buildProviderRows(cfg, nil); len(got) != 7 {
		t.Errorf("rows without secrets status = %d, want 7 (config-level presence still detected)", len(got))
	}

	noProviders, _ := jsonc.ParseDoc([]byte(`{"server": {}}`))
	if got := buildProviderRows(noProviders, nil); got != nil {
		t.Errorf("rows for a doc without providers = %+v, want nil", got)
	}
}

// teaKey helpers keep the KeyMsg construction in one place.

func teaKeyEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }

func teaKeyEsc() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEsc} }

func TestSanitizeConfigNameUTF8(t *testing.T) {
	// Valid UTF-8 must survive untouched: continuation bytes live in
	// 0x80–0xBF and must never be stripped as C1 controls.
	if got := sanitizeConfigName("pró-āciem"); got != "pró-āciem" {
		t.Errorf("sanitizeConfigName = %q, want the name untouched", got)
	}
	// C1 codepoints arriving as decoded escapes (U+0080–U+009F) are stripped
	// without harming neighbors.
	if got := sanitizeConfigName("a\u0085b"); got != "ab" {
		t.Errorf("sanitizeConfigName = %q, want %q (C1 stripped)", got, "ab")
	}
	// Non-CSI escape families are handled: OSC body through BEL.
	if got := sanitizeConfigName("\x1b]8;;http://x\x07click"); got != "click" {
		t.Errorf("sanitizeConfigName = %q, want click (OSC body dropped)", got)
	}
	// CSI final byte is any 0x40–0x7E, not just letters.
	if got := sanitizeConfigName("\x1b[3@half"); got != "half" {
		t.Errorf("sanitizeConfigName = %q, want half (@ is a valid CSI final)", got)
	}
}
