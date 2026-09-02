package workbench

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
	rfutils "penthertz/rfswift/rfutils"
)

// USBAccessCheck tells the mission form whether a container configuration
// will be able to reach forwarded USB devices: /dev/bus/usb must be mapped and
// USB device major 189 allowed ("c 189:* rwm"). Privileged mode is not
// required, and the result says so. Same logic as the CLI's pre-creation
// notice (common.CheckUSBAccess).
func (a *App) USBAccessCheck(devices, bindings, cgroupRules []string, privileged bool) common.USBAccess {
	return common.CheckUSBAccess(devices, bindings, cgroupRules, privileged)
}

// DeviceMappingCheck tells the mission form which device mappings and /dev
// bind mounts the selected engine cannot use on this machine (root-only or
// inaccessible nodes under rootless Podman, no passthrough into the Docker
// Desktop/OrbStack/Podman machine VM on macOS, devices absent from the Lima
// VM). Advisory only: the form offers to remove them. Remote agents are not
// checked (scope "none").
func (a *App) DeviceMappingCheck(engine string, devices, bindings []string) rfdock.DeviceCheck {
	if _, remote := a.eng.(*RemoteEngine); remote || engine == "nix" {
		return rfdock.DeviceCheck{Engine: engine, Scope: "none", Issues: []rfdock.DeviceIssue{}}
	}
	return rfdock.CheckDeviceMappings(engine, devices, bindings)
}

// USBDevice is a host USB device the Workbench can forward into the VM that
// runs the containers. Two backends exist, selected by USBBackend:
//
//   - lima (macOS): Docker Desktop/OrbStack and Podman cannot forward USB, so
//     the device is hot-plugged into the Lima QEMU VM over QMP (the same path
//     as `rfswift macusb`).
//   - usbipd (Windows): containers run inside the WSL 2 VM; usbipd-win forwards
//     the device into it (`rfswift usb attach`). Sharing a device for the first
//     time needs one administrator approval (a UAC prompt for usbipd.exe);
//     attaching and detaching never do.
type USBDevice struct {
	Name        string `json:"name"`
	VendorID    string `json:"vendorId"` // macOS: 0x-prefixed hex; Windows: usbipd style "1d50"
	ProductID   string `json:"productId"`
	Serial      string `json:"serial"`
	Attached    bool   `json:"attached"`              // forwarded into the VM (Lima or WSL 2)
	BusID       string `json:"busId,omitempty"`       // Windows: usbipd bus ID ("2-3"); empty while unplugged
	GUID        string `json:"guid,omitempty"`        // Windows: usbipd registration GUID while shared
	Description string `json:"description,omitempty"` // Windows driver description when it differs from Name
	State       string `json:"state"`                 // attached | shared | host | unplugged
	Shared      bool   `json:"shared"`                // Windows: bound with usbipd, attach needs no elevation
	Connected   bool   `json:"connected"`
	Warning     string `json:"warning,omitempty"` // e.g. keyboard/mouse: Windows loses it while attached
}

// USBHostInfo tells the GUI which backend is active and what it found.
type USBHostInfo struct {
	Backend   string   `json:"backend"` // lima | usbipd | ""
	Version   string   `json:"version,omitempty"`
	WSLDistro string   `json:"wslDistro,omitempty"`
	Notes     []string `json:"notes,omitempty"`
}

// WinUSBResult reports what AttachWinUSB had to do.
type WinUSBResult struct {
	Device   USBDevice `json:"device"`
	Bound    bool      `json:"bound"`    // the device was shared first (one-time)
	Elevated bool      `json:"elevated"` // sharing went through a UAC prompt
	Already  bool      `json:"already"`  // it was already attached
}

// usbDevID mirrors the QMP device id AttachUSBToLima assigns (usb-<vid>-<pid>
// without the 0x prefixes), so we can match forwarded devices in `info usb`.
func usbDevID(vendorID, productID string) string {
	return "usb-" + strings.TrimPrefix(strings.ToLower(vendorID), "0x") + "-" + strings.TrimPrefix(strings.ToLower(productID), "0x")
}

