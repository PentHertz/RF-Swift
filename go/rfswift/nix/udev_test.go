package nix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuleGroupsAndPackageName(t *testing.T) {
	content := []byte(`ATTR{idVendor}=="1d50", MODE="660", GROUP="plugdev"
ATTR{idVendor}=="2cf0", MODE="660", GROUP="bladerf"
SUBSYSTEM=="usb", MODE="666"
ATTR{idVendor}=="1d50", GROUP="plugdev"
`)
	groups := ruleGroups(content)
	if len(groups) != 2 || groups[0] != "bladerf" || groups[1] != "plugdev" {
		t.Fatalf("groups = %#v", groups)
	}
	if got := storePackageName("/nix/store/8g57hkdnh19kx6b67zlq7rb6312s1v6w-hackrf-2026.01.3/lib/udev/rules.d/53-hackrf.rules"); got != "hackrf-2026.01.3" {
		t.Fatalf("package = %q", got)
	}
	if got := storePackageName("/opt/rules/x.rules"); got != "/opt/rules/x.rules" {
		t.Fatalf("non-store path must be returned as is, got %q", got)
	}
}

func TestUdevRulesInAndState(t *testing.T) {
	installed := t.TempDir()
	udevInstalledDir = installed
	defer func() { udevInstalledDir = UdevRulesDir }()

	store := t.TempDir()
	pkg := filepath.Join(store, "abc-hackrf-1.0")
	os.MkdirAll(filepath.Join(pkg, "lib", "udev", "rules.d"), 0o755)
	rule := []byte(`ATTR{idVendor}=="1d50", MODE="660", GROUP="plugdev"` + "\n")
	os.WriteFile(filepath.Join(pkg, "lib", "udev", "rules.d", "53-hackrf.rules"), rule, 0o644)
	os.WriteFile(filepath.Join(pkg, "lib", "udev", "rules.d", "README"), []byte("no"), 0o644)

	profile := t.TempDir()
	os.MkdirAll(filepath.Join(profile, "lib", "udev", "rules.d"), 0o755)
	os.Symlink(filepath.Join(pkg, "lib", "udev", "rules.d", "53-hackrf.rules"), filepath.Join(profile, "lib", "udev", "rules.d", "53-hackrf.rules"))

	rules := udevRulesIn(profile)
	if len(rules) != 1 || rules[0].File != "53-hackrf.rules" || rules[0].State != "missing" {
		t.Fatalf("rules = %#v", rules)
	}
	if rules[0].Source != filepath.Join(pkg, "lib", "udev", "rules.d", "53-hackrf.rules") {
		t.Fatalf("source must be the resolved file, got %q", rules[0].Source)
	}
	if len(rules[0].Groups) != 1 || rules[0].Groups[0] != "plugdev" {
		t.Fatalf("groups = %#v", rules[0].Groups)
	}

	// Installed as shipped (with our header) -> installed; changed -> outdated.
	os.WriteFile(filepath.Join(installed, "53-hackrf.rules"), append([]byte(udevHeader("sdr", rules[0].Source)), rule...), 0o644)
	if got := udevRulesIn(profile)[0].State; got != "installed" {
		t.Fatalf("state = %q", got)
	}
	os.WriteFile(filepath.Join(installed, "53-hackrf.rules"), append([]byte(udevHeader("sdr", rules[0].Source)), []byte("old\n")...), 0o644)
	if got := udevRulesIn(profile)[0].State; got != "outdated" {
		t.Fatalf("state = %q", got)
	}
	// A distribution file without our header is compared by content too.
	os.WriteFile(filepath.Join(installed, "53-hackrf.rules"), rule, 0o644)
	if got := udevRulesIn(profile)[0].State; got != "installed" {
		t.Fatalf("identical distro file must count as installed, got %q", got)
	}

	os.WriteFile(filepath.Join(installed, "88-other.rules"), append([]byte(udevHeader("other", "/x")), rule...), 0o644)
	files, err := InstalledUdevRules("other")
	if err != nil || len(files) != 1 || files[0] != "88-other.rules" {
		t.Fatalf("installed for other = %#v err = %v", files, err)
	}
	if files, _ := InstalledUdevRules(""); len(files) != 1 {
		t.Fatalf("only files with our header count, got %#v", files)
	}
	if pending := PendingUdevRules([]UdevRule{{File: "a", State: "installed"}, {File: "b", State: "outdated"}, {File: "c", State: "missing"}}); len(pending) != 2 {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestUdevInstallScript(t *testing.T) {
	script, err := udevInstallScript(map[string]string{"53-hackrf.rules": "/tmp/stage/53-hackrf.rules"}, []string{"bladerf"}, []string{"bladerf", "plugdev"}, "o'neil")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"set -e\n",
		"install -m 0644 '/tmp/stage/53-hackrf.rules' '/etc/udev/rules.d/53-hackrf.rules'\n",
		"getent group bladerf >/dev/null || groupadd --system bladerf\n",
		"usermod -aG bladerf 'o'\\''neil'\n",
		"usermod -aG plugdev 'o'\\''neil'\n",
		"udevadm control --reload-rules\nudevadm trigger\n",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script lacks %q:\n%s", want, script)
		}
	}
	if _, err := udevInstallScript(map[string]string{"../evil.rules": "/x"}, nil, nil, ""); err == nil {
		t.Fatal("path-like file names must be refused")
	}
	if _, err := udevInstallScript(nil, []string{"bad group;rm"}, nil, ""); err == nil {
		t.Fatal("shell metacharacters in a group must be refused")
	}
	script, _ = udevInstallScript(nil, nil, []string{"plugdev"}, "")
	if strings.Contains(script, "usermod") {
		t.Fatal("no user known: no usermod")
	}
}
