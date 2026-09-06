/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Serial ports that come and go: hot-plug for containers.
*
*  A device mapping (--device /dev/ttyACM0) is fixed when the container is
*  created: the node must exist then, the container cannot start without it,
*  and a port plugged in later never appears. Serial ports are how a
*  Proxmark3, a flasher or a modem is reached, and they are plugged and
*  unplugged all the time. So RF Swift does not map them. Instead:
*
*    - the container's device cgroup allows the serial majors (166 CDC-ACM,
*      188 USB serial, 204 platform UARTs) at creation (SerialCgroupRules), so
*      the kernel lets it open such a node whenever one exists in its /dev;
*    - SyncSerialDevices creates the nodes inside the container's /dev (a
*      private tmpfs, CAP_MKNOD is a default capability) for every serial port
*      present on the host, and removes the ones that are gone. It runs when
*      the container starts and whenever a shell or a command is opened in it,
*      so "plug it in, open a terminal" is the whole procedure.
*
*  Rootless Podman cannot do either (no cgroup rules, no mknod in a user
*  namespace): there a serial port is mapped at creation and must be present.
 */

package dock

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	common "penthertz/rfswift/common"
)

// SerialCgroupRules let a container open serial ports that appear after it
// started, once their nodes exist inside its /dev.
var SerialCgroupRules = []string{"c 166:* rwm", "c 188:* rwm", "c 204:* rwm"}

// SerialHotplugLabel switches the hot-plug off for one container ("off"):
// the sync then leaves its /dev alone and the serial cgroup rules are taken
// out. Absent or any other value means on whenever the rules are present.
const SerialHotplugLabel = "org.rfswift.serial_hotplug"

// SerialHotplugEnabled tells the state from a container's rules and labels.
func SerialHotplugEnabled(rules []string, labels map[string]string) bool {
	if labels != nil && strings.EqualFold(strings.TrimSpace(labels[SerialHotplugLabel]), "off") {
		return false
	}
	for _, rule := range rules {
		for _, serial := range SerialCgroupRules {
			if strings.TrimSpace(rule) == serial {
				return true
			}
		}
	}
	return false
}

// SerialPortsLabel records the serial ports a container was configured with
// (comma-separated host paths). They are not device mappings, so without the
// label nothing would show that the port belongs to the container.
const SerialPortsLabel = "org.rfswift.serial_ports"

// listHostSerialNodes is hostSerialNodes, replaceable by tests that have no
// serial device.
var listHostSerialNodes = hostSerialNodes

