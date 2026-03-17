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

// setupX11 configures X11 forwarding settings for container execution, applying
// platform-specific socket bindings on Windows or enabling xhost ACLs on other systems.
//
//	in(1): bool noX11 when true, disables X11 forwarding by clearing display settings
//	in(2): string xDisplay the X display string to forward into the container (e.g. ":0")
//	in(3): bool setDisplay when true, applies the xDisplay value to the container configuration
//	out: none
func setupX11(noX11 bool, xDisplay string, setDisplay bool) {
	if noX11 {
		rfdock.ContainerSetX11("")
		rfdock.ContainerSetXDisplay("")
		return
	}
	if runtime.GOOS == "windows" {
		rfdock.ContainerSetX11("/run/desktop/mnt/host/wslg/.X11-unix:/tmp/.X11-unix,/run/desktop/mnt/host/wslg:/mnt/wslg")
	} else {
		// force xhost to add local connections ALCs, TODO: to optimize later
		rfutils.XHostEnable()
	}
	if setDisplay {
		rfdock.ContainerSetXDisplay(xDisplay)
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
		pulseServer, _ := cmd.Flags().GetString("pulseserver")
		rfutils.SetPulseCTL(pulseServer)
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

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "Display container engine information",
	Long:  `Show which container engine (Docker/Podman) is active and its status.`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.PrintEngineInfo()
	},
}

func registerHostCommands() {
	rootCmd.AddCommand(HostCmd)
	rootCmd.AddCommand(UpdateCmd)

	HostCmd.AddCommand(HostPulseAudioCmd)
	HostPulseAudioCmd.AddCommand(HostPulseAudioEnableCmd)
	HostPulseAudioCmd.AddCommand(HostPulseAudioUnloadCmd)
	HostPulseAudioEnableCmd.Flags().StringP("pulseserver", "s", "tcp:127.0.0.1:34567", "pulse server address (by default: 'tcp:127.0.0.1:34567')")
}

func init() {
	// Persistent flags
	rootCmd.PersistentFlags().String("engine", "auto",
		"Container engine to use: auto, docker, podman, lima (env: RFSWIFT_ENGINE)")
	rootCmd.PersistentFlags().BoolVarP(&common.Disconnected, "disconnect", "q", false, "Don't query updates (disconnected mode)")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		isCompletion := len(os.Args) > 1 && (os.Args[1] == "completion" || os.Args[1] == "__complete")
		if !isCompletion {
			// Initialize container engine BEFORE anything else
			engineType, _ := cmd.Flags().GetString("engine")
			if engineType != "" && engineType != "auto" {
				rfdock.SetPreferredEngine(engineType)
			}
			// Trigger detection (sets DOCKER_HOST for Podman)
			rfdock.GetEngine()

			rfutils.DisplayVersion()
		}
	}

	// Register all command groups
	registerContainerCommands()
	registerImageCommands()
	registerPropertyCommands()
	registerUpgradeBuildCommands()
	registerTransferCommands()
	registerCleanupCommands()
	registerLoggingCommands()
	registerUlimitsCommands()
	registerCompletionCommands()
	registerHostCommands()
	if runtime.GOOS == "windows" {
		registerWinUSBCommands()
	}
	if runtime.GOOS == "darwin" {
		registerMacUSBCommands()
	}
	rootCmd.AddCommand(engineCmd)
	registerNetworkCommands()
	registerProfileCommands()
	registerDoctorCommands()
}

// Execute runs the root cobra command, invoking the appropriate subcommand based on
// the provided CLI arguments, and exits with a non-zero status code on error.
//
//	out: none
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your CLI '%s'", err)
		os.Exit(1)
	}
}
