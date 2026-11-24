//go:build !windows

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import "syscall"

type groupSignal int

const (
	groupHangup groupSignal = iota
	groupTerminate
	groupKill
)

// signalProcessGroup sends sig to the process group led by pid (a PTY child
// is a session and group leader). It reports whether the group existed.
func signalProcessGroup(pid int, sig groupSignal) bool {
	s := syscall.SIGHUP
	switch sig {
	case groupTerminate:
		s = syscall.SIGTERM
	case groupKill:
		s = syscall.SIGKILL
	}
	return syscall.Kill(-pid, s) == nil
}
