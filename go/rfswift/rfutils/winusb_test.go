package rfutils

import (
	"errors"
	"strings"
	"testing"
)

// Captured from usbipd-win 5.2.0 on a host with a bladeRF 2.0, an RTL-SDR and
// a Bluetooth dongle shared but unplugged, and four connected devices.
const sampleUsbipdState = `{
  "Devices": [
    {
      "BusId": null,
      "ClientIPAddress": null,
      "Description": "Generic Bluetooth Radio",
      "InstanceId": "USB\\VID_0A12&PID_0001\\6&52C0812&0&2",
      "IsForced": false,
      "PersistedGuid": "57b6088d-b4cf-43c9-beb6-27ac9497bed5",
      "StubInstanceId": null
    },
    {
      "BusId": null,
      "ClientIPAddress": null,
      "Description": "Bulk-In, Interface",
      "InstanceId": "USB\\VID_0BDA&PID_2838\\00000001",
      "IsForced": false,
      "PersistedGuid": "e30e1cea-4cc9-4186-bd3b-3d905487215e",
      "StubInstanceId": null
    },
    {
      "BusId": "1-2",
      "ClientIPAddress": null,
      "Description": "USB Input Device, Microsoft Usbccid Smartcard Reader (WUDF)",
      "InstanceId": "USB\\VID_1050&PID_0407\\6&24CE4FEE&0&2",
      "IsForced": false,
      "PersistedGuid": null,
      "StubInstanceId": null
    },
    {
      "BusId": "1-4",
      "ClientIPAddress": null,
      "Description": "Integrated Camera, Integrated IR Camera",
      "InstanceId": "USB\\VID_13D3&PID_56D5\\200901010001",
      "IsForced": false,
      "PersistedGuid": null,
      "StubInstanceId": null
    },
    {
      "BusId": "2-4",
      "ClientIPAddress": "172.22.0.1",
      "Description": "USB Input Device, Razer Blade 14",
      "InstanceId": "USB\\VID_1532&PID_0270\\6&52C0812&0&4",
      "IsForced": false,
      "PersistedGuid": "97569ad4-65bc-472b-afe9-b2c4e9e79c4f",
      "StubInstanceId": null
    },
    {
      "BusId": null,
      "ClientIPAddress": null,
      "Description": "bladeRF 2.0",
      "InstanceId": "USB\\VID_2CF0&PID_5250\\BD7FFFBF8EFB4DE4BA08D94BD5958B06",
      "IsForced": false,
      "PersistedGuid": "062b67f3-3b5f-48ac-a885-b3a72b311549",
      "StubInstanceId": null
    },
    {
      "BusId": "2-3",
      "ClientIPAddress": null,
      "Description": "Intel(R) Wireless Bluetooth(R)",
      "InstanceId": "USB\\VID_8087&PID_0032\\6&52C0812&0&3",
      "IsForced": true,
      "PersistedGuid": "0f55ce7a-1cf8-4f75-a2c0-d1555dbe8ff6",
      "StubInstanceId": null
    }
  ]
}`

const sampleUsbipdList = `Connected:
BUSID  VID:PID    DEVICE                                                        STATE
1-2    1050:0407  USB Input Device, Microsoft Usbccid Smartcard Reader (WUDF)   Not shared
1-4    13d3:56d5  Integrated Camera, Integrated IR Camera                       Not shared
2-3    8087:0032  Intel(R) Wireless Bluetooth(R)                                Shared (forced)
2-4    1532:0270  USB Input Device, Razer Blade 14                              Attached
2-10   2cf0:5250  bladeRF 2.0                                                   Shared

Persisted:
GUID                                  DEVICE
062b67f3-3b5f-48ac-a885-b3a72b311549  bladeRF 2.0
57b6088d-b4cf-43c9-beb6-27ac9497bed5  Generic Bluetooth Radio
e30e1cea-4cc9-4186-bd3b-3d905487215e  Bulk-In, Interface

`

