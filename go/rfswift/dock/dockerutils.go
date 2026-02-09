/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */
package dock

func DockerSetx11(x11forward string) {
	/* Sets the shell to use in the Docker container
	   in(1): string command shell to use
	*/
	if x11forward != "" {
		dockerObj.x11forward = x11forward
	}
}

func DockerSetShell(shellcmd string) {
	/* Sets the shell to use in the Docker container
	   in(1): string command shell to use
	*/
	if shellcmd != "" {
		dockerObj.shell = shellcmd
	}
}

func DockerSetSeccomp(profile string) {
	/* Sets the seccomp profile to use in the Docker container (empty => default profile)
	   in(1): string indicating the profile to use
	*/
	if profile != "" {
		dockerObj.seccomp = profile
	}
}

func DockerAddBinding(addbindings string) {
	/* Add extra bindings to the Docker container
	   in(1): string of bindings separated by commas
	*/
	if addbindings != "" {
		// Check if extrabinding already has content, and append with a comma if it does
		if dockerObj.extrabinding != "" {
			dockerObj.extrabinding += "," + addbindings
		} else {
			dockerObj.extrabinding = addbindings
		}
	}
}

func DockerAddCgroups(addcgroups string) {
	/* Add extra cgroup rules to the Docker container
	   in(1): string of cgroup rules separated by commas
	*/
	if addcgroups != "" {
		// Check if cgroups already has content, and append with a comma if it does
		if dockerObj.cgroups != "" {
			dockerObj.cgroups += "," + addcgroups
		} else {
			dockerObj.cgroups = addcgroups
		}
	}
}

func DockerAddDevices(adddevices string) {
	/* Add extra devices to the Docker container
	   in(1): string of devices separated by commas
	*/
	if adddevices != "" {
		// Check if extrabinding already has content, and append with a comma if it does
		if dockerObj.devices != "" {
			dockerObj.devices += "," + adddevices
		} else {
			dockerObj.devices = adddevices
		}
	}
}

func DockerAddCaps(addcaps string) {
	/* Add extra caps to the Docker container
	   in(1): string of caps separated by commas
	*/
	if addcaps != "" {
		// Check if extracap already has content, and append with a comma if it does
		if dockerObj.caps != "" {
			dockerObj.caps += "," + addcaps
		} else {
			dockerObj.caps = addcaps
		}
	}
}

func DockerSetImage(imagename string) {
	/* Set image name to use if the default one is not used
	   in(1): string image name
	*/
	if imagename != "" {
		dockerObj.imagename = imagename
	}
}

func DockerSetPrivileges(privilege int) {
	/* Set privilege mode to use on the container
	   in(1): int privileged (1: True, 0:False)
	*/
	dockerObj.privileged = false
	if privilege == 1 {
		dockerObj.privileged = true
	}
}

func DockerSetXDisplay(display string) {
	/* Sets the XDISPLAY env variable value
	   in(1): string display
	*/
	if display != "" {
		dockerObj.xdisplay = display
	}
}

func DockerSetEnv(varenv string) {
	/* Sets the extra env variables value
	   in(1): string varenv
	*/
	if varenv != "" {
		dockerObj.extraenv = varenv
	}
}

func DockerSetPulse(pulseserv string) {
	/* Sets the PULSE_SERVER env variable value
	   in(1): string pulseserv
	*/
	if pulseserv != "" {
		dockerObj.pulse_server = pulseserv
	}
}

func DockerSetExtraHosts(extrahosts string) {
	/* Sets the Extra Hosts value
	   in(1): string extrahosts
	*/
	if extrahosts != "" {
		dockerObj.extrahosts = extrahosts
	}
}

func DockerSetNetworkMode(networkmode string) {
	if networkmode != "" {
		dockerObj.network_mode = networkmode
	}
}

func DockerSetExposedPorts(exposedports string) {
	if exposedports != "" {
		dockerObj.exposed_ports = exposedports
	}
}

func DockerSetBindexPorts(bindedports string) {
	if bindedports != "" {
		dockerObj.binded_ports = bindedports
	}
}

// TODO: Optimize it and handle errors
func DockerInstallFromScript(contid string) {
	/* Hot install inside a created Docker container
	   in(1): string function script to use
	*/
	DockerInstallScript(contid, "entrypoint.sh", dockerObj.shell)
}

func RestartDockerService() error {
    return EngineRestartService()
}

func GetHostConfigPath(containerID string) (string, error) {
    return EngineGetHostConfigPath(containerID)
}