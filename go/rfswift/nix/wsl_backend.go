/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine on Windows: the operations delegated to the Linux rfswift inside
*  the WSL 2 distribution (see wsl.go for the model). Each wsl* function is the
*  Windows body of the exported function of the same name in the rest of the
*  package; the exported function branches here when useWSL() is true. The
*  Linux CLI is the contract: flags and --json outputs added for these calls
*  are in cli/nix.go, and a Linux rfswift too old to know them is reported as
*  such (wslCommandError) rather than failing obscurely.
 */

package nix

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
)

// wslRunArgs builds the Linux `rfswift run --engine nix ...` command line for
// RunOptions. Windows paths (workspace, local flake checkout) are translated
// when the command is prepared, not here, so this stays a pure mapping.
func wslRunArgs(opts RunOptions) []string {
	args := []string{"run", "--engine", "nix", "-i", opts.Image, "-n", opts.Name}
	if opts.Command != "" {
		args = append(args, "-e", opts.Command)
	}
	switch opts.Workspace {
	case "":
	case "none":
		args = append(args, "--no-workspace")
	default:
		args = append(args, "--workspace", opts.Workspace)
	}
	if opts.FlakeRef != "" {
		args = append(args, "--flake", opts.FlakeRef)
	}
	if opts.Rebuild {
		args = append(args, "--rebuild")
	}
	if opts.Pure {
		args = append(args, "--pure")
	}
	if opts.Lazy {
		args = append(args, "--lazy")
	}
	if opts.Isolate {
		args = append(args, "--isolate")
	}
	if opts.CreateOnly {
		args = append(args, "--create-only")
	}
	return args
}

// wslRunEnvironment creates, realises and (unless CreateOnly) enters an
// environment on the Linux side. The Linux CLI offers its own udev-rule
// installation before entering, so PreEnter is not called here.
func wslRunEnvironment(opts RunOptions) error {
	if _, err := wslReady(); err != nil {
		return err
	}
	if strings.TrimSpace(opts.Name) == "" {
		return fmt.Errorf("environment name is required (use -n)")
	}
	if err := ValidateEnvironmentName(opts.Name); err != nil {
		return err
	}
	if strings.TrimSpace(opts.Image) == "" {
		return fmt.Errorf("an environment image is required (use -i, e.g. -i sdr_light)")
	}
	return runInteractive(rfswiftCommand(wslRunArgs(opts)...))
}

// wslExecEnvironment re-enters an environment (or runs a command in it).
func wslExecEnvironment(name, command string) error {
	if _, err := wslReady(); err != nil {
		return err
	}
	if _, err := GetEnvironment(name); err != nil {
		return err
	}
	args := []string{"exec", "--engine", "nix", "-c", name}
	if command != "" {
		args = append(args, "-e", command)
	}
	return runInteractive(rfswiftCommand(args...))
}

// wslInteractiveCommand prepares the environment shell for a PTY front end
// (the Workbench terminal): wsl.exe attached to a ConPTY gives the Linux side
// a real pty. The distribution's login shell is used; a shell path from the
// Windows side would mean nothing there.
func wslInteractiveCommand(name string) (*exec.Cmd, error) {
	if _, err := wslReady(); err != nil {
		return nil, err
	}
	if _, err := GetEnvironment(name); err != nil {
		return nil, err
	}
	return rfswiftCommand("exec", "--engine", "nix", "-c", name), nil
}

// wslInstallPackages installs packages through the Linux CLI, streaming nix's
// stderr to this process and through the same phase parser the local path
// uses, so GUI progress works identically.
func wslInstallPackages(flakeRef string, pkgs []string, envName string, progress InstallProgress) error {
	if _, err := wslReady(); err != nil {
		return err
	}
	if envName != "" {
		if _, err := GetEnvironment(envName); err != nil {
			return err
		}
	}
	args := []string{"nix", "install"}
	if flakeRef != "" {
		args = append(args, "--flake", flakeRef)
	}
	if envName != "" {
		args = append(args, "--env", envName)
	}
	args = append(args, pkgs...)
	if progress != nil {
		progress(5, "Resolving package selection")
	}
	cmd := rfswiftCommand(args...)
	captured := &tailWriter{max: 64 << 10}
	// A GUI has no console: keep its invalid std handles out of the child and
	// of the writers, capture instead (the CLI still streams live).
	if hasConsole() {
		cmd.Stdin, cmd.Stdout = os.Stdin, os.Stdout
	} else {
		cmd.Stdout = captured
	}
	cmd.Stderr = multiWriter(consoleWriter(os.Stderr), captured, &nixInstallProgressWriter{progress: progress})
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install %s: %w", strings.Join(pkgs, " "), wslCommandError(args, captured.String(), err))
	}
	if progress != nil {
		progress(100, "Package installed")
	}
	return nil
}

