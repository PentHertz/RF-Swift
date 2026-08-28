/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Windows USB passthrough through usbipd-win.
*
*  Containers on Windows run inside the WSL 2 virtual machine (Docker Desktop,
*  Podman machine), which has no direct view of the host USB bus. usbipd-win
*  forwards a host device into that VM, and every WSL 2 distribution shares the
*  same kernel, so a forwarded device shows up under /dev/bus/usb for the
*  container engine as well. The workflow, and who may run each step:
*
*    usbipd bind   --busid <id>        one-time per device  -> administrator
*    usbipd attach --wsl --busid <id>  after every plug     -> any user
*    usbipd detach --busid <id>        give it back         -> any user
*    usbipd unbind --busid <id>        stop sharing         -> administrator
*
*  Only "bind"/"unbind" need an elevated token (they swap the device driver for
*  the usbipd stub). RF Swift therefore keeps everything unprivileged and only
*  raises a UAC prompt - for usbipd.exe itself, never a shell - the first time a
*  device is shared. Listing uses the machine-readable "usbipd state" JSON and
*  falls back to parsing "usbipd list" for older releases.
 */

package rfutils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// USBDevice is a host USB device as reported by usbipd-win.
type USBDevice struct {
	BusID         string // e.g. "2-3"; empty when the device is shared (persisted) but unplugged
	DeviceID      string // Windows device instance ID, e.g. USB\VID_2CF0&PID_5250\<serial>
	VendorID      string // 4 lowercase hex digits, no 0x prefix
	ProductID     string // 4 lowercase hex digits, no 0x prefix
	Description   string // Windows driver description (often generic: "Bulk-In, Interface")
	Name          string // recognisable name from the built-in USB ID table, else Description
	Shared        bool   // bound with usbipd: "attach" works without administrator rights
	Attached      bool   // currently forwarded into WSL 2
	Forced        bool   // bound with --force (the host cannot use it while shared)
	Connected     bool   // physically present (BusID known)
	ClientIP      string // client address while attached
	PersistedGUID string // usbipd registration GUID while shared
}

// State summarises the usbipd view of the device for display purposes.
//
//	out: string one of "attached", "shared", "not shared", "unplugged"
func (d USBDevice) State() string {
	switch {
	case d.Attached:
		return "attached"
	case !d.Connected:
		return "unplugged"
	case d.Shared:
		return "shared"
	default:
		return "not shared"
	}
}

// HardwareID returns the usbipd "--hardware-id" form of the device (vid:pid).
func (d USBDevice) HardwareID() string {
	return d.VendorID + ":" + d.ProductID
}

// USBAttachResult reports what EnsureUSBDeviceAttached had to do.
type USBAttachResult struct {
	Device   USBDevice
	Bound    bool // the device was not shared yet and had to be bound first
	Elevated bool // the bind went through a UAC prompt
	Already  bool // the device was already attached; nothing was changed
}

// WSLDistro is one entry of "wsl --list --verbose".
type WSLDistro struct {
	Name    string
	State   string
	Version int
	Default bool
}

// WSLStatus describes the WSL installation usbipd will attach devices to.
type WSLStatus struct {
	Installed     bool
	DefaultDistro string
	Distros       []WSLDistro
}

// Sentinel errors so callers (CLI, Workbench) can decide whether to elevate,
// ask the user, or just print the message.
var (
	ErrUsbipdNotInstalled    = errors.New("usbipd-win is not installed (winget install usbipd, or https://github.com/dorssel/usbipd-win/releases)")
	ErrUSBAdminRequired      = errors.New("this usbipd operation requires administrator privileges")
	ErrUSBNotShared          = errors.New("device is not shared yet: it must be bound once with administrator privileges")
	ErrUSBNotFound           = errors.New("no USB device with this bus ID")
	ErrUSBNotConnected       = errors.New("device is shared but not plugged in")
	ErrUSBElevationDeclined  = errors.New("administrator approval was declined")
	ErrUSBElevationNotWindow = errors.New("privilege elevation is only available on Windows")
)

