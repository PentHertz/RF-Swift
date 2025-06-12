/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/FlUxIuS/pulseaudio_2"
	common "penthertz/rfswift/common"
)

// AudioSystem represents the type of audio system
type AudioSystem int

const (
	AudioSystemPulse AudioSystem = iota
	AudioSystemPipeWire
	AudioSystemUnknown
)

// USBDevice represents a USB device information
type USBDevice struct {
	BusID       string
	DeviceID    string
	VendorID    string
	ProductID   string
	Description string
}

func ListUSBDevices() ([]USBDevice, error) {
	/*
		*	ListUSBDevices executes the usbipd.exe command and lists USB devices
		*	out(1): USBDevice array
			out(2): Errors
	*/
	// Execute the usbipd.exe command
	cmd := exec.Command("usbipd.exe", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute usbipd.exe: %w", err)
	}

	// Parse the output
	var devices []USBDevice
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "BusID") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			device := USBDevice{
				BusID:       fields[0],
				DeviceID:    fields[1],
				VendorID:    fields[2],
				ProductID:   fields[3],
				Description: strings.Join(fields[4:], " "),
			}
			devices = append(devices, device)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading command output: %w", err)
	}

	return devices, nil
}

func AttachUSBDevice(busID string) error {
	/*
	*	AttachUSBDevice attaches a USB device using its BusID
	*	in(1): bus ID string to attach
	*	out: error
	 */
	cmd := exec.Command("usbipd.exe", "attach", "--wsl", "--busid", busID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to attach device %s: %w", busID, err)
	}
	return nil
}

func BindUSBDevice(busID string) error {
	/*
	*	BindUSBDevice binds a USB device using its BusID
	*	in(1): bus ID string to bind
	*	out: error
	 */
	cmd := exec.Command("usbipd.exe", "bind", "--busid", busID) // autoattach
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bind device %s: %w", busID, err)
	}
	return nil
}

func BindAndAttachDevice(busID string) {
	/*
	*	BindAndAttachAllDevices binds and attaches all listed USB devices
	*	in(1): array of bus ID to attach
	*	out: error
	 */
	if err := BindUSBDevice(busID); err != nil {
		fmt.Println("Error binding devices:", err)
	}

	if err := AttachUSBDevice(busID); err != nil {
		fmt.Println("Error attaching devices:", err)
	}
}

func UnbindAndDetachDevice(busID string) {
	/*
	*	Unbind and detach a specific USB device
	*	in(1): array of bus ID string to unbind and detach
	*	out: error
	 */
	if err := UnbindUSBDevice(busID); err != nil {
		fmt.Println("Error unbinding device:", err)
	}

	if err := DetachUSBDevice(busID); err != nil {
		fmt.Println("Error detaching device:", err)
	}
}

// TODO: find a way to blacklist some buses like the keyboard...
func BindAndAttachAllDevices(devices []USBDevice) error {
	/*
	*	BindAndAttachAllDevices binds and attaches all listed USB devices
	*	in(1): array of bus ID to attach
	*	out: error
	 */
	for _, device := range devices {
		if err := BindUSBDevice(device.BusID); err != nil {
			return err
		}
		if err := AttachUSBDevice(device.BusID); err != nil {
			return err
		}
	}
	return nil
}

func UnbindUSBDevice(busID string) error {
	/*
	*	UnbindUSBDevice unbinds a USB device using its BusID
	*	in(1): bus ID string to attach
	*	out: error
	 */
	cmd := exec.Command("usbipd.exe", "unbind", "--busid", busID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to unbind device %s: %w", busID, err)
	}
	return nil
}

func DetachUSBDevice(busID string) error {
	/*
	*	DetachUSBDevice detaches a USB device using its BusID
	*	in(1): bus ID string to attach
	*	out: error
	 */
	cmd := exec.Command("usbipd.exe", "detach", "--busid", busID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to detach device %s: %w", busID, err)
	}
	return nil
}

