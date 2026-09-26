/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package tui

import (
	"fmt"
	"os"

	huh "charm.land/huh/v2"
	"golang.org/x/term"
)

// multiSelectReservedRows keeps room under a list for huh's key help and the
// shell prompt line.
const multiSelectReservedRows = 3

// MultiSelect builds a multi-select whose options are all visible. Left
// without a height, huh v2.0.3 sizes the options area to the options' height
// minus the title line, so the last option stays hidden until the cursor
// reaches it (VPN at the bottom of "Enable features"). The list is capped to
// the terminal; when some options remain out of view, the title says so.
// description may be empty.
func MultiSelect[T comparable](title, description string, options ...huh.Option[T]) *huh.MultiSelect[T] {
	header := 1 // title
	if description != "" {
		header++
	}
	visible := len(options)
	if _, rows, err := term.GetSize(int(os.Stdout.Fd())); err == nil && rows > 0 {
		if room := rows - header - multiSelectReservedRows; room < visible {
			visible = max(room, 3)
		}
	}
	if visible < len(options) {
		title = fmt.Sprintf("%s (%d options, ↑/↓ to see all)", title, len(options))
	}
	m := huh.NewMultiSelect[T]().Title(title).Options(options...).Height(header + visible)
	if description != "" {
		m = m.Description(description)
	}
	return m
}
