/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Container lifecycle management
 */

package dock

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/moby/term"
	"golang.org/x/crypto/ssh/terminal"

	common "penthertz/rfswift/common"
	"penthertz/rfswift/tui"
)

// startDesktopInContainer launches the desktop-start script inside an already-running
// container by injecting the RFSWIFT_DESKTOP_* environment variables via a detached exec.
func startDesktopInContainer(ctx context.Context, cli *client.Client, containerID string) error {
	// Determine the listen host inside the container: for non-host network modes,
	// websockify must listen on 0.0.0.0 so Docker port forwarding can reach it.
	desktopListenHost := containerCfg.desktopHost
	containerJSON, inspectErr := cli.ContainerInspect(ctx, containerID)
	if inspectErr == nil {
		netMode := string(containerJSON.HostConfig.NetworkMode)
		if netMode != "" && netMode != "host" {
			desktopListenHost = "0.0.0.0"
		}
	}

	execConfig := container.ExecOptions{
		Detach: true,
		Cmd:    []string{"/usr/sbin/desktop-start"},
		Env: func() []string {
			sslFlag := ""
			if containerCfg.desktopSSL {
				sslFlag = "1"
			}
			return []string{
				"RFSWIFT_DESKTOP_PROTO=" + containerCfg.desktopProto,
				"RFSWIFT_DESKTOP_HOST=" + desktopListenHost,
				"RFSWIFT_DESKTOP_PORT=" + containerCfg.desktopPort,
				"RFSWIFT_DESKTOP_PASS=" + containerCfg.desktopPass,
				"RFSWIFT_DESKTOP_SSL=" + sslFlag,
			}
		}(),
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to start desktop: %v", err)
	}

	if err := cli.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{Detach: true}); err != nil {
		return fmt.Errorf("failed to start desktop: %v", err)
	}

	// Give the VNC server a moment to start
	time.Sleep(2 * time.Second)
	return nil
}

// printDesktopURL prints the desktop access URL when desktop mode is enabled.
func printDesktopURL() {
	if containerCfg.desktopProto == "" {
		return
	}
	proto := containerCfg.desktopProto
	if containerCfg.desktopSSL {
		if proto == "http" {
			proto = "https"
		} else {
			proto = "vncs"
		}
	}
	url := fmt.Sprintf("%s://%s:%s", proto, containerCfg.desktopHost, containerCfg.desktopPort)
	common.PrintInfoMessage(fmt.Sprintf("Desktop available at: %s", url))
	if containerCfg.desktopProto == "http" {
		common.PrintInfoMessage("Open the URL above in your browser to access the GUI desktop")
	} else {
		common.PrintInfoMessage("Connect with a VNC client to the address above")
	}
}

// resizeTty continuously monitors and resizes the container TTY to match the local terminal size.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string contid container ID
//	in(4): int fd file descriptor
func resizeTty(ctx context.Context, cli *client.Client, contid string, fd int) {
	for {
		width, height, err := getTerminalSize(fd)
		if err != nil {
			log.Printf("Error getting terminal size: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		err = cli.ContainerResize(ctx, contid, container.ResizeOptions{
			Height: uint(height),
			Width:  uint(width),
		})
		if err != nil {
			log.Printf("Error resizing container TTY: %v", err)
		}

		time.Sleep(1 * time.Second)
	}
}

