package dock

import (
	"reflect"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestFilterRootlessDevicesDropsBlockedAndUnopenable(t *testing.T) {
	devices := []container.DeviceMapping{
		{PathOnHost: "/dev/ttyACM0", PathInContainer: "/dev/ttyACM0", CgroupPermissions: "rwm"},
		{PathOnHost: "/dev/console", PathInContainer: "/dev/console", CgroupPermissions: "rwm"},
		{PathOnHost: "/dev/uinput", PathInContainer: "/dev/uinput", CgroupPermissions: "rwm"},
		{PathOnHost: "/dev/ttyUSB0", PathInContainer: "/dev/ttyUSB0", CgroupPermissions: "rwm"},
	}
	openable := func(path string) bool { return path == "/dev/ttyACM0" || path == "/dev/console" }
	kept, dropped := filterRootlessDevices(devices, openable)
	if len(kept) != 1 || kept[0].PathOnHost != "/dev/ttyACM0" {
		t.Fatalf("kept = %#v", kept)
	}
	if !reflect.DeepEqual(dropped, []string{"/dev/console", "/dev/uinput", "/dev/ttyUSB0"}) {
		t.Fatalf("dropped = %#v", dropped)
	}
}

func TestFilterRootlessDeviceBindsKeepsDirectoriesAndFiles(t *testing.T) {
	binds := []string{
		"/dev/bus/usb:/dev/bus/usb:rw",
		"/dev/vhci:/dev/vhci:rw",
		"/tmp/.X11-unix:/tmp/.X11-unix:rw",
		"/dev/root-only-node:/dev/node:rw",
		"/home/user/rfswift-workspace/x:/workspace:rw",
	}
	blocked := func(source string) bool {
		return rootlessBlockedDevices[source] || source == "/dev/root-only-node"
	}
	kept, dropped := filterRootlessDeviceBinds(binds, blocked)
	wantKept := []string{"/dev/bus/usb:/dev/bus/usb:rw", "/tmp/.X11-unix:/tmp/.X11-unix:rw", "/home/user/rfswift-workspace/x:/workspace:rw"}
	if !reflect.DeepEqual(kept, wantKept) {
		t.Fatalf("kept = %#v", kept)
	}
	if !reflect.DeepEqual(dropped, []string{"/dev/vhci", "/dev/root-only-node"}) {
		t.Fatalf("dropped = %#v", dropped)
	}
}

func TestRootlessBindSourceBlockedIgnoresDirectoriesAndMissingPaths(t *testing.T) {
	if rootlessBindSourceBlocked(t.TempDir()) {
		t.Fatal("a directory must never be treated as a blocked device node")
	}
	if rootlessBindSourceBlocked("/definitely/missing/path") {
		t.Fatal("a missing path is left for the engine to report")
	}
	if !rootlessBindSourceBlocked("/dev/vhci") {
		t.Fatal("/dev/vhci is on the blocked list regardless of host state")
	}
}

func TestFilterRootlessUlimitsDropsAboveHostHardLimit(t *testing.T) {
	limits := map[string]int64{"rtprio": 0, "memlock": 8388608, "nice": 0, "nofile": 1048576}
	hard := func(name string) (int64, bool) {
		v, ok := limits[name]
		return v, ok
	}
	ulimits := append(getRealtimeUlimits(),
		&container.Ulimit{Name: "nofile", Soft: 1024, Hard: 65536},
		&container.Ulimit{Name: "custom", Soft: 1, Hard: 2},
	)
	kept, dropped := filterRootlessUlimits(ulimits, hard)
	if len(kept) != 2 || kept[0].Name != "nofile" || kept[1].Name != "custom" {
		t.Fatalf("kept = %#v", kept)
	}
	if len(dropped) != 3 {
		t.Fatalf("dropped = %#v", dropped)
	}
	if !strings.HasPrefix(dropped[0], "rtprio=95:95 (host hard limit 0)") {
		t.Fatalf("dropped[0] = %q", dropped[0])
	}
	if !strings.Contains(dropped[1], "memlock=unlimited:unlimited") {
		t.Fatalf("dropped[1] = %q", dropped[1])
	}
}

func TestFilterRootlessUlimitsKeepsWhatTheHostAllows(t *testing.T) {
	hard := func(name string) (int64, bool) {
		switch name {
		case "rtprio":
			return 99, true
		case "memlock":
			return -1, true
		}
		return 0, false
	}
	kept, dropped := filterRootlessUlimits(getRealtimeUlimits(), hard)
	if len(dropped) != 0 {
		t.Fatalf("dropped = %#v", dropped)
	}
	if len(kept) != 3 {
		t.Fatalf("kept = %#v", kept)
	}
}

func TestRestrictRootlessPodmanHostConfigReportsEachChange(t *testing.T) {
	hc := &container.HostConfig{
		Binds: []string{"/dev/bus/usb:/dev/bus/usb:rw", "/dev/vhci:/dev/vhci:rw"},
		Devices: []container.DeviceMapping{
			{PathOnHost: "/dev/console", PathInContainer: "/dev/console", CgroupPermissions: "rwm"},
		},
	}
	hc.Resources.Ulimits = []*container.Ulimit{{Name: "custom", Soft: 1, Hard: 1}}
	var warnings []string
	restrictRootlessPodmanHostConfig(hc, func(msg string) { warnings = append(warnings, msg) })
	if len(hc.Devices) != 0 {
		t.Fatalf("devices = %#v", hc.Devices)
	}
	if !reflect.DeepEqual(hc.Binds, []string{"/dev/bus/usb:/dev/bus/usb:rw"}) {
		t.Fatalf("binds = %#v", hc.Binds)
	}
	if len(hc.Resources.Ulimits) != 1 {
		t.Fatalf("an unknown ulimit name must be kept: %#v", hc.Resources.Ulimits)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "/dev/console") || !strings.Contains(warnings[1], "/dev/vhci") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestRestrictRootlessPodmanKeepsGroupsOnlyWithCrun(t *testing.T) {
	orig := podmanKeepsGroups
	defer func() { podmanKeepsGroups = orig }()

	podmanKeepsGroups = func() bool { return true }
	hc := &container.HostConfig{}
	restrictRootlessPodmanHostConfig(hc, nil)
	if len(hc.GroupAdd) != 1 || hc.GroupAdd[0] != "keep-groups" {
		t.Fatalf("crun: GroupAdd = %v, want [keep-groups]", hc.GroupAdd)
	}

	podmanKeepsGroups = func() bool { return false }
	hc = &container.HostConfig{}
	restrictRootlessPodmanHostConfig(hc, nil)
	if len(hc.GroupAdd) != 0 {
		t.Fatalf("runc: GroupAdd = %v, want none (keep-groups would fail to start)", hc.GroupAdd)
	}

	podmanKeepsGroups = func() bool { return true }
	hc = &container.HostConfig{GroupAdd: []string{"audio"}}
	restrictRootlessPodmanHostConfig(hc, nil)
	if len(hc.GroupAdd) != 1 || hc.GroupAdd[0] != "audio" {
		t.Fatalf("explicit groups must be left alone, got %v", hc.GroupAdd)
	}
}