// TODO: find a way to blacklist some buses like the keyboard...
func UnbindAndDetachAllDevices(devices []USBDevice) error {
	/*
	*	Unbind and detach all USB devices
	*	in(1): array of bus ID string to unbind and detach
	*	out: error
	 */
	for _, device := range devices {
		if err := UnbindUSBDevice(device.BusID); err != nil {
			return err
		}
		if err := DetachUSBDevice(device.BusID); err != nil {
			return err
		}
	}
	return nil
}

func BindAttachUSB_Windows(busID string) {
	/*
	*	Bind a specific USB device from the Windows host
	 */
	devices, err := ListUSBDevices()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for _, device := range devices {
		fmt.Printf("BusID: %s, DeviceID: %s, VendorID: %s, ProductID: %s, Description: %s\n",
			device.BusID, device.DeviceID, device.VendorID, device.ProductID, device.Description)
	}

	if err := BindAndAttachAllDevices(devices); err != nil {
		fmt.Println("Error binding and attaching devices:", err)
	}
}

func AutoBindAttachUSB_Windows() {
	/*
	*	Automatically bind all USB devices from the Windows host
	 */
	devices, err := ListUSBDevices()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for _, device := range devices {
		fmt.Printf("BusID: %s, DeviceID: %s, VendorID: %s, ProductID: %s, Description: %s\n",
			device.BusID, device.DeviceID, device.VendorID, device.ProductID, device.Description)
	}

	if err := BindAndAttachAllDevices(devices); err != nil {
		fmt.Println("Error binding and attaching devices:", err)
	}
}

func AutoUnbindDetachUSB_Windows() {
	/*
	*	Automatically Unbind and detach all USB devices from the Windows host
	 */
	devices, err := ListUSBDevices()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("USB Devices:")
	for _, device := range devices {
		fmt.Printf("BusID: %s, DeviceID: %s, VendorID: %s, ProductID: %s, Description: %s\n",
			device.BusID, device.DeviceID, device.VendorID, device.ProductID, device.Description)
	}

	fmt.Println("\nUnbinding and detaching all devices...")
	if err := UnbindAndDetachAllDevices(devices); err != nil {
		fmt.Println("Error unbinding and detaching devices:", err)
		return
	}

	fmt.Println("Operation completed successfully.")
}

// detectAudioSystem detects whether PulseAudio or PipeWire is running
func detectAudioSystem() AudioSystem {
	// Check if PipeWire is running
	if isPipeWireRunning() {
		return AudioSystemPipeWire
	}
	
	// Check if PulseAudio is running
	if isPulseAudioRunning() {
		return AudioSystemPulse
	}
	
	return AudioSystemUnknown
}

// isPipeWireRunning checks if PipeWire is running
func isPipeWireRunning() bool {
	// Check if pipewire process is running
	cmd := exec.Command("pgrep", "-x", "pipewire")
	if err := cmd.Run(); err == nil {
		return true
	}
	
	// Alternative check: try to connect to PipeWire socket
	if _, err := os.Stat("/run/user/" + os.Getenv("USER") + "/pipewire-0"); err == nil {
		return true
	}
	
	return false
}

// isPulseAudioRunning checks if PulseAudio is running
func isPulseAudioRunning() bool {
	cmd := exec.Command("pulseaudio", "--check")
	return cmd.Run() == nil
}

func checkPulseServer(address string, port string) {
	// Combine address and port to create the endpoint
	endpoint := net.JoinHostPort(address, port)

	// Attempt to establish a connection
	conn, err := net.DialTimeout("tcp", endpoint, 5*time.Second)
	if err != nil {
		// Connection failed, prepare the error message
		message := fmt.Sprintf("\033[33mWarning: Unable to connect to audio server at %s\033[0m\n", endpoint)
		message += retInstallationInstructions()

		// Display the notification
		DisplayNotification(" Warning", message, "warning")
		return
	}

	// Close the connection if successful
	conn.Close()

	// Prepare success message
	successMessage := fmt.Sprintf("Audio server found at %s", endpoint)

	// Display success notification
	DisplayNotification(" Audio", successMessage, "info")
}

