/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*  macOS USB passthrough commands for Lima-based VMs
 */

package cli

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	rfutils "penthertz/rfswift/rfutils"
	"penthertz/rfswift/tui"
)

var limaInstance string

var macusbCmd = &cobra.Command{
	Use:   "macusb",
	Short: "Manage USB devices on macOS via Lima",
	Long: `Manage USB device passthrough to Lima VMs on macOS using QMP hot-plug.

On macOS, Docker Desktop and Podman cannot forward USB devices into containers.
Lima runs a QEMU VM with USB hot-plug support. The workflow is:

  1. rfswift macusb attach                             # interactive device picker
  2. rfswift --engine lima run -i <image>               # run container via Lima
  3. rfswift macusb detach                              # interactive detach

Or with explicit IDs:
  rfswift macusb attach --vid 0x1d50 --pid 0x604b
  rfswift macusb detach --vid 0x1d50 --pid 0x604b

Use --engine lima to route container commands through the Lima VM where USB
devices are visible. Without --engine lima, containers run in Docker Desktop
which has no USB access.`,
}

var macusbListCmd = &cobra.Command{
	Use:   "list",
	Short: "List USB devices on macOS host",
	Long:  `Lists all USB devices connected to the macOS host using system_profiler`,
	Run: func(cmd *cobra.Command, args []string) {
		devices, err := rfutils.ListMacUSBDevices()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		if len(devices) == 0 {
			fmt.Println("No USB devices found.")
			return
		}

		fmt.Println("USB Devices on macOS host:")
		fmt.Printf("%-30s %-12s %-12s %-20s\n", "NAME", "VENDOR ID", "PRODUCT ID", "SERIAL")
		fmt.Println("---------------------------------------------------------------------")
		for _, device := range devices {
			fmt.Printf("%-30s %-12s %-12s %-20s\n",
				truncate(device.Name, 29),
				device.VendorID,
				device.ProductID,
				device.Serial)
		}

		fmt.Printf("\nTo attach: rfswift macusb attach  (interactive picker)\n")
		fmt.Printf("       or: rfswift macusb attach --vid <vendor_id> --pid <product_id>\n")
	},
}

var macusbAttachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Attach USB device(s) to the Lima VM",
	Long: `Hot-plugs USB device(s) into the Lima VM via QEMU QMP protocol.
When run without --vid/--pid flags in an interactive terminal, shows a
device picker to select one or more devices to attach.`,
	Run: func(cmd *cobra.Command, args []string) {
		vendorID, _ := cmd.Flags().GetString("vid")
		productID, _ := cmd.Flags().GetString("pid")

		// If no flags provided, launch interactive picker
		if vendorID == "" && productID == "" {
			if !tui.IsInteractive() {
				fmt.Println("Error: both --vid and --pid are required in non-interactive mode")
				fmt.Println("Use 'rfswift macusb list' to find device IDs")
				return
			}
			pickedDevices := pickMacUSBDevices("Select USB device(s) to attach to Lima VM")
			if len(pickedDevices) == 0 {
				fmt.Println("No devices selected.")
				return
			}
			for _, dev := range pickedDevices {
				if err := rfutils.AttachUSBToLima(dev.VendorID, dev.ProductID, limaInstance); err != nil {
					fmt.Printf("Error attaching %s (%s:%s): %v\n", dev.Name, dev.VendorID, dev.ProductID, err)
				}
			}
			fmt.Println("")
			fmt.Println("To use these devices in a container, run with the Lima engine:")
			fmt.Println("  rfswift --engine lima run -i <image>")
			return
		}

		if vendorID == "" || productID == "" {
			fmt.Println("Error: both --vid and --pid are required")
			fmt.Println("Use 'rfswift macusb attach' without flags for interactive picker")
			return
		}

		vendorID = ensureHexPrefix(vendorID)
		productID = ensureHexPrefix(productID)

		if err := rfutils.AttachUSBToLima(vendorID, productID, limaInstance); err != nil {
			fmt.Println("Error:", err)
			return
		}

		fmt.Println("")
		fmt.Println("To use this device in a container, run with the Lima engine:")
		fmt.Println("  rfswift --engine lima run -i <image>")
	},
}

