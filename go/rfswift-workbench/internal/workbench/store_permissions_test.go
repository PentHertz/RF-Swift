package workbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreUsesOwnerOnlyPermissionsAndMigratesExistingData(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "workspaces"))
	if err := store.CreateWorkspace("private"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMission("private", Mission{ID: "lab"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveNote("private", "lab", "note.md", "secret"); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(store.notesDir("private", "lab"), "note.md")
	if err := os.Chmod(store.wsDir("private"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(note, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.SecurePermissions(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{store.Root: 0o700, store.wsDir("private"): 0o700, store.notesDir("private", "lab"): 0o700, note: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s permissions = %o, want %o", path, got, want)
		}
	}
}

func TestSecurePermissionsDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(filepath.Join(root, "workspaces"))
	if err := store.CreateWorkspace("private"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(store.wsDir("private"), "outside-link")); err != nil {
		t.Fatal(err)
	}
	if err := store.SecurePermissions(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("external symlink target permissions changed to %o", got)
	}
}
