/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * /dev/console in TTY containers.
 *
 * With a terminal, runc bind-mounts the container's pty over /dev/console,
 * so mapping the host's /dev/console as a device does nothing useful. Since
 * runc 1.2.8 / 1.3.3 (CVE-2025-52565) runc also opens /dev/console before
 * that mount. If the device was mapped in, runc opens the real console of
 * the engine's VM. On OrbStack and Docker Desktop that console has no
 * backend, so the open fails with EIO and the container never starts:
 *
 *	create /dev/console mount target: open /dev/console: input/output error
 *
 * Every container RF Swift creates has a TTY, so the mapping is dropped at
 * creation. Older config files still list it in their default devices.
 */

package dock

import (
	"path"

	"github.com/moby/moby/api/types/container"
)

// consoleDevicePath is where runc mounts the pty of a TTY container.
const consoleDevicePath = "/dev/console"

// dropTTYConsoleDevice removes the device mappings that target /dev/console
// in the container from a TTY container's host configuration.
//
//	in(1): *container.HostConfig hc host configuration to clean (nil is a no-op)
//	out: bool true when a mapping was removed
func dropTTYConsoleDevice(hc *container.HostConfig) bool {
	if hc == nil || len(hc.Devices) == 0 {
		return false
	}
	kept := hc.Devices[:0:0]
	for _, dev := range hc.Devices {
		target := dev.PathInContainer
		if target == "" {
			target = dev.PathOnHost
		}
		if path.Clean(target) == consoleDevicePath {
			continue
		}
		kept = append(kept, dev)
	}
	if len(kept) == len(hc.Devices) {
		return false
	}
	hc.Devices = kept
	return true
}
