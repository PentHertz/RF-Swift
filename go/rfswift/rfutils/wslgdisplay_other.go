//go:build !windows

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Non-Windows stubs for wslgdisplay.go; the entry points there return
*  ErrWSLgDisplayNotWindows before reaching these.
 */

package rfutils

func wslgClientPID() (int, error) { return 0, ErrWSLgDisplayNotWindows }

func terminateProcess(int) error { return ErrWSLgDisplayNotWindows }
