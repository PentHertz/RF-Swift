/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Structured container summary for API clients (Workbench GUI, remote agent).
 * It carries the same properties the CLI prints in its "Container Summary"
 * sheet after run/exec, without the terminal formatting.
 */

package dock

import (
	"context"
	"strings"

	"github.com/moby/moby/client"
)

// ContainerSummary mirrors the CLI container summary sheet.
type ContainerSummary struct {
	Name         string   `json:"name"`
	XDisplay     string   `json:"xDisplay"`
	Shell        string   `json:"shell"`
	Privileged   bool     `json:"privileged"`
	NetworkMode  string   `json:"networkMode"`
	NATSubnet    string   `json:"natSubnet,omitempty"`
	ExposedPorts []string `json:"exposedPorts"`
	PortBindings []string `json:"portBindings"`
	Image        string   `json:"image"`
	ImageVersion string   `json:"imageVersion,omitempty"`
	Size         string   `json:"size"`
	Bindings     []string `json:"bindings"`
	ExtraHosts   []string `json:"extraHosts"`
	Devices      []string `json:"devices"`
	Caps         []string `json:"caps"`
	Seccomp      string   `json:"seccomp"`
	CgroupRules  []string `json:"cgroupRules"`
	Ulimits      []string `json:"ulimits"`
	GPUs         string   `json:"gpus"`
	// SerialHotplug: the container opens serial ports through its device
	// cgroup and RF Swift attaches their nodes on demand (serialhotplug.go).
	// SerialPorts are the ports the container was configured with (label),
	// SerialPresent the ports present on the host right now.
	SerialHotplug bool     `json:"serialHotplug"`
	SerialPorts   []string `json:"serialPorts"`
	SerialPresent []string `json:"serialPresent"`
}

// serialSummary reports whether rules grant the serial majors and which of
// the host's serial ports are present, for the summary sheet.
func serialSummary(rules []string, present []string) (bool, []string) {
	hotplug := false
	for _, rule := range rules {
		for _, serial := range SerialCgroupRules {
			if strings.TrimSpace(rule) == serial {
				hotplug = true
			}
		}
	}
	ports := []string{}
	if hotplug {
		ports = append(ports, present...)
	}
	return hotplug, ports
}

// ContainerSummaryFor inspects a container and returns its summary. Only local
// engine calls are made; the image freshness check that the CLI adds to the
// sheet is left to the caller (see CheckImage) because it needs the registry.
//
//	in(1): context.Context ctx  request context
//	in(2): *client.Client cli   engine client
//	in(3): string containerID   container ID or name
//	out: ContainerSummary structured properties
//	out: error non-nil when the container or its image cannot be inspected
func ContainerSummaryFor(ctx context.Context, cli *client.Client, containerID string) (ContainerSummary, error) {
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		return ContainerSummary{}, err
	}
	containerJSON, err := inspectContainer(ctx, cli, containerID)
	if err != nil {
		return ContainerSummary{}, err
	}
	networkMode := props["NetworkMode"]
	if display := props["NetworkModeDisplay"]; display != "" {
		networkMode = display
	}
	_, tag := parseImageName(props["ImageName"])
	_, version := parseTagVersion(tag)
	var present []string
	for _, n := range listHostSerialNodes() {
		present = append(present, n.Path)
	}
	serialHotplug, serialPresent := serialSummary(splitSummaryList(props["Cgroups"], ","), present)
	if strings.EqualFold(props["SerialHotplug"], "off") {
		serialHotplug, serialPresent = false, []string{}
	}
	serialPorts := SerialPortsFromLabel(props["SerialPorts"])
	if serialPorts == nil {
		serialPorts = []string{}
	}
	return ContainerSummary{
		Name:          strings.TrimPrefix(containerJSON.Name, "/"),
		XDisplay:      props["XDisplay"],
		Shell:         props["Shell"],
		Privileged:    props["Privileged"] == "true",
		NetworkMode:   networkMode,
		NATSubnet:     props["NATSubnet"],
		ExposedPorts:  splitSummaryList(props["ExposedPorts"], ","),
		PortBindings:  splitSummaryList(props["PortBindings"], ";;"),
		Image:         props["ImageName"],
		ImageVersion:  version,
		Size:          props["Size"],
		Bindings:      splitSummaryList(props["Bindings"], ";;"),
		ExtraHosts:    splitSummaryList(props["ExtraHosts"], ","),
		Devices:       splitSummaryList(props["Devices"], ","),
		Caps:          splitSummaryList(props["Caps"], ","),
		Seccomp:       props["Seccomp"],
		CgroupRules:   splitSummaryList(props["Cgroups"], ","),
		Ulimits:       splitSummaryList(props["Ulimits"], ","),
		GPUs:          props["GPUs"],
		SerialHotplug: serialHotplug,
		SerialPorts:   serialPorts,
		SerialPresent: serialPresent,
	}, nil
}

func splitSummaryList(value, separator string) []string {
	out := []string{}
	for _, item := range strings.Split(value, separator) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
