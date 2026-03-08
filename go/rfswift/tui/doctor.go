/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// DoctorResult represents a single diagnostic check result.
type DoctorResult struct {
	Name    string
	Status  string // "ok", "warn", "fail", "skip"
	Message string
}

// DoctorSummary holds pass/warn/fail counts.
type DoctorSummary struct {
	Pass int
	Warn int
	Fail int
}

// PrintDoctorHeader prints the doctor command header.
func PrintDoctorHeader() {
	header := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		Render("🩺 RF Swift Doctor")
	separator := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Render("══════════════════════════════════════════════════════════")
	fmt.Printf("\n%s\n%s\n\n", header, separator)
}

// PrintDoctorResult prints a single doctor check result line.
func PrintDoctorResult(r DoctorResult) {
	icon := DoctorStatusIcons[r.Status]
	color, ok := DoctorStatusColors[r.Status]
	if !ok {
		color = ColorWhite
	}

	styled := lipgloss.NewStyle().Foreground(color).Render(icon)
	fmt.Printf("  %s  %-30s %s\n", styled, r.Name, r.Message)
}

// PrintDoctorSummary prints the summary line at the bottom.
func PrintDoctorSummary(s DoctorSummary) {
	separator := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Render("──────────────────────────────────────────────────────────")
	fmt.Printf("\n%s\n", separator)

	passText := lipgloss.NewStyle().Foreground(ColorSuccess).Render(fmt.Sprintf("%d passed", s.Pass))
	fmt.Printf("  %s", passText)

	if s.Warn > 0 {
		warnText := lipgloss.NewStyle().Foreground(ColorWarning).Render(fmt.Sprintf("%d warnings", s.Warn))
		fmt.Printf("  %s", warnText)
	}
	if s.Fail > 0 {
		failText := lipgloss.NewStyle().Foreground(ColorDanger).Render(fmt.Sprintf("%d failed", s.Fail))
		fmt.Printf("  %s", failText)
	}
	fmt.Printf("\n\n")
}
