/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Which Podman service a non-root RF Swift drives on Linux.
 *
 *   rootless  the per-user service ($XDG_RUNTIME_DIR/podman/podman.sock,
 *             images under ~/.local/share/containers). Root-only device
 *             nodes, device cgroup rules and realtime limits cannot be
 *             honoured, which is what SDR/USB hardware usually needs.
 *   rootful   the system service (/run/podman/podman.sock), used from the
 *             user's own process once its socket is made accessible to the
 *             user (`rfswift host qubes setup` does it on Qubes OS). RF
 *             Swift and the Workbench then stay the user's - audio server,
 *             X11, $HOME, workspace and config are the user's - while the
 *             containers get the rootful service's device access. This is
 *             the Podman counterpart of the docker group, with the same
 *             trust implication: access to the socket is root-equivalent.
 *   auto      rootless for a user, rootful for root (unchanged behaviour).
 *
 * Precedence: RFSWIFT_PODMAN_MODE > config [general] podman_mode > auto.
 */

package dock

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	common "penthertz/rfswift/common"
	"penthertz/rfswift/hostsetup"
	rfutils "penthertz/rfswift/rfutils"
)

// PodmanMode selects the rootless or the rootful Podman service.
type PodmanMode string

const (
	PodmanModeAuto     PodmanMode = "auto"
	PodmanModeRootless PodmanMode = "rootless"
	PodmanModeRootful  PodmanMode = "rootful"
)

// ParsePodmanMode validates a mode name (case-insensitive; "" is auto).
func ParsePodmanMode(s string) (PodmanMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return PodmanModeAuto, true
	case "rootless", "user":
		return PodmanModeRootless, true
	case "rootful", "root", "system":
		return PodmanModeRootful, true
	}
	return PodmanModeAuto, false
}

// podmanModeOverride is set by tests.
var podmanModeOverride PodmanMode

// ConfiguredPodmanMode returns the effective mode: RFSWIFT_PODMAN_MODE, then
// `[general] podman_mode` in config.ini, then auto. Unknown values fall back
// to auto.
func ConfiguredPodmanMode() PodmanMode {
	if podmanModeOverride != "" {
		return podmanModeOverride
	}
	if env := strings.TrimSpace(os.Getenv("RFSWIFT_PODMAN_MODE")); env != "" {
		if m, ok := ParsePodmanMode(env); ok {
			return m
		}
	}
	if m, ok := ParsePodmanMode(rfutils.ConfiguredPodmanMode(common.ConfigFileByPlatform())); ok {
		return m
	}
	return PodmanModeAuto
}

// wantsRootfulPodmanAsUser reports whether this non-root Linux process must
// use the system Podman service.
func wantsRootfulPodmanAsUser() bool {
	return runtime.GOOS == "linux" && os.Getuid() != 0 && ConfiguredPodmanMode() == PodmanModeRootful
}

// isRootfulPodmanURI reports whether a service URI is the system Podman
// socket.
func isRootfulPodmanURI(uri string) bool {
	path := strings.TrimPrefix(uri, "unix://")
	for _, p := range hostsetup.RootfulPodmanSocketPaths {
		if path == p {
			return true
		}
	}
	return false
}

// podmanURIRootless reports whether this process reaches a rootless service
// through the URI: never for root, never for the system socket.
func podmanURIRootless(uri string) bool {
	if os.Getuid() == 0 {
		return false
	}
	return !isRootfulPodmanURI(uri)
}

// containerHostExport tracks the CONTAINER_HOST this process exported for
// the rootful service, so a later mode change can take it back.
var (
	containerHostMu       sync.Mutex
	exportedContainerHost string
)

// exportRootfulContainerHost points the podman CLI at the service the API
// client uses. Some operations go through the CLI (creation with device
// cgroup rules, networks, cp, image checks); as a user it would otherwise
// talk to the rootless service and create the container in the wrong store.
// podman(1): CONTAINER_HOST set makes --remote the default.
func exportRootfulContainerHost(socket string) {
	containerHostMu.Lock()
	defer containerHostMu.Unlock()
	if cur := os.Getenv("CONTAINER_HOST"); cur != "" && cur != exportedContainerHost {
		return // the user's own setting wins
	}
	os.Setenv("CONTAINER_HOST", socket)
	exportedContainerHost = socket
}

// withdrawContainerHost removes a CONTAINER_HOST this process exported.
func withdrawContainerHost() {
	containerHostMu.Lock()
	defer containerHostMu.Unlock()
	if exportedContainerHost != "" && os.Getenv("CONTAINER_HOST") == exportedContainerHost {
		os.Unsetenv("CONTAINER_HOST")
	}
	exportedContainerHost = ""
}

// userContainerHost is CONTAINER_HOST unless this process exported it itself.
func userContainerHost() string {
	containerHostMu.Lock()
	defer containerHostMu.Unlock()
	host := os.Getenv("CONTAINER_HOST")
	if host != "" && host == exportedContainerHost {
		return ""
	}
	return host
}

// RootfulPodmanAccessHint explains, for a user configured for the rootful
// service, why the socket cannot be used and how to fix it. "" when it can
// be used or the rootful mode is not selected.
func RootfulPodmanAccessHint() string {
	if !wantsRootfulPodmanAsUser() {
		return ""
	}
	st := hostsetup.ProbeRootfulPodmanSocket()
	switch {
	case st.Accessible && st.Reachable:
		return ""
	case st.Accessible:
		return fmt.Sprintf("podman_mode = rootful, but nothing answers on %s: start it with 'sudo systemctl enable --now podman.socket'", st.Socket)
	case st.Denied:
		fix := "grant it with 'rfswift host qubes setup' (Qubes OS)"
		if !hostsetup.DetectQubes().IsQubes {
			fix = "make the socket usable by your group (podman.socket drop-in: SocketGroup=), or run RF Swift with sudo"
		}
		return fmt.Sprintf("podman_mode = rootful, but %s is not accessible to %s: %s; or set podman_mode = rootless", st.Socket, hostsetup.InvokingUser(), fix)
	default:
		return fmt.Sprintf("podman_mode = rootful, but %s does not exist: start the system service with 'sudo systemctl enable --now podman.socket'", st.Socket)
	}
}

// podmanIsRootless reports whether Podman, as this process uses it, is the
// rootless service: the active Podman engine's own answer when there is one,
// the configured mode otherwise.
func podmanIsRootless() bool {
	if runtime.GOOS != "linux" {
		return true // Podman machine: a rootless VM user by default
	}
	if os.Getuid() == 0 {
		return false
	}
	activeEngineMu.RLock()
	eng := activeEngine
	activeEngineMu.RUnlock()
	if p, ok := eng.(*PodmanEngine); ok {
		return p.IsRootless()
	}
	if host := userContainerHost(); host != "" {
		return podmanURIRootless(host)
	}
	return !wantsRootfulPodmanAsUser()
}

// rootfulPodmanHint completes "run RF Swift with sudo" advice with the way to
// get rootful Podman without sudo, where RF Swift can set it up.
func rootfulPodmanHint() string {
	if hostsetup.IsQubes() {
		return " On Qubes OS, 'rfswift host qubes setup' gives you rootful Podman without sudo."
	}
	return " Or set podman_mode = rootful once the rootful Podman socket is usable by you."
}
