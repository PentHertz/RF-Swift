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

	// Fallback: parse the running qemu process list for a -qmp argument. Done
	// WITHOUT a shell (no `bash -c` with the instance name interpolated) so a
	// crafted instance name cannot inject shell commands - `instance` is only
	// ever compared as data below, never executed.
	if psOut, psErr := exec.Command("ps", "-axww", "-o", "command").Output(); psErr == nil {
		for _, line := range strings.Split(string(psOut), "\n") {
			if !strings.Contains(line, "qemu") || !strings.Contains(line, instance) {
				continue
			}
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "-qmp" && i+1 < len(fields) {
					// e.g. "unix:/path/to/qmp.sock,server,nowait"
					val := strings.TrimPrefix(fields[i+1], "unix:")
					if idx := strings.IndexByte(val, ','); idx >= 0 {
						val = val[:idx]
					}
					if val != "" {
						return val, nil
					}
				}
			}
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

// IsValidUSBID reports whether s is a well-formed USB vendor/product ID: an
// optional "0x" prefix followed by 1-4 hexadecimal digits (USB IDs are 16-bit).
// This is a security control: vid/pid values are interpolated into a QEMU
// human-monitor "device_add" command, so anything outside this set (commas,
// spaces, quotes, extra device properties) must be rejected to prevent
// QMP/HMP argument injection.
//
//	in(1): string s the candidate USB ID
//	out: bool true if s is a valid 16-bit hex ID
func IsValidUSBID(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if len(s) < 1 || len(s) > 4 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// isSafeQMPDeviceID reports whether devID is a safe QEMU device identifier
// (letters, digits, dot, underscore, hyphen only). Used to guard the value
// interpolated into a "device_del" human-monitor command against injection.
func isSafeQMPDeviceID(devID string) bool {
	if devID == "" || len(devID) > 64 {
		return false
	}
	for _, r := range devID {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// AttachUSBToLima attaches a USB device to a Lima VM via QMP hot-plug.
//
//	in(1): string vendorID hex vendor ID (e.g., "0x1234")
//	in(2): string productID hex product ID (e.g., "0x5678")
//	in(3): string instance Lima instance name (default: "rfswift")
//	out: error
func AttachUSBToLima(vendorID, productID, instance string) error {
	// Validate before building the human-monitor command (injection guard).
	if !IsValidUSBID(vendorID) || !IsValidUSBID(productID) {
		return fmt.Errorf("invalid USB vendor/product ID %q:%q (expected hex like 0x1d50)", vendorID, productID)
	}

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

	// If no USB bus exists, add a qemu-xhci controller and retry
	if result != "" && strings.Contains(result, "No 'usb-bus' bus found") {
		addBusCmd := "device_add qemu-xhci,id=usb-bus"
		busResult, busErr := qmpHumanCommand(sockPath, addBusCmd)
		if busErr != nil {
			return fmt.Errorf("failed to add USB controller: %w (ensure Lima YAML sets video.display to a non-\"none\" value, e.g. \"vnc\")", busErr)
		}
		if busResult != "" && strings.Contains(strings.ToLower(busResult), "error") {
			return fmt.Errorf("failed to add USB controller: %s (ensure Lima YAML sets video.display to a non-\"none\" value, e.g. \"vnc\")", busResult)
		}

		// Retry the device attach now that the USB bus exists
		result, err = qmpHumanCommand(sockPath, hmpCmd)
		if err != nil {
			return fmt.Errorf("failed to attach USB device %s:%s after adding USB controller: %w", vendorID, productID, err)
		}
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
	if !IsValidUSBID(vendorID) || !IsValidUSBID(productID) {
		return fmt.Errorf("invalid USB vendor/product ID %q:%q (expected hex like 0x1d50)", vendorID, productID)
	}

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
	if !isSafeQMPDeviceID(devID) {
		return fmt.Errorf("invalid device ID %q (expected letters, digits, '.', '_', '-')", devID)
	}

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

// IsKrunkitInstalled checks if the krunkit backend (libkrun) is installed.
// krunkit is required by Lima's GPU-accelerated VM on Apple Silicon: it exposes
// the Apple GPU to Linux guests as a Vulkan device (Venus -> MoltenVK -> Metal).
//
//	out: bool true if the krunkit binary is found in PATH
func IsKrunkitInstalled() bool {
	_, err := exec.LookPath("krunkit")
	return err == nil
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

// StopLimaInstance stops a running Lima instance.
//
//	in(1): string instance name of the Lima instance to stop
//	out: error
func StopLimaInstance(instance string) error {
	cmd := exec.Command("limactl", "stop", instance)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop Lima instance '%s': %w", instance, err)
	}
	return nil
}

// DeleteLimaInstance deletes a Lima instance (must be stopped first).
//
//	in(1): string instance name of the Lima instance to delete
//	out: error
func DeleteLimaInstance(instance string) error {
	cmd := exec.Command("limactl", "delete", instance)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete Lima instance '%s': %w", instance, err)
	}
	return nil
}

// GetLimaInstanceConfigPath returns the path to the active Lima instance config.
//
//	in(1): string instance name of the Lima instance
//	out: string path to ~/.lima/<instance>/lima.yaml
func GetLimaInstanceConfigPath(instance string) string {
	home := os.Getenv("HOME")
	return filepath.Join(home, ".lima", instance, "lima.yaml")
}

// CopyFile copies src to dst, creating the destination directory if needed.
//
//	in(1): string src source file path
//	in(2): string dst destination file path
//	out: error
func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", dst, err)
	}
	return nil
}

// IsValidLimaSize reports whether s is an acceptable Lima memory/disk size such
// as "8GiB", "512MiB", "100GB", "2.5GiB", or a bare number of bytes.
//
//	in(1): string s the size string to validate
//	out: bool true if the value is well-formed
func IsValidLimaSize(s string) bool {
	// Reject surrounding/embedded whitespace rather than trimming it: the value
	// is written verbatim into the YAML, so it must be clean as-is.
	if s == "" || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	// Longest units first so "GiB" matches before "G".
	for _, u := range []string{"GiB", "MiB", "KiB", "TiB", "GB", "MB", "KB", "TB", "G", "M", "K", "T", "B"} {
		if strings.HasSuffix(s, u) {
			s = strings.TrimSuffix(s, u)
			break
		}
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	dotSeen := false
	for _, r := range s {
		if r == '.' {
			if dotSeen {
				return false
			}
			dotSeen = true
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// SetLimaResources rewrites the top-level cpus/memory/disk fields of a Lima YAML
// template in place, preserving comments and all other content. A zero cpus or
// empty memory/disk leaves that field untouched; a requested field missing from
// the template is appended. Returns the human-readable list of changes applied.
//
//	in(1): string path to the Lima YAML template
//	in(2): int cpus number of vCPUs (0 = leave unchanged)
//	in(3): string memory e.g. "16GiB" ("" = leave unchanged)
//	in(4): string disk e.g. "200GiB" ("" = leave unchanged)
//	out: ([]string, error) changes applied, and any error
func SetLimaResources(path string, cpus int, memory, disk string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read template %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	var changes []string
	setCPUs, setMem, setDisk := false, false, false

	// Only rewrite top-level keys (column 0) so nested cpus/memory/disk keys in
	// provisioning blocks are never touched.
	for i, line := range lines {
		switch {
		case cpus > 0 && !setCPUs && strings.HasPrefix(line, "cpus:"):
			lines[i] = fmt.Sprintf("cpus: %d", cpus)
			changes = append(changes, fmt.Sprintf("cpus -> %d", cpus))
			setCPUs = true
		case memory != "" && !setMem && strings.HasPrefix(line, "memory:"):
			lines[i] = fmt.Sprintf("memory: %q", memory)
			changes = append(changes, fmt.Sprintf("memory -> %s", memory))
			setMem = true
		case disk != "" && !setDisk && strings.HasPrefix(line, "disk:"):
			lines[i] = fmt.Sprintf("disk: %q", disk)
			changes = append(changes, fmt.Sprintf("disk -> %s", disk))
			setDisk = true
		}
	}

	// Append any requested key that was not present in the template.
	if cpus > 0 && !setCPUs {
		lines = append(lines, fmt.Sprintf("cpus: %d", cpus))
		changes = append(changes, fmt.Sprintf("cpus -> %d (added)", cpus))
	}
	if memory != "" && !setMem {
		lines = append(lines, fmt.Sprintf("memory: %q", memory))
		changes = append(changes, fmt.Sprintf("memory -> %s (added)", memory))
	}
	if disk != "" && !setDisk {
		lines = append(lines, fmt.Sprintf("disk: %q", disk))
		changes = append(changes, fmt.Sprintf("disk -> %s (added)", disk))
	}

	if len(changes) == 0 {
		return nil, nil
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return nil, fmt.Errorf("failed to write template %s: %w", path, err)
	}
	return changes, nil
}
