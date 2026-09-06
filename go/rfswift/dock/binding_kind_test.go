package dock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBindingKindClassifiesDevPaths(t *testing.T) {
	SetPreferredEngine("docker")
	// A device node: mapped as a device, bind-mounted (with its rule) as a
	// volume or a device path, as chosen.
	if got, rules, err := resolveBindingKind("device", "/dev/null", true); err != nil || got != "device" || len(rules) != 0 {
		t.Errorf("device /dev/null -> %s %v %v", got, rules, err)
	}
	for _, kind := range []string{"volume", "device-bind"} {
		got, rules, err := resolveBindingKind(kind, "/dev/null", true)
		if err != nil || got != "volume" || len(rules) != 1 || rules[0] != "c 1:* rwm" {
			t.Errorf("%s /dev/null -> %s %v %v", kind, got, rules, err)
		}
	}
	// A tree stays a bind, with its rule.
	if _, err := os.Stat("/dev/bus/usb"); err == nil {
		got, rules, err := resolveBindingKind("volume", "/dev/bus/usb", true)
		if err != nil || got != "volume" || len(rules) != 1 || rules[0] != "c 189:* rwm" {
			t.Errorf("/dev/bus/usb -> %s %v %v", got, rules, err)
		}
		if _, _, err := resolveBindingKind("device", "/dev/bus/usb", true); err == nil {
			t.Error("a tree as a device mapping must be refused with advice")
		}
	}
	// Outside /dev: plain volume.
	if got, _, err := resolveBindingKind("device-bind", t.TempDir(), true); err != nil || got != "volume" {
		t.Errorf("directory -> %s %v", got, err)
	}
	// Absent device: refused; absent serial port: on demand where supported.
	if _, _, err := resolveBindingKind("volume", "/dev/rfswift-no-such-node", true); err == nil || !strings.Contains(err.Error(), "not present") {
		t.Errorf("absent device: %v", err)
	}
	got, _, err := resolveBindingKind("volume", "/dev/ttyACM9", true)
	if SerialHotplugSupported(GetEngine()) {
		if err != nil || got != "device" {
			t.Errorf("absent serial port with hot-plug: %s %v", got, err)
		}
	} else if err == nil {
		t.Error("absent serial port without hot-plug must be refused")
	}
	// Removals never need the path to exist.
	if got, _, err := resolveBindingKind("volume", "/dev/rfswift-no-such-node", false); err != nil || got != "volume" {
		t.Errorf("removal: %s %v", got, err)
	}
	if _, _, err := resolveBindingKind("volume", "", true); err == nil {
		t.Error("empty source must be refused")
	}
	stray := filepath.Join(t.TempDir(), "ttyACM0")
	_ = os.Mkdir(stray, 0o755)
	if got, _, err := resolveBindingKind("volume", stray, true); err != nil || got != "volume" {
		t.Errorf("a directory outside /dev is a plain volume: %s %v", got, err)
	}
}

func TestValidateConfigChangeBeforeElevation(t *testing.T) {
	SetPreferredEngine("docker")
	cases := map[string]ConfigChange{
		"no container":    {Kind: "volume", Source: "/tmp", Target: "/x", Add: true},
		"no target":       {Container: "c", Kind: "volume", Source: "/tmp", Add: true},
		"absent device":   {Container: "c", Kind: "volume", Source: "/dev/rfswift-no-such-node", Target: "/dev/x", Add: true},
		"empty cap":       {Container: "c", Kind: "capability", Add: true},
		"unknown setting": {Container: "c", Kind: "wifi", Add: true},
	}
	for name, c := range cases {
		if err := ValidateConfigChange(c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := ValidateConfigChange(ConfigChange{Container: "c", Kind: "volume", Source: "/dev/null", Target: "/dev/null", Add: true}); err != nil {
		t.Errorf("a device node as a volume must validate: %v", err)
	}
	if err := ValidateConfigChange(ConfigChange{Container: "c", Kind: "volume", Source: "/dev/null", Target: "/dev/null", Add: false}); err != nil {
		t.Errorf("removing a bind must validate: %v", err)
	}
	if err := ValidateConfigChange(ConfigChange{Container: "c", Kind: "capability", Value: "NET_ADMIN", Add: true}); err != nil {
		t.Errorf("capability: %v", err)
	}
}
