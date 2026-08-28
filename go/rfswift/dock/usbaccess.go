/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package dock

import (
	"strings"

	common "penthertz/rfswift/common"
)

// ContainerUSBAccess evaluates the container configuration accumulated by the
// setters (config.ini defaults plus flags/wizard/profile) for USB
// reachability, so the CLI can warn before the container exists.
//
//	out: common.USBAccess
func ContainerUSBAccess() common.USBAccess {
	return common.CheckUSBAccess(splitCommaList(containerCfg.devices), splitCommaList(containerCfg.extrabinding), splitCommaList(containerCfg.cgroups), containerCfg.privileged)
}

// splitCommaList splits a comma-separated config value, dropping blanks.
func splitCommaList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
