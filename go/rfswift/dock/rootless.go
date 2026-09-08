/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Rootless Podman restrictions shared by the interactive CLI (ContainerRun)
 * and the non-interactive create path used by the Workbench GUI and the
 * remote agent (CreateContainer).
 *
 * Rootless Podman runs the container inside a user namespace. Root-only host
 * device nodes, device cgroup rules and resource limits above the host user's
 * hard limits cannot be honoured there. Podman does not always report these
 * cleanly: the OCI runtime fails at start time and the API surfaces it as
 * "container create failed (no logs from conmon)". Dropping the unsupported
 * pieces up front keeps the container creatable and lets us tell the user
 * exactly what was left out.
 */

package dock

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/moby/moby/api/types/container"
)

// rootlessBlockedDevices lists host device nodes that rootless Podman cannot
// map into a container: they are root-only nodes the runtime cannot bind or
// recreate inside a user namespace.
var rootlessBlockedDevices = map[string]bool{
	"/dev/tty":     true,
	"/dev/tty0":    true,
	"/dev/tty1":    true,
	"/dev/tty2":    true,
	"/dev/console": true,
	"/dev/vcsa":    true,
	"/dev/vhci":    true,
	"/dev/uinput":  true,
}

// hostPathOpenable reports whether the current user can open the host path
// for reading, which is the access the OCI runtime needs to map it.
//
//	in(1): string path host path to probe
//	out: bool true when the path can be opened by this process
func hostPathOpenable(path string) bool {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// rootlessBindSourceBlocked reports whether a bind-mount source is a device
// node rootless Podman cannot map: a known root-only node, or a character or
// block device the current user cannot open. Directories such as /dev/bus/usb
// and regular files are never blocked.
//
//	in(1): string source host side of the bind mount
//	out: bool true when the bind must be dropped in rootless mode
func rootlessBindSourceBlocked(source string) bool {
	if rootlessBlockedDevices[source] {
		return true
	}
	info, err := os.Stat(source)
	if err != nil || info.Mode()&os.ModeDevice == 0 {
		return false
	}
	return !hostPathOpenable(source)
}

// filterRootlessDevices keeps the device mappings a rootless container can
// receive: not on the blocked list and openable by the current user.
//
//	in(1): []container.DeviceMapping devices requested mappings
//	in(2): func(string) bool openable   host access probe (hostPathOpenable in production)
//	out: []container.DeviceMapping mappings to keep
//	out: []string host paths that were dropped
func filterRootlessDevices(devices []container.DeviceMapping, openable func(string) bool) ([]container.DeviceMapping, []string) {
	var kept []container.DeviceMapping
	var dropped []string
	for _, dev := range devices {
		if rootlessBlockedDevices[dev.PathOnHost] || !openable(dev.PathOnHost) {
			dropped = append(dropped, dev.PathOnHost)
			continue
		}
		kept = append(kept, dev)
	}
	return kept, dropped
}

// filterRootlessDeviceBinds removes bind mounts whose host source is a device
// node rootless Podman cannot map (see rootlessBindSourceBlocked).
//
//	in(1): []string binds        bind specifications "source:dest[:opts]"
//	in(2): func(string) bool blocked source probe (rootlessBindSourceBlocked in production)
//	out: []string binds to keep
//	out: []string host sources that were dropped
func filterRootlessDeviceBinds(binds []string, blocked func(string) bool) ([]string, []string) {
	var kept []string
	var dropped []string
	for _, bind := range binds {
		source := strings.SplitN(bind, ":", 2)[0]
		if source != "" && blocked(source) {
			dropped = append(dropped, source)
			continue
		}
		kept = append(kept, bind)
	}
	return kept, dropped
}

// filterRootlessUlimits drops ulimits that ask for more than the host grants
// this user. In rootless mode the OCI runtime applies the container's rlimits
// as the unprivileged user, so any value above the host hard limit fails with
// EPERM. A hard limit of -1 means unlimited on both sides.
//
//	in(1): []*container.Ulimit ulimits          requested limits
//	in(2): func(string) (int64, bool) hardLimit host hard limit by Docker ulimit name; ok=false keeps the entry
//	out: []*container.Ulimit limits to keep
//	out: []string human-readable descriptions of dropped limits
func filterRootlessUlimits(ulimits []*container.Ulimit, hardLimit func(string) (int64, bool)) ([]*container.Ulimit, []string) {
	var kept []*container.Ulimit
	var dropped []string
	for _, u := range ulimits {
		if u == nil {
			continue
		}
		hostHard, ok := hardLimit(u.Name)
		if !ok {
			kept = append(kept, u)
			continue
		}
		exceeds := func(v int64) bool {
			if hostHard == -1 {
				return false
			}
			return v == -1 || v > hostHard
		}
		if exceeds(u.Hard) || exceeds(u.Soft) {
			dropped = append(dropped, fmt.Sprintf("%s=%s (host hard limit %s)", u.Name, formatUlimitPair(u), formatUlimitValue(hostHard)))
			continue
		}
		kept = append(kept, u)
	}
	return kept, dropped
}

func formatUlimitValue(v int64) string {
	if v == -1 {
		return "unlimited"
	}
	return fmt.Sprintf("%d", v)
}

func formatUlimitPair(u *container.Ulimit) string {
	return formatUlimitValue(u.Soft) + ":" + formatUlimitValue(u.Hard)
}

// restrictRootlessPodmanHostConfig strips from a HostConfig the devices, bind
// mounts and ulimits that rootless Podman cannot honour, reporting each change
// through warn. Device cgroup rules are left to the caller: the CLI confirms
// interactively before dropping them, the API path drops them with a warning.
//
//	in(1): *container.HostConfig hc host configuration to adjust in place
//	in(2): func(string) warn         receives one message per adjustment
func restrictRootlessPodmanHostConfig(hc *container.HostConfig, warn func(string)) {
	if hc == nil {
		return
	}
	if warn == nil {
		warn = func(string) {}
	}
	if len(hc.Devices) > 0 {
		kept, dropped := filterRootlessDevices(hc.Devices, hostPathOpenable)
		if len(dropped) > 0 {
			warn(fmt.Sprintf("Rootless Podman: dropped %d device(s) this user cannot map into a user namespace: %s. Run RF Swift with sudo (rootful Podman) or Docker to attach them.", len(dropped), strings.Join(dropped, ", ")))
		}
		hc.Devices = kept
	}
	if len(hc.Binds) > 0 {
		kept, dropped := filterRootlessDeviceBinds(hc.Binds, rootlessBindSourceBlocked)
		if len(dropped) > 0 {
			warn(fmt.Sprintf("Rootless Podman: dropped %d root-only device mount(s): %s.", len(dropped), strings.Join(dropped, ", ")))
		}
		hc.Binds = kept
	}
	// Entering the user namespace drops the host's supplementary groups, so
	// a device this user may open on the host through dialout or plugdev is
	// still EACCES inside the container. crun can keep them
	// (--group-add keep-groups); runc cannot, and would fail to start.
	if len(hc.GroupAdd) == 0 && podmanKeepsGroups() {
		hc.GroupAdd = []string{"keep-groups"}
	}
	if len(hc.Resources.Ulimits) > 0 {
		kept, dropped := filterRootlessUlimits(hc.Resources.Ulimits, hostHardLimit)
		if len(dropped) > 0 {
			warn(fmt.Sprintf("Rootless Podman: dropped ulimit(s) above this user's host hard limits: %s. Realtime scheduling needs rootful Podman or Docker, or higher limits for the podman user service (LimitRTPRIO/LimitMEMLOCK).", strings.Join(dropped, "; ")))
		}
		hc.Resources.Ulimits = kept
	}
}

// podmanKeepsGroups reports whether the Podman OCI runtime is crun, the only
// runtime that honours "keep-groups". Probed once per process; overridable
// in tests.
var podmanKeepsGroups = func() func() bool {
	var once sync.Once
	var crun bool
	return func() bool {
		once.Do(func() {
			out, err := exec.Command("podman", "info", "--format", "{{.Host.OCIRuntime.Name}}").Output()
			crun = err == nil && strings.Contains(strings.ToLower(string(out)), "crun")
		})
		return crun
	}
}()
