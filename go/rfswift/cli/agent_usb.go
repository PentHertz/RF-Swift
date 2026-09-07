package cli

/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Agent-side USB passthrough. The Workbench drives the USB backend on the
*  agent's own host over the control plane (remote/usb.go): usbipd on Windows,
*  Lima on macOS, native /dev on Linux. Everything runs in-process through the
*  same rfutils helpers the local CLI/Workbench use, so the device must be on
*  the agent host. First-time sharing on Windows needs administrator rights on
*  that host - a headless remote agent cannot show the UAC prompt, so a device
*  must be pre-shared there ("usbipd bind"); attach/detach never need it.
 */

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"penthertz/rfswift/remote"
	rfutils "penthertz/rfswift/rfutils"
)

// agentLimaInstance is the Lima instance the agent forwards USB into on macOS,
// matching the Workbench (RFSWIFT_LIMA_INSTANCE, else "rfswift").
func agentLimaInstance() string {
	if v := os.Getenv("RFSWIFT_LIMA_INSTANCE"); v != "" {
		return v
	}
	return "rfswift"
}

func errUSBUnsupported() error {
	return fmt.Errorf("USB passthrough is not available on this agent host (%s)", runtime.GOOS)
}

// agentUSBInfo reports the USB passthrough backend on the agent host.
func agentUSBInfo() (remote.USBHostInfo, error) {
	info := remote.USBHostInfo{OS: runtime.GOOS}
	switch runtime.GOOS {
	case "windows":
		if !rfutils.IsUsbipdInstalled() {
			info.Notes = append(info.Notes, "usbipd-win is not installed on the agent host (winget install usbipd)")
			return info, nil
		}
		info.Backend = "usbipd"
		if v, err := rfutils.UsbipdVersion(); err == nil {
			info.Version = v
			info.Notes = append(info.Notes, "usbipd-win "+v+" on the agent host")
		}
		if wsl, err := rfutils.WSLDistributions(); err != nil {
			info.Notes = append(info.Notes, "WSL: "+err.Error())
		} else if !wsl.HasWSL2Distribution() {
			info.Notes = append(info.Notes, "No WSL 2 distribution on the agent host (wsl --install -d Ubuntu)")
		} else {
			info.WSLDistro = wsl.DefaultDistro
			if info.WSLDistro != "" {
				info.Notes = append(info.Notes, "WSL 2 default distribution: "+info.WSLDistro)
			}
		}
		info.Notes = append(info.Notes, "The device must be plugged into the agent host. Sharing one the first time needs administrator rights there; attach/detach do not")
	case "darwin":
		if !rfutils.IsLimaInstalled() {
			info.Notes = append(info.Notes, "Lima is not installed on the agent host")
			return info, nil
		}
		info.Backend = "lima"
		inst := agentLimaInstance()
		if rfutils.IsLimaInstanceRunning(inst) {
			info.VMState = "running"
		} else {
			info.VMState = "stopped"
			info.Notes = append(info.Notes, "Lima VM "+inst+" is not running on the agent host")
		}
		info.Notes = append(info.Notes, "The device must be plugged into the agent host")
	case "linux":
		info.Backend = "native"
		info.Notes = append(info.Notes, "USB is native on this Linux agent host: a device plugged into it appears under /dev/bus/usb for containers automatically - no forwarding step is needed")
	default:
		info.Notes = append(info.Notes, "USB passthrough is not supported on this agent host OS")
	}
	return info, nil
}

