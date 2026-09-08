/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - search the tool universe and install packages, including ones
*  that do not belong to any environment.
*
*  `rfswift nix run <target> <tool>` already runs any package on demand (the
*  flake's legacyPackages fall back to the whole nixpkgs set). This adds:
*    - SearchPackages: find a tool across the curated RF Swift package universe.
*    - InstallPackages: install a package persistently into a Nix profile, either
*      shared across environments or scoped to one, so it stays on PATH.
 */

package nix

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	common "penthertz/rfswift/common"
)

// tailWriter keeps only the last max bytes written to it, so we can capture a
// command's stderr for error reporting without buffering a whole build log.
type tailWriter struct {
	buf []byte
	max int
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.max {
		w.buf = w.buf[len(w.buf)-w.max:]
	}
	return len(p), nil
}

func (w *tailWriter) String() string { return string(w.buf) }

// InstallProgress reports truthful coarse phases. Nix does not expose a stable
// total byte/build count on its CLI stream, so callers should render phases
// below 100 as indeterminate rather than inventing a time estimate.
type InstallProgress func(percent int, stage string)

type nixInstallProgressWriter struct {
	progress InstallProgress
	last     string
}

func (w *nixInstallProgressWriter) Write(p []byte) (int, error) {
	lower := strings.ToLower(string(p))
	stage, percent := "", 0
	switch {
	case strings.Contains(lower, "copying path") || strings.Contains(lower, "downloading"):
		stage, percent = "Downloading package dependencies", 30
	case strings.Contains(lower, "building '") || strings.Contains(lower, "building path"):
		stage, percent = "Building package dependencies", 60
	case strings.Contains(lower, "installing '"):
		stage, percent = "Updating the mission profile", 85
	}
	if stage != "" && stage != w.last && w.progress != nil {
		w.last = stage
		w.progress(percent, stage)
	}
	return len(p), nil
}

// installFailureReason turns nix's stderr into a concise, user-facing reason.
// The GUI only sees the returned error, so a bare "exit status 1" is useless;
// this recognises the common "not available on this platform" case (Linux-only
// SDR drivers on macOS) and otherwise surfaces nix's own final error line.
func installFailureReason(pkg, sys, stderr string, err error) string {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "not available on the requested hostplatform"):
		return fmt.Sprintf("%q (or one of its dependencies) is not available on %s (likely Linux-only). "+
			"Hardware that needs a Linux driver works through the Lima engine instead.", pkg, sys)
	case strings.Contains(lower, "does not provide attribute") || strings.Contains(lower, "no attribute"):
		return fmt.Sprintf("no package named %q in the flake (check the name with: rfswift nix search %s)", pkg, pkg)
	}
	if line := lastNixErrorLine(stderr); line != "" {
		return line
	}
	return err.Error()
}

// lastNixErrorLine returns the last "error:"-prefixed line from nix stderr.
func lastNixErrorLine(stderr string) string {
	var last string
	for _, ln := range strings.Split(stderr, "\n") {
		if t := strings.TrimSpace(ln); strings.HasPrefix(t, "error:") {
			last = t
		}
	}
	return last
}

var cachedSystem string

// currentSystem returns the Nix system double (e.g. x86_64-linux), cached.
func currentSystem() string {
	if cachedSystem != "" {
		return cachedSystem
	}
	out, err := nixCommand(append(experimentalArgs(), "eval", "--raw", "--impure", "--expr", "builtins.currentSystem")...).Output()
	if err == nil && len(out) > 0 {
		cachedSystem = strings.TrimSpace(string(out))
	} else {
		cachedSystem = "x86_64-linux"
	}
	return cachedSystem
}

// PkgHit is a search result: a package and the environments that bundle it.
type PkgHit struct {
	Name string   `json:"name"`
	Envs []string `json:"envs"`
}

// PackageUniverse is the curated set of packages RF Swift environments bundle
// (the union across the catalog), sorted and de-duplicated. It is the meaningful
// search space; the full nixpkgs set is reachable too via --nixpkgs on the CLI.
func PackageUniverse() ([]string, map[string][]string) {
	cat, err := LoadCatalog()
	if err != nil {
		return nil, nil
	}
	inEnvs := map[string][]string{}
	for _, e := range cat.Environments {
		for _, p := range e.Packages {
			inEnvs[p] = append(inEnvs[p], e.Name)
		}
	}
	names := make([]string, 0, len(inEnvs))
	for p := range inEnvs {
		names = append(names, p)
	}
	sort.Strings(names)
	return names, inEnvs
}

