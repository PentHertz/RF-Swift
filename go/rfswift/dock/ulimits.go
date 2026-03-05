/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package dock

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"

	common "penthertz/rfswift/common"
)

// parseUlimitsFromString parses a comma-separated ulimit string into a slice of Docker ulimit structs.
// Accepted formats per entry: "name=value" (soft==hard) or "name=soft:hard".
// The special values -1 and "unlimited" are both treated as unlimited (-1).
//
//	in(1): string ulimitsStr comma-separated ulimit definitions (e.g. "rtprio=95,memlock=-1:-1")
//	out: []*container.Ulimit parsed ulimit structs; empty slice when input is empty
func parseUlimitsFromString(ulimitsStr string) []*container.Ulimit {
	var ulimits []*container.Ulimit

	if ulimitsStr == "" {
		return ulimits
	}

	entries := strings.Split(ulimitsStr, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			common.PrintWarningMessage(fmt.Sprintf("Invalid ulimit format: %s (expected name=value)", entry))
			continue
		}

		name := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		var soft, hard int64

		if strings.Contains(valueStr, ":") {
			valueParts := strings.Split(valueStr, ":")
			if len(valueParts) != 2 {
				common.PrintWarningMessage(fmt.Sprintf("Invalid ulimit value format: %s", valueStr))
				continue
			}

			var err error
			if valueParts[0] == "-1" || valueParts[0] == "unlimited" {
				soft = -1
			} else {
				soft, err = strconv.ParseInt(valueParts[0], 10, 64)
				if err != nil {
					common.PrintWarningMessage(fmt.Sprintf("Invalid soft limit: %s", valueParts[0]))
					continue
				}
			}

			if valueParts[1] == "-1" || valueParts[1] == "unlimited" {
				hard = -1
			} else {
				hard, err = strconv.ParseInt(valueParts[1], 10, 64)
				if err != nil {
					common.PrintWarningMessage(fmt.Sprintf("Invalid hard limit: %s", valueParts[1]))
					continue
				}
			}
		} else {
			var err error
			if valueStr == "-1" || valueStr == "unlimited" {
				soft = -1
				hard = -1
			} else {
				soft, err = strconv.ParseInt(valueStr, 10, 64)
				if err != nil {
					common.PrintWarningMessage(fmt.Sprintf("Invalid ulimit value: %s", valueStr))
					continue
				}
				hard = soft
			}
		}

		ulimits = append(ulimits, &container.Ulimit{
			Name: name,
			Soft: soft,
			Hard: hard,
		})
	}

	return ulimits
}

