/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lawl/pulseaudio"
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
	cmd := exec.Command("usbipd.exe", "attach", "-a", "--wsl", "--busid", busID)
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

func SetPulseCTL(address string) error {
	/*
	*	Use PACTL in command line to accept connection in TCP with defined port
	*/
	parts := strings.Split(address, ":")
	if len(parts) != 3 {
		return fmt.Errorf("invalid address format, expected format 'protocol:ip:port'")
	}
	port := parts[2]
	ip := parts[1]

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

	fmt.Printf("[+] Successfully loaded module-native-protocol-tcp with index %d\n", moduleIndex)
	return nil
}

func UnloadPulseCTL() error {
	/*
	*	Unload pulseaudio TCP module
	*/
	cmd := exec.Command("pactl", "list", "modules")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list PulseAudio modules: %w\nOutput: %s", err, string(output))
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

	fmt.Printf("[+] Successfully unloaded module-native-protocol-tcp with index %s\n", moduleIndex)
	return nil
}