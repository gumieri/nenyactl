package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestNewTheme(t *testing.T) {
	t.Run("returns a valid theme", func(t *testing.T) {
		theme := NewTheme()
		if theme.Title.Render("x") == "" {
			t.Error("theme.Title.Render should not be empty")
		}
		if theme.Body.Render("x") == "" {
			t.Error("theme.Body.Render should not be empty")
		}
		if theme.Success.Render("x") == "" {
			t.Error("theme.Success.Render should not be empty")
		}
		if theme.Error.Render("x") == "" {
			t.Error("theme.Error.Render should not be empty")
		}
		if theme.InputFocused.Render("x") == "" {
			t.Error("theme.InputFocused.Render should not be empty")
		}
		if theme.InputBlurred.Render("x") == "" {
			t.Error("theme.InputBlurred.Render should not be empty")
		}
	})
}

func TestCurrent(t *testing.T) {
	t.Run("returns the current theme", func(t *testing.T) {
		theme := Current()
		if theme.App.Render("x") == "" {
			t.Error("theme.App.Render should not be empty")
		}
	})
}

func TestDarkTheme(t *testing.T) {
	t.Run("dark theme has expected styles", func(t *testing.T) {
		theme := dark()
		if theme.Title.Render("x") == "" {
			t.Error("dark theme Title.Render should not be empty")
		}
		if theme.Success.Render("x") == "" {
			t.Error("dark theme Success.Render should not be empty")
		}
		if theme.Error.Render("x") == "" {
			t.Error("dark theme Error.Render should not be empty")
		}
	})
}

func TestLightTheme(t *testing.T) {
	t.Run("light theme has expected styles", func(t *testing.T) {
		theme := light()
		if theme.Title.Render("x") == "" {
			t.Error("light theme Title.Render should not be empty")
		}
		if theme.Success.Render("x") == "" {
			t.Error("light theme Success.Render should not be empty")
		}
		if theme.Error.Render("x") == "" {
			t.Error("light theme Error.Render should not be empty")
		}
	})
}

func TestMkTheme(t *testing.T) {
	t.Run("creates theme with all required fields", func(t *testing.T) {
		c := lipgloss.Color("0")
		base := lipgloss.NewStyle().Foreground(c)
		theme := mkTheme(base, c, c, c, c, c, c, c, c, c, c)
		if theme.Title.Render("x") == "" {
			t.Error("Title.Render should not be empty")
		}
		if theme.Body.Render("x") == "" {
			t.Error("Body.Render should not be empty")
		}
		if theme.Success.Render("x") == "" {
			t.Error("Success.Render should not be empty")
		}
		if theme.Error.Render("x") == "" {
			t.Error("Error.Render should not be empty")
		}
		if theme.App.Render("x") == "" {
			t.Error("App.Render should not be empty")
		}
	})
}
