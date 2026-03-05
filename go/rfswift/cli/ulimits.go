/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"os"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
)

var UlimitsCmd = &cobra.Command{
	Use:   "ulimits",
	Short: "Manage container ulimits",
	Long: `Add, remove, or list ulimits (resource limits) for a container.

Common ulimits for SDR work:
  - rtprio:  Real-time scheduling priority (0-99)
  - memlock: Maximum locked-in-memory address space (-1 for unlimited)
  - nice:    Nice priority range (40 allows nice -20)

For quick SDR setup, use 'rfswift realtime enable' instead.`,
}

var UlimitsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add or update an ulimit",
	Long: `Add or update an ulimit on a container.

Format: name=value or name=soft:hard
Examples:
  rfswift ulimits add -c mycontainer -n rtprio -v 95
  rfswift ulimits add -c mycontainer -n memlock -v -1
  rfswift ulimits add -c mycontainer -n nofile -v 1024:65536`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		name, _ := cmd.Flags().GetString("name")
		value, _ := cmd.Flags().GetString("value")
		if err := rfdock.UpdateUlimit(contID, name, value, true); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var UlimitsRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove an ulimit",
	Long:  `Remove an ulimit from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		name, _ := cmd.Flags().GetString("name")
		if err := rfdock.UpdateUlimit(contID, name, "", false); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var UlimitsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ulimits for a container",
	Long:  `Display all ulimits currently set on a container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		if err := rfdock.ListContainerUlimits(contID); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var RealtimeCmd = &cobra.Command{
	Use:   "realtime",
	Short: "Manage realtime mode for SDR operations",
	Long: `Enable or disable realtime mode on a container for low-latency SDR operations.

Realtime mode adds:
  • SYS_NICE capability (allows setting process priorities)
  • rtprio=95 ulimit (allows real-time scheduling)
  • memlock=unlimited ulimit (prevents memory swapping)
  • nice=40 ulimit (allows nice -20)

Usage inside container after enabling:
  chrt -f 50 rtl_sdr -f 433920000 -s 2048000 -
  ulimit -r  # should show 95`,
}

var RealtimeEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable realtime mode on a container",
	Long:  `Enable realtime mode (SYS_NICE + rtprio/memlock ulimits) on an existing container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		if err := rfdock.EnableRealtimeMode(contID); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var RealtimeDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable realtime mode on a container",
	Long:  `Disable realtime mode (remove SYS_NICE and realtime ulimits) from a container`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		if err := rfdock.DisableRealtimeMode(contID); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var RealtimeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check realtime mode status for a container",
	Long:  `Display whether realtime mode is enabled and show current ulimits`,
	Run: func(cmd *cobra.Command, args []string) {
		contID, _ := cmd.Flags().GetString("container")
		if err := rfdock.ListContainerUlimits(contID); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

func registerUlimitsCommands() {
	rootCmd.AddCommand(UlimitsCmd)
	rootCmd.AddCommand(RealtimeCmd)

	// Ulimits subcommands
	UlimitsCmd.AddCommand(UlimitsAddCmd)
	UlimitsCmd.AddCommand(UlimitsRmCmd)
	UlimitsCmd.AddCommand(UlimitsListCmd)

	UlimitsAddCmd.Flags().StringP("container", "c", "", "container ID or name")
	UlimitsAddCmd.Flags().StringP("name", "n", "", "ulimit name (rtprio, memlock, nofile, nice, etc.)")
	UlimitsAddCmd.Flags().StringP("value", "v", "", "ulimit value (e.g., '95', '-1', '1024:65536')")
	UlimitsAddCmd.MarkFlagRequired("container")
	UlimitsAddCmd.MarkFlagRequired("name")
	UlimitsAddCmd.MarkFlagRequired("value")

	UlimitsRmCmd.Flags().StringP("container", "c", "", "container ID or name")
	UlimitsRmCmd.Flags().StringP("name", "n", "", "ulimit name to remove")
	UlimitsRmCmd.MarkFlagRequired("container")
	UlimitsRmCmd.MarkFlagRequired("name")

	UlimitsListCmd.Flags().StringP("container", "c", "", "container ID or name")
	UlimitsListCmd.MarkFlagRequired("container")

	// Realtime subcommands
	RealtimeCmd.AddCommand(RealtimeEnableCmd)
	RealtimeCmd.AddCommand(RealtimeDisableCmd)
	RealtimeCmd.AddCommand(RealtimeStatusCmd)

	RealtimeEnableCmd.Flags().StringP("container", "c", "", "container ID or name")
	RealtimeEnableCmd.MarkFlagRequired("container")
	RealtimeDisableCmd.Flags().StringP("container", "c", "", "container ID or name")
	RealtimeDisableCmd.MarkFlagRequired("container")
	RealtimeStatusCmd.Flags().StringP("container", "c", "", "container ID or name")
	RealtimeStatusCmd.MarkFlagRequired("container")
}