var macusbDetachCmd = &cobra.Command{
	Use:   "detach",
	Short: "Detach USB device(s) from the Lima VM",
	Long: `Hot-unplugs USB device(s) from the Lima VM via QEMU QMP protocol.
When run without flags in an interactive terminal, shows a device picker.`,
	Run: func(cmd *cobra.Command, args []string) {
		vendorID, _ := cmd.Flags().GetString("vid")
		productID, _ := cmd.Flags().GetString("pid")
		devID, _ := cmd.Flags().GetString("devid")

		// If no flags at all, launch interactive picker
		if vendorID == "" && productID == "" && devID == "" {
			if !tui.IsInteractive() {
				fmt.Println("Error: provide --devid or both --vid and --pid in non-interactive mode")
				return
			}
			pickedDevices := pickMacUSBDevices("Select USB device(s) to detach from Lima VM")
			if len(pickedDevices) == 0 {
				fmt.Println("No devices selected.")
				return
			}
			for _, dev := range pickedDevices {
				if err := rfutils.DetachUSBFromLima(dev.VendorID, dev.ProductID, limaInstance); err != nil {
					fmt.Printf("Error detaching %s (%s:%s): %v\n", dev.Name, dev.VendorID, dev.ProductID, err)
				}
			}
			return
		}

		if devID != "" {
			if err := rfutils.DetachUSBByIDFromLima(devID, limaInstance); err != nil {
				fmt.Println("Error:", err)
			}
			return
		}

		if vendorID == "" || productID == "" {
			fmt.Println("Error: provide either --devid or both --vid and --pid")
			fmt.Println("Use 'rfswift macusb detach' without flags for interactive picker")
			return
		}

		vendorID = ensureHexPrefix(vendorID)
		productID = ensureHexPrefix(productID)

		if err := rfutils.DetachUSBFromLima(vendorID, productID, limaInstance); err != nil {
			fmt.Println("Error:", err)
		}
	},
}

var macusbVMDevicesCmd = &cobra.Command{
	Use:   "vm-devices",
	Short: "List USB devices attached to the Lima VM",
	Long:  `Shows USB devices currently forwarded into the Lima QEMU VM`,
	Run: func(cmd *cobra.Command, args []string) {
		result, err := rfutils.ListUSBInLimaVM(limaInstance)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		if result == "" {
			fmt.Println("No USB devices attached to VM.")
		} else {
			fmt.Println("USB devices in Lima VM:")
			fmt.Println(result)
		}
	},
}

var macusbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Lima VM status for USB passthrough",
	Long:  `Verifies that Lima is installed, the instance is running, and QMP is available`,
	Run: func(cmd *cobra.Command, args []string) {
		if !rfutils.IsLimaInstalled() {
			fmt.Println("[!] Lima is NOT installed.")
			fmt.Println("    Install with: brew install lima qemu")
			return
		}
		fmt.Println("[+] Lima is installed")

		if !rfutils.IsQEMUInstalled() {
			fmt.Println("[!] QEMU is NOT installed (required by Lima for USB passthrough).")
			fmt.Println("    Install with: brew install qemu")
			return
		}
		fmt.Println("[+] QEMU is installed")

		if rfutils.IsLimaInstanceRunning(limaInstance) {
			fmt.Printf("[+] Lima instance '%s' is running\n", limaInstance)
		} else {
			fmt.Printf("[!] Lima instance '%s' is NOT running\n", limaInstance)
			fmt.Println("    Note: rfswift will auto-create/start the VM when you run a container command.")
			fmt.Printf("    Or start manually: limactl start %s\n", limaInstance)
			return
		}

		sockPath, err := rfutils.FindLimaQMPSocket(limaInstance)
		if err != nil {
			fmt.Println("[!] QMP socket not found - USB passthrough requires vmType: qemu")
			fmt.Println("    Make sure your Lima config uses 'vmType: qemu' (not 'vz')")
		} else {
			fmt.Printf("[+] QMP socket found: %s\n", sockPath)
		}

		result, err := rfutils.ListUSBInLimaVM(limaInstance)
		if err == nil && result != "" {
			fmt.Println("[+] USB devices currently attached to VM:")
			fmt.Println("    " + result)
		}
	},
}

