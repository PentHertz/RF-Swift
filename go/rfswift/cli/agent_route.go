/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Which engine hosts a target the agent is asked about.
*
*  The agent serves every engine of its host (targets.list walks them all),
*  but the rfdock helpers it calls for one container - inspect, start, stop,
*  configure, remove, audit, the exec a terminal runs - drive the process-wide
*  engine. Left alone that is whichever engine was detected at start-up or
*  used by the last creation, so a Lima container asked about while Docker
*  Desktop is active answers "No such container". This file points the
*  process-wide engine at the container's engine before such a call, the way
*  the Workbench's LocalEngine routes missions locally.
 */

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/moby/moby/client"

	rfdock "penthertz/rfswift/dock"
)

// agentOrigDockerHost is DOCKER_HOST as the agent was started: GetEngine sets
// the variable to the Lima or Podman socket when it selects one of them, and
// a Docker client created afterwards would otherwise dial that socket.
var agentOrigDockerHost, agentOrigDockerHostSet = os.LookupEnv("DOCKER_HOST")

func agentResetEngineEnv() {
	if agentOrigDockerHostSet {
		os.Setenv("DOCKER_HOST", agentOrigDockerHost)
	} else {
		os.Unsetenv("DOCKER_HOST")
	}
}

var agentRoutes = struct {
	sync.Mutex
	byID map[string]rfdock.EngineType
}{byID: map[string]rfdock.EngineType{}}

// agentEngineFor finds the engine hosting a container: the remembered one
// when it still has the container, else the first running engine that does.
func agentEngineFor(id string) (rfdock.ContainerEngine, error) {
	has := func(eng rfdock.ContainerEngine) bool {
		cli, err := eng.GetClient()
		if err != nil {
			return false
		}
		defer cli.Close()
		_, err = cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
		return err == nil
	}
	agentRoutes.Lock()
	known, ok := agentRoutes.byID[id]
	agentRoutes.Unlock()
	agentResetEngineEnv()
	if ok {
		if eng := rfdock.EngineByType(known); eng != nil && eng.IsAvailable() && eng.IsServiceRunning() && has(eng) {
			return eng, nil
		}
	}
	for _, eng := range agentEngines() {
		if !eng.IsServiceRunning() || !has(eng) {
			continue
		}
		agentRoutes.Lock()
		agentRoutes.byID[id] = eng.Type()
		agentRoutes.Unlock()
		return eng, nil
	}
	return nil, fmt.Errorf("container %q not found on any running engine of the agent host", id)
}

// agentRoute points the process-wide engine at the one hosting the
// container, for the rfdock helpers that work on the global engine. A
// container found nowhere leaves the engine as it is: the helper reports
// the miss itself.
func agentRoute(id string) rfdock.ContainerEngine {
	eng, err := agentEngineFor(id)
	if err != nil {
		return nil
	}
	if rfdock.GetEngine().Type() != eng.Type() {
		rfdock.SetPreferredEngine(string(eng.Type()))
	}
	// GetEngine re-detects after SetPreferredEngine and sets DOCKER_HOST for
	// Lima and Podman; Docker needs the original value back.
	agentResetEngineEnv()
	rfdock.GetEngine()
	return eng
}

// agentChildEnv is the environment for a child rfswift (the exec behind a
// terminal): the agent's, with DOCKER_HOST as it was at start-up so the
// child's --engine decides the daemon, not a socket this process exported.
func agentChildEnv(extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "DOCKER_HOST=") {
			continue
		}
		env = append(env, kv)
	}
	if agentOrigDockerHostSet {
		env = append(env, "DOCKER_HOST="+agentOrigDockerHost)
	}
	return append(env, extra...)
}
