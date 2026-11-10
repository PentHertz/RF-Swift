package dock

import (
	"reflect"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestDropTTYConsoleDevice(t *testing.T) {
	hc := &container.HostConfig{Resources: container.Resources{Devices: []container.DeviceMapping{
		{PathOnHost: "/dev/console", PathInContainer: "/dev/console", CgroupPermissions: "rwm"},
		{PathOnHost: "/dev/ttyACM0", PathInContainer: "/dev/ttyACM0", CgroupPermissions: "rwm"},
		{PathOnHost: "/dev/ttyS0", PathInContainer: "/dev/console/", CgroupPermissions: "rwm"},
		{PathOnHost: "/dev/console", PathInContainer: "/dev/hostconsole", CgroupPermissions: "rwm"},
		{PathOnHost: "/dev/console", CgroupPermissions: "rwm"},
	}}}
	if !dropTTYConsoleDevice(hc) {
		t.Fatal("expected a /dev/console mapping to be removed")
	}
	want := []container.DeviceMapping{
		{PathOnHost: "/dev/ttyACM0", PathInContainer: "/dev/ttyACM0", CgroupPermissions: "rwm"},
		{PathOnHost: "/dev/console", PathInContainer: "/dev/hostconsole", CgroupPermissions: "rwm"},
	}
	if !reflect.DeepEqual(hc.Devices, want) {
		t.Fatalf("devices = %+v, want %+v", hc.Devices, want)
	}

	if dropTTYConsoleDevice(hc) {
		t.Fatal("second pass removed something")
	}
	if dropTTYConsoleDevice(nil) {
		t.Fatal("nil host config reported a removal")
	}
}
