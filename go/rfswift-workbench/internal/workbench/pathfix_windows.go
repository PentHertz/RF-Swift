//go:build windows

package workbench

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// platformPathCandidates returns the Windows install dirs of the coding-agent
// CLIs plus the PATH entries recorded in the registry. The native Claude Code
// installer drops claude.exe in %USERPROFILE%\.local\bin (already covered by
// the shared list); npm -g (codex, older claude, kimi, glm) uses %APPDATA%\npm;
// Volta uses %LOCALAPPDATA%\Volta\bin. The registry read matters because a
// process started from Explorer or a shortcut keeps the PATH captured at its
// own start-up, so anything installed afterwards is invisible until reboot.
func platformPathCandidates(home string) []string {
	var dirs []string
	if appData := os.Getenv("APPDATA"); appData != "" {
		dirs = append(dirs, filepath.Join(appData, "npm"))
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs,
			filepath.Join(local, "Volta", "bin"),
			filepath.Join(local, "Programs", "Python", "Scripts"),
		)
	}
	dirs = append(dirs, filepath.Join(home, "scoop", "shims"))
	dirs = append(dirs, registryPath(registry.CURRENT_USER, `Environment`)...)
	dirs = append(dirs, registryPath(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`)...)
	return dirs
}

// registryPath reads the Path value under key and expands %VAR% references
// (the user Path is usually REG_EXPAND_SZ with %USERPROFILE%).
func registryPath(root registry.Key, key string) []string {
	k, err := registry.OpenKey(root, key, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	raw, _, err := k.GetStringValue("Path")
	if err != nil || raw == "" {
		return nil
	}
	if expanded, err := registry.ExpandString(raw); err == nil {
		raw = expanded
	}
	var dirs []string
	for _, dir := range strings.Split(raw, ";") {
		if dir = strings.TrimSpace(dir); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}
