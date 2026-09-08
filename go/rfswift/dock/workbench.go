package dock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/moby/moby/client"
)

// UpdateBinding safely updates a mount or device mapping. Docker uses the
// RF-Swift CLI's persisted-config rebinding path; other engines use recreation.
// Unlike the legacy CLI helpers, it returns errors instead of terminating.
func UpdateBinding(containerID, kind, source, target string, add bool) error {
	source, target = strings.TrimSpace(source), strings.TrimSpace(target)
	if source == "" || target == "" {
		return errors.New("both host source and container target are required")
	}
	kind, bindRules, err := resolveBindingKind(kind, source, add)
	if err != nil {
		return err
	}
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return err
	}
	defer cli.Close()
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		return err
	}
	var entry, separator string
	switch kind {
	case "volume":
		entry, separator = source+":"+target, ";;"
	case "device":
		entry, separator = source+":"+target, ","
	default:
		return fmt.Errorf("unsupported binding kind %q", kind)
	}
	key := "Bindings"
	if kind == "device" {
		key = "Devices"
	}
	// A serial port on an engine with hot-plug is always recorded and granted
	// through the cgroup; as a device it is mapped when plugged in and left to
	// the on-demand node creation when it is not; as a bind mount it is
	// mounted as asked.
	// A bind-mounted serial port is a bind mount, nothing more; the on-demand
	// handling and the label are for ports given as devices.
	serialPort := kind == "device" && IsSerialDevicePath(source) && SerialHotplugSupported(GetEngine())
	serialOnDemand := serialPort && !devicePresent(source)
	rulesMissing := func() bool {
		for _, rule := range bindRules {
			if !strings.Contains(props["Cgroups"], rule) {
				return true
			}
		}
		return false
	}
	updated := strings.Join(updatePropertyItems(strings.Split(props[key], separator), entry, target, add), separator)
	if serialPort {
		var addPorts, removePorts []string
		if add {
			addPorts = []string{source}
		} else {
			removePorts = []string{source}
		}
		merged := MergeSerialPorts(props["SerialPorts"], addPorts, removePorts)
		if merged == props["SerialPorts"] && updated == props[key] && (!add || !rulesMissing()) {
			return nil
		}
		props["SerialPorts"] = merged
	} else if updated == props[key] && (!add || !rulesMissing()) {
		return nil
	}
	props[key] = updated
	if add && len(bindRules) > 0 {
		props["Cgroups"] = strings.Join(appendMissing(splitSummaryList(props["Cgroups"], ","), bindRules...), ",")
	}

	// Match the RF-Swift CLI: native Docker can rebind by editing its persisted
	// hostconfig/config.v2 files. Podman and Lima do not expose that storage, so
	// they retain the commit/recreate compatibility path below.
	if useDirectConfigEdit() {
		inspected, err := inspectContainer(ctx, cli, containerID)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(inspected.Name, "/")
		return directEditContainer(ctx, cli, containerID, name, func(host *HostConfigFull, configV2 map[string]interface{}) (bool, error) {
			switch kind {
			case "volume":
				host.Devices = removeDevicesAtTarget(host.Devices, target)
				removeConfigDevicesAtTarget(configV2, target)
				mount := source + ":" + target
				host.Binds = removeBindsAtTarget(host.Binds, target)
				if add {
					host.Binds = append(host.Binds, mount)
					addMountPoint(configV2, source, target)
					host.DeviceCgroupRules = appendMissing(host.DeviceCgroupRules, bindRules...)
				} else {
					host.Binds = removeBindByPrefix(host.Binds, mount)
					removeMountPoint(configV2, target)
				}
			case "device":
				// Repair containers created by older RFID profiles, which exposed
				// /dev/ttyACM0 as a bind without granting device-cgroup access.
				host.Binds = removeBindsAtTarget(host.Binds, target)
				removeMountPoint(configV2, target)
				host.Devices = removeDevicesAtTarget(host.Devices, target)
				removeConfigDevicesAtTarget(configV2, target)
				if serialPort {
					// The cgroup rules make the port usable across replugs and
					// SyncSerialDevices creates its node when it is absent
					// (serialhotplug.go); the label shows the port on the card.
					if add {
						host.DeviceCgroupRules = appendMissing(host.DeviceCgroupRules, bindRules...)
						if !serialOnDemand {
							host.Devices = append(host.Devices, DeviceMapping{PathOnHost: source, PathInContainer: target, CgroupPermissions: "rwm"})
							addDeviceMapping(configV2, source, target)
						}
					}
					if config, ok := configV2["Config"].(map[string]interface{}); ok {
						labels, _ := config["Labels"].(map[string]interface{})
						if labels == nil {
							labels = map[string]interface{}{}
							config["Labels"] = labels
						}
						current, _ := labels[SerialPortsLabel].(string)
						if merged := MergeSerialPorts(current, map[bool][]string{true: {source}}[add], map[bool][]string{true: {source}}[!add]); merged != "" {
							labels[SerialPortsLabel] = merged
						} else {
							delete(labels, SerialPortsLabel)
						}
					}
				} else if add {
					host.Devices = append(host.Devices, DeviceMapping{PathOnHost: source, PathInContainer: target, CgroupPermissions: "rwm"})
					addDeviceMapping(configV2, source, target)
				}
			}
			return true, nil
		})
	}
	return recreateContainerWithProperties(ctx, cli, containerID, props)
}

