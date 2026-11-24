//go:build windows

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

type groupSignal int

const (
	groupHangup groupSignal = iota
	groupTerminate
	groupKill
)

// signalProcessGroup has no Windows equivalent: closing the ConPTY ends the
// process tree, which is what the callers fall back to when this returns false.
func signalProcessGroup(pid int, sig groupSignal) bool { return false }
