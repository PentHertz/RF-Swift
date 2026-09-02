/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - udev rules for the environment's hardware (Linux).
*
*  In a container the tools run as root with /dev/bus/usb mapped in, so no
*  udev setup is needed. Native Nix tools run as the user: HackRF, RTL-SDR,
*  bladeRF, Airspy, LimeSDR, USRP, Proxmark and friends are only usable once
*  their udev rules are on the host. Every nixpkgs package ships its rules
*  (lib/udev/rules.d or etc/udev/rules.d), which the environment profile
*  gathers, but /etc/udev/rules.d is root's. This file lists the rules an
*  environment provides, compares them with what is installed, and installs
*  them (plus the groups they rely on) in one sudo call.
 */

package nix

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// UdevRulesDir is where the host's udev reads local rules.
const UdevRulesDir = "/etc/udev/rules.d"

// udevInstalledDir is UdevRulesDir, overridable in tests.
var udevInstalledDir = UdevRulesDir

// udevHeaderPrefix marks rules files written by RF Swift so they can be told
// apart from the distribution's and removed again.
const udevHeaderPrefix = "# Installed by RF Swift (nix engine)"

// udevRuleDirs are the locations nixpkgs packages use for their rules.
var udevRuleDirs = []string{"lib/udev/rules.d", "etc/udev/rules.d"}

// UdevRule is one rules file shipped by a package of the environment.
type UdevRule struct {
	File    string   `json:"file"`    // 53-hackrf.rules
	Source  string   `json:"source"`  // store path of the file
	Package string   `json:"package"` // hackrf-2026.01.3
	Groups  []string `json:"groups"`  // GROUP="..." values the rules rely on
	State   string   `json:"state"`   // missing | outdated | installed
}

// UdevInstallReport is what InstallUdevRules did.
type UdevInstallReport struct {
	Installed     []string // rules files written to UdevRulesDir
	GroupsCreated []string // groups that did not exist on the host
	GroupsJoined  []string // groups the invoking user was added to (needs a new login)
}

var (
	ruleGroupRe     = regexp.MustCompile(`GROUP\s*[:+]?=\s*"([^"]+)"`)
	storePackageRe  = regexp.MustCompile(`^/nix/store/[0-9a-z]{32}-([^/]+)`)
	validGroupRe    = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}\$?$`)
	validRuleFileRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.rules$`)
)

// ListUdevRules collects the rules files of everything realised for env: the
// eager profile, the device prerequisite layer and the extras profiles. Only
// Linux has udev; elsewhere the list is empty.
func ListUdevRules(env *Environment) []UdevRule {
	if useWSL() {
		// udev runs inside the WSL distribution (with systemd), so its rules
		// matter there for usbipd-forwarded hardware; the Linux side lists them.
		return wslListUdevRules(env)
	}
	if runtime.GOOS != "linux" || env == nil {
		return nil
	}
	roots := []string{}
	if env.ProfilePath != "" {
		roots = append(roots, env.ProfilePath)
	}
	roots = append(roots, prerequisitesLink(env.Name), EnvExtrasProfile(env.Name), SharedExtrasProfile())
	seen := map[string]bool{}
	var rules []UdevRule
	for _, root := range roots {
		for _, r := range udevRulesIn(root) {
			if seen[r.File] {
				continue
			}
			seen[r.File] = true
			rules = append(rules, r)
		}
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].File < rules[j].File })
	return rules
}

// udevRulesIn scans one profile or store path for rules files.
func udevRulesIn(root string) []UdevRule {
	var rules []UdevRule
	for _, sub := range udevRuleDirs {
		entries, err := os.ReadDir(filepath.Join(root, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".rules") {
				continue
			}
			path := filepath.Join(root, sub, e.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			source := path
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				source = resolved
			}
			rules = append(rules, UdevRule{
				File:    e.Name(),
				Source:  source,
				Package: storePackageName(source),
				Groups:  ruleGroups(content),
				State:   udevRuleState(e.Name(), content),
			})
		}
	}
	return rules
}

