/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"encoding/json"
	"testing"
)

// legacy SPUSBDataType shape (macOS <= 15)
const legacyUSBJSON = `[
  {
    "_name": "USB31Bus",
    "_items": [
      {
        "_name": "HydraSDR RFOne",
        "vendor_id": "0x38af  (www.hydrasdr.com)",
        "product_id": "0x0001",
        "serial_num": "HYDRASDR SN:36B463DC399D7EC7",
        "location_id": "0x01100000 / 1"
      }
    ]
  }
]`

// SPUSBHostDataType shape (macOS 26 / Darwin 25+)
const hostUSBJSON = `[
  {
    "_name": "USB 3.1 Bus",
    "Driver": "AppleT8112USBXHCI",
    "USBKeyLocationID": "0x00000000"
  },
  {
    "_name": "USB 3.1 Bus",
    "_items": [
      {
        "_name": "HydraSDR RFOne",
        "USBDeviceKeyVendorID": "0x38af",
        "USBDeviceKeyProductID": "0x0001",
        "USBDeviceKeySerialNumber": "HYDRASDR SN:36B463DC399D7EC7",
        "USBKeyLocationID": "0x01100000"
      }
    ]
  },
  {
    "_name": "Simulated Bus",
    "Driver": "AppleUSBUserHCI",
    "USBKeyLocationID": "0x80000000"
  }
]`

func extractFromJSON(t *testing.T, raw string) []MacUSBDevice {
	t.Helper()
	var items []interface{}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatalf("bad test fixture: %v", err)
	}
	var devices []MacUSBDevice
	for _, item := range items {
		extractUSBDevices(item, &devices)
	}
	return devices
}

func TestExtractUSBDevicesLegacyKeys(t *testing.T) {
	devices := extractFromJSON(t, legacyUSBJSON)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	d := devices[0]
	if d.VendorID != "0x38af" || d.ProductID != "0x0001" {
		t.Errorf("bad IDs: %s:%s", d.VendorID, d.ProductID)
	}
	if d.Name != "HydraSDR RFOne" || d.Serial != "HYDRASDR SN:36B463DC399D7EC7" {
		t.Errorf("bad name/serial: %q / %q", d.Name, d.Serial)
	}
}

func TestExtractUSBDevicesHostKeys(t *testing.T) {
	devices := extractFromJSON(t, hostUSBJSON)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device (buses without IDs skipped), got %d", len(devices))
	}
	d := devices[0]
	if d.VendorID != "0x38af" || d.ProductID != "0x0001" {
		t.Errorf("bad IDs: %s:%s", d.VendorID, d.ProductID)
	}
	if d.Serial != "HYDRASDR SN:36B463DC399D7EC7" || d.Location != "0x01100000" {
		t.Errorf("bad serial/location: %q / %q", d.Serial, d.Location)
	}
}
