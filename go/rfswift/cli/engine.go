/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*  Engine management commands — Lima VM lifecycle on macOS
 */

package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	rfdock "penthertz/rfswift/dock"
	rfutils "penthertz/rfswift/rfutils"
	"penthertz/rfswift/tui"
)

var engineLimaInstance string

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "Container engine information and management",
	Long:  `Show which container engine (Docker/Podman/Lima) is active and manage its lifecycle.`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.PrintEngineInfo()
	},
}

var engineLimaCmd = &cobra.Command{
	Use:   "lima",
	Short: "Manage the Lima VM",
	Long: `Manage the Lima QEMU VM used by the Lima engine on macOS.

Lima provides a Docker-in-VM setup with USB passthrough support. These commands
let you reconfigure, reset, or inspect the VM without using limactl directly.`,
}

var engineLimaReconfigCmd = &cobra.Command{
	Use:   "reconfig",
	Short: "Apply an updated YAML configuration to the Lima VM",
	Long: `Stops the Lima VM, applies the new YAML template, and restarts it.

This is useful after modifying the Lima template (e.g., changing CPU, memory,
port forwards, or provisioning scripts). The VM filesystem is preserved.

Use --force for changes that require a full VM recreation (disk size, base image).
With --force, the VM is deleted and recreated — all data inside the VM is lost,
but Docker images can be re-pulled.

Template search order:
  1. --template flag (explicit path)
  2. <binary_dir>/lima/rfswift.yaml
  3. ~/.config/rfswift/lima.yaml
  4. ~/.rfswift/lima.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")
		templatePath, _ := cmd.Flags().GetString("template")

		lima := &rfdock.LimaEngine{}

		// Resolve template
		if templatePath == "" {
			templatePath = lima.FindTemplate()
		}
		if templatePath == "" {
			tui.PrintError("No Lima template found.")
			fmt.Println("  Provide one with --template or place it at:")
			fmt.Println("    ~/.config/rfswift/lima.yaml")
			fmt.Println("    ~/.rfswift/lima.yaml")
			return
		}

		tui.PrintInfo(fmt.Sprintf("Using template: %s", templatePath))

		if force {
			if tui.IsInteractive() {
				if !tui.Confirm("This will DELETE the VM and recreate it. All data inside the VM will be lost. Continue?") {
					fmt.Println("Cancelled.")
					return
				}
			}
		}

		if err := lima.ReconfigureInstance(templatePath, force); err != nil {
			tui.PrintError(fmt.Sprintf("Reconfiguration failed: %v", err))
			return
		}
	},
}

var engineLimaResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete and recreate the Lima VM from scratch",
	Long: `Completely removes the Lima VM and creates a fresh one from the YAML template.

All data inside the VM is lost (Docker images, containers, etc.).
This is equivalent to 'reconfig --force' but also works when no instance exists yet.`,
	Run: func(cmd *cobra.Command, args []string) {
		templatePath, _ := cmd.Flags().GetString("template")

		lima := &rfdock.LimaEngine{}

		if templatePath == "" {
			templatePath = lima.FindTemplate()
		}
		if templatePath == "" {
			tui.PrintError("No Lima template found.")
			fmt.Println("  Provide one with --template or place it at:")
			fmt.Println("    ~/.config/rfswift/lima.yaml")
			fmt.Println("    ~/.rfswift/lima.yaml")
			return
		}

		tui.PrintInfo(fmt.Sprintf("Using template: %s", templatePath))

		if tui.IsInteractive() {
			if !tui.Confirm("This will DELETE the existing VM (if any) and create a fresh one. Continue?") {
				fmt.Println("Cancelled.")
				return
			}
		}

		if err := lima.ResetInstance(templatePath); err != nil {
			tui.PrintError(fmt.Sprintf("Reset failed: %v", err))
			return
		}
	},
}

var engineLimaStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Lima VM status and configuration",
	Long:  `Displays the Lima VM instance details: status, resources, config path, and Docker readiness.`,
	Run: func(cmd *cobra.Command, args []string) {
		instance := engineLimaInstance

		if !rfutils.IsLimaInstalled() {
			tui.PrintError("Lima is not installed (install with: brew install lima qemu)")
			return
		}

		items := []tui.PropertyItem{
			{Key: "Instance", Value: instance, ValueColor: tui.ColorPrimary},
		}

		// Running status
		if rfutils.IsLimaInstanceRunning(instance) {
			items = append(items, tui.PropertyItem{Key: "Status", Value: "Running", ValueColor: tui.ColorSuccess})
		} else {
			items = append(items, tui.PropertyItem{Key: "Status", Value: "Stopped", ValueColor: tui.ColorDanger})
		}

		// Config path
		configPath := rfutils.GetLimaInstanceConfigPath(instance)
		items = append(items, tui.PropertyItem{Key: "Config", Value: configPath})

		// Template source
		lima := &rfdock.LimaEngine{}
		if tmpl := lima.FindTemplate(); tmpl != "" {
			items = append(items, tui.PropertyItem{Key: "Template", Value: tmpl, ValueColor: tui.ColorCyan})
		} else {
			items = append(items, tui.PropertyItem{Key: "Template", Value: "not found (using inline fallback)", ValueColor: tui.ColorWarning})
		}

		// QMP socket
		if sockPath, err := rfutils.FindLimaQMPSocket(instance); err == nil {
			items = append(items, tui.PropertyItem{Key: "QMP socket", Value: sockPath, ValueColor: tui.ColorSuccess})
		} else {
			items = append(items, tui.PropertyItem{Key: "QMP socket", Value: "not found", ValueColor: tui.ColorDanger})
		}

		// Docker socket
		socketPath := lima.GetSocketPath()
		if socketPath != "" {
			items = append(items, tui.PropertyItem{Key: "Docker socket", Value: socketPath, ValueColor: tui.ColorSuccess})
		} else {
			items = append(items, tui.PropertyItem{Key: "Docker socket", Value: "not available", ValueColor: tui.ColorDanger})
		}

		tui.RenderPropertySheet("Lima VM", tui.ColorPrimary, items)
	},
}

func registerEngineCommands() {
	rootCmd.AddCommand(engineCmd)

	// Lima subcommands — macOS only
	if runtime.GOOS == "darwin" {
		engineCmd.AddCommand(engineLimaCmd)
		engineLimaCmd.AddCommand(engineLimaReconfigCmd)
		engineLimaCmd.AddCommand(engineLimaResetCmd)
		engineLimaCmd.AddCommand(engineLimaStatusCmd)

		// Instance flag (shared across lima subcommands)
		engineLimaCmd.PersistentFlags().StringVar(&engineLimaInstance, "instance", "rfswift", "Lima instance name")

		// Reconfig flags
		engineLimaReconfigCmd.Flags().Bool("force", false, "Delete and recreate the VM (destructive — loses all VM data)")
		engineLimaReconfigCmd.Flags().String("template", "", "Path to Lima YAML template (overrides auto-detection)")

		// Reset flags
		engineLimaResetCmd.Flags().String("template", "", "Path to Lima YAML template (overrides auto-detection)")
	}
}
