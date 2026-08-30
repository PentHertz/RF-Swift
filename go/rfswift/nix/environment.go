/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - lifecycle of dedicated named environments.
*
*  An environment is the Nix analogue of an RF Swift container: created once,
*  re-entered, and removed. Each lives in ~/.rfswift/nix/environments/<name> and
*  is realised as a buildEnv closure pinned by a gcroot symlink (./profile), so
*  it survives `nix store gc` and works offline after the first build.
 */

package nix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	common "penthertz/rfswift/common"
)

// RunEnvironment creates (if needed), realises, and enters an environment.
func RunEnvironment(opts RunOptions) error {
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH.\n" +
			"  Install it from https://nixos.org/download (multi-user recommended):\n" +
			"    sh <(curl -L https://nixos.org/nix/install) --daemon\n" +
			"  or use the Determinate installer, then re-run: rfswift run --engine nix")
	}
	if strings.TrimSpace(opts.Name) == "" {
		return fmt.Errorf("environment name is required (use -n)")
	}
	if strings.TrimSpace(opts.Image) == "" {
		return fmt.Errorf("an environment image is required (use -i, e.g. -i sdr_light)")
	}

	cat, err := LoadCatalog()
	if err != nil {
		return fmt.Errorf("failed to load environment catalog: %w", err)
	}
	entry := cat.Find(opts.Image)
	if entry == nil {
		return fmt.Errorf("unknown environment '%s'. See available ones with: rfswift nix catalog", opts.Image)
	}

	flakeRef := ResolveFlakeRef(opts.FlakeRef)
	envdir := EnvDir(opts.Name)
	if err := ensureDir(envdir); err != nil {
		return fmt.Errorf("failed to create environment dir: %w", err)
	}

	// Resolve and prepare the workspace (working directory).
	workspace := resolveWorkspace(opts.Name, opts.Workspace)
	if workspace != "" {
		if err := ensureDir(workspace); err != nil {
			return fmt.Errorf("failed to create workspace %s: %w", workspace, err)
		}
	}

	env := &Environment{
		Name:          opts.Name,
		Image:         entry.Name,
		FlakeRef:      flakeRef,
		Packages:      entry.Packages,
		Prerequisites: entry.Prerequisites,
		Workspace:     workspace,
		Command:       opts.Command,
		Created:       time.Now(),
		Isolate:       opts.Isolate,
	}

	// Realise the environment. Three modes:
	switch {
	case opts.Lazy:
		// On-demand: the application tools are not prebuilt - each becomes a shim
		// that builds and runs it the first time it is called. The prerequisite
		// device/driver layer (small: drivers, libraries and their udev rules) is
		// still realised up front, so hardware works and `rfswift nix udev` /
		// the entry-time rule offer can see the rules even in lazy mode.
		env.Lazy = true
		env.ProfilePath = ""
		common.PrintInfoMessage(fmt.Sprintf("Preparing on-demand environment '%s' (%s). Tools build the first time you call them.", entry.Name, opts.Image))
		if err := buildPrerequisites(flakeRef, entry.Name, entry.Prerequisites, prerequisitesLink(opts.Name)); err != nil {
			return err
		}
		env.Commands = resolveCommands(flakeRef, entry.Packages)
		if err := writeShims(env); err != nil {
			return fmt.Errorf("failed to set up on-demand environment: %w", err)
		}
	case opts.Pure:
		// Pure mode does not use a prebuilt profile; it evaluates the devShell
		// fresh each time with a clean environment.
		env.ProfilePath = ""
		if err := buildPrerequisites(flakeRef, entry.Name, entry.Prerequisites, prerequisitesLink(opts.Name)); err != nil {
			return err
		}
	default:
		// Eager: build (or refresh) the whole tool closure and pin it. We always
		// realise on `run`, so re-running with an existing name picks up catalog
		// or flake changes instead of silently reusing a stale profile. When
		// nothing changed this is a fast no-op against the Nix cache. Use `exec`
		// to re-enter without rebuilding.
		profile := profileLink(opts.Name)
		if err := buildPrerequisites(flakeRef, entry.Name, entry.Prerequisites, prerequisitesLink(opts.Name)); err != nil {
			return err
		}
		common.PrintInfoMessage(fmt.Sprintf("Realising environment '%s' (%s) from %s ...", entry.Name, opts.Image, flakeRef))
		common.PrintInfoMessage("First build fetches and compiles; refreshing an unchanged env is near-instant.")
		if err := buildProfile(flakeRef, entry.Name, profile); err != nil {
			return err
		}
		env.ProfilePath = profile
	}

	if err := writeManifest(env); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Make security awareness routine: surface the environment's posture (or a
	// nudge to check it) right after building, without blocking entry.
	if PostureIsStale(env.Name) {
		common.PrintInfoMessage(fmt.Sprintf("Security: not yet audited - check this environment with: rfswift nix audit %s", env.Name))
	} else {
		common.PrintInfoMessage("Security: " + SecurityPosture(env.Name))
	}

	if opts.CreateOnly {
		common.PrintSuccessMessage(fmt.Sprintf("Environment '%s' is ready.", opts.Name))
		return nil
	}
	if opts.PreEnter != nil {
		opts.PreEnter(env)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Environment '%s' ready. Entering shell (exit to leave).", opts.Name))
	return enter(env, opts.Command, opts.Pure, GLEnvironment(env, true))
}

