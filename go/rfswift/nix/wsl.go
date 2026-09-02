/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine on Windows: the WSL 2 backend.
*
*  Nix has no Windows port, so on Windows the engine lives inside a WSL 2
*  distribution: the nix daemon, the Linux rfswift CLI, the environments under
*  ~/.rfswift/nix and the default workspaces are all there. This process - the
*  rfswift.exe CLI or the Workbench - is a front end for it:
*
*    - reads (manifests, audit reports, profile symlinks) go straight through
*      the distribution's share, \\wsl.localhost\<distro>\..., so ListEnvironments,
*      GetEnvironment, SecurityPosture and friends need no process at all;
*    - `nix ...` invocations run inside the distribution (nixCommand), with the
*      Windows paths in their arguments translated to what Linux sees;
*    - everything that must happen on the Linux side as a whole - realising and
*      entering an environment, installing tools, updates, rollbacks, udev rules,
*      export/import - is delegated to the Linux rfswift in the distribution,
*      which is the same code as the Linux CLI (rfswiftCommand + the wsl_*
*      functions in wsl_backend.go). Interactive commands inherit this console;
*      wsl.exe gives the Linux side a real pty, so wizards and shells work.
*
*  WSLg provides the X11/Wayland display and PulseAudio to the distribution,
*  usbipd-win forwards RF hardware into its kernel, so a Nix environment there
*  gets GUI tools, sound and USB radios like on a Linux host. The CLI command
*  group `rfswift nix wsl` (cli/nix_wsl.go) provisions and inspects the
*  distribution; SetupWSL (wsl_setup.go) is the provisioning logic itself.
 */

package nix

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"sync"

	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
)

// useWSL reports whether this process drives the Nix engine through a WSL 2
// distribution, which is the case on Windows.
func useWSL() bool { return runtime.GOOS == "windows" }

// wslBackendState caches the distribution choice and its probe for the
// process lifetime: resolving starts the distribution and runs a shell in it,
// which is too slow to repeat on every call.
type wslBackendState struct {
	mu     sync.Mutex
	done   bool
	status rfutils.WSLNixStatus
	err    error
}

var wslBackend wslBackendState

// wslState resolves (once) the distribution hosting the engine.
func wslState() (rfutils.WSLNixStatus, error) {
	wslBackend.mu.Lock()
	defer wslBackend.mu.Unlock()
	if !wslBackend.done {
		wslBackend.status, wslBackend.err = rfutils.ResolveWSLNix(common.ConfigFileByPlatform())
		wslBackend.done = true
	}
	return wslBackend.status, wslBackend.err
}

// ResetWSLBackend forgets the cached distribution probe so the next call
// re-detects it (after `rfswift nix wsl setup` or `rfswift nix wsl use`).
func ResetWSLBackend() {
	wslBackend.mu.Lock()
	defer wslBackend.mu.Unlock()
	wslBackend.done = false
	wslBackend.status, wslBackend.err = rfutils.WSLNixStatus{}, nil
}

// errNotWindows is returned by the WSL-only entry points elsewhere.
var errNotWindows = errors.New("the WSL 2 backend of the Nix engine only exists on Windows")

// WSLBackend returns the WSL 2 distribution serving the Nix engine on Windows
// and what it offers. The error explains why none is usable.
func WSLBackend() (rfutils.WSLNixStatus, error) {
	if !useWSL() {
		return rfutils.WSLNixStatus{}, errNotWindows
	}
	return wslState()
}

// WSLReadyError turns a backend status into the message a user needs: what is
// missing and the command that provisions it. nil when the backend is ready.
func WSLReadyError(st rfutils.WSLNixStatus, err error) error {
	if err != nil {
		return fmt.Errorf("the Nix engine on Windows runs inside a WSL 2 distribution, but none is usable: %v\n"+
			"  Set one up with: rfswift nix wsl setup", err)
	}
	if st.Ready() {
		return nil
	}
	return fmt.Errorf("WSL distribution %q is missing %s.\n"+
		"  Provision it with: rfswift nix wsl setup --distro %s\n"+
		"  (installs Nix with flakes and the Linux rfswift CLI; needs a network connection)",
		st.Distro, strings.Join(st.Missing(), " and "), st.Distro)
}

// wslReady returns the backend status, or the guidance error when it cannot
// serve the engine yet.
func wslReady() (rfutils.WSLNixStatus, error) {
	st, err := wslState()
	if err != nil || !st.Ready() {
		return st, WSLReadyError(st, err)
	}
	return st, nil
}

// wslDistro is the name of the backend distribution ("" when unresolved).
func wslDistro() string {
	st, _ := wslState()
	return st.Distro
}

