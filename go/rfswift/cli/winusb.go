/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	rfutils "penthertz/rfswift/rfutils"
)

var winusbCmd = &cobra.Command{
	Use:   "winusb",
	Short: "Manage WinUSB devices",
}

var winusblistCmd = &cobra.Command{
	Use:   "list",
	Short: "List bus IDs",
	Long:  `Lists all USB device connecter to the Windows host`,
	Run: func(cmd *cobra.Command, args []string) {
		devices, err := rfutils.ListUSBDevices()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		fmt.Println("USB Devices:")
		for _, device := range devices {
			fmt.Printf("BusID: %s, DeviceID: %s, VendorID: %s, ProductID: %s, Description: %s\n",
				device.BusID, device.DeviceID, device.VendorID, device.ProductID, device.Description)
		}
	},
}

var winusbattachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Attach a bus ID",
	Long:  `Attach a bus ID from the host to containers`,
	Run: func(cmd *cobra.Command, args []string) {
		usbDevice, _ := cmd.Flags().GetString("busid")
		rfutils.BindAndAttachDevice(usbDevice)
	},
}

var winusbdetachCmd = &cobra.Command{
	Use:   "detach",
	Short: "Detach a bus ID",
	Long:  `Detach a bus ID from the host to containers`,
	Run: func(cmd *cobra.Command, args []string) {
		usbDevice, _ := cmd.Flags().GetString("busid")
		rfutils.BindAndAttachDevice(usbDevice)
	},
}

func registerWinUSBCommands() {
	rootCmd.AddCommand(winusbCmd)
	winusbCmd.AddCommand(winusblistCmd)
	winusbCmd.AddCommand(winusbattachCmd)
	winusbCmd.AddCommand(winusbdetachCmd)
	winusbattachCmd.Flags().StringP("busid", "i", "", "busid")
	winusbdetachCmd.Flags().StringP("busid", "i", "", "busid")
}