// ContainerLast lists the most recent RF Swift containers, optionally filtered by image, name, or ID.
//
//	in(1): string ifilter filter string
//	in(2): string labelKey label key
//	in(3): string labelValue label value
func ContainerLast(ifilter string, labelKey string, labelValue string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	// Set up container filters for labels only
	// We'll handle image ancestor and name/ID filtering manually
	containerFilters := filters.NewArgs()
	if labelKey != "" && labelValue != "" {
		containerFilters.Add("label", fmt.Sprintf("%s=%s", labelKey, labelValue))
	}

	// Get container list
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Limit:   15,
		Filters: containerFilters,
	})
	if err != nil {
		panic(err)
	}

	// Create maps to store image mappings
	imageIDToNames := make(map[string][]string)
	hashToNames := make(map[string][]string)

	// Get all images to build a mapping of image IDs to all their tags
	images, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		panic(err)
	}

	// Build image ID to names mapping
	for _, img := range images {
		shortID := img.ID[7:19] // Get a shortened version of the SHA256 hash
		fullHash := img.ID[7:]  // Remove "sha256:" prefix but keep full hash

		// Store mappings if image has tags
		if len(img.RepoTags) > 0 {
			imageIDToNames[img.ID] = img.RepoTags
			imageIDToNames[shortID] = img.RepoTags
			hashToNames[fullHash] = img.RepoTags
		}
	}

	//rfutils.ClearScreen()
	tableData := [][]string{}

	// Filter containers by image, name or ID (if ifilter is provided)
	filteredContainers := []types.Container{}

	if ifilter != "" {
		lowerFilter := strings.ToLower(ifilter)
		for _, container := range containers {
			// Check if image name contains the filter (original behavior)
			if strings.Contains(strings.ToLower(container.Image), lowerFilter) {
				filteredContainers = append(filteredContainers, container)
				continue
			}

			// Check if container ID (full or short) contains the filter
			if strings.Contains(strings.ToLower(container.ID), lowerFilter) ||
				strings.Contains(strings.ToLower(container.ID[:12]), lowerFilter) {
				filteredContainers = append(filteredContainers, container)
				continue
			}

			// Check if any container name contains the filter
			for _, name := range container.Names {
				// Remove leading slash from name if it exists
				cleanName := name
				if len(name) > 0 && name[0] == '/' {
					cleanName = name[1:]
				}

				if strings.Contains(strings.ToLower(cleanName), lowerFilter) {
					filteredContainers = append(filteredContainers, container)
					break
				}
			}
		}
	} else {
		filteredContainers = containers
	}

	for _, container := range filteredContainers {
		// Skip ghost containers that can't be inspected
		_, err := cli.ContainerInspect(ctx, container.ID)
		if err != nil {
			continue
		}

		created := time.Unix(container.Created, 0).Format(time.RFC3339)

		// Get the container image ID and associate with tags
		containerImageID := container.ImageID
		shortImageID := ""
		if len(containerImageID) >= 19 {
			shortImageID = containerImageID[7:19] // shortened SHA256
		} else if len(containerImageID) > 7 {
			shortImageID = containerImageID[7:] // Use whatever is available after "sha256:"
		} else {
			shortImageID = containerImageID // Use as-is if too short
		}

		// Get the display image name
		imageTag := container.Image

		// Check for original image label (set during container recreation)
		if label, ok := container.Labels["org.rfswift.original_image"]; ok && label != "" {
			imageTag = label
		}

		// Check if this is a SHA256 hash
		isSHA256 := strings.HasPrefix(imageTag, "sha256:")

		// If this is a SHA256 hash, try to find a friendly name
		if isSHA256 {
			hashPart := imageTag[7:] // Remove "sha256:" prefix
			// Check if we have a friendly name for this hash
			if tags, ok := hashToNames[hashPart]; ok && len(tags) > 0 {
				imageTag = tags[0] // Use the first tag
			} else if tags, ok := imageIDToNames[containerImageID]; ok && len(tags) > 0 {
				imageTag = tags[0] // Fallback to container image ID mapping
			}
		}

		// Check if this is a renamed image (date format: -DDMMYYYY)
		isRenamed := false
		if len(imageTag) > 9 { // Make sure string is long enough before checking suffix
			suffix := imageTag[len(imageTag)-9:]
			if len(suffix) > 0 && suffix[0] == '-' {
				// Check if the rest is a date format
				datePattern := true
				for i := 1; i < 9; i++ {
					if i < 9 && (suffix[i] < '0' || suffix[i] > '9') {
						datePattern = false
						break
					}
				}
				isRenamed = datePattern
			}
		}

		// Prepare the display string
		imageDisplay := imageTag

		if label, ok := container.Labels["org.rfswift.original_image"]; ok && label != "" {
			imageDisplay = fmt.Sprintf("%s (temp: %s)", label, shortImageID)
		}

		// For SHA256 or renamed images, show hash for clarity
		if isSHA256 || isRenamed {
			imageDisplay = fmt.Sprintf("%s (%s)", imageTag, shortImageID)
		}

		containerName := ""
		if len(container.Names) > 0 {
		    containerName = container.Names[0]
		    if len(containerName) > 0 && containerName[0] == '/' {
		        containerName = containerName[1:]
		    }
		} else {
		    containerName = container.ID[:12] // fallback to short ID
		}
		containerID := container.ID[:12]
		command := container.Command

		// Truncate command if too long
		if len(command) > 30 {
			command = command[:27] + "..."
		}

		tableData = append(tableData, []string{
			created,
			imageDisplay,
			containerName,
			containerID,
			command,
		})
	}

	tui.RenderTable(tui.TableConfig{
		Title:      "🤖 Last Run Containers",
		TitleColor: tui.ColorPink,
		Headers:    []string{"Created", "Image Tag (ID)", "Container Name", "Container ID", "Command"},
		Rows:       tableData,
	})
}

// latestDockerID returns the ID of the most recently created container matching the given label.
//
//	in(1): string labelKey label key
//	in(2): string labelValue label value
//	out: string container ID
func latestDockerID(labelKey string, labelValue string) string {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	containerFilters := filters.NewArgs()
	containerFilters.Add("label", fmt.Sprintf("%s=%s", labelKey, labelValue))

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: containerFilters,
	})
	if err != nil {
		panic(err)
	}

	// Sort by creation time descending, validate each candidate
	for _, cont := range containers {
		// Verify the container is actually accessible
		_, err := cli.ContainerInspect(ctx, cont.ID)
		if err != nil {
			continue // ghost container — skip
		}
		return cont.ID
	}

	return ""
}

// ContainerInfo holds basic container metadata for interactive selection.
type ContainerInfo struct {
	ID    string
	Name  string
	Image string
	State string
}

// ListContainers returns RF Swift containers with their name, image, and state.
func ListContainers(labelKey string, labelValue string) []ContainerInfo {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return nil
	}
	defer cli.Close()

	containerFilters := filters.NewArgs()
	if labelKey != "" && labelValue != "" {
		containerFilters.Add("label", fmt.Sprintf("%s=%s", labelKey, labelValue))
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Limit:   15,
		Filters: containerFilters,
	})
	if err != nil {
		return nil
	}

	var result []ContainerInfo
	for _, cont := range containers {
		_, err := cli.ContainerInspect(ctx, cont.ID)
		if err != nil {
			continue
		}

		name := cont.ID[:12]
		if len(cont.Names) > 0 {
			name = cont.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		result = append(result, ContainerInfo{
			ID:    cont.ID[:12],
			Name:  name,
			Image: cont.Image,
			State: cont.State,
		})
	}
	return result
}

