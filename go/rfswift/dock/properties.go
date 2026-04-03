package dock

/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	common "penthertz/rfswift/common"
	"penthertz/rfswift/tui"
)

// printContainerProperties displays a formatted summary table of container properties
// to stdout, including image status, version, bindings, devices, and capabilities.
//
//	in(1): context.Context ctx        request context used for Docker/Podman API calls
//	in(2): *client.Client cli         engine client used to query image status
//	in(3): string containerName       human-readable name of the container
//	in(4): map[string]string props    property map returned by getContainerProperties
//	in(5): string size                pre-formatted disk size string (e.g. "1.23 MB")
func printContainerProperties(ctx context.Context, cli *client.Client, containerName string, props map[string]string, size string) {
	// Determine if the image is up-to-date, obsolete, or custom
	repo, tag := parseImageName(props["ImageName"])
	isUpToDate, isCustom, err := checkImageStatus(ctx, cli, repo, tag)
	if err != nil {
		if err.Error() != "tag not found" {
			log.Printf("Error checking image status: %v", err)
		}
	}

	// Try to detect the version
	versionDisplay := ""
	architecture := getArchitecture()

	baseName, existingVersion := parseTagVersion(tag)
	if existingVersion != "" {
		versionDisplay = existingVersion
	} else if !common.Disconnected {
		fullImageName := fmt.Sprintf("%s:%s", repo, tag)
		localDigest := getLocalImageDigests(ctx, cli, fullImageName)
		if len(localDigest) > 0 {
			remoteVersionsByRepo := GetAllRemoteVersionsByRepo(architecture)
			if repoVersions, ok := remoteVersionsByRepo[repo]; ok {
				if versions, ok := repoVersions[baseName]; ok {
					matchedVersion := GetVersionForDigests(versions, localDigest)
					if matchedVersion != "" && matchedVersion != "latest" {
						versionDisplay = matchedVersion
					}
				}
			}
		}
	}

	// Build image status string
	imageNameWithVersion := props["ImageName"]
	if versionDisplay != "" {
		imageNameWithVersion = fmt.Sprintf("%s (v%s)", props["ImageName"], versionDisplay)
	}

	imageStatus := fmt.Sprintf("%s (Custom)", imageNameWithVersion)
	if common.Disconnected {
		imageStatus = fmt.Sprintf("%s (No network)", imageNameWithVersion)
	}
	imageStatusColor := tui.ColorWarning
	if !isCustom {
		if isUpToDate {
			imageStatus = fmt.Sprintf("%s (Up to date)", imageNameWithVersion)
			imageStatusColor = tui.ColorSuccess
		} else {
			imageStatus = fmt.Sprintf("%s (Obsolete)", imageNameWithVersion)
			imageStatusColor = tui.ColorDanger
		}
	}

	seccompValue := props["Seccomp"]
	if seccompValue == "" {
		seccompValue = "(Default)"
	}

	items := []tui.PropertyItem{
		{Key: "Container Name", Value: containerName},
		{Key: "X Display", Value: props["XDisplay"]},
		{Key: "Shell", Value: props["Shell"]},
		{Key: "Privileged Mode", Value: props["Privileged"]},
		{Key: "Network Mode", Value: func() string {
			if d := props["NetworkModeDisplay"]; d != "" {
				return d
			}
			return props["NetworkMode"]
		}()},
		{Key: "NAT Subnet", Value: props["NATSubnet"]},
		{Key: "Exposed Ports", Value: props["ExposedPorts"]},
		{Key: "Port Bindings", Value: props["PortBindings"]},
		{Key: "Image Name", Value: imageStatus, ValueColor: imageStatusColor},
		{Key: "Size on Disk", Value: size},
		{Key: "Bindings", Value: props["Bindings"]},
		{Key: "Extra Hosts", Value: props["ExtraHosts"]},
		{Key: "Devices", Value: props["Devices"]},
		{Key: "Capabilities", Value: props["Caps"]},
		{Key: "Seccomp profile", Value: seccompValue},
		{Key: "Cgroup rules", Value: props["Cgroups"]},
		{Key: "Ulimits", Value: props["Ulimits"]},
	}

	tui.RenderPropertySheet("🧊 Container Summary", tui.ColorPrimary, items)
}

// getDisplayImageName returns the display image name for a container,
// preferring the org.rfswift.original_image label set during recreation
// over the raw Config.Image field.
//
//	in(1): types.ContainerJSON containerJSON   full container inspection result
//	out:   string                              image name suitable for display
func getDisplayImageName(containerJSON types.ContainerJSON) string {
	// Check for original image label first (set during recreation)
	if label, ok := containerJSON.Config.Labels["org.rfswift.original_image"]; ok && label != "" {
		return label
	}
	return containerJSON.Config.Image
}

// getExposedPortsFromLabel returns the exposed ports string from a container's
// org.rfswift.exposed_ports label, falling back to the live ExposedPorts map
// when the label is absent. The special label value "none" is treated as empty.
//
//	in(1): types.ContainerJSON containerJSON   full container inspection result
//	out:   string                              comma-separated exposed ports, or empty string
func getExposedPortsFromLabel(containerJSON types.ContainerJSON) string {
	if label, ok := containerJSON.Config.Labels["org.rfswift.exposed_ports"]; ok {
		if label == "none" {
			return ""
		}
		return label
	}
	return convertExposedPortsToString(containerJSON.Config.ExposedPorts)
}

// getContainerProperties inspects a container and returns a map of its key
// runtime properties such as display, shell, network mode, bindings, devices,
// capabilities, and image information.
//
//	in(1): context.Context ctx    request context
//	in(2): *client.Client cli     engine client used to inspect the container
//	in(3): string containerID     container ID or name to inspect
//	out:   map[string]string      property map keyed by property name
//	out:   error                  non-nil if the inspect or image lookup fails
func getContainerProperties(ctx context.Context, cli *client.Client, containerID string) (map[string]string, error) {
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	// Extract the DISPLAY environment variable value
	var xdisplay string
	for _, env := range containerJSON.Config.Env {
		if strings.HasPrefix(env, "DISPLAY=") {
			xdisplay = strings.TrimPrefix(env, "DISPLAY=")
			break
		}
	}

	// Get the image details to find the size
	imageInfo, _, err := cli.ImageInspectWithRaw(ctx, containerJSON.Image)
	if err != nil {
		return nil, err
	}
	imageSize := fmt.Sprintf("%.2f MB", float64(imageInfo.Size)/1024/1024)

	cgroupRules := strings.Join(containerJSON.HostConfig.DeviceCgroupRules, ",")
	if cgroupRules == "" {
		if label, ok := containerJSON.Config.Labels["org.rfswift.cgroup_rules"]; ok {
			cgroupRules = label
		}
	}

	props := map[string]string{
		"XDisplay":     xdisplay,
		"Shell":        containerJSON.Path,
		"Privileged":   fmt.Sprintf("%v", containerJSON.HostConfig.Privileged),
		"NetworkMode":  string(containerJSON.HostConfig.NetworkMode),
		"ExposedPorts": getExposedPortsFromLabel(containerJSON),
		"PortBindings": convertPortBindingsToRoundTrip(containerJSON.HostConfig.PortBindings),
		"ImageName":    getDisplayImageName(containerJSON),
		"ImageHash":    imageInfo.ID,
		"Bindings":     strings.Join(containerJSON.HostConfig.Binds, ";;"),
		"ExtraHosts":   strings.Join(containerJSON.HostConfig.ExtraHosts, ","),
		"Size":         imageSize,
		"Devices":      convertDevicesToString(containerJSON.HostConfig.Devices),
		"Caps":         convertCapsToString(containerJSON.HostConfig.CapAdd),
		"Seccomp":      convertSecurityOptToString(containerJSON.HostConfig.SecurityOpt),
		"Cgroups":      cgroupRules,
	}

	// NAT subnet from label
	if natSubnet, ok := containerJSON.Config.Labels["org.rfswift.nat_subnet"]; ok && natSubnet != "" {
		props["NATSubnet"] = natSubnet
		// Keep the real network name for recreation, add friendly display name
		props["NetworkModeDisplay"] = "nat"
	}

	// Get ulimits
    var ulimitStrs []string
    for _, ulimit := range containerJSON.HostConfig.Ulimits {
        if ulimit.Soft == ulimit.Hard {
            if ulimit.Soft == -1 {
                ulimitStrs = append(ulimitStrs, fmt.Sprintf("%s=unlimited", ulimit.Name))
            } else {
                ulimitStrs = append(ulimitStrs, fmt.Sprintf("%s=%d", ulimit.Name, ulimit.Soft))
            }
        } else {
            softStr := fmt.Sprintf("%d", ulimit.Soft)
            hardStr := fmt.Sprintf("%d", ulimit.Hard)
            if ulimit.Soft == -1 {
                softStr = "unlimited"
            }
            if ulimit.Hard == -1 {
                hardStr = "unlimited"
            }
            ulimitStrs = append(ulimitStrs, fmt.Sprintf("%s=%s:%s", ulimit.Name, softStr, hardStr))
        }
    }
    props["Ulimits"] = strings.Join(ulimitStrs, ",")

	return props, nil
}

