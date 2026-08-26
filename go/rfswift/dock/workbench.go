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
	if add {
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("host source %q is not accessible: %w", source, err)
		}
		if kind == "volume" && (strings.HasPrefix(source, "/dev/") || strings.HasPrefix(target, "/dev/")) {
			return errors.New("device paths must use the Mapped device setting, not Volume / bind mount")
		}
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
	updated := strings.Join(updatePropertyItems(strings.Split(props[key], separator), entry, target, add), separator)
	if updated == props[key] {
		return nil
	}
	props[key] = updated

	// Match the RF-Swift CLI: native Docker can rebind by editing its persisted
	// hostconfig/config.v2 files. Podman and Lima do not expose that storage, so
	// they retain the commit/recreate compatibility path below.
	if EngineSupportsDirectConfigEdit() {
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
				if add {
					host.Devices = append(host.Devices, DeviceMapping{PathOnHost: source, PathInContainer: target, CgroupPermissions: "rwm"})
					addDeviceMapping(configV2, source, target)
				}
			}
			return true, nil
		})
	}
	return recreateContainerWithProperties(ctx, cli, containerID, props)
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