// LatestContainerID returns the ID of the most recent RF Swift container, or empty string.
func LatestContainerID() string {
	return latestDockerID("org.container.project", "rfswift")
}

// ContainerExec attaches to an existing container and opens an interactive shell session.
//
//	in(1): string containerIdentifier container ID or name
//	in(2): string WorkingDir working directory
func ContainerExec(containerIdentifier string, WorkingDir string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	if containerIdentifier == "" {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		containerIdentifier = latestDockerID(labelKey, labelValue)
		if containerIdentifier == "" {
			common.PrintErrorMessage(fmt.Errorf("no RF Swift container found. Create one first with: rfswift run -n <name> -i <image>"))
			return
		}
	}

	if err := cli.ContainerStart(ctx, containerIdentifier, container.StartOptions{}); err != nil {
		common.PrintErrorMessage(err)
		return
	}

	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' started successfully", containerIdentifier))

	// Get container properties and name
	props, err := getContainerProperties(ctx, cli, containerIdentifier)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	containerJSON, err := cli.ContainerInspect(ctx, containerIdentifier)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	containerName := strings.TrimPrefix(containerJSON.Name, "/")

	size := props["Size"]
	printContainerProperties(ctx, cli, containerName, props, size)

	// Desktop mode: start VNC/noVNC inside the container via exec
	if containerCfg.desktopProto != "" {
		if err := startDesktopInContainer(ctx, cli, containerIdentifier); err != nil {
			common.PrintErrorMessage(err)
			return
		}
		printDesktopURL()
	}

	// VPN mode: start VPN client inside the container via exec
	if containerCfg.vpn != "" {
		if err := startVPNInContainer(ctx, cli, containerIdentifier); err != nil {
			common.PrintErrorMessage(err)
			return
		}
		printVPNInfo()
	}

	// Determine shell to use:
	// Priority: 1) explicitly set via CLI (-e flag) if different from default
	//           2) container's original shell (from containerJSON.Path)
	//           3) fallback to /bin/bash
	shellToUse := containerCfg.shell

	// If shell is empty or default, prefer container's configured shell
	if shellToUse == "" || shellToUse == "/bin/bash" {
		containerShell := containerJSON.Path
		if containerShell != "" {
			shellToUse = containerShell
		}
	}

	// Final fallback
	if shellToUse == "" {
		shellToUse = "/bin/bash"
	}

	if err := execInteractiveSession(ctx, cli, containerIdentifier, shellToUse, WorkingDir); err != nil {
		common.PrintErrorMessage(err)
		return
	}

	common.PrintSuccessMessage(fmt.Sprintf("Shell session in container '%s' ended", containerName))
}

