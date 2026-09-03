//go:build windows

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Windows process helpers: run console tools without flashing a console
*  window (the Workbench is a GUI subsystem binary) and run a program with an
*  elevated token through the standard UAC consent prompt.
 */

package rfutils

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// hideConsoleWindow keeps a console child process from opening a visible
// console window when the parent has none (GUI apps).
//
//	in(1): *exec.Cmd cmd the command to adjust before Start/Run
func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// HideConsoleWindow is hideConsoleWindow for the other packages: a GUI
// process (the Workbench) that captures a console child's output would
// otherwise get an empty terminal window on the desktop for every wsl.exe it
// runs.
func HideConsoleWindow(cmd *exec.Cmd) { hideConsoleWindow(cmd) }

// shellExecuteInfo mirrors SHELLEXECUTEINFOW (shellapi.h). Field order and
// types match the C layout so Go applies the same alignment.
type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           windows.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       windows.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      windows.Handle
	dwHotKey       uint32
	hIconOrMonitor windows.Handle
	hProcess       windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
	swHide                = 0
)

var (
	modshell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = modshell32.NewProc("ShellExecuteExW")
)

// runElevated starts exe with args through the "runas" verb, which shows the
// UAC consent prompt for that executable, waits for it to finish and returns
// its exit code. The prompt names the program directly (no cmd.exe or
// PowerShell wrapper), and arguments are quoted individually, so nothing the
// user sees in the consent dialog differs from what runs.
//
//	in(1): string exe absolute path of the program to elevate
//	in(2): []string args program arguments
//	out(1): uint32 exit code of the elevated process
//	out(2): error ErrUSBElevationDeclined when the prompt is cancelled
func runElevated(exe string, args []string) (uint32, error) {
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, syscall.EscapeArg(a))
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return 0, err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	params, err := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil {
		return 0, err
	}

	// ShellExecuteEx wants a COM apartment on the calling thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
		defer windows.CoUninitialize()
	}

	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		nShow:        swHide,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	ret, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		var errno syscall.Errno
		if errors.As(callErr, &errno) && errno == windows.ERROR_CANCELLED {
			return 0, ErrUSBElevationDeclined
		}
		return 0, fmt.Errorf("elevation request failed: %v", callErr)
	}
	if info.hProcess == 0 {
		return 0, fmt.Errorf("elevation request returned no process handle")
	}
	defer windows.CloseHandle(info.hProcess)
	if _, err := windows.WaitForSingleObject(info.hProcess, windows.INFINITE); err != nil {
		return 0, fmt.Errorf("waiting for elevated process: %w", err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return 0, fmt.Errorf("reading elevated process exit code: %w", err)
	}
	return code, nil
}
