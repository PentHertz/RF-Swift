/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Discovery for the container `install` command: list the install functions a
*  container ships (in /root/scripts/*.sh), so users can search and pick one in
*  a TUI instead of remembering exact function names.
 */

package dock

import (
	"os/exec"
	"sort"
	"strings"
)

// ListInstallFunctions returns the install-function names available inside a
// container (e.g. grgsm_grmod_install, sdrpp_soft_install), extracted from the
// shell scripts under /root/scripts. Empty if none can be read.
func ListInstallFunctions(containerID string) []string {
	eng := GetEngine()
	if eng == nil || !eng.IsAvailable() {
		return nil
	}
	cli := engineCLI(eng.Type())
	// Match `name_install()` / `function name_install()` definitions and print
	// just the function name, de-duplicated.
	script := `grep -rhoE '^[[:space:]]*(function[[:space:]]+)?[A-Za-z0-9_]+_install[[:space:]]*\(\)' /root/scripts 2>/dev/null | ` +
		`sed -E 's/^[[:space:]]*(function[[:space:]]+)?//; s/[[:space:]]*\(\).*//' | sort -u`
	out, err := exec.Command(cli, "exec", containerID, "sh", "-c", script).Output()
	if err != nil {
		return nil
	}
	var fns []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			fns = append(fns, l)
		}
	}
	sort.Strings(fns)
	return fns
}
