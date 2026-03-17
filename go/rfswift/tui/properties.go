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

// PropertyItem represents a single key-value pair in a property sheet.
type PropertyItem struct {
	Key        string
	Value      string
	ValueColor lipgloss.Color // optional override for value color
}

// RenderPropertySheet prints a two-column key-value table (property sheet).
func RenderPropertySheet(title string, titleColor lipgloss.Color, items []PropertyItem) {
	if len(items) == 0 {
		return
	}

	width := TerminalWidth()

	// Build rows
	rows := make([][]string, len(items))
	colorMap := make(map[int]lipgloss.Color)
	for i, item := range items {
		rows[i] = []string{item.Key, item.Value}
		if item.ValueColor != lipgloss.Color("") {
			colorMap[i] = item.ValueColor
		}
	}

	t := table.New().
		Headers("Property", "Value").
		Rows(rows...).
		Border(lipgloss.RoundedBorder()).
		BorderRow(true).
		Width(width).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)

			// Header
			if row == table.HeaderRow {
				return s.Bold(true).Foreground(ColorWhite)
			}

			// Key column
			if col == 0 {
				return s.Foreground(ColorWhite).Bold(true)
			}

			// Value column — apply custom color if set
			if c, ok := colorMap[row]; ok {
				return s.Foreground(c)
			}

			return s
		}).
		BorderStyle(BorderStyle)

	// Print title
	if title != "" {
		if titleColor == lipgloss.Color("") {
			titleColor = ColorPrimary
		}
		styled := lipgloss.NewStyle().
			Foreground(titleColor).
			Bold(true).
			Padding(0, 1).
			Render(title)
		fmt.Println(styled)
	}

	fmt.Println(t.Render())
}

// WrapText wraps text to fit within maxWidth characters per line.
func WrapText(text string, maxWidth int) string {
	var result strings.Builder
	currentLineWidth := 0

	words := strings.Fields(text)
	for i, word := range words {
		if currentLineWidth+len(word) > maxWidth {
			if currentLineWidth > 0 {
				result.WriteString("\n")
				currentLineWidth = 0
			}
			if len(word) > maxWidth {
				for len(word) > maxWidth {
					result.WriteString(word[:maxWidth] + "\n")
					word = word[maxWidth:]
				}
			}
		}
		result.WriteString(word)
		currentLineWidth += len(word)
		if i < len(words)-1 && currentLineWidth+1+len(words[i+1]) <= maxWidth {
			result.WriteString(" ")
			currentLineWidth++
		}
	}

	return result.String()
}
