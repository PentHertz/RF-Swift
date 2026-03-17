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
	"penthertz/rfswift/tui"
)

var LogCmd = &cobra.Command{
	Use:   "log",
	Short: "Record and replay terminal sessions",
	Long:  `Record RF Swift operations using asciinema or script command`,
}

var LogStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start recording a session",
	Long:  `Start recording terminal session to a file`,
	Run: func(cmd *cobra.Command, args []string) {
		outputFile, _ := cmd.Flags().GetString("output")
		useScript, _ := cmd.Flags().GetBool("use-script")

		if err := rfdock.StartLogging(outputFile, useScript); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var LogStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current recording",
	Long:  `Stop the active recording session`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := rfdock.StopLogging(); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var LogReplayCmd = &cobra.Command{
	Use:   "replay [file]",
	Short: "Replay a recorded session",
	Long:  `Replay a previously recorded session. If no file is specified, pick from available recordings.`,
	Run: func(cmd *cobra.Command, args []string) {
		speed, _ := cmd.Flags().GetFloat64("speed")
		inputFile, _ := cmd.Flags().GetString("input")

		// Accept positional argument as well
		if inputFile == "" && len(args) > 0 {
			inputFile = args[0]
		}

		// Interactive picker when no file specified
		if inputFile == "" {
			logDir, _ := cmd.Flags().GetString("dir")
			entries, err := rfdock.FindLogs(logDir)
			if err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}
			if len(entries) == 0 {
				common.PrintInfoMessage("No session recordings found")
				return
			}

			options := make([]string, len(entries))
			for i, e := range entries {
				options[i] = fmt.Sprintf("%s  (%s, %.1f KB, %s)", e.Path, e.Tool, e.Size, e.ModTime)
			}

			selected, err := tui.SelectOne("Select a recording to replay", options)
			if err != nil {
				common.PrintErrorMessage(err)
				os.Exit(1)
			}

			// Extract file path (everything before the first "  (")
			for _, e := range entries {
				label := fmt.Sprintf("%s  (%s, %.1f KB, %s)", e.Path, e.Tool, e.Size, e.ModTime)
				if label == selected {
					inputFile = e.Path
					break
				}
			}
		}

		if err := rfdock.ReplayLog(inputFile, speed); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var LogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recorded sessions",
	Long:  `List all recorded session files`,
	Run: func(cmd *cobra.Command, args []string) {
		logDir, _ := cmd.Flags().GetString("dir")
		if err := rfdock.ListLogs(logDir); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

func registerLoggingCommands() {
	rootCmd.AddCommand(LogCmd)

	LogCmd.AddCommand(LogStartCmd)
	LogCmd.AddCommand(LogStopCmd)
	LogCmd.AddCommand(LogReplayCmd)
	LogCmd.AddCommand(LogListCmd)

	LogStartCmd.Flags().StringP("output", "o", "", "output file (default: rfswift-session-YYYYMMDD-HHMMSS.cast)")
	LogStartCmd.Flags().Bool("use-script", false, "force use of 'script' command instead of asciinema")

	LogReplayCmd.Flags().StringP("input", "i", "", "recording file to replay (interactive picker if omitted)")
	LogReplayCmd.Flags().Float64P("speed", "s", 1.0, "playback speed (e.g., 2.0 for 2x)")
	LogReplayCmd.Flags().String("dir", "", "directory to search for recordings (default: current directory)")

	LogListCmd.Flags().String("dir", "", "directory to search (default: current directory)")
}
