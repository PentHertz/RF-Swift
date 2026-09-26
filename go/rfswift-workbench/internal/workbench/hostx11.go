/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"context"
	"runtime"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"penthertz/rfswift/rfutils"
)

// X11 access for container missions. rfdock.CreateContainer authorises the
// container on the host X server (xhost local:root, or the host address for
// XQuartz on macOS), but the X server keeps that grant only while it runs:
// after a logout or reboot, GUI tools in an existing container fail with
// "Authorization required, but no authorization protocol specified". The CLI
// grants it again on every run and exec (cli setupX11); the Workbench does the
// same whenever it starts a mission, opens a terminal or runs a command.

// ensureMissionX11 authorises a container mission on the host X server when
// it was created with X11 forwarding. Best effort, like ensureMissionHostAudio:
// a missing xhost only logs a hint and never blocks the caller.
func (a *App) ensureMissionX11(id string) {
	local, ok := a.engine().(*LocalEngine)
	if !ok || runtime.GOOS == "windows" || isNixEnv(id) {
		return // remote hosts manage their own display; WSLg needs no xhost
	}
	if local.usesX11(id) {
		rfutils.XHostEnable()
	}
}

// usesX11 reports whether a container was created with X11 forwarding, i.e.
// has a DISPLAY in its environment (containers created with --no-x11 do not).
func (e *LocalEngine) usesX11(id string) bool {
	cli, _, err := e.clientFor(id)
	if err != nil {
		return false
	}
	defer cli.Close()
	res, err := cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
	if err != nil || res.Container.Config == nil {
		return false
	}
	for _, item := range res.Container.Config.Env {
		if strings.HasPrefix(item, "DISPLAY=") && item != "DISPLAY=" {
			return true
		}
	}
	return false
}

// hostServicesRecheck is how long a mission's host services count as
// prepared. Each check inspects the container and runs pactl and xhost, which
// would otherwise delay every console command; rechecking now and then still
// restores an audio module unloaded while the Workbench runs.
const hostServicesRecheck = time.Minute

// ensureMissionHostServices prepares the host side of a container mission
// before its tools run: the audio server's TCP module when the mission has
// host audio enabled (ensureMissionHostAudio) and the X server grant when the
// container uses X11 (ensureMissionX11). Both are lost when the host session
// restarts, so starts, terminals and console commands all call this; within
// hostServicesRecheck of the last check it returns at once.
func (a *App) ensureMissionHostServices(id string) {
	if at, ok := a.hostServicesAt.Load(id); ok && time.Since(at.(time.Time)) < hostServicesRecheck {
		return
	}
	a.ensureMissionHostAudio(id)
	a.ensureMissionX11(id)
	a.hostServicesAt.Store(id, time.Now())
}
