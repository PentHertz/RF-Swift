/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"fmt"
	"os"
	"os/exec"
)

func HostCmdExec(cmdline string) {
	/*  Executes a command on the host
	    in(1): string cmdline
	*/
	cmd := exec.Command("sh", "-c", cmdline)
	stdout, err := cmd.Output()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	// Print the output
	fmt.Println(string(stdout))
}

func XHostEnable() {
	/*  Adding local hostname in ACLs
	    TODO: implement a check for already added hosts
	*/
	s := "xhost local:root"
	HostCmdExec(s)
}

func displayEnv() (string, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		return "", fmt.Errorf("DISPLAY environment variable is not set")
	}
	return display, nil
}

func GetDisplayEnv() (string) {
	var dispenv string
	display, err := displayEnv()
	if err != nil {
		fmt.Println("Error (using default 'DISPLAY=:0 value'):", err)
		dispenv = "DISPLAY=:0"
	} else {
		dispenv = "DISPLAY="+display
	}
	return dispenv
}