var (
	busIDPattern      = regexp.MustCompile(`^[0-9]{1,3}-[0-9]{1,3}(?:\.[0-9]{1,3})*$`)
	guidPattern       = regexp.MustCompile(`^[0-9A-Fa-f]{8}(?:-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12}$`)
	instanceIDPattern = regexp.MustCompile(`(?i)VID_([0-9A-F]{4})&PID_([0-9A-F]{4})`)
	// "2-3    8087:0032  Intel(R) Wireless Bluetooth(R)     Shared"
	listConnectedLine = regexp.MustCompile(`^(\S+)\s+([0-9A-Fa-f]{4}):([0-9A-Fa-f]{4})\s+(.*\S)\s+(Not shared|Shared \(forced\)|Shared|Attached)\s*$`)
	// "062b67f3-3b5f-48ac-a885-b3a72b311549  bladeRF 2.0"
	listPersistedLine = regexp.MustCompile(`^([0-9A-Fa-f]{8}(?:-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12})\s+(.*\S)\s*$`)
	wslDistroLine     = regexp.MustCompile(`^(\*?)\s*(.+?)\s{2,}(\S+)\s+([12])\s*$`)
)

// IsValidBusID reports whether s is exactly a usbipd bus ID ("1-2", "2-3.1").
// It is a security control: the value ends up on a command line that may run
// elevated, so anything else - including surrounding whitespace - is rejected
// before it reaches usbipd. User input is normalised with normalizeBusID.
//
//	in(1): string s the candidate bus ID
//	out: bool
func IsValidBusID(s string) bool {
	return busIDPattern.MatchString(s)
}

// IsValidUsbipdGUID reports whether s is exactly the registration GUID usbipd
// shows for shared-but-unplugged devices (same command-line safety role as
// IsValidBusID).
//
//	in(1): string s the candidate GUID
//	out: bool
func IsValidUsbipdGUID(s string) bool {
	return guidPattern.MatchString(s)
}

// normalizeBusID trims user input and validates it.
func normalizeBusID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !IsValidBusID(s) {
		return "", fmt.Errorf("invalid bus ID %q (expected the BUSID column of 'usbipd list', e.g. 2-3)", s)
	}
	return s, nil
}

// normalizeUsbipdGUID trims user input and validates it.
func normalizeUsbipdGUID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !IsValidUsbipdGUID(s) {
		return "", fmt.Errorf("invalid usbipd GUID %q", s)
	}
	return s, nil
}

// UsbipdPath locates usbipd.exe: PATH first, then the default install
// directory (a GUI started before a fresh PATH was propagated still works).
//
//	out(1): string absolute path to usbipd.exe
//	out(2): error ErrUsbipdNotInstalled when it cannot be found
func UsbipdPath() (string, error) {
	for _, name := range []string{"usbipd.exe", "usbipd"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, dir := range []string{os.Getenv("ProgramW6432"), os.Getenv("ProgramFiles"), `C:\Program Files`} {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, "usbipd-win", "usbipd.exe")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", ErrUsbipdNotInstalled
}

// IsUsbipdInstalled reports whether usbipd-win can be used on this host.
func IsUsbipdInstalled() bool {
	_, err := UsbipdPath()
	return err == nil
}

// UsbipdVersion returns the short usbipd-win version ("5.2.0").
//
//	out(1): string version
//	out(2): error
func UsbipdVersion() (string, error) {
	out, err := runUsbipd("--version")
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(out)
	if i := strings.IndexAny(v, "-+ "); i > 0 {
		v = v[:i]
	}
	return v, nil
}

// runUsbipd executes usbipd with the given arguments and classifies failures.
//
//	in(1): ...string args usbipd arguments
//	out(1): string stdout
//	out(2): error a sentinel-wrapped error when usbipd reported a known condition
func runUsbipd(args ...string) (string, error) {
	exe, err := UsbipdPath()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe, args...)
	hideConsoleWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return stdout.String(), classifyUsbipdError(stderr.String()+"\n"+stdout.String(), runErr)
	}
	return stdout.String(), nil
}

