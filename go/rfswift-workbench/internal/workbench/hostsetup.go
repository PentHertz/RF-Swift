/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"errors"
	"runtime"

	"penthertz/rfswift/hostsetup"
)

// Host setup from the GUI (Linux, local engine): RF Swift's udev rules for
// RF / hardware-security devices and Docker socket access for the user. Both
// mirror `rfswift host udev` / `rfswift host docker-access`; the privileged
// part runs through pkexec (a graphical polkit prompt) since the Workbench
// has no terminal, and udev is reloaded and re-triggered in the same call so
// the rules apply without a re-plug or a new login. Nothing here runs on its
// own: the Engine doctor shows the state and offers a button.

// hostSetupApplies reports whether this App can change the local host: a
// local engine on Linux (Windows/macOS have no udev, Docker Desktop manages
// its own socket access).
func (a *App) hostSetupApplies() bool {
	if _, ok := a.eng.(*LocalEngine); !ok {
		return false
	}
	return runtime.GOOS == "linux"
}

// HostUdevStatus reports the state of RF Swift's host udev rules. Never
// changes anything; Supported is false where the GUI cannot act.
func (a *App) HostUdevStatus() hostsetup.UdevStatus {
	if !a.hostSetupApplies() {
		return hostsetup.UdevStatus{File: hostsetup.HostRulesFile, Detail: "host udev rules are managed on local Linux hosts only"}
	}
	return hostsetup.GetUdevStatus()
}

// HostUdevInstall installs (or updates) the rules, creates the plugdev group,
// adds the user to it and reloads udev, after a polkit prompt.
func (a *App) HostUdevInstall() (hostsetup.UdevReport, error) {
	if !a.hostSetupApplies() {
		return hostsetup.UdevReport{}, errors.New("host udev rules can only be installed on a local Linux host")
	}
	return hostsetup.InstallUdevRules()
}

// HostUdevRemove removes the rules RF Swift installed and reloads udev.
func (a *App) HostUdevRemove() (hostsetup.UdevStatus, error) {
	if !a.hostSetupApplies() {
		return a.HostUdevStatus(), errors.New("host udev rules can only be removed on a local Linux host")
	}
	if _, err := hostsetup.RemoveUdevRules(); err != nil {
		return hostsetup.GetUdevStatus(), err
	}
	return hostsetup.GetUdevStatus(), nil
}

// DockerAccessStatus reports whether the user can use the Docker socket, now
// and permanently. Never changes anything.
func (a *App) DockerAccessStatus() hostsetup.DockerAccess {
	if !a.hostSetupApplies() {
		return hostsetup.DockerAccess{Detail: "Docker socket access is managed on local Linux hosts only"}
	}
	return hostsetup.GetDockerAccess()
}

// DockerAccessGrant adds the user to the docker group and grants the running
// session access to the socket (ACL), after a polkit prompt, so Docker works
// without logging out or restarting the Workbench.
func (a *App) DockerAccessGrant() (hostsetup.DockerGrantReport, error) {
	if !a.hostSetupApplies() {
		return hostsetup.DockerGrantReport{}, errors.New("Docker access can only be granted on a local Linux host")
	}
	return hostsetup.GrantDockerAccess()
}
