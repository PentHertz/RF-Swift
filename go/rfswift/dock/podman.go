/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Podman-specific helpers for container recreation and compatibility
 */

package dock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	common "penthertz/rfswift/common"
)

// podmanCreateViaCLI creates a container via the podman CLI, which supports
// flags (like --device-cgroup-rule) that the Docker-compat API does not.
//
//	in(1): string name           the desired container name
//	in(2): string imageName      the image to create the container from
//	in(3): *container.Config cfg container configuration (env, cmd, labels, etc.)
//	in(4): *container.HostConfig hc host configuration (binds, devices, caps, etc.)
//	out: string the ID of the newly created container
//	out: error  non-nil if the podman CLI invocation fails
func podmanCreateViaCLI(name string, imageName string, cfg *container.Config, hc *container.HostConfig) (string, error) {
	args := []string{"create", "--name", name}

	// TTY and stdin — critical to keep the container alive
	if cfg.Tty {
		args = append(args, "-t")
	}
	if cfg.OpenStdin {
		args = append(args, "-i")
	}

	// Bind mounts
	for _, b := range hc.Binds {
		args = append(args, "-v", b)
	}

	// Devices
	for _, d := range hc.Devices {
		devStr := d.PathOnHost + ":" + d.PathInContainer
		if d.CgroupPermissions != "" {
			devStr += ":" + d.CgroupPermissions
		}
		args = append(args, "--device", devStr)
	}

	// Device cgroup rules — the whole reason we're using CLI
	for _, rule := range hc.DeviceCgroupRules {
		args = append(args, "--device-cgroup-rule", rule)
	}

	// Network mode
	if hc.NetworkMode != "" {
		args = append(args, "--network", string(hc.NetworkMode))
	}

	// Extra hosts
	for _, h := range hc.ExtraHosts {
		args = append(args, "--add-host", h)
	}

	// Environment variables
	for _, e := range cfg.Env {
		args = append(args, "-e", e)
	}

	// Labels
	for k, v := range cfg.Labels {
		args = append(args, "-l", k+"="+v)
	}

	// Capabilities
	for _, cap := range hc.CapAdd {
		args = append(args, "--cap-add", cap)
	}
	for _, cap := range hc.CapDrop {
		args = append(args, "--cap-drop", cap)
	}

	// Hostname
	if cfg.Hostname != "" {
		args = append(args, "--hostname", cfg.Hostname)
	}

	// Ulimits
	for _, u := range hc.Ulimits {
		args = append(args, "--ulimit", fmt.Sprintf("%s=%d:%d", u.Name, u.Soft, u.Hard))
	}

	// Security options
	for _, s := range hc.SecurityOpt {
		args = append(args, "--security-opt", s)
	}

	// IPC mode
	if hc.IpcMode != "" {
		args = append(args, "--ipc", string(hc.IpcMode))
	}

	// PID mode
	if hc.PidMode != "" {
		args = append(args, "--pid", string(hc.PidMode))
	}

	// Privileged
	if hc.Privileged {
		args = append(args, "--privileged")
	}

	// Tmpfs mounts
	for path, opts := range hc.Tmpfs {
		if opts != "" {
			args = append(args, "--tmpfs", path+":"+opts)
		} else {
			args = append(args, "--tmpfs", path)
		}
	}

	// Entrypoint — must be JSON array for multi-element entrypoints
	if len(cfg.Entrypoint) > 0 {
		epJSON, _ := json.Marshal(cfg.Entrypoint)
		args = append(args, "--entrypoint", string(epJSON))
	}

	// Working dir
	if cfg.WorkingDir != "" {
		args = append(args, "-w", cfg.WorkingDir)
	}

	// User
	if cfg.User != "" {
		args = append(args, "--user", cfg.User)
	}

	// Exposed ports
	for port := range cfg.ExposedPorts {
	    args = append(args, "--expose", string(port))
	}

	// Port bindings
	for port, bindings := range hc.PortBindings {
	    for _, binding := range bindings {
	        hostPart := binding.HostPort
	        if binding.HostIP != "" {
	            hostPart = binding.HostIP + ":" + hostPart
	        }
	        args = append(args, "-p", hostPart+":"+string(port))
	    }
	}

	// Image (positional, must come before cmd)
	args = append(args, imageName)

	// Command
	if len(cfg.Cmd) > 0 {
		args = append(args, cfg.Cmd...)
	}

	cmd := exec.Command("podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}

	containerID := strings.TrimSpace(string(output))
	return containerID, nil
}

// cleanupStaleTempImages removes old temporary images for a specific base image,
// skipping the one currently in use. Images are identified by the pattern
// "<repo>:<tag>_temp_<timestamp>" and any matching image that is not
// currentTempImage is force-removed.
//
//	in(1): context.Context ctx          context for Docker API calls
//	in(2): *client.Client cli           Docker/Podman API client
//	in(3): string currentTempImage      full image reference that must not be deleted
//	in(4): string repo                  repository portion of the base image name
//	in(5): string tag                   tag portion of the base image name
//	out: none
func cleanupStaleTempImages(ctx context.Context, cli *client.Client, currentTempImage string, repo string, tag string) {
	tempPattern := regexp.MustCompile(`_temp_\d{14}$`)
	basePrefix := fmt.Sprintf("%s:%s_temp_", repo, tag)
	localBasePrefix := fmt.Sprintf("localhost/%s:%s_temp_", repo, tag)

	images, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return
	}
	for _, img := range images {
		for _, imgTag := range img.RepoTags {
			if !tempPattern.MatchString(imgTag) {
				continue
			}
			// Only clean images matching our base repo:tag
			if !strings.Contains(imgTag, basePrefix) && !strings.Contains(imgTag, localBasePrefix) {
				continue
			}
			// Don't remove the one we're about to use
			if imgTag == currentTempImage {
				continue
			}
			_, err := cli.ImageRemove(ctx, img.ID, image.RemoveOptions{Force: false})
			if err == nil {
				common.PrintSuccessMessage(fmt.Sprintf("Cleaned up old temp image: %s", imgTag))
			}
		}
	}
}

