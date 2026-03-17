/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * CLI commands for NAT network management
 */

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	"penthertz/rfswift/tui"
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage container networks",
	Long:  `Manage RF Swift NAT networks used for per-container network isolation`,
}

var networkListCmd = &cobra.Command{
	Use:   "list",
	Short: "List NAT networks",
	Long:  `List all RF Swift NAT networks with their subnet allocations`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.DisplayNATNetworks()
	},
}

var networkCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a NAT network",
	Long: `Create a named NAT network that containers can join.
Multiple containers can share the same network to communicate with each other
while remaining isolated from other networks.

Examples:
  rfswift network create -n pentest_lab
  rfswift network create -n pentest_lab --subnet 172.30.10.0/24`,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		subnet, _ := cmd.Flags().GetString("subnet")

		if name == "" && tui.IsInteractive() {
			var err error
			name, err = tui.PromptInput("Network name", "pentest_lab")
			if err != nil || name == "" {
				common.PrintErrorMessage(fmt.Errorf("network name is required"))
				return
			}
		}

		if name == "" {
			common.PrintErrorMessage(fmt.Errorf("network name is required (use -n flag)"))
			return
		}

		if subnet == "" && tui.IsInteractive() {
			var err error
			subnet, err = tui.PromptInput("Custom subnet (leave empty for auto-allocation, e.g., 10.10.0.0/24)", "")
			if err != nil {
				return
			}
		}

		rfdock.CreateNATNetworkCLI(name, subnet)
	},
}

var networkRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a NAT network",
	Long:  `Remove an RF Swift NAT network by name or container name`,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")

		if name == "" && tui.IsInteractive() {
			networks, err := rfdock.ListNATNetworks()
			if err != nil {
				common.PrintErrorMessage(err)
				return
			}
			if len(networks) == 0 {
				common.PrintInfoMessage("No RF Swift NAT networks found")
				return
			}

			options := make([]string, len(networks))
			for i, n := range networks {
				options[i] = fmt.Sprintf("%s  (%s, container: %s)", n.Name, n.Subnet, n.Container)
			}

			selected, err := tui.SelectOne("Select a network to remove", options)
			if err != nil {
				return
			}
			for i, opt := range options {
				if opt == selected {
					name = networks[i].Name
					break
				}
			}

			if !tui.Confirm(fmt.Sprintf("Remove network '%s'?", name)) {
				common.PrintInfoMessage("Removal cancelled.")
				return
			}
		}

		if name == "" {
			common.PrintErrorMessage(fmt.Errorf("network name is required (use -n flag)"))
			return
		}

		rfdock.RemoveNATNetworkByName(name)
	},
}

var networkCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove orphaned NAT networks",
	Long:  `Remove NAT networks whose associated container no longer exists`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.CleanupOrphanedNATNetworks()
	},
}

func registerNetworkCommands() {
	rootCmd.AddCommand(networkCmd)
	networkCmd.AddCommand(networkListCmd)
	networkCmd.AddCommand(networkCreateCmd)
	networkCmd.AddCommand(networkRemoveCmd)
	networkCmd.AddCommand(networkCleanupCmd)

	networkCreateCmd.Flags().StringP("name", "n", "", "Network name (e.g., pentest_lab)")
	networkCreateCmd.Flags().String("subnet", "", "Custom subnet CIDR (e.g., 172.30.10.0/24). Auto-allocated if omitted")

	networkRemoveCmd.Flags().StringP("name", "n", "", "Network name or container name")
}