// UpdateMountBinding adds or removes a bind-mount from a container. On Docker it
// edits hostconfig.json and config.v2.json on disk then restarts the daemon; on
// Podman it recreates the container with the updated bind list. Windows is not
// supported and will cause the process to exit.
//
//	in(1): string containerName   name of the target container
//	in(2): string source          host-side path of the mount; defaults to target when empty
//	in(3): string target          container-side mount destination path
//	in(4): bool add               true to add the binding, false to remove it
func UpdateMountBinding(containerName string, source string, target string, add bool) {
	var timeout = 10

	if source == "" {
		source = target
		common.PrintWarningMessage(fmt.Sprintf("Source is empty. Defaulting source to target: %s", target))
	}

	// Check if source (host mount point) exists when adding a new binding
	if add {
		if _, err := os.Stat(source); os.IsNotExist(err) {
			common.PrintErrorMessage(fmt.Errorf("host mount point does not exist: %s", source))
			common.PrintInfoMessage("Please create the directory first or check the path")
			os.Exit(1)
		} else if err != nil {
			common.PrintErrorMessage(fmt.Errorf("error checking host mount point: %v", err))
			os.Exit(1)
		}
		common.PrintSuccessMessage(fmt.Sprintf("Verified host mount point exists: %s", source))
	}

	ctx := context.Background()

	common.PrintInfoMessage("Fetching container ID...")
	containerID := getContainerIDByName(ctx, containerName)
	if containerID == "" {
		common.PrintErrorMessage(fmt.Errorf("container %s not found", containerName))
		os.Exit(1)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Container ID: %s", containerID))

	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error when instantiating a client"))
		os.Exit(1)
	}

	// Stop the container
	common.PrintInfoMessage("Stopping the container...")
	if err := showLoadingIndicator(ctx, func() error {
		return cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	}, "Stopping the container..."); err != nil {
		common.PrintErrorMessage(fmt.Errorf("Failed to stop the container gracefully: %v", err))
		os.Exit(1)
	}

	// Check if the container is still running
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error inspecting container: %v", err))
		os.Exit(1)
	}
	if containerJSON.State.Running {
		common.PrintWarningMessage("Container is still running. Forcing stop...")
		err = cli.ContainerKill(ctx, containerID, "SIGKILL")
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("Failed to force stop the container: %v", err))
			os.Exit(1)
		}
		common.PrintSuccessMessage("Container forcibly stopped.")
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Container '%s' stopped", containerID))
	}

	newMount := fmt.Sprintf("%s:%s", source, target)

	// ─── Engine-aware code path ────────────────────────────────────────
	//
	// Docker:  edit hostconfig.json + config.v2.json on disk, restart daemon
	// Podman:  recreate the container with updated bind mounts (no direct edit)
	//
	if !EngineSupportsDirectConfigEdit() {
		// ── Podman path: container recreation ──────────────────────────
		common.PrintInfoMessage(fmt.Sprintf("%s does not support direct config editing — using container recreation", GetEngine().Name()))

		// Get current container config for recreation
		inspectData, err := cli.ContainerInspect(ctx, containerID)
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
			os.Exit(1)
		}

		// Update binds (use prefix matching for Podman — binds may have
		// trailing options like ":rw,rprivate,nosuid,rbind")
		currentBinds := inspectData.HostConfig.Binds
		if add {
			if !bindExistsByPrefix(currentBinds, newMount) {
				currentBinds = append(currentBinds, newMount)
				common.PrintSuccessMessage(fmt.Sprintf("Adding mount: %s", newMount))
			} else {
				common.PrintWarningMessage("Mount already exists.")
				return
			}
		} else {
			currentBinds = removeBindByPrefix(currentBinds, newMount)
			common.PrintSuccessMessage(fmt.Sprintf("Removing mount: %s", newMount))
		}

		// Recreate container with updated binds
		if err := recreateContainerWithUpdatedBinds(ctx, cli, containerName, containerID, inspectData, currentBinds); err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to recreate container: %v", err))
			os.Exit(1)
		}

		common.PrintSuccessMessage("Container recreated with updated mount bindings.")
		return
	}

	// ── Docker path: direct config file editing ───────────────────────
	common.PrintInfoMessage("Determining hostconfig.json path...")
	hostConfigPath, err := EngineGetHostConfigPath(containerID)
	if err != nil {
		common.PrintErrorMessage(err)
		os.Exit(1)
	}
	common.PrintSuccessMessage(fmt.Sprintf("HostConfig path: %s", hostConfigPath))

	engine := GetEngine()
	common.PrintInfoMessage("Loading hostconfig.json...")
	var hostConfig HostConfigFull
	if err := engineLoadJSON(engine, hostConfigPath, &hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("HostConfig loaded successfully.")

	// Load and update config.v2.json
	common.PrintInfoMessage("Determining config.v2.json path...")
	configV2Path := strings.Replace(hostConfigPath, "hostconfig.json", "config.v2.json", 1)
	common.PrintInfoMessage(fmt.Sprintf("Loading config.v2.json from: %s", configV2Path))
	var configV2 map[string]interface{}
	if err := engineLoadJSON(engine, configV2Path, &configV2); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load config.v2.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("config.v2.json loaded successfully.")

	// Update mounts in both files
	common.PrintInfoMessage("Updating mounts...")
	if add {
		if !ocontains(hostConfig.Binds, newMount) {
			hostConfig.Binds = append(hostConfig.Binds, newMount)
			addMountPoint(configV2, source, target)
			common.PrintSuccessMessage(fmt.Sprintf("Added mount: %s", newMount))
		} else {
			common.PrintWarningMessage("Mount already exists.")
		}
	} else {
		hostConfig.Binds = removeFromSlice(hostConfig.Binds, newMount)
		removeMountPoint(configV2, target)
		common.PrintSuccessMessage(fmt.Sprintf("Removed mount: %s", newMount))
	}

	// Save changes
	common.PrintInfoMessage("Saving updated hostconfig.json...")
	if err := engineSaveJSON(engine, hostConfigPath, hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("hostconfig.json updated successfully.")

	common.PrintInfoMessage("Saving updated config.v2.json...")
	if err := engineSaveJSON(engine, configV2Path, configV2); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save config.v2.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("config.v2.json updated successfully.")

	// Restart the engine service
	engineName := GetEngine().Name()
	if err := showLoadingIndicator(ctx, func() error {
		return EngineRestartService()
	}, fmt.Sprintf("Restarting %s service...", engineName)); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to restart %s service: %v", engineName, err))
		os.Exit(1)
	}
	common.PrintSuccessMessage(fmt.Sprintf("%s service restarted successfully.", engineName))
}

// addMountPoint inserts a bind-mount entry into the MountPoints section of a
// config.v2.json structure represented as a generic map.
//
//	in(1): map[string]interface{} config   parsed config.v2.json document (mutated in place)
//	in(2): string source                   host-side absolute path of the mount source
//	in(3): string target                   container-side absolute path of the mount destination
func addMountPoint(config map[string]interface{}, source string, target string) {
	mountPoints, ok := config["MountPoints"].(map[string]interface{})
	if !ok {
		mountPoints = make(map[string]interface{})
		config["MountPoints"] = mountPoints
	}

	mountPoints[target] = map[string]interface{}{
		"Source":      source,
		"Destination": target,
		"RW":          true,
		"Type":        "bind",
		"Propagation": "rprivate",
		"Spec": map[string]string{
			"Type":   "bind",
			"Source": source,
			"Target": target,
		},
		"SkipMountpointCreation": false,
	}
}

// removeMountPoint deletes the mount-point entry for the given container-side
// destination from the MountPoints section of a config.v2.json structure.
//
//	in(1): map[string]interface{} config   parsed config.v2.json document (mutated in place)
//	in(2): string target                   container-side destination path to remove
func removeMountPoint(config map[string]interface{}, target string) {
	mountPoints, ok := config["MountPoints"].(map[string]interface{})
	if !ok {
		return
	}

	delete(mountPoints, target)
}