// detectLinuxDistribution detects the Linux distribution
func detectLinuxDistribution() string {
	// Check for specific distribution files
	distributions := map[string]string{
		"/etc/arch-release":    "arch",
		"/etc/fedora-release":  "fedora",
		"/etc/redhat-release":  "rhel",
		"/etc/centos-release":  "centos",
		"/etc/debian_version":  "debian",
		"/etc/lsb-release":     "ubuntu", // Will be refined further
	}

	for file, distro := range distributions {
		if _, err := os.Stat(file); err == nil {
			// Special handling for some distributions
			if distro == "ubuntu" {
				if content, err := os.ReadFile(file); err == nil {
					if strings.Contains(string(content), "Ubuntu") {
						return "ubuntu"
					}
				}
				// If lsb-release exists but doesn't contain Ubuntu, continue checking
				continue
			}
			if distro == "rhel" {
				// Distinguish between RHEL, CentOS, and Fedora
				if content, err := os.ReadFile(file); err == nil {
					contentStr := strings.ToLower(string(content))
					if strings.Contains(contentStr, "centos") {
						return "centos"
					}
					if strings.Contains(contentStr, "fedora") {
						return "fedora"
					}
					if strings.Contains(contentStr, "red hat") {
						return "rhel"
					}
				}
			}
			return distro
		}
	}

	// Check /etc/os-release as a fallback
	if content, err := os.ReadFile("/etc/os-release"); err == nil {
		contentStr := strings.ToLower(string(content))
		if strings.Contains(contentStr, "ubuntu") {
			return "ubuntu"
		}
		if strings.Contains(contentStr, "debian") {
			return "debian"
		}
		if strings.Contains(contentStr, "fedora") {
			return "fedora"
		}
		if strings.Contains(contentStr, "rhel") || strings.Contains(contentStr, "red hat") {
			return "rhel"
		}
		if strings.Contains(contentStr, "centos") {
			return "centos"
		}
		if strings.Contains(contentStr, "arch") {
			return "arch"
		}
	}

	return "unknown"
}

// getPackageManager returns the appropriate package manager for the current distribution
func getPackageManager() string {
	switch detectLinuxDistribution() {
	case "arch":
		return "pacman"
	case "fedora":
		return "dnf"
	case "rhel", "centos":
		// Check if it's a newer version that uses dnf
		if _, err := exec.LookPath("dnf"); err == nil {
			return "dnf"
		}
		return "yum"
	case "debian", "ubuntu":
		return "apt"
	default:
		return "unknown"
	}
}

// getRHELVersion returns the major version number of RHEL/CentOS
func getRHELVersion() int {
	// Try to read version from various files
	files := []string{"/etc/redhat-release", "/etc/centos-release", "/etc/os-release"}
	
	for _, file := range files {
		if content, err := os.ReadFile(file); err == nil {
			contentStr := string(content)
			
			// Look for version patterns
			if strings.Contains(contentStr, "release 9") || strings.Contains(contentStr, "VERSION_ID=\"9") {
				return 9
			}
			if strings.Contains(contentStr, "release 8") || strings.Contains(contentStr, "VERSION_ID=\"8") {
				return 8
			}
			if strings.Contains(contentStr, "release 7") || strings.Contains(contentStr, "VERSION_ID=\"7") {
				return 7
			}
		}
	}
	
	return 8 // Default to 8 if unable to determine
}