// ExecEnvironment re-enters an existing environment, optionally running a command.
func ExecEnvironment(name, command string) error {
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH")
	}
	env, err := GetEnvironment(name)
	if err != nil {
		return err
	}
	switch {
	case env.Lazy:
		// Regenerate the shims if they were lost (e.g. manifest copied between
		// machines) so tools remain callable on demand.
		if !pathExists(shimsDir(name)) {
			if env.Commands == nil {
				env.Commands = resolveCommands(env.FlakeRef, env.Packages)
			}
			if err := writeShims(env); err != nil {
				return err
			}
		}
	case env.ProfilePath != "":
		// Eager: make sure it is realised (a user may have run `nix store gc`
		// after the gcroot was removed, or copied the manifest between machines).
		if !pathExists(env.ProfilePath) {
			common.PrintInfoMessage(fmt.Sprintf("Environment '%s' not realised yet, building ...", name))
			if err := buildProfile(env.FlakeRef, env.Image, env.ProfilePath); err != nil {
				return err
			}
		}
	}
	// pure only when it is neither a profile nor a lazy environment.
	pure := env.ProfilePath == "" && !env.Lazy
	return enter(env, command, pure, GLEnvironment(env, false))
}

// ListEnvironments returns all created environments, newest first.
func ListEnvironments() ([]*Environment, error) {
	dir := EnvironmentsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var envs []*Environment
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		env, err := GetEnvironment(e.Name())
		if err != nil {
			continue // skip unreadable/partial dirs
		}
		envs = append(envs, env)
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Created.After(envs[j].Created) })
	return envs, nil
}

// Realised reports whether a profile-based environment has its closure built
// and pinned on disk. Pure environments (no ProfilePath) are built on demand
// and always report false.
func (e *Environment) Realised() bool {
	return e.ProfilePath != "" && pathExists(e.ProfilePath)
}

// GetEnvironment loads one environment's manifest.
func GetEnvironment(name string) (*Environment, error) {
	data, err := os.ReadFile(manifestPath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("environment '%s' not found. List them with: rfswift nix list", name)
		}
		return nil, err
	}
	var env Environment
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("environment '%s' has a corrupt manifest: %w", name, err)
	}
	if env.Name == "" {
		env.Name = name
	}
	return &env, nil
}

// RemoveEnvironment deletes an environment and its gcroot. The underlying store
// paths are freed by the next `nix store gc`.
func RemoveEnvironment(name string) error {
	dir := EnvDir(name)
	if !pathExists(dir) {
		return fmt.Errorf("environment '%s' not found", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to remove environment '%s': %w", name, err)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Removed environment '%s'", name))
	return nil
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

// buildProfile realises packages.<currentSystem>.<image> into a gcroot symlink.
func buildProfile(flakeRef, image, outLink string) error {
	installable := fmt.Sprintf("%s#%s", flakeRef, image)
	args := append(experimentalArgs(),
		"build", installable,
		"--out-link", outLink,
		"--print-build-logs",
	)
	cmd := exec.Command(NixBinary(), args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nix build failed for %s: %w\n"+
			"  If this is a hash-mismatch on a source package, pin it (see RF-Swift-nix/pkgs/README.md).", installable, err)
	}
	return nil
}

