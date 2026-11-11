//go:build !windows

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Desktop-session commands under sudo.
*
*  The audio server (PipeWire / PulseAudio) is a per-user session daemon: its
*  socket lives in the user's XDG_RUNTIME_DIR (/run/user/<uid>), and pactl,
*  `systemctl --user`, `pulseaudio --check` and pw-cli all look for it there.
*  Run as root through sudo they reach root's session instead, which has no
*  audio server: pactl fails with "Connection refused", the TCP module that
*  containers play through is never loaded, and the start-up fallbacks would
*  even launch a second audio server as root. A rootful engine is the usual
*  way to reach USB devices from containers (sudo rfswift with Podman, on
*  Qubes OS in particular), so every such run lost its sound.
*
*  sessionCommand runs those programs as the user who ran sudo, with that
*  user's runtime directory, home and session bus, so they talk to the audio
*  server the desktop actually uses. Outside sudo it is plain exec.Command.
 */

package rfutils

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	common "penthertz/rfswift/common"
)

// sessionRuntimeDir is the XDG runtime directory of the desktop session RF
// Swift acts for: the sudo caller's /run/user/<uid> when elevated, this
// process's XDG_RUNTIME_DIR (or /run/user/<uid>) otherwise.
func sessionRuntimeDir() string {
	if uid, _, ok := common.InvokingIDs(); ok && runtime.GOOS == "linux" {
		return fmt.Sprintf("/run/user/%d", uid)
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return fmt.Sprintf("/run/user/%d", os.Getuid())
}

// sessionUID is the uid owning the desktop session (the sudo caller when
// elevated).
func sessionUID() int {
	if uid, _, ok := common.InvokingIDs(); ok && runtime.GOOS == "linux" {
		return uid
	}
	return os.Getuid()
}

// sessionEnv rewrites an environment for a program run in the session of the
// given user: their HOME/USER/LOGNAME, runtime directory and session bus, and
// none of sudo's own variables (pactl and systemctl --user otherwise keep
// looking at root's session).
//
//	in(1): []string env      the environment to start from (os.Environ())
//	in(2): int uid           session owner
//	in(3): string name       session owner's account name
//	in(4): string home       session owner's home directory
//	out: []string
func sessionEnv(env []string, uid int, name, home string) []string {
	runtimeDir := fmt.Sprintf("/run/user/%d", uid)
	override := map[string]string{
		"HOME":                     home,
		"USER":                     name,
		"LOGNAME":                  name,
		"XDG_RUNTIME_DIR":          runtimeDir,
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=" + filepath.Join(runtimeDir, "bus"),
	}
	drop := map[string]bool{
		"SUDO_USER": true, "SUDO_UID": true, "SUDO_GID": true, "SUDO_COMMAND": true, "SUDO_HOME": true,
		"MAIL": true, "PULSE_RUNTIME_PATH": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true,
		"XDG_STATE_HOME": true, "XDG_CACHE_HOME": true,
	}
	out := make([]string, 0, len(env)+len(override))
	for _, kv := range env {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if _, replaced := override[k]; replaced || drop[k] {
			continue
		}
		out = append(out, kv)
	}
	for _, k := range []string{"HOME", "USER", "LOGNAME", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"} {
		out = append(out, k+"="+override[k])
	}
	return out
}

// sessionCommand builds a command for a program that belongs to the user's
// desktop session (pactl, pw-cli, pulseaudio, pipewire, systemctl --user).
// Under sudo it runs as the user who ran sudo, in their session; otherwise it
// is exec.Command unchanged.
//
//	in(1): string name program
//	in(2): ...string args arguments
//	out: *exec.Cmd
func sessionCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	// Linux only: macOS keeps its Homebrew PulseAudio handling (macOSPulseEnv)
	// and Windows has no sudo (session_windows.go).
	if runtime.GOOS != "linux" {
		return cmd
	}
	uid, gid, ok := common.InvokingIDs()
	if !ok {
		return cmd
	}
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return cmd
	}
	cred := &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	if ids, err := u.GroupIds(); err == nil {
		for _, id := range ids {
			if g, err := strconv.ParseUint(id, 10, 32); err == nil {
				cred.Groups = append(cred.Groups, uint32(g))
			}
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
	cmd.Env = sessionEnv(os.Environ(), uid, u.Username, u.HomeDir)
	cmd.Dir = u.HomeDir
	return cmd
}

// sessionPulseSocket is the native PulseAudio socket of the session RF Swift
// acts for, for the pure-Go client used when pactl is missing.
func sessionPulseSocket() string {
	return filepath.Join(sessionRuntimeDir(), "pulse", "native")
}
