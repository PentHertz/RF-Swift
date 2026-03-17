/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*  macOS USB passthrough via Lima QEMU QMP
 */

package rfutils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	common "penthertz/rfswift/common"
)

// MacUSBDevice represents a USB device discovered on macOS
type MacUSBDevice struct {
	Name      string
	VendorID  string
	ProductID string
	Serial    string
	Location  string
}

// ListMacUSBDevices discovers USB devices on macOS using system_profiler.
//
//	out(1): []MacUSBDevice array of discovered USB devices
//	out(2): error
func ListMacUSBDevices() ([]MacUSBDevice, error) {
	cmd := exec.Command("system_profiler", "SPUSBDataType", "-json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute system_profiler: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse system_profiler output: %w", err)
	}

	var devices []MacUSBDevice
	if spUSB, ok := result["SPUSBDataType"]; ok {
		if items, ok := spUSB.([]interface{}); ok {
			for _, item := range items {
				extractUSBDevices(item, &devices)
			}
		}
	}

	return devices, nil
}

// extractUSBDevices recursively walks the system_profiler JSON tree to find
// USB devices with vendor_id and product_id fields.
func extractUSBDevices(item interface{}, devices *[]MacUSBDevice) {
	m, ok := item.(map[string]interface{})
	if !ok {
		return
	}

	// If this node has vendor_id and product_id, it's a device
	vendorID, hasVendor := m["vendor_id"].(string)
	productID, hasProduct := m["product_id"].(string)
	if hasVendor && hasProduct {
		dev := MacUSBDevice{
			VendorID:  cleanHexID(vendorID),
			ProductID: cleanHexID(productID),
		}
		if name, ok := m["_name"].(string); ok {
			dev.Name = name
		}
		if serial, ok := m["serial_num"].(string); ok {
			dev.Serial = serial
		}
		if loc, ok := m["location_id"].(string); ok {
			dev.Location = loc
		}
		*devices = append(*devices, dev)
	}

	// Recurse into _items (child hubs/devices)
	if items, ok := m["_items"].([]interface{}); ok {
		for _, child := range items {
			extractUSBDevices(child, devices)
		}
	}
}

// cleanHexID extracts the hex value from strings like "0x1234  (Some Corp)"
func cleanHexID(raw string) string {
	raw = strings.TrimSpace(raw)
	// system_profiler may output "0x1234" or "0x1234  (Vendor Name)"
	if idx := strings.Index(raw, " "); idx > 0 {
		return raw[:idx]
	}
	return raw
}

// --- Lima QMP integration ---

// FindLimaQMPSocket locates the QMP socket for a Lima instance.
// It searches in ~/.lima/<instance>/qmp.sock
//
//	in(1): string instance name of the Lima instance (default: "rfswift")
//	out(1): string path to the QMP socket
//	out(2): error
func FindLimaQMPSocket(instance string) (string, error) {
	if instance == "" {
		instance = "rfswift"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	// Check Lima's standard socket location
	qmpSock := filepath.Join(home, ".lima", instance, "qmp.sock")
	if _, err := os.Stat(qmpSock); err == nil {
		return qmpSock, nil
	}

	// Also check for a serial monitor socket used by some Lima versions
	serialSock := filepath.Join(home, ".lima", instance, "serial.sock")
	if _, err := os.Stat(serialSock); err == nil {
		return serialSock, nil
	}

	// Try to find via qemu process
	cmd := exec.Command("bash", "-c", fmt.Sprintf("ps aux | grep qemu | grep %s | grep -oE '\\-qmp [^ ]+' | awk '{print $2}'", instance))
	output, err := cmd.Output()
	if err == nil {
		sock := strings.TrimSpace(string(output))
		if sock != "" {
			return sock, nil
		}
	}

	return "", fmt.Errorf("QMP socket not found for Lima instance '%s'. Is the VM running with vmType: qemu?", instance)
}

// qmpCommand sends a command to QEMU via QMP protocol and returns the response.
func qmpCommand(sockPath string, command map[string]interface{}) (map[string]interface{}, error) {
	conn, err := net.DialTimeout("unix", sockPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to QMP socket %s: %w", sockPath, err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Read the QMP greeting
	_, err = reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read QMP greeting: %w", err)
	}

	// Send qmp_capabilities to enter command mode
	capsCmd := map[string]interface{}{"execute": "qmp_capabilities"}
	capsJSON, _ := json.Marshal(capsCmd)
	conn.Write(append(capsJSON, '\n'))

	// Read capabilities response
	_, err = reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read capabilities response: %w", err)
	}

	// Send the actual command
	cmdJSON, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}
	conn.Write(append(cmdJSON, '\n'))

	// Read response
	respBytes, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read command response: %w", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(respBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if errObj, ok := response["error"]; ok {
		return nil, fmt.Errorf("QMP error: %v", errObj)
	}

	return response, nil
}

// qmpHumanCommand sends a human-monitor-command (HMP passthrough) via QMP.
func qmpHumanCommand(sockPath string, hmpCmd string) (string, error) {
	cmd := map[string]interface{}{
		"execute": "human-monitor-command",
		"arguments": map[string]interface{}{
			"command-line": hmpCmd,
		},
	}

	resp, err := qmpCommand(sockPath, cmd)
	if err != nil {
		return "", err
	}

	if ret, ok := resp["return"].(string); ok {
		return ret, nil
	}

	return "", nil
}

