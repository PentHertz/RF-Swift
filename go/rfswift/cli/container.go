/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	rfutils "penthertz/rfswift/rfutils"
	"penthertz/rfswift/tui"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Create and run a program",
	Long:  `Create a container and run a program inside the docker container`,
	Run: func(cmd *cobra.Command, args []string) {
		// Retrieve all flags locally
		image, _ := cmd.Flags().GetString("image")
		execCommand, _ := cmd.Flags().GetString("command")
		extraBind, _ := cmd.Flags().GetString("bind")
		xDisplay, _ := cmd.Flags().GetString("display")
		extraHost, _ := cmd.Flags().GetString("extrahosts")
		pulseServer, _ := cmd.Flags().GetString("pulseserver")
		dockerName, _ := cmd.Flags().GetString("name")
		netMode, _ := cmd.Flags().GetString("network")
		exposedPorts, _ := cmd.Flags().GetString("exposedports")
		bindedPorts, _ := cmd.Flags().GetString("bindedports")
		devices, _ := cmd.Flags().GetString("devices")
		privileged, _ := cmd.Flags().GetInt("privileged")
		caps, _ := cmd.Flags().GetString("capabilities")
		cgroups, _ := cmd.Flags().GetString("cgroups")
		seccomp, _ := cmd.Flags().GetString("seccomp")
		noX11, _ := cmd.Flags().GetBool("no-x11")
		recordSession, _ := cmd.Flags().GetBool("record")
		recordOutput, _ := cmd.Flags().GetString("record-output")
		realtime, _ := cmd.Flags().GetBool("realtime")
		ulimits, _ := cmd.Flags().GetString("ulimits")
		desktop, _ := cmd.Flags().GetBool("desktop")
		desktopConfig, _ := cmd.Flags().GetString("desktop-config")
		desktopPass, _ := cmd.Flags().GetString("desktop-pass")
		desktopSSL, _ := cmd.Flags().GetBool("desktop-ssl")
		vpnConfig, _ := cmd.Flags().GetString("vpn")
		gpus, _ := cmd.Flags().GetString("gpus")
		profileName, _ := cmd.Flags().GetString("profile")
		workspacePath, _ := cmd.Flags().GetString("workspace")
		noWorkspace, _ := cmd.Flags().GetBool("no-workspace")
		cwdWorkspace, _ := cmd.Flags().GetBool("cwd")

		// Resolve workspace config
		if noWorkspace {
			rfdock.ContainerSetWorkspace("none")
		} else if cwdWorkspace {
			cwd, _ := os.Getwd()
			rfdock.ContainerSetWorkspace(cwd)
		} else if workspacePath != "" {
			rfdock.ContainerSetWorkspace(workspacePath)
		}

		// Apply profile if specified (profile values are used as defaults, CLI flags override)
		if profileName != "" {
			prof, err := rfdock.GetProfileByName(profileName)
			if err != nil {
				common.PrintErrorMessage(err)
				return
			}
			if image == "" {
				image = prof.Image
			}
			if netMode == "" {
				netMode = prof.Network
			}
			if !desktop && prof.Desktop {
				desktop = true
			}
			if !desktopSSL && prof.DesktopSSL {
				desktopSSL = true
			}
			if !noX11 && prof.NoX11 {
				noX11 = true
			}
			if privileged == 0 && prof.Privileged {
				privileged = 1
			}
			if !realtime && prof.Realtime {
				realtime = true
			}
			if devices == "" && prof.Devices != "" {
				devices = prof.Devices
			}
			if extraBind == "" && prof.Bindings != "" {
				extraBind = prof.Bindings
			}
			if exposedPorts == "" && prof.ExposedPorts != "" {
				exposedPorts = prof.ExposedPorts
			}
			if bindedPorts == "" && prof.PortBindings != "" {
				bindedPorts = prof.PortBindings
			}
			if caps == "" && prof.Caps != "" {
				caps = prof.Caps
			}
			if cgroups == "" && prof.Cgroups != "" {
				cgroups = prof.Cgroups
			}
			if vpnConfig == "" && prof.VPN != "" {
				vpnConfig = prof.VPN
			}
			if gpus == "" && prof.GPUs != "" {
				gpus = prof.GPUs
			}
			common.PrintInfoMessage(fmt.Sprintf("Using profile: %s (%s)", prof.Name, prof.Description))
		}

		// Launch interactive wizard if name or image not provided and terminal is interactive
		if (dockerName == "" || image == "") && tui.IsInteractive() {
			availableImages := rfdock.ListImageTags("org.container.project", "rfswift")
			existingNets := rfdock.ListNATNetworkNames()

			// Build profile options for the wizard
			var profileOpts []tui.ProfileOption
			if profileName == "" { // only offer profiles if not already set via --profile
				for _, p := range rfdock.GetAllProfiles() {
					profileOpts = append(profileOpts, tui.ProfileOption{
						Name:         p.Name,
						Description:  p.Description,
						Image:        p.Image,
						Network:      p.Network,
						ExposedPorts: p.ExposedPorts,
						PortBindings: p.PortBindings,
						Desktop:      p.Desktop,
						DesktopSSL:   p.DesktopSSL,
						NoX11:        p.NoX11,
						Privileged:   p.Privileged,
						Realtime:     p.Realtime,
						Devices:      p.Devices,
						Bindings:     p.Bindings,
						Caps:         p.Caps,
						Cgroups:      p.Cgroups,
						GPUs:         p.GPUs,
						VPN:          p.VPN,
					})
				}
			}

			wizResult, err := tui.RunWizard(availableImages, &tui.RunWizardDefaults{
				Image:         image,
				Name:          dockerName,
				Bindings:      extraBind,
				Devices:       devices,
				ExposedPorts:  exposedPorts,
				PortBindings:  bindedPorts,
				Caps:          caps,
				Cgroups:       cgroups,
				Network:       netMode,
				Desktop:       desktop,
				DesktopSSL:    desktopSSL,
				NoX11:         noX11,
				Privileged:    privileged,
				Realtime:      realtime,
				VPN:           vpnConfig,
				GPUs:          gpus,
				Workspace:     workspacePath,
				WorkspaceRoot: rfdock.DefaultWorkspaceRoot(),
				Profiles:      profileOpts,
			}, existingNets)
			if err != nil {
				common.PrintErrorMessage(fmt.Errorf("wizard cancelled: %v", err))
				return
			}
			if !wizResult.Confirmed {
				common.PrintInfoMessage("Container creation cancelled.")
				return
			}
			image = wizResult.Image
			dockerName = wizResult.Name
			if wizResult.Bindings != "" {
				extraBind = wizResult.Bindings
			}
			if wizResult.Devices != "" {
				devices = wizResult.Devices
			}
			if wizResult.ExposedPorts != "" {
				exposedPorts = wizResult.ExposedPorts
			}
			if wizResult.PortBindings != "" {
				bindedPorts = wizResult.PortBindings
			}
			if wizResult.Caps != "" {
				caps = wizResult.Caps
			}
			if wizResult.Cgroups != "" {
				cgroups = wizResult.Cgroups
			}
			if wizResult.Network != "" {
				netMode = wizResult.Network
			}
			desktop = wizResult.Desktop
			desktopSSL = wizResult.DesktopSSL
			// Apply desktop port config from wizard if user configured it
			if wizResult.DesktopHost != "" || wizResult.DesktopPort != "" {
				host := wizResult.DesktopHost
				if host == "" {
					host = "127.0.0.1"
				}
				port := wizResult.DesktopPort
				if port == "" {
					port = "6080"
				}
				desktopConfig = "http:" + host + ":" + port
			}
			noX11 = wizResult.NoX11
			privileged = wizResult.Privileged
			realtime = wizResult.Realtime
			if wizResult.VPN != "" {
				vpnConfig = wizResult.VPN
			}
			if wizResult.GPUs != "" {
				gpus = wizResult.GPUs
			}
			// Process workspace from wizard
			switch wizResult.Workspace {
			case "none":
				rfdock.ContainerSetWorkspace("none")
			case "cwd":
				cwd, _ := os.Getwd()
				rfdock.ContainerSetWorkspace(cwd)
			case "":
				// auto - default behavior
			default:
				rfdock.ContainerSetWorkspace(wizResult.Workspace)
			}
		} else if dockerName == "" {
			common.PrintErrorMessage(fmt.Errorf("container name is required (use -n flag)"))
			return
		}

		// On macOS with Lima engine, offer to attach USB devices before container creation
		if runtime.GOOS == "darwin" && rfdock.GetEngine().Type() == rfdock.EngineLima && tui.IsInteractive() {
			MacUSBWizardStep(limaInstance)
		}

		if recordSession {
			// Build extra args map for recording subprocess
			extraArgs := map[string]string{}
			if extraBind != "" {
				extraArgs["-b"] = extraBind
			}
			if extraHost != "" {
				extraArgs["-x"] = extraHost
			}
			if xDisplay != "" && xDisplay != rfutils.GetDisplayEnv() {
				extraArgs["-d"] = xDisplay
			}
			if execCommand != "" {
				extraArgs["-e"] = execCommand
			}
			if pulseServer != "tcp:127.0.0.1:34567" {
				extraArgs["-p"] = pulseServer
			}
			if netMode != "" {
				extraArgs["-t"] = netMode
			}
			if exposedPorts != "" {
				extraArgs["-z"] = exposedPorts
			}
			if bindedPorts != "" {
				extraArgs["-w"] = bindedPorts
			}
			if devices != "" {
				extraArgs["-s"] = devices
			}
			if privileged != 0 {
				extraArgs["-u"] = fmt.Sprintf("%d", privileged)
			}
			if caps != "" {
				extraArgs["-a"] = caps
			}
			if cgroups != "" {
				extraArgs["-g"] = cgroups
			}
			if seccomp != "" {
				extraArgs["-m"] = seccomp
			}
			if noX11 {
				extraArgs["--no-x11"] = ""
			}
			if desktop {
				extraArgs["--desktop"] = ""
			}
			if desktopConfig != "" {
				extraArgs["--desktop-config"] = desktopConfig
			}
			if desktopPass != "" {
				extraArgs["--desktop-pass"] = desktopPass
			}
			if desktopSSL {
				extraArgs["--desktop-ssl"] = ""
			}
			if vpnConfig != "" {
				extraArgs["--vpn"] = vpnConfig
			}
			if gpus != "" {
				extraArgs["--gpus"] = gpus
			}

			if err := rfdock.ContainerRunWithRecording(dockerName, recordOutput, image, extraArgs); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}

			if realtime {
				extraArgs["--realtime"] = ""
			}
			if ulimits != "" {
				extraArgs["--ulimits"] = ulimits
			}
		} else {
			setupX11(noX11, xDisplay, true)
			rfdock.ContainerSetShell(execCommand)
			rfdock.ContainerAddBinding(extraBind)
			rfdock.ContainerSetImage(image)
			rfdock.ContainerSetExtraHosts(extraHost)
			rfdock.ContainerSetPulse(pulseServer)
			rfdock.ContainerSetNetworkMode(netMode)
			rfdock.ContainerSetExposedPorts(exposedPorts)
			rfdock.ContainerSetBindedPorts(bindedPorts)
			rfdock.ContainerAddDevices(devices)
			rfdock.ContainerAddCaps(caps)
			rfdock.ContainerAddCgroups(cgroups)
			rfdock.ContainerSetPrivileges(privileged)
			rfdock.ContainerSetSeccomp(seccomp)
			rfdock.ContainerSetRealtime(realtime)
			rfdock.ContainerSetUlimits(ulimits)
			if desktop {
				parseAndSetDesktop(desktopConfig)
				if desktopPass != "" {
					rfdock.ContainerSetDesktopPassword(desktopPass)
				}
				rfdock.ContainerSetDesktopSSL(desktopSSL)
			}
			rfdock.ContainerSetVPN(vpnConfig)
			rfdock.ContainerSetGPUs(gpus)
			if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
				rfutils.SetPulseCTL(pulseServer)
			}
			rfdock.ContainerRun(dockerName)
		}
	},
}

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Exec a command",
	Long:  `Exec a program on a created docker container, even not started`,
	Run: func(cmd *cobra.Command, args []string) {
		// Retrieve all flags locally
		contID, _ := cmd.Flags().GetString("container")
		execCommand, _ := cmd.Flags().GetString("command")
		workingDir, _ := cmd.Flags().GetString("workdir")
		noX11, _ := cmd.Flags().GetBool("no-x11")
		recordSession, _ := cmd.Flags().GetBool("record")
		recordOutput, _ := cmd.Flags().GetString("record-output")
		desktop, _ := cmd.Flags().GetBool("desktop")
		desktopConfig, _ := cmd.Flags().GetString("desktop-config")
		desktopPass, _ := cmd.Flags().GetString("desktop-pass")
		desktopSSL, _ := cmd.Flags().GetBool("desktop-ssl")
		vpnConfig, _ := cmd.Flags().GetString("vpn")

		// If no container specified, offer interactive selection
		if contID == "" && tui.IsInteractive() {
			containers := rfdock.ListContainers("org.container.project", "rfswift")
			if len(containers) == 0 {
				common.PrintErrorMessage(fmt.Errorf("no RF Swift containers found. Create one first with: rfswift run"))
				return
			}

			// Build options: latest first with a hint
			options := make([]string, len(containers))
			for i, c := range containers {
				label := fmt.Sprintf("%s  (%s) [%s]", c.Name, c.Image, c.State)
				if i == 0 {
					label += "  ← latest"
				}
				options[i] = label
			}

			selected, err := tui.SelectOne("Select a container", options)
			if err != nil {
				common.PrintErrorMessage(fmt.Errorf("selection cancelled"))
				return
			}

			// Map selection back to container name
			for i, opt := range options {
				if opt == selected {
					contID = containers[i].Name
					break
				}
			}
		} else if contID == "" {
			// Non-interactive: fall back to latest container
			contID = rfdock.LatestContainerID()
			if contID == "" {
				common.PrintErrorMessage(fmt.Errorf("no RF Swift container found. Create one first with: rfswift run"))
				return
			}
			common.PrintInfoMessage(fmt.Sprintf("Using latest container: %s", contID))
		}

		setupX11(noX11, "", false)
		rfdock.ContainerSetShell(execCommand)
		if desktop {
			parseAndSetDesktop(desktopConfig)
			if desktopPass != "" {
				rfdock.ContainerSetDesktopPassword(desktopPass)
			}
			rfdock.ContainerSetDesktopSSL(desktopSSL)
		}
		rfdock.ContainerSetVPN(vpnConfig)
		if recordSession {
			if err := rfdock.ContainerExecWithRecording(contID, workingDir, recordOutput, execCommand); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
		} else {
			rfdock.ContainerExec(contID, workingDir)
		}
	},
}

