package dock

import "testing"

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
	if err := ValidateConfigChange(ConfigChange{Container: "c", Kind: "serial-hotplug", Add: false}); err != nil {
		t.Errorf("the switch needs no value: %v", err)
	}
}
