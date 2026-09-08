package workbench

import (
	"runtime"
	"testing"

	rfutils "penthertz/rfswift/rfutils"
)

func TestWinUSBDeviceMapping(t *testing.T) {
	shared := winUSBDevice(rfutils.USBDevice{
		BusID: "2-3", VendorID: "2cf0", ProductID: "5250", Description: "bladeRF 2.0",
		Name: "Nuand bladeRF 2.0 micro", Connected: true, Shared: true, PersistedGUID: "062b67f3-3b5f-48ac-a885-b3a72b311549",
	})
	if shared.State != "shared" || shared.Attached || !shared.Shared || shared.Description != "bladeRF 2.0" || shared.Warning != "" || shared.GUID == "" {
		t.Fatalf("shared device mapped wrongly: %+v", shared)
	}
	attached := winUSBDevice(rfutils.USBDevice{BusID: "1-2", Name: "HackRF One", Description: "HackRF One", Connected: true, Shared: true, Attached: true})
	if attached.State != "attached" || !attached.Attached || attached.Description != "" {
		t.Fatalf("attached device mapped wrongly: %+v", attached)
	}
	unplugged := winUSBDevice(rfutils.USBDevice{Name: "RTL-SDR (Realtek RTL2838)", Description: "Bulk-In, Interface", Shared: true})
	if unplugged.State != "unplugged" || unplugged.BusID != "" || unplugged.Connected {
		t.Fatalf("unplugged device mapped wrongly: %+v", unplugged)
	}
	input := winUSBDevice(rfutils.USBDevice{BusID: "2-4", Description: "USB Input Device, Razer Blade 14", Name: "USB Input Device, Razer Blade 14", Connected: true})
	if input.State != "host" || input.Warning == "" {
		t.Fatalf("input device must be flagged: %+v", input)
	}
	if !isNonForwardableWinUSB(rfutils.USBDevice{Description: "USB Root Hub (USB 3.0)"}) || isNonForwardableWinUSB(rfutils.USBDevice{Description: "bladeRF 2.0"}) {
		t.Fatal("hub filter")
	}
}

func TestUSBBackendSelection(t *testing.T) {
	remote := &App{eng: &RemoteEngine{}}
	if remote.USBBackend() != "" || remote.USBSupported() {
		t.Fatal("remote engines must not expose local USB passthrough")
	}
	local := &App{eng: NewLocalEngine()}
	backend := local.USBBackend()
	switch runtime.GOOS {
	case "linux":
		if backend != "" {
			t.Fatalf("no USB backend expected on Linux, got %q", backend)
		}
	case "windows":
		if backend != "" && backend != "usbipd" {
			t.Fatalf("unexpected backend on Windows: %q", backend)
		}
		if backend == "" {
			t.Skip("usbipd-win not installed on this host")
		}
		if _, err := local.USBHostInfo(); err != nil {
			t.Fatalf("USBHostInfo: %v", err)
		}
		if _, err := local.AttachWinUSB("not a bus id", false); err == nil {
			t.Fatal("invalid bus IDs must be rejected before reaching usbipd")
		}
	case "darwin":
		if backend != "" && backend != "lima" {
			t.Fatalf("unexpected backend on macOS: %q", backend)
		}
	}
	if backend != "usbipd" {
		if _, err := local.AttachWinUSB("1-2", false); err == nil {
			t.Fatal("AttachWinUSB must refuse to run without the usbipd backend")
		}
	}
}