// usbBackend picks the passthrough backend for this host: "lima" on macOS
// with Lima installed, "usbipd" on Windows with usbipd-win installed, "" when
// USB passthrough does not apply (Linux passes devices at creation; remote
// engines are not driven from this process).
func (a *App) usbBackend() string {
	if _, ok := a.eng.(*LocalEngine); !ok {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		if rfutils.IsLimaInstalled() {
			return "lima"
		}
	case "windows":
		if rfutils.IsUsbipdInstalled() {
			return "usbipd"
		}
	}
	return ""
}

func (a *App) usbSupported() bool { return a.usbBackend() != "" }

// USBSupported tells the GUI whether to surface USB passthrough controls at all.
func (a *App) USBSupported() bool { return a.usbSupported() }

// USBBackend tells the GUI which passthrough mechanism is active so it can
// word the dialog accordingly ("lima", "usbipd" or "").
func (a *App) USBBackend() string { return a.usbBackend() }

// USBHostInfo summarises the backend state for the dialog header.
func (a *App) USBHostInfo() (USBHostInfo, error) {
	info := USBHostInfo{Backend: a.usbBackend()}
	switch info.Backend {
	case "usbipd":
		if v, err := rfutils.UsbipdVersion(); err == nil {
			info.Version = v
			info.Notes = append(info.Notes, "usbipd-win "+v)
		}
		if wsl, err := rfutils.WSLDistributions(); err != nil {
			info.Notes = append(info.Notes, "WSL: "+err.Error())
		} else if !wsl.HasWSL2Distribution() {
			info.Notes = append(info.Notes, "No WSL 2 distribution: install one (wsl --install -d Ubuntu) before attaching")
		} else {
			info.WSLDistro = wsl.DefaultDistro
			if info.WSLDistro == "" {
				info.Notes = append(info.Notes, "WSL 2: no default distribution (wsl --set-default <name>)")
			} else {
				info.Notes = append(info.Notes, "WSL 2 default distribution: "+info.WSLDistro)
			}
		}
		// Every WSL 2 distribution shares the VM's kernel, so a forwarded device
		// is visible to the Nix engine's distribution as well.
		if nixBackend, err := rfnix.WSLBackend(); err == nil && nixBackend.Ready() {
			info.Notes = append(info.Notes, fmt.Sprintf("Nix environments (%s) see forwarded devices under /dev/bus/usb; \"Install device rules\" on a Nix mission grants non-root access (%d device(s) currently visible there)", nixBackend.Distro, nixBackend.USBDevices))
		}
		info.Notes = append(info.Notes, "Sharing asks for administrator approval once per device; attach/detach never do")
	case "lima":
		state := "stopped"
		if rfutils.IsLimaInstanceRunning(limaInstanceName()) {
			state = "running"
		}
		info.Notes = append(info.Notes, "Lima VM "+limaInstanceName()+": "+state)
	default:
		return info, fmt.Errorf("USB passthrough is available on macOS (Lima) and Windows (usbipd-win)")
	}
	return info, nil
}

// markUSB records or clears our session view of a forwarded device (Lima).
func (a *App) markUSB(devID string, attached bool) {
	a.usbMu.Lock()
	defer a.usbMu.Unlock()
	if a.usbAttached == nil {
		a.usbAttached = map[string]bool{}
	}
	if attached {
		a.usbAttached[devID] = true
	} else {
		delete(a.usbAttached, devID)
	}
}

// attachedUSBIDs is the set of forwarded QMP device ids: what we attached this
// session, unioned with anything `info usb` reports (e.g. attached earlier or
// from the CLI). The VM probe is best-effort — a stopped VM just yields our
// session set.
func (a *App) attachedUSBIDs() map[string]bool {
	out := map[string]bool{}
	a.usbMu.Lock()
	for id := range a.usbAttached {
		out[id] = true
	}
	a.usbMu.Unlock()
	if info, err := rfutils.ListUSBInLimaVM(limaInstanceName()); err == nil {
		for _, tok := range strings.Fields(strings.ReplaceAll(info, ",", " ")) {
			tok = strings.TrimSpace(tok)
			if strings.HasPrefix(tok, "usb-") {
				out[strings.ToLower(tok)] = true
			}
		}
	}
	return out
}

