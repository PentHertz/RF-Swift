package dock

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

func TestCheckImageFindsInstalledOfficialRFID(t *testing.T) {
	SetPreferredEngine("docker")
	engine := GetEngine()
	if engine == nil || engine.Type() != EngineDocker || !engine.IsServiceRunning() {
		t.Skip("Docker is not available")
	}
	availability, err := CheckImage("docker", "rfid")
	if err != nil {
		t.Fatal(err)
	}
	if !availability.Present {
		t.Skip("integration fixture penthertz/rfswift_resolute:rfid is not installed")
	}
	if availability.Resolved != "rfid" {
		t.Fatalf("GUI-facing image name = %q, want short name rfid", availability.Resolved)
	}
}

func TestCreateContainerCleansUpAfterStartFailure(t *testing.T) {
	SetPreferredEngine("docker")
	engine := GetEngine()
	if engine == nil || engine.Type() != EngineDocker || !engine.IsServiceRunning() {
		t.Skip("Docker is not available")
	}
	name := fmt.Sprintf("rfswift-cleanup-test-%d", time.Now().UnixNano())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	cli, err := NewEngineClient()
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	// The failure this test relies on — starting a container whose published
	// port is already bound on the host — only happens on daemons that reserve
	// host ports at start (Docker Desktop, native Linux). OrbStack forwards
	// ports lazily from its VM and starts the container regardless.
	if info, infoErr := cli.Info(context.Background(), client.InfoOptions{}); infoErr == nil &&
		strings.Contains(info.Info.OperatingSystem, "OrbStack") {
		t.Skip("OrbStack does not reserve published host ports at container start")
	}
	defer func() {
		_, _ = cli.ContainerRemove(context.Background(), name, client.ContainerRemoveOptions{Force: true})
	}()
	_, err = CreateContainer(CreateOptions{
		Engine: "docker", Name: name, Image: "rfid", Workspace: "none",
		Network: "bridge", ExposedPorts: "80/tcp", PortBindings: "127.0.0.1:" + port + ":80/tcp",
		NoX11: true, Start: true,
	})
	if err == nil {
		t.Fatal("container unexpectedly started with an occupied published port")
	}
	if _, inspectErr := cli.ContainerInspect(context.Background(), name, client.ContainerInspectOptions{}); inspectErr == nil {
		t.Fatalf("partial container %q remained after start failure", name)
	}
}

// A /dev/console device mapping (the macOS default devices of older config
// files) made every TTY container fail to start on OrbStack and Docker
// Desktop with runc >= 1.2.8: "create /dev/console mount target: open
// /dev/console: input/output error". CreateContainer must drop it.
func TestCreateContainerStartsWithConsoleDevice(t *testing.T) {
	SetPreferredEngine("docker")
	engine := GetEngine()
	if engine == nil || engine.Type() != EngineDocker || !engine.IsServiceRunning() {
		t.Skip("Docker is not available")
	}
	availability, err := CheckImage("docker", "corebuild")
	if err != nil || !availability.Present {
		t.Skip("integration fixture penthertz/rfswift_resolute:corebuild is not installed")
	}
	cli, err := NewEngineClient()
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	name := fmt.Sprintf("rfswift-console-test-%d", time.Now().UnixNano())
	defer func() {
		_, _ = cli.ContainerRemove(context.Background(), name, client.ContainerRemoveOptions{Force: true})
	}()
	if _, err := CreateContainer(CreateOptions{
		Engine: "docker", Name: name, Image: "corebuild", Workspace: "none",
		Network: "bridge", Devices: []string{"/dev/console:/dev/console"},
		NoX11: true, Start: true,
	}); err != nil {
		t.Fatalf("container with a /dev/console mapping did not start: %v", err)
	}
	inspect, err := cli.ContainerInspect(context.Background(), name, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, dev := range inspect.Container.HostConfig.Devices {
		if dev.PathInContainer == "/dev/console" {
			t.Fatalf("/dev/console mapping was kept: %+v", dev)
		}
	}
}
