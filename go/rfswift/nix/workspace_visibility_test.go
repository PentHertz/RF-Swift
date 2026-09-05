package nix

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestWorkspaceInShell(t *testing.T) {
	ws := t.TempDir()
	plain := &Environment{Name: "sdr", Workspace: ws}
	if got := WorkspaceInShell(plain); got != ws {
		t.Fatalf("native workspace = %q, want the host path %q", got, ws)
	}
	for _, none := range []string{"", "none"} {
		if got := WorkspaceInShell(&Environment{Name: "sdr", Workspace: none}); got != "" {
			t.Fatalf("workspace %q: WorkspaceInShell = %q, want empty", none, got)
		}
	}
	jailed := &Environment{Name: "sdr", Workspace: ws, Isolate: true}
	got := WorkspaceInShell(jailed)
	switch runtime.GOOS {
	case "linux", "windows":
		if got != jailWorkdir {
			t.Fatalf("jailed workspace = %q, want %q", got, jailWorkdir)
		}
	default:
		// The macOS sandbox keeps host paths in place.
		if got != ws {
			t.Fatalf("jailed workspace on %s = %q, want the host path", runtime.GOOS, got)
		}
	}
}

func TestWorkspaceHint(t *testing.T) {
	ws := t.TempDir()
	if hint := WorkspaceHint(&Environment{Name: "sdr"}); !strings.Contains(hint, "none") || !strings.Contains(hint, "--workspace") {
		t.Fatalf("no-workspace hint = %q", hint)
	}
	if hint := WorkspaceHint(&Environment{Name: "sdr", Workspace: ws}); hint != "Workspace: "+ws {
		t.Fatalf("native hint = %q", hint)
	}
	hint := WorkspaceHint(&Environment{Name: "sdr", Workspace: ws, Isolate: true})
	if !strings.Contains(hint, ws) {
		t.Fatalf("jailed hint %q does not name the host path", hint)
	}
	if runtime.GOOS == "linux" && !strings.Contains(hint, jailWorkdir+" inside the jail") {
		t.Fatalf("jailed hint %q does not name the in-jail path", hint)
	}
}

func TestShellEnvCarriesWorkspace(t *testing.T) {
	ws := t.TempDir()
	env := &Environment{Name: "sdr", Workspace: ws}
	gl := map[string]string{"LIBGL_DRIVERS_PATH": "/x"}
	got := shellEnv(env, ws, gl)
	if got["RFSWIFT_WORKSPACE"] != ws || got["LIBGL_DRIVERS_PATH"] != "/x" {
		t.Fatalf("shellEnv = %#v", got)
	}
	if _, leaked := gl["RFSWIFT_WORKSPACE"]; leaked {
		t.Fatal("shellEnv mutated the GL map")
	}
	// A shell that had to fall back to another directory must not claim one.
	if got := shellEnv(env, "/elsewhere", nil); got["RFSWIFT_WORKSPACE"] != "" {
		t.Fatalf("fallback cwd still advertises a workspace: %#v", got)
	}
	if got := shellEnv(&Environment{Name: "sdr", Workspace: "none"}, "none", nil); len(got) != 0 {
		t.Fatalf("workspace none produced %#v", got)
	}
}

func TestBashRCMentionsWorkspace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	env := &Environment{Name: "sdr", Image: "sdr_light", Packages: []string{"a"}}
	if err := os.MkdirAll(EnvDir(env.Name), 0o755); err != nil {
		t.Fatal(err)
	}
	rc, err := writeBashRC(env, "/bin")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `Workspace: $RFSWIFT_WORKSPACE`) {
		t.Fatal("bashrc banner does not name the workspace")
	}
}
