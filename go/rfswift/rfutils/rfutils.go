/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"fmt"
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
