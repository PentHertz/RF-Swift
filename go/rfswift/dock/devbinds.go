/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  /dev paths in bind mounts, and the stray directories they leave behind.
*
*  Docker (and Podman) treat a bind mount whose host path is missing by
*  creating it as a directory. For a device node that is unplugged at start
*  (/dev/ttyACM0 listed under "volumes"), the result is a root-owned, empty
*  directory sitting where the node belongs: the device never reappears under
*  that name, every tool fails, and the directory has to be removed as root.
*  This file keeps that from happening and cleans up after it:
*
*    - SanitizeDeviceBinds, at creation and re-creation: a /dev source that is
*      a device node becomes a device mapping (the engine refuses to start
*      without it instead of inventing a directory); a missing or stray source
*      is dropped with a warning that says what to do.
*    - PreflightDevices, before a start: refuses a start whose bind would
*      create such a directory, naming the mapping to fix.
*    - StrayDeviceDirs / RemoveStrayDeviceDirs: find and delete the leftovers
*      (rfswift host devclean, the Workbench engine doctor).
 */

package dock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"penthertz/rfswift/hostsetup"
)

// devTreeRules are the device cgroup rules that make a bind-mounted /dev tree
// usable inside the container (the mount shows the nodes, the rule lets the
// kernel open them).
var devTreeRules = map[string]string{"/dev/bus/usb": "c 189:* rwm", "/dev/snd": "c 116:* rwm", "/dev/dri": "c 226:* rwm", "/dev/input": "c 13:* rwm", "/dev/vhci": "c 137:* rwm"}

// deviceNodeName matches names of individual device nodes (ttyACM0, ttyUSB1,
// hidraw3, video0, sda1, rfcomm0), as opposed to device trees (bus, snd, dri).
var deviceNodeName = regexp.MustCompile(`^[A-Za-z_]+[0-9]+[A-Za-z0-9_]*$`)

// DevBindWarning describes a /dev bind SanitizeDeviceBinds did not keep.
type DevBindWarning struct {
	Spec   string `json:"spec"`
	Path   string `json:"path"`
	Reason string `json:"reason"` // "missing" or "stray-directory"
	Advice string `json:"advice"`
}

func (w DevBindWarning) String() string { return w.Path + ": " + w.Advice }

// IsStrayDeviceDir reports whether path is an empty directory under /dev
// with a device node's name: what a container bind mount creates when the
// device was unplugged at start.
func IsStrayDeviceDir(path string) bool {
	return isStrayDeviceDirIn("/dev", path)
}