// ContainerRun creates a new container with the given name and starts an interactive session.
//
//	in(1): string containerName container name
func ContainerRun(containerName string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	containerCfg.imagename = normalizeImageName(containerCfg.imagename)

	// VPN: adjust caps, devices, bindings, env before container creation
	if containerCfg.vpn != "" {
		if err := applyVPNConfig(); err != nil {
			common.PrintErrorMessage(err)
			return
		}
	}

	bindings := combineBindings(containerCfg.x11forward, containerCfg.extrabinding)
	extrahosts := splitAndCombine(containerCfg.extrahosts)
	dockerenv := combineEnv(containerCfg.xdisplay, containerCfg.pulseServer, containerCfg.extraenv)

	// Desktop mode: inject env vars and port configuration
	if containerCfg.desktopProto != "" {
		sslFlag := ""
		if containerCfg.desktopSSL {
			sslFlag = "1"
		}

		// For non-host network modes, websockify must listen on 0.0.0.0 inside the
		// container so Docker/Podman port forwarding can reach it (the forwarded
		// traffic arrives on the container's eth0 address, not 127.0.0.1).
		desktopListenHost := containerCfg.desktopHost
		if containerCfg.networkMode != "" && containerCfg.networkMode != "host" {
			desktopListenHost = "0.0.0.0"
		}

		dockerenv = append(dockerenv,
			"RFSWIFT_DESKTOP_PROTO="+containerCfg.desktopProto,
			"RFSWIFT_DESKTOP_HOST="+desktopListenHost,
			"RFSWIFT_DESKTOP_PORT="+containerCfg.desktopPort,
			"RFSWIFT_DESKTOP_PASS="+containerCfg.desktopPass,
			"RFSWIFT_DESKTOP_SSL="+sslFlag,
		)

		// For non-host network modes, set up port bindings so the desktop is reachable
		if containerCfg.networkMode != "" && containerCfg.networkMode != "host" {
			defaultPort := containerCfg.desktopPort
			portProto := defaultPort + "/tcp"
			if containerCfg.exposedPorts != "" {
				containerCfg.exposedPorts += "," + portProto
			} else {
				containerCfg.exposedPorts = portProto
			}
			if containerCfg.bindedPorts != "" {
				containerCfg.bindedPorts += "," + containerCfg.desktopHost + ":" + containerCfg.desktopPort + ":" + portProto
			} else {
				containerCfg.bindedPorts = containerCfg.desktopHost + ":" + containerCfg.desktopPort + ":" + portProto
			}
		}
	}

	exposedPorts := ParseExposedPorts(containerCfg.exposedPorts)
	bindedPorts := ParseBindedPorts(containerCfg.bindedPorts)

	// Prepare host config based on privileged flag
	hostConfig := &container.HostConfig{
		NetworkMode:  container.NetworkMode(containerCfg.networkMode),
		Binds:        bindings,
		ExtraHosts:   extrahosts,
		PortBindings: bindedPorts,
		Privileged:   containerCfg.privileged,
	}

	// Handle ulimits
	ulimits := getUlimitsForContainer()
	if len(ulimits) > 0 {
		hostConfig.Resources.Ulimits = ulimits
	}

	// If not in privileged mode, add device permissions
	if !containerCfg.privileged {
		devices := getDeviceMappingsFromString(containerCfg.devices)

		if containerCfg.usbforward != "" {
			parts := strings.Split(containerCfg.usbforward, ":")
			if len(parts) == 2 {
				devices = append(devices, container.DeviceMapping{
					PathOnHost:        parts[0],
					PathInContainer:   parts[1],
					CgroupPermissions: "rwm",
				})
			}
		}

		// ── Hotplug-aware device filtering ─────────────────────────────
		//
		// If a /dev subtree is already bind-mounted (e.g. /dev/bus/usb),
		// individual device entries under that subtree are redundant and
		// prevent USB hotplug from working (they are static snapshots of
		// the device nodes that existed at container creation time).
		//
		// We remove them here and rely on the bind mount (filesystem
		// visibility) + cgroup device rule (kernel access) instead.
		//
		bindSet := make(map[string]bool)
		for _, b := range bindings {
			parts := strings.SplitN(b, ":", 3)
			if len(parts) >= 2 {
				bindSet[parts[1]] = true // destination (container) path
			}
		}

		var filteredDevices []container.DeviceMapping
		for _, dev := range devices {
			covered := false
			for bindPath := range bindSet {
				if strings.HasPrefix(dev.PathOnHost, bindPath+"/") || dev.PathOnHost == bindPath {
					covered = true
					break
				}
			}
			if !covered {
				filteredDevices = append(filteredDevices, dev)
			}
		}

		hostConfig.Devices = filteredDevices

		// ── Cgroup rules ───────────────────────────────────────────────
		if containerCfg.cgroups != "" {
			rules := strings.Split(containerCfg.cgroups, ",")
			// Fix permission order: "rmw" → "rwm"
			for i, rule := range rules {
				rules[i] = strings.TrimSpace(rule)
				rules[i] = strings.Replace(rules[i], "rmw", "rwm", 1)
			}
			hostConfig.DeviceCgroupRules = rules
		}

		// Auto-inject cgroup device rules for bind-mounted /dev subtrees
		// so that hotplug works out-of-the-box without needing explicit
		// --cgroups flags for common device classes.
		devMajorRules := map[string]string{
			"/dev/bus/usb": "c 189:* rwm",
			"/dev/snd":     "c 116:* rwm",
			"/dev/dri":     "c 226:* rwm",
			"/dev/input":   "c 13:* rwm",
			"/dev/vhci":    "c 137:* rwm",
		}

		existingRules := make(map[string]bool)
		for _, rule := range hostConfig.DeviceCgroupRules {
			existingRules[rule] = true
		}

		for bindPath := range bindSet {
			if rule, ok := devMajorRules[bindPath]; ok && !existingRules[rule] {
				hostConfig.DeviceCgroupRules = append(hostConfig.DeviceCgroupRules, rule)
				existingRules[rule] = true
			}
		}

		// Also inject cgroup rules for remaining individual device entries
		for _, dev := range filteredDevices {
			for prefix, rule := range devMajorRules {
				if strings.HasPrefix(dev.PathOnHost, prefix) && !existingRules[rule] {
					hostConfig.DeviceCgroupRules = append(hostConfig.DeviceCgroupRules, rule)
					existingRules[rule] = true
				}
			}
		}

		// ── Seccomp ────────────────────────────────────────────────────
		if containerCfg.seccomp != "" {
			seccompOpts := strings.Split(containerCfg.seccomp, ",")
			for i, opt := range seccompOpts {
				if !strings.Contains(opt, "=") {
					seccompOpts[i] = "seccomp=" + opt
				}
			}
			hostConfig.SecurityOpt = seccompOpts
		}

		// ── Capabilities ───────────────────────────────────────────────
		if containerCfg.caps != "" {
			hostConfig.CapAdd = strings.Split(containerCfg.caps, ",")
		}
	}

	containerLabels := map[string]string{
		"org.container.project": "rfswift",
	}
	if containerCfg.desktopProto != "" {
		containerLabels["org.rfswift.desktop"] = fmt.Sprintf("%s://%s:%s", containerCfg.desktopProto, containerCfg.desktopHost, containerCfg.desktopPort)
	}
	if len(hostConfig.DeviceCgroupRules) > 0 {
		containerLabels["org.rfswift.cgroup_rules"] = strings.Join(hostConfig.DeviceCgroupRules, ",")
	}
	if containerCfg.exposedPorts == "" {
		containerLabels["org.rfswift.exposedPorts"] = "none"
	} else {
		containerLabels["org.rfswift.exposedPorts"] = containerCfg.exposedPorts
	}

	// ── Rootless Podman: strip unsupported features ────────────────
	if IsRootlessPodman() {
		// 1. Cgroup rules
		if len(hostConfig.DeviceCgroupRules) > 0 {
			common.PrintWarningMessage("Rootless Podman does not support device cgroup rules.")
			common.PrintWarningMessage(fmt.Sprintf("Rules that will be dropped: %s", strings.Join(hostConfig.DeviceCgroupRules, ", ")))
			common.PrintInfoMessage("Device hotplug (USB, SDR dongles) may not work without cgroup rules.")
			common.PrintInfoMessage("To use cgroup rules, run RF Swift with sudo.")
			if !tui.Confirm("Continue without cgroup rules?") {
				common.PrintInfoMessage("Aborted. Re-run with: sudo ./rfswift run ...")
				return
			}
			hostConfig.DeviceCgroupRules = nil
			delete(containerLabels, "org.rfswift.cgroup_rules")
			common.PrintInfoMessage("Cgroup rules removed — proceeding in rootless mode.")
		}

		// 2. Filter devices to only those accessible by current user
		// Some devices are readable on the host but can't be created as device nodes in rootless containers
		rootlessBlockedDevices := map[string]bool{
			"/dev/tty":     true,
			"/dev/tty0":    true,
			"/dev/tty1":    true,
			"/dev/tty2":    true,
			"/dev/console": true,
			"/dev/vcsa":    true,
			"/dev/vhci":    true,
			"/dev/uinput":  true,
		}
		if len(hostConfig.Devices) > 0 {
			var accessible []container.DeviceMapping
			var dropped []string
			for _, dev := range hostConfig.Devices {
				if rootlessBlockedDevices[dev.PathOnHost] {
					dropped = append(dropped, dev.PathOnHost)
					continue
				}
				f, err := os.OpenFile(dev.PathOnHost, os.O_RDONLY, 0)
				if err == nil {
					f.Close()
					accessible = append(accessible, dev)
				} else {
					dropped = append(dropped, dev.PathOnHost)
				}
			}
			if len(dropped) > 0 {
				common.PrintWarningMessage(fmt.Sprintf("Dropping %d inaccessible device(s) for rootless mode:", len(dropped)))
				for _, d := range dropped {
					common.PrintWarningMessage(fmt.Sprintf("  - %s", d))
				}
			}
			hostConfig.Devices = accessible
		}
	}

	// Verify the image exists locally before attempting to create container
	_, err = ImageInspectCompat(ctx, cli, containerCfg.imagename)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("image '%s' not found locally. Pull it first with: rfswift pull -i %s", containerCfg.imagename, containerCfg.imagename))
		return
	}

	// Build container config
	containerConfig := &container.Config{
		Image:        containerCfg.imagename,
		Cmd:          []string{containerCfg.shell},
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

	// Desktop mode: use entrypoint wrapper that starts VNC before the shell
	if containerCfg.desktopProto != "" {
		containerConfig.Entrypoint = []string{"/usr/sbin/rfswift-entrypoint"}
	}

	// ── NAT mode: create or join a per-container/shared network ──
	networkingConfig := &network.NetworkingConfig{}
	if isNAT, natTarget, natUserSubnet := parseNATMode(); isNAT {
		natNetName, natSubnet, natErr := createOrJoinNATNetwork(ctx, cli, containerName, natTarget, natUserSubnet)
		if natErr != nil {
			common.PrintErrorMessage(natErr)
			return
		}
		hostConfig.NetworkMode = container.NetworkMode(natNetName)
		containerLabels["org.rfswift.nat_network"] = natNetName
		containerLabels["org.rfswift.nat_subnet"] = natSubnet
		networkingConfig.EndpointsConfig = map[string]*network.EndpointSettings{
			natNetName: {},
		}
	}

	resp, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		if strings.Contains(err.Error(), "already in use") || strings.Contains(err.Error(), "already exists") {
			common.PrintErrorMessage(fmt.Errorf("container name '%s' is already in use. Use a different name with -n, or exec into the existing container with: rfswift exec -c %s", containerName, containerName))
		} else {
			common.PrintErrorMessage(err)
		}
		return
	}

	// ── Podman or recording: use exec-style attach ──
	// Podman's compat API rejects attach-before-start.
	// Recording mode uses exec so RFSWIFT_RECORDING is session-scoped
	// (not baked into the container env, which would persist forever).
	if GetEngine().Type() == EnginePodman || os.Getenv("RFSWIFT_RECORDING") == "1" {
		if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			common.PrintErrorMessage(err)
			return
		}

		props, err := getContainerProperties(ctx, cli, resp.ID)
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		size := props["Size"]
		printContainerProperties(ctx, cli, containerName, props, size)
		common.PrintSuccessMessage(fmt.Sprintf("Container '%s' started successfully", containerName))
		printDesktopURL()

		// Start VPN if configured
		if containerCfg.vpn != "" {
			if err := startVPNInContainer(ctx, cli, resp.ID); err != nil {
				common.PrintErrorMessage(err)
			}
			printVPNInfo()
		}

		// Attach via exec (same as ContainerExec)
		if err := execInteractiveSession(ctx, cli, resp.ID, containerCfg.shell, ""); err != nil {
			common.PrintErrorMessage(err)
		}
		return
	}

	waiter, err := cli.ContainerAttach(ctx, resp.ID, container.AttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  true,
		Stream: true,
	})
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer waiter.Close()

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		common.PrintErrorMessage(err)
		return
	}

	props, err := getContainerProperties(ctx, cli, resp.ID)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	size := props["Size"]
	printContainerProperties(ctx, cli, containerName, props, size)
	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' started successfully", containerName))
	printDesktopURL()

	// Start VPN if configured
	if containerCfg.vpn != "" {
		if err := startVPNInContainer(ctx, cli, resp.ID); err != nil {
			common.PrintErrorMessage(err)
		}
		printVPNInfo()
	}

	handleIOStreams(waiter)
	fd := int(os.Stdin.Fd())
	if terminal.IsTerminal(fd) {
		oldState, err := terminal.MakeRaw(fd)
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		defer terminal.Restore(fd, oldState)
		go resizeTty(ctx, cli, resp.ID, fd)
		go readAndWriteInput(waiter)
	}

	waitForContainer(ctx, cli, resp.ID)
}

