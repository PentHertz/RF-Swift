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
	Use:   "replay",
	Short: "Replay a recorded session",
	Long:  `Replay a previously recorded session`,
	Run: func(cmd *cobra.Command, args []string) {
		inputFile, _ := cmd.Flags().GetString("input")
		speed, _ := cmd.Flags().GetFloat64("speed")

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

	LogReplayCmd.Flags().StringP("input", "i", "", "input file to replay")
	LogReplayCmd.Flags().Float64P("speed", "s", 1.0, "playback speed (e.g., 2.0 for 2x)")
	LogReplayCmd.MarkFlagRequired("input")

	LogListCmd.Flags().String("dir", "", "directory to search (default: current directory)")
}
