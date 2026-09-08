package dock

import (
	"reflect"
	"testing"
)

func TestRemoveBindsAtTarget(t *testing.T) {
	got := removeBindsAtTarget([]string{"/old:/dev/ttyACM0", "/data:/data:ro"}, "/dev/ttyACM0")
	want := []string{"/data:/data:ro"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("removeBindsAtTarget() = %#v, want %#v", got, want)
	}
}

func TestRemoveDevicesAtTarget(t *testing.T) {
	got := removeDevicesAtTarget([]DeviceMapping{
		{PathOnHost: "/dev/old", PathInContainer: "/dev/ttyACM0"},
		{PathOnHost: "/dev/bus/usb", PathInContainer: "/dev/bus/usb"},
	}, "/dev/ttyACM0")
	want := []DeviceMapping{{PathOnHost: "/dev/bus/usb", PathInContainer: "/dev/bus/usb"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("removeDevicesAtTarget() = %#v, want %#v", got, want)
	}
}

func TestRemoveConfigDevicesAtTarget(t *testing.T) {
	keep := map[string]interface{}{"PathOnHost": "/dev/bus/usb", "PathInContainer": "/dev/bus/usb"}
	config := map[string]interface{}{"HostConfig": map[string]interface{}{"Devices": []interface{}{
		map[string]interface{}{"PathOnHost": "/dev/old", "PathInContainer": "/dev/ttyACM0"},
		keep,
	}}}
	removeConfigDevicesAtTarget(config, "/dev/ttyACM0")
	got := config["HostConfig"].(map[string]interface{})["Devices"].([]interface{})
	want := []interface{}{keep}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("removeConfigDevicesAtTarget() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCreationDevicesPromotesHotplugTrees(t *testing.T) {
	got, rules := normalizeCreationDevices(
		[]string{"/dev/bus/usb:/dev/bus/usb", "/dev/ttyACM0:/dev/ttyACM0"},
		[]string{"/data:/data:rw"}, []string{"c 166:* rwm"},
	)
	if !reflect.DeepEqual(got.binds, []string{"/data:/data:rw", "/dev/bus/usb:/dev/bus/usb:rw"}) {
		t.Fatalf("binds = %#v", got.binds)
	}
	if !reflect.DeepEqual(got.nodes, []string{"/dev/ttyACM0:/dev/ttyACM0"}) {
		t.Fatalf("nodes = %#v", got.nodes)
	}
	if !reflect.DeepEqual(rules, []string{"c 166:* rwm", "c 189:* rwm"}) {
		t.Fatalf("rules = %#v", rules)
	}
}
