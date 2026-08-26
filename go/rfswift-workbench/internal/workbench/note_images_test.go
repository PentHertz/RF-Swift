package workbench

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndReadManagedNoteImage(t *testing.T) {
	a := testApp(t, &fakeEngine{})
	if err := a.store.SaveMission(a.ws, Mission{ID: "lab", Title: "Lab"}); err != nil {
		t.Fatal(err)
	}
	const png = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	image, err := a.SaveNoteImage("lab", "clipboard.png", png)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(image.Path, "assets/") || filepath.Ext(image.Path) != ".png" {
		t.Fatalf("unexpected managed path %q", image.Path)
	}
	if _, err := os.Stat(filepath.Join(a.store.notesDir(a.ws, "lab"), filepath.FromSlash(image.Path))); err != nil {
		t.Fatal(err)
	}
	got, err := a.ReadNoteImage("lab", image.Path)
	if err != nil || !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("ReadNoteImage() = %q, %v", got, err)
	}
}

func TestImportNoteImageAcceptsFileURIWithSpaces(t *testing.T) {
	a := testApp(t, &fakeEngine{})
	if err := a.store.SaveMission(a.ws, Mission{ID: "lab"}); err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Screenshot With Spaces.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	image, err := a.ImportNoteImage("lab", "file://"+path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(image.Path, "assets/") {
		t.Fatalf("unexpected managed path %q", image.Path)
	}
}

func TestReadNoteImageRejectsTraversal(t *testing.T) {
	a := testApp(t, &fakeEngine{})
	if err := a.store.SaveMission(a.ws, Mission{ID: "lab"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReadNoteImage("lab", "../findings.json"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}
