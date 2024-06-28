/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	rfdock "penthertz/rfswift/dock"
	rfutils "penthertz/rfswift/rfutils"
)

var DImage string
var ContID string
var ExecCmd string
var FilterLast string
var ExtraBind string
var XDisplay string
var SInstall string
var ImageRef string
var ImageTag string
var ExtraHost string
var UsbDevice string
var PulseServer string

var rootCmd = &cobra.Command{
	Use:   "rfswift",
	Short: "rfswift - a simple CLI to transform and inspect strings",
	Long: `rfswift is a super fancy CLI (kidding)
   
One can use stringer to modify or inspect strings straight from the terminal`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Use '-h' for help")
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Create and run a program",
	Long:  `Create a container and run a program inside the docker container`,
	Run: func(cmd *cobra.Command, args []string) {
		os := runtime.GOOS
		if os == "windows" {
			rfdock.DockerSetx11("/run/desktop/mnt/host/wslg/.X11-unix:/tmp/.X11-unix,/run/desktop/mnt/host/wslg:/mnt/wslg")
		} else {
			rfutils.XHostEnable() // force xhost to add local connections ALCs, TODO: to optimize later
		}
		rfdock.DockerSetShell(ExecCmd)
		rfdock.DockerAddBiding(ExtraBind)
		rfdock.DockerSetImage(DImage)
		rfdock.DockerSetExtraHosts(ExtraHost)
		rfdock.DockerSetPulse(PulseServer)
		if os == "linux" { // use pactl to configure ACLs
			rfutils.SetPulseCTL(PulseServer)
		}
		rfdock.DockerRun()
	},
}

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Exec a command",
	Long:  `Exec a program on a created docker container, even not started`,
	Run: func(cmd *cobra.Command, args []string) {
		os := runtime.GOOS
		if os == "windows" {
			rfdock.DockerSetx11("/run/desktop/mnt/host/wslg/.X11-unix:/tmp/.X11-unix,/run/desktop/mnt/host/wslg:/mnt/wslg")
		} else {
			rfutils.XHostEnable() // force xhost to add local connections ALCs, TODO: to optimize later
		}
		rfdock.DockerSetShell(ExecCmd)
		rfdock.DockerExec(ContID, "/root")
	},
}

var lastCmd = &cobra.Command{
	Use:   "last",
	Short: "Last container run",
	Long:  `Display the latest container that was run`,
	Run: func(cmd *cobra.Command, args []string) {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		rfdock.DockerLast(FilterLast, labelKey, labelValue)
	},
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install function script",
	Long:  `Install function script inside the container`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DockerSetShell(ExecCmd)
		rfdock.DockerInstallFromScript(ContID)
	},
}

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit a container",
	Long:  `Commit a container with change we have made`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DockerSetImage(DImage)
		rfdock.DockerCommit(ContID)
	},
}

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull a container",
	Long:  `Pull a container from internet`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DockerPull(ImageRef, ImageTag)
	},
}

var renameCmd = &cobra.Command{
	Use:   "rename",
	Short: "Rename an image",
	Long:  `Rename an image with another tag`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DockerRename(ImageRef, ImageTag)
	},
}

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a container",
	Long:  `Remore an existing container`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DockerRemove(ContID)
	},
}

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
		rfutils.BindAndAttachDevice(UsbDevice)
	},
}

var winusbdetachCmd = &cobra.Command{
	Use:   "detach",
	Short: "Detach a bus ID",
	Long:  `Detach a bus ID from the host to containers`,
	Run: func(cmd *cobra.Command, args []string) {
		rfutils.BindAndAttachDevice(UsbDevice)
	},
}

var ImagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Show rfswift images",
	Long:  `Display images build for RF Swift`,
	Run: func(cmd *cobra.Command, args []string) {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		images_list, err := rfdock.ListImages(labelKey, labelValue)
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
		for _, image := range images_list {
			fmt.Println("ID:", image.ID)
			fmt.Println("RepoTags:", image.RepoTags)
			fmt.Println("Labels:", image.Labels)
			fmt.Println()
		}
	},
}

var DeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an rfswift images",
	Long:  `Delete an RF Swift image from image name or tag`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DeleteImage(DImage)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(lastCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(commitCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(renameCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(ImagesCmd)
	rootCmd.AddCommand(DeleteCmd)

	// Adding special commands for Windows
	os := runtime.GOOS
	if os == "windows" {
		rootCmd.AddCommand(winusbCmd)
		winusbCmd.AddCommand(winusblistCmd)
		winusbCmd.AddCommand(winusbattachCmd)
		winusbCmd.AddCommand(winusbdetachCmd)
		winusbattachCmd.Flags().StringVarP(&UsbDevice, "busid", "i", "", "busid")
		winusbdetachCmd.Flags().StringVarP(&UsbDevice, "busid", "i", "", "busid")
	}

	DeleteCmd.Flags().StringVarP(&ContID, "image", "i", "", "image ID or tag")
	removeCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to remove")
	installCmd.Flags().StringVarP(&ExecCmd, "install", "i", "", "function for installation")
	installCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
	pullCmd.Flags().StringVarP(&ImageRef, "image", "i", "", "image reference")
	pullCmd.Flags().StringVarP(&ImageTag, "tag", "t", "", "rename to target tag")
	pullCmd.MarkFlagRequired("image")
	//pullCmd.MarkFlagRequired("tag")
	renameCmd.Flags().StringVarP(&ImageRef, "image", "i", "", "image reference")
	renameCmd.Flags().StringVarP(&ImageTag, "tag", "t", "", "rename to target tag")
	commitCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
	commitCmd.Flags().StringVarP(&DImage, "image", "i", "", "image (default: 'myrfswift:latest')")
	commitCmd.MarkFlagRequired("container")
	commitCmd.MarkFlagRequired("image")
	execCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
	execCmd.Flags().StringVarP(&ExecCmd, "command", "e", "/bin/bash", "command to exec (by default: /bin/bash)")
	execCmd.Flags().StringVarP(&SInstall, "install", "i", "", "install from function script (e.g: 'sdrpp_soft_install')")
	//execCmd.MarkFlagRequired("command")
	runCmd.Flags().StringVarP(&ExtraHost, "extrahosts", "x", "", "set extra hosts (default: 'pluto.local:192.168.1.2', and separate them with commas)")
	runCmd.Flags().StringVarP(&XDisplay, "display", "d", "", "set X Display (by default: 'DISPLAY=:0', and separate them with commas)")
	runCmd.Flags().StringVarP(&ExecCmd, "command", "e", "", "command to exec (by default: '/bin/bash')")
	runCmd.Flags().StringVarP(&ExtraBind, "bind", "b", "", "extra bindings (separate them with commas)")
	runCmd.Flags().StringVarP(&DImage, "image", "i", "", "image (default: 'myrfswift:latest')")
	runCmd.Flags().StringVarP(&PulseServer, "pulseserver", "p", "tcp:127.0.0.1:34567", "PULSE SERVER TCP address (by default: tcp:127.0.0.1:34567)")
	lastCmd.Flags().StringVarP(&FilterLast, "filter", "f", "", "filter by image name")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your CLI '%s'", err)
		os.Exit(1)
	}
}
