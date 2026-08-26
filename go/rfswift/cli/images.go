/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	"penthertz/rfswift/tui"
)

var ImagesCmd = &cobra.Command{
	Use:   "images",
	Short: "RF Swift images management remote/local",
	Long:  `List local and remote images`,
}

var ImagesLocalCmd = &cobra.Command{
	Use:   "local",
	Short: "List local images",
	Long:  `List pulled and built images`,
	Run: func(cmd *cobra.Command, args []string) {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		showVersions, _ := cmd.Flags().GetBool("show-versions")
		filterImage, _ := cmd.Flags().GetString("filter")
		rfdock.PrintImagesTable(labelKey, labelValue, showVersions, filterImage)
	},
}

var ImagesRemoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "List remote images",
	Long:  `Lists RF Swift images from official repository`,
	Run: func(cmd *cobra.Command, args []string) {
		showVersions, _ := cmd.Flags().GetBool("show-versions")
		filterImage, _ := cmd.Flags().GetString("filter")
		rfdock.ListDockerImagesRepo(showVersions, filterImage)
	},
}

var ImagesVersionsCmd = &cobra.Command{
	Use:   "versions",
	Short: "List available versions for images",
	Long:  `List all available versions for RF Swift images`,
	Run: func(cmd *cobra.Command, args []string) {
		filterImage, _ := cmd.Flags().GetString("filter")
		rfdock.ListAvailableVersions(filterImage)
	},
}

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull a container",
	Long:  `Pull a container from internet`,
	Run: func(cmd *cobra.Command, args []string) {
		imageRef, _ := cmd.Flags().GetString("image")
		imageTag, _ := cmd.Flags().GetString("tag")
		version, _ := cmd.Flags().GetString("version")

		if version != "" {
			rfdock.ContainerPullVersion(imageRef, version, imageTag)
		} else {
			rfdock.ContainerPull(imageRef, imageTag)
		}
	},
}

var retagCmd = &cobra.Command{
	Use:   "retag",
	Short: "Rename an image",
	Long:  `Rename an image with another tag`,
	Run: func(cmd *cobra.Command, args []string) {
		imageRef, _ := cmd.Flags().GetString("image")
		imageTag, _ := cmd.Flags().GetString("tag")

		// Interactive image selection
		if imageRef == "" && tui.IsInteractive() {
			tags := rfdock.ListImageTags("org.container.project", "rfswift")
			if len(tags) == 0 {
				common.PrintErrorMessage(fmt.Errorf("no RF Swift images found"))
				os.Exit(1)
			}

			selected, err := tui.SelectOne("Select an image to retag", tags)
			if err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			imageRef = selected
		}

		if imageTag == "" {
			common.PrintErrorMessage(fmt.Errorf("target tag is required (use -t flag)"))
			os.Exit(1)
		}

		rfdock.ContainerTag(imageRef, imageTag)
	},
}

var DeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an rfswift images",
	Long:  `Delete an RF Swift image from image name or tag`,
	Run: func(cmd *cobra.Command, args []string) {
		image, _ := cmd.Flags().GetString("image")

		// Interactive image selection
		if image == "" && tui.IsInteractive() {
			tags := rfdock.ListImageTags("org.container.project", "rfswift")
			if len(tags) == 0 {
				common.PrintErrorMessage(fmt.Errorf("no RF Swift images found"))
				os.Exit(1)
			}

			selected, err := tui.SelectOne("Select an image to delete", tags)
			if err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			image = selected

			if !tui.Confirm(fmt.Sprintf("Delete image '%s'?", image)) {
				common.PrintInfoMessage("Deletion cancelled.")
				return
			}
		}

		if image == "" {
			common.PrintErrorMessage(fmt.Errorf("image is required (use -i flag)"))
			os.Exit(1)
		}

		rfdock.DeleteImage(image)
	},
}

var DownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download and save an image to tar.gz",
	Long:  `Download an image from a repository and save it locally as a compressed tar.gz file`,
	Run: func(cmd *cobra.Command, args []string) {
		imageName, _ := cmd.Flags().GetString("image")
		outputFile, _ := cmd.Flags().GetString("output")
		pullFirst, _ := cmd.Flags().GetBool("pull")

		// Interactive image selection
		if imageName == "" && tui.IsInteractive() {
			tags := rfdock.ListImageTags("org.container.project", "rfswift")
			if len(tags) == 0 {
				common.PrintErrorMessage(fmt.Errorf("no RF Swift images found"))
				os.Exit(1)
			}

			selected, err := tui.SelectOne("Select an image to download", tags)
			if err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			imageName = selected
		}

		if imageName == "" {
			common.PrintErrorMessage(fmt.Errorf("image is required (use -i flag)"))
			os.Exit(1)
		}

		if outputFile == "" {
			safe := strings.ReplaceAll(imageName, "/", "_")
			safe = strings.ReplaceAll(safe, ":", "_")
			outputFile = fmt.Sprintf("%s.tar.gz", safe)
		}

		if err := rfdock.SaveImageToFile(imageName, outputFile, pullFirst); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var ImagesAuditCmd = &cobra.Command{
	Use:   "audit <image> [image...]",
	Short: "Scan a container image for known vulnerabilities",
	Long: `Audit one or more container images (Docker/Podman) for known
vulnerabilities, so you can check an image's security posture on your own
machine, on your own engine, without leaving RF Swift.

It scans each image natively with trivy (OS packages + language dependencies).
A host-installed trivy is used if present; otherwise trivy runs as a container
via the active engine, so no extra install is needed. A host grype is used as a
second opinion when available. Nothing is fatal unless you pass --fail-on.

Reports are written to ./security-report in the formats given by --format
(stdout,json,html,pdf). PDF needs a host weasyprint or wkhtmltopdf; without one
the HTML report is still produced.

Examples:
  rfswift images audit penthertz/rfswift:sdr_full
  rfswift images audit penthertz/rfswift:corebuild --format stdout,json,html,pdf
  rfswift images audit myimage:latest --fail-on critical --out /tmp/audit`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		format, _ := cmd.Flags().GetString("format")
		failOn, _ := cmd.Flags().GetString("fail-on")
		out, _ := cmd.Flags().GetString("out")
		if format == "all" {
			format = "stdout,json,html,pdf"
		}
		if failOn == "" {
			failOn = "none"
		}
		if err := rfdock.AuditImage(args, rfdock.ImageAuditOptions{
			FailOn:  failOn,
			OutDir:  out,
			Formats: format,
		}); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

func registerImageCommands() {
	rootCmd.AddCommand(ImagesCmd)
	rootCmd.AddCommand(retagCmd)
	rootCmd.AddCommand(DeleteCmd)
	rootCmd.AddCommand(DownloadCmd)

	ImagesCmd.AddCommand(pullCmd)
	ImagesCmd.AddCommand(ImagesRemoteCmd)
	ImagesCmd.AddCommand(ImagesLocalCmd)
	ImagesCmd.AddCommand(ImagesVersionsCmd)
	ImagesCmd.AddCommand(ImagesAuditCmd)
	ImagesAuditCmd.Flags().String("format", "stdout", "report formats: stdout,json,html,pdf (or 'all')")
	ImagesAuditCmd.Flags().String("fail-on", "none", "exit non-zero if a finding is >= none|low|medium|high|critical")
	ImagesAuditCmd.Flags().String("out", "", "report output directory (default: ./security-report)")
	ImagesCmd.PersistentFlags().BoolP("show-versions", "v", false, "Show version information for images")
	ImagesCmd.PersistentFlags().StringP("filter", "f", "", "Filter images by name")

	pullCmd.Flags().StringP("image", "i", "", "image reference")
	pullCmd.Flags().StringP("tag", "t", "", "rename to target tag")
	pullCmd.Flags().StringP("version", "V", "", "specific version to pull (e.g., '1.2.0')")
	pullCmd.MarkFlagRequired("image")

	ImagesVersionsCmd.Flags().StringP("filter", "f", "", "Filter by image name")

	retagCmd.Flags().StringP("image", "i", "", "image to retag (interactive picker if omitted)")
	retagCmd.Flags().StringP("tag", "t", "", "new target tag")

	DeleteCmd.Flags().StringP("image", "i", "", "image to delete (interactive picker if omitted)")

	DownloadCmd.Flags().StringP("image", "i", "", "image to download (interactive picker if omitted)")
	DownloadCmd.Flags().StringP("output", "o", "", "output file path (auto-generated if omitted)")
	DownloadCmd.Flags().Bool("pull", false, "pull image first if not present locally")
}