// UpdateDeviceBinding adds or removes a device mapping from a container. On Docker
// it edits hostconfig.json and config.v2.json on disk then restarts the daemon;
// on Podman it recreates the container with the updated device list. Windows is
// not supported and will cause the process to exit.
//
//	in(1): string containerName      name of the target container
//	in(2): string deviceHost         host-side device path; defaults to deviceContainer when empty
//	in(3): string deviceContainer    container-side device path
//	in(4): bool add                  true to add the device mapping, false to remove it
func UpdateDeviceBinding(containerName string, deviceHost string, deviceContainer string, add bool) {
	var timeout = 10 // Stop timeout

	if deviceHost == "" {
		deviceHost = deviceContainer
		common.PrintWarningMessage(fmt.Sprintf("Host device path is empty. Defaulting to container device path: %s", deviceContainer))
	}

	ctx := context.Background()

	common.PrintInfoMessage("Fetching container ID...")
	containerID := getContainerIDByName(ctx, containerName)
	if containerID == "" {
		common.PrintErrorMessage(fmt.Errorf("container %s not found", containerName))
		os.Exit(1)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Container ID: %s", containerID))

	// Stop the container
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error when instantiating a client"))
		os.Exit(1)
	}
	common.PrintInfoMessage("Stopping the container...")

	// Attempt graceful stop
	if err := showLoadingIndicator(ctx, func() error {
		return cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	}, "Stopping the container..."); err != nil {
		common.PrintErrorMessage(fmt.Errorf("Failed to stop the container gracefully: %v", err))
		os.Exit(1)
	}

	// Check if the container is still running
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("Error inspecting container: %v", err))
		os.Exit(1)
	}
	if containerJSON.State.Running {
		common.PrintWarningMessage("Container is still running. Forcing stop...")
		err = cli.ContainerKill(ctx, containerID, "SIGKILL")
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("Failed to force stop the container: %v", err))
			os.Exit(1)
		}
		common.PrintSuccessMessage("Container forcibly stopped.")
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Container '%s' stopped", containerID))
	}

	// ─── Engine-aware code path ────────────────────────────────────────
	//
	// Docker:  edit hostconfig.json + config.v2.json on disk, restart daemon
	// Podman:  recreate the container with updated devices (no direct edit)
	//
	if !EngineSupportsDirectConfigEdit() {
		// ── Podman path: container recreation ──────────────────────────
		common.PrintInfoMessage(fmt.Sprintf("%s does not support direct config editing — using container recreation", GetEngine().Name()))

		// Get current container config for recreation
		inspectData, err := cli.ContainerInspect(ctx, containerID)
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
			os.Exit(1)
		}

		// Update devices in the inspected HostConfig
		currentDevices := inspectData.HostConfig.Devices
		if add {
			if !deviceExistsInSlice(currentDevices, deviceHost, deviceContainer) {
				newDevice := container.DeviceMapping{
					PathOnHost:        deviceHost,
					PathInContainer:   deviceContainer,
					CgroupPermissions: "rwm",
				}
				inspectData.HostConfig.Devices = append(currentDevices, newDevice)
				common.PrintSuccessMessage(fmt.Sprintf("Adding device: %s to %s", deviceHost, deviceContainer))
			} else {
				common.PrintWarningMessage("Device mapping already exists.")
				return
			}
		} else {
			inspectData.HostConfig.Devices = removeDeviceMappingFromSlice(currentDevices, deviceHost, deviceContainer)
			common.PrintSuccessMessage(fmt.Sprintf("Removing device: %s from %s", deviceHost, deviceContainer))
		}

		// Recreate container — pass current binds unchanged, devices are
		// already updated in inspectData.HostConfig.Devices
		if err := recreateContainerWithUpdatedBinds(ctx, cli, containerName, containerID, inspectData, inspectData.HostConfig.Binds); err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to recreate container: %v", err))
			os.Exit(1)
		}

		common.PrintSuccessMessage("Container recreated with updated device bindings.")
		return
	}

	// ── Docker path: direct config file editing ───────────────────────

	// Load and update hostconfig.json
	common.PrintInfoMessage("Determining hostconfig.json path...")
	hostConfigPath, err := GetHostConfigPath(containerID)
	if err != nil {
		common.PrintErrorMessage(err)
		os.Exit(1)
	}
	common.PrintSuccessMessage(fmt.Sprintf("HostConfig path: %s", hostConfigPath))

	engine := GetEngine()
	common.PrintInfoMessage("Loading hostconfig.json...")
	var hostConfig HostConfigFull
	if err := engineLoadJSON(engine, hostConfigPath, &hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("HostConfig loaded successfully.")

	// Load and update config.v2.json
	common.PrintInfoMessage("Determining config.v2.json path...")
	configV2Path := strings.Replace(hostConfigPath, "hostconfig.json", "config.v2.json", 1)
	common.PrintInfoMessage(fmt.Sprintf("Loading config.v2.json from: %s", configV2Path))
	var configV2 map[string]interface{}
	if err := engineLoadJSON(engine, configV2Path, &configV2); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load config.v2.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("config.v2.json loaded successfully.")

	// Update devices in both files
	common.PrintInfoMessage("Updating devices...")
	if add {
		if !deviceExists(hostConfig.Devices, deviceHost, deviceContainer) {
			newDevice := DeviceMapping{
				PathOnHost:        deviceHost,
				PathInContainer:   deviceContainer,
				CgroupPermissions: "rwm", // Default to read, write, mknod permissions
			}
			hostConfig.Devices = append(hostConfig.Devices, newDevice)
			addDeviceMapping(configV2, deviceHost, deviceContainer)
			common.PrintSuccessMessage(fmt.Sprintf("Added device: %s to %s", deviceHost, deviceContainer))
		} else {
			common.PrintWarningMessage("Device mapping already exists.")
		}
	} else {
		hostConfig.Devices = removeDeviceFromSlice(hostConfig.Devices, deviceHost, deviceContainer)
		removeDeviceMapping(configV2, deviceHost, deviceContainer)
		common.PrintSuccessMessage(fmt.Sprintf("Removed device: %s from %s", deviceHost, deviceContainer))
	}

	// Save changes
	common.PrintInfoMessage("Saving updated hostconfig.json...")
	if err := engineSaveJSON(engine, hostConfigPath, hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("hostconfig.json updated successfully.")

	common.PrintInfoMessage("Saving updated config.v2.json...")
	if err := engineSaveJSON(engine, configV2Path, configV2); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save config.v2.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("config.v2.json updated successfully.")

	// Restart the container
	if err := showLoadingIndicator(ctx, func() error {
		return RestartContainerService()
	}, "Restarting container engine service..."); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to restart container engine service: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("Container engine service restarted successfully.")
}

// deviceExistsInSlice reports whether a device mapping with the given host and
// container paths already exists in the slice. It operates on the Docker SDK's
// container.DeviceMapping type and is used in the Podman recreation path.
//
//	in(1): []container.DeviceMapping devices   slice of current device mappings
//	in(2): string hostPath                     host-side device path to search for
//	in(3): string containerPath                container-side device path to search for
//	out:   bool                                true if the mapping is already present
func deviceExistsInSlice(devices []container.DeviceMapping, hostPath, containerPath string) bool {
	for _, d := range devices {
		if d.PathOnHost == hostPath && d.PathInContainer == containerPath {
			return true
		}
	}
	return false
}

// removeDeviceMappingFromSlice returns a new slice with the device mapping
// identified by hostPath and containerPath removed. It operates on the Docker
// SDK's container.DeviceMapping type and is used in the Podman recreation path.
//
//	in(1): []container.DeviceMapping devices   original slice of device mappings
//	in(2): string hostPath                     host-side device path of the entry to remove
//	in(3): string containerPath                container-side device path of the entry to remove
//	out:   []container.DeviceMapping           new slice with the matching entry omitted
func removeDeviceMappingFromSlice(devices []container.DeviceMapping, hostPath, containerPath string) []container.DeviceMapping {
	var result []container.DeviceMapping
	for _, d := range devices {
		if d.PathOnHost == hostPath && d.PathInContainer == containerPath {
			continue
		}
		result = append(result, d)
	}
	return result
}

// directEditContainer is a helper that stops a Docker container, loads its
// hostconfig.json and config.v2.json, calls the mutate function to apply
// changes, saves both files, and restarts the Docker service.
// The mutate function receives pointers to both config structs and returns
// (changed bool, err error). If changed is false, no files are saved and
// the service is not restarted.
func directEditContainer(ctx context.Context, cli *client.Client, containerID string, containerName string, mutate func(*HostConfigFull, map[string]interface{}) (bool, error)) error {
	timeout := 10

	// Stop the container
	common.PrintInfoMessage("Stopping the container...")
	if err := showLoadingIndicator(ctx, func() error {
		return cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	}, "Stopping the container..."); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to stop container: %v", err))
		return fmt.Errorf("failed to stop container: %v", err)
	}

	// Ensure container is stopped and get full ID
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("error inspecting container: %v", err))
		return fmt.Errorf("error inspecting container: %v", err)
	}
	if containerJSON.State.Running {
		common.PrintWarningMessage("Container is still running. Forcing stop...")
		if err := cli.ContainerKill(ctx, containerID, "SIGKILL"); err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to force stop container: %v", err))
			return fmt.Errorf("failed to force stop container: %v", err)
		}
	}
	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' stopped", containerName))

	// Use the full container ID for filesystem path lookup
	fullID := containerJSON.ID

	// Load hostconfig.json
	engine := GetEngine()
	hostConfigPath, err := EngineGetHostConfigPath(fullID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to locate hostconfig.json: %v", err))
		return err
	}
	common.PrintInfoMessage(fmt.Sprintf("Loading hostconfig.json from: %s", hostConfigPath))
	var hostConfig HostConfigFull
	if err := engineLoadJSON(engine, hostConfigPath, &hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load hostconfig.json: %v", err))
		return err
	}

	// Load config.v2.json
	configV2Path := strings.Replace(hostConfigPath, "hostconfig.json", "config.v2.json", 1)
	var configV2 map[string]interface{}
	if err := engineLoadJSON(engine, configV2Path, &configV2); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load config.v2.json: %v", err))
		return err
	}

	// Apply mutations
	changed, err := mutate(&hostConfig, configV2)
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	if !changed {
		return nil
	}

	// Save both files
	if err := engineSaveJSON(engine, hostConfigPath, hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save hostconfig.json: %v", err))
		return err
	}
	common.PrintSuccessMessage("hostconfig.json updated successfully.")

	if err := engineSaveJSON(engine, configV2Path, configV2); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save config.v2.json: %v", err))
		return err
	}
	common.PrintSuccessMessage("config.v2.json updated successfully.")

	// Restart the engine service
	engineName := GetEngine().Name()
	if err := showLoadingIndicator(ctx, func() error {
		return EngineRestartService()
	}, fmt.Sprintf("Restarting %s service...", engineName)); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to restart %s service: %v", engineName, err))
		return err
	}
	common.PrintSuccessMessage(fmt.Sprintf("%s service restarted successfully.", engineName))
	return nil
}