// storePackageName turns /nix/store/<hash>-hackrf-2026.01.3/... into
// hackrf-2026.01.3 (the path itself when it is not a store path).
func storePackageName(path string) string {
	if m := storePackageRe.FindStringSubmatch(path); m != nil {
		return m[1]
	}
	return path
}

// ruleGroups lists the groups a rules file grants access to, sorted.
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
	sort.Strings(groups)
	return groups
}

// udevHeader is the two-line preamble written in front of an installed file.
func udevHeader(envName, source string) string {
	return fmt.Sprintf("%s for environment '%s'; remove with: rfswift nix udev %s --remove\n# Source: %s\n", udevHeaderPrefix, envName, envName, source)
}

// splitInstalled separates RF Swift's header from the rules body of an
// installed file. ours is false for a file RF Swift did not write.
func splitInstalled(data []byte) (ours bool, envName string, body []byte) {
	if !bytes.HasPrefix(data, []byte(udevHeaderPrefix)) {
		return false, "", data
	}
	first, rest, _ := bytes.Cut(data, []byte("\n"))
	if m := regexp.MustCompile(`for environment '([^']+)'`).FindSubmatch(first); m != nil {
		envName = string(m[1])
	}
	if bytes.HasPrefix(rest, []byte("# Source: ")) {
		_, rest, _ = bytes.Cut(rest, []byte("\n"))
	}
	return true, envName, rest
}

// udevRuleState compares a shipped rules file with the installed one.
func udevRuleState(file string, content []byte) string {
	installed, err := os.ReadFile(filepath.Join(udevInstalledDir, file))
	if err != nil {
		return "missing"
	}
	_, _, body := splitInstalled(installed)
	if bytes.Equal(body, content) {
		return "installed"
	}
	return "outdated"
}

// PendingUdevRules keeps the rules that are not installed as shipped.
func PendingUdevRules(rules []UdevRule) []UdevRule {
	var pending []UdevRule
	for _, r := range rules {
		if r.State != "installed" {
			pending = append(pending, r)
		}
	}
	return pending
}

// invokingUser is the account that should own the devices: the sudo caller
// when RF Swift was elevated, else the current user.
func invokingUser() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return sudoUser
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// GroupStatus reports, for the groups the rules rely on, which do not exist
// on the host and which existing ones the invoking user is not a member of.
// Both need fixing before the devices are accessible without root.
func GroupStatus(rules []UdevRule) (absent, notMember []string) {
	if useWSL() {
		return wslGroupStatus(rules)
	}
	wanted := map[string]bool{}
	for _, r := range rules {
		for _, g := range r.Groups {
			wanted[g] = true
		}
	}
	memberOf := map[string]bool{}
	if name := invokingUser(); name != "" {
		if u, err := user.Lookup(name); err == nil {
			if ids, err := u.GroupIds(); err == nil {
				for _, id := range ids {
					memberOf[id] = true
				}
			}
		}
	}
	for g := range wanted {
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

// udevInstallScript is the privileged part of an installation: copy the
// staged files into place, create missing groups, add the user to the groups
// it lacks, reload udev. Inputs are validated so the script only ever holds
// names we generated or matched against a strict pattern.
func udevInstallScript(staged map[string]string, createGroups, joinGroups []string, username string) (string, error) {
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
		fmt.Fprintf(&b, "install -m 0644 %s %s\n", shellQuote(staged[file]), shellQuote(filepath.Join(UdevRulesDir, file)))
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
			fmt.Fprintf(&b, "usermod -aG %s %s\n", g, shellQuote(username))
		}
	}
	b.WriteString("udevadm control --reload-rules\nudevadm trigger\n")
	return b.String(), nil
}

