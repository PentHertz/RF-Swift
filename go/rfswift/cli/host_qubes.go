/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  `rfswift host qubes [status|setup]`: RF Swift inside a Qubes OS qube.
*  The checks and the root script live in hostsetup/qubes.go; the user's
*  own configuration (podman_mode, default devices) in dock/qubes.go.
 */

package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	"penthertz/rfswift/hostsetup"
	"penthertz/rfswift/tui"
)

var hostQubesCmd = &cobra.Command{
	Use:   "qubes",
	Short: "Qubes OS: USB from sys-usb, persistent engines, rootful Podman without sudo",
	Long: `Checks and prepares a Qubes OS qube for RF Swift.

In a qube, USB devices come from sys-usb (qvm-usb), so /dev/bus/usb only
exists once a first device is attached; in a template-based AppVM only /home,
/rw and /usr/local survive a restart, so the rootful engines lose their images
and every change to /etc; and USB work wants rootful Podman, which used to mean
'sudo rfswift' (and lost your audio server and home on the way).

'rfswift host qubes setup' fixes all of it, in one root script it shows first:
  - loads vhci-hcd at every start: /dev/bus/usb always exists, containers can
    be created before a device is attached and see it once it is
  - keeps /var/lib/containers (and /var/lib/docker) across restarts (bind-dirs)
  - makes the rootful Podman socket usable by your user and sets
    podman_mode = rootful: rfswift and the Workbench then run as you, without
    sudo, and still get rootful device access. Access to that socket is
    root-equivalent; in a qube you already have passwordless root, so it
    grants nothing new (it is skipped where root needs a password)
  - Docker, when installed: starts docker.socket at every start (enabled in
    a TemplateVM/StandaloneVM) and gives you the socket (docker group + an
    ACL re-applied whenever the socket is created); never restarts a running
    daemon
  - Nix: runs the Nix profile's own daemon when the unit's binary is missing
    (Determinate Nix installed in the template keeps determinate-nixd in the
    template's /usr/local, which AppVMs do not see) and keeps /nix across
    restarts, so the native environments work and survive a restart
  - blacklists the DVB-T driver that grabs RTL-SDR dongles on attach, so
    native tools can open them (--no-rtl-blacklist keeps it)
  - gives root-owned workspace folders back to you and drops /dev/dri (no
    GPU in a qube) from the default devices
In an AppVM it writes /rw/config/qubes-bind-dirs.d/50_rfswift.conf and
/rw/config/rc.local.d/50-rfswift.rc (delete them to undo); in a TemplateVM or
StandaloneVM it writes /etc directly.

Examples:
  rfswift host qubes                 # show the state and what the setup would do
  rfswift host qubes setup           # show the script, ask, apply
  rfswift host qubes setup --dry-run # only print the script
  rfswift host qubes setup --yes --no-podman-socket`,
	Run: func(cmd *cobra.Command, args []string) {
		asJSON, _ := cmd.Flags().GetBool("json")
		st := rfdock.QubesStatus()
		if asJSON {
			if err := printJSON(st); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			return
		}
		if !st.Info.IsQubes {
			common.PrintInfoMessage("This is not a Qubes OS qube: nothing to do here ('rfswift host setup' prepares a regular Linux host).")
			return
		}
		printQubesStatus(st)
	},
}

var hostQubesSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Apply the Qubes OS integration (shows the root script first)",
	Long:  hostQubesCmd.Long,
	Run: func(cmd *cobra.Command, args []string) {
		yes, _ := cmd.Flags().GetBool("yes")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		st := rfdock.QubesStatus()
		if !st.Info.IsQubes {
			common.PrintInfoMessage("This is not a Qubes OS qube: nothing to do here ('rfswift host setup' prepares a regular Linux host).")
			return
		}
		o := hostsetup.DefaultQubesSetupOptions(rfdock.QubesInput())
		if cmd.Flags().Changed("podman-socket") {
			o.PodmanSocket, _ = cmd.Flags().GetBool("podman-socket")
			o.PodmanSocket = o.PodmanSocket && o.PodmanFound
		}
		if v, _ := cmd.Flags().GetBool("no-podman-socket"); v {
			o.PodmanSocket = false
		}
		if v, _ := cmd.Flags().GetBool("no-docker-group"); v {
			o.DockerGroup = false
		}
		if v, _ := cmd.Flags().GetBool("no-docker-start"); v {
			o.DockerStart = false
		}
		if v, _ := cmd.Flags().GetBool("no-persist"); v {
			o.PersistStore = false
		}
		if v, _ := cmd.Flags().GetBool("no-vhci"); v {
			o.VHCI = false
		}
		if v, _ := cmd.Flags().GetBool("no-rtl-blacklist"); v {
			o.RTLBlacklist = false
		}
		if v, _ := cmd.Flags().GetBool("no-workspace"); v {
			o.FixOwnership = nil
		}
		if g, _ := cmd.Flags().GetString("group"); g != "" {
			o.Group = g
		}

		printQubesStatus(st)
		script, err := rfdock.QubesSetupScript(o)
		if err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
		userSide := qubesUserSideChanges(st, o)
		if strings.TrimSpace(script) == "" && len(userSide) == 0 {
			common.PrintSuccessMessage("Nothing to do: RF Swift is already set up for this qube.")
			return
		}
		if strings.TrimSpace(script) != "" {
			fmt.Println("\nRoot script (exactly what runs, one sudo call):")
			fmt.Println(indent(script, "    "))
		}
		if len(userSide) > 0 {
			fmt.Println("Your config.ini (as " + st.User + ", no root):")
			for _, c := range userSide {
				fmt.Println("    - " + c)
			}
		}
		if dryRun {
			return
		}
		if !yes {
			if !tui.IsInteractive() {
				common.PrintInfoMessage("Re-run with --yes to apply without a prompt.")
				return
			}
			if !tui.ConfirmDefault("Apply this to the qube now?", true) {
				return
			}
		}
		report, changes, err := rfdock.ApplyQubesSetup(o)
		if err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
		for _, c := range changes {
			common.PrintInfoMessage("config.ini: " + c)
		}
		for _, n := range report.Notes {
			common.PrintWarningMessage(n)
		}
		after := report.Status
		if after.Ready {
			common.PrintSuccessMessage("Qubes OS integration applied: run rfswift and the Workbench as " + st.User + ", without sudo. 'rfswift host qubes' shows the state at any time.")
		} else {
			common.PrintSuccessMessage("Qubes OS integration applied. Left: " + strings.Join(after.Plan, "; ") + ".")
		}
	},
}

// qubesUserSideChanges lists what the setup changes in the user's config.ini.
func qubesUserSideChanges(st hostsetup.QubesStatus, o hostsetup.QubesSetupOptions) []string {
	var out []string
	if o.PodmanSocket && st.SetMode {
		out = append(out, "podman_mode = rootful")
	}
	if len(st.DropDevices) > 0 {
		out = append(out, "drop "+strings.Join(st.DropDevices, ", ")+" from the default devices")
	}
	return out
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

// printQubesStatus renders hostsetup.QubesStatus for a terminal.
func printQubesStatus(st hostsetup.QubesStatus) {
	tui.PrintDoctorHeaderTitle("Qubes OS integration")
	pass, warn, fail := 0, 0, 0
	for _, c := range st.Checks {
		msg := c.Detail
		if c.Fix != "" && c.Status != "ok" {
			msg += " -> " + c.Fix
		}
		tui.PrintDoctorResult(tui.DoctorResult{Name: c.Name, Status: c.Status, Message: msg})
		switch c.Status {
		case "ok":
			pass++
		case "warn":
			warn++
		case "fail":
			fail++
		}
	}
	tui.PrintDoctorSummary(tui.DoctorSummary{Pass: pass, Warn: warn, Fail: fail})
	if len(st.Plan) > 0 {
		fmt.Println("'rfswift host qubes setup' would:")
		for i, p := range st.Plan {
			fmt.Printf("  %d. %s\n", i+1, p)
		}
	} else if st.Ready {
		common.PrintSuccessMessage("RF Swift is set up for this qube.")
	}
}

func registerHostQubesCommands() {
	HostCmd.AddCommand(hostQubesCmd)
	hostQubesCmd.AddCommand(hostQubesSetupCmd)
	hostQubesCmd.Flags().Bool("json", false, "print the state as JSON")
	f := hostQubesSetupCmd.Flags()
	f.BoolP("yes", "y", false, "apply without asking")
	f.Bool("dry-run", false, "only print what would run")
	f.Bool("podman-socket", false, "grant the rootful Podman socket even where root needs a password (default: only where root is passwordless)")
	f.Bool("no-podman-socket", false, "leave the rootful Podman socket root-only (keep using sudo for USB work)")
	f.Bool("no-docker-group", false, "do not give you access to the Docker socket (docker group + ACL)")
	f.Bool("no-docker-start", false, "do not start Docker's socket at boot")
	f.Bool("no-persist", false, "do not keep the rootful engines' storage across restarts")
	f.Bool("no-vhci", false, "do not load vhci-hcd at boot")
	f.Bool("no-workspace", false, "leave root-owned workspace folders as they are")
	f.Bool("no-rtl-blacklist", false, "keep the DVB-T driver (dvb_usb_rtl28xxu) for RTL2832U dongles (TV use); native tools then cannot open them")
	f.String("group", "", "group to own the rootful Podman socket (default: your primary group)")
}
