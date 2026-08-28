/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*  Windows USB passthrough commands: usbipd-win -> WSL 2 -> containers
 */

package cli

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	huh "charm.land/huh/v2"
	"github.com/spf13/cobra"
	rfutils "penthertz/rfswift/rfutils"
	"penthertz/rfswift/tui"
)

var winusbCmd = &cobra.Command{
	Use:   "winusb",
	Short: "Manage USB passthrough on Windows (usbipd -> WSL 2)",
	Long: `Forward host USB devices into the WSL 2 VM that Docker Desktop and Podman
run in, so RF Swift containers can use them. Forwarded devices appear under
/dev/bus/usb, which the default RF Swift device list already binds.

Requires usbipd-win (winget install usbipd). Who needs what:

  share   (bind)    once per device, administrator  -> RF Swift asks via UAC
  attach / detach   every time, any user            -> no elevation needed
  unshare (unbind)  administrator                   -> RF Swift asks via UAC

Typical workflow:
  1. rfswift usb attach                 # picker; shares the device on first use
  2. rfswift run -i sdr_light -n lab    # the container sees /dev/bus/usb
  3. rfswift usb detach                 # give the device back to Windows`,
}

var winusblistCmd = &cobra.Command{
	Use:   "list",
	Short: "List host USB devices and their usbipd state",
	Long:  `Lists the USB devices connected to the Windows host, plus devices that are shared but currently unplugged, with their usbipd state.`,
	Run: func(cmd *cobra.Command, args []string) {
		devices, err := rfutils.ListUSBDevices()
		if err != nil {
			printWinUSBError(err)
			return
		}
		printWinUSBTable(devices)
		fmt.Println("")
		fmt.Println("To attach: rfswift usb attach  (interactive picker)")
		fmt.Println("       or: rfswift usb attach --busid <BUSID>")
	},
}

var winusbattachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Attach a USB device to WSL 2 so containers can use it",
	Long: `Forwards a host USB device into the WSL 2 VM shared by Docker Desktop and
Podman. A device that was never shared is bound first through a Windows UAC
prompt (administrator approval for usbipd.exe, once per device); attaching and
detaching afterwards never need elevation.

Without --busid in an interactive terminal, a device picker is shown.`,
	Run: func(cmd *cobra.Command, args []string) {
		busID, _ := cmd.Flags().GetString("busid")
		yes, _ := cmd.Flags().GetBool("yes")
		var targets []rfutils.USBDevice
		if busID == "" {
			if !tui.IsInteractive() {
				fmt.Println("Error: --busid is required in non-interactive mode")
				fmt.Println("Use 'rfswift usb list' to find bus IDs")
				return
			}
			targets = pickWinUSBDevices("Select USB device(s) to attach to WSL 2", func(d rfutils.USBDevice) bool {
				return d.Connected && !d.Attached
			})
			if len(targets) == 0 {
				fmt.Println("No devices selected.")
				return
			}
		} else {
			dev, err := rfutils.FindUSBDevice(busID)
			if err != nil {
				printWinUSBError(err)
				return
			}
			targets = []rfutils.USBDevice{dev}
		}
		attachWinUSBDevices(targets, yes)
	},
}

var winusbdetachCmd = &cobra.Command{
	Use:   "detach",
	Short: "Detach a USB device from WSL 2 (give it back to Windows)",
	Long: `Returns a forwarded device to Windows. The device stays shared, so the next
attach needs no administrator approval. Without --busid in an interactive
terminal, a picker lists the attached devices.`,
	Run: func(cmd *cobra.Command, args []string) {
		busID, _ := cmd.Flags().GetString("busid")
		var targets []rfutils.USBDevice
		if busID == "" {
			if !tui.IsInteractive() {
				fmt.Println("Error: --busid is required in non-interactive mode")
				return
			}
			targets = pickWinUSBDevices("Select USB device(s) to detach from WSL 2", func(d rfutils.USBDevice) bool {
				return d.Attached
			})
			if len(targets) == 0 {
				fmt.Println("No devices selected.")
				return
			}
		} else {
			dev, err := rfutils.FindUSBDevice(busID)
			if err != nil {
				printWinUSBError(err)
				return
			}
			targets = []rfutils.USBDevice{dev}
		}
		for _, dev := range targets {
			if !dev.Attached {
				fmt.Printf("  %s (bus %s) is not attached to WSL 2 (state: %s)\n", dev.Name, dev.BusID, dev.State())
				continue
			}
			if err := rfutils.DetachUSBDevice(dev.BusID); err != nil {
				printWinUSBError(err)
				continue
			}
			fmt.Printf("[+] Detached %s (bus %s); it is available to Windows again\n", dev.Name, dev.BusID)
		}
	},
}

