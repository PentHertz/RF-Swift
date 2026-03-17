/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	"penthertz/rfswift/tui"
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

		// Interactive container selection
		if contID == "" {
			containers := rfdock.ListContainers("org.container.project", "rfswift")
			if len(containers) == 0 {
				common.PrintErrorMessage(fmt.Errorf("no RF Swift containers found"))
				os.Exit(1)
			}

			options := make([]string, len(containers))
			for i, c := range containers {
				options[i] = fmt.Sprintf("%s  (%s, %s, %s)", c.Name, c.ID, c.Image, c.State)
			}

			selected, err := tui.SelectOne("Select a container to export", options)
			if err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}

			for _, c := range containers {
				label := fmt.Sprintf("%s  (%s, %s, %s)", c.Name, c.ID, c.Image, c.State)
				if label == selected {
					contID = c.ID
					if outputFile == "" {
						outputFile = fmt.Sprintf("%s-%s.tar.gz", c.Name, time.Now().Format("20060102"))
					}
					break
				}
			}
		}

		if outputFile == "" {
			outputFile = fmt.Sprintf("container-export-%s.tar.gz", time.Now().Format("20060102-150405"))
		}

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

		// Interactive image selection
		if len(images) == 0 {
			tags := rfdock.ListImageTags("org.container.project", "rfswift")
			if len(tags) == 0 {
				common.PrintErrorMessage(fmt.Errorf("no RF Swift images found"))
				os.Exit(1)
			}

			selected, err := tui.SelectOne("Select an image to export", tags)
			if err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			images = []string{selected}

			if outputFile == "" {
				// Derive filename from image name: penthertz/rfswift:tag -> rfswift_tag.tar.gz
				safe := strings.ReplaceAll(selected, "/", "_")
				safe = strings.ReplaceAll(safe, ":", "_")
				outputFile = fmt.Sprintf("%s.tar.gz", safe)
			}
		}

		if outputFile == "" {
			outputFile = fmt.Sprintf("image-export-%s.tar.gz", time.Now().Format("20060102-150405"))
		}

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
	Use:   "container [file]",
	Short: "Import a container from tar.gz",
	Long:  `Import a container filesystem from a tar.gz file and create an image`,
	Run: func(cmd *cobra.Command, args []string) {
		inputFile, _ := cmd.Flags().GetString("input")
		imageName, _ := cmd.Flags().GetString("name")

		// Accept positional argument
		if inputFile == "" && len(args) > 0 {
			inputFile = args[0]
		}

		// Interactive file picker
		if inputFile == "" {
			inputFile = pickTarGzFile("Select a tar.gz file to import as container")
			if inputFile == "" {
				common.PrintErrorMessage(fmt.Errorf("no tar.gz files found in current directory"))
				os.Exit(1)
			}
		}

		// Suggest image name from filename
		if imageName == "" {
			base := filepath.Base(inputFile)
			base = strings.TrimSuffix(base, ".tar.gz")
			base = strings.TrimSuffix(base, ".tar")
			suggested := fmt.Sprintf("rfswift/%s:imported", base)
			common.PrintInfoMessage(fmt.Sprintf("Using image name: %s", suggested))
			imageName = suggested
		}

		if err := rfdock.ImportContainer(inputFile, imageName); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var ImportImageCmd = &cobra.Command{
	Use:   "image [file]",
	Short: "Import an image from tar.gz",
	Long:  `Import one or more images from a tar.gz file`,
	Run: func(cmd *cobra.Command, args []string) {
		inputFile, _ := cmd.Flags().GetString("input")

		// Accept positional argument
		if inputFile == "" && len(args) > 0 {
			inputFile = args[0]
		}

		// Interactive file picker
		if inputFile == "" {
			inputFile = pickTarGzFile("Select a tar.gz file to import")
			if inputFile == "" {
				common.PrintErrorMessage(fmt.Errorf("no tar.gz files found in current directory"))
				os.Exit(1)
			}
		}

		if err := rfdock.ImportImage(inputFile); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

// pickTarGzFile lists .tar.gz and .tar files in the current directory and lets the user pick one.
func pickTarGzFile(title string) string {
	entries, err := os.ReadDir(".")
	if err != nil {
		return ""
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tar") {
			info, err := e.Info()
			if err != nil {
				continue
			}
			size := float64(info.Size()) / (1024 * 1024)
			files = append(files, fmt.Sprintf("%s  (%.1f MB)", name, size))
		}
	}

	if len(files) == 0 {
		return ""
	}

	selected, err := tui.SelectOne(title, files)
	if err != nil {
		return ""
	}

	// Extract filename before the "  ("
	if idx := strings.Index(selected, "  ("); idx > 0 {
		return selected[:idx]
	}
	return selected
}

func registerTransferCommands() {
	rootCmd.AddCommand(ExportCmd)
	rootCmd.AddCommand(ImportCmd)

	ExportCmd.AddCommand(ExportContainerCmd)
	ExportCmd.AddCommand(ExportImageCmd)
	ImportCmd.AddCommand(ImportContainerCmd)
	ImportCmd.AddCommand(ImportImageCmd)

	ExportContainerCmd.Flags().StringP("container", "c", "", "container ID or name (interactive picker if omitted)")
	ExportContainerCmd.Flags().StringP("output", "o", "", "output file path (auto-generated if omitted)")

	ExportImageCmd.Flags().StringSliceP("images", "i", []string{}, "image name(s) to export (interactive picker if omitted)")
	ExportImageCmd.Flags().StringP("output", "o", "", "output file path (auto-generated if omitted)")

	ImportContainerCmd.Flags().StringP("input", "i", "", "input tar.gz file (interactive picker if omitted)")
	ImportContainerCmd.Flags().StringP("name", "n", "", "image name for import (auto-generated if omitted)")

	ImportImageCmd.Flags().StringP("input", "i", "", "input tar.gz file (interactive picker if omitted)")
}
