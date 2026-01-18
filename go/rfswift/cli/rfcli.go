/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
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
	rfdock "penthertz/rfswift/dock"
	rfutils "penthertz/rfswift/rfutils"
)

var (
	DImage          string
	ContID          string
	ExecCmd         string
	FilterLast      string
	ExtraBind       string
	XDisplay        string
	SInstall        string
	ImageRef        string
	ImageTag        string
	ExtraHost       string
	UsbDevice       string
	PulseServer     string
	DockerName      string
	DockerNewName   string
	Bsource         string
	Btarget         string
	NetMode         string
	NetExporsedPorts string
	NetBindedPorts  string
	Devices         string
	Caps            string
	Cgroups         string
	Seccomp         string
	RecordOutput    string
	WorkingDir      string
	Privileged      int
	isADevice       bool
	NoX11           bool
	RecordSession   bool
)

func setupX11(setDisplay bool) {
	if NoX11 {
		if setDisplay {
			rfdock.DockerSetx11("")
			rfdock.DockerSetXDisplay("")
		}
		return
	}
	if runtime.GOOS == "windows" {
		rfdock.DockerSetx11("/run/desktop/mnt/host/wslg/.X11-unix:/tmp/.X11-unix,/run/desktop/mnt/host/wslg:/mnt/wslg")
	} else {
		// force xhost to add local connections ALCs, TODO: to optimize later
		rfutils.XHostEnable()
	}
	if setDisplay {
		rfdock.DockerSetXDisplay(XDisplay)
	}
}

var rootCmd = &cobra.Command{
	Use:   "rfswift",
	Short: "rfswift - you RF & HW swiss army",
	Long:  `rfswift is THE toolbox for any HAM & radiocommunications and hardware professionals`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Use '-h' for help")
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Create and run a program",
	Long:  `Create a container and run a program inside the docker container`,
	Run: func(cmd *cobra.Command, args []string) {
		setupX11(true)
		rfdock.DockerSetShell(ExecCmd)
		rfdock.DockerAddBinding(ExtraBind)
		rfdock.DockerSetImage(DImage)
		rfdock.DockerSetExtraHosts(ExtraHost)
		rfdock.DockerSetPulse(PulseServer)
		rfdock.DockerSetNetworkMode(NetMode)
		rfdock.DockerSetExposedPorts(NetExporsedPorts)
		rfdock.DockerSetBindexPorts(NetBindedPorts)
		rfdock.DockerAddDevices(Devices)
		rfdock.DockerAddCaps(Caps)
		rfdock.DockerAddCgroups(Cgroups)
		rfdock.DockerSetPrivileges(Privileged)
		rfdock.DockerSetSeccomp(Seccomp)
		if runtime.GOOS == "linux" {
			rfutils.SetPulseCTL(PulseServer)
		}

		if RecordSession {
			if err := rfdock.DockerRunWithRecording(DockerName, RecordOutput); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
		} else {
			rfdock.DockerRun(DockerName)
		}
	},
}

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Exec a command",
	Long:  `Exec a program on a created docker container, even not started`,
	Run: func(cmd *cobra.Command, args []string) {
		setupX11(false)
		rfdock.DockerSetShell(ExecCmd)
		if RecordSession {
			if err := rfdock.DockerExecWithRecording(ContID, WorkingDir, RecordOutput); err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
		} else {
			rfdock.DockerExec(ContID, WorkingDir)
		}
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

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a container",
	Long:  `Stop last or a particular container running`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DockerStop(ContID)
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

var retagCmd = &cobra.Command{
	Use:   "retag",
	Short: "Rename an image",
	Long:  `Rename an image with another tag`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DockerTag(ImageRef, ImageTag)
	},
}

var renameCmd = &cobra.Command{
	Use:   "rename",
	Short: "Rename a container",
	Long:  `Rename a container by another name`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DockerRename(DockerName, DockerNewName)
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
	Short: "RF Swift images management remote/local",
	Long:  `List local and remote images`,
}

var ImagesLocalCmd = &cobra.Command{
	Use:   "local",
	Short: "List local images",
	Long:  `List pulled and built images`,
	Run: func(cmd *cobra.Command, args []string) {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		rfdock.PrintImagesTable(labelKey, labelValue)
	},
}

var ImagesRemoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "List remote images",
	Long:  `Lists RF Swift images from official repository`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.ListDockerImagesRepo()
	},
}

var ImagesPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull a container",
	Long:  `Pull a container from internet`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DockerPull(ImageRef, ImageTag)
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

var HostCmd = &cobra.Command{
	Use:   "host",
	Short: "Host configuration",
	Long:  `Configures the host for container operations`,
}

var HostPulseAudioCmd = &cobra.Command{
	Use:   "audio",
	Short: "Pulseaudio server",
	Long:  `Manage pulseaudio server`,
}

var HostPulseAudioEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable connection",
	Long:  `Allow connections to a specific port and interface. Warning: command to be executed as user!`,
	Run: func(cmd *cobra.Command, args []string) {
		rfutils.SetPulseCTL(PulseServer)
	},
}

var HostPulseAudioUnloadCmd = &cobra.Command{
	Use:   "unload",
	Short: "Unload TCP module from Pulseaudio server",
	Run: func(cmd *cobra.Command, args []string) {
		rfutils.UnloadPulseCTL()
	},
}

var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update RF Swift",
	Long:  `Update RF Swift binary from official Penthertz' repository`,
	Run: func(cmd *cobra.Command, args []string) {
		rfutils.GetLatestRFSwift()
	},
}

var BindingsCmd = &cobra.Command{
	Use:   "bindings",
	Short: "Manage devices and volumes bindings",
	Long:  `Add, or remove, a binding for a container`,
}

var BindingsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a binding",
	Long:  `Adding a new binding for a container ID`,
	Run: func(cmd *cobra.Command, args []string) {
		if isADevice {
			rfdock.UpdateDeviceBinding(ContID, Bsource, Btarget, true)
		} else {
			rfdock.UpdateMountBinding(ContID, Bsource, Btarget, true)
		}
	},
}

var BindingsRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a binding",
	Long:  `Remove a new binding for a container ID`,
	Run: func(cmd *cobra.Command, args []string) {
		if isADevice {
			rfdock.UpdateDeviceBinding(ContID, Bsource, Btarget, false)
		} else {
			rfdock.UpdateMountBinding(ContID, Bsource, Btarget, false)
		}
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate and install completion script",
	Long: `Generate and install completion script for rfswift.
To load completions:

Bash:
  $ rfswift completion bash > /etc/bash_completion.d/rfswift
  # or
  $ rfswift completion bash > ~/.bash_completion

Zsh:
  $ rfswift completion zsh > "${fpath[1]}/_rfswift"
  # or
  $ rfswift completion zsh > ~/.zsh/completion/_rfswift

Fish:
  $ rfswift completion fish > ~/.config/fish/completions/rfswift.fish

PowerShell:
  PS> rfswift completion powershell > rfswift.ps1
`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var shell string
		if len(args) > 0 {
			shell = args[0]
		} else {
			shell = detectShell()
			common.PrintInfoMessage(fmt.Sprintf("Detected shell: %s", shell))
		}

		installCompletion(shell)
	},
}

var CapabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Manage container capabilities",
	Long:  `Add or remove capabilities for a container`,
}

var CapabilitiesAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a capability",
	Long:  `Add a new capability to a container`,
	Run: func(cmd *cobra.Command, args []string) {
		capability, _ := cmd.Flags().GetString("capability")
		rfdock.UpdateCapability(ContID, capability, true)
	},
}

var CapabilitiesRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a capability",
	Long:  `Remove a capability from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		capability, _ := cmd.Flags().GetString("capability")
		rfdock.UpdateCapability(ContID, capability, false)
	},
}

var CgroupsCmd = &cobra.Command{
	Use:   "cgroups",
	Short: "Manage container cgroup rules",
	Long:  `Add or remove cgroup device rules for a container`,
}

var CgroupsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a cgroup rule",
	Long:  `Add a new cgroup device rule to a container (e.g., 'c 189:* rwm')`,
	Run: func(cmd *cobra.Command, args []string) {
		rule, _ := cmd.Flags().GetString("rule")
		rfdock.UpdateCgroupRule(ContID, rule, true)
	},
}

var CgroupsRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a cgroup rule",
	Long:  `Remove a cgroup device rule from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		rule, _ := cmd.Flags().GetString("rule")
		rfdock.UpdateCgroupRule(ContID, rule, false)
	},
}

var PortsCmd = &cobra.Command{
	Use:   "ports",
	Short: "Manage container ports",
	Long:  `Add or remove exposed ports and port bindings for a container`,
}

var PortsExposeCmd = &cobra.Command{
	Use:   "expose",
	Short: "Expose a port",
	Long:  `Expose a new port on a container (e.g., '8080/tcp')`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetString("port")
		rfdock.UpdateExposedPort(ContID, port, true)
	},
}

