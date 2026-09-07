package dock

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func rfidDefault(t *testing.T) Profile {
	t.Helper()
	for _, p := range DefaultProfiles() {
		if p.Name == "rfid" {
			return p
		}
	}
	t.Fatal("rfid profile missing from the defaults")
	return Profile{}
}

// Every rfid layout an older release wrote (bind-mounted port, mapped port,
// USB tree alone) is an unedited built-in: it is refreshed, and the refreshed
// copy no longer carries the fixed /dev/ttyACM0.
func TestInitDefaultProfilesRefreshesEveryOlderRFIDLayout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the profiles directory comes from APPDATA there")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	dir := ProfilesDirByPlatform()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := rfidDefault(t)
	layouts := previousDefaults(def)
	if len(layouts) < 4 {
		t.Fatalf("expected the four older rfid layouts, got %d", len(layouts))
	}
	rfidPath := filepath.Join(dir, profileFilename("rfid"))
	for i, old := range layouts {
		data, err := yaml.Marshal(&old)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rfidPath, data, 0o644); err != nil {
			t.Fatal(err)
		}
		_, updated, _, stale := InitDefaultProfiles(false)
		if updated != 1 || len(stale) != 0 {
			t.Fatalf("layout %d (%+v): updated=%d stale=%v", i, old, updated, stale)
		}
		stored, err := readProfileFile(rfidPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stored.Devices, "ttyACM") || strings.Contains(stored.Bindings, "ttyACM") || !strings.Contains(stored.Devices, "/dev/tty0") {
			t.Fatalf("layout %d not brought to the current default: %+v", i, stored)
		}
	}
}

// The CLI (`run --profile`, the TUI wizard) reads the stored copy without
// running `profile init`: an unedited older copy is served as the current
// built-in, the file is left as it is, and an edited copy comes back as
// written.
func TestLoadProfilesServesCurrentDefaultForUneditedOlderCopy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the profiles directory comes from APPDATA there")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	dir := ProfilesDirByPlatform()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := rfidDefault(t)
	old := previousDefaults(def)[2] // the port as a device mapping
	if !strings.Contains(old.Devices, "/dev/ttyACM0") {
		t.Fatalf("fixture layout = %+v", old)
	}
	data, err := yaml.Marshal(&old)
	if err != nil {
		t.Fatal(err)
	}
	rfidPath := filepath.Join(dir, profileFilename("rfid"))
	if err := os.WriteFile(rfidPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := GetProfileByName("rfid")
	if err != nil {
		t.Fatal(err)
	}
	if !sameProfile(*got, def) {
		t.Fatalf("unedited older copy served as written: %+v", *got)
	}
	if after, _ := os.ReadFile(rfidPath); string(after) != string(data) {
		t.Fatal("loading a profile must not rewrite the file")
	}

	edited := old
	edited.Image = "example.org/custom:rfid"
	if data, err = yaml.Marshal(&edited); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rfidPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err = GetProfileByName("rfid"); err != nil || !sameProfile(*got, edited) {
		t.Fatalf("edited copy must come back as written: %+v %v", got, err)
	}
}
