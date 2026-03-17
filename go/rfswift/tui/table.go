/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// TableConfig holds options for rendering a table.
type TableConfig struct {
	Title      string
	TitleColor lipgloss.Color
	Headers    []string
	Rows       [][]string
	// ColorFunc optionally applies per-cell coloring. Receives row index
	// (0-based, -1 for header), column index, and cell content.
	// Return a zero-value Color to skip coloring.
	ColorFunc func(row, col int, content string) lipgloss.Color
	// BorderRow enables row separators between data rows.
	BorderRow bool
}

// RenderTable prints a styled table to stdout using lipgloss/table.
func RenderTable(cfg TableConfig) {
	if len(cfg.Headers) == 0 {
		return
	}

	width := TerminalWidth()

	t := table.New().
		Headers(cfg.Headers...).
		Rows(cfg.Rows...).
		Border(lipgloss.NormalBorder()).
		BorderRow(cfg.BorderRow).
		Width(width).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)

			// Header row
			if row == table.HeaderRow {
				return s.Bold(true).Foreground(ColorWhite)
			}

			// Apply custom color function
			if cfg.ColorFunc != nil && row < len(cfg.Rows) && col < len(cfg.Rows[row]) {
				c := cfg.ColorFunc(row, col, cfg.Rows[row][col])
				if c != lipgloss.Color("") {
					return s.Foreground(c)
				}
			}

			return s
		}).
		BorderStyle(BorderStyle)

	// Print title
	if cfg.Title != "" {
		titleColor := cfg.TitleColor
		if titleColor == lipgloss.Color("") {
			titleColor = ColorPrimary
		}
		styled := lipgloss.NewStyle().
			Foreground(titleColor).
			Bold(true).
			Padding(0, 1).
			Render(cfg.Title)
		fmt.Println(styled)
	}

	fmt.Println(t.Render())
}

// StatusColorFunc returns a ColorFunc that colors cells based on status keywords.
// statusCol is the column index containing status values.
func StatusColorFunc(statusCol int) func(row, col int, content string) lipgloss.Color {
	return func(row, col int, content string) lipgloss.Color {
		if col == statusCol {
			if c, ok := StatusColors[content]; ok {
				return c
			}
		}
		// Color version-like strings in cyan
		if strings.HasPrefix(content, "v") || (strings.Contains(content, ".") && content != "-") {
			return ColorCyan
		}
		return lipgloss.Color("")
	}
}

// ImageTableColorFunc returns a ColorFunc for the images table with optional version column.
func ImageTableColorFunc(statusCol int, versionCol int, showVersions bool) func(row, col int, content string) lipgloss.Color {
	return func(row, col int, content string) lipgloss.Color {
		if col == statusCol {
			if c, ok := StatusColors[content]; ok {
				return c
			}
		}
		if showVersions && col == versionCol && content != "-" && content != "" {
			return ColorCyan
		}
		return lipgloss.Color("")
	}
}