// wslRemoveEnvironment deletes an environment on the Linux side (its gcroot
// symlinks live there; deleting through the share would not be reliable).
func wslRemoveEnvironment(name string) error {
	if !pathExists(EnvDir(name)) {
		return fmt.Errorf("environment '%s' not found", name)
	}
	return runInteractive(rfswiftCommand("nix", "remove", name))
}

func wslListGenerations(name string) ([]Generation, error) {
	if _, err := GetEnvironment(name); err != nil {
		return nil, err
	}
	var gens []Generation
	if err := runJSON(&gens, "nix", "generations", name, "--json"); err != nil {
		return nil, err
	}
	return gens, nil
}

func wslRollbackEnvironment(name, generation string) error {
	args := []string{"nix", "rollback", name}
	if generation != "" {
		args = append(args, generation)
	}
	return runInteractive(rfswiftCommand(args...))
}

func wslUpdateEnvironment(name string, opts UpdateOptions) error {
	args := []string{"nix", "update"}
	if opts.Check {
		args = append(args, "--check")
	} else {
		args = append(args, "--yes")
	}
	if opts.Input != "" {
		args = append(args, "--input", opts.Input)
	}
	return runInteractive(rfswiftCommand(append(args, name)...))
}

func wslCheckEnvironmentUpdateOutput(name, input string) (string, error) {
	args := []string{"nix", "update", "--check"}
	if input != "" {
		args = append(args, "--input", input)
	}
	return runCapture(append(args, name)...)
}

func wslRebuildEnvironment(name string) error {
	return runInteractive(rfswiftCommand("nix", "rebuild", name))
}

func wslExportEnvironment(name, outFile string, progress ExportProgress) error {
	if _, err := wslReady(); err != nil {
		return err
	}
	if progress != nil {
		progress(10, "Exporting inside the WSL distribution")
	}
	args := []string{"nix", "export", name}
	if outFile != "" {
		args = append(args, "-o", outFile)
	}
	if err := runInteractive(rfswiftCommand(args...)); err != nil {
		return err
	}
	if progress != nil {
		progress(100, "Environment export complete")
	}
	return nil
}

func wslImportEnvironment(inFile, newName, newWorkspace string, progress ImportProgress) error {
	if _, err := wslReady(); err != nil {
		return err
	}
	if !pathExists(inFile) {
		return fmt.Errorf("archive not found: %s", inFile)
	}
	if progress != nil {
		progress(10, "Importing inside the WSL distribution")
	}
	args := []string{"nix", "import", inFile}
	if newName != "" {
		args = append(args, "--name", newName)
	}
	if newWorkspace != "" {
		args = append(args, "--workspace", newWorkspace)
	}
	if err := runInteractive(rfswiftCommand(args...)); err != nil {
		return err
	}
	if progress != nil {
		progress(100, "Environment import complete")
	}
	return nil
}

// wslRunTool runs one tool from the pinned set through the Linux CLI, which
// applies the OpenGL runtime the tool needs inside WSLg.
func wslRunTool(flakeRef, tool string, args []string) error {
	if _, err := wslReady(); err != nil {
		return err
	}
	full := []string{"nix", "run", "--flake", flakeRef, tool}
	if len(args) > 0 {
		full = append(full, "--")
		full = append(full, args...)
	}
	return runInteractive(rfswiftCommand(full...))
}

// wslGLStatus asks the Linux side what OpenGL runtime the environment gets:
// the answer depends on the WSL kernel and WSLg, not on this Windows host.
func wslGLStatus(env *Environment) (GLStatus, error) {
	args := []string{"nix", "gl"}
	if env != nil {
		args = append(args, env.Name)
	}
	var st GLStatus
	err := runJSON(&st, append(args, "--json")...)
	return st, err
}

func wslGLProbe(env *Environment) (string, error) {
	args := []string{"nix", "gl"}
	if env != nil {
		args = append(args, env.Name)
	}
	return runCapture(append(args, "--check")...)
}

// udevReport is the --json shape of `rfswift nix udev <name> --list`.
type udevReport struct {
	Rules           []UdevRule `json:"rules"`
	GroupsAbsent    []string   `json:"groupsAbsent"`
	GroupsNotMember []string   `json:"groupsNotMember"`
}

func wslUdevReport(name string) (udevReport, error) {
	var report udevReport
	err := runJSON(&report, "nix", "udev", name, "--list", "--json")
	return report, err
}

func wslListUdevRules(env *Environment) []UdevRule {
	if env == nil {
		return nil
	}
	report, err := wslUdevReport(env.Name)
	if err != nil {
		common.PrintWarningMessage(fmt.Sprintf("Could not list udev rules in WSL: %v", err))
		return nil
	}
	return report.Rules
}

