package workbench

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func FuzzProjectArchiveEntryPath(f *testing.F) {
	for _, seed := range []string{"notes/note.md", "../escape", "/absolute", `..\\escape`, "a/../../escape", "missions/lab/notes/x.md"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, entry string) {
		archive := filepath.Join(t.TempDir(), "input.zip")
		out, err := os.Create(archive)
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(out)
		meta, _ := zw.Create("workspace.json")
		_, _ = meta.Write([]byte(`{"name":"fuzz"}`))
		if entry != "" {
			if w, createErr := zw.Create(entry); createErr == nil {
				_, _ = w.Write([]byte("payload"))
			}
		}
		_ = zw.Close()
		_ = out.Close()
		root := t.TempDir()
		_, _ = importProjectArchive(NewStore(root), archive)
		if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
			t.Fatalf("archive entry %q escaped extraction root", entry)
		}
	})
}