// agentUSBList lists the forwardable USB devices on the agent host.
func agentUSBList() ([]remote.USBDevice, error) {
	switch runtime.GOOS {
	case "windows":
		devices, err := rfutils.ListUSBDevices()
		if err != nil {
			return nil, err
		}
		out := make([]remote.USBDevice, 0, len(devices))
		for _, d := range devices {
			if agentUSBIsHub(d.Description) {
				continue
			}
			out = append(out, remote.USBDevice{
				Name:        d.Name,
				VendorID:    d.VendorID,
				ProductID:   d.ProductID,
				Description: d.Description,
				BusID:       d.BusID,
				GUID:        d.PersistedGUID,
				State:       d.State(),
				Attached:    d.Attached,
				Shared:      d.Shared,
				Connected:   d.Connected,
				Forced:      d.Forced,
				InputDevice: rfutils.IsUSBInputDevice(d),
			})
		}
		return out, nil
	case "darwin":
		devices, err := rfutils.ListMacUSBDevices()
		if err != nil {
			return nil, err
		}
		attached := agentLimaAttachedIDs()
		out := make([]remote.USBDevice, 0, len(devices))
		for _, d := range devices {
			if agentUSBIsHub(d.Name) {
				continue
			}
			dev := remote.USBDevice{
				Name:      d.Name,
				VendorID:  d.VendorID,
				ProductID: d.ProductID,
				Serial:    d.Serial,
				Connected: true,
				State:     "host",
			}
			if attached[usbDevIDFor(d.VendorID, d.ProductID)] {
				dev.Attached, dev.State = true, "attached"
			}
			out = append(out, dev)
		}
		return out, nil
	case "linux":
		// Native: nothing to forward from here. The devices are visible to
		// containers directly; usb.view lists them for reference.
		return []remote.USBDevice{}, nil
	default:
		return nil, errUSBUnsupported()
	}
}

// agentUSBView is the "what the VM / host currently sees" panel.
func agentUSBView() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return rfutils.WSLUSBView()
	case "darwin":
		if !rfutils.IsLimaInstanceRunning(agentLimaInstance()) {
			return "", fmt.Errorf("the Lima VM is not running on the agent host")
		}
		return rfutils.ListUSBInLimaVM(agentLimaInstance())
	case "linux":
		out, err := exec.Command("lsusb").Output()
		if err != nil {
			// lsusb is optional; the native /dev tree is what matters.
			return "", nil
		}
		return string(out), nil
	default:
		return "", errUSBUnsupported()
	}
}

// agentUSBAttach forwards one device into the agent host's VM.
func agentUSBAttach(req remote.USBAttachRequest) (remote.USBAttachResult, error) {
	switch runtime.GOOS {
	case "windows":
		if !rfutils.IsValidBusID(req.BusID) {
			return remote.USBAttachResult{}, fmt.Errorf("invalid bus ID %q", req.BusID)
		}
		res, err := rfutils.EnsureUSBDeviceAttached(req.BusID, req.AllowElevation)
		out := remote.USBAttachResult{
			Device:   agentWinUSBDevice(res.Device),
			Bound:    res.Bound,
			Elevated: res.Elevated,
			Already:  res.Already,
		}
		return out, agentHumanWinUSBError(err)
	case "darwin":
		if req.VendorID == "" || req.ProductID == "" {
			return remote.USBAttachResult{}, errors.New("vendor and product ID are required to forward a device on a macOS (Lima) agent")
		}
		if !rfutils.IsLimaInstanceRunning(agentLimaInstance()) {
			return remote.USBAttachResult{}, errors.New("the Lima VM is not running on the agent host; start it there first")
		}
		if err := rfutils.AttachUSBToLima(req.VendorID, req.ProductID, agentLimaInstance()); err != nil {
			return remote.USBAttachResult{}, err
		}
		return remote.USBAttachResult{}, nil
	case "linux":
		return remote.USBAttachResult{}, errors.New("USB is native on the Linux agent host: plug the device into the agent - no attach is needed")
	default:
		return remote.USBAttachResult{}, errUSBUnsupported()
	}
}

