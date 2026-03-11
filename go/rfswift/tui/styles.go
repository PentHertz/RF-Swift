/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Theme colors used throughout the TUI.
var (
	ColorPrimary = lipgloss.Color("#00BFFF") // cyan/blue
	ColorSuccess = lipgloss.Color("#00FF00") // green
	ColorWarning = lipgloss.Color("#FFAA00") // yellow/orange
	ColorDanger  = lipgloss.Color("#FF4444") // red
	ColorMuted   = lipgloss.Color("#666666") // gray
	ColorWhite   = lipgloss.Color("#FFFFFF")
	ColorPink    = lipgloss.Color("#FF69B4")
	ColorCyan    = lipgloss.Color("#00FFFF")
)

// Status keyword to color mapping.
var StatusColors = map[string]lipgloss.Color{
	"Up to date": ColorSuccess,
	"Obsolete":   ColorDanger,
	"Custom":     ColorWarning,
	"No network": ColorWarning,
	"Error":      ColorDanger,
}

// Doctor status to color mapping.
var DoctorStatusColors = map[string]lipgloss.Color{
	"ok":   ColorSuccess,
	"warn": ColorWarning,
	"fail": ColorDanger,
	"skip": ColorMuted,
}

// DoctorStatusIcons maps doctor status to display icons.
var DoctorStatusIcons = map[string]string{
	"ok":   "✓",
	"warn": "!",
	"fail": "✗",
	"skip": "-",
}

// Common styles.
var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite)

	BorderStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)
)

// TerminalWidth returns the current terminal width, defaulting to 80.
func TerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// IsInteractive returns true if stdout is a terminal (not piped).
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
