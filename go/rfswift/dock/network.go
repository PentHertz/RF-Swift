/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Per-container NAT network management
 */

package dock

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"strings"

	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	common "penthertz/rfswift/common"
	"penthertz/rfswift/tui"
)

const (
	// NATNetworkPrefix is the prefix for all RF Swift managed NAT networks.
	NATNetworkPrefix = "rfswift_nat_"

	// NATLabel marks a Docker network as managed by RF Swift NAT mode.
	NATLabel = "org.rfswift.nat"

	// DefaultNATRange is the default CIDR range from which per-container subnets
	// are carved. Each container gets a /28 (16 addresses) by default.
	DefaultNATRange = "172.30.0.0/16"

	// DefaultNATNetmask is the prefix length for each per-container subnet.
	DefaultNATNetmask = 28
)

// networkName returns the Docker network name for a given container name.
func networkName(containerName string) string {
	return NATNetworkPrefix + containerName
}

// createNATNetwork creates a dedicated Docker bridge network for a container.
// If userSubnet is non-empty, it is used directly; otherwise a subnet is
// auto-allocated from the RF Swift NAT range.
// Returns the network name and the allocated subnet CIDR.
func createNATNetwork(ctx context.Context, cli *client.Client, containerName string, userSubnet string) (string, string, error) {
	name := networkName(containerName)

	// Check if network already exists
	existing, err := findNATNetwork(ctx, cli, name)
	if err == nil && existing != "" {
		// Return existing network info
		netInspect, inspErr := cli.NetworkInspect(ctx, existing, dockernetwork.InspectOptions{})
		if inspErr == nil && len(netInspect.IPAM.Config) > 0 {
			return name, netInspect.IPAM.Config[0].Subnet, nil
		}
		return name, "", nil
	}

	// Resolve subnet: user-specified or auto-allocated
	var subnet, gateway string
	if userSubnet != "" {
		subnet = userSubnet
		gw, gwErr := gatewayFromSubnet(subnet)
		if gwErr != nil {
			return "", "", fmt.Errorf("invalid subnet '%s': %v", userSubnet, gwErr)
		}
		gateway = gw
	} else {
		var allocErr error
		subnet, gateway, allocErr = allocateSubnet(ctx, cli)
		if allocErr != nil {
			return "", "", fmt.Errorf("failed to allocate NAT subnet: %v", allocErr)
		}
	}

	// Create the network
	labels := map[string]string{
		NATLabel:                "true",
		"org.container.project": "rfswift",
		"org.rfswift.container": containerName,
	}

	resp, err := cli.NetworkCreate(ctx, name, dockernetwork.CreateOptions{
		Driver:     "bridge",
		EnableIPv6: boolPtr(false),
		Labels:     labels,
		IPAM: &dockernetwork.IPAM{
			Driver: "default",
			Config: []dockernetwork.IPAMConfig{
				{
					Subnet:  subnet,
					Gateway: gateway,
				},
			},
		},
	})
	if err != nil {
		// Fallback to Podman CLI if the compat API fails
		if GetEngine().Type() == EnginePodman {
			common.PrintInfoMessage("Falling back to Podman CLI for network creation...")
			if podErr := createNATNetworkPodman(name, subnet, gateway, labels); podErr != nil {
				return "", "", fmt.Errorf("failed to create NAT network '%s': %v (API: %v)", name, podErr, err)
			}
			common.PrintSuccessMessage(fmt.Sprintf("Created NAT network '%s' (subnet: %s) via Podman CLI", name, subnet))
			return name, subnet, nil
		}
		return "", "", fmt.Errorf("failed to create NAT network '%s': %v", name, err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("Created NAT network '%s' (subnet: %s, id: %s)", name, subnet, resp.ID[:12]))
	return name, subnet, nil
}

// removeNATNetwork removes the RF Swift NAT network associated with a container.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string containerName
func removeNATNetwork(ctx context.Context, cli *client.Client, containerName string) {
	name := networkName(containerName)
	netID, err := findNATNetwork(ctx, cli, name)
	if err != nil || netID == "" {
		return // No NAT network for this container
	}

	if err := cli.NetworkRemove(ctx, netID); err != nil {
		// Fallback to Podman CLI
		if GetEngine().Type() == EnginePodman {
			if podErr := removeNATNetworkPodman(name); podErr != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to remove NAT network '%s': %v", name, podErr))
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("Removed NAT network '%s'", name))
			}
			return
		}
		common.PrintWarningMessage(fmt.Sprintf("Failed to remove NAT network '%s': %v", name, err))
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Removed NAT network '%s'", name))
	}
}

