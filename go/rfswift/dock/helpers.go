/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package dock

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	common "penthertz/rfswift/common"
	"penthertz/rfswift/tui"
)

// loadJSON reads a JSON file from disk and unmarshals its contents into v.
//
//	in(1): string path - filesystem path to the JSON file to read
//	in(2): interface{} v - pointer to the value to unmarshal into
//	out: error - non-nil if reading or unmarshalling fails
func loadJSON(path string, v interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// saveJSON marshals v to indented JSON and writes it to the file at path.
//
//	in(1): string path - filesystem path of the file to write
//	in(2): interface{} v - value to marshal into JSON
//	out: error - non-nil if marshalling or writing fails
func saveJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

// ocontains reports whether item is present in slice.
//
//	in(1): []string slice - the slice to search
//	in(2): string item - the value to look for
//	out: bool - true if item is found, false otherwise
func ocontains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// removeFromSlice returns a new slice with all occurrences of item removed.
//
//	in(1): []string slice - the original slice
//	in(2): string item - the value to remove
//	out: []string - copy of slice with every matching element omitted
func removeFromSlice(slice []string, item string) []string {
	newSlice := []string{}
	for _, s := range slice {
		if s != item {
			newSlice = append(newSlice, s)
		}
	}
	return newSlice
}

// getContainerIDByName looks up a container's full ID by its name, searching
// all containers (including stopped ones). Returns an empty string if not found.
//
//	in(1): context.Context ctx - context used for the Docker API call
//	in(2): string containerName - container name to search for (without leading '/')
//	out: string - container ID, or empty string if no match is found
func getContainerIDByName(ctx context.Context, containerName string) string {
	cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
	for _, container := range containers {
		for _, name := range container.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				return container.ID
			}
		}
	}
	return ""
}

// combineBindings merges X11-forwarding bind mounts and extra bind mounts into
// a single slice. Each argument is a comma-separated list of bind mount specs.
//
//	in(1): string x11forward - comma-separated X11 socket bind mount specs
//	in(2): string extrabinding - comma-separated additional bind mount specs
//	out: []string - combined slice of all bind mount specs
func combineBindings(x11forward, extrabinding string) []string {
	var bindings []string

	if extrabinding != "" {
		bindings = append(bindings, strings.Split(extrabinding, ",")...)
	}
	if x11forward != "" {
		bindings = append(bindings, strings.Split(x11forward, ",")...)
	}
	return bindings
}

// splitAndCombine splits a comma-separated string into a slice of strings.
// Returns an empty slice when the input is empty.
//
//	in(1): string commaSeparated - comma-separated values to split
//	out: []string - individual values, or an empty slice if input is empty
func splitAndCombine(commaSeparated string) []string {
	if commaSeparated == "" {
		return []string{}
	}
	return strings.Split(commaSeparated, ",")
}

// combineEnv assembles the container environment variable slice from the X11
// display spec, PulseAudio server address, and any extra environment variables.
//
//	in(1): string xdisplay - comma-separated DISPLAY-related environment entries
//	in(2): string pulseServer - PulseAudio server address appended as PULSE_SERVER=<value>
//	in(3): string extraenv - comma-separated additional KEY=VALUE environment entries
//	out: []string - combined environment variable slice for the container
func combineEnv(xdisplay, pulseServer, extraenv string) []string {
	var dockerenv []string
	if xdisplay != "" {
		dockerenv = append(dockerenv, strings.Split(xdisplay, ",")...)
	}
	dockerenv = append(dockerenv, "PULSE_SERVER="+pulseServer)
	if extraenv != "" {
		dockerenv = append(dockerenv, strings.Split(extraenv, ",")...)
	}
	return dockerenv
}

// normalizeImageName ensures an image reference has the proper repo:tag format.
// If the name does not contain a colon, the configured default repository tag is
// prepended and an informational message is printed.
//
//	in(1): string imageName - raw image name, with or without a tag
//	out: string - fully-qualified image reference in repo:tag form
func normalizeImageName(imageName string) string {
	if imageName == "" {
		return imageName
	}
	if !strings.Contains(imageName, ":") {
		normalized := fmt.Sprintf("%s:%s", containerCfg.repotag, imageName)
		common.PrintInfoMessage(fmt.Sprintf("Using full image reference: %s", normalized))
		return normalized
	}
	return imageName
}