// convertUlimitsToString converts a slice of Docker ulimit structs back into a comma-separated string.
// When soft equals hard the entry is written as "name=value"; otherwise "name=soft:hard".
// A soft or hard value of -1 is rendered as "unlimited".
//
//	in(1): []*container.Ulimit ulimits ulimit structs to serialise
//	out: string comma-separated ulimit string, or empty string when the slice is empty
func convertUlimitsToString(ulimits []*container.Ulimit) string {
	if len(ulimits) == 0 {
		return ""
	}

	var parts []string
	for _, ulimit := range ulimits {
		if ulimit.Soft == ulimit.Hard {
			if ulimit.Soft == -1 {
				parts = append(parts, fmt.Sprintf("%s=unlimited", ulimit.Name))
			} else {
				parts = append(parts, fmt.Sprintf("%s=%d", ulimit.Name, ulimit.Soft))
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
			parts = append(parts, fmt.Sprintf("%s=%s:%s", ulimit.Name, softStr, hardStr))
		}
	}
	return strings.Join(parts, ",")
}

// getRealtimeUlimits returns the fixed set of ulimits required for realtime SDR operations:
// rtprio=95, memlock=unlimited, and nice=40.
//
//	out: []*container.Ulimit hardcoded realtime ulimit structs
func getRealtimeUlimits() []*container.Ulimit {
	return []*container.Ulimit{
		{
			Name: "rtprio",
			Soft: 95,
			Hard: 95,
		},
		{
			Name: "memlock",
			Soft: -1,
			Hard: -1,
		},
		{
			Name: "nice",
			Soft: 40,
			Hard: 40,
		},
	}
}

// getUlimitsForContainer prepares the final ulimit slice for container creation by merging
// realtime ulimits (when containerCfg.realtime is set) with any custom ulimits from containerCfg.ulimits.
// Custom entries take precedence over realtime defaults when names collide.
// As a side-effect, SYS_NICE is appended to containerCfg.caps when realtime mode is active.
//
//	out: []*container.Ulimit merged ulimit slice ready to pass to the container host config
func getUlimitsForContainer() []*container.Ulimit {
	var ulimits []*container.Ulimit

	if containerCfg.realtime {
		ulimits = append(ulimits, getRealtimeUlimits()...)

		if !strings.Contains(containerCfg.caps, "SYS_NICE") {
			if containerCfg.caps == "" {
				containerCfg.caps = "SYS_NICE"
			} else {
				containerCfg.caps = containerCfg.caps + ",SYS_NICE"
			}
		}
		common.PrintInfoMessage("Realtime mode enabled: rtprio=95, memlock=unlimited, nice=40, SYS_NICE capability")
	}

	if containerCfg.ulimits != "" {
		customUlimits := parseUlimitsFromString(containerCfg.ulimits)

		for _, custom := range customUlimits {
			found := false
			for i, existing := range ulimits {
				if existing.Name == custom.Name {
					ulimits[i] = custom
					found = true
					break
				}
			}
			if !found {
				ulimits = append(ulimits, custom)
			}
		}
	}

	return ulimits
}

// UpdateUlimit adds, updates, or removes a single ulimit on an existing container.
// The container is recreated with the modified ulimit set to apply the change.
//
//	in(1): string containerID target container ID or name
//	in(2): string ulimitName ulimit resource name (e.g. "rtprio", "memlock")
//	in(3): string ulimitValue ulimit value in "value" or "soft:hard" format (used only when add is true)
//	in(4): bool add true to add or update the ulimit, false to remove it
//	out: error nil on success, or an error describing the failure
func UpdateUlimit(containerID string, ulimitName string, ulimitValue string, add bool) error {
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

	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	existingUlimits := parseUlimitsFromString(props["Ulimits"])

	if add {
		ulimitEntry := fmt.Sprintf("%s=%s", ulimitName, ulimitValue)

		found := false
		for i, ul := range existingUlimits {
			if ul.Name == ulimitName {
				newUlimits := parseUlimitsFromString(ulimitEntry)
				if len(newUlimits) > 0 {
					existingUlimits[i] = newUlimits[0]
				}
				found = true
				common.PrintInfoMessage(fmt.Sprintf("Updating ulimit '%s' to '%s' on container '%s'", ulimitName, ulimitValue, containerName))
				break
			}
		}

		if !found {
			newUlimits := parseUlimitsFromString(ulimitEntry)
			existingUlimits = append(existingUlimits, newUlimits...)
			common.PrintInfoMessage(fmt.Sprintf("Adding ulimit '%s=%s' to container '%s'", ulimitName, ulimitValue, containerName))
		}
	} else {
		newUlimits := []*container.Ulimit{}
		found := false
		for _, ul := range existingUlimits {
			if ul.Name != ulimitName {
				newUlimits = append(newUlimits, ul)
			} else {
				found = true
			}
		}

		if !found {
			common.PrintWarningMessage(fmt.Sprintf("Ulimit '%s' not found in container '%s'", ulimitName, containerName))
			return nil
		}

		existingUlimits = newUlimits
		common.PrintInfoMessage(fmt.Sprintf("Removing ulimit '%s' from container '%s'", ulimitName, containerName))
	}

	props["Ulimits"] = convertUlimitsToString(existingUlimits)

	return recreateContainerWithProperties(ctx, cli, containerID, props)
}

// EnableRealtimeMode enables realtime scheduling on an existing container by adding the
// SYS_NICE capability and the realtime ulimits (rtprio=95, memlock=unlimited, nice=40).
// The container is recreated to apply the changes.
//
//	in(1): string containerID target container ID or name
//	out: error nil on success, or an error describing the failure
func EnableRealtimeMode(containerID string) error {
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

	common.PrintInfoMessage(fmt.Sprintf("Enabling realtime mode on container '%s'", containerName))
	common.PrintInfoMessage("This will add: SYS_NICE capability, rtprio=95, memlock=unlimited, nice=40")

	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	caps := props["Caps"]
	if !strings.Contains(caps, "SYS_NICE") {
		if caps == "" {
			caps = "SYS_NICE"
		} else {
			caps = caps + ",SYS_NICE"
		}
		props["Caps"] = caps
	}

	existingUlimits := parseUlimitsFromString(props["Ulimits"])
	realtimeUlimits := getRealtimeUlimits()

	for _, rtUlimit := range realtimeUlimits {
		found := false
		for i, existing := range existingUlimits {
			if existing.Name == rtUlimit.Name {
				existingUlimits[i] = rtUlimit
				found = true
				break
			}
		}
		if !found {
			existingUlimits = append(existingUlimits, rtUlimit)
		}
	}

	props["Ulimits"] = convertUlimitsToString(existingUlimits)

	err = recreateContainerWithProperties(ctx, cli, containerID, props)
	if err != nil {
		return err
	}

	common.PrintSuccessMessage("Realtime mode enabled successfully!")
	common.PrintInfoMessage("You can now use chrt and nice commands inside the container for SDR operations")
	common.PrintInfoMessage("Test with: ulimit -r (should show 95)")
	return nil
}

// DisableRealtimeMode disables realtime scheduling on an existing container by removing the
// SYS_NICE capability and the realtime ulimits (rtprio, memlock, nice).
// The container is recreated to apply the changes.
//
//	in(1): string containerID target container ID or name
//	out: error nil on success, or an error describing the failure
func DisableRealtimeMode(containerID string) error {
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

	common.PrintInfoMessage(fmt.Sprintf("Disabling realtime mode on container '%s'", containerName))

	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to get container properties: %v", err))
		return err
	}

	caps := strings.Split(props["Caps"], ",")
	newCaps := []string{}
	for _, cap := range caps {
		cap = strings.TrimSpace(cap)
		if cap != "SYS_NICE" && cap != "" {
			newCaps = append(newCaps, cap)
		}
	}
	props["Caps"] = strings.Join(newCaps, ",")

	existingUlimits := parseUlimitsFromString(props["Ulimits"])
	realtimeNames := map[string]bool{"rtprio": true, "memlock": true, "nice": true}

	newUlimits := []*container.Ulimit{}
	for _, ul := range existingUlimits {
		if !realtimeNames[ul.Name] {
			newUlimits = append(newUlimits, ul)
		}
	}

	props["Ulimits"] = convertUlimitsToString(newUlimits)

	err = recreateContainerWithProperties(ctx, cli, containerID, props)
	if err != nil {
		return err
	}

	common.PrintSuccessMessage("Realtime mode disabled successfully!")
	return nil
}