// classifyUsbipdError maps usbipd's diagnostics to sentinel errors while
// keeping the original message for the user.
//
//	in(1): string output combined stderr/stdout of the failed command
//	in(2): error runErr the exec error
//	out: error
func classifyUsbipdError(output string, runErr error) error {
	msg := usbipdMessage(output)
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "access denied") || strings.Contains(lower, "administrator privileges"):
		return fmt.Errorf("%w: %s", ErrUSBAdminRequired, msg)
	case strings.Contains(lower, "is not shared"):
		return fmt.Errorf("%w (%s)", ErrUSBNotShared, msg)
	case strings.Contains(lower, "no device with busid") || strings.Contains(lower, "there is no device"):
		return fmt.Errorf("%w: %s", ErrUSBNotFound, msg)
	case msg == "":
		return fmt.Errorf("usbipd failed: %w", runErr)
	default:
		return errors.New(msg)
	}
}

// usbipdMessage extracts the most relevant "usbipd: error: ..." line, falling
// back to the last non-empty line of output.
func usbipdMessage(output string) string {
	var last string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "usbipd: error:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "usbipd: error:"))
		}
		last = line
	}
	return strings.TrimSpace(strings.TrimPrefix(last, "usbipd: warning:"))
}

// ListUSBDevices lists host USB devices with their usbipd sharing state. It
// prefers the JSON "usbipd state" output and falls back to "usbipd list" text.
//
//	out(1): []USBDevice connected devices first (by bus ID), then shared-but-unplugged ones
//	out(2): error
func ListUSBDevices() ([]USBDevice, error) {
	if _, err := UsbipdPath(); err != nil {
		return nil, err
	}
	out, err := runUsbipd("state")
	if err == nil {
		devices, perr := ParseUsbipdState([]byte(out))
		if perr == nil {
			return devices, nil
		}
		err = perr
	}
	text, lerr := runUsbipd("list")
	if lerr != nil {
		return nil, fmt.Errorf("usbipd state failed (%v) and usbipd list failed: %w", err, lerr)
	}
	return ParseUsbipdList(text), nil
}

// usbipdState mirrors the "usbipd state" JSON document.
type usbipdState struct {
	Devices []struct {
		BusID           *string `json:"BusId"`
		ClientIPAddress *string `json:"ClientIPAddress"`
		Description     string  `json:"Description"`
		InstanceID      string  `json:"InstanceId"`
		IsForced        bool    `json:"IsForced"`
		PersistedGUID   *string `json:"PersistedGuid"`
	} `json:"Devices"`
}

// ParseUsbipdState decodes the "usbipd state" JSON output.
//
//	in(1): []byte data raw JSON
//	out(1): []USBDevice sorted like ListUSBDevices
//	out(2): error
func ParseUsbipdState(data []byte) ([]USBDevice, error) {
	var state usbipdState
	if err := json.Unmarshal(bytes.TrimSpace(data), &state); err != nil {
		return nil, fmt.Errorf("cannot parse usbipd state output: %w", err)
	}
	devices := make([]USBDevice, 0, len(state.Devices))
	for _, d := range state.Devices {
		dev := USBDevice{
			DeviceID:    d.InstanceID,
			Description: strings.TrimSpace(d.Description),
			Forced:      d.IsForced,
		}
		if d.BusID != nil {
			dev.BusID = strings.TrimSpace(*d.BusID)
			dev.Connected = dev.BusID != ""
		}
		if d.PersistedGUID != nil && *d.PersistedGUID != "" {
			dev.PersistedGUID = *d.PersistedGUID
			dev.Shared = true
		}
		if d.ClientIPAddress != nil && *d.ClientIPAddress != "" {
			dev.ClientIP = *d.ClientIPAddress
			dev.Attached = true
		}
		if m := instanceIDPattern.FindStringSubmatch(d.InstanceID); m != nil {
			dev.VendorID = strings.ToLower(m[1])
			dev.ProductID = strings.ToLower(m[2])
		}
		dev.Name = USBFriendlyName(dev.VendorID, dev.ProductID, dev.Description)
		devices = append(devices, dev)
	}
	sortUSBDevices(devices)
	return devices, nil
}

