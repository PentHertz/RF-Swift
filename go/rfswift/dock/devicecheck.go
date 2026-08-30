/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Pre-creation check of device mappings: which host devices the selected
 * engine cannot map on this machine, and why. Shared by the CLI (which lists
 * them and asks once before dropping them) and the Workbench form (hint under
 * the device fields plus a dialog with a one-click removal).
 */

package dock

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/moby/moby/api/types/container"

	rfutils "penthertz/rfswift/rfutils"
)

// DeviceIssue is one device mapping (or /dev bind mount) the engine cannot
// use on this host.
type DeviceIssue struct {
	Spec   string `json:"spec"`   // the mapping as written ("/dev/ttyACM0:/dev/ttyACM0", "/dev/vhci:/dev/vhci:rw")
	Path   string `json:"path"`   // host side of the mapping
	Bind   bool   `json:"bind"`   // true for a bind mount, false for a device mapping
	Reason string `json:"reason"` // user-facing explanation
}

// DeviceCheck is the outcome of CheckDeviceMappings.
type DeviceCheck struct {
	Engine string        `json:"engine"`
	Scope  string        `json:"scope"` // host: checked against this host; vm: against the engine VM; none: not checked
	Issues []DeviceIssue `json:"issues"`
	Advice string        `json:"advice"` // what to do about the issues (empty when there are none)
}

// deviceEntry is a host path taken from the device or binding fields.
type deviceEntry struct {
	Spec string
	Path string
	Bind bool
}

// deviceCheckHost is the environment the check runs against, injectable for
// tests. exists/openable/isDeviceNode look at this host; vmExists probes the
// engine VM (Lima) and reports ok=false when the VM cannot be asked.
type deviceCheckHost struct {
	goos         string
	engine       EngineType
	rootless     bool
	exists       func(string) bool
	openable     func(string) bool
	isDeviceNode func(string) bool
	vmExists     func([]string) (map[string]bool, bool)
}

// macVMGenericDevices are the nodes every Linux VM used by Docker Desktop,
// OrbStack or Podman machine provides. Anything else under /dev needs a
// passthrough those VMs do not have.
var macVMGenericDevices = map[string]bool{
	"/dev/null": true, "/dev/zero": true, "/dev/random": true, "/dev/urandom": true,
	"/dev/tty": true, "/dev/console": true, "/dev/ptmx": true, "/dev/net/tun": true,
	"/dev/fuse": true, "/dev/kvm": true,
}

func currentDeviceCheckHost(eng ContainerEngine) deviceCheckHost {
	h := deviceCheckHost{
		goos:     runtime.GOOS,
		exists:   func(p string) bool { _, err := os.Stat(p); return err == nil },
		openable: hostPathOpenable,
		isDeviceNode: func(p string) bool {
			info, err := os.Stat(p)
			return err == nil && info.Mode()&os.ModeDevice != 0
		},
		vmExists: func([]string) (map[string]bool, bool) { return nil, false },
	}
	if eng == nil {
		return h
	}
	h.engine = eng.Type()
	h.rootless = eng.Type() == EnginePodman && os.Getuid() != 0
	if lima, ok := eng.(*LimaEngine); ok {
		h.vmExists = func(paths []string) (map[string]bool, bool) { return limaPathsExist(lima.getInstance(), paths) }
	}
	return h
}

// limaPathsExist asks the Lima VM which of the paths exist. ok is false when
// the VM is not running or limactl fails, in which case nothing is reported.
func limaPathsExist(instance string, paths []string) (map[string]bool, bool) {
	if len(paths) == 0 {
		return map[string]bool{}, true
	}
	script := `for p in "$@"; do [ -e "$p" ] && printf '%s\n' "$p"; done; exit 0`
	args := append([]string{"shell", instance, "sh", "-c", script, "sh"}, paths...)
	out, err := exec.Command(rfutils.LimaCtl(), args...).Output()
	if err != nil {
		return nil, false
	}
	found := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			found[line] = true
		}
	}
	return found, true
}

