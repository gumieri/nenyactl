package tui

import (
	"testing"
)

func TestNewHelpModel(t *testing.T) {
	t.Run("creates a non-nil help model", func(t *testing.T) {
		m := NewHelpModel()
		if m.Styles.ShortKey.Render("x") == "" {
			t.Error("expected ShortKey style to render something")
		}
	})
}

func TestListKeyMap(t *testing.T) {
	t.Run("all key bindings are defined", func(t *testing.T) {
		if ListKeyMap.Up.Help().Key == "" {
			t.Error("Up key help is empty")
		}
		if ListKeyMap.Down.Help().Key == "" {
			t.Error("Down key help is empty")
		}
		if ListKeyMap.Select.Help().Key == "" {
			t.Error("Select key help is empty")
		}
		if ListKeyMap.Quit.Help().Key == "" {
			t.Error("Quit key help is empty")
		}
	})

	t.Run("ShortHelp returns non-empty bindings", func(t *testing.T) {
		bindings := ListKeyMap.ShortHelp()
		if len(bindings) == 0 {
			t.Error("ShortHelp should return at least one binding")
		}
	})

	t.Run("FullHelp returns a slice of bindings", func(t *testing.T) {
		bindings := ListKeyMap.FullHelp()
		if len(bindings) != 1 {
			t.Errorf("expected 1 row of bindings, got %d", len(bindings))
		}
	})
}

func TestFormKeyMap(t *testing.T) {
	t.Run("ShortHelp returns non-empty bindings", func(t *testing.T) {
		bindings := FormKeyMap.ShortHelp()
		if len(bindings) == 0 {
			t.Error("FormKeyMap ShortHelp should return at least one binding")
		}
	})
}

func TestConfirmKeyMap(t *testing.T) {
	t.Run("ShortHelp returns non-empty bindings", func(t *testing.T) {
		bindings := ConfirmKeyMap.ShortHelp()
		if len(bindings) == 0 {
			t.Error("ConfirmKeyMap ShortHelp should return at least one binding")
		}
	})
}

func TestPickerKeyMap(t *testing.T) {
	t.Run("ShortHelp returns non-empty bindings", func(t *testing.T) {
		bindings := PickerKeyMap.ShortHelp()
		if len(bindings) == 0 {
			t.Error("PickerKeyMap ShortHelp should return at least one binding")
		}
	})
}

func TestDoneKeyMap(t *testing.T) {
	t.Run("ShortHelp returns non-empty bindings", func(t *testing.T) {
		bindings := DoneKeyMap.ShortHelp()
		if len(bindings) == 0 {
			t.Error("DoneKeyMap ShortHelp should return at least one binding")
		}
	})
}

func TestKeyMapShortHelp(t *testing.T) {
	t.Run("empty key map returns empty bindings", func(t *testing.T) {
		var km KeyMap
		bindings := km.ShortHelp()
		if len(bindings) != 0 {
			t.Errorf("expected 0 bindings for empty key map, got %d", len(bindings))
		}
	})
}

func TestKeyMapFullHelp(t *testing.T) {
	t.Run("returns a single row of ShortHelp", func(t *testing.T) {
		rows := ListKeyMap.FullHelp()
		if len(rows) != 1 {
			t.Errorf("expected 1 row, got %d", len(rows))
		}
		if len(rows[0]) == 0 {
			t.Error("expected at least one binding in the row")
		}
	})
}