// UpdateCapability adds or removes a Linux capability from a container.
// On Docker, it directly edits hostconfig.json. On Podman, it falls back
// to container recreation.
//
//	in(1): string containerID    container ID or name to modify
//	in(2): string capability     Linux capability name (e.g. "NET_ADMIN")
//	in(3): bool add              true to add the capability, false to remove it
//	out:   error                 non-nil if the inspect, property fetch, or recreation fails
func UpdateCapability(containerID string, capability string, add bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	if !EngineSupportsDirectConfigEdit() {
		// Podman path: fall back to container recreation
		common.PrintInfoMessage(fmt.Sprintf("%s does not support direct config editing — using container recreation", GetEngine().Name()))
		props, err := getContainerProperties(ctx, cli, containerID)
		if err != nil {
			return err
		}
		var capabilities []string
		if props["Caps"] != "" {
			capabilities = strings.Split(props["Caps"], ",")
		}
		if add {
			for _, cap := range capabilities {
				if strings.TrimSpace(cap) == capability {
					common.PrintInfoMessage(fmt.Sprintf("Capability '%s' already exists in container '%s'", capability, containerName))
					return nil
				}
			}
			capabilities = append(capabilities, capability)
		} else {
			newCaps := []string{}
			found := false
			for _, cap := range capabilities {
				if strings.TrimSpace(cap) != capability {
					newCaps = append(newCaps, cap)
				} else {
					found = true
				}
			}
			if !found {
				common.PrintWarningMessage(fmt.Sprintf("Capability '%s' not found in container '%s'", capability, containerName))
				return nil
			}
			capabilities = newCaps
		}
		props["Caps"] = strings.Join(capabilities, ",")
		return recreateContainerWithProperties(ctx, cli, containerID, props)
	}

	// Docker path: direct hostconfig.json edit
	return directEditContainer(ctx, cli, containerID, containerName, func(hostConfig *HostConfigFull, _ map[string]interface{}) (bool, error) {
		if add {
			for _, cap := range hostConfig.CapAdd {
				if strings.TrimSpace(cap) == capability {
					common.PrintInfoMessage(fmt.Sprintf("Capability '%s' already exists in container '%s'", capability, containerName))
					return false, nil
				}
			}
			hostConfig.CapAdd = append(hostConfig.CapAdd, capability)
			common.PrintSuccessMessage(fmt.Sprintf("Added capability: %s", capability))
		} else {
			newCaps := []string{}
			found := false
			for _, cap := range hostConfig.CapAdd {
				if strings.TrimSpace(cap) != capability {
					newCaps = append(newCaps, cap)
				} else {
					found = true
				}
			}
			if !found {
				common.PrintWarningMessage(fmt.Sprintf("Capability '%s' not found in container '%s'", capability, containerName))
				return false, nil
			}
			hostConfig.CapAdd = newCaps
			common.PrintSuccessMessage(fmt.Sprintf("Removed capability: %s", capability))
		}
		return true, nil
	})
}

// UpdateCgroupRule adds or removes a device cgroup rule from a container by
// recreating the container with the updated DeviceCgroupRules list via
// recreateContainerWithProperties.
//
//	in(1): string containerID   container ID or name to modify
//	in(2): string rule          cgroup rule string (e.g. "c 189:* rwm")
//	in(3): bool add             true to add the rule, false to remove it
//	out:   error                non-nil if the inspect, property fetch, or recreation fails
func UpdateCgroupRule(containerID string, rule string, add bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	if !EngineSupportsDirectConfigEdit() {
		common.PrintInfoMessage(fmt.Sprintf("%s does not support direct config editing — using container recreation", GetEngine().Name()))
		props, err := getContainerProperties(ctx, cli, containerID)
		if err != nil {
			return err
		}
		var cgroupRules []string
		if props["Cgroups"] != "" {
			cgroupRules = strings.Split(props["Cgroups"], ",")
		}
		if add {
			for _, r := range cgroupRules {
				if strings.TrimSpace(r) == rule {
					common.PrintInfoMessage(fmt.Sprintf("Cgroup rule '%s' already exists in container '%s'", rule, containerName))
					return nil
				}
			}
			cgroupRules = append(cgroupRules, rule)
		} else {
			newRules := []string{}
			found := false
			for _, r := range cgroupRules {
				if strings.TrimSpace(r) != rule {
					newRules = append(newRules, r)
				} else {
					found = true
				}
			}
			if !found {
				common.PrintWarningMessage(fmt.Sprintf("Cgroup rule '%s' not found in container '%s'", rule, containerName))
				return nil
			}
			cgroupRules = newRules
		}
		props["Cgroups"] = strings.Join(cgroupRules, ",")
		return recreateContainerWithProperties(ctx, cli, containerID, props)
	}

	// Docker path: direct hostconfig.json edit
	return directEditContainer(ctx, cli, containerID, containerName, func(hostConfig *HostConfigFull, _ map[string]interface{}) (bool, error) {
		if add {
			for _, r := range hostConfig.DeviceCgroupRules {
				if strings.TrimSpace(r) == rule {
					common.PrintInfoMessage(fmt.Sprintf("Cgroup rule '%s' already exists in container '%s'", rule, containerName))
					return false, nil
				}
			}
			hostConfig.DeviceCgroupRules = append(hostConfig.DeviceCgroupRules, rule)
			common.PrintSuccessMessage(fmt.Sprintf("Added cgroup rule: %s", rule))
		} else {
			newRules := []string{}
			found := false
			for _, r := range hostConfig.DeviceCgroupRules {
				if strings.TrimSpace(r) != rule {
					newRules = append(newRules, r)
				} else {
					found = true
				}
			}
			if !found {
				common.PrintWarningMessage(fmt.Sprintf("Cgroup rule '%s' not found in container '%s'", rule, containerName))
				return false, nil
			}
			hostConfig.DeviceCgroupRules = newRules
			common.PrintSuccessMessage(fmt.Sprintf("Removed cgroup rule: %s", rule))
		}
		return true, nil
	})
}

