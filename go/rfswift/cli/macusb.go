/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*  macOS USB passthrough commands for Lima-based VMs
 */

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	rfutils "penthertz/rfswift/rfutils"
)

var limaInstance string

var macusbCmd = &cobra.Command{
	Use:   "macusb",
	Short: "Manage USB devices on macOS via Lima",
	Long: `Manage USB device passthrough to Lima VMs on macOS using QMP hot-plug.

On macOS, Docker Desktop and Podman cannot forward USB devices into containers.
Lima runs a QEMU VM with USB hot-plug support. The workflow is:

  1. rfswift macusb list                              # see host USB devices
  2. rfswift macusb attach --vid 0x1d50 --pid 0x604b  # forward device to Lima VM
  3. rfswift --engine lima run -i <image>              # run container via Lima
  4. rfswift macusb detach --vid 0x1d50 --pid 0x604b  # unplug when done

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

		fmt.Printf("\nTo attach a device: rfswift macusb attach --vid <vendor_id> --pid <product_id>\n")
	},
}

var macusbAttachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Attach a USB device to the Lima VM",
	Long:  `Hot-plugs a USB device into the Lima VM via QEMU QMP protocol`,
	Run: func(cmd *cobra.Command, args []string) {
		vendorID, _ := cmd.Flags().GetString("vid")
		productID, _ := cmd.Flags().GetString("pid")

		if vendorID == "" || productID == "" {
			fmt.Println("Error: both --vid and --pid are required")
			fmt.Println("Use 'rfswift macusb list' to find device IDs")
			return
		}

		// Ensure hex prefix
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
	Short: "Detach a USB device from the Lima VM",
	Long:  `Hot-unplugs a USB device from the Lima VM via QEMU QMP protocol`,
	Run: func(cmd *cobra.Command, args []string) {
		vendorID, _ := cmd.Flags().GetString("vid")
		productID, _ := cmd.Flags().GetString("pid")
		devID, _ := cmd.Flags().GetString("devid")

		if devID != "" {
			if err := rfutils.DetachUSBByIDFromLima(devID, limaInstance); err != nil {
				fmt.Println("Error:", err)
			}
			return
		}

		if vendorID == "" || productID == "" {
			fmt.Println("Error: provide either --devid or both --vid and --pid")
			fmt.Println("Use 'rfswift macusb vm-devices' to list attached devices")
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
		// Check Lima installation
		if !rfutils.IsLimaInstalled() {
			fmt.Println("[!] Lima is NOT installed.")
			fmt.Println("    Install with: brew install lima")
			return
		}
		fmt.Println("[+] Lima is installed")

		// Check instance status
		if rfutils.IsLimaInstanceRunning(limaInstance) {
			fmt.Printf("[+] Lima instance '%s' is running\n", limaInstance)
		} else {
			fmt.Printf("[!] Lima instance '%s' is NOT running\n", limaInstance)
			fmt.Println("    Note: rfswift will auto-create/start the VM when you run a container command.")
			fmt.Printf("    Or start manually: limactl start %s\n", limaInstance)
			return
		}

		// Check QMP socket
		sockPath, err := rfutils.FindLimaQMPSocket(limaInstance)
		if err != nil {
			fmt.Println("[!] QMP socket not found - USB passthrough requires vmType: qemu")
			fmt.Println("    Make sure your Lima config uses 'vmType: qemu' (not 'vz')")
		} else {
			fmt.Printf("[+] QMP socket found: %s\n", sockPath)
		}

		// Show USB devices currently in VM
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
