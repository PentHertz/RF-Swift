/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Host audio for an existing container. `rfswift run` loads the audio
 * server's TCP module when it creates a container, but the module lives only
 * as long as the audio server: after a reboot, a logout or a PipeWire
 * restart it is gone, and a container entered again with `rfswift exec` has
 * no sound ("Connection refused" on its PULSE_SERVER). Re-entering loads it
 * again, as the Workbench does before a mission's terminal starts.
 */

package dock

import (
	"context"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

// ContainerPulseServer returns the PULSE_SERVER a container was created
// with, "" when it has none or cannot be inspected.
//
//	in(1): string contID container name or ID
//	out: string PULSE_SERVER value
func ContainerPulseServer(contID string) string {
	cli, err := NewEngineClient()
	if err != nil {
		return ""
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := cli.ContainerInspect(ctx, contID, client.ContainerInspectOptions{})
	if err != nil || res.Container.Config == nil {
		return ""
	}
	return pulseServerFromEnv(res.Container.Config.Env)
}

// pulseServerFromEnv extracts PULSE_SERVER from a container environment.
func pulseServerFromEnv(env []string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "PULSE_SERVER="); ok {
			return v
		}
	}
	return ""
}