// issues evaluates the entries for this host and engine.
//
//	out: []DeviceIssue what cannot be mapped
//	out: string scope (host | vm | none)
//	out: string advice for the user, empty without issues
func (h deviceCheckHost) issues(entries []deviceEntry) ([]DeviceIssue, string, string) {
	var issues []DeviceIssue
	add := func(e deviceEntry, reason string) {
		issues = append(issues, DeviceIssue{Spec: e.Spec, Path: e.Path, Bind: e.Bind, Reason: reason})
	}
	engineName := string(h.engine)
	switch {
	case h.engine == "":
		return nil, "none", ""

	case h.engine == EngineLima:
		paths := make([]string, 0, len(entries))
		for _, e := range entries {
			paths = append(paths, e.Path)
		}
		found, ok := h.vmExists(paths)
		if !ok {
			return nil, "none", ""
		}
		for _, e := range entries {
			if !found[e.Path] {
				add(e, "not present in the Lima VM")
			}
		}
		advice := "Devices must exist inside the Lima VM."
		if h.goos == "darwin" {
			advice += " Attach USB devices to it first with: rfswift macusb attach"
		}
		return issues, "vm", adviceIf(issues, advice)

	case h.goos == "darwin":
		for _, e := range entries {
			if !macVMGenericDevices[e.Path] {
				add(e, fmt.Sprintf("not available: %s on macOS runs containers in a Linux VM without USB, serial, audio or GPU passthrough", engineName))
			}
		}
		return issues, "vm", adviceIf(issues, "Use the Lima engine (rfswift --engine lima) and attach USB devices with rfswift macusb attach.")

	case h.goos == "linux":
		for _, e := range entries {
			if !h.exists(e.Path) {
				add(e, "not present on this host")
				continue
			}
			if !h.rootless {
				continue
			}
			switch {
			case rootlessBlockedDevices[e.Path]:
				add(e, "root-only device node, rootless Podman cannot map it")
			case e.Bind && !h.isDeviceNode(e.Path):
				// directories such as /dev/bus/usb are fine
			case !h.openable(e.Path):
				add(e, "not accessible to this user (join the dialout/plugdev group or fix the udev rule)")
			}
		}
		advice := "Fix the host permissions, or run RF Swift with sudo (rootful Podman) or Docker to map root-only devices."
		if !h.rootless {
			advice = "Check the device is plugged in and the kernel module loaded."
		}
		return issues, "host", adviceIf(issues, advice)
	}
	return nil, "none", ""
}

func adviceIf(issues []DeviceIssue, advice string) string {
	if len(issues) == 0 {
		return ""
	}
	return advice
}

// deviceEntriesFromSpecs turns the device lines and bind lines of a form or
// config into host paths. Only /dev sources of bind mounts are considered.
func deviceEntriesFromSpecs(devices, bindings []string) []deviceEntry {
	var entries []deviceEntry
	seen := map[string]bool{}
	for _, spec := range devices {
		spec = strings.TrimSpace(spec)
		host := strings.SplitN(spec, ":", 2)[0]
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		entries = append(entries, deviceEntry{Spec: spec, Path: host})
	}
	for _, bind := range bindings {
		bind = strings.TrimSpace(bind)
		source := strings.SplitN(bind, ":", 2)[0]
		if !strings.HasPrefix(source, "/dev/") || seen[source] {
			continue
		}
		seen[source] = true
		entries = append(entries, deviceEntry{Spec: bind, Path: source, Bind: true})
	}
	return entries
}

// deviceEntriesFromHostConfig is the same view over an assembled HostConfig.
func deviceEntriesFromHostConfig(hc *container.HostConfig) []deviceEntry {
	if hc == nil {
		return nil
	}
	devices := make([]string, 0, len(hc.Devices))
	for _, dev := range hc.Devices {
		devices = append(devices, dev.PathOnHost+":"+dev.PathInContainer)
	}
	return deviceEntriesFromSpecs(devices, hc.Binds)
}

// removeDeviceIssues drops the reported mappings from a HostConfig.
func removeDeviceIssues(hc *container.HostConfig, issues []DeviceIssue) {
	if hc == nil || len(issues) == 0 {
		return
	}
	dropDevice := map[string]bool{}
	dropBind := map[string]bool{}
	for _, issue := range issues {
		if issue.Bind {
			dropBind[issue.Path] = true
		} else {
			dropDevice[issue.Path] = true
		}
	}
	var devices []container.DeviceMapping
	for _, dev := range hc.Devices {
		if !dropDevice[dev.PathOnHost] {
			devices = append(devices, dev)
		}
	}
	hc.Devices = devices
	var binds []string
	for _, bind := range hc.Binds {
		if !dropBind[strings.SplitN(bind, ":", 2)[0]] {
			binds = append(binds, bind)
		}
	}
	hc.Binds = binds
}

// CheckDeviceMappings reports which device lines and /dev bind lines of a
// container form the engine cannot map on this machine. It is advisory: the
// caller decides whether to remove them.
//
//	in(1): string engineName docker | podman | lima (empty keeps the current engine)
//	in(2): []string devices   device mappings "host:container[:perm]"
//	in(3): []string bindings  bind mounts "source:dest[:opts]"
//	out: DeviceCheck issues, scope and advice
func CheckDeviceMappings(engineName string, devices, bindings []string) DeviceCheck {
	if engineName != "" {
		SetPreferredEngine(engineName)
	}
	eng := GetEngine()
	check := DeviceCheck{Engine: engineName, Scope: "none", Issues: []DeviceIssue{}}
	if eng == nil || (engineName != "" && string(eng.Type()) != strings.ToLower(engineName)) {
		return check
	}
	check.Engine = string(eng.Type())
	issues, scope, advice := currentDeviceCheckHost(eng).issues(deviceEntriesFromSpecs(devices, bindings))
	check.Scope = scope
	check.Advice = advice
	if issues != nil {
		check.Issues = issues
	}
	return check
}
