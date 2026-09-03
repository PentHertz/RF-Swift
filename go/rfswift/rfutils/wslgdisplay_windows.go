//go:build windows

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Process plumbing for wslgdisplay.go: find WSLg's msrdc.exe and stop it.
 */

package rfutils

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processImagePath returns the executable path of a process, "" when it
// cannot be read (a process of another user, or gone already).
func processImagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// wslgClientPID finds WSLg's display client: an msrdc.exe under the WSL
// installation directory. Another msrdc.exe (the Windows App / Azure Virtual
// Desktop client) is ignored; one whose path cannot be read is accepted only
// when no better candidate exists.
//
//	out(1): int pid, 0 when the client is not running
func wslgClientPID() (int, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, fmt.Errorf("listing processes: %w", err)
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	fallback := 0
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), wslgClientExe) {
			continue
		}
		path := strings.ToLower(processImagePath(entry.ProcessID))
		switch {
		case strings.Contains(path, wslgClientPathHint):
			return int(entry.ProcessID), nil
		case path == "" && fallback == 0:
			fallback = int(entry.ProcessID)
		}
	}
	return fallback, nil
}

// terminateProcess ends a process by id.
func terminateProcess(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}
