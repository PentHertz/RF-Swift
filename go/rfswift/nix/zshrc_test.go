package nix

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteZshRC(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	env := &Environment{Name: "sdr", Image: "sdr_light", Lazy: true, Packages: []string{"gnuradio", "hackrf"}, Commands: map[string]string{"hackrf_info": "hackrf"}, FlakeRef: "github:PentHertz/RF-Swift-nix"}
	dir, err := writeZshRC(env, "/opt/bin")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	rc := string(body)
	for _, want := range []string{`export PATH="/opt/bin":"$PATH"`, `Workspace: $RFSWIFT_WORKSPACE`, `PROMPT="(rfswift:sdr) $PROMPT"`, "command_not_found_handler()", "local -a ordered", `${(P)v}`} {
		if !strings.Contains(rc, want) {
			t.Errorf("zshrc lacks %q", want)
		}
	}
	if strings.Contains(rc, "command_not_found_handle()") || strings.Contains(rc, "${!v}") {
		t.Error("zshrc contains bash-only syntax")
	}
	if zsh, err := exec.LookPath("zsh"); err == nil {
		if out, err := exec.Command(zsh, "-n", filepath.Join(dir, ".zshrc")).CombinedOutput(); err != nil {
			t.Fatalf("zsh -n rejects the generated rc: %v\n%s", err, out)
		}
	}
	// zshEnv is only for zsh; a pure shell gets no PATH line.
	if got := zshEnv(env, "/bin/bash", "/opt/bin"); got != nil {
		t.Fatalf("zshEnv for bash = %#v", got)
	}
	got := zshEnv(env, "/bin/zsh", "")
	if got["ZDOTDIR"] != dir {
		t.Fatalf("zshEnv = %#v", got)
	}
	body, _ = os.ReadFile(filepath.Join(dir, ".zshrc"))
	if strings.Contains(string(body), `export PATH="/opt/bin"`) {
		t.Fatal("pure zshrc must not override PATH")
	}
	if got["SHELL_SESSIONS_DISABLE"] != "1" {
		t.Fatalf("zshEnv = %#v", got)
	}
}

func TestBashLazyHandlerUnchanged(t *testing.T) {
	env := &Environment{Name: "sdr", Packages: []string{"a"}}
	h := lazyHandler(env)
	if !strings.Contains(h, "command_not_found_handle() {") || !strings.Contains(h, "local a leaf out link ordered=()") {
		t.Fatalf("bash handler changed:\n%s", h)
	}
}

func TestLinkJailWorkspace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	jail := t.TempDir()
	ws := t.TempDir()
	link := filepath.Join(jail, "workspace")
	linkJailWorkspace(jail, "/workspace", ws)
	if got, err := os.Readlink(link); err != nil || got != "/workspace" {
		t.Fatalf("link = %q, %v", got, err)
	}
	// Retargeting replaces the link; no workspace removes it.
	linkJailWorkspace(jail, ws, ws)
	if got, _ := os.Readlink(link); got != ws {
		t.Fatalf("retarget = %q", got)
	}
	linkJailWorkspace(jail, "/workspace", "")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("stale link kept: %v", err)
	}
	linkJailWorkspace(jail, "/workspace", os.Getenv("HOME"))
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("home-as-workspace must not be linked")
	}
}
