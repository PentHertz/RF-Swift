package dock

import (
	"strings"
	"testing"
)

// The rfid profile reaches a Proxmark3 through the hotplug-safe USB tree and
// the serial cgroup rules; a fixed /dev/ttyACM0 entry only works while the
// reader is plugged in at creation time and, on engines without serial
// hot-plug, fails the creation or leaves an empty directory in its place, so
// it must not be part of the default. The console (/dev/tty0) is mapped
// because the Proxmark client script refuses to start without it.
func TestRFIDProfileMapsConsoleNotSerialPort(t *testing.T) {
	var found bool
	for _, p := range DefaultProfiles() {
		if p.Name != "rfid" {
			continue
		}
		found = true
		if !strings.Contains(p.Devices, "/dev/tty0:/dev/tty0") {
			t.Fatalf("rfid profile must map the console: devices = %q", p.Devices)
		}
		if strings.Contains(p.Devices, "ttyACM") || strings.Contains(p.Bindings, "ttyACM") {
			t.Fatalf("a serial port must not be part of the default: devices=%q bindings=%q", p.Devices, p.Bindings)
		}
		if p.Bindings != usbTreeBinding || !strings.Contains(p.Cgroups, "c 189:* rwm") {
			t.Fatalf("rfid profile must keep the USB tree and cgroup rule: bindings=%q cgroups=%q", p.Bindings, p.Cgroups)
		}
		// With no port listed, the serial hot-plug is armed by the cgroup
		// rules alone (SyncSerialDevices creates the node, the rule lets the
		// container open it).
		for _, rule := range []string{"c 166:* rwm", "c 188:* rwm"} {
			if !strings.Contains(p.Cgroups, rule) {
				t.Fatalf("rfid profile must grant serial ports for the hot-plug: cgroups=%q lacks %s", p.Cgroups, rule)
			}
		}
	}
	if !found {
		t.Fatal("rfid profile missing from the defaults")
	}
}