// UpdateExposedPort adds or removes an exposed port declaration from a container
// by recreating the container with the updated ExposedPorts set via
// recreateContainerWithProperties.
//
//	in(1): string containerID   container ID or name to modify
//	in(2): string port          port specification in "number/proto" form (e.g. "8080/tcp")
//	in(3): bool add             true to expose the port, false to un-expose it
//	out:   error                non-nil if the inspect, property fetch, or recreation fails
func UpdateExposedPort(containerID string, port string, add bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	if !EngineSupportsDirectConfigEdit() {
		common.PrintInfoMessage(fmt.Sprintf("%s does not support direct config editing — using container recreation", GetEngine().Name()))
		props, err := getContainerProperties(ctx, cli, containerID)
		if err != nil {
			return err
		}
		var exposedPorts []string
		if props["ExposedPorts"] != "" {
			exposedPorts = strings.Split(props["ExposedPorts"], ",")
			for i := range exposedPorts {
				exposedPorts[i] = strings.TrimSpace(exposedPorts[i])
			}
		}
		if add {
			for _, p := range exposedPorts {
				if p == port {
					common.PrintInfoMessage(fmt.Sprintf("Port '%s' already exposed in container '%s'", port, containerName))
					return nil
				}
			}
			exposedPorts = append(exposedPorts, port)
		} else {
			newPorts := []string{}
			found := false
			for _, p := range exposedPorts {
				if p != port {
					newPorts = append(newPorts, p)
				} else {
					found = true
				}
			}
			if !found {
				common.PrintWarningMessage(fmt.Sprintf("Port '%s' not found in container '%s'", port, containerName))
				return nil
			}
			exposedPorts = newPorts
		}
		props["ExposedPorts"] = strings.Join(exposedPorts, ",")
		return recreateContainerWithProperties(ctx, cli, containerID, props)
	}

	// Docker path: edit config.v2.json (ExposedPorts lives in the container config, not hostconfig)
	// Normalize port format: ensure it has a protocol suffix
	if !strings.Contains(port, "/") {
		port = port + "/tcp"
	}

	return directEditContainer(ctx, cli, containerID, containerName, func(_ *HostConfigFull, configV2 map[string]interface{}) (bool, error) {
		// ExposedPorts is in config.v2.json under Config.ExposedPorts
		configSection, _ := configV2["Config"].(map[string]interface{})
		if configSection == nil {
			configSection = map[string]interface{}{}
			configV2["Config"] = configSection
		}
		exposedPorts, _ := configSection["ExposedPorts"].(map[string]interface{})
		if exposedPorts == nil {
			exposedPorts = map[string]interface{}{}
		}

		if add {
			if _, exists := exposedPorts[port]; exists {
				common.PrintInfoMessage(fmt.Sprintf("Port '%s' already exposed in container '%s'", port, containerName))
				return false, nil
			}
			exposedPorts[port] = map[string]interface{}{}
			common.PrintSuccessMessage(fmt.Sprintf("Exposed port: %s", port))
		} else {
			if _, exists := exposedPorts[port]; !exists {
				common.PrintWarningMessage(fmt.Sprintf("Port '%s' not found in container '%s'", port, containerName))
				return false, nil
			}
			delete(exposedPorts, port)
			common.PrintSuccessMessage(fmt.Sprintf("Removed exposed port: %s", port))
		}
		configSection["ExposedPorts"] = exposedPorts
		return true, nil
	})
}

// UpdatePortBinding adds or removes a host-to-container port binding from a
// container by recreating the container with the updated PortBindings map via
// recreateContainerWithProperties.
//
//	in(1): string containerID   container ID or name to modify
//	in(2): string binding       port binding in "hostPort:containerPort/proto" form
//	in(3): bool add             true to add the binding, false to remove it
//	out:   error                non-nil if the inspect, property fetch, or recreation fails
func UpdatePortBinding(containerID string, binding string, add bool) error {
	// Normalize binding to internal format (containerPort/proto:hostPort)
	binding = normalizePortBinding(binding)

	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	if !EngineSupportsDirectConfigEdit() {
		common.PrintInfoMessage(fmt.Sprintf("%s does not support direct config editing — using container recreation", GetEngine().Name()))
		props, err := getContainerProperties(ctx, cli, containerID)
		if err != nil {
			return err
		}
		var portBindings []string
		if props["PortBindings"] != "" {
			portBindings = strings.Split(props["PortBindings"], ";;")
			for i := range portBindings {
				portBindings[i] = strings.TrimSpace(portBindings[i])
			}
		}
		if add {
			for _, b := range portBindings {
				if b == binding {
					common.PrintInfoMessage(fmt.Sprintf("Port binding '%s' already exists in container '%s'", binding, containerName))
					return nil
				}
			}
			portBindings = append(portBindings, binding)
		} else {
			newBindings := []string{}
			found := false
			for _, b := range portBindings {
				if b != binding {
					newBindings = append(newBindings, b)
				} else {
					found = true
				}
			}
			if !found {
				common.PrintWarningMessage(fmt.Sprintf("Port binding '%s' not found in container '%s'", binding, containerName))
				return nil
			}
			portBindings = newBindings
		}
		props["PortBindings"] = strings.Join(portBindings, ";;")
		return recreateContainerWithProperties(ctx, cli, containerID, props)
	}

	// Docker path: direct hostconfig.json + config.v2.json edit
	// Parse the binding: format is "containerPort/proto:hostIP:hostPort" or "containerPort/proto:hostPort"
	parts := strings.SplitN(binding, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid port binding format: %s", binding)
	}
	containerPort := parts[0] // e.g., "8080/tcp"
	hostPart := parts[1]     // e.g., "127.0.0.1:8080" or "8080"

	// Parse host IP and port
	hostIP := ""
	hostPort := hostPart
	if colonIdx := strings.LastIndex(hostPart, ":"); colonIdx != -1 {
		hostIP = hostPart[:colonIdx]
		hostPort = hostPart[colonIdx+1:]
	}

	if !strings.Contains(containerPort, "/") {
		containerPort = containerPort + "/tcp"
	}

	return directEditContainer(ctx, cli, containerID, containerName, func(hostConfig *HostConfigFull, configV2 map[string]interface{}) (bool, error) {
		if hostConfig.PortBindings == nil {
			hostConfig.PortBindings = map[string][]PortBinding{}
		}

		if add {
			newBinding := PortBinding{HostIP: hostIP, HostPort: hostPort}
			existing := hostConfig.PortBindings[containerPort]
			for _, b := range existing {
				if b.HostIP == hostIP && b.HostPort == hostPort {
					common.PrintInfoMessage(fmt.Sprintf("Port binding '%s' already exists in container '%s'", binding, containerName))
					return false, nil
				}
			}
			hostConfig.PortBindings[containerPort] = append(existing, newBinding)

			// Also add to ExposedPorts in config.v2.json
			if configV2 != nil {
				configSection, _ := configV2["Config"].(map[string]interface{})
				if configSection != nil {
					exposedPorts, _ := configSection["ExposedPorts"].(map[string]interface{})
					if exposedPorts == nil {
						exposedPorts = map[string]interface{}{}
					}
					exposedPorts[containerPort] = map[string]interface{}{}
					configSection["ExposedPorts"] = exposedPorts
				}
			}

			common.PrintSuccessMessage(fmt.Sprintf("Added port binding: %s", binding))
		} else {
			existing := hostConfig.PortBindings[containerPort]
			newBindings := []PortBinding{}
			found := false
			for _, b := range existing {
				if b.HostIP == hostIP && b.HostPort == hostPort {
					found = true
				} else {
					newBindings = append(newBindings, b)
				}
			}
			if !found {
				common.PrintWarningMessage(fmt.Sprintf("Port binding '%s' not found in container '%s'", binding, containerName))
				return false, nil
			}
			if len(newBindings) == 0 {
				delete(hostConfig.PortBindings, containerPort)
				// Also remove from ExposedPorts in config.v2.json
				if configV2 != nil {
					configSection, _ := configV2["Config"].(map[string]interface{})
					if configSection != nil {
						exposedPorts, _ := configSection["ExposedPorts"].(map[string]interface{})
						if exposedPorts != nil {
							delete(exposedPorts, containerPort)
						}
					}
				}
			} else {
				hostConfig.PortBindings[containerPort] = newBindings
			}
			common.PrintSuccessMessage(fmt.Sprintf("Removed port binding: %s", binding))
		}
		return true, nil
	})
}

