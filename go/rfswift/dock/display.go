/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package dock

import (
	"fmt"
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
