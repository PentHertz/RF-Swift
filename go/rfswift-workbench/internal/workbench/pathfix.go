package workbench

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EnsureStandardPath prepends the standard tool install locations to PATH when
// they are missing. GUI apps launched from Finder/launchd inherit the minimal
// system PATH (/usr/bin:/bin:...), which hides Homebrew, /usr/local and Nix —
// so limactl, qemu, docker, podman and nix all appear "not installed" even
// though the CLI works fine from a terminal. The same applies to coding-agent
// CLIs (claude, codex, kimi, glm) installed under $HOME. Children (limactl,
// nix, install scripts, agent terminals) inherit the fixed PATH too. Call
// before any engine or agent detection.
func EnsureStandardPath() {
	home, _ := os.UserHomeDir()
	candidates := []string{
		// User-level CLI install dirs: the native Claude Code installer links
		// ~/.local/bin/claude; npm -g, bun, cargo, go and volta use the rest.
		// These come first so a user-installed agent CLI wins over a system one.
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".claude", "local"), // legacy claude migrate-installer target
		filepath.Join(home, ".npm-global", "bin"),
		filepath.Join(home, ".bun", "bin"),
		filepath.Join(home, ".cargo", "bin"),
		filepath.Join(home, "go", "bin"),
		filepath.Join(home, ".volta", "bin"),
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
	// Platform-specific locations (e.g. %APPDATA%\npm on Windows) and, on
	// Windows, the user/system PATH from the registry: an installer that just
	// added claude/codex to PATH only updates the registry, and a GUI process
	// started from a pre-existing shortcut/Explorer keeps the stale copy.
	candidates = append(candidates, platformPathCandidates(home)...)
	current := os.Getenv("PATH")
	present := map[string]bool{}
	for _, dir := range strings.Split(current, string(os.PathListSeparator)) {
		present[pathKey(dir)] = true
	}
	var missing []string
	for _, dir := range candidates {
		if dir == "" || present[pathKey(dir)] {
			continue
		}
		present[pathKey(dir)] = true
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

// pathKey normalises a PATH entry for de-duplication; Windows paths are
// case-insensitive and may mix separators or carry a trailing backslash.
func pathKey(dir string) string {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if runtime.GOOS == "windows" {
		return strings.ToLower(dir)
	}
	return dir
}
