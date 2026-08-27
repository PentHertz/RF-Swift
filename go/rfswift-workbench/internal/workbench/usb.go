package workbench

import (
	"fmt"
	"runtime"
	"strings"

	rfutils "penthertz/rfswift/rfutils"
)

// USBDevice is a USB device on the macOS host, with whether it is currently
// forwarded into the Lima VM. USB passthrough is a macOS-only feature: Docker
// Desktop/OrbStack and Podman cannot forward USB, so RF Swift hot-plugs the
// device into the Lima QEMU VM over QMP (the same path as `rfswift macusb`).
type USBDevice struct {
	Name      string `json:"name"`
	VendorID  string `json:"vendorId"` // 0x-prefixed hex, e.g. 0x1d50
	ProductID string `json:"productId"`
	Serial    string `json:"serial"`
	Attached  bool   `json:"attached"` // forwarded into the Lima VM
}

// usbDevID mirrors the QMP device id AttachUSBToLima assigns (usb-<vid>-<pid>
// without the 0x prefixes), so we can match forwarded devices in `info usb`.
func usbDevID(vendorID, productID string) string {
	return "usb-" + strings.TrimPrefix(strings.ToLower(vendorID), "0x") + "-" + strings.TrimPrefix(strings.ToLower(productID), "0x")
}

// usbSupported reports whether USB passthrough controls apply here: macOS, a
// local (not remote) engine, and Lima installed.
func (a *App) usbSupported() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	if _, ok := a.eng.(*LocalEngine); !ok {
		return false
	}
	return rfutils.IsLimaInstalled()
}

// USBSupported tells the GUI whether to surface USB passthrough controls at all.
func (a *App) USBSupported() bool { return a.usbSupported() }

// markUSB records or clears our session view of a forwarded device.
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

// ListHostUSB lists USB devices on the macOS host, flagging those already
// forwarded into the Lima VM.
func (a *App) ListHostUSB() ([]USBDevice, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("USB passthrough is a macOS + Lima feature")
	}
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
		out = append(out, USBDevice{
			Name:      d.Name,
			VendorID:  d.VendorID,
			ProductID: d.ProductID,
			Serial:    d.Serial,
			Attached:  attached[usbDevID(d.VendorID, d.ProductID)],
		})
	}
	return out, nil
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

// AttachHostUSB forwards a host USB device (by vid:pid) into the Lima VM. The
// VM must be running — start it from the engine doctor first.
func (a *App) AttachHostUSB(vendorID, productID string) error {
	if !a.usbSupported() {
		return fmt.Errorf("USB passthrough needs macOS with Lima installed")
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
	if !a.usbSupported() {
		return fmt.Errorf("USB passthrough needs macOS with Lima installed")
	}
	if err := rfutils.DetachUSBFromLima(vendorID, productID, limaInstanceName()); err != nil {
		return err
	}
	a.markUSB(usbDevID(vendorID, productID), false)
	return nil
}

// VMUSBInfo returns the raw `info usb` view from inside the Lima VM, for the
// "what the VM currently sees" panel.
func (a *App) VMUSBInfo() (string, error) {
	if !a.usbSupported() {
		return "", fmt.Errorf("USB passthrough needs macOS with Lima installed")
	}
	if !rfutils.IsLimaInstanceRunning(limaInstanceName()) {
		return "", fmt.Errorf("the Lima VM is not running")
	}
	return rfutils.ListUSBInLimaVM(limaInstanceName())
}