func retInstallationInstructions() string {
	var retstring strings.Builder
	os := runtime.GOOS

	switch os {
	case "windows":
		retstring.WriteString("\nTo install audio server on Windows, follow these steps:\n")
		retstring.WriteString("1. Download the PulseAudio server installer from the official website.\n")
		retstring.WriteString("2. Run the installer and follow the on-screen instructions.\n")
	case "darwin":
		retstring.WriteString("To install audio server on macOS, follow these steps:\n")
		retstring.WriteString("1. Install Homebrew if you haven't already: /bin/bash -c \"$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\"\n")
		retstring.WriteString("2. Install PulseAudio using Homebrew: brew install pulseaudio\n")
		retstring.WriteString("   OR install PipeWire: brew install pipewire\n")
	case "linux":
		distro := detectLinuxDistribution()
		switch distro {
		case "arch":
			retstring.WriteString("\nTo install audio server on Arch Linux, follow these steps:\n")
			retstring.WriteString("1. Update your package database: sudo pacman -Syu\n")
			retstring.WriteString("2. Install PipeWire (recommended): sudo pacman -S pipewire pipewire-pulse pipewire-alsa\n")
			retstring.WriteString("   OR install PulseAudio: sudo pacman -S pulseaudio pulseaudio-alsa\n")
			retstring.WriteString("3. Enable user services: systemctl --user enable pipewire pipewire-pulse\n")
		case "fedora":
			retstring.WriteString("\nTo install audio server on Fedora, follow these steps:\n")
			retstring.WriteString("1. Update your system: sudo dnf update\n")
			retstring.WriteString("2. Install PipeWire (default since Fedora 34): sudo dnf install pipewire pipewire-pulseaudio pipewire-alsa\n")
			retstring.WriteString("   OR install PulseAudio: sudo dnf install pulseaudio pulseaudio-utils\n")
			retstring.WriteString("3. Enable user services: systemctl --user enable pipewire pipewire-pulse\n")
			retstring.WriteString("Note: PipeWire is the default audio system in Fedora 34+\n")
		case "rhel":
			version := getRHELVersion()
			retstring.WriteString("\nTo install audio server on Red Hat Enterprise Linux, follow these steps:\n")
			retstring.WriteString(fmt.Sprintf("1. Update your system: sudo %s update\n", getPackageManager()))
			if version >= 8 {
				retstring.WriteString("For RHEL 8+:\n")
				retstring.WriteString("2. Install PipeWire: sudo dnf install pipewire pipewire-pulseaudio pipewire-alsa\n")
				retstring.WriteString("   OR install PulseAudio: sudo dnf install pulseaudio pulseaudio-utils\n")
				retstring.WriteString("3. Enable user services: systemctl --user enable pipewire pipewire-pulse\n")
			} else {
				retstring.WriteString("For RHEL 7:\n")
				retstring.WriteString("2. Install PulseAudio: sudo yum install pulseaudio pulseaudio-utils\n")
				retstring.WriteString("3. Enable EPEL repository: sudo yum install epel-release\n")
			}
		case "centos":
			version := getRHELVersion()
			retstring.WriteString("\nTo install audio server on CentOS, follow these steps:\n")
			retstring.WriteString(fmt.Sprintf("1. Update your system: sudo %s update\n", getPackageManager()))
			if version >= 8 {
				retstring.WriteString("For CentOS Stream 8+:\n")
				retstring.WriteString("2. Install PipeWire: sudo dnf install pipewire pipewire-pulseaudio pipewire-alsa\n")
				retstring.WriteString("   OR install PulseAudio: sudo dnf install pulseaudio pulseaudio-utils\n")
				retstring.WriteString("3. Enable user services: systemctl --user enable pipewire pipewire-pulse\n")
			} else {
				retstring.WriteString("For CentOS 7:\n")
				retstring.WriteString("2. Install PulseAudio: sudo yum install pulseaudio pulseaudio-utils\n")
				retstring.WriteString("3. Enable EPEL repository: sudo yum install epel-release\n")
			}
		case "debian":
			retstring.WriteString("\nTo install audio server on Debian, follow these steps:\n")
			retstring.WriteString("1. Update your package database: sudo apt update\n")
			retstring.WriteString("2. Install PipeWire (Debian 11+): sudo apt install pipewire pipewire-pulse pipewire-alsa\n")
			retstring.WriteString("   OR install PulseAudio: sudo apt install pulseaudio pulseaudio-utils\n")
			retstring.WriteString("3. Enable user services: systemctl --user enable pipewire pipewire-pulse\n")
		case "ubuntu":
			retstring.WriteString("\nTo install audio server on Ubuntu, follow these steps:\n")
			retstring.WriteString("1. Update your package database: sudo apt update\n")
			retstring.WriteString("2. Install PipeWire (Ubuntu 22.04+): sudo apt install pipewire pipewire-pulse pipewire-alsa\n")
			retstring.WriteString("   OR install PulseAudio: sudo apt install pulseaudio pulseaudio-utils\n")
			retstring.WriteString("3. Enable user services: systemctl --user enable pipewire pipewire-pulse\n")
		default:
			retstring.WriteString("\nTo install audio server on Linux, follow these steps:\n")
			retstring.WriteString("1. Update your package manager:\n")
			retstring.WriteString("   - Debian/Ubuntu: sudo apt update\n")
			retstring.WriteString("   - Fedora: sudo dnf update\n")
			retstring.WriteString("   - RHEL/CentOS 8+: sudo dnf update\n")
			retstring.WriteString("   - RHEL/CentOS 7: sudo yum update\n")
			retstring.WriteString("2. Install audio server:\n")
			retstring.WriteString("   - PipeWire (recommended for modern systems):\n")
			retstring.WriteString("     Debian/Ubuntu: sudo apt install pipewire pipewire-pulse pipewire-alsa\n")
			retstring.WriteString("     Fedora/RHEL8+: sudo dnf install pipewire pipewire-pulseaudio pipewire-alsa\n")
			retstring.WriteString("   - PulseAudio:\n")
			retstring.WriteString("     Debian/Ubuntu: sudo apt install pulseaudio pulseaudio-utils\n")
			retstring.WriteString("     Fedora/RHEL8+: sudo dnf install pulseaudio pulseaudio-utils\n")
			retstring.WriteString("     RHEL/CentOS 7: sudo yum install pulseaudio pulseaudio-utils\n")
		}
	default:
		retstring.WriteString("\nPlease refer to the official audio server documentation for installation instructions.\n")
	}

	retstring.WriteString("\n\nAfter installation, enable the module with the following command as unprivileged user:\n")
	retstring.WriteString("\033[33m./rfswift host audio enable\033[0m")

	return retstring.String()
}

