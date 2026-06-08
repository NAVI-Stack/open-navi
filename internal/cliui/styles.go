package cliui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Brand Colors (Cyan/Teal theme)
	AccentColor  = lipgloss.Color("#00d7d7") // Cyan
	DimColor     = lipgloss.Color("#555555")
	ErrorColor   = lipgloss.Color("#ff0000")
	SuccessColor = lipgloss.Color("#00ff00")
	WarnColor    = lipgloss.Color("#ffcc00")

	// Base Styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(AccentColor).
			Bold(true).
			MarginLeft(1).
			MarginBottom(1)

	DimStyle = lipgloss.NewStyle().
			Foreground(DimColor)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ErrorColor).
			Bold(true)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(SuccessColor)

	// Box/Banner Styles
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(AccentColor).
			Padding(0, 2).
			MarginBottom(1)

	// List/Item Styles
	ItemStyle = lipgloss.NewStyle().
			MarginLeft(2)

	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(AccentColor).
				Bold(true).
				MarginLeft(1)
)

func MaskSecret(secret string) string {
	if len(secret) <= 8 {
		return "********"
	}
	return secret[:3] + "..." + secret[len(secret)-4:]
}
