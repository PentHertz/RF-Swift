/* This code is part of RF Switch by @Penthertz
*  Author(s): Sebastien Dudek (@FlUxIuS)
 */

package cli

import (
	"github.com/spf13/cobra"
	rfdock "penthertz/rfswift/dock"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system environment and configuration",
	Long:  `Diagnose your system environment for RF Swift compatibility. Checks container engine, X11, audio, USB devices, config file, and kernel modules.`,
	Run: func(cmd *cobra.Command, args []string) {
		rfdock.RunDoctor()
	},
}

func registerDoctorCommands() {
	rootCmd.AddCommand(doctorCmd)
}
