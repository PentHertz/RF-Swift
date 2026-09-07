/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Container configuration changes on Docker (Linux) rewrite hostconfig.json
*  and config.v2.json in place and restart the daemon: no copy of the
*  container, the container comes back as it was. Only root can do that, so
*  the config commands re-run the same invocation as root (one sudo prompt)
*  instead of failing with "permission denied". --recreate opts out.
 */

package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	rfdock "penthertz/rfswift/dock"
	"penthertz/rfswift/hostsetup"
)

// elevateForConfigEdit re-executes this command as root when the active
// engine needs it, then exits with the root process's status.
func elevateForConfigEdit(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && cmd.Runnable() == false {
		return nil // a group without a subcommand only prints help
	}
	// Cobra runs only the nearest persistent pre-run hook: this one replaces
	// the root's, which is where --engine / RFSWIFT_ENGINE / the configured
	// engine are applied. Run it first, or every configuration command would
	// auto-detect the engine and drive the wrong daemon (Docker Desktop
	// instead of the Lima VM on macOS, Docker instead of Podman on Linux).
	if root := cmd.Root(); root != nil && root != cmd && root.PersistentPreRun != nil {
		root.PersistentPreRun(cmd, args)
	}
	if recreate, _ := cmd.Flags().GetBool("recreate"); recreate {
		rfdock.SetConfigEditMode(rfdock.EditModeRecreate)
	}
	if os.Getenv("RFSWIFT_ELEVATED") != "" || !rfdock.NeedsRootForConfigEdit() {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "This change rewrites the container's files under /var/lib/docker and restarts the Docker service: running it as root (--recreate avoids that by re-creating the container).")
	argv := append([]string{exe, "-q"}, os.Args[1:]...)
	if os.Geteuid() != 0 {
		if _, err := exec.LookPath("sudo"); err == nil && hostsetupTerminal() {
			c := exec.Command("sudo", append([]string{"env", "RFSWIFT_ELEVATED=1"}, argv...)...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			if runErr := c.Run(); runErr != nil {
				var exitErr *exec.ExitError
				if errors.As(runErr, &exitErr) {
					os.Exit(exitErr.ExitCode())
				}
				return runErr
			}
			os.Exit(0)
		}
	}
	out, runErr := hostsetup.RunPrivilegedCommand(append([]string{"env", "RFSWIFT_ELEVATED=1"}, argv...)...)
	fmt.Fprint(os.Stdout, out)
	if runErr != nil {
		// Cobra would print the usage on a returned error; this is not a
		// usage problem, so explain and stop here. 126 and 127 are pkexec's
		// "not authorised" and "no authentication agent".
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && (exitErr.ExitCode() == 126 || exitErr.ExitCode() == 127) {
			fmt.Fprintln(os.Stderr, "Not authorised: the password prompt was refused, cancelled, or no polkit agent is available here. Run this command in a terminal (it asks through sudo), or with sudo directly.")
		} else {
			fmt.Fprintf(os.Stderr, "The change could not be applied as root: %v\n", runErr)
		}
		os.Exit(1)
	}
	os.Exit(0)
	return nil
}

func hostsetupTerminal() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
