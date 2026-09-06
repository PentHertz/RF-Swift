//go:build linux

package dock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// Drives the functions the Workbench's Configure dialog calls against a real
// container, as a plain user (the re-create path). Opt in with
// RFSWIFT_TEST_DOCKER=1 on a host with a rootful Docker daemon and a local
// image (RFSWIFT_TEST_IMAGE, default ubuntu:24.04).
func TestLiveConfigureRecreatesContainer(t *testing.T) {
	if os.Getenv("RFSWIFT_TEST_DOCKER") == "" {
		t.Skip("set RFSWIFT_TEST_DOCKER=1 to run against a Docker daemon")
	}
	if os.Geteuid() == 0 {
		t.Skip("this test covers the non-root path")
	}
	SetPreferredEngine("docker")
	// The file edit needs root; the re-create path is what a test can drive.
	SetConfigEditMode(EditModeRecreate)
	defer SetConfigEditMode(EditModeAuto)
	image := os.Getenv("RFSWIFT_TEST_IMAGE")
	if image == "" {
		image = "ubuntu:24.04"
	}
	SetPreferredEngine("docker")
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		t.Skipf("no engine: %v", err)
	}
	defer cli.Close()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "rfswift-cfg-test-" + hex.EncodeToString(b)
	labels := map[string]string{"org.container.project": "rfswift", "org.rfswift.original_image": image, "org.rfswift.exposed_ports": "none", "org.rfswift.cgroup_rules": "c 189:* rwm"}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     &container.Config{Image: image, Cmd: []string{"/bin/sh"}, OpenStdin: true, Tty: true, Labels: labels},
		HostConfig: &container.HostConfig{Binds: []string{"/tmp:/mnt/hosttmp"}, DeviceCgroupRules: []string{"c 189:* rwm"}},
		Name:       name,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() {
		_, _ = cli.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true})
	}()
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	inspect := func() container.InspectResponse {
		res, err := cli.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
		if err != nil {
			t.Fatalf("inspect %s: %v", name, err)
		}
		return res.Container
	}
	has := func(items []string, want string) bool {
		for _, it := range items {
			if strings.HasPrefix(it, want) {
				return true
			}
		}
		return false
	}

	// 1. Bind mount added from the GUI form.
	dir := t.TempDir()
	if err := UpdateBinding(name, "volume", dir, "/opt/added", true); err != nil {
		t.Fatalf("add bind: %v", err)
	}
	c := inspect()
	if !has(c.HostConfig.Binds, dir+":/opt/added") || !has(c.HostConfig.Binds, "/tmp:/mnt/hosttmp") {
		t.Fatalf("binds after add = %v", c.HostConfig.Binds)
	}
	if c.ID == created.ID {
		t.Fatal("the container was not re-created")
	}
	if c.State == nil || !c.State.Running {
		t.Fatalf("a running container must be running again after the change, state = %+v", c.State)
	}
	if c.Config.Labels["org.rfswift.cgroup_rules"] == "" || !has(c.HostConfig.DeviceCgroupRules, "c 189:* rwm") {
		t.Errorf("cgroup rules lost: %v / %q", c.HostConfig.DeviceCgroupRules, c.Config.Labels["org.rfswift.cgroup_rules"])
	}

	// 2. Capability add and remove.
	if err := UpdateCapability(name, "NET_ADMIN", true); err != nil {
		t.Fatalf("add cap: %v", err)
	}
	hasCap := func(caps []string, want string) bool {
		for _, c := range caps {
			if NormalizeCapName(c) == want {
				return true
			}
		}
		return false
	}
	if c = inspect(); !hasCap(c.HostConfig.CapAdd, "NET_ADMIN") {
		t.Fatalf("caps after add = %v", c.HostConfig.CapAdd)
	}
	if err := UpdateCapability(name, "NET_ADMIN", false); err != nil {
		t.Fatalf("remove cap: %v", err)
	}
	if c = inspect(); hasCap(c.HostConfig.CapAdd, "NET_ADMIN") {
		t.Fatalf("caps after remove = %v", c.HostConfig.CapAdd)
	}

	// 3. A device node mapped from the GUI form.
	if err := UpdateBinding(name, "device", "/dev/null", "/dev/rfswift-null", true); err != nil {
		t.Fatalf("add device: %v", err)
	}
	c = inspect()
	found := false
	for _, d := range c.HostConfig.Devices {
		if d.PathOnHost == "/dev/null" && d.PathInContainer == "/dev/rfswift-null" {
			found = true
		}
	}
	if !found {
		t.Fatalf("devices after add = %+v", c.HostConfig.Devices)
	}

	// 3b. A device node chosen as a bind mount stays one, with its rule.
	if err := UpdateBinding(name, "volume", "/dev/null", "/dev/rfswift-volnull", true); err != nil {
		t.Fatalf("add device node as volume: %v", err)
	}
	c = inspect()
	if !has(c.HostConfig.Binds, "/dev/null:/dev/rfswift-volnull") || !has(c.HostConfig.DeviceCgroupRules, "c 1:* rwm") {
		t.Fatalf("node as volume: binds=%v rules=%v", c.HostConfig.Binds, c.HostConfig.DeviceCgroupRules)
	}

	// 4. A serial port that is not plugged in: accepted, attached on demand.
	if err := UpdateBinding(name, "device", "/dev/ttyACM7", "/dev/ttyACM7", true); err != nil {
		t.Fatalf("add absent serial port: %v", err)
	}
	c = inspect()
	for _, d := range c.HostConfig.Devices {
		if IsSerialDevicePath(d.PathOnHost) {
			t.Fatalf("serial port must not be a fixed mapping: %+v", c.HostConfig.Devices)
		}
	}
	for _, rule := range SerialCgroupRules {
		if !has(c.HostConfig.DeviceCgroupRules, rule) {
			t.Errorf("serial rule %q missing: %v", rule, c.HostConfig.DeviceCgroupRules)
		}
	}
	summary, err := ContainerSummaryFor(ctx, cli, name)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.SerialHotplug {
		t.Errorf("summary must show serial ports as attached on demand: %+v", summary)
	}
	if len(summary.SerialPorts) != 1 || summary.SerialPorts[0] != "/dev/ttyACM7" {
		t.Errorf("the configured port must be recorded and listed: %+v", summary.SerialPorts)
	}
	if c.Config.Labels[SerialPortsLabel] != "/dev/ttyACM7" {
		t.Errorf("label = %q", c.Config.Labels[SerialPortsLabel])
	}
	// Removing it takes it off the label.
	if err := UpdateBinding(name, "device", "/dev/ttyACM7", "/dev/ttyACM7", false); err != nil {
		t.Fatalf("remove serial port: %v", err)
	}
	if c = inspect(); c.Config.Labels[SerialPortsLabel] != "" {
		t.Errorf("label after removal = %q", c.Config.Labels[SerialPortsLabel])
	}

	// 4b. The switch: off removes the rules and marks the container, on
	// restores them.
	if err := UpdateSerialHotplug(name, false); err != nil {
		t.Fatalf("hot-plug off: %v", err)
	}
	c = inspect()
	if has(c.HostConfig.DeviceCgroupRules, "c 166:* rwm") || c.Config.Labels[SerialHotplugLabel] != "off" {
		t.Fatalf("off: rules=%v label=%q", c.HostConfig.DeviceCgroupRules, c.Config.Labels[SerialHotplugLabel])
	}
	if summary, _ := ContainerSummaryFor(ctx, cli, name); summary.SerialHotplug {
		t.Error("summary must report the hot-plug off")
	}
	if changes, _ := SyncSerialDevices(ctx, cli, name); len(changes) != 0 {
		t.Errorf("the sync must leave a switched-off container alone, got %v", changes)
	}
	if err := UpdateSerialHotplug(name, true); err != nil {
		t.Fatalf("hot-plug on: %v", err)
	}
	if c = inspect(); !has(c.HostConfig.DeviceCgroupRules, "c 166:* rwm") || c.Config.Labels[SerialHotplugLabel] != "" {
		t.Fatalf("on: rules=%v label=%q", c.HostConfig.DeviceCgroupRules, c.Config.Labels[SerialHotplugLabel])
	}

	// 5. Bind removed.
	if err := UpdateBinding(name, "volume", dir, "/opt/added", false); err != nil {
		t.Fatalf("remove bind: %v", err)
	}
	if c = inspect(); has(c.HostConfig.Binds, dir+":/opt/added") {
		t.Fatalf("binds after remove = %v", c.HostConfig.Binds)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c = inspect(); c.State != nil && c.State.Running {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if c.State == nil || !c.State.Running {
		t.Errorf("container not running at the end: %+v", c.State)
	}
}

// The hot-plug mechanics on a real container: the sync creates the node of a
// port the host has (simulated here, this machine has no serial device),
// leaves it alone next time, and removes it once the host lost the port.
func TestLiveSerialSyncCreatesAndRemovesNodes(t *testing.T) {
	if os.Getenv("RFSWIFT_TEST_DOCKER") == "" {
		t.Skip("set RFSWIFT_TEST_DOCKER=1 to run against a Docker daemon")
	}
	image := os.Getenv("RFSWIFT_TEST_IMAGE")
	if image == "" {
		image = "ubuntu:24.04"
	}
	SetPreferredEngine("docker")
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		t.Skipf("no engine: %v", err)
	}
	defer cli.Close()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "rfswift-serial-test-" + hex.EncodeToString(b)
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     &container.Config{Image: image, Cmd: []string{"/bin/sh"}, OpenStdin: true, Tty: true, Labels: map[string]string{"org.container.project": "rfswift"}},
		HostConfig: &container.HostConfig{DeviceCgroupRules: append([]string(nil), SerialCgroupRules...)},
		Name:       name,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _, _ = cli.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true}) }()
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	original := listHostSerialNodes
	defer func() { listHostSerialNodes = original }()
	listHostSerialNodes = func() []serialNode { return []serialNode{{Path: "/dev/ttyACM0", Major: 166, Minor: 0}} }

	inside := func(cmd string) (string, error) {
		ex, err := cli.ExecCreate(ctx, name, client.ExecCreateOptions{Cmd: []string{"/bin/sh", "-c", cmd}, AttachStdout: true, AttachStderr: true})
		if err != nil {
			return "", err
		}
		att, err := cli.ExecAttach(ctx, ex.ID, client.ExecAttachOptions{})
		if err != nil {
			return "", err
		}
		defer att.Close()
		out, _ := io.ReadAll(demuxReader(att.Reader))
		return string(out), nil
	}
	changes, err := SyncSerialDevices(ctx, cli, name)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(changes) != 1 || changes[0] != "created /dev/ttyACM0" {
		t.Fatalf("changes = %v", changes)
	}
	if out, _ := inside("ls -l /dev/ttyACM0"); !strings.HasPrefix(out, "c") || !strings.Contains(out, "166,") {
		t.Fatalf("node inside the container: %q", out)
	}
	if changes, _ = SyncSerialDevices(ctx, cli, name); len(changes) != 0 {
		t.Errorf("a second sync must change nothing, got %v", changes)
	}
	listHostSerialNodes = func() []serialNode { return nil }
	if changes, _ = SyncSerialDevices(ctx, cli, name); len(changes) != 1 || changes[0] != "removed /dev/ttyACM0" {
		t.Errorf("unplugged port: %v", changes)
	}
	if out, _ := inside("ls /dev/ttyACM0 2>&1"); !strings.Contains(out, "No such file") {
		t.Errorf("node should be gone: %q", out)
	}
}