// AttachUSBToLima attaches a USB device to a Lima VM via QMP hot-plug.
//
//	in(1): string vendorID hex vendor ID (e.g., "0x1234")
//	in(2): string productID hex product ID (e.g., "0x5678")
//	in(3): string instance Lima instance name (default: "rfswift")
//	out: error
func AttachUSBToLima(vendorID, productID, instance string) error {
	sockPath, err := FindLimaQMPSocket(instance)
	if err != nil {
		return err
	}

	// Generate a device ID from vendor:product for later removal
	devID := fmt.Sprintf("usb-%s-%s", strings.TrimPrefix(vendorID, "0x"), strings.TrimPrefix(productID, "0x"))

	hmpCmd := fmt.Sprintf("device_add usb-host,vendorid=%s,productid=%s,id=%s", vendorID, productID, devID)
	result, err := qmpHumanCommand(sockPath, hmpCmd)
	if err != nil {
		return fmt.Errorf("failed to attach USB device %s:%s: %w", vendorID, productID, err)
	}

	if result != "" && strings.Contains(strings.ToLower(result), "error") {
		return fmt.Errorf("QMP device_add failed: %s", result)
	}

	common.PrintSuccessMessage(fmt.Sprintf("USB device %s:%s attached as '%s'", vendorID, productID, devID))
	return nil
}

// DetachUSBFromLima detaches a USB device from a Lima VM via QMP hot-unplug.
//
//	in(1): string vendorID hex vendor ID (e.g., "0x1234")
//	in(2): string productID hex product ID (e.g., "0x5678")
//	in(3): string instance Lima instance name (default: "rfswift")
//	out: error
func DetachUSBFromLima(vendorID, productID, instance string) error {
	sockPath, err := FindLimaQMPSocket(instance)
	if err != nil {
		return err
	}

	devID := fmt.Sprintf("usb-%s-%s", strings.TrimPrefix(vendorID, "0x"), strings.TrimPrefix(productID, "0x"))

	hmpCmd := fmt.Sprintf("device_del %s", devID)
	result, err := qmpHumanCommand(sockPath, hmpCmd)
	if err != nil {
		return fmt.Errorf("failed to detach USB device %s: %w", devID, err)
	}

	if result != "" && strings.Contains(strings.ToLower(result), "error") {
		return fmt.Errorf("QMP device_del failed: %s", result)
	}

	common.PrintSuccessMessage(fmt.Sprintf("USB device '%s' detached", devID))
	return nil
}

// DetachUSBByIDFromLima detaches a USB device using its QMP device ID.
//
//	in(1): string devID the QMP device ID (e.g., "usb-1234-5678")
//	in(2): string instance Lima instance name (default: "rfswift")
//	out: error
func DetachUSBByIDFromLima(devID, instance string) error {
	sockPath, err := FindLimaQMPSocket(instance)
	if err != nil {
		return err
	}

	hmpCmd := fmt.Sprintf("device_del %s", devID)
	result, err := qmpHumanCommand(sockPath, hmpCmd)
	if err != nil {
		return fmt.Errorf("failed to detach USB device %s: %w", devID, err)
	}

	if result != "" && strings.Contains(strings.ToLower(result), "error") {
		return fmt.Errorf("QMP device_del failed: %s", result)
	}

	common.PrintSuccessMessage(fmt.Sprintf("USB device '%s' detached", devID))
	return nil
}

// ListUSBInLimaVM lists USB devices visible inside the Lima VM.
//
//	in(1): string instance Lima instance name (default: "rfswift")
//	out(1): string the output from 'info usb' QMP command
//	out(2): error
func ListUSBInLimaVM(instance string) (string, error) {
	sockPath, err := FindLimaQMPSocket(instance)
	if err != nil {
		return "", err
	}

	result, err := qmpHumanCommand(sockPath, "info usb")
	if err != nil {
		return "", fmt.Errorf("failed to list USB devices in VM: %w", err)
	}

	return result, nil
}

// IsLimaInstalled checks if Lima is installed and available.
//
//	out: bool true if limactl is found in PATH
func IsLimaInstalled() bool {
	_, err := exec.LookPath("limactl")
	return err == nil
}

// IsQEMUInstalled checks if QEMU is installed (required by Lima with vmType: qemu).
//
//	out: bool true if qemu-system-* is found in PATH
func IsQEMUInstalled() bool {
	// Check for the architecture-specific binary
	for _, bin := range []string{"qemu-system-aarch64", "qemu-system-x86_64", "qemu-img"} {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// IsLimaInstanceRunning checks if a specific Lima instance is running.
//
//	in(1): string instance the Lima instance name
//	out: bool true if the instance is running
func IsLimaInstanceRunning(instance string) bool {
	cmd := exec.Command("limactl", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Parse each line as a JSON object (limactl outputs JSONL)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		var info map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &info); err != nil {
			continue
		}
		if name, ok := info["name"].(string); ok && name == instance {
			if status, ok := info["status"].(string); ok {
				return status == "Running"
			}
		}
	}

	return false
}

// StartLimaInstance starts a Lima instance.
//
//	in(1): string instance name of the Lima instance to start
//	out: error
func StartLimaInstance(instance string) error {
	cmd := exec.Command("limactl", "start", instance)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CreateLimaInstance creates and starts a Lima instance from a YAML template.
//
//	in(1): string yamlPath path to the Lima YAML template
//	in(2): string instance name for the new instance
//	out: error
func CreateLimaInstance(yamlPath, instance string) error {
	cmd := exec.Command("limactl", "create", "--name", instance, yamlPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Lima instance: %w", err)
	}

	return StartLimaInstance(instance)
}
