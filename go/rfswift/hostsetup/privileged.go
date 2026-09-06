/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Host setup - the privileged helpers shared by the udev rules, Docker
*  access and engine installation steps, and by the Nix engine's udev
*  installer.
*
*  Every privileged change RF Swift makes on a host goes through
*  RunPrivileged: one sudo call on a terminal, one graphical polkit prompt
*  (pkexec) from the Workbench, a plain shell when already root. The scripts
*  only ever hold names validated against strict patterns or single-quoted
*  values, and are built by the functions below so they can be unit-tested
*  and shown to the user before anything runs.
 */

package hostsetup

import (
	"fmt"
	"golang.org/x/term"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	validGroupRe    = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}\$?$`)
	validRuleFileRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.rules$`)
	// validUserRe accepts the account names usermod/grep can take unquoted in
	// the generated scripts (plus the '.' and '@' some sites use).
	validUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_.@-]{0,31}\$?$`)
)

// ValidUserName reports whether u is a plain account name.
func ValidUserName(u string) bool { return validUserRe.MatchString(u) }

// ValidGroupName reports whether g is a plain POSIX group name.
func ValidGroupName(g string) bool { return validGroupRe.MatchString(g) }

// ValidRuleFileName reports whether f is a bare udev rules file name (no path
// components).
func ValidRuleFileName(f string) bool { return validRuleFileRe.MatchString(f) }

// ShellQuote single-quotes a value for sh.
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// InvokingUser is the account that should own the devices and join the
// groups: the sudo caller when RF Swift was elevated, else the current user.
func InvokingUser() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return sudoUser
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// isTerminal reports whether f is a terminal-like character device, used to
// tell an interactive CLI (has a TTY for a sudo password prompt) from a GUI
// backend (no TTY, needs a graphical polkit prompt).
func isTerminal(f *os.File) bool {
	// A mode check is not enough: /dev/null is a character device too, and
	// a GUI or a script often hands it over as stdin.
	return f != nil && term.IsTerminal(int(f.Fd()))
}

// graphical is set by a GUI front end: the password prompt must then be a
// polkit dialog, never a sudo prompt on whatever terminal the GUI happened
// to be launched from (a Workbench started from a shell has a TTY stdin,
// and a sudo prompt there is invisible to the person using the window).
var graphical bool

// SetGraphical tells the privileged runners they serve a GUI.
func SetGraphical(on bool) { graphical = on }

// wantsPkexecFirst reports whether the polkit dialog must be tried before a
// terminal prompt.
func wantsPkexecFirst() bool { return graphical || !isTerminal(os.Stdin) }

// RunPrivileged runs a shell script as root: directly when already root, via
// sudo on an interactive terminal (one password prompt for the whole script),
// or via pkexec (a graphical polkit prompt) when there is no controlling
// terminal or the caller is a GUI, so the Workbench can apply host changes
// with a single click.
func RunPrivileged(script string) error {
	if os.Geteuid() == 0 {
		cmd := exec.Command("sh", "-c", script)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	_, hasSudo := exec.LookPath("sudo")
	pkexec, hasPkexec := exec.LookPath("pkexec")
	// On a terminal, prefer sudo (its prompt works inline); a GUI never.
	if !wantsPkexecFirst() && hasSudo == nil {
		cmd := exec.Command("sudo", "sh", "-c", script)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	// No terminal (GUI): use pkexec for a graphical password dialog.
	if hasPkexec == nil {
		cmd := exec.Command(pkexec, "/bin/sh", "-c", script)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	if hasSudo == nil {
		cmd := exec.Command("sudo", "sh", "-c", script)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("root privileges are needed for this change: install sudo or pkexec, or re-run as root")
}

// RunPrivilegedCommand runs a program as root the way RunPrivileged runs a
// script: directly when already root, via sudo on a terminal, via pkexec
// otherwise. Output is returned so a GUI can show what happened.
func RunPrivilegedCommand(argv ...string) (string, error) {
	if len(argv) == 0 {
		return "", fmt.Errorf("no command")
	}
	var cmd *exec.Cmd
	switch {
	case os.Geteuid() == 0:
		cmd = exec.Command(argv[0], argv[1:]...)
	case !wantsPkexecFirst():
		if _, err := exec.LookPath("sudo"); err != nil {
			return "", fmt.Errorf("root privileges are needed for this change: install sudo or re-run as root")
		}
		cmd = exec.Command("sudo", argv...)
		cmd.Stdin = os.Stdin
	default:
		pkexec, err := exec.LookPath("pkexec")
		if err != nil {
			if graphical {
				return "", fmt.Errorf("root privileges are needed for this change and no graphical prompt is available: install polkit (pkexec), or apply the change with the CLI (rfswift config ...)")
			}
			if _, sudoErr := exec.LookPath("sudo"); sudoErr != nil {
				return "", fmt.Errorf("root privileges are needed for this change: install pkexec (polkit) or sudo")
			}
			cmd = exec.Command("sudo", argv...)
		} else {
			cmd = exec.Command(pkexec, argv...)
		}
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// GroupStatus reports, for the given groups, which do not exist on the host
// and which existing ones the invoking user is not a member of (per the
// account database, i.e. after the next login). Both need fixing before a
// group-protected device is usable without root.
func GroupStatus(groups []string) (absent, notMember []string) {
	memberOf := map[string]bool{}
	if name := InvokingUser(); name != "" {
		if u, err := user.Lookup(name); err == nil {
			if ids, err := u.GroupIds(); err == nil {
				for _, id := range ids {
					memberOf[id] = true
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, g := range groups {
		if seen[g] {
			continue
		}
		seen[g] = true
		grp, err := user.LookupGroup(g)
		if err != nil {
			absent = append(absent, g)
			continue
		}
		if !memberOf[grp.Gid] {
			notMember = append(notMember, g)
		}
	}
	sort.Strings(absent)
	sort.Strings(notMember)
	return absent, notMember
}

// UdevInstallScript builds the privileged part of a udev rules installation:
// copy the staged files into UdevRulesDir, create missing groups, add the
// user to the groups it lacks, reload udev and re-trigger so the rules apply
// to devices that are already plugged in. Inputs are validated so the script
// only ever holds names we generated or matched against a strict pattern.
func UdevInstallScript(staged map[string]string, createGroups, joinGroups []string, username string) (string, error) {
	var b strings.Builder
	b.WriteString("set -e\n")
	files := make([]string, 0, len(staged))
	for file := range staged {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		if !validRuleFileRe.MatchString(file) {
			return "", fmt.Errorf("refusing unexpected rules file name %q", file)
		}
		fmt.Fprintf(&b, "install -m 0644 %s %s\n", ShellQuote(staged[file]), ShellQuote(filepath.Join(UdevRulesDir, file)))
	}
	for _, g := range createGroups {
		if !validGroupRe.MatchString(g) {
			return "", fmt.Errorf("refusing unexpected group name %q", g)
		}
		fmt.Fprintf(&b, "getent group %s >/dev/null || groupadd --system %s\n", g, g)
	}
	if username != "" {
		for _, g := range joinGroups {
			if !validGroupRe.MatchString(g) {
				return "", fmt.Errorf("refusing unexpected group name %q", g)
			}
			fmt.Fprintf(&b, "usermod -aG %s %s\n", g, ShellQuote(username))
		}
	}
	b.WriteString(udevReloadScript)
	return b.String(), nil
}

// udevReloadScript makes udev re-read the rules and re-apply them to the
// devices already present (permissions, group, seat ACL), so nothing has to
// be re-plugged for the common case.
const udevReloadScript = "udevadm control --reload-rules\nudevadm trigger\n"
