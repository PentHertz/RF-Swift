/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * ContainerObj setter functions
 */
package dock

// setIfNotEmpty sets *field = value when value is non-empty.
func setIfNotEmpty(field *string, value string) {
	if value != "" {
		*field = value
	}
}

// appendCommaSeparated appends value to *field with a comma delimiter.
func appendCommaSeparated(field *string, value string) {
	if value != "" {
		if *field != "" {
			*field += "," + value
		} else {
			*field = value
		}
	}
}

// ContainerSetX11 sets the X11 forward path for the container.
// Pass an empty string to explicitly disable X11 forwarding.
//
//	in(1): string x11forward path
func ContainerSetX11(x11forward string) {
	containerCfg.x11forward = x11forward
}

// ContainerSetShell sets the shell to use in the container.
//
//	in(1): string shellcmd command
func ContainerSetShell(shellcmd string) {
	setIfNotEmpty(&containerCfg.shell, shellcmd)
}

// ContainerSetSeccomp sets the seccomp profile (empty string keeps default).
//
//	in(1): string profile seccomp profile
func ContainerSetSeccomp(profile string) {
	setIfNotEmpty(&containerCfg.seccomp, profile)
}

// ContainerAddBinding appends extra bind mounts to the container config.
//
//	in(1): string addbindings comma-separated bindings
func ContainerAddBinding(addbindings string) {
	appendCommaSeparated(&containerCfg.extrabinding, addbindings)
}

// ContainerAddCgroups appends extra cgroup rules to the container config.
//
//	in(1): string addcgroups comma-separated cgroup rules
func ContainerAddCgroups(addcgroups string) {
	appendCommaSeparated(&containerCfg.cgroups, addcgroups)
}

// ContainerAddDevices appends extra devices to the container config.
//
//	in(1): string adddevices comma-separated devices
func ContainerAddDevices(adddevices string) {
	appendCommaSeparated(&containerCfg.devices, adddevices)
}

// ContainerAddCaps appends extra capabilities to the container config.
//
//	in(1): string addcaps comma-separated capabilities
func ContainerAddCaps(addcaps string) {
	appendCommaSeparated(&containerCfg.caps, addcaps)
}

// ContainerSetWorkspace sets the workspace path for the container.
// Use "" for automatic (default), "none" to disable, or a custom path.
//
//	in(1): string workspace path or control value
func ContainerSetWorkspace(workspace string) {
	containerCfg.workspace = workspace
}

// ContainerSetImage sets the image name to use for container creation.
//
//	in(1): string imagename image name
func ContainerSetImage(imagename string) {
	setIfNotEmpty(&containerCfg.imagename, imagename)
}

// ContainerSetPrivileges sets the privilege mode for the container.
//
//	in(1): int privilege (1=true, 0=false)
func ContainerSetPrivileges(privilege int) {
	containerCfg.privileged = privilege == 1
}

// ContainerSetXDisplay sets the XDISPLAY env variable value.
//
//	in(1): string display
func ContainerSetXDisplay(display string) {
	containerCfg.xdisplay = display
}

// ContainerSetEnv sets extra environment variables for the container.
//
//	in(1): string varenv extra env variables
func ContainerSetEnv(varenv string) {
	setIfNotEmpty(&containerCfg.extraenv, varenv)
}

// ContainerSetPulse sets the PULSE_SERVER environment variable.
//
//	in(1): string pulseserv pulse server address
func ContainerSetPulse(pulseserv string) {
	setIfNotEmpty(&containerCfg.pulseServer, pulseserv)
}

// ContainerSetExtraHosts sets extra host entries for the container.
//
//	in(1): string extrahosts extra hosts
func ContainerSetExtraHosts(extrahosts string) {
	setIfNotEmpty(&containerCfg.extrahosts, extrahosts)
}

// ContainerSetNetworkMode sets the container network mode.
//
//	in(1): string networkmode network mode
func ContainerSetNetworkMode(networkmode string) {
	setIfNotEmpty(&containerCfg.networkMode, networkmode)
}

// ContainerSetExposedPorts sets exposed ports for the container.
//
//	in(1): string exposedports exposed ports
func ContainerSetExposedPorts(exposedports string) {
	setIfNotEmpty(&containerCfg.exposedPorts, exposedports)
}

// ContainerSetBindedPorts sets port bindings for the container.
//
//	in(1): string bindedports binded ports
func ContainerSetBindedPorts(bindedports string) {
	setIfNotEmpty(&containerCfg.bindedPorts, bindedports)
}

// ContainerSetUlimits sets ulimits for the container.
//
//	in(1): string ulimits
func ContainerSetUlimits(ulimits string) {
	containerCfg.ulimits = ulimits
}

// ContainerAddUlimit appends a single ulimit to existing ulimits.
//
//	in(1): string ulimit single ulimit
func ContainerAddUlimit(ulimit string) {
	appendCommaSeparated(&containerCfg.ulimits, ulimit)
}

// ContainerSetRealtime enables or disables realtime mode (SYS_NICE + rtprio ulimit).
//
//	in(1): bool enabled
func ContainerSetRealtime(enabled bool) {
	containerCfg.realtime = enabled
}

// ContainerSetDesktop configures desktop/VNC access for the container.
// When proto is non-empty, desktop mode is enabled with the given protocol,
// host, and port. The protocol can be "http" (noVNC web) or "vnc" (direct VNC).
//
//	in(1): string proto desktop protocol ("http" or "vnc")
//	in(2): string host listening host/IP
//	in(3): string port listening port
func ContainerSetDesktop(proto, host, port string) {
	containerCfg.desktopProto = proto
	if host != "" {
		containerCfg.desktopHost = host
	}
	if port != "" {
		containerCfg.desktopPort = port
	}
}

// ContainerSetDesktopPassword sets the VNC password for desktop mode.
// When non-empty, VNC authentication is enabled; otherwise access is unauthenticated.
func ContainerSetDesktopPassword(password string) {
	containerCfg.desktopPass = password
}

// ContainerSetDesktopSSL enables or disables SSL/TLS for the desktop connection.
func ContainerSetDesktopSSL(enabled bool) {
	containerCfg.desktopSSL = enabled
}

// ContainerDesktopEnabled reports whether desktop mode is configured.
func ContainerDesktopEnabled() bool {
	return containerCfg.desktopProto != ""
}

// ContainerSetVPN sets the VPN configuration string.
// Format: "type:argument" (e.g., "wireguard:./wg0.conf", "tailscale:--auth-key=tskey-xxx")
func ContainerSetVPN(vpn string) {
	setIfNotEmpty(&containerCfg.vpn, vpn)
}

// ContainerInstallFromScript runs hot install inside a created container.
//
//	in(1): string contid container identifier
//
// TODO: Optimize it and handle errors
func ContainerInstallFromScript(contid string) {
	ContainerInstallScript(contid, "entrypoint.sh", containerCfg.shell)
}

// RestartContainerService restarts the active container engine service.
//
//	out: error
func RestartContainerService() error {
	return EngineRestartService()
}

// GetHostConfigPath returns the host config file path for a container.
//
//	in(1): string containerID container identifier
//	out: string path, error
func GetHostConfigPath(containerID string) (string, error) {
	return EngineGetHostConfigPath(containerID)
}