// execInteractiveSession creates an exec instance in the container and runs an interactive
// terminal session with raw mode, resize handling, and I/O copying.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string containerID
//	in(4): string shell command
//	in(5): string workingDir working directory
//	out: error
func execInteractiveSession(ctx context.Context, cli *client.Client, containerID string, shell string, workingDir string) error {
	execConfig := container.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{shell},
		WorkingDir:   workingDir,
	}

	// Propagate recording indicator into the container shell
	if os.Getenv("RFSWIFT_RECORDING") == "1" {
		execConfig.Env = []string{"RFSWIFT_RECORDING=1"}
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec instance: %v", err)
	}

	attachResp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{Tty: true})
	if err != nil {
		return fmt.Errorf("failed to attach to exec instance: %v", err)
	}
	defer attachResp.Close()

	inFd, inIsTerminal := term.GetFdInfo(os.Stdin)
	outFd, outIsTerminal := term.GetFdInfo(os.Stdout)

	if inIsTerminal {
		state, err := term.SetRawTerminal(inFd)
		if err != nil {
			return fmt.Errorf("failed to set raw terminal: %v", err)
		}
		defer term.RestoreTerminal(inFd, state)
	}

	// NOTE: Podman's compat API implicitly starts the exec session during Attach,
	// so calling ExecStart again causes "exec session state improper". Skip it.
	if GetEngine().Type() != EnginePodman {
		if err := cli.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{Tty: true}); err != nil {
			return fmt.Errorf("failed to start exec instance: %v", err)
		}
	}

	// Handle terminal resize
	go func() {
		switch runtime.GOOS {
		case "linux", "darwin":
			sigchan := make(chan os.Signal, 1)
			signal.Notify(sigchan, syscallsigwin())
			defer signal.Stop(sigchan)

			for range sigchan {
				if outIsTerminal {
					if size, err := term.GetWinsize(outFd); err == nil {
						cli.ContainerExecResize(ctx, execID.ID, container.ResizeOptions{
							Height: uint(size.Height),
							Width:  uint(size.Width),
						})
					}
				}
			}
		case "windows":
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			var lastHeight, lastWidth uint16
			for range ticker.C {
				if outIsTerminal {
					if size, err := term.GetWinsize(outFd); err == nil {
						if size.Height != lastHeight || size.Width != lastWidth {
							cli.ContainerExecResize(ctx, execID.ID, container.ResizeOptions{
								Height: uint(size.Height),
								Width:  uint(size.Width),
							})
							lastHeight = size.Height
							lastWidth = size.Width
						}
					}
				}
			}
		}
	}()

	// Trigger initial resize
	if outIsTerminal {
		if size, err := term.GetWinsize(outFd); err == nil {
			cli.ContainerExecResize(ctx, execID.ID, container.ResizeOptions{
				Height: uint(size.Height),
				Width:  uint(size.Width),
			})
		}
	}

	// Handle I/O
	outputDone := make(chan error)
	go func() {
		_, err := io.Copy(os.Stdout, attachResp.Reader)
		outputDone <- err
	}()

	go func() {
		io.Copy(attachResp.Conn, os.Stdin)
		attachResp.CloseWrite()
	}()

	if err := <-outputDone; err != nil {
		return fmt.Errorf("error in output processing: %v", err)
	}
	return nil
}

