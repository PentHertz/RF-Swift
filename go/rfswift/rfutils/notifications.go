package rfutils

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
	"os"
	"regexp"
)

// DisplayNotification renders a formatted notification box in the terminal with a
// title, message body, and visual style determined by the notification type.
//
//	in(1): string title        the heading displayed at the top of the box
//	in(2): string message      the body text; newlines produce multiple wrapped rows
//	in(3): string notificationType  style selector: "warning", "error", "info", or any other value for plain
func DisplayNotification(title string, message string, notificationType string) {
	// Get terminal width
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80 // Default width if unable to determine
	}

	// Adjust box width based on terminal width
	boxWidth := width - 4
	if boxWidth < 20 {
		boxWidth = 20
	} else if boxWidth > 100 {
		boxWidth = 100
	}

	var titleColor *color.Color
	var emoji string

	switch notificationType {
	case "warning":
		titleColor = color.New(color.FgYellow)
		emoji = "⚠️"
	case "error":
		titleColor = color.New(color.FgRed)
		emoji = "❌"
	case "info":
		titleColor = color.New(color.FgCyan)
		emoji = "ℹ️"
	default:
		titleColor = color.New(color.FgWhite)
		emoji = "📝"
	}

	// Print top border
	fmt.Printf("┌%s┐\n", strings.Repeat("─", boxWidth-2))

	// Print title
	titleLine := fmt.Sprintf(" %s %s", emoji, title)
	paddedTitle := padRight(titleLine, boxWidth-2)
	fmt.Print("│")
	titleColor.Print(paddedTitle)
	fmt.Println("│")

	// Print separator
	fmt.Printf("├%s┤\n", strings.Repeat("─", boxWidth-2))

	// Print message
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		wrappedLines := wrapText(line, boxWidth-4)
		for _, wrappedLine := range wrappedLines {
			paddedLine := padRight(wrappedLine, boxWidth-4)
			fmt.Printf("│ %s │\n", paddedLine)
		}
	}

	// Print bottom border
	fmt.Printf("└%s┘\n", strings.Repeat("─", boxWidth-2))
}

// padRight pads the string s on the right with spaces until its visible
// (ANSI-stripped) rune width reaches width characters.
//
//	in(1): string s      the string to pad (may contain ANSI escape codes)
//	in(2): int    width  the target visible character width
//	out: string   the padded string with trailing spaces appended as needed
func padRight(s string, width int) string {
	padWidth := width - runewidth.StringWidth(stripAnsi(s))
	if padWidth < 0 {
		padWidth = 0
	}
	return s + strings.Repeat(" ", padWidth)
}

// wrapText splits text into lines that do not exceed width visible characters,
// word-wrapping at space boundaries and expanding tabs to four spaces.
//
//	in(1): string text   the input text to wrap
//	in(2): int    width  maximum visible character width per output line
//	out: []string  slice of lines, each fitting within width visible characters
func wrapText(text string, width int) []string {
	var lines []string
	words := strings.Fields(strings.ReplaceAll(text, "\t", "    "))
	currentLine := ""

	for _, word := range words {
		if runewidth.StringWidth(stripAnsi(currentLine))+runewidth.StringWidth(stripAnsi(word))+1 <= width {
			if currentLine != "" {
				currentLine += " "
			}
			currentLine += word
		} else {
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

// stripAnsi removes all ANSI color/style escape sequences from str, returning
// the plain text content.
//
//	in(1): string str  the string that may contain ANSI escape codes
//	out: string  the input string with all ANSI escape sequences removed
func stripAnsi(str string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(str, "")
}
