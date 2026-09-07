package dock

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// With Lima the /dev paths of a container live in the VM: the start
// preflight must ask the VM, not stat the Mac (where /dev/bus/usb never
// exists and every Lima mission with the USB tree was refused to start).
func TestPreflightVMDevicesAsksTheVM(t *testing.T) {
	defer pinDevicePathsBelongToVM(true)()
	defer pinVMPathInfo(map[string]VMPathInfo{
		"/dev/bus/usb": {Exists: true, IsDir: true, Major: -1},
		"/dev/ttyUSB0": {Exists: true, Device: true, Kind: "c", Major: 188},
	})()
	hc := &container.HostConfig{
		Binds:   []string{"/dev/bus/usb:/dev/bus/usb:rw", "/tmp/.X11-unix:/tmp/.X11-unix:rw", "/dev/snd:/dev/snd:rw"},
		Devices: []container.DeviceMapping{{PathOnHost: "/dev/ttyUSB0", PathInContainer: "/dev/ttyUSB0"}, {PathOnHost: "/dev/ttyACM0", PathInContainer: "/dev/ttyACM0"}},
	}
	problems := preflightVMDevices(hc)
	if len(problems) != 2 {
		t.Fatalf("problems = %v", problems)
	}
	if !strings.Contains(problems[0], "/dev/snd") || !strings.Contains(problems[0], "not present in the VM") {
		t.Errorf("missing bind: %s", problems[0])
	}
	if !strings.Contains(problems[1], "/dev/ttyACM0") {
		t.Errorf("missing device: %s", problems[1])
	}
	// VM unreachable: nothing to say, the start reports.
	defer pinVMPathInfo(nil)()
	if problems := preflightVMDevices(hc); len(problems) != 0 {
		t.Fatalf("unreachable VM must not refuse: %v", problems)
	}
}

// TestPreflightDevicesLimaLive (RFSWIFT_TEST_LIMA=1, Lima VM running with an
// RF Swift image available) creates a stopped container in the VM with the
// USB tree bound and checks the preflight lets it start, then refuses one
// binding a path absent from the VM.
func TestPreflightDevicesLimaLive(t *testing.T) {
	if os.Getenv("RFSWIFT_TEST_LIMA") == "" {
		t.Skip("set RFSWIFT_TEST_LIMA=1 with a running Lima VM")
	}
	SetPreferredEngine("lima")
	lima, ok := GetEngine().(*LimaEngine)
	if !ok || !lima.IsServiceRunning() {
		t.Skip("Lima VM not running")
	}
	ctx := context.Background()
	cli, err := lima.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	images, err := cli.ImageList(ctx, client.ImageListOptions{})
	if err != nil || len(images.Items) == 0 {
		t.Skip("no image in the VM")
	}
	image := images.Items[0].ID
	create := func(name string, binds []string) {
		_, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
			Name:       name,
			Config:     &container.Config{Image: image, Cmd: []string{"/bin/sh", "-c", "true"}},
			HostConfig: &container.HostConfig{Binds: binds, DeviceCgroupRules: []string{"c 189:* rwm"}},
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		t.Cleanup(func() { _, _ = cli.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true}) })
	}
	good := "rfswift-test-preflight-ok"
	bad := "rfswift-test-preflight-bad"
	create(good, []string{"/dev/bus/usb:/dev/bus/usb:rw"})
	create(bad, []string{"/dev/bus/usb:/dev/bus/usb:rw", "/dev/rfswift-no-such-node:/dev/rfswift-no-such-node:rw"})
	if err := PreflightDevices(ctx, cli, good); err != nil {
		t.Fatalf("the USB tree exists in the VM, start must be allowed: %v", err)
	}
	err = PreflightDevices(ctx, cli, bad)
	if err == nil || !strings.Contains(err.Error(), "/dev/rfswift-no-such-node") || !strings.Contains(err.Error(), "not present in the VM") {
		t.Fatalf("absent VM path must be refused with the reason: %v", err)
	}
	if strings.Contains(err.Error(), "/dev/bus/usb") {
		t.Fatalf("the present USB tree must not be reported: %v", err)
	}
}
