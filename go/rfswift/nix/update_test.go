package nix

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerationArchiveAndRollback(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	name := "radio"
	oldStore := filepath.Join(t.TempDir(), "old-store")
	newStore := filepath.Join(t.TempDir(), "new-store")
	if err := os.MkdirAll(oldStore, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newStore, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureDir(EnvDir(name)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(oldStore, profileLink(name)); err != nil {
		t.Fatal(err)
	}
	env := &Environment{Name: name, Image: "sdr_light", FlakeRef: "path:/flake", ProfilePath: profileLink(name)}
	if err := writeManifest(env); err != nil {
		t.Fatal(err)
	}

	if err := archiveCurrentProfile(name); err != nil {
		t.Fatal(err)
	}
	if err := switchProfile(profileLink(name), newStore); err != nil {
		t.Fatal(err)
	}
	gens, err := ListGenerations(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) != 1 || gens[0].StorePath != oldStore {
		t.Fatalf("unexpected generations: %+v", gens)
	}

	if err := RollbackEnvironment(name, gens[0].Name); err != nil {
		t.Fatal(err)
	}
	active, err := filepath.EvalSymlinks(profileLink(name))
	if err != nil {
		t.Fatal(err)
	}
	if active != oldStore {
		t.Fatalf("rollback selected %q, want %q", active, oldStore)
	}
	gens, err = ListGenerations(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) != 2 {
		t.Fatalf("rollback should preserve displaced generation, got %d", len(gens))
	}
}

func TestLocalFlakePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "flake.nix"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := localFlakePath("path:" + dir); !ok || got != dir {
		t.Fatalf("local flake not resolved: %q %v", got, ok)
	}
	if _, ok := localFlakePath("github:PentHertz/RF-Swift-nix"); ok {
		t.Fatal("remote flake treated as writable local path")
	}
}

func TestEnvironmentFlakeInputsFromLocalLock(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	flake := t.TempDir()
	if err := os.WriteFile(filepath.Join(flake, "flake.nix"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := `{"root":"root","nodes":{"root":{"inputs":{"nixpkgs":"nixpkgs","systems":"systems"}},"nixpkgs":{},"systems":{}}}`
	if err := os.WriteFile(filepath.Join(flake, "flake.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(&Environment{Name: "inputs", Image: "sdr_light", FlakeRef: flake}); err != nil {
		t.Fatal(err)
	}
	inputs, err := EnvironmentFlakeInputs("inputs")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || inputs[0] != "nixpkgs" || inputs[1] != "systems" {
		t.Fatalf("unexpected inputs: %v", inputs)
	}
}

func TestFailedUpdateRestoresLockAndActiveProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX fake nix script")
	}
	state := t.TempDir()
	flake := t.TempDir()
	store := filepath.Join(t.TempDir(), "working-store")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flake, "flake.nix"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldLock := []byte("old-lock\n")
	if err := os.WriteFile(filepath.Join(flake, "flake.lock"), oldLock, 0o644); err != nil {
		t.Fatal(err)
	}

	fake := filepath.Join(t.TempDir(), "nix")
	script := `#!/bin/sh
case " $* " in
  *" flake update "*) printf 'broken-new-lock\n' > flake.lock; exit 0 ;;
  *" build "*) exit 42 ;;
esac
exit 0
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RFSWIFT_NIX_HOME", state)
	t.Setenv("RFSWIFT_NIX_BIN", fake)
	name := "safe"
	if err := ensureDir(EnvDir(name)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(store, profileLink(name)); err != nil {
		t.Fatal(err)
	}
	env := &Environment{Name: name, Image: "sdr_light", FlakeRef: flake, ProfilePath: profileLink(name)}
	if err := writeManifest(env); err != nil {
		t.Fatal(err)
	}

	if err := UpdateEnvironment(name, UpdateOptions{}); err == nil {
		t.Fatal("broken candidate build unexpectedly succeeded")
	}
	gotLock, err := os.ReadFile(filepath.Join(flake, "flake.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotLock) != string(oldLock) {
		t.Fatalf("lock not restored: %q", gotLock)
	}
	active, err := filepath.EvalSymlinks(profileLink(name))
	if err != nil {
		t.Fatal(err)
	}
	if active != store {
		t.Fatalf("active profile changed to %q", active)
	}
	gens, err := ListGenerations(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) != 0 {
		t.Fatalf("failed update created generations: %+v", gens)
	}
}