// parseImageName splits an image reference into its repository and tag parts.
// A leading "docker.io/" prefix is stripped before splitting. If no tag is
// present, "latest" is used as the default.
//
//	in(1): string imageName - image reference, optionally prefixed with "docker.io/"
//	out: string - repository portion of the image reference
//	out: string - tag portion of the image reference (defaults to "latest")
func parseImageName(imageName string) (string, string) {
	imageName = strings.TrimPrefix(imageName, "docker.io/")
	parts := strings.Split(imageName, ":")
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 {
		tag = parts[1]
	}
	return repo, tag
}

// bindExistsByPrefix reports whether a bind mount spec matching mount already
// exists in binds, ignoring trailing mount options (e.g., ":rw,rprivate,nosuid,rbind").
//
//	in(1): []string binds - current list of bind mount specs
//	in(2): string mount - the "src:dst" bind spec to look for
//	out: bool - true if a matching bind is found, false otherwise
func bindExistsByPrefix(binds []string, mount string) bool {
	for _, b := range binds {
		if b == mount || strings.HasPrefix(b, mount+":") {
			return true
		}
	}
	return false
}

// removeBindByPrefix returns a copy of binds with all entries that match mount
// (as an exact string or as a "mount:" prefix) removed, regardless of trailing
// mount options.
//
//	in(1): []string binds - current list of bind mount specs
//	in(2): string mount - the "src:dst" bind spec to remove
//	out: []string - filtered slice with matching entries omitted
func removeBindByPrefix(binds []string, mount string) []string {
	var result []string
	for _, b := range binds {
		if b == mount || strings.HasPrefix(b, mount+":") {
			continue
		}
		result = append(result, b)
	}
	return result
}

// IsRootlessPodman reports whether the current process is running under Podman
// without root privileges.
//
//	out: bool - true if the active engine is Podman and the effective UID is not 0
func IsRootlessPodman() bool {
	return GetEngine().Type() == EnginePodman && os.Getuid() != 0
}

// ParseExposedPorts parses a comma-separated list of port/protocol entries into
// a nat.PortSet suitable for use in container configuration.
//
//	in(1): string exposedPortsStr - comma-separated port specs (e.g. "80/tcp,443/tcp")
//	out: nat.PortSet - set of exposed ports, empty if input is empty
func ParseExposedPorts(exposedPortsStr string) nat.PortSet {
	exposedPorts := nat.PortSet{}

	if exposedPortsStr == "" {
		return exposedPorts
	}

	portEntries := strings.Split(exposedPortsStr, ",")
	for _, entry := range portEntries {
		port := strings.TrimSpace(entry)
		if port == "" {
			continue
		}
		exposedPorts[nat.Port(port)] = struct{}{}
	}

	return exposedPorts
}

