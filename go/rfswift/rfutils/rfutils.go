/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	common "penthertz/rfswift/common"
)

// isCommandAvailable reports whether the executable named name can be found
// in the directories listed in the PATH environment variable.
//
//	in(1): string name  name of the executable to look up (e.g. "xhost")
//	out: bool  true if the executable exists in PATH, false otherwise
func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// printErrorMessage prints a red error message to stdout explaining that xhost
// is not installed, together with the appropriate package-manager command for
// the detected operating system or Linux distribution.
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

// HostCmdExec executes cmd as a shell command via "sh -c" and prints an error
// message to stdout if execution fails.
//
//	in(1): string cmd  shell command string to execute
func HostCmdExec(cmd string) {
	err := exec.Command("sh", "-c", cmd).Run()
	if err != nil {
		fmt.Printf("Error executing command '%s': %v\n", cmd, err)
	}
}

// XHostEnable grants the local root user (Linux/other) or the host's en0 IP
// address (macOS) access to the X11 display by running the appropriate xhost
// command. If xhost is not installed, an installation hint is printed instead.
func XHostEnable() {
	// Check if xhost is installed
	if !isCommandAvailable("xhost") {
		printErrorMessage()
		return
	}

	if runtime.GOOS == "darwin" {
		// macOS-specific command
		ip, err := exec.Command("ipconfig", "getifaddr", "en0").Output()
		if err != nil {
			fmt.Println("Error getting IP address on macOS:", err)
			return
		}
		cmd := fmt.Sprintf("xhost + %s", strings.TrimSpace(string(ip)))
		HostCmdExec(cmd)
	} else {
		// Default command for other OS
		s := "xhost local:root"
		HostCmdExec(s)
	}
}

// displayEnv returns the value of the DISPLAY environment variable, or an
// error if the variable is not set.
//
//	out: string  value of the DISPLAY environment variable
//	out: error   non-nil when the DISPLAY variable is empty or unset
func displayEnv() (string, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		return "", fmt.Errorf("DISPLAY environment variable is not set")
	}
	return display, nil
}

// GetDisplayEnv returns a DISPLAY environment string suitable for passing to a
// container. On macOS it resolves the en0 IP address and appends the current
// display number; on other systems it reads the DISPLAY variable directly,
// falling back to ":0" on error.
//
//	out: string  "DISPLAY=<value>" string ready to be injected as an environment variable
func GetDisplayEnv() string {
	var dispenv string

	if runtime.GOOS == "darwin" {
		// macOS-specific handling
		currentDisplay := os.Getenv("DISPLAY")
		var displayNumber string

		// Extract the display number (e.g., ":0" from "path:0")
		if currentDisplay != "" {
			parts := strings.Split(currentDisplay, ":")
			if len(parts) > 1 {
				displayNumber = ":" + parts[1] // Retain the display number
			} else {
				displayNumber = ":0" // Fallback if the format is unexpected
			}
		} else {
			displayNumber = ":0" // Default if DISPLAY is not set
		}

		// Get the IP address and append the display number
		ip, err := exec.Command("ipconfig", "getifaddr", "en0").Output()
		if err != nil {
			fmt.Println("Error determining IP address (using default 'DISPLAY=:0'):", err)
			return "DISPLAY=:0"
		}
		dispenv = "DISPLAY=" + strings.TrimSpace(string(ip)) + displayNumber
	} else {
		// Default behavior for other OS
		display, err := displayEnv()
		if err != nil {
			fmt.Println("Error (using default 'DISPLAY=:0'):", err)
			dispenv = "DISPLAY=:0"
		} else {
			dispenv = "DISPLAY=" + display
		}
	}

	return dispenv
}

// ClearScreen clears the terminal by running the "clear" command and writing
// its output to stdout.
func ClearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// DisplayVersion fetches the latest RF-Swift release from GitHub and compares
// it against the running binary version. If the binary is up-to-date an info
// notification is shown; otherwise a warning is printed. The function is a
// no-op when common.Disconnected is true.
func DisplayVersion() {
	if common.Disconnected {
		return
	}

	owner := common.Owner
	repo := common.Repo

	release, err := GetLatestRelease(owner, repo)
	if err != nil {
		DisplayNotification(
			"Error",
			fmt.Sprintf("Unable to fetch the latest release.\nDetails: %v", err),
			"error",
		)
		return
	}

	currentVersion := common.Version
	latestVersion := release.TagName

	compareResult := VersionCompare(currentVersion, latestVersion)
	if compareResult >= 0 {
		common.PrintInfoMessage(fmt.Sprintf("Up-to-date: you are running the latest version %s", currentVersion))
		return
	}

	common.PrintWarningMessage(fmt.Sprintf("Current version: %s\nLatest version: %s", currentVersion, latestVersion))
}
