/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
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
	*	SetPulseCTL Set pulse server IP and TCP port ACLs
	*	in(1): ip string
	*	in(2): port string
	*	out: error
	 */

	parts := strings.Split(address, ":")
	portstr := "port=" + parts[2]
	ipstr := "auth-ip-acl=" + parts[1]
	cmd := exec.Command("pactl", "load-module", "module-native-protocol-tcp", portstr, ipstr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load ACLs %s: %w", address, err)
	}
	return nil
}
