/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// packageManagedHint reports how to upgrade when the running binary belongs
// to a native package: a .deb/.rpm/pacman package on Linux, the Homebrew cask
// on macOS. `rfswift update` overwriting such a binary in place would desync
// the package database, and the next package upgrade would silently put the
// packaged version back. Empty when the binary is a plain copy (tarball
// install in /usr/local/bin or ~/.rfswift/bin, `go build`).
//
//	in(1): string exe path of the running executable
//	out: string upgrade instructions, "" when not package-managed
func packageManagedHint(exe string) string {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	switch runtime.GOOS {
	case "linux":
		type owner struct {
			tool    string
			args    []string
			upgrade string
		}
		for _, o := range []owner{
			{"dpkg", []string{"-S", exe}, "sudo apt install ./rfswift_<version>_<arch>.deb"},
			{"rpm", []string{"-qf", exe}, "sudo dnf install ./rfswift-<version>-1.<arch>.rpm"},
			{"pacman", []string{"-Qo", exe}, "sudo pacman -U rfswift-<version>-1-<arch>.pkg.tar.zst"},
		} {
			if _, err := exec.LookPath(o.tool); err != nil {
				continue
			}
			if err := exec.Command(o.tool, o.args...).Run(); err != nil {
				continue // not owned by this package manager
			}
			return fmt.Sprintf("%s is installed by a native package, so 'rfswift update' leaves it alone.\n"+
				"Upgrade with the new package from https://github.com/PentHertz/RF-Swift/releases (%s),\n"+
				"or re-run the installer, which fetches it and verifies checksums and attestations:\n"+
				"  curl -fsSL https://raw.githubusercontent.com/PentHertz/RF-Swift/refs/heads/main/scripts/get_rfswift.sh | sh", exe, o.upgrade)
		}
	case "darwin":
		if strings.Contains(exe, "/Caskroom/") || strings.Contains(exe, "/Cellar/") {
			return fmt.Sprintf("%s comes from Homebrew, so 'rfswift update' leaves it alone. Upgrade with: brew upgrade --cask rfswift", exe)
		}
	}
	return ""
}