// isArchLinux checks if the current Linux distribution is Arch Linux
func isArchLinux() bool {
	return detectLinuxDistribution() == "arch"
}

// isFedora checks if the current Linux distribution is Fedora
func isFedora() bool {
	return detectLinuxDistribution() == "fedora"
}

// isRedHat checks if the current Linux distribution is Red Hat based
func isRedHat() bool {
	distro := detectLinuxDistribution()
	return distro == "rhel" || distro == "centos" || distro == "fedora"
}

// ensureAudioSystemRunning checks if audio system is running and starts it if not.
func ensureAudioSystemRunning() error {
	audioSystem := detectAudioSystem()
	
	switch audioSystem {
	case AudioSystemPipeWire:
		return ensurePipeWireRunning()
	case AudioSystemPulse:
		return ensurePulseAudioRunning()
	default:
		// Try to start PipeWire first, then PulseAudio
		if err := ensurePipeWireRunning(); err != nil {
			return ensurePulseAudioRunning()
		}
		return nil
	}
}

// ensurePulseAudioRunning checks if PulseAudio is running and starts it if not.
func ensurePulseAudioRunning() error {
	cmd := exec.Command("pulseaudio", "--check")
	if err := cmd.Run(); err != nil {
		// If PulseAudio is not running, start it
		startCmd := exec.Command("pulseaudio", "--start")
		if startErr := startCmd.Run(); startErr != nil {
			return fmt.Errorf("failed to start PulseAudio: %w", startErr)
		}
		common.PrintSuccessMessage(fmt.Sprintf("PulseAudio started successfully."))
		time.Sleep(2 * time.Second) // Wait for 2 seconds
	} else {
		common.PrintInfoMessage(fmt.Sprintf("PulseAudio is already running."))
	}
	return nil
}