// wslGroupStatus resolves the groups inside the distribution: which do not
// exist and which the login user is not a member of. Group names are checked
// against the same pattern the installer enforces before reaching a shell.
func wslGroupStatus(rules []UdevRule) (absent, notMember []string) {
	wanted := map[string]bool{}
	var groups []string
	for _, r := range rules {
		for _, g := range r.Groups {
			if validGroupRe.MatchString(g) && !wanted[g] {
				wanted[g] = true
				groups = append(groups, g)
			}
		}
	}
	if len(groups) == 0 {
		return nil, nil
	}
	const script = `for g in "$@"; do
  if getent group "$g" >/dev/null 2>&1; then
    if id -Gn 2>/dev/null | tr ' ' '\n' | grep -qx -- "$g"; then echo "member=$g"; else echo "notmember=$g"; fi
  else echo "absent=$g"; fi
done`
	cmd := wslExec(append([]string{"sh", "-c", script, "rfswift-groups"}, groups...)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "absent":
			absent = append(absent, value)
		case "notmember":
			notMember = append(notMember, value)
		}
	}
	return absent, notMember
}

// UsesWSL reports whether this process serves the Nix engine through a WSL 2
// distribution (Windows). GUI callers gate Linux-host-only actions on it.
func UsesWSL() bool { return useWSL() }

// WSLEnvironmentCommand prepares (does not start) a non-interactive command
// inside an environment for GUI callers such as the Workbench command runner:
// the Linux rfswift runs it with the environment's PATH, plugin paths and
// OpenGL runtime, as `rfswift exec -e` does.
func WSLEnvironmentCommand(name, command string) (*exec.Cmd, error) {
	if !useWSL() {
		return nil, errNotWindows
	}
	if _, err := wslReady(); err != nil {
		return nil, err
	}
	if _, err := GetEnvironment(name); err != nil {
		return nil, err
	}
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	return rfswiftCommand("exec", "--engine", "nix", "-c", name, "-e", command), nil
}

// rfswiftCommandAsRoot prepares the Linux rfswift as root inside the
// distribution while keeping the login user's state: SUDO_USER makes the
// Linux side resolve that user's ~/.rfswift and group membership, exactly as
// under sudo. WSL grants root to the Windows user without a password, so the
// privileged steps (udev rules in /etc) need no terminal for a sudo prompt
// and work from the GUI as well.
func rfswiftCommandAsRoot(args ...string) *exec.Cmd {
	st, _ := wslState()
	argv := append([]string{"env", "SUDO_USER=" + st.User, "rfswift", "-q"}, translateArgs(args)...)
	cmd := rfutils.WSLExecAs(st.Distro, "root", argv...)
	cmd.Env = wslChildEnv()
	return cmd
}

// wslInstallUdevRules installs the rules through the Linux CLI, as root in the
// distribution (no sudo prompt). The report mirrors what was requested: the
// Linux side prints the details.
func wslInstallUdevRules(env *Environment, rules []UdevRule, createGroups, joinGroups []string) (UdevInstallReport, error) {
	var report UdevInstallReport
	if len(rules) == 0 && len(createGroups) == 0 && len(joinGroups) == 0 {
		return report, nil
	}
	if _, err := wslReady(); err != nil {
		return report, err
	}
	args := []string{"nix", "udev", env.Name, "--yes"}
	if len(createGroups) == 0 && len(joinGroups) == 0 {
		args = append(args, "--no-groups")
	}
	if err := runInteractive(rfswiftCommandAsRoot(args...)); err != nil {
		return report, fmt.Errorf("udev installation in WSL failed: %w", err)
	}
	for _, r := range rules {
		report.Installed = append(report.Installed, r.File)
	}
	report.GroupsCreated = append(report.GroupsCreated, createGroups...)
	report.GroupsJoined = append(report.GroupsJoined, joinGroups...)
	return report, nil
}

var udevRemovedRe = regexp.MustCompile(`Removed (.+?) from `)

func wslRemoveUdevRules(envName string) ([]string, error) {
	if _, err := wslReady(); err != nil {
		return nil, err
	}
	cmd := rfswiftCommandAsRoot("nix", "udev", envName, "--remove")
	raw, err := cmd.CombinedOutput()
	out := stripANSI(rfutils.DecodeConsoleOutput(raw))
	if err != nil {
		return nil, wslCommandError([]string{"nix", "udev", envName, "--remove"}, out, err)
	}
	if m := udevRemovedRe.FindStringSubmatch(out); m != nil {
		var files []string
		for _, f := range strings.Split(m[1], ",") {
			if f = strings.TrimSpace(f); f != "" {
				files = append(files, f)
			}
		}
		return files, nil
	}
	return nil, nil
}
