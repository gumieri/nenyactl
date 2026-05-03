package cmd

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)