var lastCmd = &cobra.Command{
	Use:   "last",
	Short: "Last container run",
	Long:  `Display the latest container that was run`,
	Run: func(cmd *cobra.Command, args []string) {
		filterLast, _ := cmd.Flags().GetString("filter")
		labelKey := "org.container.project"
		labelValue := "rfswift"
		rfdock.ContainerLast(filterLast, labelKey, labelValue)
	},
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install function script",
	Long:  `Install function script inside the container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		execCommand, _ := cmd.Flags().GetString("install")

		// Interactive container selection
		if contID == "" && tui.IsInteractive() {
			contID = pickContainer("Select a container to install into")
			if contID == "" {
				return
			}
		}

		rfdock.ContainerSetShell(execCommand)
		rfdock.ContainerInstallFromScript(contID)
	},
}

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit a container",
	Long:  `Commit a container with change we have made`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		image, _ := cmd.Flags().GetString("image")

		// Interactive container selection
		if contID == "" && tui.IsInteractive() {
			contID = pickContainer("Select a container to commit")
			if contID == "" {
				return
			}
		}

		// Interactive image name
		if image == "" && tui.IsInteractive() {
			// Suggest the container's current image
			containers := rfdock.ListContainers("org.container.project", "rfswift")
			for _, c := range containers {
				if c.Name == contID || c.ID == contID {
					image = c.Image
					break
				}
			}
			if image == "" {
				image = "rfswift/committed:latest"
			}
			common.PrintInfoMessage(fmt.Sprintf("Committing as image: %s", image))
		}

		if contID == "" {
			common.PrintErrorMessage(fmt.Errorf("container is required (use -c flag)"))
			return
		}
		if image == "" {
			common.PrintErrorMessage(fmt.Errorf("image name is required (use -i flag)"))
			return
		}

		rfdock.ContainerSetImage(image)
		rfdock.ContainerCommit(contID)
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a container",
	Long:  `Stop last or a particular container running`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")

		// Interactive container selection (show only running)
		if contID == "" && tui.IsInteractive() {
			containers := rfdock.ListContainers("org.container.project", "rfswift")
			var running []rfdock.ContainerInfo
			for _, c := range containers {
				if c.State == "running" {
					running = append(running, c)
				}
			}
			if len(running) == 0 {
				common.PrintInfoMessage("No running RF Swift containers found")
				return
			}

			options := make([]string, len(running))
			for i, c := range running {
				options[i] = fmt.Sprintf("%s  (%s, %s)", c.Name, c.ID, c.Image)
			}

			selected, err := tui.SelectOne("Select a container to stop", options)
			if err != nil {
				return
			}

			for i, opt := range options {
				if opt == selected {
					contID = running[i].Name
					break
				}
			}
		}

		rfdock.ContainerStop(contID)
	},
}

var renameCmd = &cobra.Command{
	Use:   "rename",
	Short: "Rename a container",
	Long:  `Rename a container by another name`,
	Run: func(cmd *cobra.Command, args []string) {
		dockerName, _ := cmd.Flags().GetString("name")
		dockerNewName, _ := cmd.Flags().GetString("destination")

		// Interactive container selection
		if dockerName == "" && tui.IsInteractive() {
			dockerName = pickContainer("Select a container to rename")
			if dockerName == "" {
				return
			}
		}

		if dockerNewName == "" {
			common.PrintErrorMessage(fmt.Errorf("new name is required (use -d flag)"))
			return
		}

		rfdock.ContainerRename(dockerName, dockerNewName)
	},
}

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a container",
	Long:  `Remove an existing container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")

		// Interactive container selection
		if contID == "" && tui.IsInteractive() {
			contID = pickContainer("Select a container to remove")
			if contID == "" {
				return
			}

			if !tui.Confirm(fmt.Sprintf("Remove container '%s'?", contID)) {
				common.PrintInfoMessage("Removal cancelled.")
				return
			}
		}

		if contID == "" {
			common.PrintErrorMessage(fmt.Errorf("container is required (use -c flag)"))
			return
		}

		rfdock.ContainerRemove(contID)
	},
}

