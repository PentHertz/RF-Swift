/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Host setup - Linux distribution detection, for installing the container
*  engines with the distribution's own package manager.
 */

package hostsetup

import (
	"os"
	"os/exec"
	"strings"
)

// Distro is what /etc/os-release says about the host.
type Distro struct {
	ID             string   `json:"id"`             // ubuntu, fedora, arch, ...
	Name           string   `json:"name"`           // PRETTY_NAME or NAME
	Like           []string `json:"like"`           // ID_LIKE entries (debian, rhel fedora, arch, suse)
	PackageManager string   `json:"packageManager"` // apt | dnf | yum | pacman | zypper | apk | nix | ""
}

// osReleasePath is /etc/os-release, overridable in tests.
var osReleasePath = "/etc/os-release"

// DetectDistro reads /etc/os-release. An unreadable file yields an empty
// Distro whose PackageManager is "".
func DetectDistro() Distro {
	content, err := os.ReadFile(osReleasePath)
	if err != nil {
		return Distro{}
	}
	return parseOSRelease(string(content))
}

func parseOSRelease(content string) Distro {
	var d Distro
	name := ""
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "ID":
			d.ID = strings.ToLower(value)
		case "ID_LIKE":
			d.Like = strings.Fields(strings.ToLower(value))
		case "PRETTY_NAME":
			d.Name = value
		case "NAME":
			name = value
		}
	}
	if d.Name == "" {
		d.Name = name
	}
	d.PackageManager = packageManagerFor(d.ID, d.Like)
	return d
}

// packageManagerFor maps a distribution (and its family) to its package
// manager. yum is chosen over dnf only when dnf is absent at run time.
func packageManagerFor(id string, like []string) string {
	family := func(names ...string) bool {
		for _, n := range names {
			if id == n {
				return true
			}
			for _, l := range like {
				if l == n {
					return true
				}
			}
		}
		return false
	}
	switch {
	case family("nixos"):
		return "nix"
	case family("alpine"):
		return "apk"
	case family("arch", "archlinux", "manjaro"):
		return "pacman"
	case family("debian", "ubuntu"):
		return "apt"
	case family("fedora", "rhel", "centos", "rocky", "almalinux"):
		if _, err := exec.LookPath("dnf"); err != nil {
			if _, err := exec.LookPath("yum"); err == nil {
				return "yum"
			}
		}
		return "dnf"
	case family("suse", "opensuse", "opensuse-tumbleweed", "opensuse-leap", "sles"):
		return "zypper"
	}
	return ""
}

// isFedoraLike reports whether the distribution carries Fedora's own
// packages (moby-engine) rather than RHEL's narrower set.
func (d Distro) isFedoraLike() bool {
	return d.ID == "fedora"
}