func registerMacUSBCommands() {
	rootCmd.AddCommand(macusbCmd)
	macusbCmd.AddCommand(macusbListCmd)
	macusbCmd.AddCommand(macusbAttachCmd)
	macusbCmd.AddCommand(macusbDetachCmd)
	macusbCmd.AddCommand(macusbVMDevicesCmd)
	macusbCmd.AddCommand(macusbStatusCmd)

	// Global flag for Lima instance name
	macusbCmd.PersistentFlags().StringVar(&limaInstance, "instance", "rfswift", "Lima instance name")

	// Attach flags
	macusbAttachCmd.Flags().String("vid", "", "USB Vendor ID (hex, e.g., 0x1234)")
	macusbAttachCmd.Flags().String("pid", "", "USB Product ID (hex, e.g., 0x5678)")

	// Detach flags
	macusbDetachCmd.Flags().String("vid", "", "USB Vendor ID (hex, e.g., 0x1234)")
	macusbDetachCmd.Flags().String("pid", "", "USB Product ID (hex, e.g., 0x5678)")
	macusbDetachCmd.Flags().String("devid", "", "QMP device ID (e.g., usb-1234-5678)")
}

// pickMacUSBDevices shows an interactive multi-select of discovered USB devices.
// Returns the selected devices, or empty slice if cancelled/none selected.
func pickMacUSBDevices(title string) []rfutils.MacUSBDevice {
	devices, err := rfutils.ListMacUSBDevices()
	if err != nil {
		fmt.Println("Error listing USB devices:", err)
		return nil
	}

	if len(devices) == 0 {
		fmt.Println("No USB devices found on host.")
		return nil
	}

	// Build huh options from discovered devices
	options := make([]huh.Option[string], 0, len(devices))
	for _, dev := range devices {
		label := fmt.Sprintf("%s  [%s:%s]", dev.Name, dev.VendorID, dev.ProductID)
		if dev.Serial != "" {
			label += fmt.Sprintf("  S/N: %s", dev.Serial)
		}
		value := dev.VendorID + ":" + dev.ProductID
		options = append(options, huh.NewOption(label, value))
	}

	var selected []string
	err = huh.NewMultiSelect[string]().
		Title(title).
		Description("Use space to select, enter to confirm").
		Options(options...).
		Value(&selected).
		Run()
	if err != nil || len(selected) == 0 {
		return nil
	}

	// Map selections back to device structs
	var result []rfutils.MacUSBDevice
	for _, sel := range selected {
		parts := strings.SplitN(sel, ":", 2)
		if len(parts) != 2 {
			continue
		}
		for _, dev := range devices {
			if dev.VendorID == parts[0] && dev.ProductID == parts[1] {
				result = append(result, dev)
				break
			}
		}
	}
	return result
}

// MacUSBWizardStep runs an interactive USB device picker for the run wizard
// on macOS when using the Lima engine. Returns comma-separated vid:pid pairs
// that were attached, or empty string if skipped.
func MacUSBWizardStep(instance string) string {
	if runtime.GOOS != "darwin" || !rfutils.IsLimaInstalled() {
		return ""
	}

	attachUSB := false
	err := huh.NewConfirm().
		Title("Attach USB devices to Lima VM?").
		Description("Forward USB hardware (SDR dongles, etc.) into the VM for container access").
		Affirmative("Yes").
		Negative("No").
		Value(&attachUSB).
		Run()
	if err != nil || !attachUSB {
		return ""
	}

	pickedDevices := pickMacUSBDevices("Select USB device(s) to attach")
	if len(pickedDevices) == 0 {
		return ""
	}

	// Attach each selected device
	var attached []string
	for _, dev := range pickedDevices {
		if err := rfutils.AttachUSBToLima(dev.VendorID, dev.ProductID, instance); err != nil {
			fmt.Printf("  Warning: failed to attach %s (%s:%s): %v\n", dev.Name, dev.VendorID, dev.ProductID, err)
		} else {
			attached = append(attached, fmt.Sprintf("%s:%s", dev.VendorID, dev.ProductID))
		}
	}

	return strings.Join(attached, ",")
}

// ensureHexPrefix adds "0x" prefix if not present
func ensureHexPrefix(id string) string {
	if len(id) > 0 && id[0] != '0' {
		return "0x" + id
	}
	if len(id) >= 2 && id[:2] != "0x" {
		return "0x" + id
	}
	return id
}

// truncate shortens a string to maxLen, appending "..." if truncated
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
