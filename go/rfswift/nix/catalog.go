/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - the environment catalog.
*
*  The binary ships an embedded snapshot of the catalog so `rfswift nix catalog`
*  and the wizard work with zero network access. When a local checkout of the
*  RF-Swift-nix repository or a cached copy is present, it is preferred so users
*  get the freshest environment definitions without upgrading the binary.
 */

package nix

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed catalog.json
var embeddedCatalog []byte

// DefaultFlakeRef is the canonical published flake for RF Swift Nix
// environments. It can be overridden per-run (--flake) or globally
// (RFSWIFT_NIX_FLAKE).
const DefaultFlakeRef = "github:PentHertz/RF-Swift-nix"

// catalogSearchPaths returns candidate catalog.json locations in priority order.
func catalogSearchPaths() []string {
	var paths []string
	if v := os.Getenv("RFSWIFT_NIX_CATALOG"); v != "" {
		paths = append(paths, v)
	}
	// A local RF-Swift-nix checkout next to the working dir or under the state dir.
	for _, root := range localFlakeRoots() {
		paths = append(paths, filepath.Join(root, "catalog.json"))
	}
	// A cached copy the user may have refreshed. Not on Windows: the state
	// directory is inside the WSL distribution and looking there would start
	// it for every catalog lookup; the embedded snapshot serves the front end.
	if !useWSL() {
		paths = append(paths, filepath.Join(BaseDir(), "catalog.json"))
	}
	return paths
}

// LoadCatalog loads the environment catalog, preferring on-disk copies and
// falling back to the embedded snapshot. It never returns nil on success.
func LoadCatalog() (*Catalog, error) {
	for _, p := range catalogSearchPaths() {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		cat, err := parseCatalog(data)
		if err != nil {
			// A malformed local file should not silently mask the embedded one,
			// but it is worth surfacing.
			return nil, fmt.Errorf("failed to parse catalog %s: %w", p, err)
		}
		return cat, nil
	}
	return parseCatalog(embeddedCatalog)
}

func parseCatalog(data []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Flake == "" {
		c.Flake = DefaultFlakeRef
	}
	return &c, nil
}