func TestParseUsbipdState(t *testing.T) {
	devices, err := ParseUsbipdState([]byte(sampleUsbipdState))
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 7 {
		t.Fatalf("expected 7 devices, got %d", len(devices))
	}
	// Connected devices first, ordered by bus ID.
	wantOrder := []string{"1-2", "1-4", "2-3", "2-4", "", "", ""}
	for i, want := range wantOrder {
		if devices[i].BusID != want {
			t.Fatalf("device %d: bus %q, want %q", i, devices[i].BusID, want)
		}
	}
	byBus := map[string]USBDevice{}
	for _, d := range devices {
		if d.BusID != "" {
			byBus[d.BusID] = d
		}
	}
	if d := byBus["1-2"]; d.Shared || d.Attached || !d.Connected || d.State() != "not shared" || d.VendorID != "1050" || d.ProductID != "0407" {
		t.Fatalf("1-2 parsed wrongly: %+v", d)
	}
	if d := byBus["2-3"]; !d.Shared || d.Attached || !d.Forced || d.State() != "shared" || d.PersistedGUID != "0f55ce7a-1cf8-4f75-a2c0-d1555dbe8ff6" {
		t.Fatalf("2-3 parsed wrongly: %+v", d)
	}
	if d := byBus["2-4"]; !d.Attached || !d.Shared || d.ClientIP != "172.22.0.1" || d.State() != "attached" || d.HardwareID() != "1532:0270" {
		t.Fatalf("2-4 parsed wrongly: %+v", d)
	}
	// Shared-but-unplugged devices keep their identity, get friendly names and
	// are sorted by name.
	unplugged := devices[4:]
	wantNames := []string{"CSR8510 Bluetooth adapter", "Nuand bladeRF 2.0 micro", "RTL-SDR (Realtek RTL2838)"}
	for i, want := range wantNames {
		d := unplugged[i]
		if d.Name != want || d.Connected || !d.Shared || d.State() != "unplugged" {
			t.Fatalf("unplugged %d: %+v, want name %q", i, d, want)
		}
	}
	if unplugged[2].VendorID != "0bda" || unplugged[2].ProductID != "2838" || unplugged[2].Description != "Bulk-In, Interface" {
		t.Fatalf("RTL-SDR identity lost: %+v", unplugged[2])
	}
}

