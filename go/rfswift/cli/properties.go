/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
)

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
		contID, _ := cmd.Flags().GetString("container")
		bsource, _ := cmd.Flags().GetString("source")
		btarget, _ := cmd.Flags().GetString("target")
		isADevice, _ := cmd.Flags().GetBool("devices")
		applyBindingChange(contID, bsource, btarget, isADevice, true)
	},
}

// applyBindingChange goes through the same code as the Workbench's Configure
// dialog (rfdock.UpdateBinding): a serial port is attached on demand when
// absent and mapped when present, a device node given as a volume is
// bind-mounted with its cgroup rule, and the change is applied by the file
// edit or the re-creation the engine calls for.
func applyBindingChange(contID, source, target string, device, add bool) {
	kind := "volume"
	if device {
		kind = "device"
	}
	if target == "" {
		target = source
	}
	if source == "" {
		source = target
	}
	if err := rfdock.UpdateBinding(contID, kind, source, target, add); err != nil {
		common.PrintErrorMessage(err)
		os.Exit(1)
	}
	what := "bind mount"
	if device {
		what = "device"
	}
	if add {
		common.PrintSuccessMessage(fmt.Sprintf("%s %s -> %s added to '%s'", what, source, target, contID))
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("%s %s removed from '%s'", what, target, contID))
	}
}

var BindingsRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a binding",
	Long:  `Remove a new binding for a container ID`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		bsource, _ := cmd.Flags().GetString("source")
		btarget, _ := cmd.Flags().GetString("target")
		isADevice, _ := cmd.Flags().GetBool("devices")
		applyBindingChange(contID, bsource, btarget, isADevice, false)
	},
}

var SerialHotplugCmd = &cobra.Command{
	Use:   "serial-hotplug on|off",
	Short: "Switch a container's serial hot-plug on or off",
	Long: `With the hot-plug on (the default when a mission names a serial port), the
container may open serial ports (/dev/ttyACM*, /dev/ttyUSB*, /dev/ttyAMA*)
through its device cgroup and RF Swift creates their nodes inside it when it
starts and whenever a terminal opens, so a port plugged in later works without
re-creating the container. Off removes those cgroup rules and leaves the
container's /dev alone; ports must then be mapped or bind-mounted explicitly.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		contID, _ := cmd.Flags().GetString("container")
		if contID == "" {
			return errors.New("-c/--container is required")
		}
		var on bool
		switch strings.ToLower(args[0]) {
		case "on", "enable", "enabled":
			on = true
		case "off", "disable", "disabled":
		default:
			return fmt.Errorf("say on or off, not %q", args[0])
		}
		if err := rfdock.UpdateSerialHotplug(contID, on); err != nil {
			return err
		}
		if on {
			common.PrintSuccessMessage(fmt.Sprintf("Serial hot-plug on for '%s': plug a serial device in and open a terminal to use it.", contID))
		} else {
			common.PrintSuccessMessage(fmt.Sprintf("Serial hot-plug off for '%s': serial ports must be mapped or bind-mounted explicitly.", contID))
		}
		return nil
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
		contID, _ := cmd.Flags().GetString("container")
		capability, _ := cmd.Flags().GetString("capability")
		if err := rfdock.UpdateCapability(contID, capability, true); err != nil {
			os.Exit(1)
		}
	},
}

var CapabilitiesRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a capability",
	Long:  `Remove a capability from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		capability, _ := cmd.Flags().GetString("capability")
		if err := rfdock.UpdateCapability(contID, capability, false); err != nil {
			os.Exit(1)
		}
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
		contID, _ := cmd.Flags().GetString("container")
		rule, _ := cmd.Flags().GetString("rule")
		if err := rfdock.UpdateCgroupRule(contID, rule, true); err != nil {
			os.Exit(1)
		}
	},
}

var CgroupsRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a cgroup rule",
	Long:  `Remove a cgroup device rule from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		rule, _ := cmd.Flags().GetString("rule")
		if err := rfdock.UpdateCgroupRule(contID, rule, false); err != nil {
			os.Exit(1)
		}
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
		contID, _ := cmd.Flags().GetString("container")
		port, _ := cmd.Flags().GetString("port")
		if err := rfdock.UpdateExposedPort(contID, port, true); err != nil {
			os.Exit(1)
		}
	},
}

var PortsUnexposeCmd = &cobra.Command{
	Use:   "unexpose",
	Short: "Remove an exposed port",
	Long:  `Remove an exposed port from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		port, _ := cmd.Flags().GetString("port")
		if err := rfdock.UpdateExposedPort(contID, port, false); err != nil {
			os.Exit(1)
		}
	},
}

var PortsBindCmd = &cobra.Command{
	Use:   "bind",
	Short: "Bind a port",
	Long:  `Bind a container port to a host port (e.g., '8080:80/tcp' or '80/tcp:8080' or '127.0.0.1:8080:80/tcp')`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		binding, _ := cmd.Flags().GetString("binding")
		if err := rfdock.UpdatePortBinding(contID, binding, true); err != nil {
			os.Exit(1)
		}
	},
}

var PortsUnbindCmd = &cobra.Command{
	Use:   "unbind",
	Short: "Remove a port binding",
	Long:  `Remove a port binding from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		binding, _ := cmd.Flags().GetString("binding")
		if err := rfdock.UpdatePortBinding(contID, binding, false); err != nil {
			os.Exit(1)
		}
	},
}

var GPUsCmd = &cobra.Command{
	Use:   "gpus",
	Short: "Manage container GPU device requests",
	Long:  `Add or remove GPU device requests for a container`,
}

var GPUsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add GPU access",
	Long:  `Add GPU device request to a container (e.g., 'all' or '0,1')`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		gpus, _ := cmd.Flags().GetString("gpus")
		if err := rfdock.UpdateGPUs(contID, gpus, true); err != nil {
			os.Exit(1)
		}
	},
}

var GPUsRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove GPU access",
	Long:  `Remove GPU device requests from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		gpus, _ := cmd.Flags().GetString("gpus")
		if err := rfdock.UpdateGPUs(contID, gpus, false); err != nil {
			os.Exit(1)
		}
	},
}