// execCommandInContainer creates an exec session in the container with a shell and attaches to it.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string contid container ID
//	in(4): string WorkingDir working directory
func execCommandInContainer(ctx context.Context, cli *client.Client, contid, WorkingDir string) {
	execShell := []string{}
	if containerCfg.shell != "" {
		execShell = append(execShell, strings.Split(containerCfg.shell, " ")...)
	}

	optionsCreate := container.ExecOptions{
		WorkingDir:   WorkingDir,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Detach:       false,
		Privileged:   true,
		Tty:          true,
		Cmd:          execShell,
	}

	rst, err := cli.ContainerExecCreate(ctx, contid, optionsCreate)
	if err != nil {
		panic(err)
	}

	optionsStartCheck := container.ExecStartOptions{
		Detach: false,
		Tty:    true,
	}

	response, err := cli.ContainerExecAttach(ctx, rst.ID, optionsStartCheck)
	if err != nil {
		panic(err)
	}
	defer response.Close()

	handleIOStreams(response)
	waitForContainer(ctx, cli, contid)
}

// attachAndInteract attaches to a running container and sets up interactive I/O with TTY resizing.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string contid container ID
func attachAndInteract(ctx context.Context, cli *client.Client, contid string) {
	response, err := cli.ContainerAttach(ctx, contid, container.AttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  true,
		Stream: true,
	})
	if err != nil {
		panic(err)
	}
	defer response.Close()

	handleIOStreams(response)

	fd := int(os.Stdin.Fd())
	if terminal.IsTerminal(fd) {
		oldState, err := terminal.MakeRaw(fd)
		if err != nil {
			panic(err)
		}
		defer terminal.Restore(fd, oldState)

		go resizeTty(ctx, cli, contid, fd)
		go readAndWriteInput(response)
	}

	waitForContainer(ctx, cli, contid)
}