var PortsUnexposeCmd = &cobra.Command{
	Use:   "unexpose",
	Short: "Remove an exposed port",
	Long:  `Remove an exposed port from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetString("port")
		rfdock.UpdateExposedPort(ContID, port, false)
	},
}

var PortsBindCmd = &cobra.Command{
	Use:   "bind",
	Short: "Bind a port",
	Long:  `Bind a container port to a host port (e.g., '8080/tcp:8080' or '8080/tcp:127.0.0.1:8080')`,
	Run: func(cmd *cobra.Command, args []string) {
		binding, _ := cmd.Flags().GetString("binding")
		rfdock.UpdatePortBinding(ContID, binding, true)
	},
}

var PortsUnbindCmd = &cobra.Command{
	Use:   "unbind",
	Short: "Remove a port binding",
	Long:  `Remove a port binding from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		binding, _ := cmd.Flags().GetString("binding")
		rfdock.UpdatePortBinding(ContID, binding, false)
	},
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade container to a new/latest/another image",
	Long: `Upgrade a container by pulling a new image and recreating the container with preserved repositories.
This follows the Exegol upgrade pattern: pull new image → create new container → inherit name.

Examples:
  # Upgrade to latest version (no repositories preserved)
  rfswift upgrade -c mycontainer

  # Upgrade to specific image (no repositories preserved)
  rfswift upgrade -c mycontainer -i telecom_15012025

  # Upgrade keeping specific repositories
  rfswift upgrade -c mycontainer -i telecom_15012025 -r /root/test,/root/share,/opt/tools

  # Downgrade to previous version
  rfswift upgrade -c mycontainer -i telecom_10102024`,
	Run: func(cmd *cobra.Command, args []string) {
		containerName, _ := cmd.Flags().GetString("container")
		repositories, _ := cmd.Flags().GetString("repositories")
		imageName, _ := cmd.Flags().GetString("image")

		if containerName == "" {
			common.PrintErrorMessage(fmt.Errorf("container name (-c) is required"))
			cmd.Help()
			os.Exit(1)
		}

		if err := rfdock.DockerUpgrade(containerName, repositories, imageName); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an image from a recipe",
	Long:  `Build a Docker image from a simplified YAML recipe file`,
	Run: func(cmd *cobra.Command, args []string) {
		recipeFile, _ := cmd.Flags().GetString("recipe")
		tagName, _ := cmd.Flags().GetString("tag")
		noCache, _ := cmd.Flags().GetBool("no-cache")

		if err := rfdock.BuildFromRecipe(recipeFile, tagName, noCache); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var ExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export containers or images",
	Long:  `Export containers or images to tar.gz files for backup or transfer`,
}

var ExportContainerCmd = &cobra.Command{
	Use:   "container",
	Short: "Export a container to tar.gz",
	Long:  `Export a container's filesystem to a compressed tar.gz file`,
	Run: func(cmd *cobra.Command, args []string) {
		outputFile, _ := cmd.Flags().GetString("output")
		if err := rfdock.ExportContainer(ContID, outputFile); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var ExportImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Export an image to tar.gz",
	Long:  `Export one or more images to a compressed tar.gz file`,
	Run: func(cmd *cobra.Command, args []string) {
		outputFile, _ := cmd.Flags().GetString("output")
		images, _ := cmd.Flags().GetStringSlice("images")
		if err := rfdock.ExportImage(images, outputFile); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var ImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import containers or images",
	Long:  `Import containers or images from tar.gz files`,
}

var ImportContainerCmd = &cobra.Command{
	Use:   "container",
	Short: "Import a container from tar.gz",
	Long:  `Import a container filesystem from a tar.gz file and create an image`,
	Run: func(cmd *cobra.Command, args []string) {
		inputFile, _ := cmd.Flags().GetString("input")
		imageName, _ := cmd.Flags().GetString("name")
		if err := rfdock.ImportContainer(inputFile, imageName); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var ImportImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Import an image from tar.gz",
	Long:  `Import one or more images from a tar.gz file`,
	Run: func(cmd *cobra.Command, args []string) {
		inputFile, _ := cmd.Flags().GetString("input")
		if err := rfdock.ImportImage(inputFile); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var DownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download and save an image to tar.gz",
	Long:  `Download an image from a repository and save it locally as a compressed tar.gz file`,
	Run: func(cmd *cobra.Command, args []string) {
		imageName, _ := cmd.Flags().GetString("image")
		outputFile, _ := cmd.Flags().GetString("output")
		pullFirst, _ := cmd.Flags().GetBool("pull")

		if err := rfdock.SaveImageToFile(imageName, outputFile, pullFirst); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var CleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up containers and images",
	Long:  `Remove old or unused containers and images based on age filters`,
}

var CleanupAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Clean both containers and images",
	Long:  `Remove both old containers and images`,
	Run: func(cmd *cobra.Command, args []string) {
		olderThan, _ := cmd.Flags().GetString("older-than")
		force, _ := cmd.Flags().GetBool("force")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if err := rfdock.CleanupAll(olderThan, force, dryRun); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var CleanupContainersCmd = &cobra.Command{
	Use:   "containers",
	Short: "Clean containers only",
	Long:  `Remove old containers based on age filter`,
	Run: func(cmd *cobra.Command, args []string) {
		olderThan, _ := cmd.Flags().GetString("older-than")
		force, _ := cmd.Flags().GetBool("force")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		onlyStopped, _ := cmd.Flags().GetBool("stopped")

		if err := rfdock.CleanupContainers(olderThan, force, dryRun, onlyStopped); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var CleanupImagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Clean images only",
	Long:  `Remove old images based on age filter`,
	Run: func(cmd *cobra.Command, args []string) {
		olderThan, _ := cmd.Flags().GetString("older-than")
		force, _ := cmd.Flags().GetBool("force")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		onlyDangling, _ := cmd.Flags().GetBool("dangling")
		pruneChildren, _ := cmd.Flags().GetBool("prune-children")

		if err := rfdock.CleanupImages(olderThan, force, dryRun, onlyDangling, pruneChildren); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var LogCmd = &cobra.Command{
	Use:   "log",
	Short: "Record and replay terminal sessions",
	Long:  `Record RF Swift operations using asciinema or script command`,
}

var LogStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start recording a session",
	Long:  `Start recording terminal session to a file`,
	Run: func(cmd *cobra.Command, args []string) {
		outputFile, _ := cmd.Flags().GetString("output")
		useScript, _ := cmd.Flags().GetBool("use-script")

		if err := rfdock.StartLogging(outputFile, useScript); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var LogStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current recording",
	Long:  `Stop the active recording session`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := rfdock.StopLogging(); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var LogReplayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay a recorded session",
	Long:  `Replay a previously recorded session`,
	Run: func(cmd *cobra.Command, args []string) {
		inputFile, _ := cmd.Flags().GetString("input")
		speed, _ := cmd.Flags().GetFloat64("speed")

		if err := rfdock.ReplayLog(inputFile, speed); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var LogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recorded sessions",
	Long:  `List all recorded session files`,
	Run: func(cmd *cobra.Command, args []string) {
		logDir, _ := cmd.Flags().GetString("dir")
		if err := rfdock.ListLogs(logDir); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			// Default to PowerShell on Windows
			return "powershell"
		}
		// Default to bash
		return "bash"
	}

	// Extract the shell name from the path
	shell = filepath.Base(shell)
	switch shell {
	case "bash", "zsh", "fish":
		return shell
	default:
		return "bash" // Default to bash
	}
}

func installCompletion(shell string) {
	var err error
	var dir string
	var filename string

	fmt.Println("🔍 Finding appropriate completion directory for " + shell + "...")

	switch shell {
	case "bash":
		// Try common bash completion directories
		if runtime.GOOS == "darwin" {
			// macOS often uses homebrew's bash completion
			if _, err := os.Stat("/usr/local/etc/bash_completion.d"); err == nil {
				dir = "/usr/local/etc/bash_completion.d"
			} else {
				// Fallback to user's home directory
				dir = filepath.Join(os.Getenv("HOME"), ".bash_completion.d")
				os.MkdirAll(dir, 0755)
			}
		} else {
			// Linux
			if _, err := os.Stat("/etc/bash_completion.d"); err == nil {
				dir = "/etc/bash_completion.d"
			} else {
				// Fallback to user's home directory
				dir = filepath.Join(os.Getenv("HOME"), ".bash_completion.d")
				os.MkdirAll(dir, 0755)
			}
		}
		filename = "rfswift"

	case "zsh":
		// Try common zsh completion directories
		var zshCompletionDirs []string
		homeDir := os.Getenv("HOME")

		// Check fpath directories
		fpathCmd := exec.Command("zsh", "-c", "echo ${fpath[1]}")
		fpathOutput, err := fpathCmd.Output()
		if err == nil && len(fpathOutput) > 0 {
			zshCompletionDirs = append(zshCompletionDirs, strings.TrimSpace(string(fpathOutput)))
		}

		// Common locations
		zshCompletionDirs = append(zshCompletionDirs,
			filepath.Join(homeDir, ".zsh/completion"),
			filepath.Join(homeDir, ".oh-my-zsh/completions"),
			"/usr/local/share/zsh/site-functions",
			"/usr/share/zsh/vendor-completions",
		)

		// Find first existing directory
		for _, d := range zshCompletionDirs {
			if _, err := os.Stat(d); err == nil {
				dir = d
				common.PrintInfoMessage(fmt.Sprintf("Found existing completion directory: %s", dir))
				break
			}
		}

		// If no directory exists, create one
		if dir == "" {
			dir = filepath.Join(homeDir, ".zsh/completion")
			common.PrintInfoMessage(fmt.Sprintf("Creating completion directory: %s", dir))
			os.MkdirAll(dir, 0755)
		}
		filename = "_rfswift"

	case "fish":
		// Fish completion directory
		dir = filepath.Join(os.Getenv("HOME"), ".config/fish/completions")
		os.MkdirAll(dir, 0755)
		filename = "rfswift.fish"

	case "powershell":
		// PowerShell profile directory
		output, err := exec.Command("powershell", "-Command", "echo $PROFILE").Output()
		if err == nil {
			profileDir := filepath.Dir(strings.TrimSpace(string(output)))
			dir = filepath.Join(profileDir, "CompletionScripts")
		} else {
			dir = filepath.Join(os.Getenv("USERPROFILE"), "Documents", "WindowsPowerShell", "CompletionScripts")
		}
		os.MkdirAll(dir, 0755)
		filename = "rfswift.ps1"

	default:
		common.PrintErrorMessage(fmt.Errorf("Unsupported shell: %s", shell))
		os.Exit(1)
	}

	filepath := filepath.Join(dir, filename)
	fmt.Println("📝 Installing completion script to " + filepath)

	file, err := os.Create(filepath)
	if err != nil {
		if os.IsPermission(err) {
			common.PrintErrorMessage(fmt.Errorf("Permission denied when writing to %s", filepath))
			common.PrintWarningMessage("Try running with sudo or choose a different directory.")
		} else {
			common.PrintErrorMessage(fmt.Errorf("Error creating file: %v", err))
		}
		os.Exit(1)
	}
	defer file.Close()

	// Generate completion script
	common.PrintInfoMessage(fmt.Sprintf("Generating completion script for %s...", shell))

	switch shell {
	case "bash":
		rootCmd.GenBashCompletion(file)
	case "zsh":
		rootCmd.GenZshCompletion(file)
		// Add compdef line at the beginning
		content, err := os.ReadFile(filepath)
		if err == nil {
			newContent := []byte("#compdef rfswift\n" + string(content))
			os.WriteFile(filepath, newContent, 0644)
		}
	case "fish":
		rootCmd.GenFishCompletion(file, true)
	case "powershell":
		rootCmd.GenPowerShellCompletion(file)
	}

	os.Chmod(filepath, 0644)
	common.PrintSuccessMessage(fmt.Sprintf("Completion script installed successfully to %s", filepath))

	// Instructions for shell configuration
	fmt.Println("\n📋 Configuration Instructions:")

	switch shell {
	case "zsh":
		common.PrintInfoMessage("To enable completions, add the following to your ~/.zshrc:")
		fmt.Println("fpath=(" + dir + " $fpath)")
		fmt.Println("autoload -Uz compinit")
		fmt.Println("compinit")
		common.PrintInfoMessage("Then restart your shell or run: source ~/.zshrc")
	case "bash":
		common.PrintInfoMessage("To enable completions, add the following to your ~/.bashrc:")
		fmt.Printf("[[ -f %s ]] && source %s\n", filepath, filepath)
		common.PrintInfoMessage("Then restart your shell or run: source ~/.bashrc")
	case "fish":
		common.PrintSuccessMessage("Completions should be automatically loaded by fish.")
	case "powershell":
		common.PrintInfoMessage("To enable completions, add the following to your PowerShell profile:")
		fmt.Printf(". '%s'\n", filepath)
	}

	fmt.Println("\n🚀 Happy tab-completing with rfswift!")
}

func init() {
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(lastCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(commitCmd)
	rootCmd.AddCommand(renameCmd)
	rootCmd.AddCommand(retagCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(ImagesCmd)
	rootCmd.AddCommand(DeleteCmd)
	rootCmd.AddCommand(HostCmd)
	rootCmd.AddCommand(UpdateCmd)
	rootCmd.AddCommand(BindingsCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(CapabilitiesCmd)
	rootCmd.AddCommand(CgroupsCmd)
	rootCmd.AddCommand(PortsCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(ExportCmd)
	rootCmd.AddCommand(ImportCmd)
	rootCmd.AddCommand(DownloadCmd)
	rootCmd.AddCommand(CleanupCmd)
	rootCmd.AddCommand(LogCmd)

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		isCompletion := len(os.Args) > 1 && (os.Args[1] == "completion" || os.Args[1] == "__complete")
		if !isCompletion {
			rfutils.DisplayVersion()
		}
	}

	rootCmd.PersistentFlags().BoolVarP(&common.Disconnected, "disconnect", "q", false, "Don't query updates (disconnected mode)")

	if runtime.GOOS == "windows" {
		rootCmd.AddCommand(winusbCmd)
		winusbCmd.AddCommand(winusblistCmd)
		winusbCmd.AddCommand(winusbattachCmd)
		winusbCmd.AddCommand(winusbdetachCmd)
		winusbattachCmd.Flags().StringVarP(&UsbDevice, "busid", "i", "", "busid")
		winusbdetachCmd.Flags().StringVarP(&UsbDevice, "busid", "i", "", "busid")
	}

	ImagesCmd.AddCommand(pullCmd)
	ImagesCmd.AddCommand(ImagesRemoteCmd)
	ImagesCmd.AddCommand(ImagesLocalCmd)
	pullCmd.Flags().StringVarP(&ImageRef, "image", "i", "", "image reference")
	pullCmd.Flags().StringVarP(&ImageTag, "tag", "t", "", "rename to target tag")
	pullCmd.MarkFlagRequired("image")

	HostCmd.AddCommand(HostPulseAudioCmd)
	HostPulseAudioCmd.AddCommand(HostPulseAudioEnableCmd)
	HostPulseAudioCmd.AddCommand(HostPulseAudioUnloadCmd)
	HostPulseAudioEnableCmd.Flags().StringVarP(&PulseServer, "pulseserver", "s", "tcp:127.0.0.1:34567", "pulse server address (by default: 'tcp:127.0.0.1:34567')")

	DeleteCmd.Flags().StringVarP(&DImage, "image", "i", "", "image ID or tag")
	removeCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to remove")
	installCmd.Flags().StringVarP(&ExecCmd, "install", "i", "", "function for installation")
	installCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")

	retagCmd.Flags().StringVarP(&ImageRef, "image", "i", "", "image reference")
	retagCmd.Flags().StringVarP(&ImageTag, "tag", "t", "", "rename to target tag")
	renameCmd.Flags().StringVarP(&DockerName, "name", "n", "", "Docker current name")
	renameCmd.Flags().StringVarP(&DockerNewName, "destination", "d", "", "Docker new name")
	commitCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
	commitCmd.Flags().StringVarP(&DImage, "image", "i", "", "image (default: 'myrfswift:latest')")
	commitCmd.MarkFlagRequired("container")
	commitCmd.MarkFlagRequired("image")
	execCmd.Flags().StringVarP(&WorkingDir, "workdir", "w", "/root", "Working directory inside container")
	execCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
	execCmd.Flags().StringVarP(&ExecCmd, "command", "e", "/bin/bash", "command to exec")
	execCmd.Flags().StringVarP(&SInstall, "install", "i", "", "install from function script (e.g: 'sdrpp_soft_install')")
	execCmd.Flags().BoolVar(&NoX11, "no-x11", false, "Disable X11 forwarding")
	execCmd.Flags().BoolVar(&RecordSession, "record", false, "Record the container session")
	execCmd.Flags().StringVar(&RecordOutput, "record-output", "", "Output file for recording (default: auto-generated)")

	runCmd.Flags().StringVarP(&ExtraHost, "extrahosts", "x", "", "set extra hosts (default: 'pluto.local:192.168.1.2', and separate them with commas)")
	runCmd.Flags().StringVarP(&XDisplay, "display", "d", rfutils.GetDisplayEnv(), "set X Display (duplicates hosts's env by default)")
	runCmd.Flags().StringVarP(&ExecCmd, "command", "e", "", "command to exec (by default: '/bin/bash')")
	runCmd.Flags().StringVarP(&ExtraBind, "bind", "b", "", "extra bindings (separate them with commas)")
	runCmd.Flags().StringVarP(&DImage, "image", "i", "", "image (default: 'myrfswift:latest')")
	runCmd.Flags().StringVarP(&PulseServer, "pulseserver", "p", "tcp:127.0.0.1:34567", "PULSE SERVER TCP address (by default: tcp:127.0.0.1:34567)")
	runCmd.Flags().StringVarP(&DockerName, "name", "n", "", "A docker name")
	runCmd.Flags().StringVarP(&NetMode, "network", "t", "", "Network mode (default: 'host')")
	runCmd.Flags().StringVarP(&Devices, "devices", "s", "", "extra devices mapping (separate them with commas)")
	runCmd.Flags().IntVarP(&Privileged, "privileged", "u", 0, "Set privilege level (1: privileged, 0: unprivileged)")
	runCmd.Flags().StringVarP(&Caps, "capabilities", "a", "", "extra capabilities (separate them with commas)")
	runCmd.Flags().StringVarP(&Cgroups, "cgroups", "g", "", "extra cgroup rules (separate them with commas)")
	runCmd.Flags().StringVarP(&Seccomp, "seccomp", "m", "", "Set Seccomp profile ('default' one used by default)")
	runCmd.Flags().BoolVar(&NoX11, "no-x11", false, "Disable X11 forwarding")
	runCmd.MarkFlagRequired("name")

	runCmd.Flags().StringVarP(&NetExporsedPorts, "exposedports", "z", "", "Exposed ports")
	runCmd.Flags().StringVarP(&NetBindedPorts, "bindedports", "w", "", "Exposed ports")
	runCmd.Flags().BoolVar(&RecordSession, "record", false, "Record the container session")
	runCmd.Flags().StringVar(&RecordOutput, "record-output", "", "Output file for recording (default: auto-generated)")

	lastCmd.Flags().StringVarP(&FilterLast, "filter", "f", "", "filter by image name")

	stopCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to stop")

	BindingsCmd.AddCommand(BindingsAddCmd)
	BindingsCmd.PersistentFlags().BoolVarP(&isADevice, "devices", "d", false, "Manage a device rather than a volume")
	BindingsCmd.AddCommand(BindingsRmCmd)
	BindingsAddCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
	BindingsAddCmd.Flags().StringVarP(&Bsource, "source", "s", "", "source binding (by default: source=target)")
	BindingsAddCmd.Flags().StringVarP(&Btarget, "target", "t", "", "target binding")
	BindingsAddCmd.MarkFlagRequired("container")
	BindingsAddCmd.MarkFlagRequired("target")
	BindingsRmCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
	BindingsRmCmd.Flags().StringVarP(&Bsource, "source", "s", "", "source binding (by default: source=target)")
	BindingsRmCmd.Flags().StringVarP(&Btarget, "target", "t", "", "target binding")
	BindingsRmCmd.MarkFlagRequired("container")
	BindingsRmCmd.MarkFlagRequired("target")

	// Capabilities configuration
	CapabilitiesCmd.AddCommand(CapabilitiesAddCmd)
	CapabilitiesCmd.AddCommand(CapabilitiesRmCmd)
	CapabilitiesAddCmd.Flags().StringVarP(&ContID, "container", "c", "", "container ID or name")
	CapabilitiesAddCmd.Flags().StringP("capability", "p", "", "capability to add (e.g., NET_ADMIN, SYS_PTRACE)")
	CapabilitiesAddCmd.MarkFlagRequired("container")
	CapabilitiesAddCmd.MarkFlagRequired("capability")
	CapabilitiesRmCmd.Flags().StringVarP(&ContID, "container", "c", "", "container ID or name")
	CapabilitiesRmCmd.Flags().StringP("capability", "p", "", "capability to remove")
	CapabilitiesRmCmd.MarkFlagRequired("container")
	CapabilitiesRmCmd.MarkFlagRequired("capability")

	// Cgroups configuration
	CgroupsCmd.AddCommand(CgroupsAddCmd)
	CgroupsCmd.AddCommand(CgroupsRmCmd)
	CgroupsAddCmd.Flags().StringVarP(&ContID, "container", "c", "", "container ID or name")
	CgroupsAddCmd.Flags().StringP("rule", "r", "", "cgroup rule to add (e.g., 'c 189:* rwm')")
	CgroupsAddCmd.MarkFlagRequired("container")
	CgroupsAddCmd.MarkFlagRequired("rule")
	CgroupsRmCmd.Flags().StringVarP(&ContID, "container", "c", "", "container ID or name")
	CgroupsRmCmd.Flags().StringP("rule", "r", "", "cgroup rule to remove")
	CgroupsRmCmd.MarkFlagRequired("container")
	CgroupsRmCmd.MarkFlagRequired("rule")

	// Ports configuration
	PortsCmd.AddCommand(PortsExposeCmd)
	PortsCmd.AddCommand(PortsUnexposeCmd)
	PortsCmd.AddCommand(PortsBindCmd)
	PortsCmd.AddCommand(PortsUnbindCmd)
	PortsExposeCmd.Flags().StringVarP(&ContID, "container", "c", "", "container ID or name")
	PortsExposeCmd.Flags().StringP("port", "p", "", "port to expose (e.g., '8080/tcp')")
	PortsExposeCmd.MarkFlagRequired("container")
	PortsExposeCmd.MarkFlagRequired("port")
	PortsUnexposeCmd.Flags().StringVarP(&ContID, "container", "c", "", "container ID or name")
	PortsUnexposeCmd.Flags().StringP("port", "p", "", "port to remove")
	PortsUnexposeCmd.MarkFlagRequired("container")
	PortsUnexposeCmd.MarkFlagRequired("port")
	PortsBindCmd.Flags().StringVarP(&ContID, "container", "c", "", "container ID or name")
	PortsBindCmd.Flags().StringP("binding", "b", "", "port binding (e.g., '8080/tcp:8080' or '8080/tcp:127.0.0.1:8080')")
	PortsBindCmd.MarkFlagRequired("container")
	PortsBindCmd.MarkFlagRequired("binding")
	PortsUnbindCmd.Flags().StringVarP(&ContID, "container", "c", "", "container ID or name")
	PortsUnbindCmd.Flags().StringP("binding", "b", "", "port binding to remove")
	PortsUnbindCmd.MarkFlagRequired("container")
	PortsUnbindCmd.MarkFlagRequired("binding")

	upgradeCmd.Flags().StringP("container", "c", "", "Container name or ID to upgrade (required)")
	upgradeCmd.Flags().StringP("repositories", "r", "", "Comma-separated list of container directories to preserve (e.g., /root/share,/opt/tools). These directories will be copied from old container to new container")
	upgradeCmd.Flags().StringP("image", "i", "", "Target image name/tag (if not specified, uses 'latest')")
	upgradeCmd.MarkFlagRequired("container")

	// Build command flags
	buildCmd.Flags().StringP("recipe", "r", "rfswift-recipe.yaml", "Path to the recipe file")
	buildCmd.Flags().StringP("tag", "t", "", "Override the tag name from recipe")
	buildCmd.Flags().Bool("no-cache", false, "Build without using cache")

	ExportCmd.AddCommand(ExportContainerCmd)
	ExportCmd.AddCommand(ExportContainerCmd)
	ExportCmd.AddCommand(ExportImageCmd)
	ImportCmd.AddCommand(ImportContainerCmd)
	ImportCmd.AddCommand(ImportImageCmd)

	// Export container flags
	ExportContainerCmd.Flags().StringVarP(&ContID, "container", "c", "", "container ID or name to export")
	ExportContainerCmd.Flags().StringP("output", "o", "", "output file path (e.g., mycontainer.tar.gz)")
	ExportContainerCmd.MarkFlagRequired("container")
	ExportContainerCmd.MarkFlagRequired("output")

	// Export image flags
	ExportImageCmd.Flags().StringSliceP("images", "i", []string{}, "image name(s) to export (can specify multiple)")
	ExportImageCmd.Flags().StringP("output", "o", "", "output file path (e.g., myimages.tar.gz)")
	ExportImageCmd.MarkFlagRequired("images")
	ExportImageCmd.MarkFlagRequired("output")

	// Import container flags
	ImportContainerCmd.Flags().StringP("input", "i", "", "input tar.gz file path")
	ImportContainerCmd.Flags().StringP("name", "n", "", "name for the imported image (e.g., myimage:tag)")
	ImportContainerCmd.MarkFlagRequired("input")
	ImportContainerCmd.MarkFlagRequired("name")

	// Import image flags
	ImportImageCmd.Flags().StringP("input", "i", "", "input tar.gz file path")
	ImportImageCmd.MarkFlagRequired("input")

	// Download image to file
	DownloadCmd.Flags().StringP("image", "i", "", "image name to download (e.g., penthertz/rfswift:latest)")
	DownloadCmd.Flags().StringP("output", "o", "", "output file path (e.g., rfswift-latest.tar.gz)")
	DownloadCmd.Flags().Bool("pull", false, "pull image first if not present locally")
	DownloadCmd.MarkFlagRequired("image")
	DownloadCmd.MarkFlagRequired("output")

	// Cleanup configuration
	CleanupCmd.AddCommand(CleanupAllCmd)
	CleanupCmd.AddCommand(CleanupContainersCmd)
	CleanupCmd.AddCommand(CleanupImagesCmd)

	// Cleanup all flags
	CleanupAllCmd.Flags().String("older-than", "", "Remove items older than duration (e.g., '24h', '7d', '1m', '1y')")
	CleanupAllCmd.Flags().Bool("force", false, "Don't ask for confirmation")
	CleanupAllCmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting")

	// Cleanup containers flags
	CleanupContainersCmd.Flags().String("older-than", "", "Remove containers older than duration (e.g., '24h', '7d', '1m', '1y')")
	CleanupContainersCmd.Flags().Bool("force", false, "Don't ask for confirmation")
	CleanupContainersCmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting")
	CleanupContainersCmd.Flags().Bool("stopped", false, "Only remove stopped containers")

	// Cleanup images flags
	CleanupImagesCmd.Flags().String("older-than", "", "Remove images older than duration (e.g., '24h', '7d', '1m', '1y')")
	CleanupImagesCmd.Flags().Bool("force", false, "Don't ask for confirmation")
	CleanupImagesCmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting")
	CleanupImagesCmd.Flags().Bool("dangling", false, "Only remove dangling (untagged) images")
	CleanupImagesCmd.Flags().Bool("prune-children", false, "Also remove dependent child images")

	// Logging configuration
	LogCmd.AddCommand(LogStartCmd)
	LogCmd.AddCommand(LogStopCmd)
	LogCmd.AddCommand(LogReplayCmd)
	LogCmd.AddCommand(LogListCmd)

	// Log start flags
	LogStartCmd.Flags().StringP("output", "o", "", "output file (default: rfswift-session-YYYYMMDD-HHMMSS.cast)")
	LogStartCmd.Flags().Bool("use-script", false, "force use of 'script' command instead of asciinema")

	// Log replay flags
	LogReplayCmd.Flags().StringP("input", "i", "", "input file to replay")
	LogReplayCmd.Flags().Float64P("speed", "s", 1.0, "playback speed (e.g., 2.0 for 2x)")
	LogReplayCmd.MarkFlagRequired("input")

	// Log list flags
	LogListCmd.Flags().String("dir", "", "directory to search (default: current directory)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your CLI '%s'", err)
		os.Exit(1)
	}
}
