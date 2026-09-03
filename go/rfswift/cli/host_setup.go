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
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	"penthertz/rfswift/hostsetup"
	"penthertz/rfswift/tui"
)

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
	Short: "Prepare this Linux host: udev rules, container engine, Docker access (asks first)",
	Long: `Everything the rfswift package leaves to you on purpose, each step asked
before it runs and applied in one sudo call:

  1. udev rules    RF Swift's rules for SDR/RF/HW devices (rootless Podman and
                   Nix environments need them; Docker does not)
  2. engine        install Docker and/or Podman from your distribution's
                   repositories, or skip (Nix engine only)
  3. Docker access add you to the docker group AND make it effective in the
                   current session (socket ACL), so no logout is needed

Scripts: --yes takes every recommended default; --udev, --engine and
--docker-access pin a single step (yes|no, docker|podman|both|none).

Examples:
  rfswift host setup
  rfswift host setup --yes --engine podman
  rfswift host setup --udev no --engine none --docker-access yes`,
	Run: func(cmd *cobra.Command, args []string) {
		yes, _ := cmd.Flags().GetBool("yes")
		udev, _ := cmd.Flags().GetString("udev")
		engine, _ := cmd.Flags().GetString("engine")
		dockerAccess, _ := cmd.Flags().GetString("docker-access")
		if runtime.GOOS != "linux" {
			switch runtime.GOOS {
			case "darwin":
				common.PrintInfoMessage("On macOS run the setup script instead: curl -fsSL https://raw.githubusercontent.com/PentHertz/RF-Swift/main/scripts/setup-macos.sh | bash")
			default:
				common.PrintInfoMessage("On Windows the installer bundle (RFSwift-Setup-<version>.exe) sets up WSL 2, usbipd and the engine.")
			}
			return
		}
		step := func(n int, title string) { fmt.Printf("\n[%d/3] %s\n", n, title) }
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

		step(3, "Docker access for "+hostsetup.InvokingUser())
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

		fmt.Println()
		if failed {
			common.PrintWarningMessage("Some steps failed; see above. 'rfswift doctor' shows the current state.")
			os.Exit(1)
		}
		common.PrintSuccessMessage("Host setup complete. 'rfswift doctor' shows the current state at any time.")
	},
}

func registerHostSetupCommands() {
	HostCmd.AddCommand(hostSetupCmd, hostUdevCmd, hostDockerAccessCmd)
	hostUdevCmd.Flags().Bool("list", false, "only show the state")
	hostUdevCmd.Flags().Bool("remove", false, "remove the rules RF Swift installed")
	hostUdevCmd.Flags().BoolP("yes", "y", false, "install without asking")
	hostUdevCmd.Flags().Bool("json", false, "print the state as JSON")
	hostDockerAccessCmd.Flags().Bool("status", false, "only show the state")
	hostDockerAccessCmd.Flags().BoolP("yes", "y", false, "grant without asking")
	hostDockerAccessCmd.Flags().Bool("json", false, "print the state as JSON")
	hostSetupCmd.Flags().BoolP("yes", "y", false, "take every recommended default without asking (udev yes, Docker access yes, engine only with --engine)")
	hostSetupCmd.Flags().String("udev", "ask", "udev rules step: ask, yes or no")
	hostSetupCmd.Flags().String("engine", "ask", "engine step: ask, docker, podman, both or none")
	hostSetupCmd.Flags().String("docker-access", "ask", "Docker access step: ask, yes or no")
}
