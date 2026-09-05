package nix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveWorkspaceDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ws := filepath.Join(home, "rfswift-workspace", "lab")
	if err := os.MkdirAll(filepath.Join(ws, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "sub", "cap.bin"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorkspaceDir(ws); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ws); !os.IsNotExist(err) {
		t.Fatalf("workspace still there: %v", err)
	}
	// Gone already, or never set: fine.
	for _, p := range []string{ws, "", "none"} {
		if err := RemoveWorkspaceDir(p); err != nil {
			t.Fatalf("RemoveWorkspaceDir(%q) = %v", p, err)
		}
	}
	// Refusals.
	link := filepath.Join(home, "linked")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, "file")
	_ = os.WriteFile(file, []byte("x"), 0o600)
	for _, p := range []string{home, "/", link, file} {
		err := RemoveWorkspaceDir(p)
		if err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Fatalf("RemoveWorkspaceDir(%q) = %v, want a refusal", p, err)
		}
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("refused symlink was removed")
	}
}