// ensurePipeWireRunning checks if PipeWire is running and starts it if not.
func ensurePipeWireRunning() error {
	if isPipeWireRunning() {
		common.PrintInfoMessage("PipeWire is already running.")
		return nil
	}
	
	// Try systemd user services first (preferred method)
	if err := startPipeWireSystemd(); err == nil {
		common.PrintSuccessMessage("PipeWire started successfully via systemd.")
		time.Sleep(2 * time.Second)
		return nil
	}
	
	// Fallback: try starting pipewire directly
	directStartCmd := exec.Command("pipewire")
	if err := directStartCmd.Start(); err != nil {
		return fmt.Errorf("failed to start PipeWire directly: %w", err)
	}
	
	// For Red Hat/Fedora systems, also try to start additional services
	if isRedHat() {
		startRedHatPipeWireServices()
	}
	
	common.PrintSuccessMessage("PipeWire started successfully.")
	time.Sleep(2 * time.Second)
	return nil
}

// startPipeWireSystemd starts PipeWire using systemd user services
func startPipeWireSystemd() error {
	services := []string{"pipewire.service", "pipewire-pulse.service"}
	
	for _, service := range services {
		cmd := exec.Command("systemctl", "--user", "start", service)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to start %s: %w", service, err)
		}
	}
	
	// Also try to start wireplumber if available (session manager)
	wireplumberCmd := exec.Command("systemctl", "--user", "start", "wireplumber.service")
	wireplumberCmd.Run() // Ignore errors as wireplumber might not be installed
	
	return nil
}

// startRedHatPipeWireServices starts additional PipeWire services specific to Red Hat systems
func startRedHatPipeWireServices() {
	additionalServices := []string{
		"pipewire-media-session.service", // Legacy session manager
		"wireplumber.service",            // Modern session manager
	}
	
	for _, service := range additionalServices {
		cmd := exec.Command("systemctl", "--user", "start", service)
		cmd.Run() // Ignore errors as these services might not be available
	}
}

func SetPulseCTL(address string) error {
	/*
	 * Use PACTL in command line to accept connection in TCP with defined port
	 * Works with both PulseAudio and PipeWire (via pipewire-pulse)
	 */
	parts := strings.Split(address, ":")
	if len(parts) != 3 {
		return fmt.Errorf("invalid address format, expected format 'protocol:ip:port'")
	}
	port := parts[2]
	ip := parts[1]

	// Ensure audio system is running
	if err := ensureAudioSystemRunning(); err != nil {
		return fmt.Errorf("failed to ensure audio system is running: %w", err)
	}

	audioSystem := detectAudioSystem()
	
	switch audioSystem {
	case AudioSystemPipeWire:
		return setPipeWireTCPModule(ip, port)
	case AudioSystemPulse:
		return setPulseAudioTCPModule(ip, port)
	default:
		// Try PulseAudio method as fallback (should work with pipewire-pulse)
		return setPulseAudioTCPModule(ip, port)
	}
}

