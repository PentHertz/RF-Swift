/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  USB reachability check for a container configuration. Shared by the CLI
*  (pre-creation notice), the run wizard and the Workbench mission form so
*  every front door gives the same, verified answer:
*
*    - /dev/bus/usb must be inside the container (bind mount or device
*      mapping) for hot-plugged devices to appear, and
*    - the device cgroup must allow USB character devices (major 189),
*      otherwise the nodes are listed but open() fails with "Permission
*      denied". RF Swift infers "c 189:* rwm" from a /dev/bus/usb *device*
*      mapping; a plain bind mount needs the rule spelled out.
*
*  Privileged mode also works but is not required - the point of this check is
*  to say so before someone reaches for it.
 */

package common

import "strings"

// USBTreePath is the USB device tree containers need for hot-plug access.
const USBTreePath = "/dev/bus/usb"

// USBCgroupRule allows USB character devices (major 189) in the container.
const USBCgroupRule = "c 189:* rwm"

// USBAccess is the outcome of CheckUSBAccess.
type USBAccess struct {
	Level      string `json:"level"`      // ok | warn | none
	Summary    string `json:"summary"`    // what the configuration gives, user-facing
	Advice     string `json:"advice"`     // what to change (empty when nothing is needed)
	Tree       bool   `json:"tree"`       // /dev/bus/usb reaches the container
	Rule       bool   `json:"rule"`       // major 189 is allowed (explicit, inferred, or privileged)
	Nodes      bool   `json:"nodes"`      // individual /dev/bus/usb/BBB/DDD nodes are mapped
	Privileged bool   `json:"privileged"` // --privileged (everything reachable)
}

// CheckUSBAccess evaluates device mappings, bind mounts and cgroup rules the
// way RF Swift applies them at creation (see dock.normalizeCreationDevices).
//
//	in(1): []string devices device specs "host[:container[:opts]]"
//	in(2): []string bindings bind mounts "host:container[:mode]"
//	in(3): []string cgroupRules device cgroup rules, e.g. "c 189:* rwm"
//	in(4): bool privileged --privileged
//	out: USBAccess
func CheckUSBAccess(devices, bindings, cgroupRules []string, privileged bool) USBAccess {
	access := USBAccess{Privileged: privileged}
	treeViaDevice := false
	for _, spec := range devices {
		host, target := splitMappingSpec(spec)
		switch {
		case target == USBTreePath || host == USBTreePath:
			treeViaDevice = true
		case strings.HasPrefix(target, USBTreePath+"/") || strings.HasPrefix(host, USBTreePath+"/"):
			access.Nodes = true
		}
	}
	treeViaBind := false
	for _, spec := range bindings {
		host, target := splitMappingSpec(spec)
		if target == USBTreePath || host == USBTreePath {
			treeViaBind = true
		}
	}
	access.Tree = treeViaDevice || treeViaBind
	explicitRule := false
	for _, rule := range cgroupRules {
		if cgroupRuleAllowsUSB(rule) {
			explicitRule = true
			break
		}
	}
	// A /dev/bus/usb device mapping makes RF Swift add the major rule itself.
	access.Rule = privileged || explicitRule || treeViaDevice

	switch {
	case privileged:
		access.Level = "ok"
		access.Summary = "Privileged container: every host device is reachable."
		access.Advice = "Privileged mode is not required for USB: mapping " + USBTreePath + " with the device rule " + USBCgroupRule + " is enough and keeps the container confined."
	case access.Tree && access.Rule:
		access.Level = "ok"
		access.Summary = USBTreePath + " is mapped and USB device major 189 is allowed: devices forwarded later show up automatically (hot-plug), no privileged mode needed."
	case access.Tree:
		access.Level = "warn"
		access.Summary = USBTreePath + " is bind-mounted but the device cgroup rule " + USBCgroupRule + " is missing: USB devices will be listed but cannot be opened (Permission denied)."
		access.Advice = "Add the cgroup rule " + USBCgroupRule + ", or map " + USBTreePath + " as a device so RF Swift adds it - privileged mode is not needed."
	case access.Nodes:
		access.Level = "warn"
		access.Summary = "Only individual USB device nodes are mapped: a device forwarded later, or re-plugged, gets a new node and will not be reachable."
		access.Advice = "Map " + USBTreePath + " with the device rule " + USBCgroupRule + " for hot-plug access."
	default:
		access.Level = "none"
		access.Summary = "No " + USBTreePath + " mapping: USB devices forwarded to the VM or host will not be reachable in this container."
		access.Advice = "Map " + USBTreePath + " with the device rule " + USBCgroupRule + " (RF Swift's USB hotplug defaults) - privileged mode is not needed."
	}
	return access
}

// splitMappingSpec returns the host and container paths of "host[:container[:opts]]".
// A bare path maps to itself.
func splitMappingSpec(spec string) (string, string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", ""
	}
	parts := strings.Split(spec, ":")
	host := strings.TrimRight(strings.TrimSpace(parts[0]), "/")
	target := host
	if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
		target = strings.TrimRight(strings.TrimSpace(parts[1]), "/")
	}
	return host, target
}

// cgroupRuleAllowsUSB reports whether a device cgroup rule grants read+write
// on USB character devices: "c 189:* rwm", "c *:* rwm", "a *:* rwm" or the
// cgroup v1 catch-all "a".
func cgroupRuleAllowsUSB(rule string) bool {
	fields := strings.Fields(strings.ToLower(rule))
	switch len(fields) {
	case 1:
		return fields[0] == "a"
	case 3:
		if fields[0] != "a" && fields[0] != "c" {
			return false
		}
		major, _, found := strings.Cut(fields[1], ":")
		if !found || (major != "*" && major != "189") {
			return false
		}
		return strings.Contains(fields[2], "r") && strings.Contains(fields[2], "w")
	default:
		return false
	}
}