// ParseBindedPorts parses a port binding string into a nat.PortMap. Entries are
// separated by ";;" when coming from an internal round-trip representation, or
// by "," when supplied directly from CLI input.
//
// Both Docker-standard format and internal format are accepted:
//   - Docker-standard: "hostPort:containerPort/proto" (e.g., "8080:80/tcp")
//   - Internal format: "containerPort/proto:hostPort" (e.g., "80/tcp:8080")
//   - With host IP:    "hostIP:hostPort:containerPort/proto" or "containerPort/proto:hostIP:hostPort"
//
//	in(1): string bindedPortsStr - delimited port binding specs
//	out: nat.PortMap - map of container ports to host bindings, empty on empty input
func ParseBindedPorts(bindedPortsStr string) nat.PortMap {
	portBindings := nat.PortMap{}

	if bindedPortsStr == "" || bindedPortsStr == "\"\"" {
		return portBindings
	}
	common.PrintSuccessMessage(fmt.Sprintf("Binded: '%s'", bindedPortsStr))

	var portEntries []string
	if strings.Contains(bindedPortsStr, ";;") {
		portEntries = strings.Split(bindedPortsStr, ";;")
	} else {
		portEntries = strings.Split(bindedPortsStr, ",")
	}
	for _, entry := range portEntries {
		entry = strings.TrimSpace(entry)
		parts := strings.Split(entry, ":")
		if len(parts) < 2 || len(parts) > 3 {
			fmt.Printf("Invalid port binding format: %s (expected hostPort:containerPort/proto or containerPort/proto:hostPort)\n", entry)
			continue
		}

		var containerPortProto, hostPort, hostAddress string

		// Detect format by checking which part contains "/proto"
		if strings.Contains(parts[0], "/") {
			// Internal format: containerPort/proto:hostPort or containerPort/proto:hostIP:hostPort
			containerPortProto = strings.TrimSpace(parts[0])
			if len(parts) == 3 {
				hostAddress = strings.TrimSpace(parts[1])
				hostPort = strings.TrimSpace(parts[2])
			} else {
				hostPort = strings.TrimSpace(parts[1])
			}
		} else if len(parts) == 2 && strings.Contains(parts[1], "/") {
			// Docker-standard 2-part: hostPort:containerPort/proto
			hostPort = strings.TrimSpace(parts[0])
			containerPortProto = strings.TrimSpace(parts[1])
		} else if len(parts) == 3 && strings.Contains(parts[2], "/") {
			// Docker-standard 3-part: hostIP:hostPort:containerPort/proto
			hostAddress = strings.TrimSpace(parts[0])
			hostPort = strings.TrimSpace(parts[1])
			containerPortProto = strings.TrimSpace(parts[2])
		} else {
			fmt.Printf("Invalid port binding format: %s (no port/protocol found, expected e.g. 80/tcp)\n", entry)
			continue
		}

		portKey := nat.Port(containerPortProto)

		portBindings[portKey] = append(portBindings[portKey], nat.PortBinding{
			HostIP:   hostAddress,
			HostPort: hostPort,
		})
	}

	return portBindings
}

// getDeviceMappingsFromString parses a comma-separated list of "hostPath:containerPath"
// device specs into a slice of container.DeviceMapping with "rwm" permissions.
//
//	in(1): string devicesStr - comma-separated device mapping specs (e.g. "/dev/sdr0:/dev/sdr0")
//	out: []container.DeviceMapping - parsed device mappings, empty if input is empty
func getDeviceMappingsFromString(devicesStr string) []container.DeviceMapping {
	var devices []container.DeviceMapping

	if devicesStr == "" {
		return devices
	}

	devicesList := strings.Split(devicesStr, ",")
	for _, deviceMapping := range devicesList {
		parts := strings.Split(deviceMapping, ":")
		if len(parts) == 2 {
			devices = append(devices, container.DeviceMapping{
				PathOnHost:        parts[0],
				PathInContainer:   parts[1],
				CgroupPermissions: "rwm",
			})
		}
	}

	return devices
}

// convertPortBindingsToString formats a nat.PortMap as a human-readable
// comma-separated string of "hostIP:hostPort -> containerPort/proto" entries.
//
//	in(1): nat.PortMap portBindings - port binding map to format
//	out: string - comma-separated human-readable port binding descriptions
func convertPortBindingsToString(portBindings nat.PortMap) string {
	var result []string

	for port, bindings := range portBindings {
		for _, binding := range bindings {
			entry := fmt.Sprintf("%s:%s -> %s", binding.HostIP, binding.HostPort, port)
			result = append(result, entry)
		}
	}

	return strings.Join(result, ", ")
}

// convertPortBindingsToRoundTrip serialises a nat.PortMap into the internal
// ";;" -delimited round-trip format so it can be stored and later re-parsed by
// ParseBindedPorts. Entries with a non-default host IP include it in the output.
//
//	in(1): nat.PortMap portBindings - port binding map to serialise
//	out: string - ";;" -delimited string of "containerPort/proto:hostPort" (or ":hostIP:hostPort") entries
func convertPortBindingsToRoundTrip(portBindings nat.PortMap) string {
	var result []string
	for port, bindings := range portBindings {
		for _, binding := range bindings {
			if binding.HostIP != "" && binding.HostIP != "0.0.0.0" {
				result = append(result, fmt.Sprintf("%s:%s:%s", port, binding.HostIP, binding.HostPort))
			} else {
				result = append(result, fmt.Sprintf("%s:%s", port, binding.HostPort))
			}
		}
	}
	return strings.Join(result, ";;")
}

