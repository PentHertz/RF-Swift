package dock

import (
	"context"
	"fmt"
	"net"
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
