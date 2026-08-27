package workbench

import (
	"os"
	"path/filepath"
	"strings"
)

// EnsureStandardPath prepends the standard tool install locations to PATH when
// they are missing. GUI apps launched from Finder/launchd inherit the minimal
// system PATH (/usr/bin:/bin:...), which hides Homebrew, /usr/local and Nix —
// so limactl, qemu, docker, podman and nix all appear "not installed" even
// though the CLI works fine from a terminal. Children (limactl, nix, install
// scripts) inherit the fixed PATH too. Call before any engine detection.
func EnsureStandardPath() {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
		"/usr/local/sbin",
		"/opt/local/bin",
		"/opt/X11/bin", // XQuartz: xhost, needed for X11 forwarding on macOS
		"/usr/X11/bin",
		filepath.Join(home, ".nix-profile", "bin"),
		"/nix/var/nix/profiles/default/bin",
		"/run/current-system/sw/bin",
	}
	current := os.Getenv("PATH")
	present := map[string]bool{}
	for _, dir := range strings.Split(current, string(os.PathListSeparator)) {
		present[dir] = true
	}
	var missing []string
	for _, dir := range candidates {
		if present[dir] {
			continue
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		missing = append(missing, dir)
	}
	if len(missing) == 0 {
		return
	}
	os.Setenv("PATH", strings.Join(append(missing, current), string(os.PathListSeparator)))
}
