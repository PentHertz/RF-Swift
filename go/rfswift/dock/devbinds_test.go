package dock

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStrayDeviceDirDetection(t *testing.T) {
	dev := t.TempDir()
	mk := func(name string) string {
		p := filepath.Join(dev, name)
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stray := mk("ttyACM0")
	stray2 := mk("hidraw3")
	tree := mk("bus")
	if err := os.WriteFile(filepath.Join(tree, "usb"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	full := mk("ttyUSB0")
	if err := os.WriteFile(filepath.Join(full, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mk("snd")
	if err := os.WriteFile(filepath.Join(dev, "video0"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !isStrayDeviceDirIn(dev, stray) || !isStrayDeviceDirIn(dev, stray2) {
		t.Error("empty device-named directories must be recognised")
	}
	for _, p := range []string{tree, full, filepath.Join(dev, "snd"), filepath.Join(dev, "video0"), filepath.Join(dev, "missing0")} {
		if isStrayDeviceDirIn(dev, p) {
			t.Errorf("%s must not count as stray", p)
		}
	}
	if isStrayDeviceDirIn(dev, filepath.Join(dev, "sub", "ttyACM0")) {
		t.Error("only direct children of the device root count")
	}
	got := strayDeviceDirsIn(dev)
	if len(got) != 2 || got[0] != stray2 || got[1] != stray {
		t.Errorf("stray list = %v", got)
	}
	if strayDeviceDirsIn(filepath.Join(dev, "nope")) != nil {
		t.Error("a missing device root lists nothing")
	}
}

func TestSanitizeDeviceBinds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("device nodes are a Unix notion")
	}
	binds := []string{
		"/home/user/work:/root/work:rw",
		"/dev/null:/dev/null",
		"/dev/rfswift-test-no-such-device-0:/dev/ttyACM0",
		"/dev/shm:/dev/shm",
		"not-a-bind",
	}
	kept, devices, rules, warnings := SanitizeDeviceBinds(binds)
	if len(kept) != 4 || kept[0] != binds[0] || kept[1] != "/dev/null:/dev/null" || kept[2] != "/dev/shm:/dev/shm" || kept[3] != "not-a-bind" {
		t.Errorf("kept = %v", kept)
	}
	if len(devices) != 0 {
		t.Errorf("a bind mount of a device node stays a bind mount, got mappings %v", devices)
	}
	if len(rules) != 1 || rules[0] != "c 1:* rwm" {
		t.Errorf("the node's cgroup rule must come with it, got %v", rules)
	}
	if len(warnings) != 1 || warnings[0].Reason != "missing" || warnings[0].Path != "/dev/rfswift-test-no-such-device-0" {
		t.Fatalf("warnings = %+v", warnings)
	}
	if !strings.Contains(warnings[0].String(), "created a directory") {
		t.Errorf("the warning must say what the engine would have done: %s", warnings[0])
	}
	if got := DeviceRuleFor("/dev/null"); got != "c 1:* rwm" {
		t.Errorf("DeviceRuleFor(/dev/null) = %q", got)
	}
	if got := DeviceRuleFor("/dev/rfswift-no-such-node"); got != "" {
		t.Errorf("a missing node has no rule, got %q", got)
	}
}

func TestRemoveStrayDeviceDirsRefusesForeignPaths(t *testing.T) {
	for _, p := range []string{"/etc", "/dev/bus", "/tmp/ttyACM0", "/dev/ttyACM0/../..", ""} {
		if err := RemoveStrayDeviceDirs([]string{p}); err == nil {
			t.Errorf("%q accepted for removal", p)
		}
	}
	if err := RemoveStrayDeviceDirs(nil); err != nil {
		t.Errorf("nothing to remove must be fine: %v", err)
	}
}

func TestSerialPortsAreRequired(t *testing.T) {
	for p, want := range map[string]bool{"/dev/ttyACM0": true, "/dev/ttyUSB1": true, "/dev/ttyAMA0": true, "/dev/ttyS0": false, "/dev/tty0": false, "/dev/tty": false, "/dev/rfkill": false, "/dev/bus/usb": false, "/dev/ttyACM0/": false} {
		if IsSerialDevicePath(p) != want {
			t.Errorf("IsSerialDevicePath(%q) = %v", p, !want)
		}
	}
	warnings := []DevBindWarning{{Path: "/dev/rfkill", Reason: "missing", Advice: "not present"}}
	if err := RequiredDevicesError(warnings, []string{"/dev/net/tun"}); err != nil {
		t.Errorf("optional devices must stay warnings, got %v", err)
	}
	err := RequiredDevicesError(warnings, []string{"/dev/ttyACM0"})
	if err == nil || !strings.Contains(err.Error(), "/dev/ttyACM0") || !strings.Contains(err.Error(), "Plug the device in") {
		t.Errorf("a missing serial port must stop creation with the notice, got %v", err)
	}
	err = RequiredDevicesError([]DevBindWarning{{Path: "/dev/ttyUSB0", Reason: "stray-directory", Advice: "is an empty directory"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "empty directory") {
		t.Errorf("a stray serial directory must stop creation with the cleanup hint, got %v", err)
	}
}
