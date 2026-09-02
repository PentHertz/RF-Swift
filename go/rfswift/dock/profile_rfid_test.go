package dock

import (
	"strings"
	"testing"
)

// The rfid profile reaches a Proxmark3 through the hotplug-safe USB tree and
// cgroup rule; a fixed /dev/ttyACM0 mapping only exists while the reader is
// plugged in at creation time and otherwise fails the container start, so it
// must not be part of the default.
func TestRFIDProfileHasNoFixedSerialNode(t *testing.T) {
	var found bool
	for _, p := range DefaultProfiles() {
		if p.Name != "rfid" {
			continue
		}
		found = true
		if strings.Contains(p.Devices, "ttyACM") || strings.Contains(p.Bindings, "ttyACM") {
			t.Fatalf("rfid profile still maps a fixed serial node: devices=%q bindings=%q", p.Devices, p.Bindings)
		}
		if p.Bindings != usbTreeBinding || !strings.Contains(p.Cgroups, "c 189:* rwm") {
			t.Fatalf("rfid profile must keep the USB tree and cgroup rule: bindings=%q cgroups=%q", p.Bindings, p.Cgroups)
		}
	}
	if !found {
		t.Fatal("rfid profile missing from the defaults")
	}
}
