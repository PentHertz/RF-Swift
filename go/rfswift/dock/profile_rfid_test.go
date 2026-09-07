package dock

import (
	"strings"
	"testing"
)

// The rfid profile reaches a Proxmark3 through the hotplug-safe USB tree and
// cgroup rule; a fixed /dev/ttyACM0 mapping only exists while the reader is
// plugged in at creation time and otherwise fails the container start, so it
// must not be part of the default.
func TestRFIDProfileMapsConsoleAndSerialPort(t *testing.T) {
	var found bool
	for _, p := range DefaultProfiles() {
		if p.Name != "rfid" {
			continue
		}
		found = true
		// The Proxmark client needs /dev/tty0; the port is handled by the
		// serial hot-plug (mapped when present, on demand when absent).
		if !strings.Contains(p.Devices, "/dev/tty0:/dev/tty0") || !strings.Contains(p.Devices, "/dev/ttyACM0:/dev/ttyACM0") {
			t.Fatalf("rfid profile devices = %q", p.Devices)
		}
		if strings.Contains(p.Bindings, "ttyACM") {
			t.Fatalf("a serial port is a device, not a bind mount: bindings=%q", p.Bindings)
		}
		if p.Bindings != usbTreeBinding || !strings.Contains(p.Cgroups, "c 189:* rwm") {
			t.Fatalf("rfid profile must keep the USB tree and cgroup rule: bindings=%q cgroups=%q", p.Bindings, p.Cgroups)
		}
	}
	if !found {
		t.Fatal("rfid profile missing from the defaults")
	}
}