// recreateContainerWithProperties stops a container, commits its filesystem state
// to a temporary image, removes the original container, and creates a fresh
// replacement using the property overrides supplied in props. The replacement is
// started automatically and retains the original container name.
//
//	in(1): context.Context ctx          request context
//	in(2): *client.Client cli           engine client
//	in(3): string containerID           ID or name of the container to recreate
//	in(4): map[string]string props      property overrides (keys: Caps, Cgroups, ExposedPorts,
//	                                    PortBindings, Bindings, XDisplay, Shell, NetworkMode,
//	                                    Privileged, Devices, Seccomp, ExtraHosts, Ulimits)
//	out:   error                        non-nil if any step of the recreation process fails
func recreateContainerWithProperties(ctx context.Context, cli *client.Client, containerID string, props map[string]string) error {
	// Get fresh container info
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}

	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	common.PrintInfoMessage(fmt.Sprintf("Updating container '%s'", containerName))

	// Stop the container
	common.PrintInfoMessage("Stopping container...")
	timeout := 10
	if err := cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		// Container might already be stopped — not fatal
		common.PrintWarningMessage(fmt.Sprintf("Stop returned: %v (may already be stopped)", err))
	}

	// Determine the original image name
	originalImageName := containerJSON.Config.Image
	if label, ok := containerJSON.Config.Labels["org.rfswift.original_image"]; ok && label != "" {
		originalImageName = label
	}
	repo, tag := parseImageName(originalImageName)
	// Strip localhost/ prefix to avoid double-prefixing (localhost/localhost/...)
	repo = strings.TrimPrefix(repo, "localhost/")
	// Strip any existing _temp_YYYYMMDDHHMMSS suffix to avoid stacking
	if idx := strings.Index(tag, "_temp_"); idx != -1 {
		tag = tag[:idx]
	}

	// ── 1. Commit the container state to a temporary image ──
	tempImageTag := fmt.Sprintf("localhost/%s:%s_temp_%s", repo, tag, time.Now().Format("20060102150405"))
	common.PrintInfoMessage(fmt.Sprintf("Committing container state to temporary image: %s", tempImageTag))

	commitLabels := make(map[string]string)
	for k, v := range containerJSON.Config.Labels {
		commitLabels[k] = v
	}
	if props["ExposedPorts"] == "" {
		commitLabels["org.rfswift.exposed_ports"] = "none"
	} else {
		commitLabels["org.rfswift.exposed_ports"] = props["ExposedPorts"]
	}

	commitResp, err := cli.ContainerCommit(ctx, containerID, container.CommitOptions{
		Reference: tempImageTag,
		Comment:   "RF Swift: temporary image for container property update",
		Pause:     true,
		Config: &container.Config{
			ExposedPorts: ParseExposedPorts(props["ExposedPorts"]),
			Labels:       commitLabels,
		},
	})

	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to commit container: %v", err))
		return err
	}
	common.PrintSuccessMessage(fmt.Sprintf("Committed as: %s (ID: %s)", tempImageTag, commitResp.ID[:12]))

	// ── 2. Remove old container ──
	common.PrintInfoMessage("Removing old container...")
	if err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to remove container: %v", err))
		return err
	}
	common.PrintSuccessMessage("Old container removed.")

	// 2b. Clean up stale temp images (now unreferenced)
	cleanupStaleTempImages(ctx, cli, tempImageTag, repo, tag)

	// ── 3. Rebuild container config from inspected data + prop overrides ──


	// ── 1. Commit the container state to a temporary image ──
	bindings := []string{}
	if props["Bindings"] != "" {
	    bindings = strings.Split(props["Bindings"], ";;")
	}

	extrahosts := []string{}
	if props["ExtraHosts"] != "" {
		extrahosts = strings.Split(props["ExtraHosts"], ",")
	}

	// Rebuild environment — preserve ALL original env vars, update DISPLAY
	var dockerenv []string
	displaySet := false
	for _, env := range containerJSON.Config.Env {
		if strings.HasPrefix(env, "DISPLAY=") {
			if props["XDisplay"] != "" {
				dockerenv = append(dockerenv, fmt.Sprintf("DISPLAY=%s", props["XDisplay"]))
			} else {
				dockerenv = append(dockerenv, env) // keep original
			}
			displaySet = true
		} else {
			dockerenv = append(dockerenv, env)
		}
	}
	if !displaySet && props["XDisplay"] != "" {
		dockerenv = append(dockerenv, fmt.Sprintf("DISPLAY=%s", props["XDisplay"]))
	}

	exposedPorts := ParseExposedPorts(props["ExposedPorts"])
	bindedPorts := ParseBindedPorts(props["PortBindings"])
	devices := getDeviceMappingsFromString(props["Devices"])

	privileged := props["Privileged"] == "true"

	hostConfig := &container.HostConfig{
		NetworkMode:  container.NetworkMode(props["NetworkMode"]),
		Binds:        bindings,
		ExtraHosts:   extrahosts,
		PortBindings: bindedPorts,
		Privileged:   privileged,
	}

	// Handle ulimits
	if props["Ulimits"] != "" {
		hostConfig.Resources.Ulimits = parseUlimitsFromString(props["Ulimits"])
	}

	if !privileged {
		hostConfig.Devices = devices

		if props["Cgroups"] != "" {
			hostConfig.DeviceCgroupRules = strings.Split(props["Cgroups"], ",")
		}
		if props["Seccomp"] != "" && props["Seccomp"] != "(Default)" {
			hostConfig.SecurityOpt = []string{"seccomp=" + props["Seccomp"]}
		}
		if props["Caps"] != "" {
			hostConfig.CapAdd = strings.Split(props["Caps"], ",")
		}
	}

	// ── Restore cgroup rules from label if inspect returned empty ──
	if len(hostConfig.DeviceCgroupRules) == 0 {
		if label, ok := containerJSON.Config.Labels["org.rfswift.cgroup_rules"]; ok && label != "" {
			hostConfig.DeviceCgroupRules = strings.Split(label, ",")
		}
	}

	// Build labels — preserve existing + update tracking labels
	containerLabels := make(map[string]string)
	for k, v := range containerJSON.Config.Labels {
		containerLabels[k] = v
	}
	containerLabels["org.container.project"] = "rfswift"
	containerLabels["org.rfswift.original_image"] = originalImageName
	if len(hostConfig.DeviceCgroupRules) > 0 {
		containerLabels["org.rfswift.cgroup_rules"] = strings.Join(hostConfig.DeviceCgroupRules, ",")
	}

	if props["ExposedPorts"] == "" {
		containerLabels["org.rfswift.exposed_ports"] = "none"
	} else {
		containerLabels["org.rfswift.exposed_ports"] = props["ExposedPorts"]
	}

	// Determine shell
	shell := props["Shell"]
	if shell == "" {
		shell = containerJSON.Path
	}
	if shell == "" {
		shell = "/bin/bash"
	}

	containerConfig := &container.Config{
		Image:        tempImageTag, // ← use committed snapshot
		Cmd:          []string{shell},
		Env:          dockerenv,
		ExposedPorts: exposedPorts,
		OpenStdin:    true,
		StdinOnce:    false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Labels:       containerLabels,
	}

	// Preserve entrypoint
	if len(containerJSON.Config.Entrypoint) > 0 {
		containerConfig.Entrypoint = containerJSON.Config.Entrypoint
	}

	// ── Sanitize HostConfig for Podman cgroup v2 compat ──
	if !EngineSupportsDirectConfigEdit() {
		sanitizeHostConfigForPodman(hostConfig)
	}

	// ── 4. Create the new container ──
	common.PrintInfoMessage("Creating new container with updated configuration...")

	tempContainerName := fmt.Sprintf("%s_rfswift_tmp_%d", containerName, time.Now().UnixNano())

	var newContainerID string

	// Podman: use native CLI when cgroup rules are present
	// (Lima uses Docker inside the VM, so it goes through the compat API below)
	if len(hostConfig.DeviceCgroupRules) > 0 && GetEngine().Type() == EnginePodman && !IsRootlessPodman() {
		cid, err := podmanCreateViaCLI(tempContainerName, tempImageTag, containerConfig, hostConfig)
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to create container via Podman CLI: %v", err))
			// ── ROLLBACK ──
			rollbackContainer(ctx, cli, containerName, tempImageTag, containerJSON)
			return err
		}
		newContainerID = cid
	} else {

		// ── Rootless Podman: silently drop unsupported cgroup rules ────
		if IsRootlessPodman() && len(hostConfig.DeviceCgroupRules) > 0 {
			common.PrintWarningMessage("Rootless Podman: dropping device cgroup rules (not supported)")
			hostConfig.DeviceCgroupRules = nil
		}

		// ── Compat API path ──
		// CRITICAL: pass nil for networking and platform.
		// Podman's compat API rejects empty structs like &network.NetworkingConfig{}.
		resp, err := cli.ContainerCreate(ctx,
			containerConfig,
			hostConfig,
			nil, // networking — must be nil, NOT &network.NetworkingConfig{}
			nil, // platform
			tempContainerName,
		)
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to create new container: %v", err))
			// ── ROLLBACK ──
			rollbackContainer(ctx, cli, containerName, tempImageTag, containerJSON)
			return err
		}
		newContainerID = resp.ID
	}
	common.PrintSuccessMessage(fmt.Sprintf("New container created: %s", newContainerID[:12]))

	// ── 5. Rename temp container to original name ──
	common.PrintInfoMessage(fmt.Sprintf("Renaming container to '%s'...", containerName))
	if err := cli.ContainerRename(ctx, newContainerID, containerName); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to rename container: %v", err))
		return err
	}

	// ── 6. Start the new container ──
	common.PrintInfoMessage("Starting new container...")
	if err := cli.ContainerStart(ctx, newContainerID, container.StartOptions{}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to start new container: %v", err))
		return err
	}

	// ── 7. Clean up the temporary image ──
	// Docker allows removing an image tag while a container uses it (layers stay).
	// Podman does not — skip the attempt; cleanupStaleTempImages handles it next time.
	if GetEngine().Type() != EnginePodman {
		if _, err := cli.ImageRemove(ctx, tempImageTag, image.RemoveOptions{Force: false}); err != nil {
			common.PrintWarningMessage(fmt.Sprintf("Could not remove temp image '%s': %v (you can remove it manually)", tempImageTag, err))
		} else {
			common.PrintSuccessMessage(fmt.Sprintf("Cleaned up temporary image: %s", tempImageTag))
		}
	}

	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' updated successfully!", containerName))
	return nil
}