// removeNATNetworkByFullName removes a NAT network by its full Docker network name.
func removeNATNetworkByFullName(ctx context.Context, cli *client.Client, fullName string) {
	netID, err := findNATNetwork(ctx, cli, fullName)
	if err != nil || netID == "" {
		return
	}

	if err := cli.NetworkRemove(ctx, netID); err != nil {
		if GetEngine().Type() == EnginePodman {
			if podErr := removeNATNetworkPodman(fullName); podErr != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to remove NAT network '%s': %v", fullName, podErr))
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("Removed NAT network '%s'", fullName))
			}
			return
		}
		common.PrintWarningMessage(fmt.Sprintf("Failed to remove NAT network '%s': %v", fullName, err))
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Removed NAT network '%s'", fullName))
	}
}

// removeNATNetworkByLabel removes the NAT network associated with a container,
// looking up by the org.rfswift.container label instead of name convention.
func removeNATNetworkByLabel(ctx context.Context, cli *client.Client, containerName string) {
	networks, err := cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return
	}

	for _, n := range networks {
		if n.Labels[NATLabel] == "true" && n.Labels["org.rfswift.container"] == containerName {
			if err := cli.NetworkRemove(ctx, n.ID); err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to remove NAT network '%s': %v", n.Name, err))
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("Removed NAT network '%s'", n.Name))
			}
		}
	}
}

// findNATNetwork looks up a network by name and returns its ID if found.
func findNATNetwork(ctx context.Context, cli *client.Client, name string) (string, error) {
	netInspect, err := cli.NetworkInspect(ctx, name, dockernetwork.InspectOptions{})
	if err != nil {
		return "", err
	}
	return netInspect.ID, nil
}

// allocateSubnet finds the next free /28 subnet within the RF Swift NAT range
// that doesn't conflict with any existing Docker network.
func allocateSubnet(ctx context.Context, cli *client.Client) (subnet string, gateway string, err error) {
	_, baseNet, err := net.ParseCIDR(DefaultNATRange)
	if err != nil {
		return "", "", fmt.Errorf("invalid NAT range: %v", err)
	}

	// Collect all subnets currently in use by Docker networks
	usedSubnets, err := collectUsedSubnets(ctx, cli)
	if err != nil {
		return "", "", err
	}

	// Iterate through possible /28 subnets within the base range
	baseIP := ipToUint32(baseNet.IP)
	baseMask := ipToUint32(net.IP(baseNet.Mask))
	subnetSize := uint32(1) << (32 - DefaultNATNetmask) // 16 for /28

	for candidate := baseIP; (candidate & baseMask) == (baseIP & baseMask); candidate += subnetSize {
		candidateNet := fmt.Sprintf("%s/%d", uint32ToIP(candidate).String(), DefaultNATNetmask)

		if !subnetConflicts(candidateNet, usedSubnets) {
			// Gateway is first usable IP (candidate + 1)
			gw := uint32ToIP(candidate + 1).String()
			return candidateNet, gw, nil
		}
	}

	return "", "", fmt.Errorf("no free /%d subnet available in %s", DefaultNATNetmask, DefaultNATRange)
}

// collectUsedSubnets returns all CIDR strings from existing Docker networks.
func collectUsedSubnets(ctx context.Context, cli *client.Client) ([]string, error) {
	networks, err := cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %v", err)
	}

	var subnets []string
	for _, n := range networks {
		for _, config := range n.IPAM.Config {
			if config.Subnet != "" {
				subnets = append(subnets, config.Subnet)
			}
		}
	}
	return subnets, nil
}

// subnetConflicts checks if a candidate CIDR overlaps with any existing subnet.
func subnetConflicts(candidate string, existing []string) bool {
	_, candidateNet, err := net.ParseCIDR(candidate)
	if err != nil {
		return true // Treat parse errors as conflicts
	}

	for _, s := range existing {
		_, existingNet, err := net.ParseCIDR(s)
		if err != nil {
			continue
		}
		if candidateNet.Contains(existingNet.IP) || existingNet.Contains(candidateNet.IP) {
			return true
		}
	}
	return false
}

// ipToUint32 converts a net.IP to a uint32 (IPv4 only).
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip)
}

// uint32ToIP converts a uint32 back to net.IP.
func uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}

func boolPtr(b bool) *bool {
	return &b
}

// ---------------------------------------------------------------------------
// Public API for CLI commands
// ---------------------------------------------------------------------------

