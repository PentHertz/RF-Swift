/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine on Windows: the CLI side of the WSL 2 backend.
*
*  Every Nix-engine command typed in a Windows console is served by the Linux
*  rfswift inside the WSL 2 distribution (bridgeNixCommandToWSL): the same
*  wizards, builds, shells and messages, on the side where nix, WSLg and the
*  usbipd-forwarded hardware are. The `rfswift nix wsl` group is the only part
*  that runs on Windows itself: it inspects and provisions that distribution.
*  See nix/wsl.go for the model.
 */

package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfnix "penthertz/rfswift/nix"
	rfutils "penthertz/rfswift/rfutils"
	"penthertz/rfswift/tui"
)

// engineCommandNames are the container-or-nix commands whose behaviour depends
// on the selected engine (run/exec/install and their resource-first clones).
var engineCommandNames = map[string]bool{"run": true, "exec": true, "install": true, "create": true, "shell": true}

// isNixWSLCommand reports whether cmd belongs to `rfswift nix wsl` (or its
// `env wsl` clone), which manages the distribution from Windows itself.
func isNixWSLCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "wsl" {
			return true
		}
	}
	return false
}

// shouldBridgeNixToWSL decides whether a command must run inside the WSL
// distribution: the nix/env command groups always, the engine-dependent
// commands when the Nix engine is the selected one (the caller established
// that), never the `nix wsl` group itself.
func shouldBridgeNixToWSL(cmd *cobra.Command) bool {
	if cmd == nil || isNixWSLCommand(cmd) {
		return false
	}
	return isNixCommand(cmd) || engineCommandNames[cmd.Name()]
}

// bridgeNixCommandToWSL, on Windows, runs the current command line through the
// Linux rfswift of the WSL 2 backend and exits with its status. It returns
// only when the command is not one to bridge (or the OS is not Windows). A
// backend that is not provisioned yet is offered `nix wsl setup` first.
func bridgeNixCommandToWSL(cmd *cobra.Command) {
	if runtime.GOOS != "windows" || !shouldBridgeNixToWSL(cmd) {
		return
	}
	st, err := rfnix.WSLBackend()
	if err != nil || !st.Ready() {
		if tui.IsInteractive() && offerNixWSLSetup(st, err) {
			st, err = rfnix.WSLBackend()
		}
		if err != nil || !st.Ready() {
			common.PrintErrorMessage(rfnix.WSLReadyError(st, err))
			os.Exit(1)
		}
	}
	if st.RFSwiftVersion != "" && st.RFSwiftVersion != "unknown" && st.RFSwiftVersion != common.Version {
		common.PrintInfoMessage(fmt.Sprintf("The Linux rfswift inside WSL distribution %s is %s (this one is %s); align them with: rfswift nix wsl setup --update", st.Distro, st.RFSwiftVersion, common.Version))
	}
	// The environment will want the radios: forward them into the WSL 2 kernel
	// first, exactly as before a container is created on Windows - when an
	// environment is created and when one is re-entered (a device plugged in
	// since is the common case).
	switch cmd.Name() {
	case "run", "create", "exec", "shell":
		WinUSBWizardStepForNix()
		// The tools will want a window: a WSLg display client that stopped
		// painting them (only a taskbar icon appears) is restarted first.
		rfnix.WSLDisplayPreflight()
	}
	code, err := rfnix.RunRFSwiftInWSL(rfnix.WSLBridgeArgs(os.Args[1:], engineCommandNames[cmd.Name()]))
	if err != nil {
		common.PrintErrorMessage(err)
		if code == 0 {
			code = 1
		}
	}
	if cmd.Name() == "gc" && code == 0 {
		// The Linux side freed space inside the distribution; the Windows
		// drive only sees it once the virtual disk is sparse or compacted.
		common.PrintInfoMessage(fmt.Sprintf("Space freed inside WSL is not returned to the Windows drive by itself: once, run 'wsl --shutdown' then 'wsl --manage %s --set-sparse true' (or compact its ext4.vhdx with Optimize-VHD).", st.Distro))
	}
	os.Exit(code)
}

