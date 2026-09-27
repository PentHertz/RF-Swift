package dock

import (
	"os"
	"testing"
)

func TestSetPreferredEngineUndoesEngineDockerHost(t *testing.T) {
	defer SetPreferredEngine("auto")

	// DOCKER_HOST exported by a previous detection (Podman socket) goes away.
	t.Setenv("DOCKER_HOST", "unix:///run/user/1000/podman/podman.sock")
	activeEngineMu.Lock()
	engineDockerHost = true
	activeEngineMu.Unlock()
	SetPreferredEngine("docker")
	if v, ok := os.LookupEnv("DOCKER_HOST"); ok {
		t.Fatalf("DOCKER_HOST from the previous engine kept: %q", v)
	}

	// A DOCKER_HOST the user set stays.
	t.Setenv("DOCKER_HOST", "tcp://build-host:2376")
	SetPreferredEngine("podman")
	if v := os.Getenv("DOCKER_HOST"); v != "tcp://build-host:2376" {
		t.Fatalf("the user's DOCKER_HOST was changed: %q", v)
	}
}
