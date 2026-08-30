package dock

import (
	"reflect"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func fakeHost(goos string, engine EngineType, rootless bool, existing, openable map[string]bool) deviceCheckHost {
	return deviceCheckHost{
		goos: goos, engine: engine, rootless: rootless,
		exists:       func(p string) bool { return existing[p] },
		openable:     func(p string) bool { return openable[p] },
		isDeviceNode: func(p string) bool { return existing[p] && !strings.HasSuffix(p, "/") },
		vmExists:     func([]string) (map[string]bool, bool) { return nil, false },
	}
}

func TestDeviceEntriesFromSpecsCollectsDevicesAndDevBinds(t *testing.T) {
	entries := deviceEntriesFromSpecs(
		[]string{"/dev/ttyACM0:/dev/ttyACM0", "/dev/console:/dev/console:rwm", ""},
		[]string{"/dev/bus/usb:/dev/bus/usb:rw", "/home/user/data:/data", "/dev/vhci:/dev/vhci:rw"},
	)
	var paths []string
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	if !reflect.DeepEqual(paths, []string{"/dev/ttyACM0", "/dev/console", "/dev/bus/usb", "/dev/vhci"}) {
		t.Fatalf("paths = %#v", paths)
	}
	if !entries[2].Bind || entries[0].Bind {
		t.Fatalf("bind flags wrong: %#v", entries)
	}
}

func TestDeviceCheckRootlessPodmanOnLinux(t *testing.T) {
	existing := map[string]bool{"/dev/console": true, "/dev/ttyACM0": true, "/dev/ttyUSB0": true, "/dev/bus/usb/": true, "/dev/vhci": true}
	openable := map[string]bool{"/dev/ttyACM0": true}
	h := fakeHost("linux", EnginePodman, true, existing, openable)
	h.isDeviceNode = func(p string) bool { return p != "/dev/bus/usb" }
	existing["/dev/bus/usb"] = true
	entries := deviceEntriesFromSpecs(
		[]string{"/dev/console:/dev/console", "/dev/ttyACM0:/dev/ttyACM0", "/dev/ttyUSB0:/dev/ttyUSB0", "/dev/rfkill:/dev/rfkill"},
		[]string{"/dev/bus/usb:/dev/bus/usb:rw", "/dev/vhci:/dev/vhci:rw"},
	)
	issues, scope, advice := h.issues(entries)
	if scope != "host" || advice == "" {
		t.Fatalf("scope = %q advice = %q", scope, advice)
	}
	got := map[string]string{}
	for _, i := range issues {
		got[i.Path] = i.Reason
	}
	if len(got) != 4 {
		t.Fatalf("issues = %#v", issues)
	}
	if !strings.Contains(got["/dev/console"], "root-only") || !strings.Contains(got["/dev/vhci"], "root-only") {
		t.Fatalf("root-only reasons missing: %#v", got)
	}
	if !strings.Contains(got["/dev/ttyUSB0"], "not accessible") || got["/dev/rfkill"] != "not present on this host" {
		t.Fatalf("reasons = %#v", got)
	}
}

func TestDeviceCheckRootfulDockerOnLinuxOnlyFlagsMissing(t *testing.T) {
	h := fakeHost("linux", EngineDocker, false, map[string]bool{"/dev/console": true}, map[string]bool{})
	issues, scope, _ := h.issues(deviceEntriesFromSpecs([]string{"/dev/console:/dev/console", "/dev/ttyACM0:/dev/ttyACM0"}, nil))
	if scope != "host" || len(issues) != 1 || issues[0].Path != "/dev/ttyACM0" {
		t.Fatalf("issues = %#v scope = %q", issues, scope)
	}
}

func TestDeviceCheckDockerOnMacFlagsPassthroughDevices(t *testing.T) {
	h := fakeHost("darwin", EngineDocker, false, map[string]bool{}, map[string]bool{})
	issues, scope, advice := h.issues(deviceEntriesFromSpecs(
		[]string{"/dev/console:/dev/console", "/dev/ttyACM0:/dev/ttyACM0"},
		[]string{"/dev/bus/usb:/dev/bus/usb:rw"},
	))
	if scope != "vm" || len(issues) != 2 || !strings.Contains(advice, "lima") {
		t.Fatalf("issues = %#v scope = %q advice = %q", issues, scope, advice)
	}
	if issues[0].Path != "/dev/ttyACM0" || !issues[1].Bind {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestDeviceCheckLimaUsesTheVM(t *testing.T) {
	h := fakeHost("darwin", EngineLima, false, map[string]bool{}, map[string]bool{})
	h.vmExists = func(paths []string) (map[string]bool, bool) { return map[string]bool{"/dev/bus/usb": true}, true }
	issues, scope, _ := h.issues(deviceEntriesFromSpecs([]string{"/dev/ttyACM0:/dev/ttyACM0"}, []string{"/dev/bus/usb:/dev/bus/usb:rw"}))
	if scope != "vm" || len(issues) != 1 || issues[0].Reason != "not present in the Lima VM" {
		t.Fatalf("issues = %#v scope = %q", issues, scope)
	}
	h.vmExists = func([]string) (map[string]bool, bool) { return nil, false }
	if issues, scope, _ := h.issues(deviceEntriesFromSpecs([]string{"/dev/ttyACM0:/dev/ttyACM0"}, nil)); scope != "none" || len(issues) != 0 {
		t.Fatalf("an unreachable VM must not produce issues: %#v %q", issues, scope)
	}
}

func TestRemoveDeviceIssuesDropsOnlyTheReportedMappings(t *testing.T) {
	hc := &container.HostConfig{
		Binds: []string{"/dev/bus/usb:/dev/bus/usb:rw", "/dev/vhci:/dev/vhci:rw", "/tmp/.X11-unix:/tmp/.X11-unix:rw"},
		Devices: []container.DeviceMapping{
			{PathOnHost: "/dev/console", PathInContainer: "/dev/console"},
			{PathOnHost: "/dev/ttyACM0", PathInContainer: "/dev/ttyACM0"},
		},
	}
	removeDeviceIssues(hc, []DeviceIssue{{Path: "/dev/console"}, {Path: "/dev/vhci", Bind: true}})
	if len(hc.Devices) != 1 || hc.Devices[0].PathOnHost != "/dev/ttyACM0" {
		t.Fatalf("devices = %#v", hc.Devices)
	}
	if !reflect.DeepEqual(hc.Binds, []string{"/dev/bus/usb:/dev/bus/usb:rw", "/tmp/.X11-unix:/tmp/.X11-unix:rw"}) {
		t.Fatalf("binds = %#v", hc.Binds)
	}
}
