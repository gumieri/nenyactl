package containers

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Tests for the NCTL-51 wizard fixes that go beyond the navigation set in
// tui_test.go: keystroke routing, credential masking, provider dedupe, and
// custom-provider validation.

func TestContainersTUI_TypingReachesInputs(t *testing.T) {
	providers := []ProviderDef{
		{Name: "gemini", Help: "Gemini", Auth: "sk-...", NeedsKey: true},
	}

	t.Run("typed runes reach the focused key input", func(t *testing.T) {
		m := newTUIModelWithProviders(providers)
		m.selected[0] = true
		mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		for _, r := range "ab1" {
			mp = update(mp, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		if got := mp.keys[0].input.Value(); got != "ab1" {
			t.Errorf("input value = %q, want ab1 (keystrokes must reach the input)", got)
		}
	})

	t.Run("typed runes reach the custom name input", func(t *testing.T) {
		m := newTUIModelWithProviders(providers)
		m.table.SetCursor(len(providers)) // "+ Add custom provider..." row
		mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		for _, r := range "cu" {
			mp = update(mp, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		if got := mp.customName.Value(); got != "cu" {
			t.Errorf("custom name = %q, want cu (keystrokes must reach the input)", got)
		}
	})

	t.Run("typed runes reach the custom key input after tab", func(t *testing.T) {
		m := newTUIModelWithProviders(providers)
		m.table.SetCursor(len(providers))
		mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		mp = update(mp, tea.KeyMsg{Type: tea.KeyTab}) // name -> key field
		for _, r := range "sk9" {
			mp = update(mp, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		if got := mp.customKey.Value(); got != "sk9" {
			t.Errorf("custom key = %q, want sk9 (keystrokes must reach the focused input)", got)
		}
		if mp.customFocus != 1 {
			t.Errorf("customFocus = %d, want 1 (key field)", mp.customFocus)
		}
	})

	t.Run("shift+tab moves back from the key field to the name field", func(t *testing.T) {
		m := newTUIModelWithProviders(providers)
		m.table.SetCursor(len(providers))
		mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		mp = update(mp, tea.KeyMsg{Type: tea.KeyTab})
		mp = update(mp, tea.KeyMsg{Type: tea.KeyShiftTab})
		if mp.customFocus != 0 {
			t.Errorf("customFocus = %d, want 0 (name field)", mp.customFocus)
		}
	})
}

func TestContainersTUI_CustomScreenFocusLifecycle(t *testing.T) {
	providers := []ProviderDef{{Name: "gemini", Help: "Gemini", Auth: "sk-...", NeedsKey: true}}

	t.Run("esc and re-entry leave exactly one input focused", func(t *testing.T) {
		m := newTUIModelWithProviders(providers)
		m.table.SetCursor(len(providers))
		mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		mp = update(mp, tea.KeyMsg{Type: tea.KeyTab}) // key field focused
		mp = update(mp, tea.KeyMsg{Type: tea.KeyEsc})
		if mp.screen != screenSelect {
			t.Fatalf("screen = %v, want screenSelect", mp.screen)
		}
		reentered := update(mp, tea.KeyMsg{Type: tea.KeyEnter})
		focused := 0
		if reentered.customName.Focused() {
			focused++
		}
		if reentered.customKey.Focused() {
			focused++
		}
		if focused != 1 {
			t.Errorf("focused inputs after re-entry = %d, want 1 (double cursor bug)", focused)
		}
		if !reentered.customName.Focused() {
			t.Error("re-entry must focus the name field")
		}
	})

	t.Run("a custom name colliding with an earlier custom entry is rejected", func(t *testing.T) {
		m := newTUIModelWithProviders(providers)
		m.table.SetCursor(len(providers))
		mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		mp.customFocus = 1
		mp.customName.SetValue("myprov")
		mp.customKey.SetValue("sk-first")
		mp = update(mp, tea.KeyMsg{Type: tea.KeyEnter})
		if len(mp.customResults) != 1 {
			t.Fatalf("first save failed: %v", mp.validationMsg)
		}
		mp.customFocus = 1 // save reset focus to the name field
		mp.customName.SetValue("myprov")
		mp.customKey.SetValue("sk-second")
		mp = update(mp, tea.KeyMsg{Type: tea.KeyEnter})
		if len(mp.customResults) != 1 {
			t.Error("duplicate custom name must be rejected")
		}
		if !strings.Contains(mp.validationMsg, "already in use") {
			t.Errorf("validationMsg = %q, want a duplicate-name message", mp.validationMsg)
		}
	})
}

func TestContainersTUI_SubmittedFlag(t *testing.T) {
	providers := []ProviderDef{{Name: "gemini", Help: "Gemini", Auth: "sk-...", NeedsKey: true}}

	t.Run("gated submit sets submitted", func(t *testing.T) {
		m := newTUIModelWithProviders(providers)
		m.selected[0] = true
		mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		mp.keys[0].input.SetValue("sk-real")
		done := update(mp, tea.KeyMsg{Type: tea.KeyEnter})
		if !done.submitted {
			t.Error("gated submit must set submitted")
		}
	})

	t.Run("abort does not set submitted", func(t *testing.T) {
		m := newTUIModelWithProviders(providers)
		m.selected[0] = true
		mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		mp.keys[0].input.SetValue("sk-real")
		mp = update(mp, tea.KeyMsg{Type: tea.KeyEsc})
		mp = update(mp, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		if !mp.quitting {
			t.Fatal("q should quit from the select screen")
		}
		if mp.submitted {
			t.Error("abort (esc, then q) must not set submitted")
		}
	})

	t.Run("custom-only finish persists custom entries", func(t *testing.T) {
		m := newTUIModelWithProviders(providers) // nothing NeedsKey-selected
		m.table.SetCursor(len(providers))
		mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
		mp.customFocus = 1
		mp.customName.SetValue("myprov")
		mp.customKey.SetValue("sk-x")
		mp = update(mp, tea.KeyMsg{Type: tea.KeyEnter})
		if len(mp.customResults) != 1 {
			t.Fatalf("custom save failed: %v", mp.validationMsg)
		}
		mp = update(mp, tea.KeyMsg{Type: tea.KeyEsc}) // back to select
		mp.table.SetCursor(0)                         // off the "+ custom" row
		fin := update(mp, tea.KeyMsg{Type: tea.KeyEnter})
		if !fin.quitting || !fin.submitted {
			t.Fatalf("quitting=%v submitted=%v, want true/true (enter finish)", fin.quitting, fin.submitted)
		}
		if got := fin.Results()["myprov"]; got != "sk-x" {
			t.Errorf("Results()[myprov] = %q, want sk-x (custom entries must survive a custom-only finish)", got)
		}
	})
}

func TestContainersTUI_CredentialMasking(t *testing.T) {
	providers := []ProviderDef{{Name: "gemini", Help: "Gemini", Auth: "sk-...", NeedsKey: true}}
	m := newTUIModelWithProviders(providers)
	m.selected[0] = true
	mp := update(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if mp.keys[0].input.EchoMode != textinput.EchoPassword {
		t.Error("key input must be echo-masked (credentials must not hit scrollback)")
	}
	if mp.customKey.EchoMode != textinput.EchoPassword {
		t.Error("custom key input must be echo-masked")
	}
}

func TestContainersTUI_DedupeProviders(t *testing.T) {
	m := newTUIModelWithProviders([]ProviderDef{
		{Name: "dup", Help: "first", Auth: "a", NeedsKey: true},
		{Name: "dup", Help: "second", Auth: "b", NeedsKey: true},
		{Name: "unique", Help: "u", Auth: "c", NeedsKey: true},
	})
	count := 0
	for _, p := range m.providers {
		if p.Name == "dup" {
			count++
			if p.Help != "first" {
				t.Errorf("kept the wrong duplicate: help = %q, want first", p.Help)
			}
		}
	}
	if count != 1 {
		t.Errorf("duplicate providers present %d times, want 1 (first kept)", count)
	}
}

func TestContainersTUI_CustomProviderValidation(t *testing.T) {
	providers := []ProviderDef{{Name: "gemini", Help: "Gemini", Auth: "sk-...", NeedsKey: true}}

	enterCustom := func(t *testing.T) *tuiModel {
		t.Helper()
		m := newTUIModelWithProviders(providers)
		m.table.SetCursor(len(providers))
		return update(&m, tea.KeyMsg{Type: tea.KeyEnter})
	}

	t.Run("whitespace-only fields are rejected with a message", func(t *testing.T) {
		m := enterCustom(t)
		m.customFocus = 1
		m.customName.SetValue("  ")
		m.customKey.SetValue("  ")
		m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
		if len(m.customResults) != 0 {
			t.Error("whitespace-only name/key must not be saved")
		}
		if m.validationMsg == "" {
			t.Error("expected a validation message for empty fields")
		}
	})

	t.Run("values are trimmed on save", func(t *testing.T) {
		m := enterCustom(t)
		m.customFocus = 1
		m.customName.SetValue("  myprov  ")
		m.customKey.SetValue("  sk-secret  ")
		m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
		if len(m.customResults) != 1 {
			t.Fatalf("customResults = %d, want 1", len(m.customResults))
		}
		if m.customResults[0].Name != "myprov" || m.customResults[0].Value != "sk-secret" {
			t.Errorf("saved = %q/%q, want trimmed myprov/sk-secret", m.customResults[0].Name, m.customResults[0].Value)
		}
		if got := m.Results()["myprov"]; got != "sk-secret" {
			t.Errorf("Results()[myprov] = %q, want sk-secret", got)
		}
	})

	t.Run("duplicate names are rejected", func(t *testing.T) {
		m := enterCustom(t)
		m.customFocus = 1
		m.customName.SetValue("gemini")
		m.customKey.SetValue("sk-x")
		m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
		if len(m.customResults) != 0 {
			t.Error("a custom provider shadowing a builtin must be rejected")
		}
		if m.validationMsg == "" {
			t.Error("expected a duplicate-name validation message")
		}
	})
}

func TestContainersTUI_MissingKeysMessageCap(t *testing.T) {
	missing := []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7"}
	msg := missingKeysMessage(missing)
	if !strings.Contains(msg, "p1") || !strings.Contains(msg, "p5") {
		t.Errorf("message %q must name the first five providers", msg)
	}
	if strings.Contains(msg, "p6") {
		t.Errorf("message %q must cap the named list at five", msg)
	}
	if !strings.Contains(msg, "2 more") {
		t.Errorf("message %q must report the overflow count", msg)
	}
	if short := missingKeysMessage(missing[:3]); strings.Contains(short, "more") {
		t.Errorf("short list message %q must not show an overflow count", short)
	}
	if exact := missingKeysMessage(missing[:5]); strings.Contains(exact, "more") {
		t.Errorf("exactly-five message %q must not show an overflow count (boundary)", exact)
	}
}
