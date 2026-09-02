/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - store maintenance (garbage collection / disk reclaim).
*
*  Realising environments accumulates store paths fast (each source build, each
*  intermediate closure). This drives `nix store gc` so users can reclaim disk
*  space without dropping to the raw nix CLI. Created environments each keep a
*  `profile` gcroot (see paths.go / buildProfile), and on-demand environments
*  keep one out-link per tool they have built (tools/<attr>, see pin.go), so
*  garbage collection never deletes a built environment or a built on-demand
*  tool that still exists: only build leftovers and paths no environment
*  references any more are removed. To reclaim a built environment's closure
*  too, remove it first with `rfswift nix remove`.
 */

package nix

import (
	"fmt"
	"strconv"
)

// GCOptions controls a store garbage collection pass.
type GCOptions struct {
	// DryRun lists what would be freed and deletes nothing.
	DryRun bool
	// MaxFree stops collection once this many bytes have been freed (0 = no
	// limit). Useful to cap how long a collection runs on a huge store.
	MaxFree int64
}

// GarbageCollect runs `nix store gc`, deleting every store path not reachable
// from a gcroot. Output (including nix's "N store paths deleted, M freed"
// summary) is streamed straight to the terminal.
func GarbageCollect(opts GCOptions) error {
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH")
	}
	args := append(experimentalArgs(), "store", "gc")
	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.MaxFree > 0 {
		args = append(args, "--max", strconv.FormatInt(opts.MaxFree, 10))
	}
	// runInteractive attaches the console when there is one and captures the
	// output otherwise: the Workbench on Windows has no console, and handing
	// its invalid standard handles to wsl.exe fails ("The handle is invalid").
	return runInteractive(nixCommand(args...))
}
