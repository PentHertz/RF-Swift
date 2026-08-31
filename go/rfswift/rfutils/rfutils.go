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
	"time"

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

// X11DisplayHint returns an actionable, one-paragraph hint when this host is
// missing what X11 GUI forwarding needs, or "" when things look ready. On macOS
// that is XQuartz (it provides both the X server and the xhost tool); on Linux
// it is the xhost utility from the distro's X server package.
//
// Callers surface it through the container-creation warn callback so it reaches
// both the CLI (printed as a warning) and the Workbench GUI (collected into the
// mission warnings and shown in the UI). Without it, GUI tools launched in the
// container — gqrx, sdrpp, ... — fail with a bare "could not connect to display"
// / Qt "xcb" plugin error that gives the user nothing to act on.
//
//	out: string  the hint, or "" when X11 tooling is present
func X11DisplayHint() string {
	if runtime.GOOS == "windows" {
		// WSLg provides the display on Windows; nothing to install.
		return ""
	}
	if isCommandAvailable("xhost") {
		return ""
	}
	if runtime.GOOS == "darwin" {
		// GUI apps launched from Finder inherit a minimal PATH that hides
		// XQuartz's /opt/X11/bin, so xhost may be installed yet not on PATH.
		// Treat XQuartz as present when its binary exists on disk.
		if _, err := os.Stat("/opt/X11/bin/xhost"); err == nil {
			return ""
		}
		return "XQuartz is required for X11 GUI tools (gqrx, sdrpp, ...) but is not installed, so the " +
			"container has no display to connect to. RF Swift ships scripts/setup-xquartz-macos.sh which " +
			"installs and configures everything, or do it by hand:\n" +
			"  brew install --cask xquartz\n" +
			"  defaults write org.xquartz.X11 nolisten_tcp -bool false   # allow connections from network clients\n" +
			"  open -a XQuartz   # then log out and back in once for a fresh install"
	}
	return linuxXhostHint()
}

// linuxXhostHint returns the distro-appropriate command to install the xhost
// utility, or a generic pointer when the distribution cannot be determined.
func linuxXhostHint() string {
	const generic = "xhost is not installed. Install your distribution's X server utilities (the package that " +
		"provides xhost) to enable X11 GUI forwarding into the container."

	osRelease, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return generic
	}

	distID := ""
	for _, line := range strings.Split(string(osRelease), "\n") {
		if strings.HasPrefix(line, "ID=") {
			distID = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
			break
		}
	}

	var cmd string
	switch distID {
	case "ubuntu", "debian":
		cmd = "sudo apt-get install x11-xserver-utils"
	case "fedora":
		cmd = "sudo dnf install xorg-x11-server-utils"
	case "centos", "rhel":
		cmd = "sudo yum install xorg-x11-server-utils"
	case "arch":
		cmd = "sudo pacman -S xorg-xhost"
	default:
		return generic
	}
	return "xhost is not installed; X11 GUI tools cannot forward their display. Install it with:\n  " + cmd
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

// resolveXhost returns the path to the xhost executable, or "" when it cannot be
// found. On macOS XQuartz installs xhost under /opt/X11/bin, which is not on the
// default PATH (especially for a GUI app launched from Finder), so fall back to
// that fixed location before giving up.
func resolveXhost() string {
	if p, err := exec.LookPath("xhost"); err == nil {
		return p
	}
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/opt/X11/bin/xhost"); err == nil {
			return "/opt/X11/bin/xhost"
		}
	}
	return ""
}

// ensureXQuartzRunning starts XQuartz on macOS when its X server is not up yet
// and waits briefly for the display socket to appear. Containers forward their
// display to XQuartz over TCP, but a freshly installed or simply not-launched
// XQuartz means nothing is listening on :0/:6000 and every xhost/GUI call fails
// silently. No-op when the server is already running or off macOS.
func ensureXQuartzRunning() {
	if runtime.GOOS != "darwin" {
		return
	}
	// The :0 Unix socket exists exactly while the X server is up.
	if _, err := os.Stat("/tmp/.X11-unix/X0"); err == nil {
		return
	}
	if _, err := os.Stat("/opt/X11/bin/xhost"); err != nil {
		return // XQuartz not installed; X11DisplayHint covers this case.
	}
	_ = exec.Command("open", "-a", "XQuartz").Run()
	for i := 0; i < 40; i++ { // up to ~10s for the server to come up
		if _, err := os.Stat("/tmp/.X11-unix/X0"); err == nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// XHostEnable grants the local root user (Linux/other) or the host's en0 IP
// address (macOS) access to the X11 display by running the appropriate xhost
// command. On macOS it first makes sure XQuartz is actually running. If xhost is
// not installed, an installation hint is printed instead.
func XHostEnable() {
	xhostBin := resolveXhost()
	if xhostBin == "" {
		if hint := X11DisplayHint(); hint != "" {
			common.PrintWarningMessage(hint)
		}
		return
	}

	if runtime.GOOS == "darwin" {
		// A container reaches XQuartz over TCP, so the server must be up and the
		// host IP authorised. Docker Desktop, Lima and OrbStack all present the
		// container's traffic to XQuartz as the host's en0 address.
		ensureXQuartzRunning()
		ip, err := exec.Command("ipconfig", "getifaddr", "en0").Output()
		if err != nil {
			fmt.Println("Error getting IP address on macOS:", err)
			return
		}
		// Pass "+" and the IP as separate arguments (as the original command
		// did) and set DISPLAY=:0 explicitly: XQuartz may have only just been
		// started, so this process's environment has no DISPLAY for xhost to use.
		cmd := exec.Command(xhostBin, "+", strings.TrimSpace(string(ip)))
		cmd.Env = append(os.Environ(), "DISPLAY=:0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error enabling X11 access on macOS (DISPLAY=:0): %v\n", err)
		}
	} else {
		display := strings.TrimPrefix(GetDisplayEnv(), "DISPLAY=")
		// SSH-forwarded displays authenticate with an xauth cookie. Container
		// creation mounts that cookie read-only; xhost is neither needed nor
		// desirable for this case.
		lower := strings.ToLower(display)
		if strings.HasPrefix(lower, "localhost:") || strings.HasPrefix(lower, "127.0.0.1:") || strings.HasPrefix(lower, "[::1]:") {
			return
		}
		cmd := exec.Command(xhostBin, "local:root")
		cmd.Env = append(os.Environ(), "DISPLAY="+display)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error enabling local X11 access on DISPLAY=%s: %v\n", display, err)
		}
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
			// On Windows the container display is WSLg's :0; an unset host
			// DISPLAY is the normal state, not an error worth printing.
			if runtime.GOOS != "windows" {
				fmt.Println("Error (using default 'DISPLAY=:0'):", err)
			}
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