// sanitizeHostConfigForPodman normalizes a Docker-inspected HostConfig so that
// it is compatible with Podman's stricter validation rules. It patches device
// permissions, memory settings, cgroup rules, bind deduplication, and device
// existence checks in-place.
//
//	in(1): *container.HostConfig hc the host configuration to sanitize (modified in place)
//	out: none
func sanitizeHostConfigForPodman(hc *container.HostConfig) {
	if hc == nil {
		return
	}

	// 1. Device permissions — Podman rejects empty CgroupPermissions
	for i := range hc.Devices {
		if hc.Devices[i].CgroupPermissions == "" {
			hc.Devices[i].CgroupPermissions = "rwm"
		}
	}

	// 2. MemorySwappiness — crun rejects on cgroup v2
	swappiness := int64(-1)
	hc.MemorySwappiness = &swappiness

	// 3. KernelMemory — deprecated, rejected by Podman
	hc.KernelMemory = 0
	hc.KernelMemoryTCP = 0

	// 4. PidsLimit — Podman rejects 0 (use -1 for unlimited)
	if hc.PidsLimit != nil && *hc.PidsLimit == 0 {
		unlimited := int64(-1)
		hc.PidsLimit = &unlimited
	}

	// 5. OomKillDisable — crun rejects on cgroup v2
	hc.OomKillDisable = nil

	// 6. DeviceCgroupRules — remove empty strings and fix permission order
	var cleanRules []string
	for _, rule := range hc.DeviceCgroupRules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		// Fix common permission order mistake: "rmw" → "rwm"
		rule = strings.Replace(rule, "rmw", "rwm", 1)
		cleanRules = append(cleanRules, rule)
	}
	hc.DeviceCgroupRules = cleanRules

	// 7. Deduplicate Binds
	hc.Binds = deduplicateBinds(hc.Binds)

	// 8. Build a set of bind-mounted destination paths
	bindDests := make(map[string]bool)
	for _, bind := range hc.Binds {
		dest := parseBindDestination(bind)
		bindDests[dest] = true
	}

	// 9. Remove Binds that conflict with Devices (same destination path)
	//    Podman rejects "duplicate mount destination" when a path appears
	//    in both Devices and Binds — but only for exact matches.
	if len(hc.Devices) > 0 && len(hc.Binds) > 0 {
		deviceDests := make(map[string]bool)
		for _, dev := range hc.Devices {
			deviceDests[dev.PathInContainer] = true
		}
		var deduplicatedBinds []string
		for _, bind := range hc.Binds {
			dest := parseBindDestination(bind)
			if deviceDests[dest] {
				continue // skip — already covered by Devices
			}
			deduplicatedBinds = append(deduplicatedBinds, bind)
		}
		hc.Binds = deduplicatedBinds
	}

	// 10. USB / device hotplug support
	//
	//     When a /dev subtree is bind-mounted (e.g. /dev/bus/usb), the bind
	//     gives filesystem visibility for new device nodes.  However, cgroup v2
	//     still blocks open() unless an explicit device-cgroup rule is present.
	//
	//     Additionally, individual device entries under a bind-mounted subtree
	//     are counter-productive: they are static snapshots captured at container
	//     creation time and break when devices are unplugged/replugged (the new
	//     device node number won't match the frozen mapping).
	//
	//     Strategy:
	//       • Inject the correct cgroup major-number rule
	//       • Remove individual Device entries that fall under the bind mount
	//
	devMajorRules := map[string]string{
		"/dev/bus/usb": "c 189:* rwm", // USB
		"/dev/snd":     "c 116:* rwm", // ALSA sound
		"/dev/dri":     "c 226:* rwm", // DRI / GPU
		"/dev/input":   "c 13:* rwm",  // Input devices (evdev, mice, js)
		"/dev/vhci":    "c 137:* rwm", // USB/IP VHCI
	}

	existingRules := make(map[string]bool)
	for _, rule := range hc.DeviceCgroupRules {
		existingRules[rule] = true
	}

	for prefix, rule := range devMajorRules {
		if !bindDests[prefix] {
			continue
		}

		// Inject cgroup rule if missing
		if !existingRules[rule] {
			hc.DeviceCgroupRules = append(hc.DeviceCgroupRules, rule)
			existingRules[rule] = true
		}

		// Remove individual Device entries under this prefix — the bind
		// mount + cgroup rule handles them, and keeping static entries
		// prevents hotplug from working.
		var cleanDevices []container.DeviceMapping
		for _, dev := range hc.Devices {
			if strings.HasPrefix(dev.PathOnHost, prefix) {
				continue // covered by bind + cgroup
			}
			cleanDevices = append(cleanDevices, dev)
		}
		hc.Devices = cleanDevices
	}

	// 11. Also inject cgroup rules for device entries that are NOT covered
	//     by a bind mount (standalone --device mappings).  This ensures
	//     that existing device access keeps working after recreation.
	for _, dev := range hc.Devices {
		for prefix, rule := range devMajorRules {
			if strings.HasPrefix(dev.PathOnHost, prefix) && !existingRules[rule] {
				hc.DeviceCgroupRules = append(hc.DeviceCgroupRules, rule)
				existingRules[rule] = true
			}
		}
	}

	// 12. Filter out devices whose host path no longer exists
	if len(hc.Devices) > 0 {
		var existingDevices []container.DeviceMapping
		for _, dev := range hc.Devices {
			if _, err := os.Stat(dev.PathOnHost); err == nil {
				existingDevices = append(existingDevices, dev)
			} else {
				common.PrintWarningMessage(fmt.Sprintf("Skipping non-existent device: %s", dev.PathOnHost))
			}
		}
		hc.Devices = existingDevices
	}

	// 13. Filter out binds whose host source no longer exists
	if len(hc.Binds) > 0 {
		var existingBinds []string
		for _, bind := range hc.Binds {
			parts := strings.SplitN(bind, ":", 3)
			if len(parts) >= 2 {
				if _, err := os.Stat(parts[0]); err != nil {
					common.PrintWarningMessage(fmt.Sprintf("Skipping non-existent bind source: %s", parts[0]))
					continue
				}
			}
			existingBinds = append(existingBinds, bind)
		}
		hc.Binds = existingBinds
	}
}

// deduplicateBinds removes bind entries with duplicate container destinations,
// keeping the last occurrence so that newly added binds take precedence over
// earlier ones.
//
//	in(1): []string binds the list of bind-mount specifications to deduplicate
//	out: []string deduplicated list of bind-mount specifications
func deduplicateBinds(binds []string) []string {
	seen := make(map[string]int) // destination → index in result
	var result []string

	for _, bind := range binds {
		dest := parseBindDestination(bind)
		if idx, exists := seen[dest]; exists {
			// Replace the earlier entry with this one
			result[idx] = bind
		} else {
			seen[dest] = len(result)
			result = append(result, bind)
		}
	}
	return result
}

// parseBindDestination extracts the destination (container-side) path from a
// bind-mount specification. Accepted formats are "source:dest" and
// "source:dest:opts". If the string contains no colon the raw value is
// returned unchanged.
//
//	in(1): string bind the bind-mount specification to parse
//	out: string the container destination path extracted from the specification
func parseBindDestination(bind string) string {
	parts := strings.SplitN(bind, ":", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return bind // bare path
}