// handleIOStreams sets up goroutines to copy stdout, stderr, and stdin between the terminal and the container.
//
//	in(1): types.HijackedResponse response
func handleIOStreams(response types.HijackedResponse) {
	go io.Copy(os.Stdout, response.Reader)
	go io.Copy(os.Stderr, response.Reader)
	go io.Copy(response.Conn, os.Stdin)
}

// readAndWriteInput reads bytes from stdin and writes them to the container connection.
//
//	in(1): types.HijackedResponse response
func readAndWriteInput(response types.HijackedResponse) {
	reader := bufio.NewReaderSize(os.Stdin, 4096) // Increased buffer size for larger inputs
	for {
		input, err := reader.ReadByte()
		if err != nil {
			return
		}
		response.Conn.Write([]byte{input})
	}
}

// waitForContainer blocks until the container exits or an error occurs.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string contid container ID
func waitForContainer(ctx context.Context, cli *client.Client, contid string) {
	statusCh, errCh := cli.ContainerWait(ctx, contid, container.WaitConditionNextExit)
	select {
	case err := <-errCh:
		if err != nil {
			panic(err)
		}
	case <-statusCh:
	}
}

// ContainerCommit commits the current state of a container as a new image.
//
//	in(1): string contid container ID
func ContainerCommit(contid string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	if err := cli.ContainerStart(ctx, contid, container.StartOptions{}); err != nil {
		panic(err)
	}

	commitResp, err := cli.ContainerCommit(ctx, contid, container.CommitOptions{Reference: containerCfg.imagename})
	if err != nil {
		panic(err)
	}
	fmt.Println(commitResp.ID)
}

// findContainerByIdentifier searches the container list for a container matching by ID or name.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string identifier container ID or name
//	out: types.Container, error
func findContainerByIdentifier(ctx context.Context, cli *client.Client, identifier string) (types.Container, error) {
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return types.Container{}, err
	}

	for _, c := range containers {
		if c.ID == identifier || (len(c.Names) > 0 && c.Names[0] == "/"+identifier) {
			return c, nil
		}
	}
	return types.Container{}, fmt.Errorf("container with ID or name '%s' not found", identifier)
}

// ContainerRename renames an existing container identified by ID or name.
//
//	in(1): string currentIdentifier current container ID or name
//	in(2): string newName new container name
func ContainerRename(currentIdentifier string, newName string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	found, err := findContainerByIdentifier(ctx, cli, currentIdentifier)
	if err != nil {
		log.Fatalf("Container with ID or name '%s' not found.", currentIdentifier)
	}

	// Rename the container
	err = cli.ContainerRename(ctx, found.ID, newName)
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("[+] Container '%s' renamed to '%s'!\n", currentIdentifier, newName)
	}
}

// ContainerRemove removes a container by ID or name, including any associated temp images.
//
//	in(1): string containerIdentifier container ID or name
func ContainerRemove(containerIdentifier string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	found, err := findContainerByIdentifier(ctx, cli, containerIdentifier)
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	containerImage := found.Image

	// Check for associated NAT network before removing the container
	containerJSON, inspectErr := cli.ContainerInspect(ctx, found.ID)
	hasNATNetwork := false
	natContainerName := ""
	if inspectErr == nil {
		if containerJSON.Config != nil && containerJSON.Config.Labels["org.rfswift.nat_network"] != "" {
			hasNATNetwork = true
			// Extract container name for network cleanup
			if len(found.Names) > 0 {
				natContainerName = found.Names[0]
				if len(natContainerName) > 0 && natContainerName[0] == '/' {
					natContainerName = natContainerName[1:]
				}
			}
		}
	}

	// Remove the container
	err = cli.ContainerRemove(ctx, found.ID, container.RemoveOptions{Force: true})
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' removed successfully", containerIdentifier))

	// Clean up associated NAT network (only if not shared or empty)
	if hasNATNetwork {
		natNetName := ""
		if containerJSON.Config != nil {
			natNetName = containerJSON.Config.Labels["org.rfswift.nat_network"]
		}
		if natNetName != "" {
			if isSharedNATNetwork(ctx, cli, natNetName) {
				remaining := countContainersOnNetwork(ctx, cli, natNetName)
				if remaining == 0 {
					removeNATNetworkByFullName(ctx, cli, natNetName)
				} else {
					common.PrintInfoMessage(fmt.Sprintf("NAT network '%s' still has %d container(s), keeping it", natNetName, remaining))
				}
			} else if natContainerName != "" {
				removeNATNetwork(ctx, cli, natContainerName)
			}
		}
	}

	// Clean up associated temp image if any
	tempPattern := regexp.MustCompile(`_temp_\d{14}`)
	if tempPattern.MatchString(containerImage) {
		_, err := cli.ImageRemove(ctx, containerImage, image.RemoveOptions{Force: false})
		if err == nil {
			common.PrintSuccessMessage(fmt.Sprintf("Cleaned up temp image: %s", containerImage))
		}
	}
}

