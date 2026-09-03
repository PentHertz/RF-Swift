/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Host setup - installing Docker and/or Podman from the distribution's own
*  repositories (the same packages the rfswift deb/rpm/pacman packages
*  Recommend), then finishing what a package cannot do for a specific user:
*  the docker group + session ACL, and rootless Podman's subordinate ID ranges
*  and lingering.
 */

package hostsetup

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// EngineChoice is what the user asked to install.
type EngineChoice string

const (
	EngineDocker EngineChoice = "docker"
	EnginePodman EngineChoice = "podman"
	EngineBoth   EngineChoice = "both"
	EngineNone   EngineChoice = "none"
)

// ParseEngineChoice accepts docker | podman | both | none (case-insensitive).
func ParseEngineChoice(s string) (EngineChoice, error) {
	switch c := EngineChoice(strings.ToLower(strings.TrimSpace(s))); c {
	case EngineDocker, EnginePodman, EngineBoth, EngineNone:
		return c, nil
	case "":
		return EngineNone, nil
	}
	return "", fmt.Errorf("unknown engine choice %q (docker, podman, both or none)", s)
}

// EnginePresent reports whether the engine's CLI is on PATH.
func EnginePresent(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// EngineInstallPlan is everything InstallEngines will do, so the user can see
// it before agreeing.
type EngineInstallPlan struct {
	Distro   Distro   `json:"distro"`
	Docker   bool     `json:"docker"`
	Podman   bool     `json:"podman"`
	Packages []string `json:"packages"` // distribution packages to install
	Steps    []string `json:"steps"`    // human-readable summary of the privileged script
	Script   string   `json:"script"`   // the privileged script itself
	User     string   `json:"user"`     // account set up for docker / rootless podman
}

// enginePackages lists the distribution packages per engine, or an error when
// the distribution does not carry the engine.
func enginePackages(d Distro, engine EngineChoice) ([]string, error) {
	switch d.PackageManager {
	case "apt":
		if engine == EngineDocker {
			return []string{"docker.io"}, nil
		}
		return []string{"podman", "uidmap", "slirp4netns", "fuse-overlayfs"}, nil
	case "dnf", "yum":
		if engine == EngineDocker {
			if d.isFedoraLike() {
				return []string{"moby-engine"}, nil
			}
			return nil, fmt.Errorf("%s does not package Docker: install Podman (rfswift works the same) or follow https://docs.docker.com/engine/install/", d.Name)
		}
		return []string{"podman", "slirp4netns", "fuse-overlayfs"}, nil
	case "pacman":
		if engine == EngineDocker {
			return []string{"docker"}, nil
		}
		return []string{"podman", "crun", "slirp4netns", "fuse-overlayfs"}, nil
	case "zypper":
		if engine == EngineDocker {
			return []string{"docker"}, nil
		}
		return []string{"podman", "slirp4netns", "fuse-overlayfs"}, nil
	case "apk":
		if engine == EngineDocker {
			return []string{"docker"}, nil
		}
		return []string{"podman", "fuse-overlayfs", "slirp4netns"}, nil
	case "nix":
		return nil, errors.New("on NixOS enable virtualisation.docker / virtualisation.podman in configuration.nix")
	}
	return nil, fmt.Errorf("unsupported distribution (%s): install Docker or Podman with its package manager", d.Name)
}

// installCommand is the package manager's non-interactive install line.
func installCommand(pm string, packages []string) string {
	pkgs := strings.Join(packages, " ")
	switch pm {
	case "apt":
		return "export DEBIAN_FRONTEND=noninteractive\napt-get update\napt-get install -y " + pkgs
	case "dnf":
		return "dnf install -y " + pkgs
	case "yum":
		return "yum install -y " + pkgs
	case "pacman":
		return "pacman -Sy --noconfirm --needed " + pkgs
	case "zypper":
		return "zypper --non-interactive install " + pkgs
	case "apk":
		return "apk add " + pkgs
	}
	return ""
}

// podmanRootlessScript gives the user the subordinate ID ranges rootless
// Podman needs and lets its user services outlive the login session.
func podmanRootlessScript(username string) string {
	q := ShellQuote(username)
	return "grep -q \"^\" /etc/subuid 2>/dev/null || touch /etc/subuid\n" +
		"grep -q \"^\" /etc/subgid 2>/dev/null || touch /etc/subgid\n" +
		fmt.Sprintf("grep -q \"^%s:\" /etc/subuid || usermod --add-subuids 100000-165535 %s\n", username, q) +
		fmt.Sprintf("grep -q \"^%s:\" /etc/subgid || usermod --add-subgids 100000-165535 %s\n", username, q) +
		fmt.Sprintf("if command -v loginctl >/dev/null 2>&1; then loginctl enable-linger %s || true; fi\n", q)
}

// PlanEngineInstall builds the plan for choice on this host.
func PlanEngineInstall(choice EngineChoice) (EngineInstallPlan, error) {
	plan := EngineInstallPlan{Distro: DetectDistro(), User: InvokingUser()}
	if runtime.GOOS != "linux" {
		return plan, errors.New("engine installation from rfswift is for Linux; use the macOS setup script or the Windows installer bundle")
	}
	switch choice {
	case EngineDocker:
		plan.Docker = true
	case EnginePodman:
		plan.Podman = true
	case EngineBoth:
		plan.Docker, plan.Podman = true, true
	case EngineNone:
		return plan, errors.New("nothing to install")
	default:
		return plan, fmt.Errorf("unknown engine choice %q", choice)
	}
	if plan.User == "" || !validUserRe.MatchString(plan.User) {
		return plan, fmt.Errorf("cannot determine a plain user account to set up (got %q)", plan.User)
	}
	var b strings.Builder
	b.WriteString("set -e\n")
	if plan.Docker {
		pkgs, err := enginePackages(plan.Distro, EngineDocker)
		if err != nil {
			return plan, err
		}
		plan.Packages = append(plan.Packages, pkgs...)
	}
	if plan.Podman {
		pkgs, err := enginePackages(plan.Distro, EnginePodman)
		if err != nil {
			return plan, err
		}
		plan.Packages = append(plan.Packages, pkgs...)
	}
	b.WriteString(installCommand(plan.Distro.PackageManager, plan.Packages) + "\n")
	plan.Steps = append(plan.Steps, fmt.Sprintf("install %s with %s", strings.Join(plan.Packages, ", "), plan.Distro.PackageManager))
	if plan.Docker {
		if plan.Distro.PackageManager == "apk" {
			b.WriteString("rc-update add docker default >/dev/null 2>&1 || true\nservice docker start >/dev/null 2>&1 || true\n")
		} else {
			b.WriteString("if command -v systemctl >/dev/null 2>&1; then systemctl enable --now docker; fi\n")
		}
		plan.Steps = append(plan.Steps, "enable and start the docker service")
		grant, err := dockerGrantScript(plan.User, dockerSocket)
		if err != nil {
			return plan, err
		}
		b.WriteString(strings.TrimPrefix(grant, "set -e\n"))
		plan.Steps = append(plan.Steps, fmt.Sprintf("add %s to the docker group and grant this session access to the socket", plan.User))
	}
	if plan.Podman {
		b.WriteString(podmanRootlessScript(plan.User))
		plan.Steps = append(plan.Steps, fmt.Sprintf("give %s subordinate UID/GID ranges for rootless Podman and enable lingering", plan.User))
	}
	plan.Script = b.String()
	return plan, nil
}

// EngineInstallReport is what InstallEngines did.
type EngineInstallReport struct {
	Plan         EngineInstallPlan `json:"plan"`
	DockerAccess *DockerAccess     `json:"dockerAccess,omitempty"` // state after the change, when Docker was installed
	PodmanSocket bool              `json:"podmanSocket"`           // the user's podman.socket was enabled
	Detail       string            `json:"detail"`
}

// InstallEngines runs the plan: one privileged call for the packages, the
// service and the per-user setup, then the unprivileged part (the user's
// podman.socket, best effort).
func InstallEngines(plan EngineInstallPlan) (EngineInstallReport, error) {
	report := EngineInstallReport{Plan: plan}
	if plan.Script == "" {
		return report, errors.New("empty plan")
	}
	if err := RunPrivileged(plan.Script); err != nil {
		return report, fmt.Errorf("engine installation failed: %w", err)
	}
	var parts []string
	if plan.Docker {
		st := GetDockerAccess()
		report.DockerAccess = &st
		if st.Accessible {
			parts = append(parts, "Docker is installed and usable right away in this session")
		} else {
			parts = append(parts, "Docker is installed; "+st.Detail)
		}
	}
	if plan.Podman {
		if _, err := exec.LookPath("systemctl"); err == nil {
			if err := exec.Command("systemctl", "--user", "enable", "--now", "podman.socket").Run(); err == nil {
				report.PodmanSocket = true
			}
		}
		parts = append(parts, "Podman is installed for rootless use (containers run as "+plan.User+", no daemon)")
	}
	report.Detail = strings.Join(parts, "; ")
	return report, nil
}
