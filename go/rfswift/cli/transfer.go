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
		contID, _ := cmd.Flags().GetString("container")
		outputFile, _ := cmd.Flags().GetString("output")
		if err := rfdock.ExportContainer(contID, outputFile); err != nil {
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

func registerTransferCommands() {
	rootCmd.AddCommand(ExportCmd)
	rootCmd.AddCommand(ImportCmd)

	ExportCmd.AddCommand(ExportContainerCmd)
	ExportCmd.AddCommand(ExportImageCmd)
	ImportCmd.AddCommand(ImportContainerCmd)
	ImportCmd.AddCommand(ImportImageCmd)

	ExportContainerCmd.Flags().StringP("container", "c", "", "container ID or name to export")
	ExportContainerCmd.Flags().StringP("output", "o", "", "output file path (e.g., mycontainer.tar.gz)")
	ExportContainerCmd.MarkFlagRequired("container")
	ExportContainerCmd.MarkFlagRequired("output")

	ExportImageCmd.Flags().StringSliceP("images", "i", []string{}, "image name(s) to export (can specify multiple)")
	ExportImageCmd.Flags().StringP("output", "o", "", "output file path (e.g., myimages.tar.gz)")
	ExportImageCmd.MarkFlagRequired("images")
	ExportImageCmd.MarkFlagRequired("output")

	ImportContainerCmd.Flags().StringP("input", "i", "", "input tar.gz file path")
	ImportContainerCmd.Flags().StringP("name", "n", "", "name for the imported image (e.g., myimage:tag)")
	ImportContainerCmd.MarkFlagRequired("input")
	ImportContainerCmd.MarkFlagRequired("name")

	ImportImageCmd.Flags().StringP("input", "i", "", "input tar.gz file path")
	ImportImageCmd.MarkFlagRequired("input")
}