// resolveBindingKind checks a /dev path handed to a binding setting and
// says how it is applied, honouring the chosen setting: "device" maps a node
// (a serial port is attached on demand when absent); "volume" and
// "device-bind" bind-mount a node as asked, with the cgroup rule its major
// needs, and mount a directory such as /dev/bus/usb as a tree with its rule.
// A stray directory or an absent device is refused with the fix. Paths
// outside /dev are plain volumes. It touches nothing, so a front end can
// call it before asking for a password.
func resolveBindingKind(kind, source string, add bool) (resolved string, rules []string, err error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return kind, nil, errors.New("host source is required")
	}
	if !strings.HasPrefix(source, "/dev/") {
		if kind == "device" {
			return kind, nil, nil
		}
		return "volume", nil, nil
	}
	if !add {
		if kind == "device" {
			return kind, nil, nil
		}
		return "volume", nil, nil
	}
	if devicePathsBelongToVM() {
		return resolveVMBindingKind(kind, source)
	}
	info, statErr := os.Stat(source)
	switch {
	case statErr == nil && info.Mode()&os.ModeDevice != 0:
		if kind == "device" {
			return "device", nil, nil
		}
		if rule := DeviceRuleFor(source); rule != "" {
			rules = append(rules, rule)
		}
		if IsSerialDevicePath(source) {
			rules = appendMissing(rules, SerialCgroupRules...)
		}
		return "volume", rules, nil
	case statErr == nil && info.IsDir():
		if IsStrayDeviceDir(source) {
			return kind, nil, fmt.Errorf("%s is an empty directory left where a device node belongs, not a device; remove it with rfswift host devclean and plug the device in", source)
		}
		if kind == "device" {
			return kind, nil, fmt.Errorf("%s is a directory: add it as a bind mount (it is mounted as a tree with its cgroup rule)", source)
		}
		if rule := devTreeRules[source]; rule != "" {
			rules = append(rules, rule)
		}
		return "volume", rules, nil
	case statErr == nil:
		return kind, nil, fmt.Errorf("%s is neither a device node nor a directory", source)
	case IsSerialDevicePath(source) && SerialHotplugSupported(GetEngine()):
		// Attached on demand once plugged in (serialhotplug.go).
		return "device", nil, nil
	default:
		return kind, nil, fmt.Errorf("%s is not present on this host: plug the device in first (a serial port can be added while unplugged)", source)
	}
}