var winusbbindCmd = &cobra.Command{
	Use:     "bind",
	Aliases: []string{"share"},
	Short:   "Share a device (one-time administrator approval)",
	Long: `Registers a device with usbipd so it can be attached to WSL 2 later without
administrator rights. This is the only step that needs elevation; RF Swift
requests it through a UAC prompt for usbipd.exe. 'rfswift usb attach' does
this automatically for devices that were never shared.`,
	Run: func(cmd *cobra.Command, args []string) {
		busID, _ := cmd.Flags().GetString("busid")
		yes, _ := cmd.Flags().GetBool("yes")
		var targets []rfutils.USBDevice
		if busID == "" {
			if !tui.IsInteractive() {
				fmt.Println("Error: --busid is required in non-interactive mode")
				return
			}
			targets = pickWinUSBDevices("Select USB device(s) to share", func(d rfutils.USBDevice) bool {
				return d.Connected && !d.Shared
			})
			if len(targets) == 0 {
				fmt.Println("No devices selected.")
				return
			}
		} else {
			dev, err := rfutils.FindUSBDevice(busID)
			if err != nil {
				printWinUSBError(err)
				return
			}
			targets = []rfutils.USBDevice{dev}
		}
		for _, dev := range targets {
			if dev.Shared {
				fmt.Printf("  %s (bus %s) is already shared\n", dev.Name, dev.BusID)
				continue
			}
			if !confirmWinUSBShare(dev, yes) {
				fmt.Printf("  Skipped %s (bus %s)\n", dev.Name, dev.BusID)
				continue
			}
			elevated, err := rfutils.ShareUSBDevice(dev.BusID, true)
			if err != nil {
				printWinUSBError(err)
				continue
			}
			fmt.Printf("[+] Shared %s (bus %s)%s\n", dev.Name, dev.BusID, elevationNote(elevated))
		}
	},
}

var winusbunbindCmd = &cobra.Command{
	Use:     "unbind",
	Aliases: []string{"unshare"},
	Short:   "Stop sharing a device (administrator approval)",
	Long: `Unregisters a device from usbipd and returns it to Windows for good. Use
--busid for a connected device or --guid (from 'rfswift usb list') for a
shared device that is currently unplugged.`,
	Run: func(cmd *cobra.Command, args []string) {
		busID, _ := cmd.Flags().GetString("busid")
		guid, _ := cmd.Flags().GetString("guid")
		yes, _ := cmd.Flags().GetBool("yes")
		type target struct {
			ref  string
			name string
		}
		var targets []target
		switch {
		case guid != "":
			targets = append(targets, target{ref: strings.TrimSpace(guid), name: "persisted device " + strings.TrimSpace(guid)})
		case busID != "":
			dev, err := rfutils.FindUSBDevice(busID)
			if err != nil {
				printWinUSBError(err)
				return
			}
			targets = append(targets, target{ref: dev.BusID, name: dev.Name})
		default:
			if !tui.IsInteractive() {
				fmt.Println("Error: --busid or --guid is required in non-interactive mode")
				return
			}
			for _, dev := range pickWinUSBDevices("Select USB device(s) to stop sharing", func(d rfutils.USBDevice) bool {
				return d.Connected && d.Shared
			}) {
				targets = append(targets, target{ref: dev.BusID, name: dev.Name})
			}
			if len(targets) == 0 {
				fmt.Println("No devices selected.")
				return
			}
		}
		for _, t := range targets {
			if !yes && tui.IsInteractive() {
				ok := false
				err := huh.NewConfirm().
					Title(fmt.Sprintf("Stop sharing %s?", t.name)).
					Description("Windows will show a UAC prompt for usbipd.exe (unbind). Sharing it again later needs another approval.").
					Affirmative("Unshare").Negative("Cancel").
					Value(&ok).Run()
				if err != nil || !ok {
					fmt.Printf("  Skipped %s\n", t.name)
					continue
				}
			}
			elevated, err := rfutils.UnshareUSBDevice(t.ref, true)
			if err != nil {
				printWinUSBError(err)
				continue
			}
			fmt.Printf("[+] Stopped sharing %s%s\n", t.name, elevationNote(elevated))
		}
	},
}

var winusbstatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check usbipd-win, WSL 2 and shared devices",
	Long:  `Verifies that usbipd-win is installed, that a WSL 2 distribution exists for it to attach devices to, and summarises the shared/attached devices.`,
	Run: func(cmd *cobra.Command, args []string) {
		path, err := rfutils.UsbipdPath()
		if err != nil {
			fmt.Println("[!] usbipd-win is NOT installed.")
			fmt.Println("    Install with: winget install usbipd   (https://github.com/dorssel/usbipd-win)")
			return
		}
		version, verr := rfutils.UsbipdVersion()
		if verr != nil {
			fmt.Printf("[!] usbipd found at %s but not working: %v\n", path, verr)
			return
		}
		fmt.Printf("[+] usbipd-win %s (%s)\n", version, path)

		wsl, werr := rfutils.WSLDistributions()
		switch {
		case werr != nil:
			fmt.Printf("[!] WSL: %v\n", werr)
		case !wsl.HasWSL2Distribution():
			fmt.Println("[!] No WSL 2 distribution installed - usbipd attaches devices to the default one")
			fmt.Println("    Install one with: wsl --install -d Ubuntu")
		default:
			def := wsl.DefaultDistro
			if def == "" {
				def = "(none set: wsl --set-default <name>)"
			}
			fmt.Printf("[+] WSL 2 default distribution: %s\n", def)
			for _, d := range wsl.Distros {
				mark := ""
				if d.Default {
					mark = " (default)"
				}
				fmt.Printf("    %-24s v%d %s%s\n", d.Name, d.Version, d.State, mark)
			}
		}

		devices, err := rfutils.ListUSBDevices()
		if err != nil {
			printWinUSBError(err)
			return
		}
		var connected, shared, attached int
		for _, d := range devices {
			if d.Connected {
				connected++
			}
			if d.Shared {
				shared++
			}
			if d.Attached {
				attached++
			}
		}
		fmt.Printf("[+] %d device(s) connected, %d shared, %d attached to WSL 2\n", connected, shared, attached)
		fmt.Println("    Administrator rights: never for attach/detach; once per device to share it (RF Swift asks via UAC).")
		fmt.Println("    Details: rfswift usb list")
	},
}

