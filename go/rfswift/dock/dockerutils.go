/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */
package dock

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

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

func DockerSetUnprivileges(privilege bool) {
	/* Set privilege mode to use on the container
	   in(1): bool privileged? True or False
	*/
	if privilege {
		dockerObj.privileged = !privilege
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
	switch runtime.GOOS {
	case "linux":
		return exec.Command("sudo", "systemctl", "restart", "docker").Run()
	case "darwin":
		return exec.Command("osascript", "-e", `do shell script "brew services restart docker" with administrator privileges`).Run()
	case "windows":
		return exec.Command("powershell", "Restart-Service", "Docker").Run()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func GetHostConfigPath(containerID string) (string, error) {
	var configPath string

	switch runtime.GOOS {
	case "linux":
		configPath = fmt.Sprintf("/var/lib/docker/containers/%s/hostconfig.json", containerID)
	case "darwin": // macOS
		configPath = fmt.Sprintf("/var/lib/docker/containers/%s/hostconfig.json", containerID)
	case "windows":
		configPath = fmt.Sprintf("C:\\ProgramData\\docker\\containers\\%s\\hostconfig.json", containerID)
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	// Check if the file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", configPath)
	} else if err != nil {
		return "", fmt.Errorf("error checking file: %v", err)
	}

	return configPath, nil
}
