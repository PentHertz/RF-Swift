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
	rfnix "penthertz/rfswift/nix"
	rfutils "penthertz/rfswift/rfutils"
)

// isNixEngineRequested reports whether the Nix engine was selected via the
// --engine flag or the RFSWIFT_ENGINE environment variable.
func isNixEngineRequested(engineFlag string) bool {
	if strings.EqualFold(strings.TrimSpace(engineFlag), "nix") {
		return true
	}
	env := strings.TrimSpace(os.Getenv("RFSWIFT_ENGINE"))
	if strings.EqualFold(env, "nix") && (engineFlag == "" || engineFlag == "auto") {
		return true
	}
	return false
}

// isNixCommand reports whether the invoked command belongs to the `nix` group,
// which manages native environments and never needs a container engine.
func isNixCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "nix" || c.Name() == "env" {
			return true
		}
	}
	return false
}

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
	// `rfswift --version` prints the version without touching the network or
	// an engine; the Windows front end reads it from the Linux rfswift inside
	// WSL to detect version skew.
	Version: common.Version,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Use '-h' for help")
	},
}

var HostCmd = &cobra.Command{
	Use:   "host",
	Short: "Host configuration (setup, udev rules, Docker access, audio)",
	Long: `Configures the host for container and native operations. 'rfswift host setup'
walks through the opt-in steps the Linux packages leave to you: udev rules for
RF hardware, installing Docker and/or Podman, and Docker socket access that
works without logging out.`,
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
		if err := rfutils.SetPulseCTL(pulseServer); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var HostPulseAudioUnloadCmd = &cobra.Command{
	Use:   "unload",
	Short: "Unload TCP module from Pulseaudio server",
	Run: func(cmd *cobra.Command, args []string) {
		if err := rfutils.UnloadPulseCTL(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
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

func registerHostCommands() {
	rootCmd.AddCommand(HostCmd)
	rootCmd.AddCommand(UpdateCmd)

	HostCmd.AddCommand(HostPulseAudioCmd)
	registerHostSetupCommands()
	HostPulseAudioCmd.AddCommand(HostPulseAudioEnableCmd)
	HostPulseAudioCmd.AddCommand(HostPulseAudioUnloadCmd)
	HostPulseAudioEnableCmd.Flags().StringP("pulseserver", "s", "tcp:127.0.0.1:34567", "pulse server address (by default: 'tcp:127.0.0.1:34567')")
}

func init() {
	// Persistent flags
	rootCmd.PersistentFlags().String("engine", "auto",
		"Engine to use: auto, docker, podman, lima, or nix (native environments) (env: RFSWIFT_ENGINE; config: [general] engine)")
	rootCmd.PersistentFlags().Bool("gpu", false,
		"Use the GPU-accelerated Lima VM on macOS Apple Silicon (krunkit/Vulkan). Implies --engine lima; provides GPU compute but NOT USB passthrough")
	rootCmd.PersistentFlags().BoolVarP(&common.Disconnected, "disconnect", "q", false, "Don't query updates (disconnected mode)")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		isCompletion := len(os.Args) > 1 && (os.Args[1] == "completion" || os.Args[1] == "__complete")
		if !isCompletion {
			// Initialize container engine BEFORE anything else
			engineType, _ := cmd.Flags().GetString("engine")
			// --gpu selects the separate krunkit Lima VM (Vulkan via Venus/MoltenVK).
			// It exists only on Lima and is mutually exclusive with USB passthrough,
			// so it is a distinct instance ("rfswift-gpu") on a distinct backend.
			if gpu, _ := cmd.Flags().GetBool("gpu"); gpu {
				if os.Getenv("RFSWIFT_LIMA_INSTANCE") == "" {
					os.Setenv("RFSWIFT_LIMA_INSTANCE", "rfswift-gpu")
				}
				engineType = "lima"
			}

			// When neither --engine nor RFSWIFT_ENGINE picked an engine, fall back
			// to the default set in the config file ([general] engine). Precedence:
			// --engine flag > RFSWIFT_ENGINE > config file > auto.
			if engineType == "auto" && strings.TrimSpace(os.Getenv("RFSWIFT_ENGINE")) == "" {
				if e := rfutils.ConfiguredEngine(common.ConfigFileByPlatform()); e != "" && e != "auto" {
					engineType = e
				}
			}

			// Nix engine drives native environments, not a container daemon, so
			// it must not run the container engine detection below (that would
			// require Docker/Podman and print misleading messages). This covers
			// both `--engine nix` on run/exec and the `rfswift nix` group.
			if isNixEngineRequested(engineType) || isNixCommand(cmd) {
				rfnix.SetSelected(true)
				// Windows: the engine lives in a WSL 2 distribution; the Linux
				// rfswift there serves the command (does not return when it does).
				bridgeNixCommandToWSL(cmd)
				rfutils.DisplayVersion()
				return
			}

			if engineType != "" && engineType != "auto" {
				rfdock.SetPreferredEngine(engineType)
			}
			// Trigger detection (sets DOCKER_HOST for Podman)
			eng := rfdock.GetEngine()

			// No container engine at all (Linux without one, or the Windows
			// installer's "Nix only" choice): point at the Nix engine when it
			// is set up instead of leaving the user with a Docker error.
			if eng != nil && !eng.IsAvailable() && rfnix.IsAvailable() {
				common.PrintInfoMessage("The Nix engine is set up on this host: run tools natively with 'rfswift --engine nix ...', or make it the default with 'rfswift engine set nix'.")
			}

			rfutils.DisplayVersion()

			// Nudge the user when the configured repository still points at an
			// official image on an older Ubuntu base than the current one
			// (e.g. rfswift_noble while resolute is current). Runs after the
			// banner so the notice lands in the normal output flow.
			rfutils.NotifyIfOutdatedImage(rfdock.ContainerGetRepoTag())
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
	registerEngineCommands()
	registerNixCommands()
	registerNetworkCommands()
	registerProfileCommands()
	registerReportCommands()
	registerAuditCommand()
	registerDoctorCommands()
	registerAgentCommand()

	// Organize help into sections and add the config/system/usb convenience
	// parents. Must run last, once every command is registered.
	organizeCommands()
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