// shellQuote single-quotes a value for sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// isCharDevice reports whether f is a terminal-like character device, used to
// tell an interactive CLI (has a TTY for a sudo password prompt) from a GUI
// backend (no TTY, needs a graphical polkit prompt).
func isCharDevice(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// runPrivileged runs a shell script as root: directly when already root, via
// sudo on an interactive terminal (one password prompt for the whole script),
// or via pkexec (a graphical polkit prompt) when there is no controlling
// terminal - so the Workbench GUI can install udev rules with a single click.
func runPrivileged(script string) error {
	if os.Geteuid() == 0 {
		cmd := exec.Command("sh", "-c", script)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	_, hasSudo := exec.LookPath("sudo")
	pkexec, hasPkexec := exec.LookPath("pkexec")
	// On a terminal, prefer sudo (its prompt works inline).
	if isCharDevice(os.Stdin) && hasSudo == nil {
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
	return fmt.Errorf("root privileges are needed to write %s and reload udev: install sudo or pkexec, or re-run as root", UdevRulesDir)
}

// InstallUdevRules writes rules into /etc/udev/rules.d (each with a header
// naming the environment and its store source), creates the groups in
// createGroups, adds the invoking user to joinGroups, and reloads udev; all
// of it in a single privileged call. Nothing happens when there is nothing
// to do.
func InstallUdevRules(env *Environment, rules []UdevRule, createGroups, joinGroups []string) (UdevInstallReport, error) {
	if useWSL() {
		return wslInstallUdevRules(env, rules, createGroups, joinGroups)
	}
	var report UdevInstallReport
	if runtime.GOOS != "linux" {
		return report, fmt.Errorf("udev rules only apply on Linux")
	}
	if len(rules) == 0 && len(createGroups) == 0 && len(joinGroups) == 0 {
		return report, nil
	}
	staging, err := os.MkdirTemp("", "rfswift-udev-")
	if err != nil {
		return report, err
	}
	defer os.RemoveAll(staging)
	staged := map[string]string{}
	for _, r := range rules {
		content, err := os.ReadFile(r.Source)
		if err != nil {
			return report, fmt.Errorf("read %s: %w", r.Source, err)
		}
		path := filepath.Join(staging, r.File)
		if err := os.WriteFile(path, append([]byte(udevHeader(env.Name, r.Source)), content...), 0o644); err != nil {
			return report, err
		}
		staged[r.File] = path
		report.Installed = append(report.Installed, r.File)
	}
	username := invokingUser()
	script, err := udevInstallScript(staged, createGroups, joinGroups, username)
	if err != nil {
		return report, err
	}
	if err := runPrivileged(script); err != nil {
		return report, fmt.Errorf("udev installation failed: %w", err)
	}
	report.GroupsCreated = append(report.GroupsCreated, createGroups...)
	if username != "" {
		report.GroupsJoined = append(report.GroupsJoined, joinGroups...)
	}
	return report, nil
}

// InstalledUdevRules lists the rules files RF Swift wrote for envName ("" for
// any environment).
func InstalledUdevRules(envName string) ([]string, error) {
	entries, err := os.ReadDir(udevInstalledDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".rules") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(udevInstalledDir, e.Name()))
		if err != nil {
			continue
		}
		ours, owner, _ := splitInstalled(data)
		if ours && (envName == "" || owner == envName) {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

// RemoveUdevRules deletes the rules files RF Swift installed for envName and
// reloads udev. Groups are left alone.
func RemoveUdevRules(envName string) ([]string, error) {
	if useWSL() {
		return wslRemoveUdevRules(envName)
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("udev rules only apply on Linux")
	}
	files, err := InstalledUdevRules(envName)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, file := range files {
		if !validRuleFileRe.MatchString(file) {
			return nil, fmt.Errorf("refusing unexpected rules file name %q", file)
		}
		fmt.Fprintf(&b, "rm -f %s\n", shellQuote(filepath.Join(UdevRulesDir, file)))
	}
	b.WriteString("udevadm control --reload-rules\nudevadm trigger\n")
	if err := runPrivileged(b.String()); err != nil {
		return nil, fmt.Errorf("udev removal failed: %w", err)
	}
	return files, nil
}

// RealisePrerequisites builds the environment's device/library layer and pins
// it under the environment directory, so its udev rules can be collected even
// for on-demand environments where nothing else is realised yet.
func RealisePrerequisites(env *Environment) error {
	if env == nil || len(env.Prerequisites) == 0 || env.FlakeRef == "" {
		return nil
	}
	if pathExists(prerequisitesLink(env.Name)) {
		return nil
	}
	return buildPrerequisites(env.FlakeRef, env.Image, env.Prerequisites, prerequisitesLink(env.Name))
}