var winusbvmdevicesCmd = &cobra.Command{
	Use:     "vm-devices",
	Aliases: []string{"wsl-devices"},
	Short:   "List USB devices as seen inside WSL 2",
	Long:    `Shows what the WSL 2 kernel (shared by Docker Desktop and Podman) exposes under /dev/bus/usb. Starts the default distribution if needed.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := rfutils.WSLUSBView()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		if out == "" {
			fmt.Println("No USB devices visible in WSL 2 (/dev/bus/usb is empty).")
			return
		}
		fmt.Println("USB devices in WSL 2:")
		fmt.Println(out)
	},
}

func registerWinUSBCommands() {
	rootCmd.AddCommand(winusbCmd)
	winusbCmd.AddCommand(winusblistCmd)
	winusbCmd.AddCommand(winusbattachCmd)
	winusbCmd.AddCommand(winusbdetachCmd)
	winusbCmd.AddCommand(winusbbindCmd)
	winusbCmd.AddCommand(winusbunbindCmd)
	winusbCmd.AddCommand(winusbstatusCmd)
	winusbCmd.AddCommand(winusbvmdevicesCmd)

	// "-i" is kept for backward compatibility with earlier releases.
	winusbattachCmd.Flags().StringP("busid", "i", "", "bus ID from 'rfswift usb list' (e.g. 2-3)")
	winusbattachCmd.Flags().BoolP("yes", "y", false, "allow the administrator (UAC) prompt without asking, needed in non-interactive mode")
	winusbdetachCmd.Flags().StringP("busid", "i", "", "bus ID from 'rfswift usb list' (e.g. 2-3)")
	winusbbindCmd.Flags().StringP("busid", "i", "", "bus ID from 'rfswift usb list' (e.g. 2-3)")
	winusbbindCmd.Flags().BoolP("yes", "y", false, "allow the administrator (UAC) prompt without asking")
	winusbunbindCmd.Flags().StringP("busid", "i", "", "bus ID of a connected device")
	winusbunbindCmd.Flags().String("guid", "", "usbipd GUID of a shared device that is currently unplugged")
	winusbunbindCmd.Flags().BoolP("yes", "y", false, "allow the administrator (UAC) prompt without asking")
}

// printWinUSBTable prints devices in the same spirit as 'usbipd list', with
// RF Swift's friendly names and a single STATE column.
func printWinUSBTable(devices []rfutils.USBDevice) {
	if len(devices) == 0 {
		fmt.Println("No USB devices reported by usbipd.")
		return
	}
	fmt.Println("USB devices on the Windows host:")
	fmt.Printf("%-7s %-10s %-11s %s\n", "BUSID", "VID:PID", "STATE", "DEVICE")
	fmt.Println(strings.Repeat("-", 76))
	for _, d := range devices {
		bus := d.BusID
		if bus == "" {
			bus = "-"
		}
		hw := "-"
		if d.VendorID != "" && d.ProductID != "" {
			hw = d.HardwareID()
		}
		name := d.Name
		if d.Description != "" && d.Description != d.Name {
			name += " (" + d.Description + ")"
		}
		if d.Forced {
			name += " [forced]"
		}
		if !d.Connected && d.PersistedGUID != "" {
			name += "  guid " + d.PersistedGUID
		}
		fmt.Printf("%-7s %-10s %-11s %s\n", bus, hw, d.State(), name)
	}
}

// printWinUSBError prints an error and the most useful next step.
func printWinUSBError(err error) {
	fmt.Println("Error:", err)
	switch {
	case errors.Is(err, rfutils.ErrUsbipdNotInstalled):
		fmt.Println("  Install usbipd-win: winget install usbipd   (https://github.com/dorssel/usbipd-win)")
	case errors.Is(err, rfutils.ErrUSBNotShared), errors.Is(err, rfutils.ErrUSBAdminRequired):
		fmt.Println("  Sharing a device needs administrator rights once. Run 'rfswift usb attach' (RF Swift asks")
		fmt.Println("  through a UAC prompt) or, from an administrator terminal: usbipd bind --busid <BUSID>")
	case errors.Is(err, rfutils.ErrUSBElevationDeclined):
		fmt.Println("  The administrator prompt was cancelled; the device was left unchanged.")
	case errors.Is(err, rfutils.ErrUSBNotConnected):
		fmt.Println("  Plug the device in and run 'rfswift usb list' again.")
	}
}

func elevationNote(elevated bool) string {
	if elevated {
		return " (administrator approval granted)"
	}
	return ""
}

// confirmWinUSBShare asks before raising a UAC prompt for a never-shared
// device. Non-interactive callers must pass --yes so a script never pops a
// consent dialog by surprise.
func confirmWinUSBShare(dev rfutils.USBDevice, assumeYes bool) bool {
	if assumeYes {
		return true
	}
	if !tui.IsInteractive() {
		fmt.Printf("  %s (bus %s) is not shared yet; sharing needs administrator approval. Re-run with --yes to allow the UAC prompt.\n", dev.Name, dev.BusID)
		return false
	}
	ok := false
	err := huh.NewConfirm().
		Title(fmt.Sprintf("Share %s (bus %s) with administrator approval?", dev.Name, dev.BusID)).
		Description("Windows will show a UAC prompt for usbipd.exe (bind --busid " + dev.BusID + "). Needed once per device; attach/detach never need it.").
		Affirmative("Share").Negative("Skip").
		Value(&ok).Run()
	return err == nil && ok
}

// confirmWinUSBInputDevice warns before forwarding a keyboard/mouse-like
// device, which Windows loses while it is attached to WSL 2.
func confirmWinUSBInputDevice(dev rfutils.USBDevice, assumeYes bool) bool {
	if assumeYes || !rfutils.IsUSBInputDevice(dev) || !tui.IsInteractive() {
		return true
	}
	ok := false
	err := huh.NewConfirm().
		Title(fmt.Sprintf("%s looks like an input device", dev.Name)).
		Description("Windows cannot use it while it is attached to WSL 2. Attach anyway?").
		Affirmative("Attach").Negative("Skip").
		Value(&ok).Run()
	return err == nil && ok
}

// attachWinUSBDevices shares (with consent) and attaches each device, then
// prints how containers reach it.
//
//	in(1): []rfutils.USBDevice devices to attach
//	in(2): bool assumeYes skip the consent questions (--yes)
//	out: []rfutils.USBDevice the devices that ended up attached
func attachWinUSBDevices(devices []rfutils.USBDevice, assumeYes bool) []rfutils.USBDevice {
	var attached []rfutils.USBDevice
	for _, dev := range devices {
		if dev.Attached {
			fmt.Printf("  %s (bus %s) is already attached to WSL 2\n", dev.Name, dev.BusID)
			attached = append(attached, dev)
			continue
		}
		if !dev.Connected {
			fmt.Printf("  %s is shared but not plugged in\n", dev.Name)
			continue
		}
		if !confirmWinUSBInputDevice(dev, assumeYes) {
			fmt.Printf("  Skipped %s (bus %s)\n", dev.Name, dev.BusID)
			continue
		}
		allowElevation := dev.Shared
		if !dev.Shared {
			allowElevation = confirmWinUSBShare(dev, assumeYes)
			if !allowElevation {
				fmt.Printf("  Skipped %s (bus %s): not shared\n", dev.Name, dev.BusID)
				continue
			}
		}
		res, err := rfutils.EnsureUSBDeviceAttached(dev.BusID, allowElevation)
		if err != nil {
			printWinUSBError(err)
			continue
		}
		if res.Bound {
			fmt.Printf("[+] Shared %s (bus %s)%s\n", res.Device.Name, res.Device.BusID, elevationNote(res.Elevated))
		}
		fmt.Printf("[+] Attached %s (bus %s) to WSL 2\n", res.Device.Name, res.Device.BusID)
		attached = append(attached, res.Device)
	}
	if len(attached) > 0 {
		fmt.Println("")
		fmt.Println("Containers reach the device through /dev/bus/usb (bound by the default RF Swift device list).")
		fmt.Println("  New container:      rfswift run -i <image> -n <name>")
		fmt.Println("  Existing container: it sees the device right away (hot-plug)")
		fmt.Println("  Give it back:       rfswift usb detach --busid <BUSID>")
	}
	return attached
}

// pickWinUSBDevices lists host devices, keeps those matching keep and shows a
// multi-select picker.
func pickWinUSBDevices(title string, keep func(rfutils.USBDevice) bool) []rfutils.USBDevice {
	devices, err := rfutils.ListUSBDevices()
	if err != nil {
		printWinUSBError(err)
		return nil
	}
	var candidates []rfutils.USBDevice
	for _, d := range devices {
		if keep == nil || keep(d) {
			candidates = append(candidates, d)
		}
	}
	if len(candidates) == 0 {
		fmt.Println("No matching USB devices found. Plug the device in and run 'rfswift usb list'.")
		return nil
	}
	return pickFromWinUSBDevices(title, candidates)
}

// pickFromWinUSBDevices shows a multi-select over the given devices; values
// are bus IDs. Returns the selection, or nil when cancelled/empty.
func pickFromWinUSBDevices(title string, candidates []rfutils.USBDevice) []rfutils.USBDevice {
	options := make([]huh.Option[string], 0, len(candidates))
	for _, d := range candidates {
		label := fmt.Sprintf("%s  [%s]  bus %s  %s", d.Name, d.HardwareID(), d.BusID, d.State())
		if rfutils.IsUSBInputDevice(d) {
			label += "  (input device: Windows loses it while attached)"
		}
		options = append(options, huh.NewOption(label, d.BusID))
	}
	var selected []string
	err := huh.NewMultiSelect[string]().
		Title(title).
		Description("Use space to select, enter to confirm").
		Options(options...).
		Value(&selected).
		Run()
	if err != nil || len(selected) == 0 {
		return nil
	}
	var result []rfutils.USBDevice
	for _, bus := range selected {
		for _, d := range candidates {
			if d.BusID == bus {
				result = append(result, d)
				break
			}
		}
	}
	return result
}

// WinUSBWizardStep offers to forward USB hardware into WSL 2 before a
// container is created on Windows. It only speaks up when there is something
// worth forwarding - a connected device that is already shared, or known RF
// hardware - so plain keyboards and webcams never trigger a question.
func WinUSBWizardStep() {
	if runtime.GOOS != "windows" || !tui.IsInteractive() || !rfutils.IsUsbipdInstalled() {
		return
	}
	devices, err := rfutils.ListUSBDevices()
	if err != nil {
		return
	}
	var candidates []rfutils.USBDevice
	var names []string
	for _, d := range devices {
		if d.Connected && !d.Attached && (d.Shared || rfutils.IsKnownRFHardware(d)) && !rfutils.IsUSBInputDevice(d) {
			candidates = append(candidates, d)
			names = append(names, d.Name)
		}
	}
	if len(candidates) == 0 {
		return
	}
	attach := false
	err = huh.NewConfirm().
		Title("Attach USB devices to WSL 2 for this container?").
		Description("Detected: " + strings.Join(names, ", ") + ". Forwarded devices appear under /dev/bus/usb in the container.").
		Affirmative("Yes").
		Negative("No").
		Value(&attach).
		Run()
	if err != nil || !attach {
		return
	}
	picked := pickFromWinUSBDevices("Select USB device(s) to attach", candidates)
	if len(picked) == 0 {
		return
	}
	attachWinUSBDevices(picked, false)
}
