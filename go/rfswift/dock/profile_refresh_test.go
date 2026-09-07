package dock

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A built-in profile stored by an older version is refreshed when it was not
// edited, kept (and reported) when it was.
func TestInitDefaultProfilesRefreshesUneditedBuiltins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the profiles directory comes from APPDATA there")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	dir := ProfilesDirByPlatform()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The rfid profile as older versions wrote it: no devices, no fingerprint.
	legacy := "name: rfid\ndescription: RFID/NFC tools (Proxmark3, libnfc) over USB\nimage: " + officialImage("rfid") + "\nnetwork: host\nbindings: " + usbTreeBinding + "\ncgroups: c 189:* rwm\n"
	rfidPath := filepath.Join(dir, profileFilename("rfid"))
	if err := os.WriteFile(rfidPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	created, updated, skipped, stale := InitDefaultProfiles(false)
	if updated != 1 || skipped != 0 || len(stale) != 0 || created != len(DefaultProfiles())-1 {
		t.Fatalf("first init: created=%d updated=%d skipped=%d stale=%v", created, updated, skipped, stale)
	}
	stored, err := readProfileFile(rfidPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.Devices, "/dev/tty0") || stored.Fingerprint == "" {
		t.Fatalf("rfid not refreshed: %+v", stored)
	}

	// Everything is current now: nothing happens.
	if created, updated, skipped, stale = InitDefaultProfiles(false); created != 0 || updated != 0 || skipped != len(DefaultProfiles()) || len(stale) != 0 {
		t.Fatalf("second init: created=%d updated=%d skipped=%d stale=%v", created, updated, skipped, stale)
	}

	// An edited copy is kept and reported, fingerprint or not.
	stored.Image = "example.org/custom:rfid"
	data, _ := os.ReadFile(rfidPath)
	edited := strings.Replace(string(data), officialImage("rfid"), "example.org/custom:rfid", 1)
	if err := os.WriteFile(rfidPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, updated, _, stale = InitDefaultProfiles(false); updated != 0 || len(stale) != 1 || stale[0] != "rfid" {
		t.Fatalf("edited copy: updated=%d stale=%v", updated, stale)
	}
	if after, _ := readProfileFile(rfidPath); after.Image != "example.org/custom:rfid" {
		t.Fatal("the edited copy was overwritten")
	}
}
