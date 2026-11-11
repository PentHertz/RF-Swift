/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package rfutils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// installMethod is how the running rfswift got onto this machine. It decides
// how `rfswift update` upgrades it: a binary owned by a native package must go
// through that package (overwriting it in place would desync the package
// database, and the next package upgrade would silently put the packaged
// version back); a standalone copy is replaced in place.
type installMethod int

const (
	installStandalone   installMethod = iota // tarball (/usr/local/bin, ~/.rfswift/bin) or `go build`
	installDeb                               // dpkg owns it
	installRPM                               // rpm owns it
	installPacman                            // pacman owns it
	installHomebrew                          // the rfswift cask (macOS)
	installWindowsSetup                      // the MSI / setup.exe, under %ProgramFiles%\RF Swift
)

// String names the install method for the update summary.
func (m installMethod) String() string {
	switch m {
	case installDeb:
		return "Debian package"
	case installRPM:
		return "RPM package"
	case installPacman:
		return "Arch package"
	case installHomebrew:
		return "Homebrew cask"
	case installWindowsSetup:
		return "RF Swift Windows installer"
	default:
		return "standalone binary"
	}
}

// ownedByPackage reports whether the package manager behind tool claims path.
// A variable so tests can stand in for dpkg/rpm/pacman.
var ownedByPackage = func(tool string, args ...string) bool {
	if _, err := exec.LookPath(tool); err != nil {
		return false
	}
	return exec.Command(tool, args...).Run() == nil
}

// detectInstallMethod works out how the binary at exe (symlinks already
// resolved) was installed.
//
//	in(1): string exe     resolved path of the running executable
//	in(2): string goos    runtime.GOOS (a parameter for tests)
//	out: installMethod
func detectInstallMethod(exe, goos string) installMethod {
	switch goos {
	case "linux":
		switch {
		case ownedByPackage("dpkg", "-S", exe):
			return installDeb
		case ownedByPackage("rpm", "-qf", exe):
			return installRPM
		case ownedByPackage("pacman", "-Qo", exe):
			return installPacman
		}
	case "darwin":
		if strings.Contains(exe, "/Caskroom/") || strings.Contains(exe, "/Cellar/") {
			return installHomebrew
		}
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramW6432"} {
			pf := os.Getenv(env)
			if pf != "" && strings.EqualFold(filepath.Dir(exe), filepath.Join(pf, "RF Swift")) {
				return installWindowsSetup
			}
		}
	}
	return installStandalone
}

// packageAssetName is the release file of the native package for version
// (no leading "v") on goarch, named the way the release publishes it
// (nfpm's conventional names, the same ones get_rfswift.sh downloads):
// rfswift_4.0.5_amd64.deb, rfswift-4.0.5-1.x86_64.rpm,
// rfswift-4.0.5-1-x86_64.pkg.tar.zst. False when the release has no such
// package (Arch on riscv64, or an architecture RF Swift does not build).
func packageAssetName(m installMethod, version, goarch string) (string, bool) {
	var arches map[string]string
	var format string
	switch m {
	case installDeb:
		arches = map[string]string{"amd64": "amd64", "arm64": "arm64", "riscv64": "riscv64"}
		format = "rfswift_%s_%s.deb"
	case installRPM:
		arches = map[string]string{"amd64": "x86_64", "arm64": "aarch64", "riscv64": "riscv64"}
		format = "rfswift-%s-1.%s.rpm"
	case installPacman:
		arches = map[string]string{"amd64": "x86_64", "arm64": "aarch64"}
		format = "rfswift-%s-1-%s.pkg.tar.zst"
	default:
		return "", false
	}
	arch, ok := arches[goarch]
	if !ok {
		return "", false
	}
	return fmt.Sprintf(format, version, arch), true
}

// archiveAssetName is the release archive holding the standalone binary for
// goos/goarch (goreleaser's uname-style names). False for a platform RF Swift
// does not build.
func archiveAssetName(goos, goarch string) (string, bool) {
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64", "riscv64": "riscv64"}[goarch]
	switch {
	case arch == "":
		return "", false
	case goos == "linux":
		return "rfswift_Linux_" + arch + ".tar.gz", true
	case goos == "darwin" && goarch != "riscv64":
		return "rfswift_Darwin_" + arch + ".tar.gz", true
	case goos == "windows" && goarch != "riscv64":
		return "rfswift_Windows_" + arch + ".zip", true
	}
	return "", false
}

// windowsSetupAssetName is the Windows setup program for version on goarch.
func windowsSetupAssetName(version, goarch string) (string, bool) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if arch == "" {
		return "", false
	}
	return fmt.Sprintf("RFSwift-Setup-%s-%s.exe", version, arch), true
}

// packageInstallArgs is the package-manager command installing the local
// package file pkg (without sudo). It prefers the front end that resolves
// dependencies (apt-get, dnf, yum, zypper) and falls back to the low-level
// tool. The packages are not GPG-signed; their SHA-256 is checked against the
// release before this runs, as get_rfswift.sh does.
//
//	in(1): installMethod m  installDeb, installRPM or installPacman
//	in(2): string pkg       absolute path of the downloaded package
//	in(3): func lookPath    exec.LookPath (a parameter for tests)
//	out: []string           argv, nil for a method that has no package manager
func packageInstallArgs(m installMethod, pkg string, lookPath func(string) (string, error)) []string {
	has := func(tool string) bool { _, err := lookPath(tool); return err == nil }
	switch m {
	case installDeb:
		if has("apt-get") {
			return []string{"apt-get", "install", "-y", pkg}
		}
		return []string{"dpkg", "-i", pkg}
	case installRPM:
		switch {
		case has("dnf"):
			return []string{"dnf", "install", "-y", pkg}
		case has("yum"):
			return []string{"yum", "install", "-y", pkg}
		case has("zypper"):
			return []string{"zypper", "--non-interactive", "install", "--allow-unsigned-rpm", pkg}
		}
		return []string{"rpm", "-U", pkg}
	case installPacman:
		return []string{"pacman", "-U", "--noconfirm", pkg}
	}
	return nil
}

// dirWritable reports whether this process can create files in dir, which is
// what replacing a binary in it takes (a new file renamed over the old one).
// It probes by creating and removing a temporary file, which also answers
// correctly on Windows and on read-only mounts.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".rfswift-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// isRoot reports whether the process runs as root (never on Windows).
func isRoot() bool {
	return runtime.GOOS != "windows" && os.Geteuid() == 0
}