// ListContainerUlimits prints all ulimits configured on a container and reports whether
// realtime mode is considered active (SYS_NICE capability + rtprio + memlock=unlimited).
//
//	in(1): string containerID target container ID or name
//	out: error nil on success, or an error describing the failure
func ListContainerUlimits(containerID string) error {
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

	ulimits := containerJSON.HostConfig.Ulimits

	if len(ulimits) == 0 {
		common.PrintInfoMessage(fmt.Sprintf("Container '%s' has no custom ulimits set", containerName))
	} else {
		fmt.Printf("Ulimits for container '%s':\n", containerName)
		for _, ul := range ulimits {
			softStr := fmt.Sprintf("%d", ul.Soft)
			hardStr := fmt.Sprintf("%d", ul.Hard)
			if ul.Soft == -1 {
				softStr = "unlimited"
			}
			if ul.Hard == -1 {
				hardStr = "unlimited"
			}
			fmt.Printf("  • %s: soft=%s, hard=%s\n", ul.Name, softStr, hardStr)
		}
	}

	hasSysNice := false
	for _, cap := range containerJSON.HostConfig.CapAdd {
		if cap == "SYS_NICE" {
			hasSysNice = true
			break
		}
	}

	hasRtprio := false
	hasMemlock := false
	for _, ul := range ulimits {
		if ul.Name == "rtprio" && ul.Soft > 0 {
			hasRtprio = true
		}
		if ul.Name == "memlock" && ul.Soft == -1 {
			hasMemlock = true
		}
	}

	fmt.Println()
	if hasSysNice && hasRtprio && hasMemlock {
		common.PrintSuccessMessage("Realtime mode: ENABLED")
	} else {
		common.PrintInfoMessage("Realtime mode: DISABLED")
		if !hasSysNice {
			common.PrintInfoMessage("  - Missing SYS_NICE capability")
		}
		if !hasRtprio {
			common.PrintInfoMessage("  - Missing rtprio ulimit")
		}
		if !hasMemlock {
			common.PrintInfoMessage("  - Missing memlock=unlimited ulimit")
		}
	}

	return nil
}