// SearchPackages returns curated packages whose name matches term (case-
// insensitive substring), with the environments that include each.
func SearchPackages(term string) []PkgHit {
	names, inEnvs := PackageUniverse()
	t := strings.ToLower(strings.TrimSpace(term))
	var hits []PkgHit
	for _, n := range names {
		if t == "" || strings.Contains(strings.ToLower(n), t) {
			hits = append(hits, PkgHit{Name: n, Envs: inEnvs[n]})
		}
	}
	return hits
}

// SearchNixpkgs runs `nix search` over the flake's nixpkgs for a term, returning
// the raw results as attribute-path -> one-line description.
func SearchNixpkgs(flakeRef, term string) (map[string]string, error) {
	// Search the flake's pinned nixpkgs via --inputs-from so results match what
	// would actually install.
	args := append(experimentalArgs(), "search", "--json", "--inputs-from", flakeRef, "nixpkgs", term)
	out, err := nixCommand(args...).Output()
	if err != nil {
		return nil, err
	}
	// `nix search --json` returns { "legacyPackages.<sys>.<attr>": {pname,version,description} }
	var raw map[string]struct {
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if e := json.Unmarshal(out, &raw); e != nil {
		return nil, e
	}
	res := map[string]string{}
	for k, v := range raw {
		// strip the "legacyPackages.<sys>." prefix
		attr := k
		if i := strings.Index(k, "."); i >= 0 {
			parts := strings.SplitN(k, ".", 3)
			if len(parts) == 3 {
				attr = parts[2]
			}
		}
		desc := v.Description
		if v.Version != "" {
			desc = v.Version + " - " + desc
		}
		res[attr] = desc
	}
	return res, nil
}

// InstallPackages installs one or more packages into a persistent Nix profile.
// With envName set, they go to that environment's extras profile (on PATH when
// entering it); otherwise to the shared extras profile (on PATH in every
// environment). Packages resolve against the flake's legacyPackages, so any
// nixpkgs package works, not only those in an environment.
func InstallPackages(flakeRef string, pkgs []string, envName string) error {
	return InstallPackagesWithProgress(flakeRef, pkgs, envName, nil)
}

// InstallPackagesWithProgress installs packages and reports observable Nix
// phases for GUI clients while retaining the live stderr stream for the CLI.
func InstallPackagesWithProgress(flakeRef string, pkgs []string, envName string, progress InstallProgress) error {
	if useWSL() {
		return wslInstallPackages(flakeRef, pkgs, envName, progress)
	}
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH")
	}
	profile := SharedExtrasProfile()
	scope := "shared (available in every environment)"
	if envName != "" {
		if _, err := GetEnvironment(envName); err != nil {
			return err
		}
		profile = EnvExtrasProfile(envName)
		scope = fmt.Sprintf("environment '%s'", envName)
	}
	if err := ensureDir(filepath.Dir(profile)); err != nil {
		return err
	}
	sys := currentSystem()
	if progress != nil {
		progress(5, "Resolving package selection")
	}
	for _, p := range pkgs {
		installable := fmt.Sprintf("%s#legacyPackages.%s.%s", flakeRef, sys, p)
		common.PrintInfoMessage(fmt.Sprintf("Installing %s into %s ...", p, scope))
		args := append(experimentalArgs(), "profile", "install", "--profile", profile, installable)
		cmd := nixCommand(args...)
		// Tee stderr: stream it live (the CLI shows nix's progress) while
		// capturing the tail, so the returned error carries nix's real reason
		// instead of a bare "exit status 1" — all the GUI would otherwise see.
		captured := &tailWriter{max: 64 << 10}
		cmd.Stdin, cmd.Stdout = os.Stdin, os.Stdout
		progressWriter := &nixInstallProgressWriter{progress: progress}
		cmd.Stderr = io.MultiWriter(os.Stderr, captured, progressWriter)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install %s: %s", p, installFailureReason(p, sys, captured.String(), err))
		}
	}
	if progress != nil {
		progress(100, "Package installed")
	}
	common.PrintSuccessMessage(fmt.Sprintf("Installed %d package(s) into %s.", len(pkgs), scope))
	if envName != "" {
		common.PrintInfoMessage(fmt.Sprintf("They are on PATH when you enter '%s' (rfswift exec --engine nix -c %s).", envName, envName))
	} else {
		common.PrintInfoMessage(fmt.Sprintf("They are on PATH in every RF Swift nix environment. Profile: %s", profile))
	}
	return nil
}
