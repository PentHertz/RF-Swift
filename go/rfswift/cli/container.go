/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	rfutils "penthertz/rfswift/rfutils"
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
			if runtime.GOOS == "linux" {
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

		setupX11(noX11, "", false)
		rfdock.ContainerSetShell(execCommand)
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
		rfdock.ContainerRename(dockerName, dockerNewName)
	},
}

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a container",
	Long:  `Remore an existing container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		rfdock.ContainerRemove(contID)
	},
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
	runCmd.MarkFlagRequired("name")
	runCmd.Flags().StringP("exposedports", "z", "", "Exposed ports")
	runCmd.Flags().StringP("bindedports", "w", "", "Exposed ports")
	runCmd.Flags().Bool("record", false, "Record the container session")
	runCmd.Flags().String("record-output", "", "Output file for recording (default: auto-generated)")
	runCmd.Flags().Bool("realtime", false, "Enable realtime mode (SYS_NICE + rtprio=95 + memlock=unlimited)")
	runCmd.Flags().String("ulimits", "", "Set ulimits (e.g., 'rtprio=95,memlock=-1,nofile=1024:65536')")

	execCmd.Flags().StringP("workdir", "w", "/root", "Working directory inside container")
	execCmd.Flags().StringP("container", "c", "", "container to run")
	execCmd.Flags().StringP("command", "e", "/bin/bash", "command to exec")
	execCmd.Flags().StringP("install", "i", "", "install from function script (e.g: 'sdrpp_soft_install')")
	execCmd.Flags().Bool("no-x11", false, "Disable X11 forwarding")
	execCmd.Flags().Bool("record", false, "Record the container session")
	execCmd.Flags().String("record-output", "", "Output file for recording (default: auto-generated)")

	lastCmd.Flags().StringP("filter", "f", "", "filter by image name")

	stopCmd.Flags().StringP("container", "c", "", "container to stop")

	installCmd.Flags().StringP("install", "i", "", "function for installation")
	installCmd.Flags().StringP("container", "c", "", "container to run")

	commitCmd.Flags().StringP("container", "c", "", "container to run")
	commitCmd.Flags().StringP("image", "i", "", "image (default: 'myrfswift:latest')")
	commitCmd.MarkFlagRequired("container")
	commitCmd.MarkFlagRequired("image")

	renameCmd.Flags().StringP("name", "n", "", "Docker current name")
	renameCmd.Flags().StringP("destination", "d", "", "Docker new name")

	removeCmd.Flags().StringP("container", "c", "", "container to remove")
}
