/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */
package dock

import (
	"fmt"
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

func DockerAddBiding(addbindings string) {
	/* Add extra bindings to the Docker container to run
	   in(1): string of bindings separated by commas
	*/
	if addbindings != "" {
		dockerObj.extrabinding = addbindings
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

// TODO: Optimize it and handle errors
func DockerInstallFromScript(contid string) {
	/* Hot install inside a created Docker container
	   in(1): string function script to use
	*/
	s := fmt.Sprintf("./entrypoint.sh %s", dockerObj.shell)
	fmt.Println(s)
	dockerObj.shell = s
	DockerExec(contid, "/root/scripts")
}
