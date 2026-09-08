/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Host setup - Docker socket access for the user.
*
*  /var/run/docker.sock is root:docker 0660, so a user needs the docker group.
*  Group membership only takes effect at the next login; to make Docker usable
*  right away, GrantDockerAccess additionally puts an ACL for the user on the
*  socket (setfacl), which lasts until the daemon recreates the socket, by
*  which time the group is in effect. Both happen in one privileged call.
 */

package hostsetup

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"
)

// DockerSocket is the daemon's default socket path on Linux.
const DockerSocket = "/var/run/docker.sock"

// dockerSocket is DockerSocket, overridable in tests.
var dockerSocket = DockerSocket

// DockerGroup is the group the daemon grants socket access to.
const DockerGroup = "docker"

// DockerAccess describes whether the user can talk to the Docker daemon.
type DockerAccess struct {
	Supported   bool   `json:"supported"`   // Linux only
	DockerFound bool   `json:"dockerFound"` // a docker CLI is on PATH
	Socket      string `json:"socket"`      // socket path probed
	SocketFound bool   `json:"socketFound"` // the socket exists (daemon installed and started at least once)
	Root        bool   `json:"root"`        // running as root: nothing to grant
	GroupExists bool   `json:"groupExists"` // the docker group exists
	Member      bool   `json:"member"`      // user is in the docker group per the account database
	Active      bool   `json:"active"`      // the docker group is in this process's groups (login after joining)
	Accessible  bool   `json:"accessible"`  // the socket can be opened read-write right now (group, ACL or root)
	Reachable   bool   `json:"reachable"`   // the daemon answered on the socket
	User        string `json:"user"`        // the account inspected
	Ready       bool   `json:"ready"`       // accessible, and permanently so (member or root)
	Detail      string `json:"detail"`      // one-line summary for humans
}

// probeSocket connects to a unix socket to learn two things at once: whether
// this process may use it (permission) and whether something listens.
func probeSocket(path string) (accessible, reachable, exists bool) {
	if _, err := os.Stat(path); err != nil {
		return false, false, false
	}
	conn, err := net.DialTimeout("unix", path, 500*time.Millisecond)
	if err == nil {
		conn.Close()
		return true, true, true
	}
	if errors.Is(err, os.ErrPermission) {
		return false, false, true
	}
	// ECONNREFUSED / ENOENT on connect: we were allowed to try, nobody listens.
	return true, false, true
}

// GetDockerAccess inspects the socket, the group and the user; never changes
// anything.
func GetDockerAccess() DockerAccess {
	st := DockerAccess{Socket: dockerSocket, User: InvokingUser()}
	if runtime.GOOS != "linux" {
		st.Detail = "Docker socket access applies to Linux hosts only"
		return st
	}
	st.Supported = true
	_, err := exec.LookPath("docker")
	st.DockerFound = err == nil
	st.Root = os.Geteuid() == 0
	st.Accessible, st.Reachable, st.SocketFound = probeSocket(dockerSocket)
	if grp, err := user.LookupGroup(DockerGroup); err == nil {
		st.GroupExists = true
		if st.User != "" {
			if u, err := user.Lookup(st.User); err == nil {
				if ids, err := u.GroupIds(); err == nil {
					for _, id := range ids {
						if id == grp.Gid {
							st.Member = true
						}
					}
				}
			}
		}
		if gids, err := os.Getgroups(); err == nil {
			for _, gid := range gids {
				if fmt.Sprint(gid) == grp.Gid {
					st.Active = true
				}
			}
		}
	}
	st.Ready = st.Root || (st.Accessible && st.Member)
	switch {
	case st.Root:
		st.Detail = "running as root"
	case !st.DockerFound && !st.SocketFound:
		st.Detail = "Docker is not installed"
	case !st.SocketFound:
		st.Detail = "Docker is installed but " + dockerSocket + " does not exist: start the daemon (systemctl enable --now docker)"
	case st.Accessible && st.Member:
		st.Detail = "usable"
		if !st.Reachable {
			st.Detail += " (daemon not answering: systemctl start docker)"
		}
	case st.Accessible:
		st.Detail = "usable in this session only: " + st.User + " is not in the docker group"
	case st.Member:
		st.Detail = st.User + " is in the docker group but this session predates it: log in again, or run 'newgrp docker'"
	case !st.GroupExists:
		st.Detail = "the docker group does not exist yet; " + st.User + " cannot use the daemon without sudo"
	default:
		st.Detail = st.User + " is not in the docker group: Docker needs sudo"
	}
	return st
}

// dockerGrantScript is the privileged part of GrantDockerAccess: make sure
// the group exists, add the user, and put an ACL on the socket so the change
// works immediately in the current session (setfacl is optional).
func dockerGrantScript(username, socket string) (string, error) {
	if username == "" {
		return "", errors.New("no user to grant Docker access to")
	}
	q := ShellQuote(username)
	var b strings.Builder
	b.WriteString("set -e\n")
	fmt.Fprintf(&b, "getent group %s >/dev/null || groupadd --system %s\n", DockerGroup, DockerGroup)
	fmt.Fprintf(&b, "usermod -aG %s %s\n", DockerGroup, q)
	fmt.Fprintf(&b, "if command -v setfacl >/dev/null 2>&1 && [ -S %s ]; then setfacl -m u:%s:rw %s; fi\n", ShellQuote(socket), q, ShellQuote(socket))
	return b.String(), nil
}

// DockerGrantReport is what GrantDockerAccess did.
type DockerGrantReport struct {
	GroupJoined    bool         `json:"groupJoined"`    // usermod ran (permanent, active after the next login)
	SessionGranted bool         `json:"sessionGranted"` // the socket is usable right now, without a new login
	Status         DockerAccess `json:"status"`         // state after the change
	Detail         string       `json:"detail"`         // one-line summary for humans
}

// GrantDockerAccess adds the invoking user to the docker group and grants the
// current session access to the socket, in one privileged call.
func GrantDockerAccess() (DockerGrantReport, error) {
	var report DockerGrantReport
	before := GetDockerAccess()
	if !before.Supported {
		return report, fmt.Errorf("Docker socket access only applies on Linux")
	}
	if before.Root {
		report.Status = before
		report.Detail = "running as root, nothing to grant"
		return report, nil
	}
	if before.Member && before.Accessible {
		report.Status = before
		report.Detail = "already usable"
		return report, nil
	}
	script, err := dockerGrantScript(before.User, dockerSocket)
	if err != nil {
		return report, err
	}
	if err := RunPrivileged(script); err != nil {
		return report, fmt.Errorf("granting Docker access failed: %w", err)
	}
	report.GroupJoined = !before.Member
	report.Status = GetDockerAccess()
	report.SessionGranted = report.Status.Accessible
	switch {
	case report.SessionGranted:
		report.Detail = "Docker is usable right away in this session; the docker group makes it permanent from your next login"
	case !report.Status.SocketFound:
		report.Detail = "added to the docker group; the daemon's socket does not exist yet (start it with: sudo systemctl enable --now docker)"
	default:
		report.Detail = "added to the docker group; this session could not be granted (setfacl missing?): run 'newgrp docker' or log in again"
	}
	return report, nil
}