// normalizePortBinding converts a port binding from Docker-standard format
// (hostPort:containerPort/proto) to the internal format (containerPort/proto:hostPort)
// if needed. If the binding is already in internal format, it is returned as-is.
//
//	in(1): string binding - port binding in either format
//	out: string - binding in internal format (containerPort/proto:hostPort)
func normalizePortBinding(binding string) string {
	parts := strings.Split(binding, ":")
	switch len(parts) {
	case 2:
		if strings.Contains(parts[0], "/") {
			// Already internal: containerPort/proto:hostPort
			return binding
		}
		if strings.Contains(parts[1], "/") {
			// Docker-standard: hostPort:containerPort/proto
			return parts[1] + ":" + parts[0]
		}
	case 3:
		if strings.Contains(parts[0], "/") {
			// Already internal: containerPort/proto:hostIP:hostPort
			return binding
		}
		if strings.Contains(parts[2], "/") {
			// Docker-standard: hostIP:hostPort:containerPort/proto
			return parts[2] + ":" + parts[0] + ":" + parts[1]
		}
	}
	return binding
}

// convertExposedPortsToString formats a nat.PortSet as a comma-separated string
// of port/protocol entries.
//
//	in(1): nat.PortSet exposedPorts - set of exposed ports to format
//	out: string - comma-separated port/protocol entries (e.g. "80/tcp, 443/tcp")
func convertExposedPortsToString(exposedPorts nat.PortSet) string {
	var result []string
	for port := range exposedPorts {
		result = append(result, string(port))
	}
	return strings.Join(result, ", ")
}

// convertDevicesToString formats a slice of container.DeviceMapping as a
// comma-separated string of "hostPath:containerPath" pairs.
//
//	in(1): []container.DeviceMapping devices - device mappings to format
//	out: string - comma-separated "hostPath:containerPath" device specs
func convertDevicesToString(devices []container.DeviceMapping) string {
	deviceStrings := make([]string, len(devices))
	for i, device := range devices {
		deviceStrings[i] = fmt.Sprintf("%s:%s", device.PathOnHost, device.PathInContainer)
	}
	return strings.Join(deviceStrings, ",")
}

// convertCapsToString joins a slice of Linux capability names into a single
// comma-separated string. Returns an empty string when the slice is empty.
//
//	in(1): []string caps - capability names (e.g. ["NET_ADMIN", "SYS_PTRACE"])
//	out: string - comma-separated capability names, or "" if caps is empty
func convertCapsToString(caps []string) string {
	if len(caps) == 0 {
		return ""
	}
	return strings.Join(caps, ",")
}

// convertSecurityOptToString extracts the seccomp profile path from a slice of
// security option strings. It returns the value of the first entry prefixed with
// "seccomp=", or an empty string if no such entry is found.
//
//	in(1): []string securityOpts - security option strings (e.g. ["seccomp=/path/to/profile.json"])
//	out: string - seccomp profile path, or "" if none is present
func convertSecurityOptToString(securityOpts []string) string {
	if len(securityOpts) == 0 {
		return ""
	}
	for _, opt := range securityOpts {
		if strings.HasPrefix(opt, "seccomp=") {
			return strings.TrimPrefix(opt, "seccomp=")
		}
	}
	return ""
}

// showLoadingIndicator runs commandFunc in a goroutine and displays a rotating
// clock emoji animation on stdout while it executes. It prints a success or
// error message when commandFunc returns.
//
//	in(1): context.Context ctx - context (reserved for future cancellation support)
//	in(2): func() error commandFunc - the operation to execute concurrently
//	in(3): string stepName - human-readable label shown in the loading animation and completion message
//	out: error - the error returned by commandFunc, or nil on success
func showLoadingIndicator(ctx context.Context, commandFunc func() error, stepName string) error {
	spinner := tui.NewSpinner(stepName)
	spinner.Start()

	err := commandFunc()

	if err != nil {
		spinner.StopWithMessage(fmt.Sprintf("Error during %s: %v", stepName, err))
		return err
	}
	spinner.StopWithMessage(fmt.Sprintf("%s completed", stepName))
	return nil
}
