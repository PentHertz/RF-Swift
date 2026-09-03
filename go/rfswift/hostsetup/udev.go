/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Host setup - RF Swift's own udev rules for RF / hardware-security devices.
*
*  Docker runs containers as root and never needs this. Rootless Podman and
*  native Nix environments run as the user and are stopped by the root-owned
*  nodes under /dev/bus/usb: the rules must be on the HOST (a rules file inside
*  a container is never evaluated). The file is embedded in the binary (so a
*  tarball install can use it too) and shipped by the Linux packages under
*  /usr/share/rfswift/udev/ as a reference copy; it is only installed into
*  /etc/udev/rules.d when the user asks (`rfswift host udev`, `rfswift host
*  setup`, the installer's prompt, or the Workbench's Engine doctor).
 */

package hostsetup

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// UdevRulesDir is where the host's udev reads local rules.
const UdevRulesDir = "/etc/udev/rules.d"

// udevRulesDir is UdevRulesDir, overridable in tests.
var udevRulesDir = UdevRulesDir

// HostRulesFile is the name of RF Swift's rules file. 70 sorts before
// systemd's 73-seat-late.rules, which turns the uaccess tag into the seat ACL.
const HostRulesFile = "70-rfswift.rules"

// SharedRulesPath is where the Linux packages ship the reference copy.
const SharedRulesPath = "/usr/share/rfswift/udev/" + HostRulesFile

//go:embed rules/70-rfswift.rules
var hostRules []byte

// hostHeaderPrefix marks the installed file as written by RF Swift, so it can
// be told apart from a user-managed file of the same name and removed again.
const hostHeaderPrefix = "# Installed by RF Swift (host rules)"

// UdevState values.
const (
	UdevMissing   = "missing"   // no file in UdevRulesDir
	UdevOutdated  = "outdated"  // RF Swift's file, but from another rfswift version
	UdevInstalled = "installed" // RF Swift's file, matching this binary
	UdevForeign   = "foreign"   // a file of that name RF Swift did not write
)

var ruleGroupRe = regexp.MustCompile(`GROUP\s*[:+]?=\s*"([^"]+)"`)

// HostRules returns the rules file content shipped in this binary.
func HostRules() []byte { return append([]byte(nil), hostRules...) }

// hostHeader is prepended to the installed copy.
func hostHeader() string {
	return hostHeaderPrefix + "; remove with: rfswift host udev --remove\n"
}

// splitHostInstalled separates RF Swift's header from the body of an installed
// file. ours is false for a file RF Swift did not write.
func splitHostInstalled(data []byte) (ours bool, body []byte) {
	if !bytes.HasPrefix(data, []byte(hostHeaderPrefix)) {
		return false, data
	}
	_, rest, _ := bytes.Cut(data, []byte("\n"))
	return true, rest
}

// ruleGroups lists the GROUP="..." values a rules file relies on.
func ruleGroups(content []byte) []string {
	seen := map[string]bool{}
	var groups []string
	for _, m := range ruleGroupRe.FindAllSubmatch(content, -1) {
		g := string(m[1])
		if !seen[g] {
			seen[g] = true
			groups = append(groups, g)
		}
	}
	return groups
}

// UdevStatus describes RF Swift's host rules on this machine.
type UdevStatus struct {
	Supported       bool     `json:"supported"`       // Linux only
	File            string   `json:"file"`            // 70-rfswift.rules
	Path            string   `json:"path"`            // /etc/udev/rules.d/70-rfswift.rules
	State           string   `json:"state"`           // missing | outdated | installed | foreign
	Groups          []string `json:"groups"`          // groups the rules rely on (plugdev)
	GroupsAbsent    []string `json:"groupsAbsent"`    // not present on the host
	GroupsNotMember []string `json:"groupsNotMember"` // present, user not a member (per account database)
	User            string   `json:"user"`            // the account the rules are set up for
	Ready           bool     `json:"ready"`           // installed and the groups are in place
	Detail          string   `json:"detail"`          // one-line summary for humans
}