// agentUSBDetach returns one forwarded device to the agent host.
func agentUSBDetach(req remote.USBDetachRequest) error {
	switch runtime.GOOS {
	case "windows":
		if !rfutils.IsValidBusID(req.BusID) {
			return fmt.Errorf("invalid bus ID %q", req.BusID)
		}
		return agentHumanWinUSBError(rfutils.DetachUSBDevice(req.BusID))
	case "darwin":
		if req.VendorID == "" || req.ProductID == "" {
			return errors.New("vendor and product ID are required to detach a device on a macOS (Lima) agent")
		}
		return rfutils.DetachUSBFromLima(req.VendorID, req.ProductID, agentLimaInstance())
	case "linux":
		return errors.New("USB is native on the Linux agent host: nothing to detach")
	default:
		return errUSBUnsupported()
	}
}

// agentUSBUnshare stops sharing a device (usbipd unbind) on a Windows agent.
func agentUSBUnshare(ref string) (remote.USBUnshareResult, error) {
	if runtime.GOOS != "windows" {
		return remote.USBUnshareResult{}, errUSBUnsupported()
	}
	elevated, err := rfutils.UnshareUSBDevice(ref, true)
	return remote.USBUnshareResult{Elevated: elevated}, agentHumanWinUSBError(err)
}

// agentWinUSBDevice maps a usbipd device to the wire model.
func agentWinUSBDevice(d rfutils.USBDevice) remote.USBDevice {
	return remote.USBDevice{
		Name:        d.Name,
		VendorID:    d.VendorID,
		ProductID:   d.ProductID,
		Description: d.Description,
		BusID:       d.BusID,
		GUID:        d.PersistedGUID,
		State:       d.State(),
		Attached:    d.Attached,
		Shared:      d.Shared,
		Connected:   d.Connected,
		Forced:      d.Forced,
		InputDevice: rfutils.IsUSBInputDevice(d),
	}
}

// agentUSBIsHub drops hubs and controllers that would otherwise bury the RF
// hardware in the picker (matches the Workbench's local filter).
func agentUSBIsHub(name string) bool {
	n := strings.ToLower(name)
	for _, kw := range []string{"root hub", "generic usb hub", "hub", "host controller", "xhci", "ehci", "billboard"} {
		if strings.Contains(n, kw) {
			return true
		}
	}
	return false
}

// usbDevIDFor mirrors the QMP device id AttachUSBToLima assigns
// (usb-<vid>-<pid> without the 0x prefixes).
func usbDevIDFor(vendorID, productID string) string {
	return "usb-" + strings.TrimPrefix(strings.ToLower(vendorID), "0x") + "-" + strings.TrimPrefix(strings.ToLower(productID), "0x")
}

// agentLimaAttachedIDs is the set of QMP device ids currently forwarded into
// the agent's Lima VM (best-effort: a stopped VM yields the empty set).
func agentLimaAttachedIDs() map[string]bool {
	out := map[string]bool{}
	info, err := rfutils.ListUSBInLimaVM(agentLimaInstance())
	if err != nil {
		return out
	}
	for _, tok := range strings.Fields(strings.ReplaceAll(info, ",", " ")) {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if strings.HasPrefix(tok, "usb-") {
			out[tok] = true
		}
	}
	return out
}

// agentHumanWinUSBError turns rfutils' sentinel USB errors into actionable
// messages before they cross the wire (the sentinel identity is lost as a
// string), pointing at the agent host where the action must happen.
func agentHumanWinUSBError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, rfutils.ErrUSBElevationDeclined):
		return errors.New("administrator approval was declined on the agent host; the device was left unchanged")
	case errors.Is(err, rfutils.ErrUSBNotShared), errors.Is(err, rfutils.ErrUSBAdminRequired):
		return fmt.Errorf("%v. Sharing needs administrator rights on the agent host once: run 'usbipd bind --busid <id>' in an administrator terminal there (a headless agent cannot show the UAC prompt), then attach", err)
	case errors.Is(err, rfutils.ErrUsbipdNotInstalled):
		return errors.New("usbipd-win is not installed on the agent host (winget install usbipd)")
	default:
		return err
	}
}
