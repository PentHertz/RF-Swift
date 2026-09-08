/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Small USB ID table for the hardware RF Swift users forward into containers.
*  Windows reports the driver name rather than the product ("Bulk-In,
*  Interface" for an RTL-SDR, "USB Serial Device" for a Proxmark), which makes
*  a device picker hard to read; this table restores recognisable names. It is
*  deliberately short and only lists IDs that are stable in the field.
 */

package rfutils

import "strings"

// usbProductNames maps "vid:pid" (lowercase, no 0x) to a product name.
var usbProductNames = map[string]string{
	// SDR receivers / transceivers
	"0bda:2838": "RTL-SDR (Realtek RTL2838)",
	"0bda:2832": "RTL-SDR (Realtek RTL2832U)",
	"1d50:6089": "HackRF One",
	"1d50:604b": "HackRF Jawbreaker",
	"2cf0:5250": "Nuand bladeRF 2.0 micro",
	"2cf0:5246": "Nuand bladeRF",
	"1d50:6066": "Nuand bladeRF (legacy firmware)",
	"1d50:60a1": "Airspy R2 / Mini",
	"03eb:800c": "Airspy HF+",
	"1d50:6108": "LimeSDR USB",
	"0403:601f": "LimeSDR Mini (FT601)",
	"2500:0020": "Ettus USRP B200/B210",
	"2500:0021": "Ettus USRP B200mini",
	"2500:0022": "Ettus USRP B205mini",
	"0456:b673": "ADALM-PLUTO (PlutoSDR)",
	"1df7:2500": "SDRplay RSP1",
	"1df7:3000": "SDRplay RSP1A",
	"1df7:3010": "SDRplay RSP2",
	"1df7:3020": "SDRplay RSPduo",
	"1df7:3030": "SDRplay RSPdx",
	"1df7:3050": "SDRplay RSP1B",
	// Sub-GHz / Bluetooth / Wi-Fi research hardware
	"1d50:605b": "YARD Stick One",
	"1d50:6002": "Ubertooth One",
	"1d50:60e6": "GreatFET One",
	"0451:16ae": "TI CC2531 USB dongle (packet sniffer)",
	"0451:bef3": "TI XDS110 debug probe (CC26xx LaunchPad, Sniffle)",
	"1915:521f": "Nordic nRF52840 dongle (DFU bootloader)",
	"0a12:0001": "CSR8510 Bluetooth adapter",
	// RFID / NFC
	"9ac4:4b8f": "Proxmark3 (RRG/Iceman firmware)",
	"2d2d:504d": "Proxmark3 RDV4",
	"072f:2200": "ACS ACR122U NFC reader",
	"04e6:5591": "SCM SCL3711 NFC reader",
	// Automotive / hardware hacking
	"1d50:606f": "CANtact",
	"1d50:6018": "Black Magic Probe",
	"0483:3748": "ST-Link/V2",
	"0483:374b": "ST-Link/V2-1",
	"0483:374e": "ST-Link/V3",
	"0483:374f": "ST-Link/V3",
	"0483:df11": "STM32 DFU bootloader",
	"0483:5740": "STM32 virtual COM port (Flipper Zero, STM32 CDC)",
	"1366:0101": "SEGGER J-Link",
	"1366:0105": "SEGGER J-Link",
	"21a9:1001": "Saleae Logic",
	"21a9:1003": "Saleae Logic 4",
	"21a9:1004": "Saleae Logic 8",
	"21a9:1005": "Saleae Logic Pro 8",
	"21a9:1006": "Saleae Logic Pro 16",
	"2e8a:0003": "Raspberry Pi RP2040 (BOOTSEL)",
	"2e8a:000a": "Raspberry Pi Pico (CDC)",
	"303a:1001": "Espressif ESP32-S3/C3 (native USB)",
	// USB-serial bridges (ESP32 boards, Arduino clones, adapters)
	"0403:6001": "FTDI FT232R USB-serial",
	"0403:6010": "FTDI FT2232 USB-serial/JTAG",
	"0403:6011": "FTDI FT4232 USB-serial/JTAG",
	"0403:6014": "FTDI FT232H USB-serial/JTAG",
	"0403:6015": "FTDI FT-X USB-serial",
	"10c4:ea60": "Silicon Labs CP210x USB-serial",
	"10c4:ea70": "Silicon Labs CP2105 USB-serial",
	"1a86:7523": "WCH CH340 USB-serial",
	"1a86:55d4": "WCH CH9102 USB-serial",
	"067b:2303": "Prolific PL2303 USB-serial",
	"2341:0043": "Arduino Uno",
	"2341:0042": "Arduino Mega 2560",
}

// usbVendorNames provides a vendor label when the exact product is unknown.
var usbVendorNames = map[string]string{
	"0bda": "Realtek",
	"1d50": "OpenMoko (open-hardware ID)",
	"1209": "pid.codes (open-hardware ID)",
	"2cf0": "Nuand",
	"2500": "Ettus Research",
	"1df7": "SDRplay",
	"0456": "Analog Devices",
	"03eb": "Atmel / Microchip",
	"0403": "FTDI",
	"10c4": "Silicon Labs",
	"1a86": "WCH",
	"067b": "Prolific",
	"0483": "STMicroelectronics",
	"1366": "SEGGER",
	"21a9": "Saleae",
	"0451": "Texas Instruments",
	"1915": "Nordic Semiconductor",
	"0a12": "Cambridge Silicon Radio",
	"9ac4": "Proxmark",
	"2d2d": "Proxmark RDV4",
	"072f": "ACS",
	"04e6": "SCM Microsystems",
	"2e8a": "Raspberry Pi",
	"303a": "Espressif",
	"2341": "Arduino",
	"8087": "Intel",
	"0cf3": "Qualcomm Atheros",
	"148f": "Ralink / MediaTek",
	"0e8d": "MediaTek",
	"2357": "TP-Link",
	"0846": "Netgear",
	"7392": "Edimax",
	"04b4": "Cypress",
	"1fc9": "NXP",
	"04d8": "Microchip",
	"18d1": "Google",
	"04e8": "Samsung",
	"12d1": "Huawei",
	"2c7c": "Quectel",
	"1bc7": "Telit",
}

// genericWindowsDescriptions are driver names Windows uses for many unrelated
// devices; a vendor label is more useful than these.
var genericWindowsDescriptions = []string{
	"bulk-in, interface",
	"usb serial device",
	"usb serial converter",
	"usb composite device",
	"usb input device",
	"unknown usb device",
	"winusb device",
	"libusb",
	"usb device",
}

// USBFriendlyName picks the most recognisable name for a device: the product
// table first, then "<Vendor> device" for generic Windows descriptions, else
// the description itself.
//
//	in(1): string vendorID 4 hex digits
//	in(2): string productID 4 hex digits
//	in(3): string fallback the OS-provided description
//	out: string display name (never empty when any input is non-empty)
func USBFriendlyName(vendorID, productID, fallback string) string {
	vid := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(vendorID), "0x"))
	pid := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(productID), "0x"))
	fallback = strings.TrimSpace(fallback)
	if name, ok := usbProductNames[vid+":"+pid]; ok {
		return name
	}
	if isGenericUSBDescription(fallback) {
		if vendor, ok := usbVendorNames[vid]; ok {
			if fallback == "" {
				return vendor + " device"
			}
			return vendor + " - " + fallback
		}
	}
	if fallback != "" {
		return fallback
	}
	if vendor, ok := usbVendorNames[vid]; ok {
		return vendor + " device"
	}
	if vid != "" || pid != "" {
		return "USB device " + vid + ":" + pid
	}
	return "USB device"
}

// isGenericUSBDescription reports whether a Windows description says nothing
// about the actual product.
func isGenericUSBDescription(desc string) bool {
	lower := strings.ToLower(desc)
	for _, g := range genericWindowsDescriptions {
		if strings.HasPrefix(lower, g) {
			return true
		}
	}
	return false
}

// IsUSBInputDevice flags keyboards/mice-like devices: forwarding one removes it
// from the Windows session, so pickers warn before doing that.
//
//	in(1): USBDevice d
//	out: bool
func IsUSBInputDevice(d USBDevice) bool {
	lower := strings.ToLower(d.Description)
	return strings.Contains(lower, "input device") || strings.Contains(lower, "keyboard") || strings.Contains(lower, "mouse")
}

// IsKnownRFHardware reports whether the device is in the product table, i.e.
// something an RF Swift user most likely wants inside a container.
//
//	in(1): USBDevice d
//	out: bool
func IsKnownRFHardware(d USBDevice) bool {
	_, ok := usbProductNames[strings.ToLower(d.VendorID)+":"+strings.ToLower(d.ProductID)]
	return ok
}
