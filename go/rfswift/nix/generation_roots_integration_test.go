package nix

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in: adds two harmless fixture files to a local Nix store. Never runs GC.
func TestGenerationRootIntegration(t *testing.T) {
	if os.Getenv("RFSWIFT_TEST_NIX_ROOTS") != "1" {
		t.Skip("set RFSWIFT_TEST_NIX_ROOTS=1 with Nix installed")
	}
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	t.Setenv("RFSWIFT_NIX_BIN", "nix")
	add := func(name string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("nix-store", "--add", path).CombinedOutput()
		if err != nil {
			t.Fatalf("add fixture: %v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	old, next := add("old-rfswift-root-fixture"), add("new-rfswift-root-fixture")
	name := "root-integration"
	if err := ensureDir(EnvDir(name)); err != nil {
		t.Fatal(err)
	}
	args := append(experimentalArgs(), "build", "--out-link", profileLink(name), old)
	if out, err := nixCommand(args...).CombinedOutput(); err != nil {
		t.Fatalf("initial root: %v: %s", err, out)
	}
	if err := archiveCurrentProfile(name); err != nil {
		t.Fatal(err)
	}
	if err := switchProfile(profileLink(name), next); err != nil {
		t.Fatal(err)
	}
	roots, err := exec.Command("nix-store", "--query", "--roots", old).CombinedOutput()
	if err != nil || !strings.Contains(string(roots), generationsDir(name)) {
		t.Fatalf("generation not rooted after switching: %v: %s", err, roots)
	}
}