func registerPropertyCommands() {
	rootCmd.AddCommand(BindingsCmd)
	rootCmd.AddCommand(CapabilitiesCmd)
	rootCmd.AddCommand(CgroupsCmd)
	rootCmd.AddCommand(GPUsCmd)
	rootCmd.AddCommand(PortsCmd)

	// Bindings
	BindingsCmd.AddCommand(BindingsAddCmd)
	BindingsCmd.AddCommand(BindingsRmCmd)
	BindingsCmd.PersistentFlags().BoolP("devices", "d", false, "Manage a device rather than a volume")
	BindingsAddCmd.Flags().StringP("container", "c", "", "container to run")
	BindingsAddCmd.Flags().StringP("source", "s", "", "source binding (by default: source=target)")
	BindingsAddCmd.Flags().StringP("target", "t", "", "target binding")
	BindingsAddCmd.MarkFlagRequired("container")
	BindingsAddCmd.MarkFlagRequired("target")
	BindingsRmCmd.Flags().StringP("container", "c", "", "container to run")
	BindingsRmCmd.Flags().StringP("source", "s", "", "source binding (by default: source=target)")
	BindingsRmCmd.Flags().StringP("target", "t", "", "target binding")
	BindingsRmCmd.MarkFlagRequired("container")
	BindingsRmCmd.MarkFlagRequired("target")

	// Capabilities
	CapabilitiesCmd.AddCommand(CapabilitiesAddCmd)
	CapabilitiesCmd.AddCommand(CapabilitiesRmCmd)
	CapabilitiesAddCmd.Flags().StringP("container", "c", "", "container ID or name")
	CapabilitiesAddCmd.Flags().StringP("capability", "p", "", "capability to add (e.g., NET_ADMIN, SYS_PTRACE)")
	CapabilitiesAddCmd.MarkFlagRequired("container")
	CapabilitiesAddCmd.MarkFlagRequired("capability")
	CapabilitiesRmCmd.Flags().StringP("container", "c", "", "container ID or name")
	CapabilitiesRmCmd.Flags().StringP("capability", "p", "", "capability to remove")
	CapabilitiesRmCmd.MarkFlagRequired("container")
	CapabilitiesRmCmd.MarkFlagRequired("capability")

	// Cgroups
	CgroupsCmd.AddCommand(CgroupsAddCmd)
	CgroupsCmd.AddCommand(CgroupsRmCmd)
	CgroupsAddCmd.Flags().StringP("container", "c", "", "container ID or name")
	CgroupsAddCmd.Flags().StringP("rule", "r", "", "cgroup rule to add (e.g., 'c 189:* rwm')")
	CgroupsAddCmd.MarkFlagRequired("container")
	CgroupsAddCmd.MarkFlagRequired("rule")
	CgroupsRmCmd.Flags().StringP("container", "c", "", "container ID or name")
	CgroupsRmCmd.Flags().StringP("rule", "r", "", "cgroup rule to remove")
	CgroupsRmCmd.MarkFlagRequired("container")
	CgroupsRmCmd.MarkFlagRequired("rule")

	// GPUs
	GPUsCmd.AddCommand(GPUsAddCmd)
	GPUsCmd.AddCommand(GPUsRmCmd)
	GPUsAddCmd.Flags().StringP("container", "c", "", "container ID or name")
	GPUsAddCmd.Flags().StringP("gpus", "g", "all", "GPU specifier ('all' or comma-separated IDs)")
	GPUsAddCmd.MarkFlagRequired("container")
	GPUsRmCmd.Flags().StringP("container", "c", "", "container ID or name")
	GPUsRmCmd.Flags().StringP("gpus", "g", "", "GPU specifier to remove (empty = remove all)")
	GPUsRmCmd.MarkFlagRequired("container")

	// Ports
	PortsCmd.AddCommand(PortsExposeCmd)
	PortsCmd.AddCommand(PortsUnexposeCmd)
	PortsCmd.AddCommand(PortsBindCmd)
	PortsCmd.AddCommand(PortsUnbindCmd)
	PortsExposeCmd.Flags().StringP("container", "c", "", "container ID or name")
	PortsExposeCmd.Flags().StringP("port", "p", "", "port to expose (e.g., '8080/tcp')")
	PortsExposeCmd.MarkFlagRequired("container")
	PortsExposeCmd.MarkFlagRequired("port")
	PortsUnexposeCmd.Flags().StringP("container", "c", "", "container ID or name")
	PortsUnexposeCmd.Flags().StringP("port", "p", "", "port to remove")
	PortsUnexposeCmd.MarkFlagRequired("container")
	PortsUnexposeCmd.MarkFlagRequired("port")
	PortsBindCmd.Flags().StringP("container", "c", "", "container ID or name")
	PortsBindCmd.Flags().StringP("binding", "b", "", "port binding (e.g., '8080/tcp:8080' or '8080/tcp:127.0.0.1:8080')")
	PortsBindCmd.MarkFlagRequired("container")
	PortsBindCmd.MarkFlagRequired("binding")
	PortsUnbindCmd.Flags().StringP("container", "c", "", "container ID or name")
	PortsUnbindCmd.Flags().StringP("binding", "b", "", "port binding to remove")
	PortsUnbindCmd.MarkFlagRequired("container")
	PortsUnbindCmd.MarkFlagRequired("binding")
}
