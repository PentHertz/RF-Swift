/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - types shared across the nix package.
*
*  Unlike the Docker/Podman/Lima backends (which implement the container-centric
*  dock.ContainerEngine interface), the Nix engine is a parallel execution path:
*  it materialises RF Swift tool sets into dedicated *native* environments using
*  the Nix package manager instead of running containers.
 */

package nix

import "time"

// CatalogEntry describes one available RF Swift environment ("image") in the
// Nix world: a named set of nixpkgs packages that reproduces the tools an
// equivalent Docker image installs.
type CatalogEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Packages    []string `json:"packages"`
	// Prerequisites is the runtime device/library layer realised before apps.
	Prerequisites []string `json:"prerequisites,omitempty"`
	// Missing lists tools that have no nixpkgs equivalent yet; documented for
	// transparency rather than silently dropped.
	Missing []string `json:"missing,omitempty"`
}

// Catalog is the machine-readable index of available environments, mirrored
// from the RF-Swift-nix repository's environments.nix. The binary embeds a
// snapshot as a fallback and prefers a local/remote copy when available.
type Catalog struct {
	Version      string         `json:"version"`
	Flake        string         `json:"flake"`
	Environments []CatalogEntry `json:"environments"`
}

// Find returns the catalog entry with the given name (case-insensitive) or nil.
func (c *Catalog) Find(name string) *CatalogEntry {
	for i := range c.Environments {
		if equalFold(c.Environments[i].Name, name) {
			return &c.Environments[i]
		}
	}
	return nil
}

// Names returns the environment names in catalog order.
func (c *Catalog) Names() []string {
	out := make([]string, 0, len(c.Environments))
	for i := range c.Environments {
		out = append(out, c.Environments[i].Name)
	}
	return out
}

// Environment is a created, persisted named environment on disk. It is the Nix
// analogue of an RF Swift container: something you create once, re-enter, and
// remove.
type Environment struct {
	Name            string    `json:"name"`
	Image           string    `json:"image"`                   // catalog entry this env was built from
	FlakeRef        string    `json:"flakeRef"`                // resolved flake reference used to realise it
	Packages        []string  `json:"packages"`                // resolved package list (snapshot at creation)
	Prerequisites   []string  `json:"prerequisites,omitempty"` // runtime device/library layer
	Workspace       string    `json:"workspace"`               // host path mounted/linked as the working dir
	Command         string    `json:"command"`                 // default command (empty = interactive shell)
	Created         time.Time `json:"created"`
	Updated         time.Time `json:"updated,omitempty"`
	LastUpdateInput string    `json:"lastUpdateInput,omitempty"`
	// ProfilePath is the on-disk gcroot symlink to the realised buildEnv closure
	// (.../environments/<name>/profile). Empty until the environment is realised,
	// and always empty for lazy environments.
	ProfilePath string `json:"profilePath,omitempty"`
	// Lazy marks an on-demand environment: tools are not prebuilt. Each is a
	// shim in .../environments/<name>/bin that builds and runs it on first call.
	Lazy bool `json:"lazy,omitempty"`
	// Commands maps a runnable command name to the flake attribute that provides
	// it (for lazy environments). Populated at creation from each package's
	// meta.mainProgram.
	Commands map[string]string `json:"commands,omitempty"`
	// Isolate records that this environment should be entered inside a
	// bubblewrap jail (Linux). Set at creation from --isolate / the TUI / GUI,
	// and honoured on every subsequent entry (run, exec, GUI terminal).
	Isolate bool `json:"isolate,omitempty"`
}

// Generation is a previous, GC-rooted eager environment closure available for
// rollback. StorePath remains valid while the generation symlink exists.
type Generation struct {
	Name      string    `json:"name"`
	StorePath string    `json:"storePath"`
	Created   time.Time `json:"created"`
}

// RunOptions carries the parameters for creating and entering an environment.
type RunOptions struct {
	Name       string
	Image      string
	Command    string
	Workspace  string // "" = default (~/rfswift-workspace/<name>), "none" = disabled, else path
	FlakeRef   string // override; empty = resolve from catalog/env/defaults
	Rebuild    bool   // force re-realisation even if a gcroot already exists
	Pure       bool   // enter a pure environment (nix develop --ignore-environment)
	Lazy       bool   // on-demand: build each tool the first time it is called, not all up front
	CreateOnly bool   // create/realise and persist without entering an interactive shell
	Isolate    bool   // enter inside a bubblewrap jail (Linux): hide $HOME/host FS and give
	// the shell private PID/IPC/tmp, while keeping USB/serial devices, sysfs/udev,
	// the display and the network. No effect (errors) on macOS.
	// PreEnter runs once the environment is realised, right before the shell
	// is entered (not for CreateOnly). The CLI uses it to offer the host-side
	// setup that needs a terminal, such as installing udev rules.
	PreEnter func(env *Environment)
}
