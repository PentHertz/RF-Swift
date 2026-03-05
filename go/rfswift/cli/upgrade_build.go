/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
)

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

		if err := rfdock.ContainerUpgrade(containerName, repositories, imageName); err != nil {
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

func registerUpgradeBuildCommands() {
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(buildCmd)

	upgradeCmd.Flags().StringP("container", "c", "", "Container name or ID to upgrade (required)")
	upgradeCmd.Flags().StringP("repositories", "r", "", "Comma-separated list of container directories to preserve (e.g., /root/share,/opt/tools). These directories will be copied from old container to new container")
	upgradeCmd.Flags().StringP("image", "i", "", "Target image name/tag (if not specified, uses 'latest')")
	upgradeCmd.MarkFlagRequired("container")

	buildCmd.Flags().StringP("recipe", "r", "rfswift-recipe.yaml", "Path to the recipe file")
	buildCmd.Flags().StringP("tag", "t", "", "Override the tag name from recipe")
	buildCmd.Flags().Bool("no-cache", false, "Build without using cache")
}
