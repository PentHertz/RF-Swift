package dock

import (
	"strings"
	"testing"
)

func TestSerialHotplugEnabled(t *testing.T) {
	rules := []string{"c 189:* rwm", "c 166:* rwm", "c 188:* rwm", "c 204:* rwm"}
	if !SerialHotplugEnabled(rules, nil) {
		t.Error("rules present, no label: on")
	}
	if SerialHotplugEnabled(rules, map[string]string{SerialHotplugLabel: "off"}) {
		t.Error("label off wins over the rules")
	}
	if SerialHotplugEnabled([]string{"c 189:* rwm"}, map[string]string{}) {
		t.Error("no serial rule: off")
	}
	SetPreferredEngine("docker")
	err := ValidateConfigChange(ConfigChange{Container: "c", Kind: "serial-hotplug", Add: false})
	if SerialHotplugSupported(GetEngine()) {
		if err != nil {
			t.Errorf("the switch needs no value: %v", err)
		}
	} else if err == nil {
		t.Error("the switch must be refused where hot-plug does not work")
	}
}

// TestSerialHotplugNotOfferedWithoutSupport: the Lima VM and rootless Podman
// cannot hot-plug, so the switch is refused with the reason and the summary
// never reports it "on" (the serial cgroup rules a mission carries are not
// hot-plug there).
func TestSerialHotplugNotOfferedWithoutSupport(t *testing.T) {
	if serialHotplugSupportedType(EngineLima) {
		t.Error("Lima must not report serial hot-plug support")
	}
	err := SerialHotplugUnavailableError(&LimaEngine{})
	if err == nil || !strings.Contains(err.Error(), "Lima") {
		t.Errorf("Lima refusal must say why: %v", err)
	}
	if !strings.Contains(err.Error(), "macusb attach") {
		t.Errorf("Lima refusal must say what to do instead: %v", err)
	}
	SetPreferredEngine("lima")
	if err := ValidateConfigChange(ConfigChange{Container: "c", Kind: "serial-hotplug", Add: true}); err == nil {
		t.Error("validation must refuse the switch on Lima before any container is touched")
	}
	SetPreferredEngine("docker")
}
