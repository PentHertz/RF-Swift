package workbench

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveEncryptionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	encrypted := filepath.Join(dir, "archive.rfenv")
	decrypted := filepath.Join(dir, "decrypted")
	want := []byte("portable RF Swift archive\x00content")
	if err := os.WriteFile(source, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := encryptArchive(source, encrypted, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	protected, err := archiveIsEncrypted(encrypted)
	if err != nil || !protected {
		t.Fatalf("archiveIsEncrypted = %v, %v", protected, err)
	}
	if err := decryptArchive(encrypted, decrypted, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(decrypted)
	if err != nil || string(got) != string(want) {
		t.Fatalf("decrypted archive = %q, %v", got, err)
	}
	if err := decryptArchive(encrypted, filepath.Join(dir, "wrong"), "wrong"); err == nil {
		t.Fatal("decryptArchive accepted an incorrect password")
	}
}

func TestMissionNotesCompanionArchive(t *testing.T) {
	dir := t.TempDir()
	a := NewApp()
	a.store = NewStore(filepath.Join(dir, "store"))
	a.ws = "project"
	if err := a.store.CreateWorkspace(a.ws); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SaveMission(a.ws, Mission{ID: "radio", Engine: "nix"}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SaveNote(a.ws, "radio", "note.md", "# HydraSDR notes"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.store.reportsDir(a.ws, "radio"), "assessment.md"), []byte("exported report"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "radio.rfenv")
	notes, err := a.exportMissionNotes("radio", target, "")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(notes)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 2 || zr.File[0].Name != "mission/notes/note.md" || zr.File[1].Name != "mission/reports/assessment.md" {
		t.Fatalf("unexpected notes archive entries: %#v", zr.File)
	}
	if err := zr.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SaveNote(a.ws, "radio", "note.md", "stale local note"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.store.reportsDir(a.ws, "radio"), "assessment.md"), []byte("stale report"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.ImportTargetNotes("radio", target, ""); err != nil {
		t.Fatal(err)
	}
	got, err := a.store.GetNote(a.ws, "radio", "note.md")
	if err != nil || got != "# HydraSDR notes" {
		t.Fatalf("restored note = %q, %v", got, err)
	}
	report, err := os.ReadFile(filepath.Join(a.store.reportsDir(a.ws, "radio"), "assessment.md"))
	if err != nil || string(report) != "exported report" {
		t.Fatalf("restored report = %q, %v", report, err)
	}
}
