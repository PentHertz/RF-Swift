package dock

import (
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"syscall"
)

// getTerminalSize returns the current width and height of the terminal associated
// with the given file descriptor.
//
//	in(1): int fd file descriptor of the terminal to query
//	out: int width of the terminal in columns
//	out: int height of the terminal in rows
//	out: error non-nil if the terminal size could not be determined
func getTerminalSize(fd int) (int, int, error) {
	return terminal.GetSize(fd)
}

// syscallsigwin returns the OS signal used to notify processes of a terminal
// window size change (SIGWINCH on macOS/Darwin).
//
//	out: os.Signal the SIGWINCH signal indicating a terminal resize event
func syscallsigwin() os.Signal {
	return syscall.SIGWINCH
}