// ListHostUSB lists host USB devices with their forwarding state for the
// active backend.
func (a *App) ListHostUSB() ([]USBDevice, error) {
	switch a.usbBackend() {
	case "lima":
		return a.listLimaUSB()
	case "usbipd":
		return a.listWinUSB()
	default:
		return nil, fmt.Errorf("USB passthrough is available on macOS (Lima) and Windows (usbipd-win)")
	}
}

func (a *App) listLimaUSB() ([]USBDevice, error) {
	devices, err := rfutils.ListMacUSBDevices()
	if err != nil {
		return nil, err
	}
	attached := a.attachedUSBIDs()
	out := make([]USBDevice, 0, len(devices))
	for _, d := range devices {
		if isNonForwardableUSB(d) {
			continue
		}
		dev := USBDevice{
			Name:      d.Name,
			VendorID:  d.VendorID,
			ProductID: d.ProductID,
			Serial:    d.Serial,
			Attached:  attached[usbDevID(d.VendorID, d.ProductID)],
			Connected: true,
			State:     "host",
		}
		if dev.Attached {
			dev.State = "attached"
		}
		out = append(out, dev)
	}
	return out, nil
}

func (a *App) listWinUSB() ([]USBDevice, error) {
	devices, err := rfutils.ListUSBDevices()
	if err != nil {
		return nil, err
	}
	out := make([]USBDevice, 0, len(devices))
	for _, d := range devices {
		if isNonForwardableWinUSB(d) {
			continue
		}
		out = append(out, winUSBDevice(d))
	}
	return out, nil
}

// winUSBDevice maps a usbipd device to the GUI model.
func winUSBDevice(d rfutils.USBDevice) USBDevice {
	dev := USBDevice{
		Name:      d.Name,
		VendorID:  d.VendorID,
		ProductID: d.ProductID,
		Attached:  d.Attached,
		BusID:     d.BusID,
		GUID:      d.PersistedGUID,
		Shared:    d.Shared,
		Connected: d.Connected,
	}
	if d.Description != "" && d.Description != d.Name {
		dev.Description = d.Description
	}
	switch {
	case d.Attached:
		dev.State = "attached"
	case !d.Connected:
		dev.State = "unplugged"
	case d.Shared:
		dev.State = "shared"
	default:
		dev.State = "host"
	}
	if rfutils.IsUSBInputDevice(d) {
		dev.Warning = "Looks like a keyboard or mouse: Windows cannot use it while it is attached to WSL 2."
	}
	if d.Forced {
		dev.Warning = strings.TrimSpace(dev.Warning + " Shared with --force: Windows cannot use it while shared.")
	}
	return dev
}

// isNonForwardableUSB filters the endpoints you would never pass through: USB
// hubs, root/host controllers and billboard (cable) devices, which otherwise
// bury the actual RF hardware in the picker.
func isNonForwardableUSB(d rfutils.MacUSBDevice) bool {
	n := strings.ToLower(d.Name)
	for _, kw := range []string{"hub", "billboard", "root", "host controller", "xhci", "ehci"} {
		if strings.Contains(n, kw) {
			return true
		}
	}
	return false
}

// isNonForwardableWinUSB drops hubs and controllers usbipd may list.
func isNonForwardableWinUSB(d rfutils.USBDevice) bool {
	n := strings.ToLower(d.Description)
	for _, kw := range []string{"root hub", "generic usb hub", "host controller", "billboard"} {
		if strings.Contains(n, kw) {
			return true
		}
	}
	return false
}

// AttachHostUSB forwards a host USB device (by vid:pid) into the Lima VM. The
// VM must be running — start it from the engine doctor first.
func (a *App) AttachHostUSB(vendorID, productID string) error {
	if a.usbBackend() != "lima" {
		return fmt.Errorf("USB passthrough by vendor/product ID needs macOS with Lima installed (use AttachWinUSB on Windows)")
	}
	if !rfutils.IsLimaInstanceRunning(limaInstanceName()) {
		return fmt.Errorf("the Lima VM is not running. Start it from the engine doctor, then attach the device")
	}
	if err := rfutils.AttachUSBToLima(vendorID, productID, limaInstanceName()); err != nil {
		return err
	}
	a.markUSB(usbDevID(vendorID, productID), true)
	return nil
}