// WSLWorkspaceRoot is where the Linux side keeps default workspaces
// (~/rfswift-workspace), as a Windows path Explorer can open. "" when the
// backend is not resolved.
func WSLWorkspaceRoot() string {
	st, err := wslState()
	if err != nil || st.Home == "" {
		return ""
	}
	return rfutils.WSLPathToWindows(st.Distro, st.Home+"/rfswift-workspace")
}

// wslWorkspaceHint tells a user of the Linux side running inside WSL 2 where
// Windows sees an environment's workspace (Explorer opens \\wsl.localhost\...).
// "" outside WSL, without a workspace, or when the workspace already lives on
// a Windows drive (/mnt/<drive>).
func wslWorkspaceHint(workspace string) string {
	distro := os.Getenv("WSL_DISTRO_NAME")
	if distro == "" || workspace == "" || !strings.HasPrefix(workspace, "/") || strings.HasPrefix(workspace, "/mnt/") {
		return ""
	}
	return fmt.Sprintf(`From Windows, this workspace is at \\wsl.localhost\%s%s`, distro, strings.ReplaceAll(workspace, "/", `\`))
}

// hostPath maps a path as the Linux side of the engine sees it to a path this
// process can open: on Windows a Linux absolute path becomes the
// distribution's share (or the drive it lives on for /mnt/<drive>); anywhere
// else paths are local already.
func hostPath(p string) string {
	if !useWSL() || p == "" || !strings.HasPrefix(p, "/") {
		return p
	}
	return rfutils.WSLPathToWindows(wslDistro(), p)
}

// translateWindowsPath maps a drive-letter or \\wsl.localhost path to the
// Linux side. Linux paths and non-paths are reported as not translated.
func translateWindowsPath(s string) (string, bool) {
	if rfutils.IsWindowsAbsPath(s) {
		return rfutils.WindowsPathToWSL(s)
	}
	if _, _, ok := rfutils.SplitWSLUNC(s); ok {
		return rfutils.WindowsPathToWSL(s)
	}
	return "", false
}

// translateArg rewrites one command-line argument for the Linux side:
// Windows paths, including in `--flag=C:\path` form, become Linux paths;
// everything else passes through untouched.
func translateArg(a string) string {
	if p, ok := translateWindowsPath(a); ok {
		return p
	}
	if strings.HasPrefix(a, "-") {
		if flag, value, ok := strings.Cut(a, "="); ok {
			if p, ok := translateWindowsPath(value); ok {
				return flag + "=" + p
			}
		}
	}
	return a
}

// translateArgs applies translateArg to a whole command line.
func translateArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = translateArg(a)
	}
	return out
}

// wslForwardedVars are the RF Swift settings a Windows user may have set that
// the Linux side must honour too; WSLENV carries them across.
var wslForwardedVars = []string{
	"RFSWIFT_ENGINE", "RFSWIFT_NIX_FLAKE", "RFSWIFT_NIX_GL", "RFSWIFT_NIX_HOME",
	"RFSWIFT_NIX_CATALOG", "RFSWIFT_NIX_BIN", "RFSWIFT_NO_BANNER", "NO_COLOR", "CLICOLOR_FORCE",
}

// wslPathVars are forwarded variables whose value is a path: a Windows path
// there is translated before it crosses.
var wslPathVars = []string{"RFSWIFT_NIX_FLAKE", "RFSWIFT_NIX_HOME", "RFSWIFT_NIX_CATALOG"}

// composeWSLENV appends variable names to an existing WSLENV value without
// duplicating entries (an entry may carry /p-style flags after its name).
func composeWSLENV(existing string, names []string) string {
	seen := map[string]bool{}
	var parts []string
	for _, p := range strings.Split(existing, ":") {
		if p == "" {
			continue
		}
		name, _, _ := strings.Cut(p, "/")
		if seen[name] {
			continue
		}
		seen[name] = true
		parts = append(parts, p)
	}
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			parts = append(parts, n)
		}
	}
	return strings.Join(parts, ":")
}

// wslChildEnv is the environment handed to wsl.exe: this process's, with the
// RF Swift settings listed in WSLENV so WSL copies them into the Linux
// process, path-valued ones translated first.
func wslChildEnv() []string {
	// This process already printed the banner (or is a GUI); the Linux side
	// must not print a second one, and must never mix it into --json output.
	overrides := map[string]string{"RFSWIFT_NO_BANNER": "1"}
	for _, name := range wslPathVars {
		if v := os.Getenv(name); v != "" {
			if p, ok := translateWindowsPath(v); ok {
				overrides[name] = p
			}
		}
	}
	var forwarded []string
	for _, name := range wslForwardedVars {
		if _, set := os.LookupEnv(name); set || overrides[name] != "" {
			forwarded = append(forwarded, name)
		}
	}
	overrides["WSLENV"] = composeWSLENV(os.Getenv("WSLENV"), forwarded)
	return withEnv(os.Environ(), overrides)
}

// wslExec prepares (does not start) argv inside the backend distribution with
// a login environment, Windows paths translated and settings forwarded.
func wslExec(argv ...string) *exec.Cmd {
	cmd := rfutils.WSLExec(wslDistro(), translateArgs(argv)...)
	cmd.Env = wslChildEnv()
	return cmd
}

// nixCommand prepares a nix invocation: the local binary on Linux and macOS,
// nix inside the WSL distribution on Windows. Every nix call of the package
// goes through here so the two hosts stay in step.
func nixCommand(args ...string) *exec.Cmd {
	if useWSL() {
		return wslExec(append([]string{"nix"}, args...)...)
	}
	return exec.Command(NixBinary(), args...)
}

// rfswiftCommand prepares a delegated operation: the Linux rfswift CLI inside
// the distribution. -q keeps it from querying GitHub for a newer release on
// every call (this process already did, or was told not to).
func rfswiftCommand(args ...string) *exec.Cmd {
	return wslExec(append([]string{"rfswift", "-q"}, args...)...)
}

// hasConsole reports whether this process has usable standard streams to hand
// to a child. A Windows GUI application (the Workbench) has none: its std
// handles are null, and a child that inherits them - or a copier writing to
// them - fails ("The handle is invalid"), while the Linux rfswift behind
// wsl.exe can die with SIGPIPE on its first print. Such callers get their
// output captured instead.
func hasConsole() bool {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		if f == nil {
			return false
		}
		if _, err := f.Stat(); err != nil {
			return false
		}
	}
	return true
}

// consoleWriter returns w when the process has a console, else nil (so a
// MultiWriter never includes an invalid handle).
func consoleWriter(w *os.File) io.Writer {
	if hasConsole() {
		return w
	}
	return nil
}

// multiWriter is io.MultiWriter without nil entries.
func multiWriter(ws ...io.Writer) io.Writer {
	var out []io.Writer
	for _, w := range ws {
		if w != nil {
			out = append(out, w)
		}
	}
	return io.MultiWriter(out...)
}

// runInteractive attaches cmd to this console and waits for it. Ctrl+C is
// left to the child - wsl.exe forwards it to the Linux process as SIGINT -
// instead of killing this front end and orphaning the shell inside. Without a
// console (GUI) the output is captured and its last line reported on failure.
func runInteractive(cmd *exec.Cmd) error {
	if !hasConsole() {
		captured := &tailWriter{max: 64 << 10}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, captured, captured
		if err := cmd.Run(); err != nil {
			if line := lastNonEmptyLine(stripANSI(captured.String())); line != "" {
				return fmt.Errorf("%s (%w)", line, err)
			}
			return err
		}
		return nil
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-interrupts:
			case <-done:
				return
			}
		}
	}()
	return cmd.Run()
}

// runJSON runs the Linux rfswift with args and decodes the JSON it prints.
func runJSON(dst any, args ...string) error {
	cmd := rfswiftCommand(args...)
	var stdout bytes.Buffer
	stderr := &tailWriter{max: 64 << 10}
	cmd.Stdout, cmd.Stderr = &stdout, stderr
	if err := cmd.Run(); err != nil {
		return wslCommandError(args, stderr.String(), err)
	}
	out := extractJSON(rfutils.DecodeConsoleOutput(stdout.Bytes()))
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		return fmt.Errorf("rfswift %s in WSL %s returned no JSON: %w", strings.Join(args, " "), wslDistro(), err)
	}
	return nil
}

// extractJSON returns the JSON document the CLI printed with --json, skipping
// any informational lines before it ("[i] ..." notices start with a bracket
// too, so the document is recognised by a line that is only its opening
// bracket, an empty document, or an object/array start).
func extractJSON(out string) string {
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case t == "[" || t == "{" || t == "[]" || t == "{}",
			strings.HasPrefix(t, "{\""), strings.HasPrefix(t, "[{"), strings.HasPrefix(t, "[\""):
			return strings.TrimSpace(strings.Join(lines[i:], "\n"))
		}
	}
	return strings.TrimSpace(out)
}

// runCapture runs the Linux rfswift and returns its combined output with
// terminal colours stripped, for dialogs.
func runCapture(args ...string) (string, error) {
	cmd := rfswiftCommand(args...)
	raw, err := cmd.CombinedOutput()
	out := stripANSI(rfutils.DecodeConsoleOutput(raw))
	if err != nil {
		return out, wslCommandError(args, out, err)
	}
	return out, nil
}

var ansiEscapeRe = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

// stripANSI removes terminal escape sequences.
func stripANSI(s string) string { return ansiEscapeRe.ReplaceAllString(s, "") }

// lastNonEmptyLine is the most relevant line of a command's output: the text
// of the last message box the CLI drew (its wrapped lines rejoined), else the
// last non-empty line.
func lastNonEmptyLine(s string) string {
	if msg := lastBoxedMessage(s); msg != "" {
		return msg
	}
	var last string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			last = t
		}
	}
	return last
}

// boxEdge is the set of characters the CLI's message boxes are drawn with.
const boxEdge = "│┃|┌┐└┘├┤╭╮╰╯─━┬┴┼ \t"

// lastBoxedMessage extracts the body of the last message box printed by the
// Linux CLI (common.PrintErrorMessage and friends draw a title row, a
// separator, then word-wrapped body lines between vertical bars). Lines are
// walked from the bottom: the body is everything between the bottom border
// and the separator, with the wrapping undone. "" when no box is present.
func lastBoxedMessage(s string) string {
	lines := strings.Split(s, "\n")
	var body []string
	inBox := false
	for i := len(lines) - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			continue
		}
		switch {
		case !inBox:
			if strings.HasPrefix(t, "└") || strings.HasPrefix(t, "╰") {
				inBox = true
			}
		case strings.HasPrefix(t, "├") || strings.HasPrefix(t, "┌") || strings.HasPrefix(t, "╭"):
			// Above the separator sits the title row; the body is complete.
			return strings.Join(body, " ")
		default:
			if inner := strings.Trim(t, boxEdge); inner != "" {
				body = append([]string{inner}, body...)
			}
		}
	}
	return ""
}

// wslCommandError makes a delegated failure actionable. An unknown flag or
// command means the Linux rfswift is older than this one and lacks the
// operation; otherwise nix's or rfswift's last line is what the user needs.
func wslCommandError(args []string, output string, err error) error {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "unknown flag") || strings.Contains(lower, "unknown command") || strings.Contains(lower, "unknown shorthand flag") {
		st, _ := wslState()
		version := st.RFSwiftVersion
		if version == "" {
			version = "unknown"
		}
		return fmt.Errorf("the rfswift inside WSL distribution %s (version %s) does not support this operation; this RF Swift is %s. Update it with: rfswift nix wsl setup --update", st.Distro, version, common.Version)
	}
	if line := lastNonEmptyLine(stripANSI(output)); line != "" {
		return fmt.Errorf("%s (rfswift %s in WSL %s)", line, strings.Join(args, " "), wslDistro())
	}
	return fmt.Errorf("rfswift %s in WSL %s: %w", strings.Join(args, " "), wslDistro(), err)
}

// hasEngineFlag reports whether a command line already selects an engine.
func hasEngineFlag(args []string) bool {
	for _, a := range args {
		if a == "--engine" || strings.HasPrefix(a, "--engine=") {
			return true
		}
	}
	return false
}

// WSLBridgeArgs prepares a Windows rfswift command line for the Linux rfswift.
// For a command that depends on the selected engine (run, exec, install) the
// Nix engine is made explicit, so the Linux side does not consult its own
// config file or RFSWIFT_ENGINE to find out what this process already decided.
func WSLBridgeArgs(args []string, engineCommand bool) []string {
	out := make([]string, 0, len(args)+2)
	if engineCommand && !hasEngineFlag(args) {
		out = append(out, "--engine", "nix")
	}
	return append(out, args...)
}

// RunRFSwiftInWSL runs the Linux rfswift inside the backend distribution with
// args, attached to this console, and returns its exit code. This is how the
// Windows CLI serves every Nix-engine command: same code, same wizards, same
// output, on the side where nix and the hardware are.
func RunRFSwiftInWSL(args []string) (int, error) {
	if _, err := wslReady(); err != nil {
		return 1, err
	}
	err := runInteractive(wslExec(append([]string{"rfswift"}, args...)...))
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return 1, err
}

// WSLShellCommand prepares a login shell inside the backend distribution, in
// the user's home, for `rfswift nix wsl shell`.
func WSLShellCommand() (*exec.Cmd, error) {
	st, err := wslState()
	if err != nil {
		return nil, err
	}
	wsl, err := rfutils.WSLExePath()
	if err != nil {
		return nil, err
	}
	return exec.Command(wsl, "-d", st.Distro, "--cd", "~"), nil
}

// RunWSLShell opens that shell on this console.
func RunWSLShell() error {
	cmd, err := WSLShellCommand()
	if err != nil {
		return err
	}
	return runInteractive(cmd)
}
