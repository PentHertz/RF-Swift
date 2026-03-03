/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Internal utility functions
 *
 * loadJSON                 - in(1): string path, in(2): interface{} v, out: error
 * saveJSON                 - in(1): string path, in(2): interface{} v, out: error
 * ocontains                - in(1): []string slice, in(2): string item, out: bool
 * removeFromSlice          - in(1): []string slice, in(2): string item, out: []string
 * getContainerIDByName     - in(1): context.Context, in(2): string containerName, out: string
 * combineBindings          - in(1): string x11forward, in(2): string extrabinding, out: []string
 * splitAndCombine           - in(1): string commaSeparated, out: []string
 * combineEnv               - in(1): string xdisplay, in(2): string pulse_server, in(3): string extraenv, out: []string
 * normalizeImageName       - in(1): string imageName, out: string
 * parseImageName           - in(1): string imageName, out: string repo, string tag
 * bindExistsByPrefix       - in(1): []string binds, in(2): string mount, out: bool
 * removeBindByPrefix       - in(1): []string binds, in(2): string mount, out: []string
 * IsRootlessPodman         - out: bool
 * ParseExposedPorts        - in(1): string exposedPortsStr, out: nat.PortSet
 * ParseBindedPorts         - in(1): string bindedPortsStr, out: nat.PortMap
 * getDeviceMappingsFromString - in(1): string devicesStr, out: []container.DeviceMapping
 * convertPortBindingsToString - in(1): nat.PortMap, out: string
 * convertPortBindingsToRoundTrip - in(1): nat.PortMap, out: string
 * convertExposedPortsToString - in(1): nat.PortSet, out: string
 * convertDevicesToString      - in(1): []container.DeviceMapping, out: string
 * convertCapsToString         - in(1): []string caps, out: string
 * convertSecurityOptToString  - in(1): []string securityOpts, out: string
 * showLoadingIndicator     - in(1): context.Context, in(2): func() error, in(3): string stepName, out: error
 */
package dock

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	common "penthertz/rfswift/common"
)

func loadJSON(path string, v interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func saveJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

func ocontains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func removeFromSlice(slice []string, item string) []string {
	newSlice := []string{}
	for _, s := range slice {
		if s != item {
			newSlice = append(newSlice, s)
		}
	}
	return newSlice
}

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

func splitAndCombine(commaSeparated string) []string {
	if commaSeparated == "" {
		return []string{}
	}
	return strings.Split(commaSeparated, ",")
}

func combineEnv(xdisplay, pulse_server, extraenv string) []string {
	dockerenv := append(strings.Split(xdisplay, ","), "PULSE_SERVER="+pulse_server)
	if extraenv != "" {
		dockerenv = append(dockerenv, strings.Split(extraenv, ",")...)
	}
	return dockerenv
}

// normalizeImageName ensures an image has proper repo:tag format.
func normalizeImageName(imageName string) string {
	if imageName == "" {
		return imageName
	}
	if !strings.Contains(imageName, ":") {
		normalized := fmt.Sprintf("%s:%s", dockerObj.repotag, imageName)
		common.PrintInfoMessage(fmt.Sprintf("Using full image reference: %s", normalized))
		return normalized
	}
	return imageName
}

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

// bindExistsByPrefix checks if a bind mount already exists in the slice,
// ignoring trailing mount options (e.g., ":rw,rprivate,nosuid,rbind").
func bindExistsByPrefix(binds []string, mount string) bool {
	for _, b := range binds {
		if b == mount || strings.HasPrefix(b, mount+":") {
			return true
		}
	}
	return false
}

// removeBindByPrefix removes binds matching "src:dst" regardless of trailing mount options.
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

// IsRootlessPodman returns true when running under Podman without root privileges.
func IsRootlessPodman() bool {
	return GetEngine().Type() == EnginePodman && os.Getuid() != 0
}

// ParseExposedPorts parses a comma-separated port string into a nat.PortSet.
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

// ParseBindedPorts parses a port binding string into a nat.PortMap.
// Supports both ";;" (internal round-trip) and "," (CLI input) delimiters.
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
			fmt.Printf("Invalid binded port format: %s (expected containerPort/protocol:hostPort or containerPort/protocol:hostAddress:hostPort)\n", entry)
			continue
		}

		var containerPortProto, hostPort, hostAddress string

		containerPortProto = strings.TrimSpace(parts[0])

		if !strings.Contains(containerPortProto, "/") {
			fmt.Printf("Invalid container port format: %s (expected format: port/protocol, e.g., 80/tcp)\n", containerPortProto)
			continue
		}

		if len(parts) == 3 {
			hostAddress = strings.TrimSpace(parts[1])
			hostPort = strings.TrimSpace(parts[2])
		} else {
			hostAddress = ""
			hostPort = strings.TrimSpace(parts[1])
		}

		portKey := nat.Port(containerPortProto)

		portBindings[portKey] = append(portBindings[portKey], nat.PortBinding{
			HostIP:   hostAddress,
			HostPort: hostPort,
		})
	}

	return portBindings
}

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

func convertExposedPortsToString(exposedPorts nat.PortSet) string {
	var result []string
	for port := range exposedPorts {
		result = append(result, string(port))
	}
	return strings.Join(result, ", ")
}

func convertDevicesToString(devices []container.DeviceMapping) string {
	deviceStrings := make([]string, len(devices))
	for i, device := range devices {
		deviceStrings[i] = fmt.Sprintf("%s:%s", device.PathOnHost, device.PathInContainer)
	}
	return strings.Join(deviceStrings, ",")
}

func convertCapsToString(caps []string) string {
	if len(caps) == 0 {
		return ""
	}
	return strings.Join(caps, ",")
}

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

// showLoadingIndicator displays a rotating clock animation while a command runs.
func showLoadingIndicator(ctx context.Context, commandFunc func() error, stepName string) error {
	done := make(chan error)
	go func() {
		done <- commandFunc()
	}()

	clockEmojis := []string{"🕛", "🕐", "🕑", "🕒", "🕓", "🕔", "🕕", "🕖", "🕗", "🕘", "🕙", "🕚"}
	i := 0
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			if err != nil {
				common.PrintErrorMessage(fmt.Errorf("Error during %s: %v", stepName, err))
				return err
			}
			fmt.Printf("\n")
			common.PrintSuccessMessage(fmt.Sprintf("%s completed", stepName))
			return nil
		case <-ticker.C:
			fmt.Printf("\r%s %s", clockEmojis[i%len(clockEmojis)], stepName)
			i++
		}
	}
}