// rollbackContainer attempts to recreate a container from a previously committed
// temporary image when the primary creation step inside recreateContainerWithProperties
// fails. It logs recovery instructions if the rollback itself also fails.
//
//	in(1): context.Context ctx               request context
//	in(2): *client.Client cli                engine client
//	in(3): string containerName              original name to restore the container under
//	in(4): string tempImageTag               tag of the committed temporary image to recover from
//	in(5): types.ContainerJSON originalJSON  original container inspection data used to
//	                                         rebuild a minimal host and container config
func rollbackContainer(ctx context.Context, cli *client.Client, containerName string, tempImageTag string, originalJSON types.ContainerJSON) {
	common.PrintWarningMessage("Creation failed — attempting rollback from committed image...")

	// Try to create a basic container from the committed image with minimal config
	// (avoid the fields that may have caused the original failure)
	rollbackHostConfig := &container.HostConfig{
	    NetworkMode: originalJSON.HostConfig.NetworkMode,
	    Binds:       originalJSON.HostConfig.Binds,
	    Privileged:  originalJSON.HostConfig.Privileged,
	    CapAdd:      originalJSON.HostConfig.CapAdd,
	    SecurityOpt: originalJSON.HostConfig.SecurityOpt,
	}

	// Sanitize for Podman if needed
	if !EngineSupportsDirectConfigEdit() {
		sanitizeHostConfigForPodman(rollbackHostConfig)
	}

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:     tempImageTag,
			OpenStdin: true,
			Tty:       true,
			Labels:    originalJSON.Config.Labels,
			Env:       originalJSON.Config.Env,
		},
		rollbackHostConfig,
		nil, nil,
		containerName,
	)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("rollback also failed: %v", err))
		common.PrintWarningMessage(fmt.Sprintf("Your container state is preserved in image: %s", tempImageTag))
		common.PrintWarningMessage(fmt.Sprintf("Manual recovery: podman create --name %s -it %s", containerName, tempImageTag))
		return
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		common.PrintWarningMessage(fmt.Sprintf("Rollback container created but failed to start: %v", err))
		common.PrintWarningMessage(fmt.Sprintf("Try manually: podman start %s", containerName))
	} else {
		common.PrintSuccessMessage("Rollback successful — container restored with original configuration")
		common.PrintInfoMessage("The requested change was NOT applied. Please check the error above and retry.")
	}
}

// deviceExists reports whether a device mapping with the given host and container
// paths already exists in the local DeviceMapping slice (used in the Docker
// direct-config-edit path).
//
//	in(1): []DeviceMapping devices   slice of current device mappings
//	in(2): string hostPath           host-side device path to search for
//	in(3): string containerPath      container-side device path to search for
//	out:   bool                      true if the mapping is already present
func deviceExists(devices []DeviceMapping, hostPath string, containerPath string) bool {
	for _, device := range devices {
		if device.PathOnHost == hostPath && device.PathInContainer == containerPath {
			return true
		}
	}
	return false
}

// removeDeviceFromSlice returns a new slice with the local DeviceMapping entry
// identified by hostPath and containerPath removed (used in the Docker
// direct-config-edit path).
//
//	in(1): []DeviceMapping devices   original slice of device mappings
//	in(2): string hostPath           host-side device path of the entry to remove
//	in(3): string containerPath      container-side device path of the entry to remove
//	out:   []DeviceMapping           new slice with the matching entry omitted
func removeDeviceFromSlice(devices []DeviceMapping, hostPath string, containerPath string) []DeviceMapping {
	var result []DeviceMapping
	for _, device := range devices {
		if device.PathOnHost != hostPath || device.PathInContainer != containerPath {
			result = append(result, device)
		}
	}
	return result
}

// addDeviceMapping inserts a device mapping entry into the HostConfig.Devices
// array of a config.v2.json structure represented as a generic map. Duplicate
// entries (same hostPath and containerPath) are silently ignored.
//
//	in(1): map[string]interface{} config   parsed config.v2.json document (mutated in place)
//	in(2): string hostPath                 host-side device path to add
//	in(3): string containerPath            container-side device path to add
func addDeviceMapping(config map[string]interface{}, hostPath string, containerPath string) {
	// Check if "HostConfig" exists in the config
	hostConfig, ok := config["HostConfig"].(map[string]interface{})
	if !ok {
		// Create HostConfig if it doesn't exist
		hostConfig = make(map[string]interface{})
		config["HostConfig"] = hostConfig
	}

	// Get existing devices or create new devices array
	devices, ok := hostConfig["Devices"].([]interface{})
	if !ok {
		devices = make([]interface{}, 0)
	}

	// Create a new device mapping
	newDevice := map[string]interface{}{
		"PathOnHost":        hostPath,
		"PathInContainer":   containerPath,
		"CgroupPermissions": "rwm", // Default permissions
	}

	// Check if the device already exists
	exists := false
	for _, device := range devices {
		if deviceMap, ok := device.(map[string]interface{}); ok {
			if deviceMap["PathOnHost"] == hostPath && deviceMap["PathInContainer"] == containerPath {
				exists = true
				break
			}
		}
	}

	// Add the new device mapping if it doesn't exist
	if !exists {
		devices = append(devices, newDevice)
		hostConfig["Devices"] = devices
	}
}

// removeDeviceMapping deletes the device mapping entry identified by hostPath
// and containerPath from the HostConfig.Devices array of a config.v2.json
// structure represented as a generic map.
//
//	in(1): map[string]interface{} config   parsed config.v2.json document (mutated in place)
//	in(2): string hostPath                 host-side device path of the entry to remove
//	in(3): string containerPath            container-side device path of the entry to remove
func removeDeviceMapping(config map[string]interface{}, hostPath string, containerPath string) {
	// Check if "HostConfig" exists in the config
	hostConfig, ok := config["HostConfig"].(map[string]interface{})
	if !ok {
		return // No host config
	}

	// Get existing devices
	devices, ok := hostConfig["Devices"].([]interface{})
	if !ok {
		return // No devices
	}

	// Filter out the device to remove
	var updatedDevices []interface{}
	for _, device := range devices {
		if deviceMap, ok := device.(map[string]interface{}); ok {
			if deviceMap["PathOnHost"] != hostPath || deviceMap["PathInContainer"] != containerPath {
				updatedDevices = append(updatedDevices, device)
			}
		}
	}

	// Update the devices list
	hostConfig["Devices"] = updatedDevices
}