// ParseUsbipdList parses the human-readable "usbipd list" output (usbipd-win
// 4.x/5.x layout: BUSID, VID:PID, DEVICE, STATE, then a "Persisted:" section).
//
//	in(1): string text raw output
//	out: []USBDevice
func ParseUsbipdList(text string) []USBDevice {
	var devices []USBDevice
	section := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "Connected:"):
			section = "connected"
			continue
		case strings.HasPrefix(trimmed, "Persisted:"):
			section = "persisted"
			continue
		case strings.HasPrefix(trimmed, "BUSID") || strings.HasPrefix(trimmed, "GUID"):
			continue
		}
		switch section {
		case "connected":
			m := listConnectedLine.FindStringSubmatch(trimmed)
			if m == nil || !IsValidBusID(m[1]) {
				continue
			}
			dev := USBDevice{
				BusID:       m[1],
				VendorID:    strings.ToLower(m[2]),
				ProductID:   strings.ToLower(m[3]),
				Description: strings.TrimSpace(m[4]),
				Connected:   true,
			}
			switch m[5] {
			case "Attached":
				dev.Attached, dev.Shared = true, true
			case "Shared":
				dev.Shared = true
			case "Shared (forced)":
				dev.Shared, dev.Forced = true, true
			}
			dev.Name = USBFriendlyName(dev.VendorID, dev.ProductID, dev.Description)
			devices = append(devices, dev)
		case "persisted":
			m := listPersistedLine.FindStringSubmatch(trimmed)
			if m == nil {
				continue
			}
			desc := strings.TrimSpace(m[2])
			devices = append(devices, USBDevice{
				PersistedGUID: m[1],
				Description:   desc,
				Name:          desc,
				Shared:        true,
			})
		}
	}
	sortUSBDevices(devices)
	return devices
}