// NetworkInfo holds display information for a NAT network.
type NetworkInfo struct {
	Name       string
	ID         string
	Subnet     string
	Gateway    string
	Container  string
	Driver     string
	Shared     bool
	Containers int
}

// ListNATNetworks returns all RF Swift NAT networks.
func ListNATNetworks() ([]NetworkInfo, error) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	networks, err := cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %v", err)
	}

	var result []NetworkInfo
	for _, n := range networks {
		if n.Labels[NATLabel] != "true" {
			continue
		}

		// NetworkList doesn't populate Containers; inspect to get the count
		connected := countContainersOnNetwork(ctx, cli, n.ID)

		info := NetworkInfo{
			Name:       n.Name,
			ID:         n.ID[:12],
			Container:  n.Labels["org.rfswift.container"],
			Driver:     n.Driver,
			Shared:     n.Labels["org.rfswift.shared"] == "true",
			Containers: connected,
		}
		if len(n.IPAM.Config) > 0 {
			info.Subnet = n.IPAM.Config[0].Subnet
			info.Gateway = n.IPAM.Config[0].Gateway
		}
		result = append(result, info)
	}
	return result, nil
}

// DisplayNATNetworks prints a table of RF Swift NAT networks.
func DisplayNATNetworks() {
	networks, err := ListNATNetworks()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}

	if len(networks) == 0 {
		common.PrintInfoMessage("No RF Swift NAT networks found")
		return
	}

	tableData := [][]string{}
	for _, n := range networks {
		netType := "auto"
		if n.Shared {
			netType = "shared"
		}
		owner := n.Container
		if owner == "" {
			owner = "-"
		}
		tableData = append(tableData, []string{
			n.Name,
			n.Subnet,
			netType,
			fmt.Sprintf("%d", n.Containers),
			owner,
			n.ID,
		})
	}
	tui.RenderTable(tui.TableConfig{
		Title:   "RF Swift NAT Networks",
		Headers: []string{"Name", "Subnet", "Type", "Connected", "Owner", "ID"},
		Rows:    tableData,
	})
}

// RemoveNATNetworkByName removes a NAT network by its name or the container name.
func RemoveNATNetworkByName(name string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	// Try direct name first
	netID, err := findNATNetwork(ctx, cli, name)
	if err != nil {
		// Try with prefix
		netID, err = findNATNetwork(ctx, cli, NATNetworkPrefix+name)
	}
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("NAT network '%s' not found", name))
		return
	}

	if err := cli.NetworkRemove(ctx, netID); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to remove network: %v", err))
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Removed NAT network '%s'", name))
	}
}

// CleanupOrphanedNATNetworks removes NAT networks whose associated container no longer exists.
func CleanupOrphanedNATNetworks() {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	networks, err := cli.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to list networks: %v", err))
		return
	}

	removed := 0
	for _, n := range networks {
		if n.Labels[NATLabel] != "true" {
			continue
		}

		containerName := n.Labels["org.rfswift.container"]
		if containerName == "" {
			continue
		}

		// Check if container still exists
		_, err := findContainerByName(ctx, cli, containerName)
		if err != nil {
			// Container gone — network is orphaned
			if err := cli.NetworkRemove(ctx, n.ID); err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to remove orphaned network '%s': %v", n.Name, err))
			} else {
				common.PrintSuccessMessage(fmt.Sprintf("Removed orphaned NAT network '%s' (container '%s' no longer exists)", n.Name, containerName))
				removed++
			}
		}
	}

	if removed == 0 {
		common.PrintInfoMessage("No orphaned NAT networks found")
	} else {
		common.PrintSuccessMessage(fmt.Sprintf("Cleaned up %d orphaned NAT network(s)", removed))
	}
}

// findContainerByName checks if a container with the given name exists.
func findContainerByName(ctx context.Context, cli *client.Client, name string) (string, error) {
	containerJSON, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		// Also try with / prefix
		if !strings.HasPrefix(name, "/") {
			containerJSON, err = cli.ContainerInspect(ctx, "/"+name)
		}
		if err != nil {
			return "", err
		}
	}
	return containerJSON.ID, nil
}

