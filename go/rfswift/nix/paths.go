/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - on-disk layout for persisted environments.
 */

package nix

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// homeDir returns the invoking user's home directory, honoring SUDO_USER so a
// sudo-elevated invocation still targets the real user's ~/.rfswift tree
// (consistent with common.ConfigFileByPlatform).
func homeDir() string {
	home := os.Getenv("HOME")
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil && u.HomeDir != "" {
			return u.HomeDir
		}
	}
	if home != "" {
		return home
	}
	if u, err := user.Current(); err == nil {
		return u.HomeDir
	}
	return "."
}

// BaseDir is the root of RF Swift's Nix state: ~/.rfswift/nix.
// Override with RFSWIFT_NIX_HOME.
func BaseDir() string {
	if v := os.Getenv("RFSWIFT_NIX_HOME"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), ".rfswift", "nix")
}

// EnvironmentsDir holds one subdirectory per created environment.
func EnvironmentsDir() string {
	return filepath.Join(BaseDir(), "environments")
}

// EnvDir returns the directory for a single named environment.
func EnvDir(name string) string {
	return filepath.Join(EnvironmentsDir(), name)
}

// manifestPath is where an environment's metadata is stored.
func manifestPath(name string) string {
	return filepath.Join(EnvDir(name), "manifest.json")
}

// profileLink is the gcroot symlink to the realised closure for an environment.
func profileLink(name string) string {
	return filepath.Join(EnvDir(name), "profile")
}

// prerequisitesLink is the gcroot symlink to the realised device/library
// layer of an environment (its udev rules are read from there).
func prerequisitesLink(name string) string {
	return filepath.Join(EnvDir(name), "prerequisites")
}

func generationsDir(name string) string {
	return filepath.Join(EnvDir(name), "generations")
}

// shimsDir holds the build-on-first-call wrapper scripts for a lazy environment.
func shimsDir(name string) string {
	return filepath.Join(EnvDir(name), "bin")
}

// EnvExtrasProfile is the per-environment Nix profile holding packages the user
// added with `rfswift nix install --env <name>`, kept separate from the
// flake-defined closure so it survives env rebuilds. Its bin is added to PATH
// when entering the environment.
func EnvExtrasProfile(name string) string {
	return filepath.Join(EnvDir(name), "extras")
}

// SharedExtrasProfile is the Nix profile for packages installed with
// `rfswift nix install <pkg>` (no --env): available across environments.
func SharedExtrasProfile() string {
	return filepath.Join(BaseDir(), "extras")
}

// ensureDir creates a directory (and parents) if missing.
func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// equalFold is a tiny case-insensitive comparison used across the package
// without pulling strings into every file.
func equalFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
