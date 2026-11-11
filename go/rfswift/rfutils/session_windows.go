//go:build windows

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Windows counterpart of session_unix.go: there is no sudo and no per-user
*  audio session to switch to (containers use WSLg's PulseAudio socket).
 */

package rfutils

import (
	"os"
	"os/exec"
	"path/filepath"
)

func sessionRuntimeDir() string { return os.Getenv("XDG_RUNTIME_DIR") }

func sessionUID() int { return os.Getuid() }

func sessionCommand(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }

func sessionPulseSocket() string { return filepath.Join(sessionRuntimeDir(), "pulse", "native") }