// resolveVMBindingKind is resolveBindingKind for a /dev path that lives in
// the engine's VM (Lima): the VM is asked what the path is. When the VM
// cannot answer, the path is taken as named (a known tree such as
// /dev/bus/usb gets its rule, a serial port its rules) rather than refused:
// the engine reports a path that is really absent when the container starts.
func resolveVMBindingKind(kind, source string) (resolved string, rules []string, err error) {
	infos, ok := vmPathInfo([]string{source})
	if !ok {
		if kind == "device" {
			if _, tree := devTreeRules[source]; tree {
				return kind, nil, fmt.Errorf("%s is a device tree: add it as a bind mount (it is mounted with its cgroup rule)", source)
			}
			return "device", nil, nil
		}
		if rule := devTreeRules[source]; rule != "" {
			rules = append(rules, rule)
		}
		if IsSerialDevicePath(source) {
			rules = appendMissing(rules, SerialCgroupRules...)
		}
		return "volume", rules, nil
	}
	info := infos[source]
	switch {
	case info.Device:
		if kind == "device" {
			return "device", nil, nil
		}
		if rule := info.Rule(); rule != "" {
			rules = append(rules, rule)
		}
		if IsSerialDevicePath(source) {
			rules = appendMissing(rules, SerialCgroupRules...)
		}
		return "volume", rules, nil
	case info.IsDir:
		if kind == "device" {
			return kind, nil, fmt.Errorf("%s is a directory in the VM: add it as a bind mount (it is mounted as a tree with its cgroup rule)", source)
		}
		if rule := devTreeRules[source]; rule != "" {
			rules = append(rules, rule)
		}
		return "volume", rules, nil
	case info.Exists:
		return kind, nil, fmt.Errorf("%s is neither a device node nor a directory in the VM", source)
	default:
		return kind, nil, fmt.Errorf("%s is not present in the VM that runs the containers: attach the device first (rfswift macusb attach, or the USB button in the Workbench), then add it", source)
	}
}

func removeConfigDevicesAtTarget(config map[string]interface{}, target string) {
	host, ok := config["HostConfig"].(map[string]interface{})
	if !ok {
		return
	}
	devices, ok := host["Devices"].([]interface{})
	if !ok {
		return
	}
	out := make([]interface{}, 0, len(devices))
	for _, device := range devices {
		mapping, ok := device.(map[string]interface{})
		if ok && mapping["PathInContainer"] == target {
			continue
		}
		out = append(out, device)
	}
	host["Devices"] = out
}

func removeBindsAtTarget(items []string, target string) []string {
	out := items[:0]
	for _, item := range items {
		parts := strings.Split(item, ":")
		if len(parts) > 1 && parts[1] == target {
			continue
		}
		out = append(out, item)
	}
	return out
}

func removeDevicesAtTarget(items []DeviceMapping, target string) []DeviceMapping {
	out := items[:0]
	for _, item := range items {
		if item.PathInContainer != target {
			out = append(out, item)
		}
	}
	return out
}

func updatePropertyItems(items []string, entry, target string, add bool) []string {
	out := make([]string, 0, len(items)+1)
	found := false
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.Split(item, ":")
		matches := len(parts) > 1 && parts[1] == target
		if matches {
			found = true
			if !add {
				continue
			}
			item = entry
		}
		out = append(out, item)
	}
	if add && !found {
		out = append(out, entry)
	}
	return out
}

// RemoveContainer removes only the runtime container. Bind-mounted host data,
// including the Workbench workspace, is intentionally not deleted.
func RemoveContainer(containerID string) error {
	if strings.TrimSpace(containerID) == "" {
		return errors.New("container ID is required")
	}
	cli, err := NewEngineClient()
	if err != nil {
		return err
	}
	defer cli.Close()
	_, err = cli.ContainerRemove(context.Background(), containerID, client.ContainerRemoveOptions{Force: true})
	return err
}
