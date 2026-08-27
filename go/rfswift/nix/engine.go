/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - availability, flake resolution and selection state.
*
*  The Nix engine is selected with `--engine nix` (or RFSWIFT_ENGINE=nix). It is
*  not a dock.ContainerEngine: it does not talk to a container daemon. Instead it
*  drives the `nix` CLI to realise RF Swift tool sets into native environments.
 */

package nix

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// RunTool builds (only if needed) and runs a single tool from the pinned package
// set, on demand. This is the "step by step" model: nothing is prebuilt, and
// only the named tool's closure is realised.
//
//	nix run <flakeRef>#<tool> -- <args...>
func RunTool(flakeRef, tool string, args []string) error {
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH")
	}
	full := append(experimentalArgs(), "run", fmt.Sprintf("%s#%s", flakeRef, tool))
	if len(args) > 0 {
		full = append(full, "--")
		full = append(full, args...)
	}
	cmd := exec.Command(NixBinary(), full...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// RunAudit runs the RF Swift security / vulnerability / supply-chain audit,
// exposed by the flake as the `audit` app. Arguments are passed straight through
// to scripts/security-audit.sh (e.g. --all, --env NAME, --pkg ATTR, --format,
// --fail-on, --out). The app resolves the flake source and writes reports to the
// caller's working directory, so this works against a local checkout or the
// published flake alike.
//
//	nix run <flakeRef>#audit -- <args...>
func RunAudit(flakeRef string, args []string) error {
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH")
	}
	full := append(experimentalArgs(), "run", fmt.Sprintf("%s#audit", flakeRef))
	if len(args) > 0 {
		full = append(full, "--")
		full = append(full, args...)
	}
	cmd := exec.Command(NixBinary(), full...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

var (
	selected   bool
	selectedMu sync.RWMutex
)

// SetSelected records whether the Nix engine is the active backend for this run.
// The CLI calls it from PersistentPreRun so command handlers can branch away
// from the container path.
func SetSelected(v bool) {
	selectedMu.Lock()
	defer selectedMu.Unlock()
	selected = v
}

// IsSelected reports whether the Nix engine is active.
func IsSelected() bool {
	selectedMu.RLock()
	defer selectedMu.RUnlock()
	return selected
}

// nixBinaryPath caches the resolved nix location for the process lifetime.
var nixBinaryPath string

// NixBinary is the nix executable; overridable via RFSWIFT_NIX_BIN. Nix is
// normally put on PATH by shell profile scripts, which GUI apps launched from
// Finder/launchd never source — fall back to the standard install locations
// so the Workbench finds it too.
func NixBinary() string {
	if v := os.Getenv("RFSWIFT_NIX_BIN"); v != "" {
		return v
	}
	if nixBinaryPath != "" {
		return nixBinaryPath
	}
	if p, err := exec.LookPath("nix"); err == nil {
		nixBinaryPath = p
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		"/nix/var/nix/profiles/default/bin/nix",
		filepath.Join(home, ".nix-profile", "bin", "nix"),
		"/run/current-system/sw/bin/nix",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			nixBinaryPath = p
			return p
		}
	}
	return "nix"
}

// IsAvailable reports whether a usable nix binary is on PATH.
func IsAvailable() bool {
	_, err := exec.LookPath(NixBinary())
	return err == nil
}

// Version returns the output of `nix --version`, or an error if nix is absent.
func Version() (string, error) {
	out, err := exec.Command(NixBinary(), "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// experimentalArgs enables the flakes + nix-command features on every call, so
// RF Swift works regardless of the user's nix.conf. These are prepended to the
// nix subcommand.
func experimentalArgs() []string {
	return []string{"--extra-experimental-features", "nix-command flakes"}
}

// localFlakeRoots returns candidate directories that may hold a local checkout
// of the RF-Swift-nix repository, in priority order. Used both to prefer a
// local catalog.json and to resolve a local flake reference for offline work.
func localFlakeRoots() []string {
	var roots []string
	add := func(p string) {
		if p == "" {
			return
		}
		if abs, err := filepath.Abs(p); err == nil {
			roots = append(roots, abs)
		}
	}

	// Explicit override that points at a directory.
	if v := os.Getenv("RFSWIFT_NIX_FLAKE"); v != "" && !looksLikeFlakeURL(v) {
		add(v)
	}
	// Working directory and its parent (matches the dev layout where
	// RF-Swift and RF-Swift-nix are siblings).
	if cwd, err := os.Getwd(); err == nil {
		add(filepath.Join(cwd, "RF-Swift-nix"))
		add(filepath.Join(filepath.Dir(cwd), "RF-Swift-nix"))
	}
	// Alongside the binary.
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		add(filepath.Join(dir, "RF-Swift-nix"))
		add(filepath.Join(filepath.Dir(dir), "RF-Swift-nix"))
	}
	// Under the RF Swift state directory.
	add(filepath.Join(BaseDir(), "RF-Swift-nix"))
	return roots
}

// looksLikeFlakeURL reports whether a string is a flake reference rather than a
// filesystem path (github:, git+, path:, a URL scheme, or a bare "owner/repo").
func looksLikeFlakeURL(s string) bool {
	for _, prefix := range []string{"github:", "gitlab:", "sourcehut:", "git+", "path:", "flake:", "http://", "https://", "tarball+"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	// A relative or absolute path is not a flake URL.
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") {
		return false
	}
	return false
}

// hasFlake reports whether dir contains a flake.nix.
func hasFlake(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "flake.nix"))
	return err == nil
}

// ResolveFlakeRef returns the flake reference to use for an environment, in
// priority order: explicit override, RFSWIFT_NIX_FLAKE, a local RF-Swift-nix
// checkout, then the published default. A local checkout is preferred so users
// hacking on environments get their changes without pushing.
func ResolveFlakeRef(override string) string {
	if override != "" {
		return override
	}
	if v := os.Getenv("RFSWIFT_NIX_FLAKE"); v != "" {
		return v
	}
	for _, root := range localFlakeRoots() {
		if hasFlake(root) {
			return root
		}
	}
	if cat, err := LoadCatalog(); err == nil && cat.Flake != "" {
		return cat.Flake
	}
	return DefaultFlakeRef
}
