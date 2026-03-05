/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"os"

	"github.com/spf13/cobra"
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
		if isADevice {
			rfdock.UpdateDeviceBinding(contID, bsource, btarget, true)
		} else {
			rfdock.UpdateMountBinding(contID, bsource, btarget, true)
		}
	},
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
		if isADevice {
			rfdock.UpdateDeviceBinding(contID, bsource, btarget, false)
		} else {
			rfdock.UpdateMountBinding(contID, bsource, btarget, false)
		}
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
	Long:  `Bind a container port to a host port (e.g., '8080/tcp:8080' or '8080/tcp:127.0.0.1:8080')`,
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

func registerPropertyCommands() {
	rootCmd.AddCommand(BindingsCmd)
	rootCmd.AddCommand(CapabilitiesCmd)
	rootCmd.AddCommand(CgroupsCmd)
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
