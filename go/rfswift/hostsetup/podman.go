/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Host setup - access to the rootful Podman service from the desktop user.
*
*  Rootless Podman cannot map root-only device nodes or apply device cgroup
*  rules, so SDR/USB work usually needs the rootful service. Running RF Swift
*  itself through sudo gets there, but then the program runs as root: it
*  looks for the audio server, the X11 cookie, the workspace and the config
*  in root's session and home. Making the rootful service's socket usable by
*  the user (what the docker group does for Docker) lets RF Swift and the
*  Workbench stay the user's while the containers get root's device access.
*  `rfswift host qubes setup` does it on Qubes OS, where the user already has
*  passwordless root inside the qube; the probe below is shared by that
*  command, `rfswift doctor`, the engine selection and the Workbench.
 */

package hostsetup

import (
	"errors"
	"net"
	"os"
	"time"
)

// RootfulPodmanSocket is the system Podman service's API socket on Linux
// (podman.socket: ListenStream=%t/podman/podman.sock with %t = /run).
const RootfulPodmanSocket = "/run/podman/podman.sock"

// rootfulPodmanSocket is RootfulPodmanSocket, overridable in tests.
var rootfulPodmanSocket = RootfulPodmanSocket

// RootfulPodmanSocketPaths are the paths the system Podman socket is found
// at (/var/run is a symlink to /run on current distributions).
var RootfulPodmanSocketPaths = []string{"/run/podman/podman.sock", "/var/run/podman/podman.sock"}

// PodmanSocketAccess is what this process can do with the rootful Podman
// socket.
type PodmanSocketAccess struct {
	Socket     string `json:"socket"`     // socket path probed
	Exists     bool   `json:"exists"`     // the socket file exists (false also when /run/podman cannot be searched)
	Accessible bool   `json:"accessible"` // this process may connect to it
	Reachable  bool   `json:"reachable"`  // something answered the connection
	Denied     bool   `json:"denied"`     // the socket or its directory refused this user (EACCES)
}

// ProbeRootfulPodmanSocket inspects the rootful Podman socket without
// changing anything. /run/podman is 0700 root by default (tmpfiles.d), in
// which case the socket cannot even be seen by a user: Denied is set and
// Exists stays false.
func ProbeRootfulPodmanSocket() PodmanSocketAccess {
	st := PodmanSocketAccess{Socket: rootfulPodmanSocket}
	if _, err := os.Stat(rootfulPodmanSocket); err != nil {
		st.Denied = errors.Is(err, os.ErrPermission)
		return st
	}
	st.Exists = true
	conn, err := net.DialTimeout("unix", rootfulPodmanSocket, 500*time.Millisecond)
	if err == nil {
		conn.Close()
		st.Accessible, st.Reachable = true, true
		return st
	}
	if errors.Is(err, os.ErrPermission) {
		st.Denied = true
		return st
	}
	// Allowed to try, nobody listening (socket unit stopped).
	st.Accessible = true
	return st
}