// GetUdevStatus inspects UdevRulesDir and the groups; never changes anything.
func GetUdevStatus() UdevStatus {
	st := UdevStatus{File: HostRulesFile, Path: filepath.Join(udevRulesDir, HostRulesFile), Groups: ruleGroups(hostRules), User: InvokingUser()}
	if runtime.GOOS != "linux" {
		st.Detail = "udev rules apply to Linux hosts only"
		return st
	}
	st.Supported = true
	installed, err := os.ReadFile(st.Path)
	switch {
	case err != nil:
		st.State = UdevMissing
	default:
		ours, body := splitHostInstalled(installed)
		switch {
		case !ours:
			st.State = UdevForeign
		case bytes.Equal(body, hostRules):
			st.State = UdevInstalled
		default:
			st.State = UdevOutdated
		}
	}
	st.GroupsAbsent, st.GroupsNotMember = GroupStatus(st.Groups)
	if st.GroupsAbsent == nil {
		st.GroupsAbsent = []string{}
	}
	if st.GroupsNotMember == nil {
		st.GroupsNotMember = []string{}
	}
	st.Ready = st.State == UdevInstalled && len(st.GroupsAbsent) == 0 && len(st.GroupsNotMember) == 0
	switch st.State {
	case UdevMissing:
		st.Detail = "not installed: SDR/RF hardware needs root in rootless Podman and Nix environments"
	case UdevOutdated:
		st.Detail = "installed by an earlier rfswift; this version ships updated rules"
	case UdevForeign:
		st.Detail = st.Path + " was not written by RF Swift and is left untouched"
	case UdevInstalled:
		st.Detail = "installed"
		if len(st.GroupsAbsent) > 0 {
			st.Detail += "; group(s) missing on this host: " + strings.Join(st.GroupsAbsent, ", ")
		} else if len(st.GroupsNotMember) > 0 {
			st.Detail += "; " + st.User + " is not in group " + strings.Join(st.GroupsNotMember, ", ") + " yet (seat ACL still applies to the logged-in user)"
		}
	}
	return st
}

// UdevReport is what InstallUdevRules did.
type UdevReport struct {
	Installed     bool       `json:"installed"`     // the rules file was (re)written
	GroupsCreated []string   `json:"groupsCreated"` // groups that did not exist on the host
	GroupsJoined  []string   `json:"groupsJoined"`  // groups the user was added to (needs a new login)
	Status        UdevStatus `json:"status"`        // state after the change
	Detail        string     `json:"detail"`        // one-line summary for humans
}

// InstallUdevRules writes RF Swift's rules into UdevRulesDir (with a header
// marking them as ours), creates the groups they rely on, adds the invoking
// user to the ones it lacks, and reloads udev, in a single privileged call.
// A foreign file of the same name is never overwritten. Nothing happens when
// everything is already in place.
func InstallUdevRules() (UdevReport, error) {
	var report UdevReport
	before := GetUdevStatus()
	if !before.Supported {
		return report, fmt.Errorf("udev rules only apply on Linux")
	}
	if before.State == UdevForeign {
		return report, fmt.Errorf("%s exists and was not written by RF Swift; move it away first", before.Path)
	}
	joinGroups := append(append([]string{}, before.GroupsAbsent...), before.GroupsNotMember...)
	needFile := before.State != UdevInstalled
	if !needFile && len(joinGroups) == 0 {
		report.Status = before
		report.Detail = "already installed"
		return report, nil
	}
	staged := map[string]string{}
	if needFile {
		staging, err := os.MkdirTemp("", "rfswift-udev-")
		if err != nil {
			return report, err
		}
		defer os.RemoveAll(staging)
		path := filepath.Join(staging, HostRulesFile)
		if err := os.WriteFile(path, append([]byte(hostHeader()), hostRules...), 0o644); err != nil {
			return report, err
		}
		staged[HostRulesFile] = path
	}
	username := InvokingUser()
	script, err := UdevInstallScript(staged, before.GroupsAbsent, joinGroups, username)
	if err != nil {
		return report, err
	}
	if err := RunPrivileged(script); err != nil {
		return report, fmt.Errorf("udev installation failed: %w", err)
	}
	report.Installed = needFile
	report.GroupsCreated = before.GroupsAbsent
	if username != "" {
		report.GroupsJoined = joinGroups
	}
	report.Status = GetUdevStatus()
	switch {
	case report.Installed && len(report.GroupsJoined) > 0:
		report.Detail = fmt.Sprintf("rules installed and udev reloaded; added %s to group %s (active after the next login; the seat ACL already applies)", username, strings.Join(report.GroupsJoined, ", "))
	case report.Installed:
		report.Detail = "rules installed and udev reloaded"
	default:
		report.Detail = fmt.Sprintf("added %s to group %s (active after the next login)", username, strings.Join(report.GroupsJoined, ", "))
	}
	return report, nil
}

// udevRemoveScript is the privileged part of RemoveUdevRules.
func udevRemoveScript(path string) string {
	return "set -e\nrm -f " + ShellQuote(path) + "\n" + udevReloadScript
}

// RemoveUdevRules deletes RF Swift's rules file and reloads udev. Groups are
// left alone. Returns false when there was nothing of ours to remove; a
// foreign file of the same name is left in place.
func RemoveUdevRules() (bool, error) {
	st := GetUdevStatus()
	if !st.Supported {
		return false, fmt.Errorf("udev rules only apply on Linux")
	}
	switch st.State {
	case UdevMissing:
		return false, nil
	case UdevForeign:
		return false, fmt.Errorf("%s was not written by RF Swift; not removing it", st.Path)
	}
	if err := RunPrivileged(udevRemoveScript(st.Path)); err != nil {
		return false, fmt.Errorf("udev removal failed: %w", err)
	}
	return true, nil
}
