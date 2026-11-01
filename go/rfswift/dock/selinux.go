/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * SELinux on the container host (Fedora, RHEL and their derivatives - and
 * so the default Fedora templates of Qubes OS).
 *
 * With SELinux enforcing, Podman and Docker run every unprivileged
 * container as container_t with a private MCS category. RF Swift containers
 * are built to share the host: the workspace in the user's home
 * (user_home_t), the X11 socket, /dev/bus/usb and the other device nodes.
 * container_t may use none of them, so an unprivileged container cannot
 * even read its own /workspace ("Permission denied"), let alone open the
 * SDR. Only privileged containers worked, because privileged mode turns the
 * labelling off.
 *
 * Relabelling the binds (:z) is not an option: /tmp/.X11-unix and /dev
 * belong to the host and must keep their labels. RF Swift therefore does
 * what toolbox and distrobox do for the same reason: it disables SELinux
 * separation for its own containers (--security-opt label=disable). Their
 * isolation is the namespaces, capabilities and seccomp they already have;
 * the host's SELinux policy keeps applying to everything else.
 *
 * Opt out with `[container] selinux_label = keep` in config.ini, or
 * RFSWIFT_SELINUX_LABEL=keep.
 */

package dock

import (
	"os"
	"runtime"
	"strings"

	"github.com/moby/moby/api/types/container"

	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
)

// selinuxEnforceFile reports the SELinux mode; overridable in tests.
var selinuxEnforceFile = "/sys/fs/selinux/enforce"

// selinuxEnforcing reports whether SELinux is enforcing on this host.
func selinuxEnforcing() bool {
	b, err := os.ReadFile(selinuxEnforceFile)
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// selinuxLabelPolicy is "keep" when the user opted out, "auto" otherwise.
func selinuxLabelPolicy() string {
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("RFSWIFT_SELINUX_LABEL"))); v != "" {
		return v
	}
	if v := strings.ToLower(rfutils.ConfigValue(common.ConfigFileByPlatform(), "container", "selinux_label")); v != "" {
		return v
	}
	return "auto"
}

// hasLabelOpt reports whether a SecurityOpt list already sets a label.
func hasLabelOpt(opts []string) bool {
	for _, o := range opts {
		if strings.HasPrefix(o, "label=") || strings.HasPrefix(o, "label:") {
			return true
		}
	}
	return false
}

// selinuxLabelApplies reports whether label=disable must be added for this
// engine on this host: a Linux host with SELinux enforcing, a native engine
// (the Lima VM has its own kernel), an unprivileged container, no explicit
// label option, and no opt-out.
func selinuxLabelApplies(engine EngineType, enforcing bool, policy string, hc *container.HostConfig) bool {
	if hc == nil || hc.Privileged || !enforcing || policy == "keep" {
		return false
	}
	if engine != EngineDocker && engine != EnginePodman {
		return false
	}
	return !hasLabelOpt(hc.SecurityOpt)
}

// applyHostSELinux adds label=disable to an RF Swift container's host
// configuration when SELinux would otherwise keep it from its workspace,
// display and devices (see the file comment). notify, when set, receives
// one line explaining it.
func applyHostSELinux(hc *container.HostConfig, notify func(string)) {
	if runtime.GOOS != "linux" {
		return
	}
	eng := GetEngine()
	if eng == nil || !selinuxLabelApplies(eng.Type(), selinuxEnforcing(), selinuxLabelPolicy(), hc) {
		return
	}
	hc.SecurityOpt = append(hc.SecurityOpt, "label=disable")
	if notify != nil {
		notify("SELinux is enforcing: SELinux separation is disabled for this container (label=disable) so it can use its workspace, the display and the USB devices. Opt out with [container] selinux_label = keep.")
	}
}