func TestParseUsbipdStateRejectsGarbage(t *testing.T) {
	if _, err := ParseUsbipdState([]byte("usbipd: error: something")); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestParseUsbipdList(t *testing.T) {
	devices := ParseUsbipdList(sampleUsbipdList)
	if len(devices) != 8 {
		t.Fatalf("expected 8 devices, got %d: %+v", len(devices), devices)
	}
	wantOrder := []string{"1-2", "1-4", "2-3", "2-4", "2-10", "", "", ""}
	for i, want := range wantOrder {
		if devices[i].BusID != want {
			t.Fatalf("device %d: bus %q, want %q", i, devices[i].BusID, want)
		}
	}
	if d := devices[0]; d.Shared || d.VendorID != "1050" || d.Description != "USB Input Device, Microsoft Usbccid Smartcard Reader (WUDF)" {
		t.Fatalf("1-2 parsed wrongly: %+v", d)
	}
	if d := devices[2]; !d.Shared || !d.Forced || d.Attached {
		t.Fatalf("2-3 parsed wrongly: %+v", d)
	}
	if d := devices[3]; !d.Attached || !d.Shared {
		t.Fatalf("2-4 parsed wrongly: %+v", d)
	}
	if d := devices[4]; !d.Shared || d.Attached || d.Name != "Nuand bladeRF 2.0 micro" {
		t.Fatalf("2-10 parsed wrongly: %+v", d)
	}
	if d := devices[5]; d.PersistedGUID != "062b67f3-3b5f-48ac-a885-b3a72b311549" || d.Name != "bladeRF 2.0" || d.Connected || !d.Shared {
		t.Fatalf("persisted parsed wrongly: %+v", d)
	}
}

func TestClassifyUsbipdError(t *testing.T) {
	base := errors.New("exit status 1")
	cases := []struct {
		output string
		want   error
	}{
		{"usbipd: error: Access denied; this operation requires administrator privileges.\n", ErrUSBAdminRequired},
		{"usbipd: error: Device is not shared; run 'usbipd bind --busid 1-2' as administrator first.\n", ErrUSBNotShared},
		{"usbipd: error: There is no device with busid '9-9'.\n", ErrUSBNotFound},
	}
	for _, c := range cases {
		err := classifyUsbipdError(c.output, base)
		if !errors.Is(err, c.want) {
			t.Fatalf("%q classified as %v, want %v", c.output, err, c.want)
		}
		if strings.Contains(err.Error(), "usbipd: error:") {
			t.Fatalf("prefix not stripped: %v", err)
		}
	}
	other := classifyUsbipdError("usbipd: warning: something\nusbipd: error: WSL 2 is not installed.\n", base)
	if other.Error() != "WSL 2 is not installed." {
		t.Fatalf("unexpected message: %v", other)
	}
	if empty := classifyUsbipdError("", base); !strings.Contains(empty.Error(), "exit status 1") {
		t.Fatalf("empty output should keep the exec error: %v", empty)
	}
}

func TestIsValidBusID(t *testing.T) {
	for _, ok := range []string{"1-2", "2-3.1", "12-10.2.3"} {
		if !IsValidBusID(ok) {
			t.Fatalf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "1", "1-", "-2", " 1-2 ", "1-2 --force", "1-2;calc", "a-b", "1-2\n", "0x1d50"} {
		if IsValidBusID(bad) {
			t.Fatalf("%q should be rejected", bad)
		}
	}
	// User input is trimmed before validation, never passed through raw.
	if got, err := normalizeBusID(" 2-3\n"); err != nil || got != "2-3" {
		t.Fatalf("normalizeBusID: %q, %v", got, err)
	}
	if _, err := normalizeBusID("2-3 --force"); err == nil {
		t.Fatal("extra arguments must be rejected")
	}
	if err := AttachUSBDevice("2-3 --force"); err == nil || !strings.Contains(err.Error(), "invalid bus ID") {
		t.Fatalf("AttachUSBDevice must validate before running usbipd: %v", err)
	}
}

func TestIsValidUsbipdGUID(t *testing.T) {
	if !IsValidUsbipdGUID("062b67f3-3b5f-48ac-a885-b3a72b311549") {
		t.Fatal("valid GUID rejected")
	}
	for _, bad := range []string{"", "062b67f3", "062b67f3-3b5f-48ac-a885-b3a72b311549 --all", "{062b67f3-3b5f-48ac-a885-b3a72b311549}"} {
		if IsValidUsbipdGUID(bad) {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

func TestDecodeConsoleOutput(t *testing.T) {
	utf16 := func(s string, bom bool) []byte {
		var b []byte
		if bom {
			b = append(b, 0xFF, 0xFE)
		}
		for _, r := range s {
			b = append(b, byte(r), byte(r>>8))
		}
		return b
	}
	if got := DecodeConsoleOutput(utf16("  NAME  STATE\r\n", true)); got != "  NAME  STATE\r\n" {
		t.Fatalf("BOM UTF-16 decode: %q", got)
	}
	if got := DecodeConsoleOutput(utf16("* Ubuntu-24.04", false)); got != "* Ubuntu-24.04" {
		t.Fatalf("bare UTF-16 decode: %q", got)
	}
	if got := DecodeConsoleOutput([]byte("plain utf-8 é")); got != "plain utf-8 é" {
		t.Fatalf("UTF-8 passthrough: %q", got)
	}
	if got := DecodeConsoleOutput(nil); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestParseWSLList(t *testing.T) {
	text := "  NAME              STATE           VERSION\r\n* Ubuntu-24.04      Running         2\r\n  docker-desktop    Running         2\r\n  Legacy            Stopped         1\r\n"
	distros := ParseWSLList(text)
	if len(distros) != 3 {
		t.Fatalf("expected 3 distros, got %+v", distros)
	}
	if d := distros[0]; d.Name != "Ubuntu-24.04" || !d.Default || d.State != "Running" || d.Version != 2 {
		t.Fatalf("default distro parsed wrongly: %+v", d)
	}
	if d := distros[2]; d.Name != "Legacy" || d.Default || d.Version != 1 {
		t.Fatalf("legacy distro parsed wrongly: %+v", d)
	}
	status := WSLStatus{Installed: true, Distros: distros}
	if !status.HasWSL2Distribution() {
		t.Fatal("expected a WSL 2 distribution")
	}
	if (WSLStatus{Distros: distros[2:]}).HasWSL2Distribution() {
		t.Fatal("WSL 1 only must not count")
	}
}

func TestUSBFriendlyName(t *testing.T) {
	cases := []struct{ vid, pid, desc, want string }{
		{"0bda", "2838", "Bulk-In, Interface", "RTL-SDR (Realtek RTL2838)"},
		{"0BDA", "2838", "", "RTL-SDR (Realtek RTL2838)"},
		{"0x1d50", "0x6089", "HackRF One", "HackRF One"},
		{"0403", "6bad", "USB Serial Converter", "FTDI - USB Serial Converter"},
		{"0403", "6bad", "", "FTDI device"},
		{"8087", "0032", "Intel(R) Wireless Bluetooth(R)", "Intel(R) Wireless Bluetooth(R)"},
		{"ffff", "0001", "", "USB device ffff:0001"},
		{"", "", "", "USB device"},
	}
	for _, c := range cases {
		if got := USBFriendlyName(c.vid, c.pid, c.desc); got != c.want {
			t.Fatalf("USBFriendlyName(%q,%q,%q) = %q, want %q", c.vid, c.pid, c.desc, got, c.want)
		}
	}
	if !IsUSBInputDevice(USBDevice{Description: "USB Input Device, Razer Blade 14"}) || IsUSBInputDevice(USBDevice{Description: "bladeRF 2.0"}) {
		t.Fatal("input device detection")
	}
	if !IsKnownRFHardware(USBDevice{VendorID: "2cf0", ProductID: "5250"}) || IsKnownRFHardware(USBDevice{VendorID: "1532", ProductID: "0270"}) {
		t.Fatal("known hardware detection")
	}
}

func TestBusIDOrdering(t *testing.T) {
	if !busIDLess("2-3", "2-10") || busIDLess("2-10", "2-3") || !busIDLess("1-9", "2-1") || !busIDLess("2-3", "2-3.1") {
		t.Fatal("bus IDs must sort numerically")
	}
}

func TestUsbipdMessage(t *testing.T) {
	if got := usbipdMessage("usbipd: warning: old\nusbipd: error: real problem\n"); got != "real problem" {
		t.Fatalf("got %q", got)
	}
	if got := usbipdMessage("usbipd: info: Device with busid '1-2' was already not attached.\n"); got != "usbipd: info: Device with busid '1-2' was already not attached." {
		t.Fatalf("got %q", got)
	}
}
