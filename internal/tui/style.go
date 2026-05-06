package tui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Title     lipgloss.Style
	Subtitle  lipgloss.Style
	Body      lipgloss.Style
	Dimmed    lipgloss.Style
	Highlight lipgloss.Style
	Accent    lipgloss.Style
	Success   lipgloss.Style
	Warning   lipgloss.Style
	Error     lipgloss.Style

	Cursor      lipgloss.Style
	SelectedRow lipgloss.Style
	Scrollbar   lipgloss.Style

	Checked   lipgloss.Style
	Unchecked lipgloss.Style
	RadioOn   lipgloss.Style
	RadioOff  lipgloss.Style

	InputFocused lipgloss.Style
	InputBlurred lipgloss.Style

	Border          lipgloss.Border
	BorderColor     lipgloss.Style
	BorderFocused   lipgloss.Border
	BorderColorFoc  lipgloss.Style

	App lipgloss.Style
}

var current Theme

func init() {
	current = NewTheme()
}

func Current() Theme {
	return current
}

func NewTheme() Theme {
	if lipgloss.HasDarkBackground() {
		return dark()
	}
	return light()
}

func dark() Theme {
	bg := lipgloss.Color("#1a1b26")
	fg := lipgloss.Color("#a9b1d6")
	blue := lipgloss.Color("#7aa2f7")
	purple := lipgloss.Color("#bb9af7")
	cyan := lipgloss.Color("#7dcfff")
	green := lipgloss.Color("#9ece6a")
	orange := lipgloss.Color("#e0af68")
	red := lipgloss.Color("#f7768e")
	gray := lipgloss.Color("#565f89")
	selBg := lipgloss.Color("#2f3346")

	base := lipgloss.NewStyle().Foreground(fg)

	return mkTheme(base, bg, fg, blue, purple, cyan, green, orange, red, gray, selBg)
}

func light() Theme {
	bg := lipgloss.Color("#d5d6db")
	fg := lipgloss.Color("#343b58")
	blue := lipgloss.Color("#2e7de9")
	purple := lipgloss.Color("#8c43d1")
	cyan := lipgloss.Color("#0f73b6")
	green := lipgloss.Color("#587539")
	orange := lipgloss.Color("#b47109")
	red := lipgloss.Color("#c64343")
	gray := lipgloss.Color("#9699a3")
	selBg := lipgloss.Color("#c0c1c7")

	base := lipgloss.NewStyle().Foreground(fg)

	return mkTheme(base, bg, fg, blue, purple, cyan, green, orange, red, gray, selBg)
}

func mkTheme(base lipgloss.Style, bg, fg, blue, purple, cyan, green, orange, red, gray, selBg lipgloss.Color) Theme {
	return Theme{
		Title:     base.Bold(true).Foreground(blue),
		Subtitle:  base.Bold(true).Foreground(purple),
		Body:      base,
		Dimmed:    base.Foreground(gray),
		Highlight: base.Background(selBg),
		Accent:    base.Foreground(cyan),
		Success:   base.Foreground(green),
		Warning:   base.Foreground(orange),
		Error:     base.Foreground(red),

		Cursor:      base.Bold(true).Foreground(cyan),
		SelectedRow: lipgloss.NewStyle().Background(selBg).Foreground(fg),
		Scrollbar:   lipgloss.NewStyle().Background(gray).Foreground(gray),

		Checked:   lipgloss.NewStyle().Foreground(green),
		Unchecked: base.Foreground(gray),
		RadioOn:   lipgloss.NewStyle().Foreground(blue),
		RadioOff:  base.Foreground(gray),

		InputFocused: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(blue),
		InputBlurred: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(gray),

		Border:         lipgloss.RoundedBorder(),
		BorderColor:    base.Border(lipgloss.RoundedBorder()).BorderForeground(gray),
		BorderFocused:  lipgloss.RoundedBorder(),
		BorderColorFoc: base.Border(lipgloss.RoundedBorder()).BorderForeground(blue),

		App: base.Background(bg),
	}
}