// setPulseAudioTCPModule sets up TCP module for PulseAudio
func setPulseAudioTCPModule(ip, port string) error {
	// Connect to PulseAudio
	client, err := pulseaudio.NewClient()
	if err != nil {
		return fmt.Errorf("failed to connect to PulseAudio: %w", err)
	}
	defer client.Close()

	// Construct the module arguments string
	moduleArgs := fmt.Sprintf("port=%s auth-ip-acl=%s", port, ip)

	// Load module-native-protocol-tcp with the specified IP and port
	moduleIndex, err := client.LoadModule("module-native-protocol-tcp", moduleArgs)
	if err != nil {
		return fmt.Errorf("failed to load module-native-protocol-tcp: %w", err)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Successfully loaded module-native-protocol-tcp with index %d", moduleIndex))
	return nil
}

// setPipeWireTCPModule sets up TCP module for PipeWire using pactl (pipewire-pulse compatibility)
func setPipeWireTCPModule(ip, port string) error {
	// PipeWire with pipewire-pulse should support pactl commands
	moduleArgs := fmt.Sprintf("port=%s auth-ip-acl=%s", port, ip)
	
	cmd := exec.Command("pactl", "load-module", "module-native-protocol-tcp", moduleArgs)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to load module-native-protocol-tcp via pactl: %w\nOutput: %s", err, string(output))
	}
	
	common.PrintSuccessMessage(fmt.Sprintf("Successfully loaded module-native-protocol-tcp via PipeWire"))
	return nil
}

func UnloadPulseCTL() error {
	/*
	*	Unload audio TCP module (works with both PulseAudio and PipeWire)
	 */
	cmd := exec.Command("pactl", "list", "modules")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list audio modules: %w\nOutput: %s", err, string(output))
	}

	// Parse the output to find the module-native-protocol-tcp index
	lines := strings.Split(string(output), "\n")
	var moduleIndex string
	for i, line := range lines {
		if strings.Contains(line, "Name: module-native-protocol-tcp") {
			// Find the "Index:" line above the module name
			for j := i; j >= 0; j-- {
				if strings.Contains(lines[j], "Module #") {
					moduleIndex = strings.TrimSpace(strings.TrimPrefix(lines[j], "Module #"))
					break
				}
			}
			break
		}
	}

	if moduleIndex == "" {
		return fmt.Errorf("module-native-protocol-tcp not found")
	}

	// Execute pactl unload-module to unload the module
	unloadCmd := exec.Command("pactl", "unload-module", moduleIndex)
	unloadOutput, err := unloadCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to unload module-native-protocol-tcp: %w\nOutput: %s", err, string(unloadOutput))
	}
	fmt.Printf("Command output: %s\n", string(unloadOutput))

	audioSystemName := "audio system"
	if detectAudioSystem() == AudioSystemPipeWire {
		audioSystemName = "PipeWire"
	} else if detectAudioSystem() == AudioSystemPulse {
		audioSystemName = "PulseAudio"
	}

	common.PrintSuccessMessage(fmt.Sprintf("Successfully unloaded module-native-protocol-tcp from %s with index %s", audioSystemName, moduleIndex))
	return nil
}

// Additional PipeWire-specific functions

// GetPipeWireInfo returns information about the current PipeWire session
func GetPipeWireInfo() (string, error) {
	cmd := exec.Command("pw-cli", "info")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get PipeWire info: %w", err)
	}
	return string(output), nil
}

// ListPipeWireNodes lists all PipeWire nodes
func ListPipeWireNodes() (string, error) {
	cmd := exec.Command("pw-cli", "list-objects", "Node")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list PipeWire nodes: %w", err)
	}
	return string(output), nil
}

// GetAudioSystemStatus returns the status of the current audio system
func GetAudioSystemStatus() string {
	audioSystem := detectAudioSystem()
	
	switch audioSystem {
	case AudioSystemPipeWire:
		if info, err := GetPipeWireInfo(); err == nil {
			return fmt.Sprintf("PipeWire is running\n%s", info)
		}
		return "PipeWire is running (info unavailable)"
	case AudioSystemPulse:
		return "PulseAudio is running"
	default:
		return "No audio system detected"
	}
}