// parseNATMode checks if the network mode is a NAT request.
// Returns (isNAT, networkName, subnet) where networkName is empty for auto-create
// or a specific name when the user wants to join an existing network.
// Format: "nat" | "nat:name" | "nat::subnet" | "nat:name:subnet"
func parseNATMode() (bool, string, string) {
	mode := strings.ToLower(containerCfg.networkMode)
	if mode == "nat" {
		return true, "", ""
	}
	if strings.HasPrefix(mode, "nat:") {
		rest := containerCfg.networkMode[4:] // preserve original case
		parts := strings.SplitN(rest, ":", 2)
		name := parts[0]
		subnet := ""
		if len(parts) == 2 {
			subnet = parts[1]
		}
		return true, name, subnet
	}
	return false, "", ""
}

// isNATMode returns true when the current container config requests NAT networking.
func isNATMode() bool {
	isNAT, _, _ := parseNATMode()
	return isNAT
}

// ---------------------------------------------------------------------------
// Podman compatibility
// ---------------------------------------------------------------------------

// createNATNetworkPodman creates a NAT network via the Podman CLI when the
// Docker compat API fails (common in rootless mode or older Podman versions).
func createNATNetworkPodman(name string, subnet string, gateway string, labels map[string]string) error {
	args := []string{"network", "create", "--driver", "bridge"}

	if subnet != "" {
		args = append(args, "--subnet", subnet)
		if gateway != "" {
			args = append(args, "--gateway", gateway)
		}
	}

	for k, v := range labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, name)

	cmd := exec.Command("podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("podman network create failed: %v — %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// removeNATNetworkPodman removes a network via the Podman CLI.
func removeNATNetworkPodman(name string) error {
	cmd := exec.Command("podman", "network", "rm", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("podman network rm failed: %v — %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// createOrJoinNATNetwork creates a new NAT network or joins an existing one.
// When targetName is empty, a per-container network is auto-created.
// When targetName is set, the named network is reused if it exists or created if not.
// userSubnet, if non-empty, overrides auto-allocation with the given CIDR.
//
//	in(1): context.Context ctx
//	in(2): *client.Client cli
//	in(3): string containerName  the container being created
//	in(4): string targetName     the named NAT network to join (empty = auto-create)
//	in(5): string userSubnet     user-specified subnet CIDR (empty = auto-allocate)
//	out: (networkName string, subnet string, err error)
func createOrJoinNATNetwork(ctx context.Context, cli *client.Client, containerName string, targetName string, userSubnet string) (string, string, error) {
	// Warn rootless Podman users about potential limitations
	if IsRootlessPodman() {
		common.PrintWarningMessage("Rootless Podman: NAT bridge networks may have limited functionality.")
		common.PrintInfoMessage("For full NAT support with Podman, run with sudo or use rootful mode.")
	}

	if targetName == "" {
		// Auto mode: create a per-container network
		return createNATNetwork(ctx, cli, containerName, userSubnet)
	}

	// Named mode: prefix with rfswift_nat_ if not already
	fullName := targetName
	if !strings.HasPrefix(fullName, NATNetworkPrefix) {
		fullName = NATNetworkPrefix + fullName
	}

	// Check if the named network already exists
	existing, err := findNATNetwork(ctx, cli, fullName)
	if err == nil && existing != "" {
		// Join existing network
		netInspect, inspErr := cli.NetworkInspect(ctx, existing, dockernetwork.InspectOptions{})
		subnet := ""
		if inspErr == nil && len(netInspect.IPAM.Config) > 0 {
			subnet = netInspect.IPAM.Config[0].Subnet
		}
		common.PrintInfoMessage(fmt.Sprintf("Joining existing NAT network '%s' (subnet: %s)", fullName, subnet))
		return fullName, subnet, nil
	}

	// Create the named network
	return createNamedNATNetwork(ctx, cli, fullName, targetName, userSubnet)
}

// createNamedNATNetwork creates a named NAT network (shared, not tied to a single container).
// If userSubnet is non-empty, it is used directly; otherwise a subnet is auto-allocated.
func createNamedNATNetwork(ctx context.Context, cli *client.Client, fullName string, displayName string, userSubnet string) (string, string, error) {
	var subnet, gateway string
	if userSubnet != "" {
		subnet = userSubnet
		gw, gwErr := gatewayFromSubnet(subnet)
		if gwErr != nil {
			return "", "", fmt.Errorf("invalid subnet '%s': %v", userSubnet, gwErr)
		}
		gateway = gw
	} else {
		var allocErr error
		subnet, gateway, allocErr = allocateSubnet(ctx, cli)
		if allocErr != nil {
			return "", "", fmt.Errorf("failed to allocate NAT subnet: %v", allocErr)
		}
	}

	labels := map[string]string{
		NATLabel:                "true",
		"org.container.project": "rfswift",
		"org.rfswift.shared":    "true",
	}

	resp, err := cli.NetworkCreate(ctx, fullName, dockernetwork.CreateOptions{
		Driver:     "bridge",
		EnableIPv6: boolPtr(false),
		Labels:     labels,
		IPAM: &dockernetwork.IPAM{
			Driver: "default",
			Config: []dockernetwork.IPAMConfig{
				{
					Subnet:  subnet,
					Gateway: gateway,
				},
			},
		},
	})
	if err != nil {
		// Fallback to Podman CLI
		if GetEngine().Type() == EnginePodman {
			common.PrintInfoMessage("Falling back to Podman CLI for network creation...")
			if podErr := createNATNetworkPodman(fullName, subnet, gateway, labels); podErr != nil {
				return "", "", fmt.Errorf("failed to create NAT network '%s': %v (API: %v)", fullName, podErr, err)
			}
			common.PrintSuccessMessage(fmt.Sprintf("Created NAT network '%s' (subnet: %s) via Podman CLI", displayName, subnet))
			return fullName, subnet, nil
		}
		return "", "", fmt.Errorf("failed to create NAT network '%s': %v", fullName, err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("Created NAT network '%s' (subnet: %s, id: %s)", displayName, subnet, resp.ID[:12]))
	return fullName, subnet, nil
}

// CreateNATNetworkCLI is the public API for "rfswift network create".
func CreateNATNetworkCLI(name string, subnet string) {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		common.PrintErrorMessage(err)
		return
	}
	defer cli.Close()

	fullName := name
	if !strings.HasPrefix(fullName, NATNetworkPrefix) {
		fullName = NATNetworkPrefix + fullName
	}

	// Check if already exists
	existing, findErr := findNATNetwork(ctx, cli, fullName)
	if findErr == nil && existing != "" {
		common.PrintErrorMessage(fmt.Errorf("NAT network '%s' already exists", name))
		return
	}

	if subnet != "" {
		// User-specified subnet
		gateway, gwErr := gatewayFromSubnet(subnet)
		if gwErr != nil {
			common.PrintErrorMessage(gwErr)
			return
		}

		resp, createErr := cli.NetworkCreate(ctx, fullName, dockernetwork.CreateOptions{
			Driver:     "bridge",
			EnableIPv6: boolPtr(false),
			Labels: map[string]string{
				NATLabel:                "true",
				"org.container.project": "rfswift",
				"org.rfswift.shared":    "true",
			},
			IPAM: &dockernetwork.IPAM{
				Driver: "default",
				Config: []dockernetwork.IPAMConfig{
					{
						Subnet:  subnet,
						Gateway: gateway,
					},
				},
			},
		})
		if createErr != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to create network: %v", createErr))
			return
		}
		common.PrintSuccessMessage(fmt.Sprintf("Created NAT network '%s' (subnet: %s, id: %s)", name, subnet, resp.ID[:12]))
	} else {
		// Auto-allocate subnet
		_, _, createErr := createNamedNATNetwork(ctx, cli, fullName, name, "")
		if createErr != nil {
			common.PrintErrorMessage(createErr)
		}
	}
}

// gatewayFromSubnet derives the gateway IP (first usable address) from a CIDR.
func gatewayFromSubnet(cidr string) (string, error) {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid subnet '%s': %v", cidr, err)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "", fmt.Errorf("only IPv4 subnets are supported")
	}
	n := ipToUint32(ip4)
	return uint32ToIP(n + 1).String(), nil
}

// isSharedNATNetwork checks if a NAT network is shared (not auto-created per container).
func isSharedNATNetwork(ctx context.Context, cli *client.Client, networkName string) bool {
	netInspect, err := cli.NetworkInspect(ctx, networkName, dockernetwork.InspectOptions{})
	if err != nil {
		return false
	}
	return netInspect.Labels["org.rfswift.shared"] == "true"
}

// countContainersOnNetwork returns how many containers are connected to a network.
func countContainersOnNetwork(ctx context.Context, cli *client.Client, networkName string) int {
	netInspect, err := cli.NetworkInspect(ctx, networkName, dockernetwork.InspectOptions{})
	if err != nil {
		return 0
	}
	return len(netInspect.Containers)
}

// ListNATNetworkNames returns the names of all NAT networks (for wizard picker).
func ListNATNetworkNames() []string {
	networks, err := ListNATNetworks()
	if err != nil {
		return nil
	}
	names := make([]string, len(networks))
	for i, n := range networks {
		names[i] = n.Name
	}
	return names
}
