package rfutils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValueAndNixWSLDistro(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.ini")
	content := "[general]\nimagename = x\nengine = Nix\n\n[nix]\n# the distro\nwsl_distro = Ubuntu-24.04\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ConfiguredEngine(path); got != "nix" {
		t.Fatalf("ConfiguredEngine = %q", got)
	}
	if got := ConfiguredNixWSLDistro(path); got != "Ubuntu-24.04" {
		t.Fatalf("ConfiguredNixWSLDistro = %q", got)
	}
	if got := ConfiguredNixWSLDistro(filepath.Join(t.TempDir(), "missing.ini")); got != "" {
		t.Fatalf("missing file must give %q, got %q", "", got)
	}
}

func TestSetConfigValueReplacesAppendsAndCreatesSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.ini")
	if err := os.WriteFile(path, []byte("[general]\nengine = auto\n\n[audio]\npulse_server = tcp:localhost:34567\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// New section appended at the end.
	if err := SetConfigValue(path, "nix", "wsl_distro", "Debian"); err != nil {
		t.Fatal(err)
	}
	if got := ConfiguredNixWSLDistro(path); got != "Debian" {
		t.Fatalf("after create: %q", got)
	}
	// Existing key replaced in place, nothing duplicated.
	if err := SetConfigValue(path, "nix", "wsl_distro", "Ubuntu"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "wsl_distro") != 1 || ConfiguredNixWSLDistro(path) != "Ubuntu" {
		t.Fatalf("after replace:\n%s", data)
	}
	// Key added to an existing section that is not the last one.
	if err := SetConfigValue(path, "general", "repotag", "penthertz/rfswift_resolute"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	text := string(data)
	general := text[strings.Index(text, "[general]"):strings.Index(text, "[audio]")]
	if !strings.Contains(general, "repotag = penthertz/rfswift_resolute") || !strings.Contains(general, "engine = auto") {
		t.Fatalf("repotag must land in [general]:\n%s", text)
	}
	if ConfiguredEngine(path) != "auto" || ConfiguredNixWSLDistro(path) != "Ubuntu" {
		t.Fatalf("other values disturbed:\n%s", text)
	}
	// A missing file is created with the defaults first.
	fresh := filepath.Join(t.TempDir(), "sub", "config.ini")
	if err := SetConfigValue(fresh, "nix", "wsl_distro", "Ubuntu-24.04"); err != nil {
		t.Fatal(err)
	}
	if ConfiguredNixWSLDistro(fresh) != "Ubuntu-24.04" || ConfiguredEngine(fresh) != "auto" {
		t.Fatalf("fresh config: distro=%q engine=%q", ConfiguredNixWSLDistro(fresh), ConfiguredEngine(fresh))
	}
}