// recreateContainerWithUpdatedBinds commits a container's filesystem state to a
// temporary image, removes the original container, and creates a replacement with
// newBinds as the bind-mount list. All other host configuration is preserved from
// inspectData. The replacement is started automatically under the original name.
//
//	in(1): context.Context ctx              request context
//	in(2): *client.Client cli               engine client
//	in(3): string containerName             original container name to restore after recreation
//	in(4): string containerID               ID of the container to replace
//	in(5): types.ContainerJSON inspectData  full inspection result of the original container
//	in(6): []string newBinds                complete list of bind-mount strings to apply
//	out:   error                            non-nil if any step of the recreation process fails
func recreateContainerWithUpdatedBinds(ctx context.Context, cli *client.Client, containerName string, containerID string, inspectData types.ContainerJSON, newBinds []string) error {

	// 0. Determine original image name
	oldConfig := inspectData.Config
	oldHostConfig := inspectData.HostConfig

	originalImageName := oldConfig.Image
	if label, ok := oldConfig.Labels["org.rfswift.original_image"]; ok && label != "" {
		originalImageName = label
	}

	// 1. Commit the current container state to a temporary image
	repo, tag := parseImageName(originalImageName)
	repo = strings.TrimPrefix(repo, "localhost/")
	if idx := strings.Index(tag, "_temp_"); idx != -1 {
		tag = tag[:idx]
	}
	tempImageTag := fmt.Sprintf("localhost/%s:%s_temp_%s", repo, tag, time.Now().Format("20060102150405"))
	common.PrintInfoMessage(fmt.Sprintf("Committing container state to temporary image: %s", tempImageTag))

	commitResp, err := cli.ContainerCommit(ctx, containerID, container.CommitOptions{
		Reference: tempImageTag,
		Comment:   "RF Swift: temporary image for mount binding update",
		Pause:     true,
	})
	if err != nil {
		return fmt.Errorf("failed to commit container: %v", err)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Committed as: %s (ID: %s)", tempImageTag, commitResp.ID[:12]))

	// 2. Rebuild container config with updated binds
	oldHostConfig.Binds = newBinds

	// ── Sanitize HostConfig for Podman compat ──
	sanitizeHostConfigForPodman(oldHostConfig)

	oldConfig.Image = tempImageTag // ← use committed snapshot

	// Store original image name + cgroup rules in labels for display purposes.
	// Podman's compat API doesn't return DeviceCgroupRules in inspect, so we
	// persist them as a label sidecar.
	if oldConfig.Labels == nil {
		oldConfig.Labels = make(map[string]string)
	}
	oldConfig.Labels["org.rfswift.original_image"] = originalImageName

	// ── CRITICAL: restore cgroup rules from label into HostConfig ──
	// Podman's inspect returns DeviceCgroupRules as empty, but we stored
	// the real rules in a label. We must inject them back so the recreated
	// container actually gets the cgroup rules applied (not just displayed).
	if len(oldHostConfig.DeviceCgroupRules) == 0 {
		if label, ok := oldConfig.Labels["org.rfswift.cgroup_rules"]; ok && label != "" {
			oldHostConfig.DeviceCgroupRules = strings.Split(label, ",")
		}
	}
	// Update the label with current rules (may have been restored above)
	if len(oldHostConfig.DeviceCgroupRules) > 0 {
		oldConfig.Labels["org.rfswift.cgroup_rules"] = strings.Join(oldHostConfig.DeviceCgroupRules, ",")
	}

	oldConfig.Labels["org.rfswift.exposed_ports"] = convertExposedPortsToString(oldConfig.ExposedPorts)

	// ── USB bind-mount sanitization ──
	// When /dev/bus/usb is bind-mounted, individual /dev/bus/usb/* device
	// entries must be removed (they create specific allow rules that conflict
	// with the wildcard cgroup rule needed for hotplug). Also ensure the
	// USB cgroup rule c 189:* rwm is present.
	// This mirrors the same sanitization done in ContainerRun().
	hasUSBBind := false
	for _, b := range newBinds {
		if strings.HasPrefix(b, "/dev/bus/usb:") || strings.HasPrefix(b, "/dev/bus/usb/") || b == "/dev/bus/usb" {
			hasUSBBind = true
			break
		}
	}
	if hasUSBBind {
		// Remove individual USB device entries — the bind mount covers them
		var cleanDevices []container.DeviceMapping
		for _, d := range oldHostConfig.Devices {
			if !strings.HasPrefix(d.PathOnHost, "/dev/bus/usb/") {
				cleanDevices = append(cleanDevices, d)
			}
		}
		oldHostConfig.Devices = cleanDevices

		// Ensure c 189:* rwm rule is present for USB hotplug
		usbRule := "c 189:* rwm"
		hasUSBRule := false
		for _, r := range oldHostConfig.DeviceCgroupRules {
			if r == usbRule {
				hasUSBRule = true
				break
			}
		}
		if !hasUSBRule {
			oldHostConfig.DeviceCgroupRules = append(oldHostConfig.DeviceCgroupRules, usbRule)
		}
		// Update label
		oldConfig.Labels["org.rfswift.cgroup_rules"] = strings.Join(oldHostConfig.DeviceCgroupRules, ",")
	}

	// 3. Create a temporary-named container first, then swap
	tempContainerName := fmt.Sprintf("%s_rfswift_tmp_%d", containerName, time.Now().UnixNano())
	common.PrintInfoMessage(fmt.Sprintf("Creating new container (temp: %s)...", tempContainerName))

	var newContainerID string

	// ── Podman: use native CLI for creation when cgroup rules are present ──
	// The Docker compat API silently ignores DeviceCgroupRules in ContainerCreate.
	// We must use `podman create` directly to guarantee the rules are applied.
	// (Lima uses Docker inside the VM, so cgroup rules work via the compat API.)
	if len(oldHostConfig.DeviceCgroupRules) > 0 && GetEngine().Type() == EnginePodman {
		containerID, err := podmanCreateViaCLI(tempContainerName, tempImageTag, oldConfig, oldHostConfig)
		if err != nil {
			return fmt.Errorf("failed to create new container via Podman CLI: %v", err)
		}
		newContainerID = containerID
	} else {
		resp, err := cli.ContainerCreate(ctx,
			oldConfig,
			oldHostConfig,
			nil, // networking config — will be reattached
			nil, // platform
			tempContainerName,
		)
		if err != nil {
			return fmt.Errorf("failed to create new container: %v", err)
		}
		newContainerID = resp.ID
	}
	common.PrintSuccessMessage(fmt.Sprintf("New container created: %s", newContainerID[:12]))

	// 4. Remove old container (safe — new one already exists)
	common.PrintInfoMessage("Removing old container...")
	err = cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	if err != nil {
		return fmt.Errorf("failed to remove old container: %v", err)
	}
	common.PrintSuccessMessage("Old container removed.")

	// 4b. Clean up stale temp images (now unreferenced)
	cleanupStaleTempImages(ctx, cli, tempImageTag, repo, tag)

	// 5. Rename temp container to original name
	common.PrintInfoMessage(fmt.Sprintf("Renaming container to '%s'...", containerName))
	if err := cli.ContainerRename(ctx, newContainerID, containerName); err != nil {
		return fmt.Errorf("failed to rename container: %v", err)
	}

	// 6. Start the new container
	common.PrintInfoMessage("Starting new container...")
	if err := cli.ContainerStart(ctx, newContainerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start new container: %v", err)
	}
	common.PrintSuccessMessage("Container started with updated mount bindings.")

	// Clean up the temporary image.
	// Docker allows removing a tag while the container uses it; Podman does not.
	if GetEngine().Type() != EnginePodman {
		if _, err := cli.ImageRemove(ctx, tempImageTag, image.RemoveOptions{Force: false}); err != nil {
			common.PrintWarningMessage(fmt.Sprintf("Could not remove temp image '%s': %v (will be cleaned up next time)", tempImageTag, err))
		} else {
			common.PrintSuccessMessage(fmt.Sprintf("Cleaned up temporary image: %s", tempImageTag))
		}
	}

	return nil
}
