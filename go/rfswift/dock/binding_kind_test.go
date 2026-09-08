package dock

import (
	"os"
	"path/filepath"
	"reflect"
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

// pinVMPathInfo makes the VM probe answer from a table (nil: VM unreachable).
func pinVMPathInfo(table map[string]VMPathInfo) func() {
	prev := vmPathInfoFn
	vmPathInfoFn = func(paths []string) (map[string]VMPathInfo, bool) {
		if table == nil {
			return nil, false
		}
		out := map[string]VMPathInfo{}
		for _, p := range paths {
			if info, ok := table[p]; ok {
				out[p] = info
			}
		}
		return out, true
	}
	return func() { vmPathInfoFn = prev }
}

// TestResolveBindingKindLimaVM: with Lima the /dev paths live in the VM, so
// the bindings dialog and `rfswift bindings add` must ask the VM instead of
// refusing every path as "not present on this host" (the macOS bug where
// /dev/bus/usb could not be added or removed from a mission).
func TestResolveBindingKindLimaVM(t *testing.T) {
	defer pinDevicePathsBelongToVM(true)()
	defer pinVMPathInfo(map[string]VMPathInfo{
		"/dev/bus/usb": {Exists: true, IsDir: true, Major: -1},
		"/dev/ttyACM0": {Exists: true, Device: true, Kind: "c", Major: 166},
		"/dev/etc":     {Exists: true, Major: -1},
	})()
	for _, kind := range []string{"volume", "device-bind"} {
		got, rules, err := resolveBindingKind(kind, "/dev/bus/usb", true)
		if err != nil || got != "volume" || len(rules) != 1 || rules[0] != "c 189:* rwm" {
			t.Errorf("%s /dev/bus/usb in the VM -> %s %v %v", kind, got, rules, err)
		}
	}
	if _, _, err := resolveBindingKind("device", "/dev/bus/usb", true); err == nil {
		t.Error("a VM directory as a device mapping must be refused with advice")
	}
	if got, rules, err := resolveBindingKind("device", "/dev/ttyACM0", true); err != nil || got != "device" || len(rules) != 0 {
		t.Errorf("device node in the VM -> %s %v %v", got, rules, err)
	}
	got, rules, err := resolveBindingKind("volume", "/dev/ttyACM0", true)
	if err != nil || got != "volume" || !reflect.DeepEqual(rules, []string{"c 166:* rwm", "c 188:* rwm", "c 204:* rwm"}) {
		t.Errorf("serial node bound in the VM -> %s %v %v", got, rules, err)
	}
	if _, _, err := resolveBindingKind("volume", "/dev/etc", true); err == nil {
		t.Error("a plain file in the VM must be refused")
	}
	if _, _, err := resolveBindingKind("volume", "/dev/ttyUSB0", true); err == nil || !strings.Contains(err.Error(), "not present in the VM") {
		t.Errorf("absent in the VM: %v", err)
	}
	// Removal never asks.
	if got, _, err := resolveBindingKind("volume", "/dev/ttyUSB0", false); err != nil || got != "volume" {
		t.Errorf("removal: %s %v", got, err)
	}
	// VM unreachable: taken as named, rules by name, nothing refused.
	defer pinVMPathInfo(nil)()
	if got, rules, err := resolveBindingKind("volume", "/dev/bus/usb", true); err != nil || got != "volume" || !reflect.DeepEqual(rules, []string{"c 189:* rwm"}) {
		t.Errorf("unreachable VM, USB tree -> %s %v %v", got, rules, err)
	}
	if got, _, err := resolveBindingKind("device", "/dev/ttyACM0", true); err != nil || got != "device" {
		t.Errorf("unreachable VM, device -> %s %v", got, err)
	}
	if _, _, err := resolveBindingKind("device", "/dev/bus/usb", true); err == nil {
		t.Error("unreachable VM: the USB tree as a device mapping is still refused")
	}
}

// TestNormalizeCreationDevicesLimaVM: a VM directory given under devices is
// promoted to a bind mount from the VM's answer, not a host stat.
func TestNormalizeCreationDevicesLimaVM(t *testing.T) {
	defer pinDevicePathsBelongToVM(true)()
	defer pinVMPathInfo(map[string]VMPathInfo{
		"/dev/serial":  {Exists: true, IsDir: true, Major: -1},
		"/dev/ttyACM0": {Exists: true, Device: true, Kind: "c", Major: 166},
	})()
	got, _ := normalizeCreationDevices([]string{"/dev/serial:/dev/serial", "/dev/ttyACM0:/dev/ttyACM0", "/dev/bus/usb:/dev/bus/usb"}, nil, nil)
	if !reflect.DeepEqual(got.binds, []string{"/dev/serial:/dev/serial:rw", "/dev/bus/usb:/dev/bus/usb:rw"}) {
		t.Fatalf("binds = %#v", got.binds)
	}
	if !reflect.DeepEqual(got.nodes, []string{"/dev/ttyACM0:/dev/ttyACM0"}) {
		t.Fatalf("nodes = %#v", got.nodes)
	}
}

func TestParseVMPathInfo(t *testing.T) {
	got := parseVMPathInfo("c a6 /dev/ttyACM0\nd - /dev/bus/usb\nf - /dev/etc\nbad line\n")
	if info := got["/dev/ttyACM0"]; !info.Device || info.Major != 166 || info.Rule() != "c 166:* rwm" {
		t.Errorf("ttyACM0 = %#v", info)
	}
	if info := got["/dev/bus/usb"]; !info.IsDir || info.Rule() != "" {
		t.Errorf("bus/usb = %#v", info)
	}
	if info := got["/dev/etc"]; !info.Exists || info.IsDir || info.Device {
		t.Errorf("etc = %#v", info)
	}
	if len(got) != 3 {
		t.Errorf("entries = %d", len(got))
	}
}