func isStrayDeviceDirIn(devRoot, path string) bool {
	clean := filepath.Clean(path)
	if filepath.Dir(clean) != filepath.Clean(devRoot) || !deviceNodeName.MatchString(filepath.Base(clean)) {
		return false
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(clean)
	return err == nil && len(entries) == 0
}

// DeviceRuleFor is the device cgroup rule that lets a container open the
// device node at path ("c 166:* rwm" for /dev/ttyACM0), "" when path is not
// a device node or the numbers cannot be read.
func DeviceRuleFor(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.Mode()&os.ModeDevice == 0 {
		return ""
	}
	major, _, ok := deviceNumbers(path)
	if !ok {
		return ""
	}
	kind := "b"
	if info.Mode()&os.ModeCharDevice != 0 {
		kind = "c"
	}
	return fmt.Sprintf("%s %d:* rwm", kind, major)
}

// SanitizeDeviceBinds goes through bind mounts ("source:target[:opts]") and
// takes the /dev sources the engine would mishandle out of its way:
//
//	device node  -> kept as the bind mount that was asked for, plus the
//	                cgroup rule that lets the container open it (rules)
//	missing      -> dropped, warned (would become a directory)
//	stray dir    -> dropped, warned with the cleanup command
//	real tree    -> kept as a bind (/dev/bus/usb, /dev/snd, ...)
//
// Everything outside /dev is kept untouched. Sources inside a VM (Lima) are
// left to the engine: they are not visible from here. devices is kept for
// callers that want mappings instead; it is empty now that a bind mount of
// a node is honoured as such.
func SanitizeDeviceBinds(binds []string) (kept []string, devices []string, rules []string, warnings []DevBindWarning) {
	// With Lima the container runs inside a QEMU VM: the /dev paths a mission
	// binds (the USB tree, serial ports) exist in that VM, not on this macOS
	// host, so stat'ing them here would wrongly drop every one as "missing".
	// Keep them and attach the cgroup rule by name instead, leaving the paths
	// to the engine as the rest of this package does.
	remote := devicePathsBelongToVM()
	for _, bind := range binds {
		spec := strings.TrimSpace(bind)
		parts := strings.SplitN(spec, ":", 3)
		source := parts[0]
		if len(parts) < 2 || !strings.HasPrefix(source, "/dev/") {
			kept = append(kept, bind)
			continue
		}
		if remote {
			kept = append(kept, bind)
			target := parts[1]
			if rule := devTreeRules[source]; rule != "" {
				rules = appendMissing(rules, rule)
			} else if rule := devTreeRules[target]; rule != "" {
				rules = appendMissing(rules, rule)
			}
			if IsSerialDevicePath(source) || IsSerialDevicePath(target) {
				rules = appendMissing(rules, SerialCgroupRules...)
			}
			continue
		}
		info, err := os.Stat(source)
		switch {
		case err != nil:
			warnings = append(warnings, DevBindWarning{Spec: spec, Path: source, Reason: "missing",
				Advice: fmt.Sprintf("not present on this host, so it was not mounted: the engine would have created a directory in its place. Plug the device in and add it from the mission's configuration, or bind the USB tree (/dev/bus/usb) with its cgroup rule for hot-plugging (%s).", source)})
		case info.Mode()&os.ModeDevice != 0:
			kept = append(kept, bind)
			if rule := DeviceRuleFor(source); rule != "" {
				rules = appendMissing(rules, rule)
			}
			if IsSerialDevicePath(source) {
				rules = appendMissing(rules, SerialCgroupRules...)
			}
		case info.IsDir() && IsStrayDeviceDir(source):
			warnings = append(warnings, DevBindWarning{Spec: spec, Path: source, Reason: "stray-directory",
				Advice: fmt.Sprintf("is an empty directory, not a device: a container was started while the device was unplugged and the engine created it. Remove it with `rfswift host devclean` (or `sudo rmdir %s`), then plug the device in.", source)})
		default:
			kept = append(kept, bind)
			if rule := devTreeRules[source]; rule != "" {
				rules = appendMissing(rules, rule)
			}
		}
	}
	return kept, devices, rules, warnings
}

// devicePathsBelongToVMFn is the seam tests use to pin the VM-path decision
// without a live engine; production keeps the engine-backed implementation.
var devicePathsBelongToVMFn = devicePathsBelongToVMFromEngine

// devicePathsBelongToVM reports whether the /dev paths in a bind mount refer
// to a virtual machine rather than this host. On macOS the Lima engine runs
// containers in a QEMU VM whose /dev tree (USB devices hot-plugged with
// `rfswift macusb attach`, serial ports) is not visible from the host, so a
// host stat of those paths is meaningless and must not gate the bind.
func devicePathsBelongToVM() bool { return devicePathsBelongToVMFn() }

func devicePathsBelongToVMFromEngine() bool {
	if runtime.GOOS == "linux" {
		return false
	}
	engine := GetEngine()
	return engine != nil && engine.Type() == EngineLima
}

// PreflightDevices refuses to start a container whose /dev bind mount would
// be created as a directory, and explains a device mapping that is missing,
// which the engine would otherwise report as "error gathering device
// information".
func PreflightDevices(ctx context.Context, cli *client.Client, id string) error {
	res, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return nil // let the start report the real problem
	}
	hc := res.Container.HostConfig
	if hc == nil {
		return nil
	}
	var problems []string
	if devicePathsBelongToVM() {
		// The paths live in the engine's VM (Lima): ask it, the host knows
		// nothing about them. An unreachable VM leaves the start to report.
		problems = preflightVMDevices(hc)
		if len(problems) == 0 {
			return nil
		}
		return fmt.Errorf("cannot start %s: %s. Attach the device to the VM (rfswift macusb attach, or the USB button in the Workbench), or remove the mapping from the mission's configuration", strings.TrimPrefix(res.Container.Name, "/"), strings.Join(problems, "; "))
	}
	for _, bind := range hc.Binds {
		source := strings.SplitN(bind, ":", 2)[0]
		if !strings.HasPrefix(source, "/dev/") {
			continue
		}
		if _, err := os.Stat(source); err != nil {
			problems = append(problems, fmt.Sprintf("%s is bound as a volume but not present; starting now would create a directory in its place", source))
		} else if IsStrayDeviceDir(source) {
			problems = append(problems, fmt.Sprintf("%s is an empty directory left by an earlier start, not a device (remove it: rfswift host devclean)", source))
		}
	}
	for _, dev := range hc.Devices {
		if _, err := os.Stat(dev.PathOnHost); err != nil {
			if IsSerialDevicePath(dev.PathOnHost) {
				problems = append(problems, fmt.Sprintf("serial port %s is not plugged in (it was mapped while plugged in; remove it from the mission's devices to have it attached on demand instead)", dev.PathOnHost))
			} else {
				problems = append(problems, fmt.Sprintf("device %s is not present", dev.PathOnHost))
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("cannot start %s: %s. Plug the device in, or remove the mapping from the mission's configuration", strings.TrimPrefix(res.Container.Name, "/"), strings.Join(problems, "; "))
}

// preflightVMDevices is PreflightDevices for a container whose /dev paths
// live in the engine's VM: the missing ones, by the VM's account.
func preflightVMDevices(hc *container.HostConfig) []string {
	var paths []string
	for _, bind := range hc.Binds {
		if source := strings.SplitN(bind, ":", 2)[0]; strings.HasPrefix(source, "/dev/") {
			paths = append(paths, source)
		}
	}
	for _, dev := range hc.Devices {
		paths = append(paths, dev.PathOnHost)
	}
	infos, ok := vmPathInfo(paths)
	if !ok {
		return nil
	}
	var problems []string
	for _, bind := range hc.Binds {
		source := strings.SplitN(bind, ":", 2)[0]
		if strings.HasPrefix(source, "/dev/") && !infos[source].Exists {
			problems = append(problems, fmt.Sprintf("%s is bound as a volume but not present in the VM; starting now would create a directory in its place", source))
		}
	}
	for _, dev := range hc.Devices {
		if !infos[dev.PathOnHost].Exists {
			problems = append(problems, fmt.Sprintf("device %s is not present in the VM", dev.PathOnHost))
		}
	}
	return problems
}

// serialPortName matches the hot-pluggable serial ports: USB CDC-ACM
// (ttyACM*), USB serial adapters (ttyUSB*) and ttyAMA*. Virtual consoles
// (/dev/tty0, /dev/tty1, ...), /dev/tty itself and on-board UARTs
// (/dev/ttyS*) are not: they never come and go, and a container that maps
// /dev/tty0 needs it (the Proxmark client refuses to start without it).
var serialPortName = regexp.MustCompile(`^/dev/tty(ACM|USB|AMA)[0-9]+$`)

// IsSerialDevicePath reports whether path names a hot-pluggable serial port
// (/dev/ttyACM0, /dev/ttyUSB0, /dev/ttyAMA0): the usual way to talk to a
// Proxmark3, a flasher or a modem.
func IsSerialDevicePath(path string) bool {
	return serialPortName.MatchString(strings.TrimSpace(path))
}

// RequiredDevicesError turns the /dev entries that could not be mapped into a
// hard error when one of them is a serial port. A mission that names a
// serial port needs it: creating the mission without it would only move the
// failure to the first tool that opens the port, with no hint why. Other
// optional devices (rfkill, tun) stay warnings.
func RequiredDevicesError(warnings []DevBindWarning, missingDevices []string) error {
	var lines []string
	for _, w := range warnings {
		if IsSerialDevicePath(w.Path) {
			lines = append(lines, w.String())
		}
	}
	for _, path := range missingDevices {
		if IsSerialDevicePath(path) {
			lines = append(lines, path+": not present on this host")
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("this mission needs a serial port that is not available:\n  %s\nPlug the device in (check with: ls -l /dev/tty*) and create the mission again, or remove the port from its devices and bind mounts. A serial port must exist as a TTY device when the mission is created; it cannot be added later by plugging in", strings.Join(lines, "\n  "))
}

func appendMissing(list []string, values ...string) []string {
	for _, v := range values {
		found := false
		for _, existing := range list {
			if strings.TrimSpace(existing) == v {
				found = true
				break
			}
		}
		if !found {
			list = append(list, v)
		}
	}
	return list
}

// StrayDeviceDirs lists the empty device-named directories under /dev.
func StrayDeviceDirs() []string { return strayDeviceDirsIn("/dev") }

func strayDeviceDirsIn(devRoot string) []string {
	entries, err := os.ReadDir(devRoot)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(devRoot, e.Name())
		if isStrayDeviceDirIn(devRoot, p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// RemoveStrayDeviceDirs deletes the given stray directories as root (sudo on
// a terminal, a polkit prompt from the Workbench). Only paths StrayDeviceDirs
// would list are accepted, and each is re-checked in the script itself.
func RemoveStrayDeviceDirs(paths []string) error {
	var lines []string
	for _, p := range paths {
		if !IsStrayDeviceDir(p) {
			return fmt.Errorf("%s is not an empty device-named directory under /dev", p)
		}
		q := hostsetup.ShellQuote(filepath.Clean(p))
		lines = append(lines, fmt.Sprintf(`if [ -d %s ] && [ -z "$(ls -A %s)" ]; then rmdir %s; fi`, q, q, q))
	}
	if len(lines) == 0 {
		return nil
	}
	return hostsetup.RunPrivileged("set -e\n" + strings.Join(lines, "\n") + "\n")
}
