package workbench

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeProgram creates an executable script at path.
func writeProgram(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAgentProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bits and ~ paths are Unix behaviour")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	program := filepath.Join(home, ".nvm", "versions", "node", "v22.11.0", "bin", "codex")
	writeProgram(t, program)
	plain := filepath.Join(home, "notes.txt")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got, err := checkAgentProgram("  ~/.nvm/versions/node/v22.11.0/bin/codex "); err != nil || got != program {
		t.Fatalf("~ path = %q %v", got, err)
	}
	for path, want := range map[string]string{
		"codex":                        "full path",
		filepath.Join(home, "missing"): "not found",
		filepath.Dir(program):          "folder",
		plain:                          "not executable",
	} {
		if _, err := checkAgentProgram(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("checkAgentProgram(%q) = %v, want an error about %q", path, err, want)
		}
	}
}

func TestCleanAgentPathsChecksBeforeSaving(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bits are Unix behaviour")
	}
	program := filepath.Join(t.TempDir(), "opencode")
	writeProgram(t, program)
	got, err := cleanAgentPaths(map[string]string{"opencode": " " + program + " ", "codex": "", "unknown": "/bin/sh"})
	if err != nil || len(got) != 1 || got["opencode"] != program {
		t.Fatalf("cleanAgentPaths = %v %v", got, err)
	}
	if _, err := cleanAgentPaths(map[string]string{"mistral": "/no/such/vibe"}); err == nil || !strings.Contains(err.Error(), "Mistral Vibe") {
		t.Fatalf("a bad path must be refused with the client's name, got %v", err)
	}
	if got, err := cleanAgentPaths(nil); err != nil || got != nil {
		t.Fatalf("no paths = %v %v", got, err)
	}
}

func TestAgentClientsUseTheConfiguredProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bits are Unix behaviour")
	}
	program := filepath.Join(t.TempDir(), "my-claude")
	writeProgram(t, program)
	byID := map[string]AgentClient{}
	for _, c := range AgentClients(map[string]string{"claude": program, "kimi": "/no/such/kimi"}) {
		byID[c.ID] = c
	}
	if c := byID["claude"]; !c.Available || !c.Custom || c.Path != program || c.Problem != "" {
		t.Errorf("claude = %+v", c)
	}
	if c := byID["kimi"]; c.Available || !c.Custom || !strings.Contains(c.Problem, "not found") {
		t.Errorf("kimi with a missing program = %+v", c)
	}
}

// npm -g under nvm puts codex next to node, in a folder a desktop launcher's
// PATH does not have: the search finds it, newest Node version first.
func TestFindAgentCommandSearchesNodeVersionManagers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix install layout")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")
	t.Setenv("FNM_DIR", "")
	t.Setenv("PNPM_HOME", "")
	t.Setenv("PATH", t.TempDir())
	versions := filepath.Join(home, ".nvm", "versions", "node")
	writeProgram(t, filepath.Join(versions, "v20.9.0", "bin", "codex"))
	writeProgram(t, filepath.Join(versions, "v20.18.1", "bin", "codex"))
	writeProgram(t, filepath.Join(home, ".local", "share", "fnm", "node-versions", "v22.1.0", "installation", "bin", "opencode"))
	writeProgram(t, filepath.Join(home, ".local", "share", "pnpm", "kimi"))

	if got, ok := findAgentCommand("codex"); !ok || got != filepath.Join(versions, "v20.18.1", "bin", "codex") {
		t.Errorf("codex = %q %v, want the newest nvm version", got, ok)
	}
	if got, ok := findAgentCommand("opencode"); !ok || !strings.Contains(got, filepath.Join("fnm", "node-versions", "v22.1.0")) {
		t.Errorf("opencode = %q %v", got, ok)
	}
	if got, ok := findAgentCommand("kimi"); !ok || !strings.HasSuffix(got, filepath.Join("pnpm", "kimi")) {
		t.Errorf("kimi = %q %v", got, ok)
	}
	if got, ok := findAgentCommand("vibe"); ok {
		t.Errorf("vibe found at %q, but it is not installed", got)
	}
}

func TestNodeVersionLess(t *testing.T) {
	ordered := []string{"system", "v8.17.0", "v10.0.0", "v20.9.0", "v20.18.1", "v22.0.0"}
	for i := 1; i < len(ordered); i++ {
		if !nodeVersionLess(ordered[i-1], ordered[i]) || nodeVersionLess(ordered[i], ordered[i-1]) {
			t.Errorf("%s must sort before %s", ordered[i-1], ordered[i])
		}
	}
}

func TestAgentEnvPutsTheProgramFolderFirst(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	dir := filepath.Join(t.TempDir(), "bin")
	env := agentEnv(filepath.Join(dir, "codex"))
	path, term := "", ""
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		switch {
		case strings.EqualFold(key, "PATH"):
			path = value
		case key == "TERM":
			term = value
		}
	}
	if !strings.HasPrefix(path, dir+string(os.PathListSeparator)) || term != "xterm-256color" {
		t.Fatalf("PATH = %q, TERM = %q", path, term)
	}
}
