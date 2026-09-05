/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  `rfswift host setup|udev|docker-access`: the opt-in host preparation the
*  Linux packages deliberately do not perform on their own. Each step asks
*  (or takes --yes / a flag for scripts), shows what it will run, and goes
*  through a single privileged call.
 */

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	"penthertz/rfswift/hostsetup"
	"penthertz/rfswift/tui"
)

var packageSetupMarker = "/var/lib/rfswift/host-setup-pending"

func maybeOfferPackagedHostSetup(cmd *cobra.Command) {
	if runtime.GOOS != "linux" || !tui.IsInteractive() || os.Geteuid() == 0 {
		return
	}
	if _, err := os.Stat(packageSetupMarker); err != nil {
		return
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c == hostSetupCmd || c == hostUdevCmd || c == hostDockerAccessCmd || c == hostIsolateCmd {
			return
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	seen := filepath.Join(home, ".config", "rfswift", "package-setup-seen")
	if data, err := os.ReadFile(seen); err == nil && strings.TrimSpace(string(data)) == common.Version {
		return
	}
	_ = os.MkdirAll(filepath.Dir(seen), 0700)
	run := tui.ConfirmDefault("RF Swift was installed from a package. Configure xhost/pactl checks, Nix, Docker/Podman and hardware access now?", true)
	_ = os.WriteFile(seen, []byte(common.Version+"\n"), 0600)
	if run {
		hostSetupCmd.Run(hostSetupCmd, nil)
		fmt.Println()
	}
}

func hostNixInstall(answer string, yes bool) error {
	if _, err := exec.LookPath("nix"); err == nil {
		common.PrintSuccessMessage("Nix is installed.")
		return nil
	}
	install := strings.EqualFold(answer, "yes")
	if strings.EqualFold(answer, "no") {
		return nil
	}
	if answer == "ask" {
		if yes {
			install = true
		} else {
			if !tui.IsInteractive() {
				common.PrintInfoMessage("Pass --nix yes to install Nix non-interactively.")
				return nil
			}
			install = tui.ConfirmDefault("Install Nix with the Determinate installer (flakes enabled, no container required)?", true)
		}
	}
	if !install {
		return nil
	}
	if !yes && !tui.IsInteractive() {
		return fmt.Errorf("Nix installation needs a terminal or --yes")
	}
	common.PrintInfoMessage("Downloading and running the Determinate Nix installer...")
	command := "curl -fsSL https://install.determinate.systems/nix | sh -s -- install --no-confirm"
	if _, err := exec.LookPath("curl"); err != nil {
		if _, wgetErr := exec.LookPath("wget"); wgetErr != nil {
			return fmt.Errorf("install curl or wget before installing Nix")
		}
		command = "wget -qO- https://install.determinate.systems/nix | sh -s -- install --no-confirm"
	}
	c := exec.Command("sh", "-c", command)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// printHostUdevStatus renders hostsetup.UdevStatus for a terminal.
func printHostUdevStatus(st hostsetup.UdevStatus) {
	fmt.Printf("  %-22s %s\n", "rules file:", st.Path)
	fmt.Printf("  %-22s %s\n", "state:", st.State)
	fmt.Printf("  %-22s %s\n", "reference copy:", hostsetup.SharedRulesPath+" (also embedded in this binary)")
	if len(st.Groups) > 0 {
		groups := strings.Join(st.Groups, ", ")
		switch {
		case len(st.GroupsAbsent) > 0:
			groups += "  (missing on this host: " + strings.Join(st.GroupsAbsent, ", ") + ")"
		case len(st.GroupsNotMember) > 0:
			groups += "  (" + st.User + " is not a member of: " + strings.Join(st.GroupsNotMember, ", ") + ")"
		default:
			groups += "  (" + st.User + " is a member)"
		}
		fmt.Printf("  %-22s %s\n", "groups:", groups)
	}
	fmt.Printf("  %-22s %s\n", "summary:", st.Detail)
}

// hostUdevInstall installs the host rules after confirmation (unless yes),
// and returns whether it ran.
func hostUdevInstall(st hostsetup.UdevStatus, yes bool) (bool, error) {
	if st.State == hostsetup.UdevForeign {
		common.PrintWarningMessage(st.Detail)
		return false, nil
	}
	if st.Ready {
		common.PrintSuccessMessage("RF Swift's udev rules are installed and the groups are in place.")
		return false, nil
	}
	what := "Install RF Swift's udev rules"
	if st.State == hostsetup.UdevInstalled {
		what = "Fix the group membership for RF Swift's udev rules"
	} else if st.State == hostsetup.UdevOutdated {
		what = "Update RF Swift's udev rules"
	}
	if !yes {
		if !tui.IsInteractive() {
			common.PrintInfoMessage(what + ": re-run with --yes to apply without a prompt.")
			return false, nil
		}
		if !tui.ConfirmDefault(what+" in "+hostsetup.UdevRulesDir+"? (SDR/RF hardware without root in rootless Podman and Nix environments; asks for your password once)", true) {
			return false, nil
		}
	}
	report, err := hostsetup.InstallUdevRules()
	if err != nil {
		return false, err
	}
	common.PrintSuccessMessage(report.Detail + ". Devices already plugged in were re-triggered; re-plug one that still fails.")
	if len(report.GroupsJoined) > 0 {
		common.PrintInfoMessage(fmt.Sprintf("Group membership becomes active at your next login (or 'newgrp %s' in this terminal). Rootless Podman keeps your groups only with the crun runtime; the logged-in user's seat ACL works right away.", report.GroupsJoined[0]))
	}
	return true, nil
}

var hostUdevCmd = &cobra.Command{
	Use:   "udev",
	Short: "Install RF Swift's udev rules for RF/HW devices on this host (Linux)",
	Long: `Docker runs containers as root and needs no udev setup. Rootless Podman and
native Nix environments run as your user and cannot open the root-owned USB
device nodes, so SDR / RFID / Bluetooth / debug hardware fails with
"permission denied" until udev rules on the HOST grant access (rules inside a
container are never evaluated).

The rfswift package ships its rules as a reference copy under
/usr/share/rfswift/udev/ and does not activate them. This command installs the
same rules (embedded in this binary) into ` + hostsetup.UdevRulesDir + `, creates the
plugdev group, adds you to it and reloads udev - in one sudo call, after
asking. The rules use MODE 0660 + GROUP plugdev + the systemd uaccess tag
(seat ACL for the logged-in user), never world-writable nodes. Serial ports
keep the dialout group.

Examples:
  rfswift host udev            # show the state, offer to install
  rfswift host udev --list     # only show
  rfswift host udev --yes      # install without asking (scripts)
  rfswift host udev --remove   # remove what RF Swift installed`,
	Run: func(cmd *cobra.Command, args []string) {
		listOnly, _ := cmd.Flags().GetBool("list")
		remove, _ := cmd.Flags().GetBool("remove")
		yes, _ := cmd.Flags().GetBool("yes")
		asJSON, _ := cmd.Flags().GetBool("json")
		if runtime.GOOS != "linux" {
			common.PrintInfoMessage("udev rules are a Linux mechanism. On macOS the Lima VM carries them; on Windows install the WinUSB driver with Zadig and forward the device with 'rfswift usb attach'.")
			return
		}
		if remove {
			removed, err := hostsetup.RemoveUdevRules()
			if err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			if !removed {
				common.PrintInfoMessage("No RF Swift host udev rules installed.")
				return
			}
			common.PrintSuccessMessage("Removed " + hostsetup.HostRulesFile + " from " + hostsetup.UdevRulesDir + " and reloaded udev.")
			return
		}
		st := hostsetup.GetUdevStatus()
		if asJSON {
			if err := printJSON(st); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			return
		}
		printHostUdevStatus(st)
		if listOnly {
			return
		}
		if _, err := hostUdevInstall(st, yes); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

// printDockerAccess renders hostsetup.DockerAccess for a terminal.
func printDockerAccess(st hostsetup.DockerAccess) {
	fmt.Printf("  %-22s %s\n", "socket:", st.Socket)
	fmt.Printf("  %-22s %v\n", "docker installed:", st.DockerFound)
	fmt.Printf("  %-22s %v (in this session: %v)\n", "in docker group:", st.Member, st.Active)
	fmt.Printf("  %-22s %v (daemon answering: %v)\n", "socket usable now:", st.Accessible, st.Reachable)
	fmt.Printf("  %-22s %s\n", "summary:", st.Detail)
}

// hostDockerGrant grants Docker access after confirmation (unless yes), and
// returns whether it ran.
func hostDockerGrant(st hostsetup.DockerAccess, yes bool) (bool, error) {
	if st.Ready {
		common.PrintSuccessMessage("Docker is usable by " + st.User + ".")
		return false, nil
	}
	if !st.DockerFound && !st.SocketFound {
		common.PrintInfoMessage("Docker is not installed (rfswift host setup can install it).")
		return false, nil
	}
	if !yes {
		if !tui.IsInteractive() {
			common.PrintInfoMessage("Re-run with --yes to add " + st.User + " to the docker group without a prompt.")
			return false, nil
		}
		if !tui.ConfirmDefault("Add "+st.User+" to the docker group and make Docker usable in this session right away (no logout)? Members of the docker group are root-equivalent on this machine.", true) {
			return false, nil
		}
	}
	report, err := hostsetup.GrantDockerAccess()
	if err != nil {
		return false, err
	}
	common.PrintSuccessMessage(report.Detail + ".")
	return true, nil
}

var hostDockerAccessCmd = &cobra.Command{
	Use:   "docker-access",
	Short: "Let your user talk to the Docker daemon, effective immediately (Linux)",
	Long: `The Docker socket belongs to root:docker. Being added to the docker group only
counts from the next login; this command adds you AND puts an ACL for you on the
socket so Docker works in the current session right away (the ACL lasts until
the daemon recreates the socket, by which time the group is active). One sudo
call, after asking. Note: docker group members are root-equivalent.

Examples:
  rfswift host docker-access            # show the state, offer to fix it
  rfswift host docker-access --status   # only show
  rfswift host docker-access --yes      # fix without asking (scripts)`,
	Run: func(cmd *cobra.Command, args []string) {
		statusOnly, _ := cmd.Flags().GetBool("status")
		yes, _ := cmd.Flags().GetBool("yes")
		asJSON, _ := cmd.Flags().GetBool("json")
		if runtime.GOOS != "linux" {
			common.PrintInfoMessage("Docker socket group access is a Linux mechanism; Docker Desktop manages access on macOS and Windows.")
			return
		}
		st := hostsetup.GetDockerAccess()
		if asJSON {
			if err := printJSON(st); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			return
		}
		printDockerAccess(st)
		if statusOnly {
			return
		}
		if _, err := hostDockerGrant(st, yes); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

// printIsolationStatus renders hostsetup.IsolationStatus for a terminal.
func printIsolationStatus(st hostsetup.IsolationStatus) {
	bwrap := st.Bwrap
	switch {
	case bwrap == "":
		bwrap = "none installed (--isolate builds one from nixpkgs at first use)"
	case st.Setuid:
		bwrap += " (setuid-root)"
	}
	fmt.Printf("  %-22s %s\n", "bubblewrap:", bwrap)
	fmt.Printf("  %-22s %v\n", "sandbox works:", st.Ready)
	if st.AppArmorRestricted {
		profile := "not enabled"
		switch {
		case st.Ready && st.Bwrap == hostsetup.DistroBwrap:
			profile = "active"
		case st.ProfileInstalled:
			profile = "installed in " + hostsetup.AppArmorDir + ", not loaded"
		case st.ProfileAvailable:
			profile = "available in " + hostsetup.AppArmorExtraDir + ", not enabled"
		}
		fmt.Printf("  %-22s %s\n", "AppArmor userns:", "restricted to profiled programs (Ubuntu 24.04+); "+hostsetup.BwrapProfile+" "+profile)
	}
	if st.UsernsClone != "" {
		fmt.Printf("  %-22s kernel.unprivileged_userns_clone=%s\n", "user namespaces:", st.UsernsClone)
	}
	if st.Error != "" && !st.Ready {
		fmt.Printf("  %-22s %s\n", "bwrap said:", st.Error)
	}
	fmt.Printf("  %-22s %s\n", "summary:", st.Detail)
}

// hostIsolateFix applies the targeted fix for the Nix jail after confirmation
// (unless yes) - or, with sysctl, the last-resort sysctl - and returns whether
// it ran.
func hostIsolateFix(st hostsetup.IsolationStatus, yes, sysctl bool) (bool, error) {
	if !st.Supported {
		common.PrintInfoMessage(st.Detail)
		return false, nil
	}
	if st.Ready {
		common.PrintSuccessMessage("The bubblewrap sandbox works: 'rfswift run --engine nix --isolate' is ready.")
		return false, nil
	}
	if !st.CanFix && !sysctl {
		if st.AppArmorRestricted {
			common.PrintWarningMessage(st.Detail + ". No packaged AppArmor profile to enable here; --sysctl lifts the restriction for every program instead (it weakens the host).")
		} else {
			common.PrintWarningMessage(st.Detail + ". Nothing RF Swift can apply automatically.")
		}
		return false, nil
	}
	plan, err := hostsetup.PlanIsolationFix(st, sysctl)
	if err != nil {
		return false, err
	}
	if plan.Distro.Name != "" {
		fmt.Printf("  %-22s %s (%s)\n", "host:", plan.Distro.Name, plan.Distro.PackageManager)
	}
	for i, step := range plan.Steps {
		fmt.Printf("  %-22s %s\n", fmt.Sprintf("step %d:", i+1), step)
	}
	if !yes {
		if !tui.IsInteractive() {
			common.PrintInfoMessage("Re-run with --yes to run these steps without a prompt.")
			return false, nil
		}
		question := "Run these steps now? (one sudo password prompt; the script is exactly what is listed)"
		if plan.Sysctl {
			question = "Lift the user-namespace restriction for EVERY program on this host? This weakens its hardening; the AppArmor profile route is preferred when available (one sudo password prompt)"
		}
		if !tui.ConfirmDefault(question, !plan.Sysctl) {
			return false, nil
		}
	}
	report, err := hostsetup.EnableIsolation(plan)
	if err != nil {
		return false, err
	}
	if report.Status.Ready {
		common.PrintSuccessMessage(report.Detail + ".")
	} else {
		common.PrintWarningMessage(report.Detail + ".")
	}
	return true, nil
}

var hostIsolateCmd = &cobra.Command{
	Use:   "isolate",
	Short: "Make the Nix engine's --isolate jail (bubblewrap) work on this host (Linux)",
	Long: `'rfswift run --engine nix --isolate' hides your $HOME and the host filesystem
from a Nix environment with a bubblewrap jail. bubblewrap must be able to
create a user namespace as your user, and Ubuntu 24.04+ restricts that with
AppArmor: only a profiled bubblewrap may, and the profile
(bwrap-userns-restrict) is attached to /usr/bin/bwrap. Without it the jail
fails with "bwrap: setting up uid map: Permission denied".

This command shows what is in the way and applies the targeted fix in one sudo
call, after asking: the distribution's bubblewrap package when the bwrap in
use is another one (a Nix profile's, a nixpkgs build), then the AppArmor
profile (loaded from /etc/apparmor.d, else copied from apparmor-profiles'
extra profiles). The restriction stays in force for every other program. A
Debian kernel with kernel.unprivileged_userns_clone=0 gets the distribution
default back. --sysctl is the last resort: it lifts Ubuntu's restriction for
EVERY program (kernel.apparmor_restrict_unprivileged_userns=0, persisted
under /etc/sysctl.d), which weakens the host.

Examples:
  rfswift host isolate            # show the state, offer the fix
  rfswift host isolate --status   # only show
  rfswift host isolate --yes      # apply without asking (scripts)
  rfswift host isolate --sysctl   # last resort, see above`,
	Run: func(cmd *cobra.Command, args []string) {
		statusOnly, _ := cmd.Flags().GetBool("status")
		yes, _ := cmd.Flags().GetBool("yes")
		sysctl, _ := cmd.Flags().GetBool("sysctl")
		asJSON, _ := cmd.Flags().GetBool("json")
		if runtime.GOOS != "linux" {
			common.PrintInfoMessage("The bubblewrap jail is a Linux mechanism; --isolate uses sandbox-exec on macOS and the WSL 2 distribution on Windows.")
			return
		}
		st := hostsetup.GetIsolationStatus()
		if asJSON {
			if err := printJSON(st); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			return
		}
		printIsolationStatus(st)
		if statusOnly {
			return
		}
		if _, err := hostIsolateFix(st, yes, sysctl); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

// hostEngineInstall runs the engine selection and installation step of the
// wizard. choice "ask" prompts; anything else is taken as given.
func hostEngineInstall(choice string, yes bool) error {
	hasDocker, hasPodman := hostsetup.EnginePresent("docker"), hostsetup.EnginePresent("podman")
	switch {
	case hasDocker && hasPodman:
		common.PrintSuccessMessage("Docker and Podman are installed; RF Swift auto-detects the engine (override with --engine).")
	case hasDocker:
		common.PrintSuccessMessage("Docker is installed.")
	case hasPodman:
		common.PrintSuccessMessage("Podman is installed.")
	default:
		common.PrintWarningMessage("No container engine found. The Nix engine (rfswift --engine nix) works without one.")
	}
	var wanted hostsetup.EngineChoice
	if choice != "ask" {
		c, err := hostsetup.ParseEngineChoice(choice)
		if err != nil {
			return err
		}
		wanted = c
	} else if !tui.IsInteractive() {
		common.PrintInfoMessage("Pass --engine docker|podman|both to install an engine without a prompt.")
		return nil
	} else {
		switch {
		case hasDocker && hasPodman:
			return nil
		case hasDocker:
			if tui.ConfirmDefault("Also install Podman (daemonless, rootless containers)?", false) {
				wanted = hostsetup.EnginePodman
			}
		case hasPodman:
			if tui.ConfirmDefault("Also install Docker (root daemon, broadest compatibility)?", false) {
				wanted = hostsetup.EngineDocker
			}
		default:
			options := []string{
				"Docker      - root daemon, broadest compatibility, devices work without udev rules",
				"Podman      - daemonless and rootless, containers run as you (udev rules recommended)",
				"Both        - RF Swift picks at runtime, --engine overrides",
				"Skip        - Nix engine only, or install an engine yourself",
			}
			pick, err := tui.SelectOne("Which container engine should RF Swift install?", options)
			if err != nil {
				return nil
			}
			switch {
			case strings.HasPrefix(pick, "Docker"):
				wanted = hostsetup.EngineDocker
			case strings.HasPrefix(pick, "Podman"):
				wanted = hostsetup.EnginePodman
			case strings.HasPrefix(pick, "Both"):
				wanted = hostsetup.EngineBoth
			}
		}
	}
	// Do not reinstall what is present.
	switch wanted {
	case hostsetup.EngineDocker:
		if hasDocker {
			wanted = hostsetup.EngineNone
		}
	case hostsetup.EnginePodman:
		if hasPodman {
			wanted = hostsetup.EngineNone
		}
	case hostsetup.EngineBoth:
		switch {
		case hasDocker && hasPodman:
			wanted = hostsetup.EngineNone
		case hasDocker:
			wanted = hostsetup.EnginePodman
		case hasPodman:
			wanted = hostsetup.EngineDocker
		}
	}
	if wanted == "" || wanted == hostsetup.EngineNone {
		return nil
	}
	plan, err := hostsetup.PlanEngineInstall(wanted)
	if err != nil {
		return err
	}
	fmt.Printf("  %-22s %s (%s)\n", "host:", plan.Distro.Name, plan.Distro.PackageManager)
	fmt.Printf("  %-22s %s\n", "packages:", strings.Join(plan.Packages, " "))
	for i, step := range plan.Steps {
		fmt.Printf("  %-22s %s\n", fmt.Sprintf("step %d:", i+1), step)
	}
	if !yes {
		if !tui.IsInteractive() {
			common.PrintInfoMessage("Re-run with --yes to run these steps without a prompt.")
			return nil
		}
		if !tui.ConfirmDefault("Run these steps now? (one sudo password prompt; the script is exactly what is listed)", true) {
			return nil
		}
	}
	report, err := hostsetup.InstallEngines(plan)
	if err != nil {
		return err
	}
	common.PrintSuccessMessage(report.Detail + ".")
	return nil
}

var hostSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Prepare this Linux host: dependencies, Nix, containers and hardware access",
	Long: `Everything the rfswift package leaves to you on purpose, each step asked
before it runs and applied in one sudo call:

  1. udev rules    RF Swift's rules for SDR/RF/HW devices (rootless Podman and
                   Nix environments need them; Docker does not)
  2. engine        install Docker and/or Podman from your distribution's
                   repositories, or skip (Nix engine only)
  3. Nix           install the native, daemon-backed Nix engine with flakes
  4. Docker access add you to the docker group AND make it effective in the
                   current session (socket ACL), so no logout is needed
  5. isolation     make the Nix engine's --isolate jail work: bubblewrap and,
                   on Ubuntu 24.04+, its AppArmor profile (rfswift host isolate)

The deb/rpm packages install xhost and pactl automatically before this wizard.
Scripts: --yes takes every recommended default; --udev, --engine, --nix,
--docker-access and --isolate pin a single step (yes|no, docker|podman|both|none).

Examples:
  rfswift host setup
  rfswift host setup --yes --engine podman
  rfswift host setup --udev no --engine none --docker-access yes`,
	Run: func(cmd *cobra.Command, args []string) {
		yes, _ := cmd.Flags().GetBool("yes")
		udev, _ := cmd.Flags().GetString("udev")
		engine, _ := cmd.Flags().GetString("engine")
		nixChoice, _ := cmd.Flags().GetString("nix")
		dockerAccess, _ := cmd.Flags().GetString("docker-access")
		isolate, _ := cmd.Flags().GetString("isolate")
		if runtime.GOOS != "linux" {
			switch runtime.GOOS {
			case "darwin":
				common.PrintInfoMessage("On macOS run the setup script instead: curl -fsSL https://raw.githubusercontent.com/PentHertz/RF-Swift/main/scripts/setup-macos.sh | bash")
			default:
				common.PrintInfoMessage("On Windows the installer bundle (RFSwift-Setup-<version>.exe) sets up WSL 2, usbipd and the engine.")
			}
			return
		}
		step := func(n int, title string) { fmt.Printf("\n[%d/5] %s\n", n, title) }
		failed := false

		step(1, "udev rules for RF / hardware-security devices")
		if strings.EqualFold(udev, "no") {
			common.PrintInfoMessage("skipped (--udev no)")
		} else {
			st := hostsetup.GetUdevStatus()
			printHostUdevStatus(st)
			if _, err := hostUdevInstall(st, yes || strings.EqualFold(udev, "yes")); err != nil {
				common.PrintErrorMessage(err)
				failed = true
			}
		}

		step(2, "container engine")
		if strings.EqualFold(engine, "none") || strings.EqualFold(engine, "no") {
			common.PrintInfoMessage("skipped (--engine none)")
		} else {
			if yes && engine == "ask" {
				engine = "none"
			}
			if err := hostEngineInstall(engine, yes); err != nil {
				common.PrintErrorMessage(err)
				failed = true
			}
		}

		step(3, "native Nix engine")
		if err := hostNixInstall(nixChoice, yes); err != nil {
			common.PrintErrorMessage(err)
			failed = true
		}

		step(4, "Docker access for "+hostsetup.InvokingUser())
		if strings.EqualFold(dockerAccess, "no") {
			common.PrintInfoMessage("skipped (--docker-access no)")
		} else {
			st := hostsetup.GetDockerAccess()
			printDockerAccess(st)
			if _, err := hostDockerGrant(st, yes || strings.EqualFold(dockerAccess, "yes")); err != nil {
				common.PrintErrorMessage(err)
				failed = true
			}
		}

		step(5, "isolation for Nix environments (--isolate, bubblewrap)")
		if strings.EqualFold(isolate, "no") {
			common.PrintInfoMessage("skipped (--isolate no)")
		} else {
			st := hostsetup.GetIsolationStatus()
			printIsolationStatus(st)
			if _, err := hostIsolateFix(st, yes || strings.EqualFold(isolate, "yes"), false); err != nil {
				common.PrintErrorMessage(err)
				failed = true
			}
		}

		fmt.Println()
		if failed {
			common.PrintWarningMessage("Some steps failed; see above. 'rfswift doctor' shows the current state.")
			os.Exit(1)
		}
		common.PrintSuccessMessage("Host setup complete. 'rfswift doctor' shows the current state at any time.")
	},
}

func registerHostSetupCommands() {
	HostCmd.AddCommand(hostSetupCmd, hostUdevCmd, hostDockerAccessCmd, hostIsolateCmd)
	hostIsolateCmd.Flags().Bool("status", false, "only show the state")
	hostIsolateCmd.Flags().BoolP("yes", "y", false, "apply without asking")
	hostIsolateCmd.Flags().Bool("sysctl", false, "last resort: lift Ubuntu's user-namespace restriction for every program (weakens the host)")
	hostIsolateCmd.Flags().Bool("json", false, "print the state as JSON")
	hostUdevCmd.Flags().Bool("list", false, "only show the state")
	hostUdevCmd.Flags().Bool("remove", false, "remove the rules RF Swift installed")
	hostUdevCmd.Flags().BoolP("yes", "y", false, "install without asking")
	hostUdevCmd.Flags().Bool("json", false, "print the state as JSON")
	hostDockerAccessCmd.Flags().Bool("status", false, "only show the state")
	hostDockerAccessCmd.Flags().BoolP("yes", "y", false, "grant without asking")
	hostDockerAccessCmd.Flags().Bool("json", false, "print the state as JSON")
	hostSetupCmd.Flags().BoolP("yes", "y", false, "take every recommended default without asking (udev yes, Docker access yes, isolation yes, engine only with --engine)")
	hostSetupCmd.Flags().String("isolate", "ask", "Nix jail step (bubblewrap + AppArmor profile): ask, yes or no")
	hostSetupCmd.Flags().String("udev", "ask", "udev rules step: ask, yes or no")
	hostSetupCmd.Flags().String("engine", "ask", "engine step: ask, docker, podman, both or none")
	hostSetupCmd.Flags().String("nix", "ask", "Nix step: ask, yes or no")
	hostSetupCmd.Flags().String("docker-access", "ask", "Docker access step: ask, yes or no")
}
