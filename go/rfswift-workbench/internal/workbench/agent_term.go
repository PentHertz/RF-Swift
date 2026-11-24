/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import "unicode/utf8"

// termCleaner turns a terminal output stream (PTY semantics: CR LF line ends,
// ANSI colour and cursor sequences, progress bars redrawn with a bare CR) into
// plain text an agent can read without paying for escape sequences. It is a
// small streaming state machine, so a sequence split across two reads is still
// removed. A bare CR followed by more text overwrites the current line, the way
// a terminal shows it, so a progress bar leaves its last state only. Lines are
// committed on LF; the current, unterminated line (an interactive prompt) is
// available through Partial and committed by Flush.
type termCleaner struct {
	state     int
	line      []byte
	pendingCR bool
	csiParams []byte
}

const (
	tcNormal = iota
	tcEsc
	tcCSI
	tcOSC
	tcOSCEsc
	tcString
	tcStringEsc
	tcCharset
)

// termLineLimit bounds one line so a stream without newlines cannot grow the
// buffer without bound; a longer line is committed as it stands.
const termLineLimit = 64 << 10

// Write consumes raw terminal output and returns the text of the lines it
// completed.
func (c *termCleaner) Write(p []byte) []byte {
	var out []byte
	for _, b := range p {
		switch c.state {
		case tcEsc:
			switch b {
			case '[':
				c.state, c.csiParams = tcCSI, c.csiParams[:0]
			case ']':
				c.state = tcOSC
			case 'P', 'X', '^', '_':
				c.state = tcString
			case '(', ')', '*', '+', '-', '.', '/', '#', '%':
				c.state = tcCharset
			default:
				c.state = tcNormal // two-byte escape (ESC 7, ESC =, ESC M, ...)
			}
			continue
		case tcCharset:
			c.state = tcNormal
			continue
		case tcCSI:
			if b >= 0x40 && b <= 0x7e {
				// "erase line" right after a CR (or of the whole line) clears it.
				if b == 'K' && (c.pendingCR || string(c.csiParams) == "2") {
					c.line, c.pendingCR = c.line[:0], false
				}
				c.state = tcNormal
			} else if len(c.csiParams) < 32 {
				c.csiParams = append(c.csiParams, b)
			}
			continue
		case tcOSC:
			if b == 0x07 {
				c.state = tcNormal
			} else if b == 0x1b {
				c.state = tcOSCEsc
			}
			continue
		case tcOSCEsc:
			c.state = tcNormal // ESC \ (string terminator) or a stray ESC
			continue
		case tcString:
			if b == 0x1b {
				c.state = tcStringEsc
			}
			continue
		case tcStringEsc:
			if b == '\\' {
				c.state = tcNormal
			} else {
				c.state = tcString
			}
			continue
		}
		// tcNormal
		switch {
		case b == 0x1b:
			c.state = tcEsc
		case b == '\n':
			out = append(out, c.line...)
			out = append(out, '\n')
			c.line, c.pendingCR = c.line[:0], false
		case b == '\r':
			c.pendingCR = true
		case b == '\b':
			if len(c.line) > 0 {
				_, size := utf8.DecodeLastRune(c.line)
				c.line = c.line[:len(c.line)-size]
			}
		case b == '\t' || b >= 0x20 && b != 0x7f:
			if c.pendingCR {
				c.line, c.pendingCR = c.line[:0], false
			}
			c.line = append(c.line, b)
			if len(c.line) >= termLineLimit {
				out = append(out, c.line...)
				out = append(out, '\n')
				c.line = c.line[:0]
			}
		default:
			// BEL, NUL, DEL and other control characters are dropped.
		}
	}
	return out
}

// Partial returns the current unterminated line without consuming it.
func (c *termCleaner) Partial() []byte {
	return append([]byte(nil), c.line...)
}

// Flush commits the current unterminated line (without adding a newline).
func (c *termCleaner) Flush() []byte {
	out := append([]byte(nil), c.line...)
	c.line, c.pendingCR = c.line[:0], false
	return out
}