// Creation through the function the Workbench's create dialog calls, with a
// serial port under Mapped devices and a device node under bind mounts.
func TestLiveCreateWithSerialPortAndDeviceBind(t *testing.T) {
	if os.Getenv("RFSWIFT_TEST_DOCKER") == "" {
		t.Skip("set RFSWIFT_TEST_DOCKER=1 to run against a Docker daemon")
	}
	image := os.Getenv("RFSWIFT_TEST_IMAGE")
	if image == "" {
		image = "ubuntu:24.04"
	}
	SetPreferredEngine("docker")
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		t.Skipf("no engine: %v", err)
	}
	defer cli.Close()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "rfswift-create-test-" + hex.EncodeToString(b)
	defer func() { _, _ = cli.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true}) }()
	var warnings []string
	id, err := CreateContainer(CreateOptions{Engine: "docker", Name: name, Image: image, Workspace: "none", Shell: "/bin/sh", NoX11: true, Start: true,
		Devices:  []string{"/dev/ttyACM0:/dev/ttyACM0", "/dev/null:/dev/rfswift-null"},
		Bindings: []string{"/dev/null:/dev/rfswift-vol"},
		Warn:     func(msg string) { warnings = append(warnings, msg) }})
	if err != nil {
		t.Fatalf("create: %v (warnings %v)", err, warnings)
	}
	res, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c := res.Container
	mapped := map[string]bool{}
	for _, d := range c.HostConfig.Devices {
		mapped[d.PathInContainer] = true
	}
	if !mapped["/dev/rfswift-null"] || mapped["/dev/rfswift-vol"] {
		t.Errorf("a node under Devices is a mapping, a node under bind mounts is not: %+v", c.HostConfig.Devices)
	}
	if mapped["/dev/ttyACM0"] {
		t.Errorf("an absent serial port must not be a fixed mapping: %+v", c.HostConfig.Devices)
	}
	boundVol := false
	for _, bind := range c.HostConfig.Binds {
		if strings.HasPrefix(bind, "/dev/null:/dev/rfswift-vol") {
			boundVol = true
		}
	}
	if !boundVol {
		t.Errorf("a device node under bind mounts stays a bind mount: %v", c.HostConfig.Binds)
	}
	for _, rule := range append([]string{"c 1:* rwm"}, SerialCgroupRules...) {
		found := false
		for _, r := range c.HostConfig.DeviceCgroupRules {
			if r == rule {
				found = true
			}
		}
		if !found {
			t.Errorf("serial rule %q missing: %v", rule, c.HostConfig.DeviceCgroupRules)
		}
	}
	if c.Config.Labels[SerialPortsLabel] != "/dev/ttyACM0" {
		t.Errorf("serial label = %q", c.Config.Labels[SerialPortsLabel])
	}
	if c.State == nil || !c.State.Running {
		t.Fatalf("container not running: %+v", c.State)
	}
	summary, err := ContainerSummaryFor(ctx, cli, name)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.SerialHotplug || len(summary.SerialPorts) != 1 || summary.SerialPorts[0] != "/dev/ttyACM0" {
		t.Errorf("card: hotplug=%v ports=%v", summary.SerialHotplug, summary.SerialPorts)
	}
	// The port shows up inside once the host has it (simulated) and a start
	// or terminal runs the sync.
	original := listHostSerialNodes
	defer func() { listHostSerialNodes = original }()
	listHostSerialNodes = func() []serialNode { return []serialNode{{Path: "/dev/ttyACM0", Major: 166, Minor: 0}} }
	if changes, err := SyncSerialDevices(ctx, cli, name); err != nil || len(changes) != 1 {
		t.Fatalf("sync: %v %v", changes, err)
	}
	ex, err := cli.ExecCreate(ctx, name, client.ExecCreateOptions{Cmd: []string{"/bin/sh", "-c", "ls -l /dev/ttyACM0 /dev/rfswift-null /dev/rfswift-vol"}, AttachStdout: true, AttachStderr: true})
	if err != nil {
		t.Fatal(err)
	}
	att, err := cli.ExecAttach(ctx, ex.ID, client.ExecAttachOptions{})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(demuxReader(att.Reader))
	att.Close()
	for _, want := range []string{"166, 0", "/dev/rfswift-null", "/dev/rfswift-vol"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("inside the container, %q missing:\n%s", want, out)
		}
	}
}