// ContainerInstallScript runs an installation script inside a container with apt setup and ldconfig.
//
//	in(1): string containerIdentifier container ID or name
//	in(2): string scriptName script file name
//	in(3): string functionScript function to call in the script
//	out: error
func ContainerInstallScript(containerIdentifier, scriptName, functionScript string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Check if the container is running; if not, start it
	containerJSON, err := cli.ContainerInspect(ctx, containerIdentifier)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %v", err)
	}

	if containerJSON.State.Status != "running" {
		if err := cli.ContainerStart(ctx, containerIdentifier, container.StartOptions{}); err != nil {
			return fmt.Errorf("failed to start container: %v", err)
		}
	}

	// Step 1: Run "apt update" with clock-based loading indicator
	common.PrintInfoMessage("Running 'apt update'...")
	if err := showLoadingIndicator(ctx, func() error {
		return execCommand(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", "apt update"})
	}, "apt update"); err != nil {
		return err
	}

	// Step 2: Run "apt --fix-broken install" with clock-based loading indicator
	common.PrintInfoMessage("Running 'apt --fix-broken install'...")
	if err := showLoadingIndicator(ctx, func() error {
		return execCommand(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", "apt --fix-broken install -y"})
	}, "apt --fix-broken install"); err != nil {
		return err
	}

	// Step 3: Run the provided script with clock-based loading indicator
	common.PrintInfoMessage(fmt.Sprintf("Running script './%s %s'...", scriptName, functionScript))
	if err := showLoadingIndicator(ctx, func() error {
		return execCommand(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", fmt.Sprintf("./%s %s", scriptName, functionScript)}, "/root/scripts")
	}, fmt.Sprintf("script './%s %s'", scriptName, functionScript)); err != nil {
		return err
	}

	// Step 4: Run "ldconfig"
	common.PrintInfoMessage("Running 'ldconfig'...")
	if err := showLoadingIndicator(ctx, func() error {
		return execCommand(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", "ldconfig"})
	}, "ldconfig"); err != nil {
		return err
	}

	return nil
}

// execCommand executes a command in the container, capturing only errors if any.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string containerID
//	in(4): []string cmd command to execute
//	in(5): ...string workingDir optional working directory
//	out: error
func execCommand(ctx context.Context, cli *client.Client, containerID string, cmd []string, workingDir ...string) error {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	// Optional working directory
	if len(workingDir) > 0 {
		execConfig.WorkingDir = workingDir[0]
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec instance: %v", err)
	}

	attachResp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("failed to attach to exec instance: %v", err)
	}
	defer attachResp.Close()

	// Capture only error messages, suppressing standard output
	_, err = io.Copy(io.Discard, attachResp.Reader)
	return err
}

// execCommandWithOutput executes a command and returns its output.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string containerID
//	in(4): []string cmd command to execute
//	out: (string, error)
func execCommandWithOutput(ctx context.Context, cli *client.Client, containerID string, cmd []string) (string, error) {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	execID, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec instance: %v", err)
	}

	attachResp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec instance: %v", err)
	}
	defer attachResp.Close()

	// Read the output
	output, err := io.ReadAll(attachResp.Reader)
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// ContainerStop stops a running container, using the latest RF Swift container if none is specified.
//
//	in(1): string containerIdentifier container ID or name
func ContainerStop(containerIdentifier string) {
	ctx := context.Background()

	// Initialize Docker client
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	// Retrieve the latest container if no identifier is provided
	if containerIdentifier == "" {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		containerIdentifier = latestDockerID(labelKey, labelValue)
		if containerIdentifier == "" {
			common.PrintErrorMessage(fmt.Errorf("no running containers found with label %s=%s", labelKey, labelValue))
			return
		}
	}

	// Inspect the container to get its current state
	containerJSON, err := cli.ContainerInspect(ctx, containerIdentifier)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return
	}

	containerName := strings.TrimPrefix(containerJSON.Name, "/")
	if !containerJSON.State.Running {
		common.PrintSuccessMessage(fmt.Sprintf("Container '%s' is already stopped", containerName))
		return
	}

	// Stop the container
	timeout := 10 // Grace period in seconds before force stop
	if err := cli.ContainerStop(ctx, containerIdentifier, container.StopOptions{Timeout: &timeout}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to stop container: %v", err))
		return
	}

	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' stopped successfully", containerName))
}