// SerialPortsFromLabel splits the label value.
func SerialPortsFromLabel(value string) []string {
	var out []string
	for _, p := range strings.Split(value, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// MergeSerialPorts adds and removes ports in a label value, keeping order.
func MergeSerialPorts(value string, add, remove []string) string {
	ports := SerialPortsFromLabel(value)
	for _, p := range add {
		ports = appendMissing(ports, p)
	}
	if len(remove) > 0 {
		var kept []string
		for _, p := range ports {
			drop := false
			for _, r := range remove {
				if r == p {
					drop = true
				}
			}
			if !drop {
				kept = append(kept, p)
			}
		}
		ports = kept
	}
	return strings.Join(ports, ",")
}

// serialNode is a serial port on the host with its device numbers.
type serialNode struct {
	Path         string
	Major, Minor uint32
}

// hostSerialNodes lists the serial ports present on the host.
func hostSerialNodes() []serialNode {
	var out []serialNode
	for _, pattern := range []string{"/dev/ttyACM*", "/dev/ttyUSB*", "/dev/ttyAMA*"} {
		matches, _ := filepath.Glob(pattern)
		for _, p := range matches {
			major, minor, ok := deviceNumbers(p)
			if ok {
				out = append(out, serialNode{Path: p, Major: major, Minor: minor})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// SerialHotplugSupported reports whether this engine can attach serial ports
// after creation: a rootful daemon on Linux (device cgroup rules and mknod
// inside the container both work). Rootless Podman and the Lima VM cannot.
func SerialHotplugSupported(eng ContainerEngine) bool {
	if eng == nil || eng.Type() == EngineLima || IsRootlessPodman() {
		return false
	}
	return deviceNumbersSupported()
}

// buildSerialSyncScript writes the shell run inside the container: create
// the node of every present port (mode 660, root:dialout as udev would),
// delete serial nodes that no longer exist on the host, print what changed.
// devRoot is /dev in the container; tests point it at a directory. Port
// paths never contain spaces or quotes (the kernel names them), so the keep
// list is a plain space-separated string.
func buildSerialSyncScript(devRoot string, nodes []serialNode) string {
	var b strings.Builder
	b.WriteString("keep=\"\"\n")
	for _, n := range nodes {
		p := filepath.Join(devRoot, filepath.Base(n.Path))
		fmt.Fprintf(&b, "keep=\"$keep %s\"\n", p)
		fmt.Fprintf(&b, "if [ ! -c %s ]; then rm -rf %s; if mknod -m 660 %s c %d %d; then chgrp dialout %s 2>/dev/null || true; echo \"created %s\"; else echo \"failed %s\" >&2; fi; fi\n", p, p, p, n.Major, n.Minor, p, p, p)
	}
	root := strings.TrimRight(devRoot, "/")
	// A bind-mounted node cannot be unlinked (EBUSY): that is fine, it is the
	// host's node and stays in step with the host by itself.
	fmt.Fprintf(&b, "for p in %s/ttyACM* %s/ttyUSB* %s/ttyAMA*; do [ -e \"$p\" ] || continue; case \" $keep \" in *\" $p \"*) ;; *) rm -f \"$p\" 2>/dev/null && echo \"removed $p\" || true;; esac; done\n", root, root, root)
	return b.String()
}

// SyncSerialDevices makes the container's /dev reflect the host's serial
// ports: nodes are created for the ports present and stale ones removed. It
// returns the changes ("created /dev/ttyACM0") and warns, through the error,
// when the container lacks the cgroup rules that make the nodes usable.
func SyncSerialDevices(ctx context.Context, cli *client.Client, id string) ([]string, error) {
	if !deviceNumbersSupported() {
		return nil, nil
	}
	res, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil || res.Container.State == nil || !res.Container.State.Running {
		return nil, nil
	}
	hc := res.Container.HostConfig
	if hc == nil || hc.Privileged {
		return nil, nil // a privileged container sees the host's /dev already
	}
	if res.Container.Config != nil && strings.EqualFold(res.Container.Config.Labels[SerialHotplugLabel], "off") {
		return nil, nil // switched off for this container: its /dev is left alone
	}
	nodes := listHostSerialNodes()
	allowed := map[string]bool{}
	for _, rule := range hc.DeviceCgroupRules {
		allowed[strings.TrimSpace(rule)] = true
	}
	script := buildSerialSyncScript("/dev", nodes)
	created, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{User: "root", Cmd: []string{"/bin/sh", "-c", script}, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return nil, fmt.Errorf("serial hot-plug: %w", err)
	}
	attached, err := cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("serial hot-plug: %w", err)
	}
	defer attached.Close()
	var changes []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(demuxReader(attached.Reader))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "created ") || strings.HasPrefix(line, "removed ") {
				changes = append(changes, line)
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
	}
	var missingRules []string
	for _, n := range nodes {
		rule := fmt.Sprintf("c %d:* rwm", n.Major)
		if !allowed[rule] {
			missingRules = append(missingRules, rule)
		}
	}
	if len(missingRules) > 0 {
		return changes, fmt.Errorf("serial hot-plug: the container was created without the device cgroup rule(s) %s, so it cannot open the port; re-create it (Configure > apply) to add them", strings.Join(uniqueStrings(missingRules), ", "))
	}
	return changes, nil
}

// UpdateSerialHotplug switches the serial hot-plug of a container on or off:
// on adds the serial cgroup rules and clears the switch label, off removes
// the rules and sets the label so the sync leaves the container's /dev
// alone. Applied by the file edit or the re-creation the engine calls for.
func UpdateSerialHotplug(containerID string, on bool) error {
	ctx := context.Background()
	cli, err := NewEngineClient()
	if err != nil {
		return err
	}
	defer cli.Close()
	props, err := getContainerProperties(ctx, cli, containerID)
	if err != nil {
		return err
	}
	rules := splitSummaryList(props["Cgroups"], ",")
	var kept []string
	for _, rule := range rules {
		serial := false
		for _, s := range SerialCgroupRules {
			if strings.TrimSpace(rule) == s {
				serial = true
			}
		}
		if !serial {
			kept = append(kept, rule)
		}
	}
	if on {
		kept = appendMissing(kept, SerialCgroupRules...)
		props["SerialHotplug"] = ""
	} else {
		props["SerialHotplug"] = "off"
	}
	props["Cgroups"] = strings.Join(kept, ",")
	if useDirectConfigEdit() {
		inspected, err := inspectContainer(ctx, cli, containerID)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(inspected.Name, "/")
		return directEditContainer(ctx, cli, containerID, name, func(host *HostConfigFull, configV2 map[string]interface{}) (bool, error) {
			host.DeviceCgroupRules = kept
			if config, ok := configV2["Config"].(map[string]interface{}); ok {
				labels, _ := config["Labels"].(map[string]interface{})
				if labels == nil {
					labels = map[string]interface{}{}
					config["Labels"] = labels
				}
				if on {
					delete(labels, SerialHotplugLabel)
				} else {
					labels[SerialHotplugLabel] = "off"
				}
			}
			return true, nil
		})
	}
	return recreateContainerWithProperties(ctx, cli, containerID, props)
}

// syncSerialAfterStart runs the sync for a container that was just started
// and reports what it did; a failure is a warning, the start itself stands.
func syncSerialAfterStart(ctx context.Context, cli *client.Client, id string) {
	changes, err := SyncSerialDevices(ctx, cli, id)
	if err != nil {
		common.PrintWarningMessage(err.Error())
		return
	}
	if len(changes) > 0 {
		common.PrintInfoMessage("Serial ports: " + strings.Join(changes, ", "))
	}
}

// demuxReader strips the 8-byte stream headers Docker multiplexes on a
// non-TTY exec attach, leaving plain stdout/stderr text.
func demuxReader(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		header := make([]byte, 8)
		for {
			if _, err := io.ReadFull(r, header); err != nil {
				pw.Close()
				return
			}
			size := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
			if _, err := io.CopyN(pw, r, int64(size)); err != nil {
				pw.Close()
				return
			}
		}
	}()
	return pr
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// applySerialHotplug removes the serial ports from a host config's device
// mappings and grants them through the device cgroup instead, when the engine
// supports attaching them on demand (see the file comment). Used by every
// path that assembles a HostConfig: creation, re-creation, upgrade. It
// returns the host paths it took out, for the SerialPortsLabel.
func applySerialHotplug(hc *container.HostConfig) []string {
	if hc == nil || hc.Privileged || !SerialHotplugSupported(GetEngine()) {
		return nil
	}
	var rest []container.DeviceMapping
	var ports []string
	for _, dev := range hc.Devices {
		if IsSerialDevicePath(dev.PathOnHost) {
			ports = appendMissing(ports, dev.PathOnHost)
			if !devicePresent(dev.PathOnHost) {
				continue // absent now: attached on demand instead of failing the start
			}
		}
		rest = append(rest, dev)
	}
	if len(ports) == 0 {
		return nil
	}
	hc.Devices = rest
	hc.DeviceCgroupRules = appendMissing(hc.DeviceCgroupRules, SerialCgroupRules...)
	return ports
}

// serialSpecs splits device specs into the serial ports absent from the host
// right now (attached on demand: not mapped, granted through the cgroup) and
// the rest, which includes serial ports that are plugged in: those are mapped
// like any device so they are visible and usable at once, and the cgroup
// rules keep them usable after a replug.
func serialSpecs(specs []string) (absentSerial, rest []string) {
	for _, spec := range specs {
		host := strings.SplitN(strings.TrimSpace(spec), ":", 2)[0]
		if IsSerialDevicePath(host) && !devicePresent(host) {
			absentSerial = append(absentSerial, spec)
		} else {
			rest = append(rest, spec)
		}
	}
	return absentSerial, rest
}

// devicePresent reports whether a host device node exists.
func devicePresent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeDevice != 0
}

// serialPortsIn lists the serial ports named by device specs.
func serialPortsIn(specs []string) []string {
	var ports []string
	for _, spec := range specs {
		host := strings.SplitN(strings.TrimSpace(spec), ":", 2)[0]
		if IsSerialDevicePath(host) {
			ports = appendMissing(ports, host)
		}
	}
	return ports
}