// pickContainer shows an interactive container picker and returns the selected name.
func pickContainer(title string) string {
	containers := rfdock.ListContainers("org.container.project", "rfswift")
	if len(containers) == 0 {
		common.PrintErrorMessage(fmt.Errorf("no RF Swift containers found"))
		return ""
	}

	options := make([]string, len(containers))
	for i, c := range containers {
		options[i] = fmt.Sprintf("%s  (%s, %s) [%s]", c.Name, c.ID, c.Image, c.State)
	}

	selected, err := tui.SelectOne(title, options)
	if err != nil {
		return ""
	}

	for i, opt := range options {
		if opt == selected {
			return containers[i].Name
		}
	}
	return ""
}

// parseAndSetDesktop parses the --desktop-config flag value and configures
// desktop mode. Format: "proto:host:port" (e.g., "http:0.0.0.0:6080" or "vnc::5900").
// All parts are optional and fall back to defaults (http, 127.0.0.1, 6080).
func parseAndSetDesktop(config string) {
	proto := "http"
	host := "127.0.0.1"
	port := "6080"

	if config != "" {
		parts := strings.Split(config, ":")
		if len(parts) >= 1 && parts[0] != "" {
			p := strings.ToLower(parts[0])
			if p == "http" || p == "vnc" {
				proto = p
			} else {
				common.PrintWarningMessage(fmt.Sprintf("Unknown desktop protocol '%s', using 'http'", parts[0]))
			}
		}
		if len(parts) >= 2 && parts[1] != "" {
			host = parts[1]
		}
		if len(parts) >= 3 && parts[2] != "" {
			port = parts[2]
		}
	}

	if proto == "vnc" && port == "6080" {
		port = "5900"
	}

	rfdock.ContainerSetDesktop(proto, host, port)
}

