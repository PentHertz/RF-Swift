/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - pinning on-demand environments to a flake revision.
*
*  An eager environment is pinned by construction: its profile is one store
*  path, built once. An on-demand (lazy) environment used to keep the flake
*  reference it was created from - typically `github:PentHertz/RF-Swift-nix`,
*  a moving branch - and every shim re-resolved it on each call: nix asked
*  GitHub for the tip once per tarball-ttl and, whenever the branch had moved,
*  silently rebuilt the tool (SDR++ compiles for minutes) before running it,
*  and `nix run` results were never gcroots, so a store GC took them away too.
*
*  A lazy environment now records the locked revision at creation (FlakeRef)
*  and the reference it came from (FlakeOrigin); its shims build with
*  `--out-link` under <env>/tools/<attr>, which registers a gcroot, and run
*  the link directly. `rfswift env update <name>` moves the pin to the
*  origin's current tip and rebuilds the tools already built. A local
*  checkout is never pinned: developers want their working tree.
 */

package nix

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	common "penthertz/rfswift/common"
)

// flakeMetadata is the part of `nix flake metadata --json` needed to pin.
type flakeMetadata struct {
	URL      string `json:"url"`      // the locked URL, e.g. github:owner/repo/<rev>
	Revision string `json:"revision"` // the locked revision when the fetcher has one
	Locked   struct {
		Type string `json:"type"`
		Rev  string `json:"rev"`
	} `json:"locked"`
}

// parseLockedFlakeRef extracts the locked flake URL from metadata JSON. ok is
// false when the metadata carries no revision (a path flake, a tarball
// without one): pinning such a reference means nothing.
func parseLockedFlakeRef(data []byte) (locked string, ok bool, err error) {
	var m flakeMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return "", false, fmt.Errorf("parse flake metadata: %w", err)
	}
	rev := m.Revision
	if rev == "" {
		rev = m.Locked.Rev
	}
	if m.URL == "" || rev == "" || m.Locked.Type == "path" {
		return "", false, nil
	}
	return m.URL, true, nil
}

// lockedFlakeRef asks nix for the locked form of flakeRef. refresh bypasses
// nix's tarball-ttl so a branch reference resolves to its current tip (what
// `env update` wants); without it nix answers from its cache while fresh. A
// local checkout is returned as is, not pinned.
func lockedFlakeRef(flakeRef string, refresh bool) (locked string, ok bool, err error) {
	if _, local := localFlakePath(flakeRef); local {
		return flakeRef, false, nil
	}
	args := append(experimentalArgs(), "flake", "metadata", "--json")
	if refresh {
		args = append(args, "--refresh")
	}
	args = append(args, flakeRef)
	cmd := nixCommand(args...)
	cmd.Stderr = consoleWriter(os.Stderr)
	out, err := cmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("nix flake metadata %s: %w", flakeRef, err)
	}
	return parseLockedFlakeRef(out)
}

// pinLazyFlake resolves the reference an on-demand environment is created
// from to its locked revision. origin is "" when nothing was pinned: a local
// checkout, a reference that already names a revision, or a lookup failure
// (reported; the environment then follows the reference as before).
func pinLazyFlake(flakeRef string) (pinned, origin string) {
	locked, ok, err := lockedFlakeRef(flakeRef, false)
	if err != nil {
		common.PrintWarningMessage(fmt.Sprintf("Could not pin %s to a revision (%v); the environment follows it unpinned.", flakeRef, err))
		return flakeRef, ""
	}
	if !ok || locked == flakeRef {
		return flakeRef, ""
	}
	return locked, flakeRef
}

var fullRevRe = regexp.MustCompile(`([0-9a-f]{40})$`)

// shortRev abbreviates the revision at the end of a locked flake URL for
// messages (github:owner/repo/ff12ceb11c7d... -> github:owner/repo/ff12ceb11c7d).
func shortRev(ref string) string {
	if m := fullRevRe.FindStringIndex(ref); m != nil {
		return ref[:m[0]] + ref[m[0]:m[0]+12]
	}
	return ref
}

// lazyPinReport compares an on-demand environment's pin with the current tip
// of the reference it came from, for `env update --check`.
func lazyPinReport(env *Environment) (string, error) {
	current, ok, err := lockedFlakeRef(env.FlakeOrigin, true)
	if err != nil {
		return "", err
	}
	if !ok {
		return fmt.Sprintf("%s resolves to no revision; there is no pin to compare.", env.FlakeOrigin), nil
	}
	if current == env.FlakeRef {
		return fmt.Sprintf("Pinned at %s, still the tip of %s: nothing would change.", shortRev(env.FlakeRef), env.FlakeOrigin), nil
	}
	return fmt.Sprintf("Pinned at %s; %s now points at %s. 'rfswift env update %s' moves the pin and rebuilds the tools already built.", shortRev(env.FlakeRef), env.FlakeOrigin, shortRev(current), env.Name), nil
}

// linkedTools lists the attributes an on-demand environment has pinned under
// tools/: the tools that were built at least once.
func linkedTools(name string) []string {
	entries, err := os.ReadDir(hostPath(toolsDir(name)))
	if err != nil {
		return nil
	}
	var attrs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		attrs = append(attrs, e.Name())
	}
	sort.Strings(attrs)
	return attrs
}

// rebuildLinkedTools re-realises every tool an on-demand environment has
// pinned, against its current FlakeRef, so the next call runs what the pin
// now designates instead of what was built under the previous one. A tool
// that no longer builds loses its link (the shim rebuilds it, and reports
// the error, on its next call) and is named in the returned error.
func rebuildLinkedTools(env *Environment) error {
	attrs := linkedTools(env.Name)
	if len(attrs) == 0 {
		return nil
	}
	common.PrintInfoMessage(fmt.Sprintf("Rebuilding %d pinned tool(s) of '%s' against %s ...", len(attrs), env.Name, shortRev(env.FlakeRef)))
	var failed []string
	for _, attr := range attrs {
		link := toolLink(env.Name, attr)
		args := append(experimentalArgs(), "build", "--out-link", link, fmt.Sprintf("%s#%s", env.FlakeRef, attr))
		if err := runInteractive(nixCommand(args...)); err != nil {
			_ = os.Remove(hostPath(link))
			failed = append(failed, attr)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("could not rebuild %s against %s; each is rebuilt again on its next call", strings.Join(failed, ", "), env.FlakeRef)
	}
	return nil
}
