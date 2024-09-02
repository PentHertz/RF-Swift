/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// isCommandAvailable checks if a command is available in the system
func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// printErrorMessage prints the error message in red with installation commands
func printErrorMessage() {
	red := "\033[0;31m"
	reset := "\033[0m"
	fmt.Printf("%sxhost is not installed. Please install it using the following command for your distribution:%s\n", red, reset)

	if runtime.GOOS == "darwin" {
		// macOS specific installation command
		fmt.Println("brew install xquartz")
		fmt.Println("After installation, start XQuartz and ensure 'Allow connections from network clients' is checked in XQuartz Preferences.")
		return
	}

	osRelease, err := os.ReadFile("/etc/os-release")
	if err != nil {
		fmt.Println("Please refer to your distribution's package manager documentation to install xhost.")
		return
	}

	// Parse /etc/os-release to find the distribution ID
	distID := ""
	lines := strings.Split(string(osRelease), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "ID=") {
			distID = strings.TrimPrefix(line, "ID=")
			distID = strings.Trim(distID, "\"")
			break
		}
	}

	switch distID {
	case "ubuntu", "debian":
		fmt.Println("sudo apt-get install x11-xserver-utils")
	case "fedora":
		fmt.Println("sudo dnf install xorg-x11-server-utils")
	case "centos", "rhel":
		fmt.Println("sudo yum install xorg-x11-server-utils")
	case "arch":
		fmt.Println("sudo pacman -S xorg-xhost")
	default:
		fmt.Println("Please refer to your distribution's package manager documentation to install xhost.")
	}
}

// HostCmdExec executes the given command
func HostCmdExec(cmd string) {
	err := exec.Command("sh", "-c", cmd).Run()
	if err != nil {
		fmt.Printf("Error executing command '%s': %v\n", cmd, err)
	}
}

func XHostEnable() {
	// Check if xhost is installed
	if !isCommandAvailable("xhost") {
		printErrorMessage()
		return
	}

	// Adding local hostname in ACLs
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

func GetDisplayEnv() string {
	var dispenv string
	display, err := displayEnv()
	if err != nil {
		fmt.Println("Error (using default 'DISPLAY=:0 value'):", err)
		dispenv = "DISPLAY=:0"
	} else {
		dispenv = "DISPLAY=" + display
	}
	return dispenv
}

func ClearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}