func registerContainerCommands() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(lastCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(commitCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(renameCmd)
	rootCmd.AddCommand(removeCmd)

	runCmd.Flags().StringP("extrahosts", "x", "", "set extra hosts (default: 'pluto.local:192.168.1.2', and separate them with commas)")
	runCmd.Flags().StringP("display", "d", rfutils.GetDisplayEnv(), "set X Display (duplicates hosts's env by default)")
	runCmd.Flags().StringP("command", "e", "", "command to exec (by default: '/bin/bash')")
	runCmd.Flags().StringP("bind", "b", "", "extra bindings (separate them with commas)")
	runCmd.Flags().StringP("image", "i", "", "image (default: 'myrfswift:latest')")
	runCmd.Flags().StringP("pulseserver", "p", "tcp:127.0.0.1:34567", "PULSE SERVER TCP address (by default: tcp:127.0.0.1:34567)")
	runCmd.Flags().StringP("name", "n", "", "A docker name")
	runCmd.Flags().StringP("network", "t", "", "Network mode (default: 'host')")
	runCmd.Flags().StringP("devices", "s", "", "extra devices mapping (separate them with commas)")
	runCmd.Flags().IntP("privileged", "u", 0, "Set privilege level (1: privileged, 0: unprivileged)")
	runCmd.Flags().StringP("capabilities", "a", "", "extra capabilities (separate them with commas)")
	runCmd.Flags().StringP("cgroups", "g", "", "extra cgroup rules (separate them with commas)")
	runCmd.Flags().StringP("seccomp", "m", "", "Set Seccomp profile ('default' one used by default)")
	runCmd.Flags().Bool("no-x11", false, "Disable X11 forwarding")
	runCmd.Flags().StringP("exposedports", "z", "", "Exposed ports")
	runCmd.Flags().StringP("bindedports", "w", "", "Exposed ports")
	runCmd.Flags().Bool("record", false, "Record the container session")
	runCmd.Flags().String("record-output", "", "Output file for recording (default: auto-generated)")
	runCmd.Flags().Bool("realtime", false, "Enable realtime mode (SYS_NICE + rtprio=95 + memlock=unlimited)")
	runCmd.Flags().String("ulimits", "", "Set ulimits (e.g., 'rtprio=95,memlock=-1,nofile=1024:65536')")
	runCmd.Flags().Bool("desktop", false, "Enable remote desktop via VNC/noVNC (access GUI tools from a browser)")
	runCmd.Flags().String("desktop-config", "", "Desktop config as proto:host:port (e.g., 'http:0.0.0.0:6080' or 'vnc::5900')")
	runCmd.Flags().String("desktop-pass", "", "Set VNC password for desktop access (recommended when exposing on 0.0.0.0)")
	runCmd.Flags().Bool("desktop-ssl", false, "Enable SSL/TLS for desktop connections (auto-generates self-signed certificate)")
	runCmd.Flags().String("vpn", "", "Enable VPN inside container (wireguard:./wg0.conf, openvpn:./client.ovpn, tailscale:--auth-key=tskey-xxx, netbird:--setup-key=xxx)")
	runCmd.Flags().String("gpus", "", "GPU devices to add ('all' for all GPUs, or comma-separated IDs: '0,1')")
	runCmd.Flags().String("profile", "", "Use a preset profile (e.g., sdr-full, wifi, pentest-full). See 'rfswift profile list'")
	runCmd.Flags().String("workspace", "", "Workspace path on host (default: ~/rfswift-workspace/<name>/)")
	runCmd.Flags().Bool("no-workspace", false, "Disable automatic workspace mounting")
	runCmd.Flags().Bool("cwd", false, "Mount current working directory as workspace")

	execCmd.Flags().StringP("workdir", "w", "/root", "Working directory inside container")
	execCmd.Flags().StringP("container", "c", "", "container to run")
	execCmd.Flags().StringP("command", "e", "/bin/bash", "command to exec")
	execCmd.Flags().StringP("install", "i", "", "install from function script (e.g: 'sdrpp_soft_install')")
	execCmd.Flags().Bool("no-x11", false, "Disable X11 forwarding")
	execCmd.Flags().Bool("record", false, "Record the container session")
	execCmd.Flags().String("record-output", "", "Output file for recording (default: auto-generated)")
	execCmd.Flags().Bool("desktop", false, "Enable remote desktop via VNC/noVNC (access GUI tools from a browser)")
	execCmd.Flags().String("desktop-config", "", "Desktop config as proto:host:port (e.g., 'http:0.0.0.0:6080' or 'vnc::5900')")
	execCmd.Flags().String("desktop-pass", "", "Set VNC password for desktop access (recommended when exposing on 0.0.0.0)")
	execCmd.Flags().Bool("desktop-ssl", false, "Enable SSL/TLS for desktop connections (auto-generates self-signed certificate)")
	execCmd.Flags().String("vpn", "", "Start VPN inside container (wireguard:./wg0.conf, openvpn:./client.ovpn, tailscale, netbird)")

	lastCmd.Flags().StringP("filter", "f", "", "filter by image name")

	stopCmd.Flags().StringP("container", "c", "", "container to stop (interactive picker if omitted)")

	installCmd.Flags().StringP("install", "i", "", "function for installation")
	installCmd.Flags().StringP("container", "c", "", "container (interactive picker if omitted)")

	commitCmd.Flags().StringP("container", "c", "", "container to commit (interactive picker if omitted)")
	commitCmd.Flags().StringP("image", "i", "", "image name for commit (auto-suggested if omitted)")

	renameCmd.Flags().StringP("name", "n", "", "current container name (interactive picker if omitted)")
	renameCmd.Flags().StringP("destination", "d", "", "new container name")

	removeCmd.Flags().StringP("container", "c", "", "container to remove (interactive picker if omitted)")
}
