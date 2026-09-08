package nix

import (
	"os"
	"strings"
	"testing"
)

func TestJailMountsToolsWritableForLazy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ws := t.TempDir()
	lazy := &Environment{Name: "lz", Lazy: true, Isolate: true, Workspace: ws}
	var tools, work *jailMount
	for i, m := range jailMounts(lazy, ws) {
		m := m
		switch {
		case m.host == toolsDir("lz"):
			tools = &jailMounts(lazy, ws)[i]
		case m.host == ws:
			work = &m
		}
		if m.host == EnvDir("lz") && m.rw {
			t.Fatal("the state dir must stay read-only")
		}
	}
	if tools == nil || !tools.rw || tools.jail != jailEnv+"/tools" || tools.workdir {
		t.Fatalf("tools mount = %#v", tools)
	}
	if work == nil || !work.workdir || !work.rw {
		t.Fatalf("workspace mount = %#v", work)
	}
	if _, err := os.Stat(toolsDir("lz")); err != nil {
		t.Fatalf("tools dir not created: %v", err)
	}
	eager := &Environment{Name: "eg", Isolate: true, Workspace: ws}
	for _, m := range jailMounts(eager, ws) {
		if m.host == toolsDir("eg") {
			t.Fatal("eager environments have no tools/ to expose")
		}
	}
	// The shell still starts in the workspace, not in the last rw mount.
	args := isolateArgs(lazy, ws)
	for i := range args {
		if args[i] == "--chdir" && args[i+1] != jailWorkdir {
			t.Fatalf("--chdir %s, want %s", args[i+1], jailWorkdir)
		}
	}
}

func TestDarwinProfileToolsWritableForLazy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lazy := &Environment{Name: "lz", Lazy: true, Isolate: true}
	p := darwinSandboxProfile(lazy, "", jailHomeDir("lz"))
	if !strings.Contains(p, `(allow file-read* file-write* (subpath "`+toolsDir("lz")+`"))`) {
		t.Fatalf("tools/ not writable:\n%s", p)
	}
	if strings.Contains(p, `(allow file-read* file-write* (subpath "`+EnvDir("lz")+`"))`) {
		t.Fatal("the whole state dir must not be writable")
	}
	eager := &Environment{Name: "eg", Isolate: true}
	if strings.Contains(darwinSandboxProfile(eager, "", jailHomeDir("eg")), toolsDir("eg")) {
		t.Fatal("eager profile mentions tools/")
	}
}

func TestShimsRebuildPrerequisitesOnlyWhenMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	env := &Environment{Name: "lz", Image: "rfid", Lazy: true, FlakeRef: "github:PentHertz/RF-Swift-nix", Prerequisites: []string{"libnfc"}, Commands: map[string]string{"pm5": "proxmark5"}}
	if err := writeShims(env); err != nil {
		t.Fatal(err)
	}
	shim, err := os.ReadFile(shimsDir("lz") + "/pm5")
	if err != nil {
		t.Fatal(err)
	}
	want := `if [ ! -e "` + prerequisitesLink("lz") + `" ]; then`
	if !strings.Contains(string(shim), want) {
		t.Fatalf("shim lacks %q:\n%s", want, shim)
	}
	if !strings.Contains(string(shim), "# rfswift-shim-format: 3") {
		t.Fatal("shim format not bumped: existing shims would keep the unconditional rebuild")
	}
	for _, sh := range []string{"bash", "zsh"} {
		if h := lazyHandlerFor(env, sh); !strings.Contains(h, want) {
			t.Fatalf("%s lazy handler lacks the guard:\n%s", sh, h)
		}
	}
}
