package remote

// USB passthrough over the agent control plane. The Workbench drives the USB
// backend on the *agent's* host (usbipd on Windows, Lima on macOS, native
// /dev on Linux) through these control methods, so a device plugged into the
// machine that actually runs the containers can be forwarded into its VM the
// same way it is for a local engine:
//
//	usb.info     -> USBHostInfo     which backend the agent host offers
//	usb.list     -> []USBDevice     forwardable devices on the agent host
//	usb.view     -> string          what the VM / host currently sees
//	usb.attach   -> USBAttachResult forward one device (USBAttachRequest)
//	usb.detach                      return one device (USBDetachRequest)
//	usb.unshare  -> USBUnshareResult stop sharing one device (USBUnshareRequest)
//
// The device must be physically on the agent host: the Workbench cannot tunnel
// a USB device from its own machine to a remote agent. First-time sharing on a
// Windows agent needs administrator rights on that host (a UAC prompt for
// usbipd.exe), so a headless remote agent needs the device pre-shared; attach
// and detach never need elevation.

// USBHostInfo describes the USB passthrough backend on the agent host.
type USBHostInfo struct {
	Backend   string   `json:"backend"` // "usbipd" | "lima" | "native" | ""
	OS        string   `json:"os"`      // agent host GOOS
	Version   string   `json:"version,omitempty"`
	WSLDistro string   `json:"wslDistro,omitempty"` // Windows: default WSL 2 distribution
	VMState   string   `json:"vmState,omitempty"`   // macOS: Lima VM running|stopped
	Notes     []string `json:"notes,omitempty"`
}

// USBDevice is one USB device on the agent host, backend-neutral.
type USBDevice struct {
	Name        string `json:"name"`
	VendorID    string `json:"vendorId"`
	ProductID   string `json:"productId"`
	Serial      string `json:"serial,omitempty"`
	Description string `json:"description,omitempty"`
	BusID       string `json:"busId,omitempty"` // usbipd bus ID ("2-3")
	GUID        string `json:"guid,omitempty"`  // usbipd registration GUID while shared
	State       string `json:"state"`           // attached | shared | host | unplugged
	Attached    bool   `json:"attached"`
	Shared      bool   `json:"shared"`
	Connected   bool   `json:"connected"`
	Forced      bool   `json:"forced,omitempty"`
	InputDevice bool   `json:"inputDevice,omitempty"` // keyboard/mouse: host loses it while attached
}

// USBAttachRequest forwards a device. Windows identifies it by BusID; macOS
// (Lima) by VendorID/ProductID. AllowElevation permits a first-time share to
// raise a UAC prompt on the agent host (Windows).
type USBAttachRequest struct {
	BusID          string `json:"busId,omitempty"`
	VendorID       string `json:"vendorId,omitempty"`
	ProductID      string `json:"productId,omitempty"`
	AllowElevation bool   `json:"allowElevation,omitempty"`
}

// USBAttachResult reports what the agent had to do to forward the device.
type USBAttachResult struct {
	Device   USBDevice `json:"device"`
	Bound    bool      `json:"bound"`    // it was shared first (one-time)
	Elevated bool      `json:"elevated"` // sharing went through a UAC prompt on the agent
	Already  bool      `json:"already"`  // it was already attached; nothing changed
}

// USBDetachRequest returns a forwarded device. Same identifiers as attach.
type USBDetachRequest struct {
	BusID     string `json:"busId,omitempty"`
	VendorID  string `json:"vendorId,omitempty"`
	ProductID string `json:"productId,omitempty"`
}

// USBUnshareRequest stops sharing a device (usbipd unbind). Ref is the bus ID
// of a connected device or the GUID of an unplugged one.
type USBUnshareRequest struct {
	Ref string `json:"ref"`
}

// USBUnshareResult says whether a UAC prompt was used on the agent host.
type USBUnshareResult struct {
	Elevated bool `json:"elevated"`
}
