/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * DockerObj setter functions
 *
 * DockerSetx11         - in(1): string x11forward path
 * DockerSetShell       - in(1): string shell command
 * DockerSetSeccomp     - in(1): string seccomp profile
 * DockerAddBinding     - in(1): string comma-separated bindings
 * DockerAddCgroups     - in(1): string comma-separated cgroup rules
 * DockerAddDevices     - in(1): string comma-separated devices
 * DockerAddCaps        - in(1): string comma-separated capabilities
 * DockerSetImage       - in(1): string image name
 * DockerSetPrivileges  - in(1): int privileged flag (1=true, 0=false)
 * DockerSetXDisplay    - in(1): string display value
 * DockerSetEnv         - in(1): string extra env variables
 * DockerSetPulse       - in(1): string pulse server address
 * DockerSetExtraHosts  - in(1): string extra hosts
 * DockerSetNetworkMode - in(1): string network mode
 * DockerSetExposedPorts- in(1): string exposed ports
 * DockerSetBindexPorts - in(1): string binded ports
 * DockerSetUlimits     - in(1): string ulimits
 * DockerAddUlimit      - in(1): string single ulimit
 * DockerSetRealtime    - in(1): bool enabled
 * DockerInstallFromScript - in(1): string container ID
 * RestartDockerService - out: error
 * GetHostConfigPath    - in(1): string containerID, out: string path, error
 */
package dock

// DockerSetx11 sets the X11 forward path for the container.
//   in(1): string x11forward - X11 socket forward path
func DockerSetx11(x11forward string) {
	if x11forward != "" {
		dockerObj.x11forward = x11forward
	}
}

// DockerSetShell sets the shell to use in the container.
//   in(1): string shellcmd - command shell to use
func DockerSetShell(shellcmd string) {
	if shellcmd != "" {
		dockerObj.shell = shellcmd
	}
}

// DockerSetSeccomp sets the seccomp profile (empty string keeps default).
//   in(1): string profile - seccomp profile name
func DockerSetSeccomp(profile string) {
	if profile != "" {
		dockerObj.seccomp = profile
	}
}

// DockerAddBinding appends extra bind mounts to the container config.
//   in(1): string addbindings - comma-separated bind mount strings
func DockerAddBinding(addbindings string) {
	if addbindings != "" {
		if dockerObj.extrabinding != "" {
			dockerObj.extrabinding += "," + addbindings
		} else {
			dockerObj.extrabinding = addbindings
		}
	}
}

// DockerAddCgroups appends extra cgroup rules to the container config.
//   in(1): string addcgroups - comma-separated cgroup rules
func DockerAddCgroups(addcgroups string) {
	if addcgroups != "" {
		if dockerObj.cgroups != "" {
			dockerObj.cgroups += "," + addcgroups
		} else {
			dockerObj.cgroups = addcgroups
		}
	}
}

// DockerAddDevices appends extra devices to the container config.
//   in(1): string adddevices - comma-separated device mappings
func DockerAddDevices(adddevices string) {
	if adddevices != "" {
		if dockerObj.devices != "" {
			dockerObj.devices += "," + adddevices
		} else {
			dockerObj.devices = adddevices
		}
	}
}

// DockerAddCaps appends extra capabilities to the container config.
//   in(1): string addcaps - comma-separated capabilities
func DockerAddCaps(addcaps string) {
	if addcaps != "" {
		if dockerObj.caps != "" {
			dockerObj.caps += "," + addcaps
		} else {
			dockerObj.caps = addcaps
		}
	}
}

// DockerSetImage sets the image name to use for container creation.
//   in(1): string imagename - Docker image reference
func DockerSetImage(imagename string) {
	if imagename != "" {
		dockerObj.imagename = imagename
	}
}

// DockerSetPrivileges sets the privilege mode for the container.
//   in(1): int privilege - 1 for privileged, 0 for unprivileged
func DockerSetPrivileges(privilege int) {
	dockerObj.privileged = false
	if privilege == 1 {
		dockerObj.privileged = true
	}
}

// DockerSetXDisplay sets the DISPLAY environment variable value.
//   in(1): string display - X display identifier
func DockerSetXDisplay(display string) {
	if display != "" {
		dockerObj.xdisplay = display
	}
}

// DockerSetEnv sets extra environment variables for the container.
//   in(1): string varenv - comma-separated environment variables
func DockerSetEnv(varenv string) {
	if varenv != "" {
		dockerObj.extraenv = varenv
	}
}

// DockerSetPulse sets the PULSE_SERVER environment variable.
//   in(1): string pulseserv - PulseAudio server address
func DockerSetPulse(pulseserv string) {
	if pulseserv != "" {
		dockerObj.pulse_server = pulseserv
	}
}

// DockerSetExtraHosts sets extra host entries for the container.
//   in(1): string extrahosts - comma-separated host entries
func DockerSetExtraHosts(extrahosts string) {
	if extrahosts != "" {
		dockerObj.extrahosts = extrahosts
	}
}

// DockerSetNetworkMode sets the container network mode.
//   in(1): string networkmode - network mode (host, bridge, etc.)
func DockerSetNetworkMode(networkmode string) {
	if networkmode != "" {
		dockerObj.network_mode = networkmode
	}
}

// DockerSetExposedPorts sets exposed ports for the container.
//   in(1): string exposedports - comma-separated port specifications
func DockerSetExposedPorts(exposedports string) {
	if exposedports != "" {
		dockerObj.exposed_ports = exposedports
	}
}

// DockerSetBindexPorts sets port bindings for the container.
//   in(1): string bindedports - port binding specifications
func DockerSetBindexPorts(bindedports string) {
	if bindedports != "" {
		dockerObj.binded_ports = bindedports
	}
}

// DockerSetUlimits sets ulimits for the container.
//   in(1): string ulimits - comma-separated ulimit specifications
func DockerSetUlimits(ulimits string) {
	dockerObj.ulimits = ulimits
}

// DockerAddUlimit appends a single ulimit to existing ulimits.
//   in(1): string ulimit - ulimit specification (e.g., "rtprio=95")
func DockerAddUlimit(ulimit string) {
	if ulimit == "" {
		return
	}
	if dockerObj.ulimits == "" {
		dockerObj.ulimits = ulimit
	} else {
		dockerObj.ulimits = dockerObj.ulimits + "," + ulimit
	}
}

// DockerSetRealtime enables or disables realtime mode (SYS_NICE + rtprio ulimit).
//   in(1): bool enabled - whether to enable realtime mode
func DockerSetRealtime(enabled bool) {
	dockerObj.realtime = enabled
}

// TODO: Optimize it and handle errors
// DockerInstallFromScript runs hot install inside a created container.
//   in(1): string contid - container identifier
func DockerInstallFromScript(contid string) {
	DockerInstallScript(contid, "entrypoint.sh", dockerObj.shell)
}

// RestartDockerService restarts the active container engine service.
func RestartDockerService() error {
	return EngineRestartService()
}

// GetHostConfigPath returns the host config file path for a container.
//   in(1): string containerID - container identifier
//   out: string path, error
func GetHostConfigPath(containerID string) (string, error) {
	return EngineGetHostConfigPath(containerID)
}
