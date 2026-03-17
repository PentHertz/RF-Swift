/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Container upgrade: migrate a container to a new image while preserving data
 */

package dock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/term"

	common "penthertz/rfswift/common"
)

// ContainerUpgrade migrates an existing container to a new image while preserving
// specified directories and all original host bindings, network settings, and
// device mappings. The old container is committed as a timestamped backup image
// before removal. If containerIdentifier is empty the most recently used
// rfswift container is selected automatically. If newImage is empty the current
// image repository is re-pulled at the "latest" tag.
//
//	in(1): string containerIdentifier - name or ID of the container to upgrade (empty = auto-detect)
//	in(2): string repositoriesToPreserve - comma-separated list of in-container paths to back up and restore
//	in(3): string newImage - target image reference to upgrade to (empty = current repo:latest)
//	out: error - non-nil if any step of the upgrade process fails
func ContainerUpgrade(containerIdentifier string, repositoriesToPreserve string, newImage string) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return err
	}
	defer cli.Close()

	// Get latest container if not specified
	if containerIdentifier == "" {
		labelKey := "org.container.project"
		labelValue := "rfswift"
		containerIdentifier = latestDockerID(labelKey, labelValue)
		if containerIdentifier == "" {
			return fmt.Errorf("no container found with label")
		}
	}

	// Get container info
	containerJSON, err := cli.ContainerInspect(ctx, containerIdentifier)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to inspect container: %v", err))
		return err
	}

	containerName := strings.TrimPrefix(containerJSON.Name, "/")
	originalImage := containerJSON.Config.Image

	// Determine new image
	if newImage == "" {
		// If no image specified, use the current image's latest version
		repo, _ := parseImageName(originalImage)
		newImage = fmt.Sprintf("%s:latest", repo)
	} else {
		// Normalize the provided image name
		newImage = normalizeImageName(newImage)
	}

	common.PrintInfoMessage("═══════════════════════════════════════")
	common.PrintInfoMessage("       Container Upgrade Process")
	common.PrintInfoMessage("═══════════════════════════════════════")
	fmt.Printf("  Container: %s\n", containerName)
	fmt.Printf("  Current image: %s\n", originalImage)
	fmt.Printf("  Target image: %s\n", newImage)
	common.PrintInfoMessage("═══════════════════════════════════════")
	fmt.Println()

	// STEP 1: Check if new image exists locally, if not pull it FIRST
	// This prevents removing the old container if the pull fails
	_, err = ImageInspectCompat(ctx, cli, newImage)
	if err != nil {
		common.PrintInfoMessage(fmt.Sprintf("Pulling image '%s'...", newImage))

		// Parse image for pulling
		parts := strings.Split(newImage, ":")
		repo := parts[0]
		tag := "latest"
		if len(parts) > 1 {
			tag = parts[1]
		}

		// Pull the image
		architecture := getArchitecture()
		isOfficial := IsOfficialImage(newImage)
		actualPullRef := newImage

		if isOfficial && architecture != "" {
			hasArchSuffix := strings.HasSuffix(tag, "_amd64") ||
				strings.HasSuffix(tag, "_arm64") ||
				strings.HasSuffix(tag, "_riscv64") ||
				strings.HasSuffix(tag, "_arm")

			if !hasArchSuffix {
				actualPullRef = fmt.Sprintf("%s:%s_%s", repo, tag, architecture)
			}
		}

		out, err := cli.ImagePull(ctx, actualPullRef, image.PullOptions{})
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to pull image: %v", err))
			common.PrintInfoMessage("Old container preserved - no changes made")
			return err
		}
		defer out.Close()

		// Process pull output
		fd, isTerminal := term.GetFdInfo(os.Stdout)
		jsonDecoder := json.NewDecoder(out)
		for {
			var msg jsonmessage.JSONMessage
			if err := jsonDecoder.Decode(&msg); err == io.EOF {
				break
			} else if err != nil {
				common.PrintErrorMessage(err)
				common.PrintInfoMessage("Old container preserved - no changes made")
				return err
			}
			if isTerminal {
				_ = jsonmessage.DisplayJSONMessagesStream(out, os.Stdout, fd, isTerminal, nil)
			}
		}

		// Tag if needed
		if newImage != actualPullRef && IsOfficialImage(actualPullRef) {
			remoteInspect, _, _ := cli.ImageInspectWithRaw(ctx, actualPullRef)
			if remoteInspect.ID != "" {
				cli.ImageTag(ctx, remoteInspect.ID, newImage)
				cli.ImageRemove(ctx, actualPullRef, image.RemoveOptions{Force: false})
			}
		}

		common.PrintSuccessMessage(fmt.Sprintf("Image '%s' pulled successfully", newImage))
		fmt.Println()
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Using local image: %s", newImage))
		fmt.Println()
	}

	// Parse repositories to preserve
	var reposToCopy []string
	if repositoriesToPreserve != "" {
		reposToCopy = strings.Split(repositoriesToPreserve, ",")
		for i, repo := range reposToCopy {
			reposToCopy[i] = strings.TrimSpace(repo)
		}
		common.PrintInfoMessage("Repositories/directories to preserve:")
		for _, repo := range reposToCopy {
			fmt.Printf("  • %s\n", repo)
		}
		fmt.Println()
	}

	// Create temporary directory to store preserved data
	var tempDir string
	var preservedData = make(map[string]string) // map[containerPath]hostTempPath

	if len(reposToCopy) > 0 {
		tempDir, err = os.MkdirTemp("", "rfswift-upgrade-*")
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to create temp directory: %v", err))
			return err
		}
		defer os.RemoveAll(tempDir) // Clean up on exit

		common.PrintInfoMessage(fmt.Sprintf("Created temporary storage: %s", tempDir))

		// Ensure container is running before copying
		if !containerJSON.State.Running {
			common.PrintInfoMessage("Starting container to copy data...")
			if err := cli.ContainerStart(ctx, containerIdentifier, container.StartOptions{}); err != nil {
				common.PrintErrorMessage(fmt.Errorf("failed to start container: %v", err))
				return err
			}
		}

		// Copy data from old container to temp directory
		for _, repoPath := range reposToCopy {
			common.PrintInfoMessage(fmt.Sprintf("Backing up: %s", repoPath))

			// Create subdirectory in temp for this path
			safeName := strings.ReplaceAll(strings.Trim(repoPath, "/"), "/", "_")
			hostPath := filepath.Join(tempDir, safeName)

			// Check if directory exists in container
			checkCmd := fmt.Sprintf("[ -d '%s' ] && echo 'exists' || echo 'not_found'", repoPath)
			exists, err := execCommandWithOutput(ctx, cli, containerIdentifier, []string{"/bin/bash", "-c", checkCmd})
			if err != nil || !strings.Contains(exists, "exists") {
				common.PrintWarningMessage(fmt.Sprintf("Directory '%s' not found in container, skipping", repoPath))
				continue
			}

			// Use docker cp to copy from container to host
			reader, _, err := cli.CopyFromContainer(ctx, containerIdentifier, repoPath)
			if err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to copy %s: %v", repoPath, err))
				continue
			}
			defer reader.Close()

			// Create the host directory
			if err := os.MkdirAll(hostPath, 0755); err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to create directory %s: %v", hostPath, err))
				continue
			}

			// Extract the tar archive
			if err := extractTarArchive(reader, hostPath); err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to extract %s: %v", repoPath, err))
				continue
			}

			preservedData[repoPath] = hostPath
			common.PrintSuccessMessage(fmt.Sprintf("Backed up: %s → %s", repoPath, hostPath))
		}
		fmt.Println()
	}

	// Create backup of current container
	currentTime := time.Now()
	backupTag := fmt.Sprintf("%s-backup-%02d%02d%d-%02d%02d%02d",
		originalImage,
		currentTime.Day(),
		currentTime.Month(),
		currentTime.Year(),
		currentTime.Hour(),
		currentTime.Minute(),
		currentTime.Second())

	common.PrintInfoMessage("Creating backup of current container...")
	_, err = cli.ContainerCommit(ctx, containerIdentifier, container.CommitOptions{
		Reference: backupTag,
	})
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to create backup: %v", err))
		return err
	}
	common.PrintSuccessMessage(fmt.Sprintf("Backup created: %s", backupTag))

	// Stop the container
	common.PrintInfoMessage("Stopping container...")
	timeout := 10
	if err := cli.ContainerStop(ctx, containerIdentifier, container.StopOptions{Timeout: &timeout}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to stop container: %v", err))
		return err
	}
	common.PrintSuccessMessage("Container stopped")

	// Remove the old container
	common.PrintInfoMessage("Removing old container...")
	if err := cli.ContainerRemove(ctx, containerIdentifier, container.RemoveOptions{Force: true}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to remove container: %v", err))
		return err
	}
	common.PrintSuccessMessage("Old container removed")

	// Get original container properties
	props := make(map[string]string)
	props["Shell"] = containerJSON.Path
	if props["Shell"] == "" {
		props["Shell"] = "/bin/bash"
	}
	props["Privileged"] = fmt.Sprintf("%v", containerJSON.HostConfig.Privileged)
	props["NetworkMode"] = string(containerJSON.HostConfig.NetworkMode)
	props["ExposedPorts"] = convertExposedPortsToString(containerJSON.Config.ExposedPorts)
	props["PortBindings"] = convertPortBindingsToRoundTrip(containerJSON.HostConfig.PortBindings)
	props["ExtraHosts"] = strings.Join(containerJSON.HostConfig.ExtraHosts, ",")
	props["Devices"] = convertDevicesToString(containerJSON.HostConfig.Devices)
	props["Caps"] = convertCapsToString(containerJSON.HostConfig.CapAdd)
	props["Seccomp"] = convertSecurityOptToString(containerJSON.HostConfig.SecurityOpt)
	props["Cgroups"] = strings.Join(containerJSON.HostConfig.DeviceCgroupRules, ",")
	props["XDisplay"] = ":0"
	for _, env := range containerJSON.Config.Env {
		if strings.HasPrefix(env, "DISPLAY=") {
			props["XDisplay"] = strings.TrimPrefix(env, "DISPLAY=")
			break
		}
	}

	// Preserve all existing bindings
	bindingsToKeep := containerJSON.HostConfig.Binds

	// Create new container with new image
	common.PrintInfoMessage("Creating new container with upgraded image...")

	extrahosts := []string{}
	if props["ExtraHosts"] != "" {
		extrahosts = strings.Split(props["ExtraHosts"], ",")
	}

	dockerenv := []string{fmt.Sprintf("DISPLAY=%s", props["XDisplay"])}
	if containerCfg.pulseServer != "" {
		dockerenv = append(dockerenv, "PULSE_SERVER="+containerCfg.pulseServer)
	}

	exposedPorts := ParseExposedPorts(props["ExposedPorts"])
	bindedPorts := ParseBindedPorts(props["PortBindings"])
	devices := getDeviceMappingsFromString(props["Devices"])

	privileged := props["Privileged"] == "true"

	// Create HostConfig WITHOUT Devices in the literal
	hostConfig := &container.HostConfig{
		NetworkMode:  container.NetworkMode(props["NetworkMode"]),
		Binds:        bindingsToKeep,
		ExtraHosts:   extrahosts,
		PortBindings: bindedPorts,
		Privileged:   privileged,
	}

	// Set devices and other settings based on privilege mode
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

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:        newImage,
		Cmd:          []string{props["Shell"]},
		Env:          dockerenv,
		ExposedPorts: exposedPorts,
		OpenStdin:    true,
		StdinOnce:    false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Labels: map[string]string{
			"org.container.project": "rfswift",
		},
	}, hostConfig, &network.NetworkingConfig{}, nil, containerName)

	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to create new container: %v", err))
		common.PrintWarningMessage(fmt.Sprintf("Restore from backup: docker run %s", backupTag))
		return err
	}

	common.PrintSuccessMessage(fmt.Sprintf("New container '%s' created", containerName))

	// Start the new container
	common.PrintInfoMessage("Starting new container...")
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to start new container: %v", err))
		return err
	}
	common.PrintSuccessMessage(fmt.Sprintf("Container '%s' started successfully", containerName))

	// Restore preserved data to new container
	if len(preservedData) > 0 {
		fmt.Println()
		common.PrintInfoMessage("Restoring preserved repositories to new container...")

		for containerPath, hostPath := range preservedData {
			common.PrintInfoMessage(fmt.Sprintf("Restoring: %s", containerPath))

			// Create tar archive from host path
			tarReader, err := createTarArchive(hostPath, containerPath)
			if err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to create archive for %s: %v", containerPath, err))
				continue
			}

			// Copy to new container
			err = cli.CopyToContainer(ctx, resp.ID, filepath.Dir(containerPath), tarReader, container.CopyToContainerOptions{})
			tarReader.Close()

			if err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to restore %s: %v", containerPath, err))
				continue
			}

			common.PrintSuccessMessage(fmt.Sprintf("Restored: %s", containerPath))
		}
	}

	// Print summary
	fmt.Println()
	common.PrintInfoMessage("═══════════════════════════════════════")
	common.PrintSuccessMessage("✓ Container upgrade completed!")
	fmt.Printf("  Container: %s\n", containerName)
	fmt.Printf("  Old image: %s\n", originalImage)
	fmt.Printf("  New image: %s\n", newImage)
	fmt.Printf("  Backup: %s\n", backupTag)
	fmt.Printf("  Host bindings: %d\n", len(bindingsToKeep))
	fmt.Printf("  Repositories restored: %d\n", len(preservedData))
	common.PrintInfoMessage("═══════════════════════════════════════")

	return nil
}
