package workbench

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkbenchProjectRoundTrip(t *testing.T) {
	sourceRoot := t.TempDir()
	source := NewStore(sourceRoot)
	if err := source.CreateWorkspace("ground-truth"); err != nil {
		t.Fatal(err)
	}
	m := Mission{ID: "rfid-lab", Title: "RFID assessment", Engine: "docker", Image: "rfswift/rfid:latest", Caps: []string{"NET_ADMIN"}, Mounts: []string{"/dev/bus/usb"}}
	if err := source.SaveMission("ground-truth", m); err != nil {
		t.Fatal(err)
	}
	if err := source.SaveNote("ground-truth", m.ID, "note.md", "# Evidence\n\nRecorded command output."); err != nil {
		t.Fatal(err)
	}
	if err := source.SaveFindings("ground-truth", m.ID, []Finding{{ID: "F-1", Title: "Replay accepted", Sev: "high"}}); err != nil {
		t.Fatal(err)
	}
	recordings := filepath.Join(source.missionDir("ground-truth", m.ID), "recordings")
	if err := os.MkdirAll(recordings, 0o755); err != nil {
		t.Fatal(err)
	}
	cast := []byte("{\"version\":2,\"width\":80,\"height\":24}\n[0.1,\"o\",\"pm3 --list\\r\\n\"]\n")
	if err := os.WriteFile(filepath.Join(recordings, "session.cast"), cast, 0o644); err != nil {
		t.Fatal(err)
	}
	captures := source.capturesDir("ground-truth", m.ID)
	artifact := []byte{0xd4, 0xc3, 0xb2, 0xa1, 0x01, 0x02, 0x03}
	if err := os.WriteFile(filepath.Join(captures, "radio.pcap"), artifact, 0o644); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "ground-truth.rfswift-workbench.zip")
	if err := writeProjectArchive(source.wsDir("ground-truth"), archive); err != nil {
		t.Fatal(err)
	}
	destination := NewStore(t.TempDir())
	name, err := importProjectArchive(destination, archive)
	if err != nil {
		t.Fatal(err)
	}
	if name != "ground-truth" {
		t.Fatalf("imported workspace = %q", name)
	}
	checks := map[string][]byte{
		"missions/rfid-lab/notes/note.md":           []byte("# Evidence\n\nRecorded command output."),
		"missions/rfid-lab/recordings/session.cast": cast,
		"missions/rfid-lab/captures/radio.pcap":     artifact,
	}
	for rel, want := range checks {
		got, err := os.ReadFile(filepath.Join(destination.wsDir(name), filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read restored %s: %v", rel, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("restored %s differs: got %x want %x", rel, got, want)
		}
	}
	findings, err := destination.LoadFindings(name, m.ID)
	if err != nil || len(findings) != 1 || findings[0].Title != "Replay accepted" {
		t.Fatalf("restored findings = %#v, %v", findings, err)
	}
}

func TestWorkbenchProjectRejectsPathTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "evil.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../outside.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("must not escape"))
	_ = zw.Close()
	_ = f.Close()
	destinationRoot := t.TempDir()
	if _, err := importProjectArchive(NewStore(destinationRoot), archive); err == nil {
		t.Fatal("unsafe archive was accepted")
	}
	if _, err := os.Stat(filepath.Join(destinationRoot, "outside.txt")); !os.IsNotExist(err) {
		t.Fatalf("path traversal wrote outside workspace: %v", err)
	}
}
