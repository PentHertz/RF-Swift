package dock

import (
	"golang.org/x/sys/windows"
	"os"
)

// getTerminalSize returns the current width and height of the Windows console
// window by querying the standard output handle's screen buffer info. The fd
// parameter is accepted for interface compatibility but is unused on Windows.
//
//	in(1): int fd file descriptor (unused on Windows; kept for cross-platform interface compatibility)
//	out: int width of the console window in columns
//	out: int height of the console window in rows
//	out: error non-nil if the standard output handle or screen buffer info could not be retrieved
func getTerminalSize(fd int) (int, int, error) {
	hOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return 0, 0, err
	}

	var info windows.ConsoleScreenBufferInfo
	err = windows.GetConsoleScreenBufferInfo(hOut, &info)
	if err != nil {
		return 0, 0, err
	}

	width := int(info.Window.Right - info.Window.Left + 1)
	height := int(info.Window.Bottom - info.Window.Top + 1)
	return width, height, nil
}

// syscallsigwin returns the OS signal used to notify processes of a terminal
// window size change. Windows has no SIGWINCH equivalent, so nil is returned.
//
//	out: os.Signal always nil on Windows as no terminal resize signal exists
func syscallsigwin() os.Signal {
	return nil // No signal equivalent for Windows
}