// offerNixWSLSetup proposes to provision the distribution when a Nix command
// finds it not ready. Returns true when setup ran (the caller re-probes).
func offerNixWSLSetup(st rfutils.WSLNixStatus, err error) bool {
	if err != nil {
		common.PrintWarningMessage(fmt.Sprintf("The Nix engine on Windows runs inside a WSL 2 distribution, and none is usable yet: %v", err))
	} else {
		common.PrintWarningMessage(fmt.Sprintf("WSL distribution %q is not ready for the Nix engine: it lacks %s.", st.Distro, strings.Join(st.Missing(), " and ")))
	}
	if !tui.Confirm("Set it up now? (installs Nix with flakes and the Linux rfswift CLI in the distribution; needs a network connection)") {
		return false
	}
	if _, setupErr := rfnix.SetupWSL(rfnix.WSLSetupOptions{Distro: st.Distro, Confirm: tui.Confirm}); setupErr != nil {
		common.PrintErrorMessage(setupErr)
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// `rfswift nix wsl` command group (Windows)
// ---------------------------------------------------------------------------

var nixWSLCmd = &cobra.Command{
	Use:   "wsl",
	Short: "Inspect and set up the WSL 2 distribution that hosts the Nix engine (Windows)",
	Long: `On Windows the Nix engine runs inside a WSL 2 Linux distribution: Nix, the
Linux rfswift CLI, the environments and their default workspaces live there,
and every 'rfswift ... --engine nix' or 'rfswift nix ...' command you type here
is served by that Linux rfswift. WSLg gives the environments a display and
sound; 'rfswift usb attach' (usbipd) forwards your radios into the same kernel.

  rfswift nix wsl status          what the distribution offers (nix, rfswift, WSLg, USB)
  rfswift nix wsl setup           provision it (systemd, Nix with flakes, the Linux rfswift)
  rfswift nix wsl use <distro>    pick the distribution when several are installed
  rfswift nix wsl shell           open a login shell inside it
  rfswift nix wsl display-reset   restart WSLg's display client when a tool shows only a taskbar icon

The distribution is chosen from RFSWIFT_WSL_DISTRO, then [nix] wsl_distro in
config.ini, then the default WSL 2 distribution (container engines' utility
VMs such as docker-desktop are never used). Workspaces created with the
default location are reachable from Explorer at \\wsl.localhost\<distro>\home\<user>\rfswift-workspace.`,
}

var nixWSLStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the WSL 2 distribution serving the Nix engine and what it offers",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		st, err := rfnix.WSLBackend()
		if err != nil {
			common.PrintErrorMessage(rfnix.WSLReadyError(st, err))
			os.Exit(1)
		}
		yesNo := func(v bool) string {
			if v {
				return "yes"
			}
			return "no"
		}
		orMissing := func(v string) string {
			if v == "" {
				return "missing"
			}
			return v
		}
		rfswiftLabel := orMissing(st.RFSwiftVersion)
		if st.RFSwiftVersion != "" && st.RFSwiftVersion != "unknown" && st.RFSwiftVersion != common.Version {
			rfswiftLabel += fmt.Sprintf(" (this Windows rfswift is %s: run 'rfswift nix wsl setup --update')", common.Version)
		}
		items := []tui.PropertyItem{
			{Key: "Distribution", Value: st.Distro, ValueColor: tui.ColorPrimary},
			{Key: "Linux user", Value: fmt.Sprintf("%s (%s)", st.User, st.Home)},
			{Key: "Kernel", Value: st.Kernel},
			{Key: "Nix", Value: orMissing(st.NixVersion), ValueColor: readyColor(st.HasNix())},
			{Key: "rfswift (Linux)", Value: rfswiftLabel, ValueColor: readyColor(st.HasRFSwift())},
			{Key: "systemd", Value: yesNo(st.Systemd)},
			{Key: "WSLg display / audio", Value: yesNo(st.X11) + " / " + yesNo(st.Audio)},
			{Key: "WSLg GPU libraries", Value: yesNo(st.GPULibs)},
			{Key: "bubblewrap (--isolate)", Value: yesNo(st.Bubblewrap) + " (built from nixpkgs on first use when absent)"},
			{Key: "USB devices in WSL", Value: fmt.Sprintf("%d (forward radios with 'rfswift usb attach')", st.USBDevices)},
		}
		display, displayErr := rfnix.WSLDisplayState()
		if displayErr == nil {
			item := tui.PropertyItem{Key: "WSLg display client", Value: display.Summary()}
			switch {
			case display.Degraded:
				item.Value += " - fix: rfswift nix wsl display-reset"
				item.ValueColor = tui.ColorDanger
			case display.ClientRunning:
				item.ValueColor = tui.ColorSuccess
			}
			items = append(items, item)
		}
		if root := rfnix.WSLWorkspaceRoot(); root != "" {
			items = append(items, tui.PropertyItem{Key: "Workspaces (Windows)", Value: root, ValueColor: tui.ColorCyan})
		}
		tui.RenderPropertySheet("🪟 Nix engine on Windows (WSL 2)", tui.ColorPrimary, items)
		if displayErr == nil && display.Degraded {
			fmt.Println()
			common.PrintWarningMessage("WSLg's display client stopped painting windows: GUI tools show only a taskbar icon until it is restarted. Run: rfswift nix wsl display-reset")
		}
		if !st.Ready() {
			fmt.Println()
			common.PrintWarningMessage(rfnix.WSLReadyError(st, nil).Error())
			os.Exit(1)
		}
		fmt.Println()
		common.PrintInfoMessage("Ready. Create an environment with: rfswift run --engine nix -i sdr_light -n mysdr")
	},
}

