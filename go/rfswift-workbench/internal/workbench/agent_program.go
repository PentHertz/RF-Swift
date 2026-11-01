package workbench

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// findAgentCommand looks a coding-agent CLI up on PATH, then in the
// user-level install folders a desktop launcher's PATH usually lacks: Node
// version managers (nvm, fnm) keep `npm install -g` programs next to the node
// they run with, and pnpm, yarn, deno and mise have their own folders. The
// first match wins, the newest Node version first.
func findAgentCommand(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	for _, dir := range agentSearchDirs(home) {
		// LookPath on a path checks it is executable, and on Windows tries
		// the PATHEXT extensions (codex.cmd).
		if path, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return path, true
		}
	}
	return "", false
}

// agentSearchDirs lists the folders findAgentCommand searches after PATH.
func agentSearchDirs(home string) []string {
	nvmDir := os.Getenv("NVM_DIR")
	if nvmDir == "" {
		nvmDir = filepath.Join(home, ".nvm")
	}
	var dirs []string
	dirs = append(dirs, nodeVersionDirs(filepath.Join(nvmDir, "versions", "node"), "bin")...)
	fnmDirs := []string{os.Getenv("FNM_DIR"), filepath.Join(home, ".local", "share", "fnm"), filepath.Join(home, ".fnm")}
	if runtime.GOOS == "darwin" {
		fnmDirs = append(fnmDirs, filepath.Join(home, "Library", "Application Support", "fnm"))
	}
	for _, fnm := range fnmDirs {
		if fnm != "" {
			dirs = append(dirs, nodeVersionDirs(filepath.Join(fnm, "node-versions"), filepath.Join("installation", "bin"))...)
		}
	}
	if pnpm := os.Getenv("PNPM_HOME"); pnpm != "" {
		dirs = append(dirs, pnpm)
	}
	dirs = append(dirs,
		filepath.Join(home, ".local", "share", "pnpm"),
		filepath.Join(home, "Library", "pnpm"),
		filepath.Join(home, ".yarn", "bin"),
		filepath.Join(home, ".config", "yarn", "global", "node_modules", ".bin"),
		filepath.Join(home, ".deno", "bin"),
		filepath.Join(home, ".local", "share", "mise", "shims"),
	)
	return dirs
}

// nodeVersionDirs returns root/<version>/sub for every installed Node
// version under root (v22.11.0, v20.18.1 ...), newest first.
func nodeVersionDirs(root, sub string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	sort.Slice(versions, func(i, j int) bool { return nodeVersionLess(versions[j], versions[i]) })
	dirs := make([]string, 0, len(versions))
	for _, version := range versions {
		dirs = append(dirs, filepath.Join(root, version, sub))
	}
	return dirs
}

// nodeVersionLess compares versions such as v20.9.0 and v20.18.1 number by
// number; a name that is not a version sorts before every version.
func nodeVersionLess(a, b string) bool {
	pa, oka := nodeVersionParts(a)
	pb, okb := nodeVersionParts(b)
	if oka != okb {
		return !oka
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return a < b
}

func nodeVersionParts(name string) ([3]int, bool) {
	var parts [3]int
	fields := strings.Split(strings.TrimPrefix(name, "v"), ".")
	if len(fields) != 3 {
		return parts, false
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

// checkAgentProgram checks a program path typed or picked in the Agent
// settings and returns it cleaned, with a leading ~ expanded.
func checkAgentProgram(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s: give the full path of the program", path)
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return "", fmt.Errorf("%s was not found", path)
	case info.IsDir():
		return "", fmt.Errorf("%s is a folder, not a program", path)
	case runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0:
		return "", fmt.Errorf("%s is not executable (chmod +x)", path)
	}
	return path, nil
}

// cleanAgentPaths checks the program paths of the Agent settings before
// they are saved: empty entries and unknown clients are dropped, a path that
// cannot be used is refused with the client's name.
func cleanAgentPaths(paths map[string]string) (map[string]string, error) {
	labels := map[string]string{}
	for _, client := range agentClientDefs() {
		labels[client.ID] = client.Label
	}
	out := map[string]string{}
	for id, path := range paths {
		label, known := labels[id]
		if !known || strings.TrimSpace(path) == "" {
			continue
		}
		resolved, err := checkAgentProgram(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", label, err)
		}
		out[id] = resolved
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// agentEnv is the environment a coding-agent CLI starts with: the
// Workbench's, with the program's folder first on PATH. An npm-installed CLI
// is a "#!/usr/bin/env node" script, and nvm or fnm keep node in that same
// folder, which the PATH of a desktop launcher does not have.
func agentEnv(command string) []string {
	env := os.Environ()
	dir := filepath.Dir(command)
	extra := []string{"TERM=xterm-256color", "COLORTERM=truecolor"}
	for i, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok && (key == "PATH" || runtime.GOOS == "windows" && strings.EqualFold(key, "PATH")) {
			env[i] = key + "=" + dir + string(os.PathListSeparator) + value
			return append(env, extra...)
		}
	}
	return append(env, append([]string{"PATH=" + dir}, extra...)...)
}
