package dock

/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"golang.org/x/crypto/ssh/terminal"

	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
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
	white := "\033[37m"
	blue := "\033[34m"
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	cyan := "\033[36m"
	reset := "\033[0m"

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

	// First check if the tag already contains a version
	baseName, existingVersion := parseTagVersion(tag)
	if existingVersion != "" {
		versionDisplay = existingVersion
	} else if !common.Disconnected {
		// Try to find version by matching digest with remote versions
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

	// Build image status string with version if available
	imageNameWithVersion := props["ImageName"]
	if versionDisplay != "" {
		imageNameWithVersion = fmt.Sprintf("%s %sv%s%s", props["ImageName"], cyan, versionDisplay, reset)
	}

	imageStatus := fmt.Sprintf("%s (Custom)", imageNameWithVersion)
	if common.Disconnected {
		imageStatus = fmt.Sprintf("%s (No network)", imageNameWithVersion)
	}
	imageStatusColor := yellow
	if !isCustom {
		if isUpToDate {
			imageStatus = fmt.Sprintf("%s (Up to date)", imageNameWithVersion)
			imageStatusColor = green
		} else {
			imageStatus = fmt.Sprintf("%s (Obsolete)", imageNameWithVersion)
			imageStatusColor = red
		}
	}

	seccompValue := props["Seccomp"]
	if seccompValue == "" {
		seccompValue = "(Default)"
	}

	properties := [][]string{
		{"Container Name", containerName},
		{"X Display", props["XDisplay"]},
		{"Shell", props["Shell"]},
		{"Privileged Mode", props["Privileged"]},
		{"Network Mode", props["NetworkMode"]},
		{"Exposed Ports", props["ExposedPorts"]},
		{"Port Bindings", props["PortBindings"]},
		{"Image Name", imageStatus},
		{"Size on Disk", size},
		{"Bindings", props["Bindings"]},
		{"Extra Hosts", props["ExtraHosts"]},
		{"Devices", props["Devices"]},
		{"Capabilities", props["Caps"]},
		{"Seccomp profile", seccompValue},
		{"Cgroup rules", props["Cgroups"]},
		{"Ulimits", props["Ulimits"]},
	}

	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80 // default width if terminal size cannot be determined
	}

	// Adjust width for table borders and padding
	maxContentWidth := width - 4
	if maxContentWidth < 20 {
		maxContentWidth = 20 // Minimum content width
	}

	maxKeyLen := 0
	for _, property := range properties {
		if len(property[0]) > maxKeyLen {
			maxKeyLen = len(property[0])
		}
	}

	maxValueLen := maxContentWidth - maxKeyLen - 7 // 7 for borders and spaces
	if maxValueLen < 10 {
		maxValueLen = 10 // Minimum value length
	}

	totalWidth := maxKeyLen + maxValueLen + 7

	// Print the title in blue, aligned to the left with some padding
	title := "🧊 Container Summary"
	leftPadding := 2 // You can adjust this value for more or less left padding
	fmt.Printf("%s%s%s%s%s\n", blue, strings.Repeat(" ", leftPadding), title, strings.Repeat(" ", totalWidth-leftPadding-len(title)), reset)

	fmt.Printf("%s", white) // Switch to white color for the box
	fmt.Printf("╭%s╮\n", strings.Repeat("─", totalWidth-2))

	for i, property := range properties {
		key := property[0]
		value := property[1]
		valueColor := white

		if key == "Image Name" {
			valueColor = imageStatusColor
		}

		// Wrap long values
		wrappedValue := wrapText(value, maxValueLen)
		valueLines := strings.Split(wrappedValue, "\n")

		for j, line := range valueLines {
			if j == 0 {
				fmt.Printf("│ %-*s │ %s%-*s%s │\n", maxKeyLen, key, valueColor, maxValueLen, line, reset)
			} else {
				fmt.Printf("│ %-*s │ %s%-*s%s │\n", maxKeyLen, "", valueColor, maxValueLen, line, reset)
			}

			if j < len(valueLines)-1 {
				fmt.Printf("│%s│%s│\n", strings.Repeat(" ", maxKeyLen+2), strings.Repeat(" ", maxValueLen+2))
			}
		}

		if i < len(properties)-1 {
			fmt.Printf("├%s┼%s┤\n", strings.Repeat("─", maxKeyLen+2), strings.Repeat("─", maxValueLen+2))
		}
	}

	fmt.Printf("╰%s╯\n", strings.Repeat("─", totalWidth-2))
	fmt.Printf("%s", reset)
	fmt.Println() // Ensure we end with a newline for clarity
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

	// Check if the system is Windows
	if runtime.GOOS == "windows" {
		title := "Unsupported on Windows"
		message := `This function is not supported on Windows.
However, you can achieve similar functionality by using the following commands:
- "rfswift commit" to create a new image with a new tag.
- "rfswift remove" to remove the existing container.
- "rfswift run" to run a container with new bindings.`

		rfutils.DisplayNotification(title, message, "warning")
		os.Exit(1)
	}

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

	common.PrintInfoMessage("Loading hostconfig.json...")
	var hostConfig HostConfigFull
	if err := loadJSON(hostConfigPath, &hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("HostConfig loaded successfully.")

	// Load and update config.v2.json
	common.PrintInfoMessage("Determining config.v2.json path...")
	configV2Path := strings.Replace(hostConfigPath, "hostconfig.json", "config.v2.json", 1)
	common.PrintInfoMessage(fmt.Sprintf("Loading config.v2.json from: %s", configV2Path))
	var configV2 map[string]interface{}
	if err := loadJSON(configV2Path, &configV2); err != nil {
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
	if err := saveJSON(hostConfigPath, hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("hostconfig.json updated successfully.")

	common.PrintInfoMessage("Saving updated config.v2.json...")
	if err := saveJSON(configV2Path, configV2); err != nil {
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

	// Check if the system is Windows
	if runtime.GOOS == "windows" {
		title := "Unsupported on Windows"
		message := `This function is not supported on Windows.
However, you can achieve similar functionality by using the following commands:
- "rfswift commit" to create a new image with a new tag.
- "rfswift remove" to remove the existing container.
- "rfswift run" to run a container with new device bindings.`

		rfutils.DisplayNotification(title, message, "warning")
		os.Exit(1) // Exit since this function is not supported on Windows
	}

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

	common.PrintInfoMessage("Loading hostconfig.json...")
	var hostConfig HostConfigFull
	if err := loadJSON(hostConfigPath, &hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to load hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("HostConfig loaded successfully.")

	// Load and update config.v2.json
	common.PrintInfoMessage("Determining config.v2.json path...")
	configV2Path := strings.Replace(hostConfigPath, "hostconfig.json", "config.v2.json", 1)
	common.PrintInfoMessage(fmt.Sprintf("Loading config.v2.json from: %s", configV2Path))
	var configV2 map[string]interface{}
	if err := loadJSON(configV2Path, &configV2); err != nil {
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
	if err := saveJSON(hostConfigPath, hostConfig); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to save hostconfig.json: %v", err))
		os.Exit(1)
	}
	common.PrintSuccessMessage("hostconfig.json updated successfully.")

	common.PrintInfoMessage("Saving updated config.v2.json...")
	if err := saveJSON(configV2Path, configV2); err != nil {
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

// UpdateCapability adds or removes a Linux capability from a container by
// recreating the container with the updated CapAdd list via
// recreateContainerWithProperties.
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

	// Get container info first
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	// Parse existing capabilities
	var capabilities []string
	if props["Caps"] != "" {
		capabilities = strings.Split(props["Caps"], ",")
	}

	// Add or remove the capability
	if add {
		// Check if already exists
		found := false
		for _, cap := range capabilities {
			if strings.TrimSpace(cap) == capability {
				found = true
				break
			}
		}

		if found {
			common.PrintInfoMessage(fmt.Sprintf("Capability '%s' already exists in container '%s'", capability, containerName))
			return nil
		}

		capabilities = append(capabilities, capability)
		common.PrintInfoMessage(fmt.Sprintf("Adding capability '%s' to container '%s'", capability, containerName))
	} else {
		// Remove capability
		newCapabilities := []string{}
		found := false
		for _, cap := range capabilities {
			if strings.TrimSpace(cap) != capability {
				newCapabilities = append(newCapabilities, cap)
			} else {
				found = true
			}
		}

		if !found {
			common.PrintWarningMessage(fmt.Sprintf("Capability '%s' not found in container '%s'", capability, containerName))
			return nil
		}

		capabilities = newCapabilities
		common.PrintInfoMessage(fmt.Sprintf("Removing capability '%s' from container '%s'", capability, containerName))
	}

	// Update the container
	props["Caps"] = strings.Join(capabilities, ",")

	return recreateContainerWithProperties(ctx, cli, containerID, props)
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

	// Get container info first
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	// Parse existing cgroup rules
	var cgroupRules []string
	if props["Cgroups"] != "" {
		cgroupRules = strings.Split(props["Cgroups"], ",")
	}

	// Add or remove the rule
	if add {
		// Check if already exists
		found := false
		for _, r := range cgroupRules {
			if strings.TrimSpace(r) == rule {
				found = true
				break
			}
		}

		if found {
			common.PrintInfoMessage(fmt.Sprintf("Cgroup rule '%s' already exists in container '%s'", rule, containerName))
			return nil
		}

		cgroupRules = append(cgroupRules, rule)
		common.PrintInfoMessage(fmt.Sprintf("Adding cgroup rule '%s' to container '%s'", rule, containerName))
	} else {
		// Remove rule
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
		common.PrintInfoMessage(fmt.Sprintf("Removing cgroup rule '%s' from container '%s'", rule, containerName))
	}

	// Update the container
	props["Cgroups"] = strings.Join(cgroupRules, ",")

	return recreateContainerWithProperties(ctx, cli, containerID, props)
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

	// Get container info first
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	// Parse existing exposed ports
	exposedPortsStr := props["ExposedPorts"]
	var exposedPorts []string
	if exposedPortsStr != "" {
		exposedPorts = strings.Split(exposedPortsStr, ",")
		// Trim spaces
		for i := range exposedPorts {
			exposedPorts[i] = strings.TrimSpace(exposedPorts[i])
		}
	}

	// Add or remove the port
	if add {
		// Check if already exists
		found := false
		for _, p := range exposedPorts {
			if p == port {
				found = true
				break
			}
		}

		if found {
			common.PrintInfoMessage(fmt.Sprintf("Port '%s' already exposed in container '%s'", port, containerName))
			return nil
		}

		exposedPorts = append(exposedPorts, port)
		common.PrintInfoMessage(fmt.Sprintf("Exposing port '%s' on container '%s'", port, containerName))
	} else {
		// Remove port
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
		common.PrintInfoMessage(fmt.Sprintf("Removing exposed port '%s' from container '%s'", port, containerName))
	}

	// Update the container
	props["ExposedPorts"] = strings.Join(exposedPorts, ",")

	return recreateContainerWithProperties(ctx, cli, containerID, props)
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
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get container info first
	containerJSON, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	// Get container properties
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	// Parse existing port bindings
	portBindingsStr := props["PortBindings"]
	var portBindings []string
	if portBindingsStr != "" {
		portBindings = strings.Split(portBindingsStr, ";;")
		// Trim spaces
		for i := range portBindings {
			portBindings[i] = strings.TrimSpace(portBindings[i])
		}
	}

	// Add or remove the binding
	if add {
		// Check if already exists
		found := false
		for _, b := range portBindings {
			if b == binding {
				found = true
				break
			}
		}

		if found {
			common.PrintInfoMessage(fmt.Sprintf("Port binding '%s' already exists in container '%s'", binding, containerName))
			return nil
		}

		portBindings = append(portBindings, binding)
		common.PrintInfoMessage(fmt.Sprintf("Adding port binding '%s' to container '%s'", binding, containerName))
	} else {
		// Remove binding
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
		common.PrintInfoMessage(fmt.Sprintf("Removing port binding '%s' from container '%s'", binding, containerName))
	}

	// Update the container
	props["PortBindings"] = strings.Join(portBindings, ";;")

	return recreateContainerWithProperties(ctx, cli, containerID, props)
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
	if len(hostConfig.DeviceCgroupRules) > 0 && !EngineSupportsDirectConfigEdit() && !IsRootlessPodman() {
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
	if len(oldHostConfig.DeviceCgroupRules) > 0 && !EngineSupportsDirectConfigEdit() {
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

	// NOTE: We intentionally do NOT delete the temp image here.
	// The new container references it, so Podman would refuse anyway.
	// It will be cleaned up at the START of the next recreation

	return nil
}
