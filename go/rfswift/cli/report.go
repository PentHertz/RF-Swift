/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*  Integrated reporting CLI commands
 */

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	"penthertz/rfswift/tui"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate assessment reports",
	Long: `Generate structured reports from container sessions.

Reports combine container metadata, session recordings, shell history,
and workspace artifacts into a single document for documentation,
client deliverables, or research papers.

Supported formats: markdown (default), html, pdf (requires pandoc)`,
}

var reportGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a report from a container",
	Long: `Collect data from a container and its workspace, then generate a
structured report. The report includes:

  - Container configuration and metadata
  - Session recordings inventory
  - Shell command history
  - Workspace file artifacts (captures, configs, logs, scripts)
  - Editable notes section

Examples:
  rfswift report generate -c my_sdr
  rfswift report generate -c my_sdr --format html
  rfswift report generate -c my_sdr --format pdf -o report.pdf
  rfswift report generate -c my_sdr --title "HackRF Assessment 2026"`,
	Run: func(cmd *cobra.Command, args []string) {
		containerName, _ := cmd.Flags().GetString("container")
		formatStr, _ := cmd.Flags().GetString("format")
		outputPath, _ := cmd.Flags().GetString("output")
		title, _ := cmd.Flags().GetString("title")

		// Interactive container picker if not specified
		if containerName == "" {
			if !tui.IsInteractive() {
				common.PrintErrorMessage(fmt.Errorf("container name required (use -c flag)"))
				os.Exit(1)
			}
			picked := pickContainer("Select container for report")
			if picked == "" {
				common.PrintInfoMessage("Report generation cancelled.")
				return
			}
			containerName = picked
		}

		// Parse format
		var format rfdock.ReportFormat
		switch formatStr {
		case "html":
			format = rfdock.ReportFormatHTML
		case "pdf":
			format = rfdock.ReportFormatPDF
		case "md", "markdown", "":
			format = rfdock.ReportFormatMarkdown
		default:
			common.PrintErrorMessage(fmt.Errorf("unsupported format: %s (use: markdown, html, pdf)", formatStr))
			os.Exit(1)
		}

		reportPath, err := rfdock.GenerateReport(containerName, format, outputPath, title)
		if err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}

		common.PrintSuccessMessage(fmt.Sprintf("Report generated: %s", reportPath))
	},
}

func registerReportCommands() {
	rootCmd.AddCommand(reportCmd)
	reportCmd.AddCommand(reportGenerateCmd)

	reportGenerateCmd.Flags().StringP("container", "c", "", "Container name (interactive picker if omitted)")
	reportGenerateCmd.Flags().StringP("format", "f", "markdown", "Output format: markdown, html, pdf")
	reportGenerateCmd.Flags().StringP("output", "o", "", "Output file path (auto-generated if omitted)")
	reportGenerateCmd.Flags().StringP("title", "t", "", "Report title (auto-generated if omitted)")
}
