/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Qubes OS integration: the glue between hostsetup's qube checks and setup
 * (which sit below the engine and config packages) and RF Swift's own
 * configuration. Shared by `rfswift host qubes`, `rfswift doctor` and the
 * Workbench, so all three see and change the same thing.
 */

package dock

import (
	"fmt"
	"os"
	"strings"

	common "penthertz/rfswift/common"
	"penthertz/rfswift/hostsetup"
	rfutils "penthertz/rfswift/rfutils"
)

// QubesInput collects what the qube checks need from RF Swift's side: the
// user RF Swift acts for, the effective podman_mode, the workspace root and
// the configured default devices.
func QubesInput() hostsetup.QubesStatusInput {
	return hostsetup.QubesStatusInput{
		User:          hostsetup.InvokingUser(),
		PodmanMode:    string(ConfiguredPodmanMode()),
		WorkspaceRoot: DefaultWorkspaceRoot(),
		Devices:       rfutils.ConfigValue(common.ConfigFileByPlatform(), "container", "devices"),
	}
}

// QubesStatus inspects this qube; never changes anything.
func QubesStatus() hostsetup.QubesStatus { return hostsetup.GetQubesStatus(QubesInput()) }

// QubesSetupScript is the root script the setup would run with o.
func QubesSetupScript(o hostsetup.QubesSetupOptions) (string, error) {
	return hostsetup.QubesSetupScript(QubesInput(), o)
}

// ApplyQubesSetup runs the root part of the Qubes setup, then the user's own
// part: podman_mode = rootful when the socket was granted, and the default
// devices this qube cannot have dropped from config.ini. The engine is
// re-detected afterwards so this process (the Workbench) switches to the
// rootful service without a restart.
//
//	in(1): hostsetup.QubesSetupOptions o what to do
//	out: hostsetup.QubesSetupReport report (Status re-read after everything)
//	out: []string the config.ini changes made
//	out: error
func ApplyQubesSetup(o hostsetup.QubesSetupOptions) (hostsetup.QubesSetupReport, []string, error) {
	in := QubesInput()
	before := hostsetup.GetQubesStatus(in)
	report, err := hostsetup.RunQubesSetup(in, o)
	if err != nil {
		return report, nil, err
	}
	var changes []string
	cfg := common.ConfigFileByPlatform()
	if o.PodmanSocket && o.PodmanFound && ConfiguredPodmanMode() != PodmanModeRootful {
		if err := rfutils.SetConfigValue(cfg, "general", "podman_mode", string(PodmanModeRootful)); err != nil {
			return report, changes, fmt.Errorf("the socket is ready but podman_mode could not be saved in %s: %w", cfg, err)
		}
		changes = append(changes, "podman_mode = rootful ("+cfg+")")
		if env := strings.TrimSpace(os.Getenv("RFSWIFT_PODMAN_MODE")); env != "" {
			report.Notes = append(report.Notes, "RFSWIFT_PODMAN_MODE="+env+" is set in this environment and overrides config.ini")
		}
	}
	if len(before.DropDevices) > 0 {
		devices := hostsetup.DropDevices(in.Devices, before.DropDevices)
		if err := rfutils.SetConfigValue(cfg, "container", "devices", devices); err != nil {
			return report, changes, fmt.Errorf("could not update the default devices in %s: %w", cfg, err)
		}
		changes = append(changes, "default devices without "+strings.Join(before.DropDevices, ", "))
	}
	ResetEngineDetection()
	report.Status = hostsetup.GetQubesStatus(QubesInput())
	return report, changes, nil
}