// buildPrerequisites realises the environment's declared runtime driver and
// library layer before applications. Nix still owns dependency correctness;
// this extra phase primarily guarantees separately packaged runtime plugins
// (for example SoapySDR modules) are present before GUI tools start probing.
// outLink pins the layer (its udev rules are read from there); "" for no link.
func buildPrerequisites(flakeRef, image string, prerequisites []string, outLink string) error {
	if len(prerequisites) == 0 {
		return nil
	}
	installable := fmt.Sprintf("%s#%s-prerequisites", flakeRef, image)
	common.PrintInfoMessage(fmt.Sprintf("Realising device/library prerequisites for '%s' ...", image))
	link := []string{"--no-link"}
	if outLink != "" {
		link = []string{"--out-link", outLink}
	}
	args := append(experimentalArgs(), "build", installable)
	args = append(args, link...)
	args = append(args, "--print-build-logs")
	cmd := exec.Command(NixBinary(), args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nix prerequisite build failed for %s: %w", installable, err)
	}
	return nil
}

// resolveCommands maps each tool that has a main program to the flake attribute
// that provides it, by evaluating meta.mainProgram against the pinned package
// set. Best-effort: on any failure it falls back to a heuristic (the last path
// component of each attribute) so shims are still created.
func resolveCommands(flakeRef string, packages []string) map[string]string {
	out := map[string]string{}
	if data, err := evalMainPrograms(flakeRef, packages); err == nil {
		for attr, main := range data {
			if main != "" {
				out[main] = attr
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	common.PrintWarningMessage("Could not resolve tool command names from the flake; using attribute names for shims.")
	for _, p := range packages {
		name := p
		if i := strings.LastIndexByte(p, '.'); i >= 0 {
			name = p[i+1:]
		}
		out[name] = p
	}
	return out
}

// EvalVersions resolves each package's version in a single nix evaluation,
// returning a map of attribute path -> version string ("" when unknown). Used by
// `nix info --versions` to show what's in an environment without a build.
func EvalVersions(flakeRef string, packages []string) map[string]string {
	names, _ := json.Marshal(packages)
	expr := fmt.Sprintf(`
let
  f = builtins.getFlake %q;
  lib = f.inputs.nixpkgs.lib;
  sys = builtins.currentSystem;
  lp = f.legacyPackages.${sys};
  names = builtins.fromJSON ''%s'';
  ver = n: let t = builtins.tryEval (let p = lib.attrByPath (lib.splitString "." n) null lp; in if p == null then null else (p.version or (p.name or null))); in if t.success && t.value != null then t.value else "";
in builtins.listToAttrs (map (n: { name = n; value = ver n; }) names)
`, flakeRef, string(names))
	args := append(experimentalArgs(), "eval", "--impure", "--json", "--expr", expr)
	cmd := exec.Command(NixBinary(), args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		return map[string]string{}
	}
	return m
}

// evalMainPrograms asks nix for the meta.mainProgram of each package in one
// evaluation. Returns a map of attribute path -> command name (empty when the
// package has no main program, e.g. a library).
func evalMainPrograms(flakeRef string, packages []string) (map[string]string, error) {
	names, _ := json.Marshal(packages)
	expr := fmt.Sprintf(`
let
  f = builtins.getFlake %q;
  lib = f.inputs.nixpkgs.lib;
  sys = builtins.currentSystem;
  lp = f.legacyPackages.${sys};
  names = builtins.fromJSON ''%s'';
  main = n: let t = builtins.tryEval (let p = lib.attrByPath (lib.splitString "." n) null lp; in if p == null then null else (p.meta.mainProgram or null)); in if t.success && t.value != null then t.value else "";
in builtins.listToAttrs (map (n: { name = n; value = main n; }) names)
`, flakeRef, string(names))
	args := append(experimentalArgs(), "eval", "--impure", "--json", "--expr", expr)
	cmd := exec.Command(NixBinary(), args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// writeShims creates the build-on-first-call wrapper scripts for a lazy
// environment: one per command in env.Commands.
func writeShims(env *Environment) error {
	dir := shimsDir(env.Name)
	if err := ensureDir(dir); err != nil {
		return err
	}
	nixbin := NixBinary()
	for command, attr := range env.Commands {
		if command == "" || strings.ContainsAny(command, "/ ") {
			continue
		}
		installable := fmt.Sprintf("%s#%s", env.FlakeRef, attr)
		prereq := fmt.Sprintf("%s#%s-prerequisites", env.FlakeRef, env.Image)
		prereqStep := ""
		if len(env.Prerequisites) > 0 {
			prereqStep = fmt.Sprintf("%s --extra-experimental-features \"nix-command flakes\" build --out-link %q %q || exit $?\n", nixbin, prerequisitesLink(env.Name), prereq)
		}
		script := fmt.Sprintf(`#!/bin/sh
# RF Swift lazy tool shim: builds %s on first call, then runs it.
%s
exec %s --extra-experimental-features "nix-command flakes" run %q -- "$@"
`, attr, prereqStep, nixbin, installable)
		if err := os.WriteFile(filepath.Join(dir, command), []byte(script), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// setupX11 best-effort grants local clients access to the X server so GUI RF
// tools (gqrx, sdrpp, wireshark, ...) can open a window. It only acts when a
// DISPLAY is present; on a headless session there is nothing to connect to and
// GUI tools cannot run regardless.
func setupX11() {
	if runtime.GOOS != "linux" {
		return
	}
	if os.Getenv("DISPLAY") == "" {
		return
	}
	if _, err := exec.LookPath("xhost"); err != nil {
		return
	}
	_ = exec.Command("xhost", "+local:").Run()
}

// warnIfNoDisplay nudges the user when a GUI tool has nowhere to draw, with the
// options that need no host X server.
func warnIfNoDisplay() {
	if runtime.GOOS != "linux" {
		return
	}
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return
	}
	common.PrintInfoMessage("No display detected (DISPLAY unset). For GUI tools: reconnect with 'ssh -X', set DISPLAY, or run a Qt tool headless with 'QT_QPA_PLATFORM=vnc <tool>' and connect a VNC client to port 5900.")
}

// enter launches an interactive shell (or runs a command) inside the
// environment. gl holds the OpenGL runtime variables for non-NixOS hosts (see
// gl.go), nil when none are needed.
func enter(env *Environment, command string, pure bool, gl map[string]string) error {
	setupX11()
	warnIfNoDisplay()

	workdir := env.Workspace
	if workdir == "" || !pathExists(workdir) {
		workdir, _ = os.Getwd()
	}

	if pure {
		// Evaluate the devShell fresh, with a clean environment.
		shell := userShell()
		nixArgs := append(experimentalArgs(),
			"develop", fmt.Sprintf("%s#%s", env.FlakeRef, env.Image),
			"--ignore-environment",
		)
		for _, key := range glEnvKeys(gl) {
			nixArgs = append(nixArgs, "--keep", key)
		}
		if command != "" {
			nixArgs = append(nixArgs, "--command", shell, "-c", command)
		} else {
			nixArgs = append(nixArgs, "--command", shell)
		}
		cmd := exec.Command(NixBinary(), nixArgs...)
		cmd.Env = withEnv(os.Environ(), gl)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Dir = workdir
		if env.Isolate {
			jailed, err := IsolateCommand(cmd, env, workdir)
			if err != nil {
				return err
			}
			cmd = jailed
		}
		return cmd.Run()
	}

	// Lazy environments put their build-on-first-call shims on PATH; eager ones
	// put the realised profile's bin dir.
	binDir := filepath.Join(env.ProfilePath, "bin")
	if env.Lazy {
		binDir = shimsDir(env.Name)
	}
	// Prepend any packages the user added with `rfswift nix install` - both the
	// per-environment extras and the shared ones - so they are on PATH too.
	pathParts := []string{binDir}
	if p := filepath.Join(EnvExtrasProfile(env.Name), "bin"); pathExists(p) {
		pathParts = append(pathParts, p)
	}
	if p := filepath.Join(SharedExtrasProfile(), "bin"); pathExists(p) {
		pathParts = append(pathParts, p)
	}
	pathParts = append(pathParts, os.Getenv("PATH"))
	vars := map[string]string{
		"PATH":            strings.Join(pathParts, string(os.PathListSeparator)),
		"RFSWIFT_NIX_ENV": env.Name,
		"RFSWIFT_ENGINE":  "nix",
	}
	for k, v := range gl {
		vars[k] = v
	}
	for k, v := range pluginPathEnv(env) {
		vars[k] = v
	}
	childEnv := withEnv(os.Environ(), vars)

	shell := userShell()
	var cmd *exec.Cmd
	if command != "" {
		cmd = exec.Command(shell, "-c", command)
	} else if filepath.Base(shell) == "bash" {
		rc, err := writeBashRC(env, binDir)
		if err == nil {
			cmd = exec.Command(shell, "--rcfile", rc, "-i")
		} else {
			cmd = exec.Command(shell, "-i")
		}
	} else {
		cmd = exec.Command(shell, "-i")
	}
	cmd.Env = childEnv
	cmd.Dir = workdir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if env.Isolate {
		jailed, err := IsolateCommand(cmd, env, workdir)
		if err != nil {
			return err
		}
		cmd = jailed
	}
	return cmd.Run()
}

// soapyModulesGlob matches SoapySDR's per-ABI module directory (modules0.8-3, ...).
const soapyModulesGlob = "lib/SoapySDR/modules*"

// pluginPathEnv returns the search-path variables for libraries that find
// their device modules by directory, for an environment's realised profiles.
// SoapySDR (SOAPY_SDR_PLUGIN_PATH): every tool in the environment is linked
// against RF Swift's own plugin set already, so this adds what that set cannot
// know about - a Soapy module the user installed later with `rfswift nix
// install` (the extras profiles) or one shipped by a package a tool was not
// linked against. The profile merges every package's module directory, so a
// single path covers them all; the host's own value stays behind it.
func pluginPathEnv(env *Environment) map[string]string {
	if env == nil {
		return nil
	}
	var roots []string
	if env.ProfilePath != "" {
		roots = append(roots, env.ProfilePath)
	}
	roots = append(roots, EnvExtrasProfile(env.Name), SharedExtrasProfile())
	var dirs []string
	for _, root := range roots {
		matches, _ := filepath.Glob(filepath.Join(root, soapyModulesGlob))
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && fi.IsDir() {
				dirs = append(dirs, m)
			}
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	value := strings.Join(dirs, string(os.PathListSeparator))
	if cur := os.Getenv("SOAPY_SDR_PLUGIN_PATH"); cur != "" {
		value += string(os.PathListSeparator) + cur
	}
	return map[string]string{"SOAPY_SDR_PLUGIN_PATH": value}
}

// writeBashRC creates a throwaway rcfile that sources the user's own bashrc,
// shows a short banner, and marks the prompt so it is obvious you are inside an
// RF Swift Nix environment.
func writeBashRC(env *Environment, binDir string) (string, error) {
	rc := filepath.Join(EnvDir(env.Name), "bashrc")
	toolLine := fmt.Sprintf("~%d tools on PATH. Type 'exit' to leave.", len(env.Packages))
	if env.Lazy {
		toolLine = fmt.Sprintf("%d tools available; each builds the first time you call it. Type 'exit' to leave.", len(env.Commands))
	}
	content := fmt.Sprintf(`# Generated by RF Swift (nix engine). Do not edit.
if [ -f /etc/bashrc ]; then . /etc/bashrc; fi
if [ -f "$HOME/.bashrc" ]; then . "$HOME/.bashrc"; fi
export PATH=%q:"$PATH"
export RFSWIFT_NIX_ENV=%q
PS1="(rfswift:%s) $PS1"
# Run an environment tool as root: sudo resets the environment, so pass PATH,
# the display and the OpenGL runtime (non-NixOS hosts) through.
rfsudo() {
  local keep=() v
  for v in DISPLAY XAUTHORITY WAYLAND_DISPLAY LD_LIBRARY_PATH LIBGL_DRIVERS_PATH LIBVA_DRIVERS_PATH GBM_BACKENDS_PATH __EGL_VENDOR_LIBRARY_FILENAMES; do
    [ -n "${!v:-}" ] && keep+=("$v=${!v}")
  done
  sudo env "PATH=$PATH" "${keep[@]}" "$@"
}
echo ""
echo "  RF Swift (nix) - environment '%s' [%s]"
echo "  %s"
echo "  Root: run a tool with sudo via 'rfsudo <tool>' (e.g. rfsudo airmon-ng)."
if [ -n "${RFSWIFT_NIX_GL_RUNTIME:-}" ]; then echo "  OpenGL: nix GL runtime active for GUI tools (rfswift nix gl %s)."; fi
echo ""
`, binDir, env.Name, env.Name, env.Name, env.Image, toolLine, env.Name)

	// Lazy environments only pre-create shims for tools that declare a main
	// program. Add a fallback so ANY command builds the package that provides
	// it on first use, even multi-binary packages (gnuradio -> gnuradio-companion)
	// or ones with no declared main program (urh).
	if env.Lazy {
		content += lazyHandler(env)
	}

	if err := os.WriteFile(rc, []byte(content), 0o644); err != nil {
		return "", err
	}
	return rc, nil
}

// lazyHandler generates a bash command_not_found_handle that, for a command not
// on PATH, builds the environment's package that ships it and runs it. Packages
// whose name relates to the command are tried first so it stays close to
// on-demand rather than building everything.
func lazyHandler(env *Environment) string {
	quoted := make([]string, 0, len(env.Packages))
	for _, p := range env.Packages {
		quoted = append(quoted, fmt.Sprintf("%q", p))
	}
	attrs := strings.Join(quoted, " ")
	prereqStep := ""
	if len(env.Prerequisites) > 0 {
		prereqStep = fmt.Sprintf("  \"$__rfx_nix\" --extra-experimental-features \"nix-command flakes\" build --out-link %q %q || return $?\n", prerequisitesLink(env.Name), fmt.Sprintf("%s#%s-prerequisites", env.FlakeRef, env.Image))
	}
	return fmt.Sprintf(`
__rfx_flake=%q
__rfx_nix=%q
__rfx_attrs=(%s)
command_not_found_handle() {
  local cmd="$1"; shift
  local a leaf out ordered=()
%s
  # Try packages whose name relates to the command first.
  for a in "${__rfx_attrs[@]}"; do
    leaf="${a##*.}"
    if [[ "$leaf" == *"$cmd"* || "$cmd" == *"$leaf"* ]]; then ordered+=("$a"); fi
  done
  ordered+=("${__rfx_attrs[@]}")
  echo "rfswift: '$cmd' not built yet; building the tool that provides it..." >&2
  for a in "${ordered[@]}"; do
    out=$("$__rfx_nix" --extra-experimental-features "nix-command flakes" build --no-link --print-out-paths "$__rfx_flake#$a" 2>/dev/null) || continue
    if [ -n "$out" ] && [ -x "$out/bin/$cmd" ]; then
      export PATH="$out/bin:$PATH"
      "$out/bin/$cmd" "$@"; return $?
    fi
  done
  echo "rfswift: '$cmd' is not provided by environment '$RFSWIFT_NIX_ENV'." >&2
  return 127
}
`, env.FlakeRef, NixBinary(), attrs, prereqStep)
}

// userShell picks the interactive shell to launch.
func userShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	return "sh"
}

// resolveWorkspace mirrors the container workspace semantics: "" = default
// (~/rfswift-workspace/<name>), "none" = no workspace, else the given path.
func resolveWorkspace(name, cfg string) string {
	switch cfg {
	case "none":
		return ""
	case "":
		return filepath.Join(homeDir(), "rfswift-workspace", name)
	default:
		abs, err := filepath.Abs(cfg)
		if err != nil {
			return cfg
		}
		return abs
	}
}

func writeManifest(env *Environment) error {
	if err := ensureDir(EnvDir(env.Name)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(env.Name), data, 0o644)
}

func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// withEnv returns a copy of env with the given keys upserted.
func withEnv(env []string, kv map[string]string) []string {
	out := make([]string, 0, len(env)+len(kv))
	seen := map[string]bool{}
	for _, e := range env {
		key := e
		if i := strings.IndexByte(e, '='); i >= 0 {
			key = e[:i]
		}
		if v, ok := kv[key]; ok {
			out = append(out, key+"="+v)
			seen[key] = true
		} else {
			out = append(out, e)
		}
	}
	for k, v := range kv {
		if !seen[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}