// sortUSBDevices orders connected devices by bus/port number, then the
// shared-but-unplugged devices by name.
func sortUSBDevices(devices []USBDevice) {
	sort.SliceStable(devices, func(i, j int) bool {
		a, b := devices[i], devices[j]
		if a.Connected != b.Connected {
			return a.Connected
		}
		if a.Connected {
			return busIDLess(a.BusID, b.BusID)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

// busIDLess compares bus IDs numerically ("2-3" < "2-10", "1-2" < "2-1").
func busIDLess(a, b string) bool {
	as := strings.FieldsFunc(a, func(r rune) bool { return r == '-' || r == '.' })
	bs := strings.FieldsFunc(b, func(r rune) bool { return r == '-' || r == '.' })
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, _ := strconv.Atoi(as[i])
		bi, _ := strconv.Atoi(bs[i])
		if ai != bi {
			return ai < bi
		}
	}
	return len(as) < len(bs)
}

// FindUSBDevice returns the device with the given bus ID.
//
//	in(1): string busID
//	out(1): USBDevice
//	out(2): error ErrUSBNotFound when absent
func FindUSBDevice(busID string) (USBDevice, error) {
	busID, err := normalizeBusID(busID)
	if err != nil {
		return USBDevice{}, err
	}
	devices, err := ListUSBDevices()
	if err != nil {
		return USBDevice{}, err
	}
	for _, d := range devices {
		if d.BusID == busID {
			return d, nil
		}
	}
	return USBDevice{}, fmt.Errorf("%w: %s", ErrUSBNotFound, busID)
}

// AttachUSBDevice forwards a shared device into WSL 2 (no elevation needed).
//
//	in(1): string busID
//	out: error ErrUSBNotShared when the device was never bound
func AttachUSBDevice(busID string) error {
	busID, err := normalizeBusID(busID)
	if err != nil {
		return err
	}
	if _, err := runUsbipd("attach", "--wsl", "--busid", busID); err != nil {
		return fmt.Errorf("failed to attach device %s: %w", busID, err)
	}
	return nil
}

// DetachUSBDevice returns a forwarded device to Windows (no elevation needed).
//
//	in(1): string busID
//	out: error
func DetachUSBDevice(busID string) error {
	busID, err := normalizeBusID(busID)
	if err != nil {
		return err
	}
	if _, err := runUsbipd("detach", "--busid", busID); err != nil {
		return fmt.Errorf("failed to detach device %s: %w", busID, err)
	}
	return nil
}

// BindUSBDevice shares a device with the current token. It succeeds directly
// in an elevated session and returns ErrUSBAdminRequired otherwise.
//
//	in(1): string busID
//	out: error
func BindUSBDevice(busID string) error {
	busID, err := normalizeBusID(busID)
	if err != nil {
		return err
	}
	if _, err := runUsbipd("bind", "--busid", busID); err != nil {
		return fmt.Errorf("failed to bind device %s: %w", busID, err)
	}
	return nil
}

// UnbindUSBDevice stops sharing a device with the current token (see
// BindUSBDevice for the privilege behaviour).
//
//	in(1): string busID
//	out: error
func UnbindUSBDevice(busID string) error {
	busID, err := normalizeBusID(busID)
	if err != nil {
		return err
	}
	if _, err := runUsbipd("unbind", "--busid", busID); err != nil {
		return fmt.Errorf("failed to unbind device %s: %w", busID, err)
	}
	return nil
}

// UnbindUSBDeviceGUID stops sharing a persisted (currently unplugged) device
// by its usbipd registration GUID, with the current token.
//
//	in(1): string guid
//	out: error
func UnbindUSBDeviceGUID(guid string) error {
	guid, err := normalizeUsbipdGUID(guid)
	if err != nil {
		return err
	}
	if _, err := runUsbipd("unbind", "--guid", guid); err != nil {
		return fmt.Errorf("failed to unbind persisted device %s: %w", guid, err)
	}
	return nil
}

// BindUSBDeviceElevated shares a device through a Windows UAC prompt. The
// prompt is raised for usbipd.exe itself with exactly "bind --busid <id>", so
// the operator can verify what receives administrator rights; no shell is
// involved and the bus ID has been validated.
//
//	in(1): string busID
//	out: error ErrUSBElevationDeclined when the prompt is dismissed
func BindUSBDeviceElevated(busID string) error {
	busID, err := normalizeBusID(busID)
	if err != nil {
		return err
	}
	return runUsbipdElevated("bind", "--busid", busID)
}

// UnbindUSBDeviceElevated stops sharing a device through a UAC prompt.
//
//	in(1): string busID
//	out: error
func UnbindUSBDeviceElevated(busID string) error {
	busID, err := normalizeBusID(busID)
	if err != nil {
		return err
	}
	return runUsbipdElevated("unbind", "--busid", busID)
}

// UnbindUSBDeviceGUIDElevated stops sharing a persisted device through a UAC
// prompt.
//
//	in(1): string guid
//	out: error
func UnbindUSBDeviceGUIDElevated(guid string) error {
	guid, err := normalizeUsbipdGUID(guid)
	if err != nil {
		return err
	}
	return runUsbipdElevated("unbind", "--guid", guid)
}

// runUsbipdElevated runs "usbipd <verb> <selector> <value>" with
// administrator rights via UAC. Callers validate value first.
func runUsbipdElevated(verb, selector, value string) error {
	exe, err := UsbipdPath()
	if err != nil {
		return err
	}
	code, err := runElevated(exe, []string{verb, selector, value})
	if err != nil {
		return fmt.Errorf("usbipd %s %s %s: %w", verb, selector, value, err)
	}
	if code != 0 {
		return fmt.Errorf("usbipd %s %s %s failed with exit code %d (run it in an administrator terminal for details)", verb, selector, value, code)
	}
	return nil
}

// ShareUSBDevice makes sure a device is bound: directly when the process is
// already elevated, otherwise through a UAC prompt when allowElevation is set.
//
//	in(1): string busID
//	in(2): bool allowElevation permit a UAC prompt
//	out(1): bool true when a UAC prompt was used
//	out(2): error ErrUSBAdminRequired when elevation was needed but not allowed
func ShareUSBDevice(busID string, allowElevation bool) (bool, error) {
	busID, err := normalizeBusID(busID)
	if err != nil {
		return false, err
	}
	err = BindUSBDevice(busID)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, ErrUSBAdminRequired) {
		return false, err
	}
	if !allowElevation {
		return false, err
	}
	if err := BindUSBDeviceElevated(busID); err != nil {
		return true, err
	}
	dev, err := FindUSBDevice(busID)
	if err != nil {
		return true, err
	}
	if !dev.Shared {
		return true, fmt.Errorf("usbipd reported success but device %s is still not shared", busID)
	}
	return true, nil
}

// UnshareUSBDevice is the counterpart of ShareUSBDevice. ref is a bus ID for
// a connected device or the registration GUID of a shared-but-unplugged one
// (usbipd detaches an attached device itself before unbinding it).
//
//	in(1): string ref bus ID or usbipd GUID
//	in(2): bool allowElevation permit a UAC prompt
//	out(1): bool true when a UAC prompt was used
//	out(2): error
func UnshareUSBDevice(ref string, allowElevation bool) (bool, error) {
	ref = strings.TrimSpace(ref)
	var direct func(string) error
	var elevated func(string) error
	switch {
	case IsValidBusID(ref):
		direct, elevated = UnbindUSBDevice, UnbindUSBDeviceElevated
	case IsValidUsbipdGUID(ref):
		direct, elevated = UnbindUSBDeviceGUID, UnbindUSBDeviceGUIDElevated
	default:
		return false, fmt.Errorf("invalid device reference %q (expected a bus ID like 2-3 or a usbipd GUID)", ref)
	}
	err := direct(ref)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, ErrUSBAdminRequired) || !allowElevation {
		return false, err
	}
	if err := elevated(ref); err != nil {
		return true, err
	}
	return true, nil
}

// EnsureUSBDeviceAttached is the one-step path used by the CLI wizard and the
// Workbench: share the device if needed (UAC prompt, once per device), then
// attach it to WSL 2 and confirm the new state.
//
//	in(1): string busID
//	in(2): bool allowElevation permit a UAC prompt for a never-shared device
//	out(1): USBAttachResult what happened and the final device state
//	out(2): error ErrUSBNotShared when the device needs sharing and elevation is not allowed
func EnsureUSBDeviceAttached(busID string, allowElevation bool) (USBAttachResult, error) {
	var result USBAttachResult
	busID, err := normalizeBusID(busID)
	if err != nil {
		return result, err
	}
	dev, err := FindUSBDevice(busID)
	if err != nil {
		return result, err
	}
	result.Device = dev
	if !dev.Connected {
		return result, fmt.Errorf("%w: %s", ErrUSBNotConnected, dev.Name)
	}
	if dev.Attached {
		result.Already = true
		return result, nil
	}
	if !dev.Shared {
		if !allowElevation {
			return result, fmt.Errorf("%w (%s, bus %s)", ErrUSBNotShared, dev.Name, busID)
		}
		elevated, err := ShareUSBDevice(busID, true)
		result.Bound, result.Elevated = true, elevated
		if err != nil {
			return result, err
		}
	}
	if err := AttachUSBDevice(busID); err != nil {
		return result, err
	}
	if dev, err := FindUSBDevice(busID); err == nil {
		result.Device = dev
		if !dev.Attached {
			return result, fmt.Errorf("usbipd attach returned without error but %s is not attached; check 'wsl --list --verbose' shows a running WSL 2 distribution", dev.Name)
		}
	}
	return result, nil
}

// WSLDistributions reports the WSL distributions usbipd can attach to. WSL
// prints UTF-16 on most hosts, so the output is decoded before parsing.
//
//	out(1): WSLStatus
//	out(2): error when wsl.exe is missing or fails
func WSLDistributions() (WSLStatus, error) {
	var status WSLStatus
	wsl, err := exec.LookPath("wsl.exe")
	if err != nil {
		return status, fmt.Errorf("wsl.exe not found: WSL 2 is required for USB passthrough (wsl --install)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, wsl, "--list", "--verbose")
	hideConsoleWindow(cmd)
	raw, err := cmd.Output()
	text := DecodeConsoleOutput(raw)
	if err != nil {
		if strings.TrimSpace(text) == "" {
			return status, fmt.Errorf("wsl --list --verbose failed: %w", err)
		}
		// wsl.exe exits non-zero when no distribution is installed, but still
		// prints an explanation; surface it.
		return status, errors.New(strings.TrimSpace(text))
	}
	status.Installed = true
	status.Distros = ParseWSLList(text)
	for _, d := range status.Distros {
		if d.Default {
			status.DefaultDistro = d.Name
		}
	}
	return status, nil
}

// ParseWSLList parses decoded "wsl --list --verbose" output.
//
//	in(1): string text decoded output
//	out: []WSLDistro
func ParseWSLList(text string) []WSLDistro {
	var out []WSLDistro
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := wslDistroLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := strings.TrimSpace(m[2])
		if strings.EqualFold(name, "NAME") {
			continue
		}
		version, _ := strconv.Atoi(m[4])
		out = append(out, WSLDistro{Name: name, State: m[3], Version: version, Default: m[1] == "*"})
	}
	return out
}

// HasWSL2Distribution reports whether at least one WSL 2 distribution exists,
// which "usbipd attach --wsl" needs.
func (s WSLStatus) HasWSL2Distribution() bool {
	for _, d := range s.Distros {
		if d.Version == 2 {
			return true
		}
	}
	return false
}

// WSLUSBView lists what the WSL 2 kernel currently exposes under
// /dev/bus/usb (and lsusb when the default distribution ships it). Best
// effort: it starts the default distribution if needed and gives up after a
// short timeout.
//
//	out(1): string human-readable listing ("" when nothing is attached)
//	out(2): error
func WSLUSBView() (string, error) {
	wsl, err := exec.LookPath("wsl.exe")
	if err != nil {
		return "", fmt.Errorf("wsl.exe not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	script := "ls -1 /dev/bus/usb/*/* 2>/dev/null; if command -v lsusb >/dev/null 2>&1; then lsusb 2>/dev/null; fi"
	cmd := exec.CommandContext(ctx, wsl, "-e", "sh", "-c", script)
	hideConsoleWindow(cmd)
	raw, err := cmd.CombinedOutput()
	text := strings.TrimSpace(DecodeConsoleOutput(raw))
	if err != nil && text == "" {
		return "", fmt.Errorf("could not query WSL: %w", err)
	}
	return text, nil
}

// DecodeConsoleOutput turns wsl.exe output into a Go string. wsl.exe writes
// UTF-16LE (with or without BOM) when stdout is redirected; other tools write
// UTF-8. The heuristic keys on NUL bytes, which never appear in UTF-8 text.
//
//	in(1): []byte raw process output
//	out: string decoded text
func DecodeConsoleOutput(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		raw = raw[2:]
	} else if !bytes.Contains(raw, []byte{0}) {
		return string(raw)
	}
	if len(raw)%2 == 1 {
		raw = raw[:len(raw)-1]
	}
	u16 := make([]uint16, len(raw)/2)
	for i := range u16 {
		u16[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	return string(utf16.Decode(u16))
}