func readyColor(ok bool) lipgloss.Color {
	if ok {
		return tui.ColorSuccess
	}
	return tui.ColorDanger
}

var nixWSLSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Provision the WSL 2 distribution: systemd, Nix with flakes, the Linux rfswift CLI",
	Long: `Make a WSL 2 distribution ready for the Nix engine. Each step is skipped when
already done and asked for before it changes anything (--yes answers for you):

  1. a WSL 2 distribution exists (offers 'wsl --install -d Ubuntu' when none does)
  2. systemd is enabled in it, so the nix daemon and udev run as services
  3. Nix is installed with flakes (Determinate Systems installer, as root)
  4. the Linux rfswift CLI is installed in /usr/local/bin, matching this
     version when that release exists, else the latest release

This is what the Windows installer's optional "Set up Nix in WSL 2" step does.

Examples:
  rfswift nix wsl setup                       # the default distribution
  rfswift nix wsl setup --distro Debian       # a specific one (remembered in config.ini)
  rfswift nix wsl setup --update              # refresh the Linux rfswift to this version
  rfswift nix wsl setup --binary .\rfswift_linux_amd64   # developers: install a local build`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		distro, _ := cmd.Flags().GetString("distro")
		install, _ := cmd.Flags().GetString("install-distro")
		yes, _ := cmd.Flags().GetBool("yes")
		update, _ := cmd.Flags().GetBool("update")
		version, _ := cmd.Flags().GetString("version")
		binary, _ := cmd.Flags().GetString("binary")
		skipNix, _ := cmd.Flags().GetBool("no-nix")
		skipRFSwift, _ := cmd.Flags().GetBool("no-rfswift")
		if !yes && !tui.IsInteractive() {
			return fmt.Errorf("no interactive terminal to confirm the steps; re-run with --yes")
		}
		st, err := rfnix.SetupWSL(rfnix.WSLSetupOptions{
			Distro: distro, InstallDistro: install, Yes: yes, Confirm: tui.Confirm,
			SkipNix: skipNix, SkipRFSwift: skipRFSwift, Update: update, RFSwiftVersion: version, RFSwiftBinary: binary,
		})
		if err != nil {
			return err
		}
		if !st.Ready() {
			return rfnix.WSLReadyError(st, nil)
		}
		common.PrintSuccessMessage(fmt.Sprintf("Nix engine ready in WSL 2 distribution %q. Try: rfswift run --engine nix", st.Distro))
		return nil
	},
}

