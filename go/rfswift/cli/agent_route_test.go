package cli

import (
	"os"
	"strings"
	"testing"

	rfdock "penthertz/rfswift/dock"
)

// A child rfswift (the exec behind a terminal) must not inherit the socket
// GetEngine exported for Lima or Podman: its --engine decides the daemon.
func TestAgentChildEnvRestoresDockerHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///tmp/rfswift-test-lima.sock")
	env := agentChildEnv("TERM=xterm")
	for _, kv := range env {
		if strings.HasPrefix(kv, "DOCKER_HOST=") && !agentOrigDockerHostSet {
			t.Fatalf("exported socket leaked into the child: %s", kv)
		}
		if strings.HasPrefix(kv, "DOCKER_HOST=") && kv != "DOCKER_HOST="+agentOrigDockerHost {
			t.Fatalf("child got %s, want the start-up value %q", kv, agentOrigDockerHost)
		}
	}
	if env[len(env)-1] != "TERM=xterm" {
		t.Fatalf("extra variables must be appended, got %v", env[len(env)-1])
	}
	agentResetEngineEnv()
	if v, ok := os.LookupEnv("DOCKER_HOST"); ok != agentOrigDockerHostSet || v != agentOrigDockerHost {
		t.Fatalf("reset left DOCKER_HOST=%q (set=%v)", v, ok)
	}
}

// TestAgentRoutesToTheEngineHostingTheContainer runs against this host's
// engines (RFSWIFT_TEST_LIMA=1 on a Mac with the Lima VM up and one RF Swift
// container in it): a Lima container inspected while Docker is the active
// engine must be found and reported on Lima, and the terminal's exec must be
// told --engine lima.
func TestAgentRoutesToTheEngineHostingTheContainer(t *testing.T) {
	if os.Getenv("RFSWIFT_TEST_LIMA") == "" {
		t.Skip("set RFSWIFT_TEST_LIMA=1 with a running Lima VM")
	}
	targets, err := agentListTargets()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, target := range targets {
		if target.Engine == "lima" {
			id = target.ID
			break
		}
	}
	if id == "" {
		t.Skip("no Lima container on this host")
	}
	rfdock.SetPreferredEngine("docker")
	got, err := agentInspect(id)
	if err != nil {
		t.Fatalf("inspect %s with Docker active: %v", id, err)
	}
	if got.Engine != "lima" {
		t.Fatalf("engine = %q, want lima", got.Engine)
	}
	if rfdock.GetEngine().Type() != rfdock.EngineLima {
		t.Fatalf("the process-wide engine must follow the container, got %s", rfdock.GetEngine().Type())
	}
	if got.Summary == nil || got.Summary.SerialHotplugAvailable {
		t.Fatalf("summary must be Lima's (no serial hot-plug), got %+v", got.Summary)
	}
}
