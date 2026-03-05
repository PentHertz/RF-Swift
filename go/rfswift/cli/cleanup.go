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

func registerCleanupCommands() {
	rootCmd.AddCommand(CleanupCmd)

	CleanupCmd.AddCommand(CleanupAllCmd)
	CleanupCmd.AddCommand(CleanupContainersCmd)
	CleanupCmd.AddCommand(CleanupImagesCmd)

	CleanupAllCmd.Flags().String("older-than", "", "Remove items older than duration (e.g., '24h', '7d', '1m', '1y')")
	CleanupAllCmd.Flags().Bool("force", false, "Don't ask for confirmation")
	CleanupAllCmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting")

	CleanupContainersCmd.Flags().String("older-than", "", "Remove containers older than duration (e.g., '24h', '7d', '1m', '1y')")
	CleanupContainersCmd.Flags().Bool("force", false, "Don't ask for confirmation")
	CleanupContainersCmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting")
	CleanupContainersCmd.Flags().Bool("stopped", false, "Only remove stopped containers")

	CleanupImagesCmd.Flags().String("older-than", "", "Remove images older than duration (e.g., '24h', '7d', '1m', '1y')")
	CleanupImagesCmd.Flags().Bool("force", false, "Don't ask for confirmation")
	CleanupImagesCmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting")
	CleanupImagesCmd.Flags().Bool("dangling", false, "Only remove dangling (untagged) images")
	CleanupImagesCmd.Flags().Bool("prune-children", false, "Also remove dependent child images")
}
