//go:build !windows

package rfutils

import (
	"os"
	"strings"
	"testing"
)

func TestSessionEnvSwitchesToTheUser(t *testing.T) {
	env := []string{"HOME=/root", "USER=root", "LOGNAME=root", "SUDO_USER=alice", "SUDO_UID=1000", "XDG_RUNTIME_DIR=/run/user/0", "DISPLAY=:0", "PULSE_RUNTIME_PATH=/root/.pulse", "PATH=/usr/bin"}
	got := strings.Join(sessionEnv(env, 1000, "alice", "/home/alice"), "\n")
	for _, want := range []string{"HOME=/home/alice", "USER=alice", "LOGNAME=alice", "XDG_RUNTIME_DIR=/run/user/1000", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus", "DISPLAY=:0", "PATH=/usr/bin"} {
		if !strings.Contains(got, want) {
			t.Errorf("session env lacks %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"HOME=/root", "SUDO_USER", "SUDO_UID", "/run/user/0", "PULSE_RUNTIME_PATH"} {
		if strings.Contains(got, gone) {
			t.Errorf("session env keeps %q:\n%s", gone, got)
		}
	}
}

func TestSessionCommandOutsideSudoIsPlain(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("runs as root")
	}
	cmd := sessionCommand("pactl", "info")
	if cmd.SysProcAttr != nil || cmd.Env != nil {
		t.Fatal("without sudo, session commands must run unchanged")
	}
}