var nixWSLUseCmd = &cobra.Command{
	Use:   "use <distro>",
	Short: "Choose which WSL 2 distribution hosts the Nix engine (saved in config.ini)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		list, err := rfutils.WSLDistributions()
		if err != nil {
			return err
		}
		found := false
		for _, d := range list.Distros {
			if strings.EqualFold(d.Name, name) {
				name, found = d.Name, true
				if d.Version != 2 {
					return fmt.Errorf("%s runs WSL %d; the Nix engine needs WSL 2 (wsl --set-version %s 2)", d.Name, d.Version, d.Name)
				}
			}
		}
		if !found {
			return fmt.Errorf("no WSL distribution named %q (see: wsl --list --verbose)", name)
		}
		if rfutils.IsWSLUtilityDistro(name) {
			return fmt.Errorf("%s is a container engine's utility VM; use a Linux distribution such as Ubuntu", name)
		}
		if err := rfutils.SetConfigValue(common.ConfigFileByPlatform(), "nix", "wsl_distro", name); err != nil {
			return err
		}
		rfnix.ResetWSLBackend()
		common.PrintSuccessMessage(fmt.Sprintf("The Nix engine now uses WSL distribution %q (config.ini [nix] wsl_distro). Check it with: rfswift nix wsl status", name))
		return nil
	},
}

var nixWSLShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open a login shell inside the distribution that hosts the Nix engine",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rfnix.RunWSLShell()
	},
}

var nixWSLDisplayResetCmd = &cobra.Command{
	Use:     "display-reset",
	Aliases: []string{"reset-display"},
	Short:   "Restart WSLg's display client when a GUI tool shows only a taskbar icon",
	Long: `Every window a tool opens inside the WSL 2 distribution is painted on the
Windows desktop by WSLg's RDP client, msrdc.exe. When that client trips over
an RDP graphics error it keeps running but stops painting: a tool started
afterwards shows only a taskbar icon while its log inside the distribution
looks healthy (SDR++ loads its modules and saves its config). The client
recovers by itself only minutes later, when it reconnects.

This command restarts it now: WSL relaunches the client within seconds and
every open window is shown again. No 'wsl --shutdown', nothing inside the
distribution is touched. 'rfswift nix wsl status' and 'rfswift doctor' show
when the client is in that state (the errors are in the Windows event log,
Microsoft-Windows-TerminalServices-RDPClient/Operational), and run, exec and
shell do this restart on their own when they find it (` + rfnix.WSLDisplayAutoResetVar + `=0 disables that).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		before, err := rfnix.WSLDisplayState()
		if err != nil {
			return err
		}
		common.PrintInfoMessage("WSLg display client: " + before.Summary())
		if !before.ClientRunning {
			return rfutils.ErrWSLgDisplayNotRunning
		}
		if !before.Degraded {
			common.PrintInfoMessage("No graphics error recorded since it connected; restarting it anyway (open windows vanish for a few seconds and come back).")
		}
		after, err := rfnix.WSLDisplayReset()
		if err != nil {
			return err
		}
		common.PrintSuccessMessage("WSLg display client restarted: " + after.Summary())
		return nil
	},
}

// registerNixWSLCommands adds the `nix wsl` group. Only on Windows: elsewhere
// the Nix engine runs natively and the group would be meaningless.
func registerNixWSLCommands() {
	if runtime.GOOS != "windows" {
		return
	}
	nixCmd.AddCommand(nixWSLCmd)
	nixWSLCmd.AddCommand(nixWSLStatusCmd, nixWSLSetupCmd, nixWSLUseCmd, nixWSLShellCmd, nixWSLDisplayResetCmd)
	nixWSLSetupCmd.Flags().String("distro", "", "WSL 2 distribution to provision (default: the one the engine resolves)")
	nixWSLSetupCmd.Flags().String("install-distro", "Ubuntu", "distribution to install when none exists")
	nixWSLSetupCmd.Flags().BoolP("yes", "y", false, "answer every question with yes")
	nixWSLSetupCmd.Flags().Bool("update", false, "reinstall the Linux rfswift CLI even when present (align versions)")
	nixWSLSetupCmd.Flags().String("version", "", "release tag of the Linux rfswift to install (default: this version, then the latest release)")
	nixWSLSetupCmd.Flags().String("binary", "", "local Linux rfswift binary to install instead of a release download")
	nixWSLSetupCmd.Flags().Bool("no-nix", false, "skip the Nix installation step")
	nixWSLSetupCmd.Flags().Bool("no-rfswift", false, "skip the Linux rfswift CLI step")
}
