/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Confirm prompts the user with a yes/no question.
// Returns true if user confirms, false otherwise.
// Falls back to simple text prompt if not interactive.
func Confirm(message string) bool {
	if !IsInteractive() {
		return false
	}

	var result bool
	err := huh.NewConfirm().
		Title(message).
		Affirmative("Yes").
		Negative("No").
		Value(&result).
		Run()

	if err != nil {
		return false
	}
	return result
}

// SelectOne presents a selection menu and returns the chosen value.
func SelectOne(title string, options []string) (string, error) {
	if !IsInteractive() || len(options) == 0 {
		return "", fmt.Errorf("no interactive terminal or empty options")
	}

	opts := make([]huh.Option[string], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, o)
	}

	var result string
	err := huh.NewSelect[string]().
		Title(title).
		Options(opts...).
		Value(&result).
		Run()

	return result, err
}

// PromptInput prompts the user for a text value with a placeholder default.
func PromptInput(title string, placeholder string) (string, error) {
	if !IsInteractive() {
		return "", fmt.Errorf("no interactive terminal")
	}

	var result string
	err := huh.NewInput().
		Title(title).
		Placeholder(placeholder).
		Value(&result).
		Run()
	if err != nil {
		return "", err
	}
	if result == "" {
		result = placeholder
	}
	return result, nil
}

// PrintSuccess prints a green success message.
func PrintSuccess(msg string) {
	icon := lipgloss.NewStyle().Foreground(ColorSuccess).Render("✓")
	fmt.Printf("  %s %s\n", icon, msg)
}

// PrintWarning prints a yellow warning message.
func PrintWarning(msg string) {
	icon := lipgloss.NewStyle().Foreground(ColorWarning).Render("!")
	fmt.Printf("  %s %s\n", icon, msg)
}

// PrintError prints a red error message.
func PrintError(msg string) {
	icon := lipgloss.NewStyle().Foreground(ColorDanger).Render("✗")
	fmt.Printf("  %s %s\n", icon, msg)
}

// PrintInfo prints an informational message.
func PrintInfo(msg string) {
	icon := lipgloss.NewStyle().Foreground(ColorPrimary).Render("→")
	fmt.Printf("  %s %s\n", icon, msg)
}

// PrintRecap prints a recap box with key-value pairs.
func PrintRecap(title string, items map[string]string, keys []string) {
	titleStyled := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(title)
	fmt.Printf("\n%s\n", titleStyled)

	separator := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(strings.Repeat("─", 50))
	fmt.Println(separator)

	for _, k := range keys {
		v := items[k]
		label := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Width(20).
			Render(k)
		value := lipgloss.NewStyle().
			Foreground(ColorWhite).
			Render(v)
		fmt.Printf("  %s %s\n", label, value)
	}

	fmt.Println(separator)
}

// PrintCLIEquivalent prints the equivalent CLI command.
func PrintCLIEquivalent(cmd string) {
	label := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true).
		Render("Equivalent command:")
	command := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Render(cmd)
	fmt.Printf("\n  %s\n  %s\n\n", label, command)
}
