//go:build !windows

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Non-Windows stubs for the process helpers in exec_windows.go, so the
*  usbipd wrapper compiles everywhere (the Workbench links it on every OS and
*  gates the feature on runtime.GOOS).
 */

package rfutils

import "os/exec"

// hideConsoleWindow is a no-op outside Windows.
func hideConsoleWindow(*exec.Cmd) {}

// runElevated is only implemented on Windows (UAC).
func runElevated(string, []string) (uint32, error) {
	return 0, ErrUSBElevationNotWindow
}
