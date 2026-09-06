package dock

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The script runs for real against a directory standing in for /dev: a
// symlink to /dev/null plays a port whose node already exists (the kernel's
// device number does not matter for the test), a plain file plays a stale
// node of a port that was unplugged.
func TestSerialSyncScriptKeepsPresentAndPrunesStale(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	dev := t.TempDir()
	if err := os.Symlink("/dev/null", filepath.Join(dev, "ttyACM0")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dev, "ttyUSB3"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dev, "ttyS0"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	script := buildSerialSyncScript(dev, []serialNode{{Path: "/dev/ttyACM0", Major: 166, Minor: 0}})
	out, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s\n%s", err, out, script)
	}
	if _, err := os.Lstat(filepath.Join(dev, "ttyACM0")); err != nil {
		t.Fatalf("the present port's node was removed:\n%s", out)
	}
	if _, err := os.Lstat(filepath.Join(dev, "ttyUSB3")); err == nil {
		t.Fatalf("the stale node was kept:\n%s", out)
	}
	if _, err := os.Lstat(filepath.Join(dev, "ttyS0")); err != nil {
		t.Fatal("an on-board UART node is none of the script's business")
	}
	if strings.Contains(string(out), "created") || !strings.Contains(string(out), "removed "+filepath.Join(dev, "ttyUSB3")) {
		t.Errorf("unexpected report:\n%s", out)
	}
	// Nothing present on the host: every hot-pluggable node goes, nothing is
	// created.
	if err := os.WriteFile(filepath.Join(dev, "ttyUSB3"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	empty := buildSerialSyncScript(dev, nil)
	if _, err := exec.Command("sh", "-c", empty).CombinedOutput(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dev, "ttyUSB3")); err == nil {
		t.Fatal("stale node kept when the host has no port")
	}
	if _, err := os.Lstat(filepath.Join(dev, "ttyACM0")); err == nil {
		t.Fatal("a node with no port on the host must go")
	}
}

func TestSerialSyncScriptCreatesMissingNodes(t *testing.T) {
	script := buildSerialSyncScript("/dev", []serialNode{{Path: "/dev/ttyACM0", Major: 166, Minor: 0}, {Path: "/dev/ttyUSB1", Major: 188, Minor: 1}})
	for _, want := range []string{"mknod -m 660 /dev/ttyACM0 c 166 0", "mknod -m 660 /dev/ttyUSB1 c 188 1", "[ ! -c /dev/ttyACM0 ]", `keep="$keep /dev/ttyACM0"`} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "'") {
		t.Error("quotes in the keep list break the match against the glob")
	}
}

func TestSerialSpecsSplit(t *testing.T) {
	specs := []string{"/dev/ttyACM0:/dev/ttyACM0", "/dev/bus/usb:/dev/bus/usb", "/dev/ttyUSB0:/dev/ttyUSB0:rwm", "", "/dev/rfkill:/dev/rfkill"}
	if ports := serialPortsIn(specs); len(ports) != 2 || ports[0] != "/dev/ttyACM0" || ports[1] != "/dev/ttyUSB0" {
		t.Errorf("ports = %v", ports)
	}
	// Neither port exists on this test host, so both are absent: on demand.
	if devicePresent("/dev/ttyACM0") || devicePresent("/dev/ttyUSB0") {
		t.Skip("a real serial port is plugged into the test host")
	}
	serial, rest := serialSpecs(specs)
	if len(serial) != 2 || serial[0] != "/dev/ttyACM0:/dev/ttyACM0" || serial[1] != "/dev/ttyUSB0:/dev/ttyUSB0:rwm" {
		t.Errorf("serial = %v", serial)
	}
	if len(rest) != 3 || rest[0] != "/dev/bus/usb:/dev/bus/usb" || rest[2] != "/dev/rfkill:/dev/rfkill" {
		t.Errorf("rest = %v", rest)
	}
	if got := appendMissing([]string{"c 189:* rwm", " c 166:* rwm "}, SerialCgroupRules...); len(got) != 4 {
		t.Errorf("appendMissing = %v", got)
	}
}