// DetachHostUSB removes a forwarded USB device from the Lima VM.
func (a *App) DetachHostUSB(vendorID, productID string) error {
	if a.usbBackend() != "lima" {
		return fmt.Errorf("USB passthrough by vendor/product ID needs macOS with Lima installed (use DetachWinUSB on Windows)")
	}
	if err := rfutils.DetachUSBFromLima(vendorID, productID, limaInstanceName()); err != nil {
		return err
	}
	a.markUSB(usbDevID(vendorID, productID), false)
	return nil
}

// AttachWinUSB forwards a host device into WSL 2 with usbipd. A device that
// was never shared is bound first; with allowElevation that happens through
// a UAC prompt for usbipd.exe (once per device), otherwise the call fails
// with an explanation. Attaching itself never needs administrator rights.
func (a *App) AttachWinUSB(busID string, allowElevation bool) (WinUSBResult, error) {
	var out WinUSBResult
	if a.usbBackend() != "usbipd" {
		return out, fmt.Errorf("USB passthrough via usbipd needs Windows with usbipd-win installed")
	}
	if !rfutils.IsValidBusID(busID) {
		return out, fmt.Errorf("invalid bus ID %q", busID)
	}
	res, err := rfutils.EnsureUSBDeviceAttached(busID, allowElevation)
	out = WinUSBResult{Device: winUSBDevice(res.Device), Bound: res.Bound, Elevated: res.Elevated, Already: res.Already}
	if err != nil {
		return out, humanWinUSBError(err)
	}
	return out, nil
}

// DetachWinUSB returns a forwarded device to Windows (no elevation needed).
func (a *App) DetachWinUSB(busID string) error {
	if a.usbBackend() != "usbipd" {
		return fmt.Errorf("USB passthrough via usbipd needs Windows with usbipd-win installed")
	}
	if !rfutils.IsValidBusID(busID) {
		return fmt.Errorf("invalid bus ID %q", busID)
	}
	return humanWinUSBError(rfutils.DetachUSBDevice(busID))
}

// UnshareWinUSB stops sharing a device (usbipd unbind) through a UAC prompt.
// ref is the bus ID of a connected device or the GUID of an unplugged one.
// Returns true when a UAC prompt was used.
func (a *App) UnshareWinUSB(ref string) (bool, error) {
	if a.usbBackend() != "usbipd" {
		return false, fmt.Errorf("USB passthrough via usbipd needs Windows with usbipd-win installed")
	}
	elevated, err := rfutils.UnshareUSBDevice(ref, true)
	return elevated, humanWinUSBError(err)
}

// humanWinUSBError adds the next step to the sentinel errors from rfutils.
func humanWinUSBError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, rfutils.ErrUSBElevationDeclined):
		return fmt.Errorf("administrator approval was declined; the device was left unchanged")
	case errors.Is(err, rfutils.ErrUSBNotShared), errors.Is(err, rfutils.ErrUSBAdminRequired):
		return fmt.Errorf("%v. Sharing needs administrator approval once: use Share & attach, or run 'usbipd bind --busid <id>' in an administrator terminal", err)
	case errors.Is(err, rfutils.ErrUsbipdNotInstalled):
		return fmt.Errorf("usbipd-win is not installed (winget install usbipd)")
	default:
		return err
	}
}

// VMUSBInfo returns the raw view from inside the VM - `info usb` for Lima,
// /dev/bus/usb (and lsusb when available) for WSL 2 - for the "what the VM
// currently sees" panel.
func (a *App) VMUSBInfo() (string, error) {
	switch a.usbBackend() {
	case "lima":
		if !rfutils.IsLimaInstanceRunning(limaInstanceName()) {
			return "", fmt.Errorf("the Lima VM is not running")
		}
		return rfutils.ListUSBInLimaVM(limaInstanceName())
	case "usbipd":
		return rfutils.WSLUSBView()
	default:
		return "", fmt.Errorf("USB passthrough is available on macOS (Lima) and Windows (usbipd-win)")
	}
}
