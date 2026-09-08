package workbench

import (
	"testing"

	rfdock "penthertz/rfswift/dock"
)

func TestNormalizeLegacyProfileDropsRFIDSerialNode(t *testing.T) {
	// The exact rfid layout older RF Swift releases wrote to the user's YAML:
	// the serial node under bindings, no devices, no cgroup rule.
	old := rfdock.Profile{Name: "rfid", Bindings: "/dev/bus/usb:/dev/bus/usb,/dev/ttyACM0:/dev/ttyACM0"}
	got := normalizeLegacyProfile("rfid", old)
	if got.Bindings != "/dev/bus/usb:/dev/bus/usb" || got.Devices != "" {
		t.Fatalf("serial node must be dropped: bindings=%q devices=%q", got.Bindings, got.Devices)
	}
	if got.Cgroups != "c 189:* rwm" {
		t.Fatalf("USB cgroup rule must be restored: %q", got.Cgroups)
	}
	// The intermediate layout (node under devices) is normalised the same way,
	// and a user's own cgroup rules are kept.
	mid := rfdock.Profile{Name: "rfid", Bindings: "/dev/bus/usb:/dev/bus/usb", Devices: "/dev/ttyACM0:/dev/ttyACM0", Cgroups: "c 189:* rwm,c 166:* rwm"}
	got = normalizeLegacyProfile("rfid", mid)
	if got.Devices != "" || got.Cgroups != "c 189:* rwm,c 166:* rwm" {
		t.Fatalf("mid layout: devices=%q cgroups=%q", got.Devices, got.Cgroups)
	}
	// Other profiles are untouched, so a user who wants the node keeps it.
	custom := rfdock.Profile{Name: "proxmark", Devices: "/dev/ttyACM0:/dev/ttyACM0"}
	if got := normalizeLegacyProfile("proxmark", custom); got.Devices != custom.Devices {
		t.Fatalf("custom profile changed: %#v", got)
	}
}
