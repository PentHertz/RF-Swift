/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package dock

import (
	"fmt"
	"regexp"
	"strings"
)

// setTerminalTitle sets the terminal window title via ANSI escape sequence.
//
//	in(1): string title - the text to display in the terminal window title bar
func setTerminalTitle(title string) {
	fmt.Printf("\033]0;%s\007", title)
}

// wrapText wraps text to fit within maxWidth characters per line, breaking on
// word boundaries and splitting words that exceed the maximum width.
//
//	in(1): string text     - the input text to wrap
//	in(2): int maxWidth    - the maximum number of characters allowed per line
//	out:   string          - the wrapped text with newlines inserted as needed
func wrapText(text string, maxWidth int) string {
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

// formatVersionsMultiLine formats a slice of version strings into display lines,
// grouping entries by a per-line count limit and an optional character width limit.
// Returns []string{"-"} when versions is empty or all entries are exhausted without output.
//
//	in(1): []string versions  - the list of version strings to format
//	in(2): int maxPerLine     - maximum number of version entries allowed on a single line
//	in(3): int maxWidth       - maximum character width of a line (0 disables width limiting)
//	out:   []string           - lines of comma-separated version groups, or ["-"] if empty
func formatVersionsMultiLine(versions []string, maxPerLine int, maxWidth int) []string {
	if len(versions) == 0 {
		return []string{"-"}
	}

	var lines []string
	var currentLine strings.Builder
	countOnLine := 0

	for i, v := range versions {
		separator := ""
		if countOnLine > 0 {
			separator = ", "
		}

		testLen := currentLine.Len() + len(separator) + len(v)

		if countOnLine >= maxPerLine || (maxWidth > 0 && testLen > maxWidth) {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
			}
			currentLine.Reset()
			countOnLine = 0
			separator = ""
		}

		if countOnLine > 0 {
			currentLine.WriteString(", ")
		}
		currentLine.WriteString(v)
		countOnLine++

		if i == len(versions)-1 && currentLine.Len() > 0 {
			lines = append(lines, currentLine.String())
		}
	}

	if len(lines) == 0 {
		return []string{"-"}
	}

	return lines
}

// printTableWithMultiLineSupport prints a formatted Unicode box-drawing table to stdout,
// supporting cells that span multiple lines (e.g. cells containing []string values).
// A colored title is printed above the table border.
//
//	in(1): []string headers          - column header labels
//	in(2): [][]interface{} rows      - table rows; each cell may be a string, []string, or any fmt.Sprintf-able value
//	in(3): []int columnWidths        - display width (in characters) for each column
//	in(4): string title              - text to display as the table title
//	in(5): string titleColor         - ANSI escape code string used to color the title
func printTableWithMultiLineSupport(headers []string, rows [][]interface{}, columnWidths []int, title string, titleColor string) {
	white := "\033[37m"
	reset := "\033[0m"

	totalWidth := 1
	for _, w := range columnWidths {
		totalWidth += w + 3
	}

	fmt.Printf("%s%s%s%s%s\n", titleColor, strings.Repeat(" ", 2), title, strings.Repeat(" ", totalWidth-2-len(title)), reset)
	fmt.Print(white)

	printHorizontalBorder(columnWidths, "┌", "┬", "┐")

	headerStrings := make([]string, len(headers))
	for i, h := range headers {
		headerStrings[i] = h
	}
	printRow(headerStrings, columnWidths, "│")
	printHorizontalBorder(columnWidths, "├", "┼", "┤")

	for rowIdx, row := range rows {
		cellLines := make([][]string, len(row))
		maxLines := 1

		for colIdx, cell := range row {
			switch v := cell.(type) {
			case string:
				cellLines[colIdx] = []string{v}
			case []string:
				if len(v) == 0 {
					cellLines[colIdx] = []string{""}
				} else {
					cellLines[colIdx] = v
				}
			default:
				cellLines[colIdx] = []string{fmt.Sprintf("%v", v)}
			}
			if len(cellLines[colIdx]) > maxLines {
				maxLines = len(cellLines[colIdx])
			}
		}

		for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
			fmt.Print("│")
			for colIdx, lines := range cellLines {
				content := ""
				if lineIdx < len(lines) {
					content = lines[lineIdx]
				}

				color := getColumnColor(colIdx, content, len(row))

				if color != "" {
					fmt.Printf(" %s%-*s%s ", color, columnWidths[colIdx], truncateString(content, columnWidths[colIdx]), reset)
				} else {
					fmt.Printf(" %-*s ", columnWidths[colIdx], truncateString(content, columnWidths[colIdx]))
				}
				fmt.Print("│")
			}
			fmt.Println()
		}

		if rowIdx < len(rows)-1 {
			printHorizontalBorder(columnWidths, "├", "┼", "┤")
		}
	}

	printHorizontalBorder(columnWidths, "└", "┴", "┘")
	fmt.Print(reset)
	fmt.Println()
}

