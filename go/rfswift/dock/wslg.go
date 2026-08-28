/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  WSLg plumbing for Windows. Containers run inside the WSL 2 VM (Docker
*  Desktop, Podman machine) where WSLg already provides an X11 server and a
*  PulseAudio server under /mnt/wslg. Mounting that directory into the
*  container and pointing PULSE_SERVER at the socket gives sound without
*  installing PulseAudio on Windows or running 'rfswift host audio enable'.
*  Verified with Docker Desktop (WSL 2 backend): pactl inside an RF Swift
*  container connects to unix:/mnt/wslg/PulseServer.
 */

package dock

import (
	"fmt"
	"runtime"
	"strings"

	common "penthertz/rfswift/common"
)

const (
	// WSLgContainerPath is where the WSLg directory is mounted in containers.
	WSLgContainerPath = "/mnt/wslg"
	// WSLgPulseServer is the PulseAudio socket WSLg exposes, as PULSE_SERVER.
	WSLgPulseServer = "unix:" + WSLgContainerPath + "/PulseServer"
	// wslgDockerHostPath is the Docker Desktop VM's view of the host WSLg mount.
	wslgDockerHostPath = "/run/desktop/mnt/host/wslg"
	// wslgPodmanHostPath: the Podman machine is a WSL distribution, so WSLg is
	// mounted at its usual place.
	wslgPodmanHostPath = "/mnt/wslg"
)

// wslgHostPath returns where the active engine's VM exposes WSLg.
func wslgHostPath() string {
	if eng := GetEngine(); eng != nil && eng.Type() == EnginePodman {
		return wslgPodmanHostPath
	}
	return wslgDockerHostPath
}

// WSLgBindings returns the bind mounts that expose WSLg's X11 socket and its
// PulseAudio server to a container on Windows (same mounts the CLI's --x11
// path has always used, plus the audio socket that lives in the same tree).
func WSLgBindings() []string {
	host := wslgHostPath()
	return []string{host + "/.X11-unix:/tmp/.X11-unix", host + ":" + WSLgContainerPath}
}

// wslgPulseServerFor decides whether a configured PulseAudio target should be
// replaced by WSLg's socket: on Windows, the CLI default (tcp:localhost /
// 127.0.0.1) points at nothing, so it is swapped; an explicit remote server
// is kept.
//
//	in(1): string goos runtime.GOOS
//	in(2): string pulseServer configured PULSE_SERVER value
//	out(1): string effective PULSE_SERVER
//	out(2): bool true when WSLg is used
func wslgPulseServerFor(goos, pulseServer string) (string, bool) {
	if goos != "windows" {
		return pulseServer, false
	}
	trimmed := strings.TrimSpace(pulseServer)
	if trimmed == "" || trimmed == WSLgPulseServer || strings.Contains(trimmed, "localhost") || strings.Contains(trimmed, "127.0.0.1") {
		return WSLgPulseServer, true
	}
	return pulseServer, false
}

// UsesWSLgAudio reports whether container audio on this host goes through
// WSLg for the given configured server.
func UsesWSLgAudio(pulseServer string) bool {
	_, ok := wslgPulseServerFor(runtime.GOOS, pulseServer)
	return ok
}

// resolvePulseServer applies the host-specific PULSE_SERVER adjustments:
// WSLg's socket on Windows, the VM gateway for Lima on macOS (PulseAudio
// listens on the macOS host, and 127.0.0.1 inside the VM is not the host).
func resolvePulseServer(pulseServer string) string {
	if server, ok := wslgPulseServerFor(runtime.GOOS, pulseServer); ok {
		if server != strings.TrimSpace(pulseServer) {
			common.PrintInfoMessage(fmt.Sprintf("Windows: container audio goes through WSLg PulseAudio (%s)", server))
		}
		return server
	}
	if runtime.GOOS == "darwin" {
		engine := GetEngine()
		if engine != nil && engine.Type() == EngineLima {
			if strings.Contains(pulseServer, "127.0.0.1") || strings.Contains(pulseServer, "localhost") {
				gateway := getLimaHostGatewayIP()
				if gateway != "" {
					old := pulseServer
					pulseServer = strings.Replace(pulseServer, "127.0.0.1", gateway, 1)
					pulseServer = strings.Replace(pulseServer, "localhost", gateway, 1)
					common.PrintInfoMessage(fmt.Sprintf("Lima: adjusted PULSE_SERVER from %s to %s (VM gateway → macOS host)", old, pulseServer))
				}
			}
		}
	}
	return pulseServer
}

// ensureWSLgMount adds the /mnt/wslg bind on Windows when audio goes through
// WSLg and nothing already mounts it (e.g. --no-x11 containers, which skip
// the X11 bindings but still deserve sound).
func ensureWSLgMount(binds []string, pulseServer string) []string {
	if _, ok := wslgPulseServerFor(runtime.GOOS, pulseServer); !ok {
		return binds
	}
	if bindTargets(binds, WSLgContainerPath) {
		return binds
	}
	return append(binds, wslgHostPath()+":"+WSLgContainerPath)
}

// bindTargets reports whether any bind mount lands on dest inside the container.
func bindTargets(binds []string, dest string) bool {
	for _, b := range binds {
		if bindDestination(b) == dest {
			return true
		}
	}
	return false
}

// bindDestination extracts the container path of "host:container[:opts]",
// tolerating Windows drive-letter sources ("C:\ws:/workspace:rw"), which a
// naive split on ':' would cut in the wrong place.
func bindDestination(bind string) string {
	rest := bind
	if len(rest) >= 3 && rest[1] == ':' && (rest[2] == '\\' || rest[2] == '/') {
		rest = rest[2:] // skip the drive letter and colon
	}
	parts := strings.Split(rest, ":")
	if len(parts) < 2 {
		return strings.TrimSpace(parts[0])
	}
	return strings.TrimSpace(parts[1])
}

// envHasKey reports whether a KEY=value slice already defines key.
func envHasKey(env []string, key string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return true
		}
	}
	return false
}
