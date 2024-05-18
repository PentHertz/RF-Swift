/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*/

package cli

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
    rfdock "penthertz/rfswift/dock"
    rfutils "penthertz/rfswift/rfutils"
)

var DImage string
var ContID string
var ExecCmd string
var FilterLast string
var ExtraBind string
var XDisplay string
var SInstall string

var rootCmd = &cobra.Command{
    Use:  "rfswift",
    Short: "rfswift - a simple CLI to transform and inspect strings",
    Long: `rfswift is a super fancy CLI (kidding)
   
One can use stringer to modify or inspect strings straight from the terminal`,
    Run: func(cmd *cobra.Command, args []string) {
            fmt.Println("Use '-h' for help")
    },
}

var runCmd = &cobra.Command{
  Use:   "run",
  Short: "create and run a program",
  Long:  `Create a container and run a program inside the docker container`,
  Run: func(cmd *cobra.Command, args []string) {
    rfutils.XHostEnable() // force xhost to add local connections ALCs, TODO: to optimize later
    rfdock.DockerSetShell(ExecCmd)
    rfdock.DockerAddBiding(ExtraBind)
    rfdock.DockerSetImage(DImage)
    rfdock.DockerRun()
  },
}

var execCmd = &cobra.Command{
  Use:   "exec",
  Short: "exec a command",
  Long:  `Exec a program on a created docker container, even not started`,
  Run: func(cmd *cobra.Command, args []string) {
    rfutils.XHostEnable() // force xhost to add local connections ALCs, TODO: to optimize later
    rfdock.DockerSetShell(ExecCmd)
    rfdock.DockerExec(ContID, "/root")
  },
}

var lastCmd = &cobra.Command{
  Use:   "last",
  Short: "last container run",
  Long:  `Display the latest container that was run`,
  Run: func(cmd *cobra.Command, args []string) {
    rfdock.DockerLast(FilterLast)
  },
}

var installCmd = &cobra.Command{
  Use:   "install",
  Short: "install function script",
  Long:  `Install function script inside the container`,
  Run: func(cmd *cobra.Command, args []string) {
    rfdock.DockerSetShell(ExecCmd)
    rfdock.DockerInstallFromScript(ContID)
  },
}

var commitCmd = &cobra.Command{
  Use:   "commit",
  Short: "commit a container",
  Long:  `Commit a container with change we have made`,
  Run: func(cmd *cobra.Command, args []string) {
    rfdock.DockerSetImage(DImage)
    rfdock.DockerCommit(ContID)
  },
}

func init() {
    rootCmd.AddCommand(runCmd)
    rootCmd.AddCommand(lastCmd)
    rootCmd.AddCommand(execCmd)
    rootCmd.AddCommand(commitCmd)
    //rootCmd.AddCommand(installCmd) // TODO: fix this function
    installCmd.Flags().StringVarP(&ExecCmd, "install", "i", "", "function for installation")
    installCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
    commitCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
    commitCmd.Flags().StringVarP(&DImage, "image", "i", "", "image (by default: 'myrfswift:latest')")
    commitCmd.MarkFlagRequired("command")
    execCmd.Flags().StringVarP(&ContID, "container", "c", "", "container to run")
    execCmd.Flags().StringVarP(&ExecCmd, "command", "e", "", "command to exec (required!)")
    execCmd.Flags().StringVarP(&SInstall, "install", "i", "", "install from function script (e.g: 'sdrpp_soft_install')")
    execCmd.MarkFlagRequired("command")
    runCmd.Flags().StringVarP(&XDisplay, "display", "d", "", "set X Display (by default: 'DISPLAY=:0')")
    runCmd.Flags().StringVarP(&ExecCmd, "command", "e", "", "command to exec (by default: '/bin/bash')")
    runCmd.Flags().StringVarP(&ExtraBind, "bind", "b", "", "extra bindings (separe them with commas)")
    runCmd.Flags().StringVarP(&DImage, "image", "i", "", "image (by default: 'myrfswift:latest')")
    lastCmd.Flags().StringVarP(&FilterLast, "filter", "f", "", "filter by image name")
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your CLI '%s'", err)
        os.Exit(1)
    }
}