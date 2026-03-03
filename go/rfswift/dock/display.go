/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Table rendering and terminal display utilities
 *
 * setTerminalTitle              - in(1): string title
 * wrapText                      - in(1): string text, in(2): int maxWidth, out: string
 * formatVersionsMultiLine       - in(1): []string versions, in(2): int maxPerLine, in(3): int maxWidth, out: []string
 * printTableWithMultiLineSupport- in(1): []string headers, in(2): [][]interface{} rows, in(3): []int columnWidths, in(4): string title, in(5): string titleColor
 * getColumnColor                - in(1): int colIdx, in(2): string content, in(3): int totalCols, out: string
 * printRowWithColorAndVersion   - in(1): []string row, in(2): []int columnWidths, in(3): string separator, in(4): bool showVersions
 * printRowWithColor             - in(1): []string row, in(2): []int columnWidths, in(3): string separator
 * stripAnsiCodes                - in(1): string s, out: string
 * distributeColumnWidths        - in(1): int availableWidth, in(2): []int columnWidths, out: []int
 * max                           - in(1): int a, in(2): int b, out: int
 * min                           - in(1): int a, in(2): int b, out: int
 */
package dock

import (
	"fmt"
	"regexp"
	"strings"
)

// setTerminalTitle sets the terminal window title via ANSI escape sequence.
func setTerminalTitle(title string) {
	fmt.Printf("\033]0;%s\007", title)
}

// wrapText wraps text to fit within maxWidth characters per line.
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

// formatVersionsMultiLine formats version strings into multiple lines.
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

// printTableWithMultiLineSupport prints a table where cells can have multiple lines.
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

// getColumnColor returns the ANSI color for a specific column value based on status keywords.
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

// printRowWithColorAndVersion prints a row with status and version column coloring.
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

// printRowWithColor prints a row with status column coloring.
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

// stripAnsiCodes removes ANSI escape codes from a string.
func stripAnsiCodes(s string) string {
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansi.ReplaceAllString(s, "")
}

// distributeColumnWidths proportionally distributes available width among columns.
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