// getColumnColor returns the ANSI color escape code to apply to a cell value,
// selecting colors based on known status keywords or version-string heuristics.
// Returns an empty string when no special coloring applies.
//
//	in(1): int colIdx      - zero-based index of the column being rendered
//	in(2): string content  - the cell content to evaluate for color selection
//	in(3): int totalCols   - total number of columns in the row (reserved for future use)
//	out:   string          - ANSI color escape code, or "" if no coloring should be applied
func getColumnColor(colIdx int, content string, totalCols int) string {
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	cyan := "\033[36m"

	statusKeywords := map[string]string{
		"Up to date": green,
		"Obsolete":   red,
		"Custom":     yellow,
		"No network": yellow,
		"Error":      red,
	}

	if color, ok := statusKeywords[content]; ok {
		return color
	}

	if strings.HasPrefix(content, "v") || strings.Contains(content, ".") {
		if len(content) > 0 && content != "-" {
			return cyan
		}
	}

	return ""
}

// printRowWithColorAndVersion prints a single table row to stdout, applying ANSI
// color codes to the status column (index 5) and optionally to the version column
// (index 6) when showVersions is true.
//
//	in(1): []string row          - cell values for the row
//	in(2): []int columnWidths    - display width (in characters) for each column
//	in(3): string separator      - border character printed between cells (e.g. "│")
//	in(4): bool showVersions     - when true, the version column (index 6) is colored cyan
func printRowWithColorAndVersion(row []string, columnWidths []int, separator string, showVersions bool) {
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	cyan := "\033[36m"
	reset := "\033[0m"

	fmt.Print(separator)
	for i, col := range row {
		color := ""

		if i == 5 {
			switch col {
			case "Custom", "No network":
				color = yellow
			case "Up to date":
				color = green
			case "Obsolete", "Error":
				color = red
			}
		} else if showVersions && i == 6 && col != "-" {
			color = cyan
		}

		if color != "" {
			fmt.Printf(" %s%-*s%s ", color, columnWidths[i], truncateString(col, columnWidths[i]), reset)
		} else {
			fmt.Printf(" %-*s ", columnWidths[i], truncateString(col, columnWidths[i]))
		}
		fmt.Print(separator)
	}
	fmt.Println()
}

// printRowWithColor prints a single table row to stdout, applying ANSI color codes
// to the last column based on its status keyword value (Custom, Up to date, Obsolete, Error).
//
//	in(1): []string row          - cell values for the row
//	in(2): []int columnWidths    - display width (in characters) for each column
//	in(3): string separator      - border character printed between cells (e.g. "│")
func printRowWithColor(row []string, columnWidths []int, separator string) {
	fmt.Print(separator)
	for i, col := range row {
		if i == len(row)-1 {
			status := col
			color := ""
			switch status {
			case "Custom":
				color = "\033[33m"
			case "Up to date":
				color = "\033[32m"
			case "Obsolete":
				color = "\033[31m"
			case "Error":
				color = "\033[31m"
			}
			fmt.Printf(" %s%-*s\033[0m ", color, columnWidths[i], status)
		} else {
			fmt.Printf(" %-*s ", columnWidths[i], truncateString(col, columnWidths[i]))
		}
		fmt.Print(separator)
	}
	fmt.Println()
}

// stripAnsiCodes removes all ANSI SGR escape sequences from a string,
// returning the plain text without any color or formatting codes.
//
//	in(1): string s  - the input string potentially containing ANSI escape codes
//	out:   string    - the input string with all ANSI escape sequences removed
func stripAnsiCodes(s string) string {
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansi.ReplaceAllString(s, "")
}

// distributeColumnWidths proportionally redistributes column widths to fit within
// availableWidth, scaling each column relative to its share of the current total.
// Columns are guaranteed a minimum width of 1 character. The input slice is modified
// in place and also returned.
//
//	in(1): int availableWidth      - the total character width to distribute across all columns
//	in(2): []int columnWidths      - current column widths used as proportional weights
//	out:   []int                   - the updated columnWidths slice scaled to availableWidth
func distributeColumnWidths(availableWidth int, columnWidths []int) []int {
	totalCurrentWidth := 0
	for _, width := range columnWidths {
		totalCurrentWidth += width
	}
	for i := range columnWidths {
		columnWidths[i] = int(float64(columnWidths[i]) / float64(totalCurrentWidth) * float64(availableWidth))
		if columnWidths[i] < 1 {
			columnWidths[i] = 1
		}
	}
	return columnWidths
}

// max returns the larger of two integers.
//
//	in(1): int a  - first integer operand
//	in(2): int b  - second integer operand
//	out:   int    - the greater of a and b
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min returns the smaller of two integers.
//
//	in(1): int a  - first integer operand
//	in(2): int b  - second integer operand
//	out:   int    - the lesser of a and b
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
