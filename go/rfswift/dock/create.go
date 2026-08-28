package dock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	rfutils "penthertz/rfswift/rfutils"
)

type PullProgress struct {
	Image   string
	Layer   string
	Status  string
	Current int64
	Total   int64
}

type ImageAvailability struct {
	Resolved        string
	Present         bool
	UpdateAvailable bool
	Custom          bool
}

// CheckImage reports local presence and, for official RF-Swift images, whether
// the registry contains a newer digest/version.
func CheckImage(engineName, imageName string) (ImageAvailability, error) {
	SetPreferredEngine(engineName)
	engine := GetEngine()
	if engine == nil || string(engine.Type()) != strings.ToLower(engineName) {
		return ImageAvailability{}, fmt.Errorf("%s is not available", engineName)
	}
	cli, err := NewEngineClient()
	if err != nil {
		return ImageAvailability{}, err
	}
	defer cli.Close()
	resolved := normalizeImageName(imageName)
	result := ImageAvailability{Resolved: strings.TrimSpace(imageName), Custom: !IsOfficialImage(resolved)}
	if _, err := ImageInspectCompat(context.Background(), cli, resolved); err != nil {
		// An official image may already exist under an equivalent registry prefix,
		// Ubuntu codename, or architecture alias. Resolve the requested toolset tag
		// against the local engine before telling the GUI it must pull again.
		_, wantedTag := parseImageName(resolved)
		if images, listErr := cli.ImageList(context.Background(), client.ImageListOptions{All: true}); listErr == nil {
			for _, local := range images.Items {
				for _, localRef := range local.RepoTags {
					clean := strings.TrimPrefix(localRef, "docker.io/")
					_, localTag := parseImageName(clean)
					if localTag == wantedTag && IsOfficialImage(clean) {
						result.Present = true
						result.Custom = false
						return result, nil
					}
				}
			}
		}
		return result, nil
	}
	result.Present = true
	if result.Custom {
		return result, nil
	}
	repo, tag := parseImageName(resolved)
	current, custom, err := checkImageStatus(context.Background(), cli, repo, tag)
	if err != nil {
		return result, err
	}
	result.Custom = custom
	result.UpdateAvailable = !custom && !current
	return result, nil
}

var containerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// CreateOptions is the non-interactive container creation surface used by
// desktop/API clients. ContainerRun remains the interactive CLI workflow.
type CreateOptions struct {
	Context         context.Context
	Engine          string
	Name            string
	Image           string
	Workspace       string
	Network         string
	Shell           string
	Caps            []string
	Bindings        []string
	Devices         []string
	ExposedPorts    string
	PortBindings    string
	CgroupRules     []string
	GPUs            string
	Seccomp         string
	ExtraHosts      []string
	Environment     []string
	Realtime        bool
	Desktop         bool
	DesktopProto    string
	DesktopHost     string
	DesktopPort     string
	DesktopPassword string
	DesktopSSL      bool
	NoX11           bool
	Privileged      bool
	Start           bool
}

// CreateContainer creates an RF-Swift-labelled container without attaching an
// interactive terminal. The image must already be present locally.
func CreateContainer(opts CreateOptions) (string, error) {
	if !containerNamePattern.MatchString(opts.Name) {
		return "", errors.New("container name may contain only letters, numbers, '.', '_' and '-'")
	}
	if strings.TrimSpace(opts.Image) == "" {
		return "", errors.New("container image is required")
	}
	if opts.Engine != "" {
		SetPreferredEngine(opts.Engine)
		selected := GetEngine()
		if selected == nil || string(selected.Type()) != strings.ToLower(opts.Engine) {
			actual := "unavailable"
			if selected != nil {
				actual = string(selected.Type())
			}
			return "", fmt.Errorf("%s was requested but is not available (selected engine: %s)", opts.Engine, actual)
		}
	}
	if opts.Network == "" {
		opts.Network = "bridge"
	}
	if opts.Shell == "" {
		opts.Shell = "/bin/zsh"
	}
	cli, err := NewEngineClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	resolvedImage := normalizeImageName(opts.Image)
	if _, err := ImageInspectCompat(ctx, cli, resolvedImage); err != nil {
		_, wantedTag := parseImageName(resolvedImage)
		if images, listErr := cli.ImageList(ctx, client.ImageListOptions{All: true}); listErr == nil {
			for _, local := range images.Items {
				for _, localRef := range local.RepoTags {
					clean := strings.TrimPrefix(localRef, "docker.io/")
					_, localTag := parseImageName(clean)
					if localTag == wantedTag && IsOfficialImage(clean) {
						resolvedImage = clean
						break
					}
				}
			}
		}
		if _, retryErr := ImageInspectCompat(ctx, cli, resolvedImage); retryErr != nil {
			return "", fmt.Errorf("image %q resolves to %q but is not available in %s; pull it first with: rfswift --engine %s pull -i %s: %w", opts.Image, resolvedImage, opts.Engine, opts.Engine, opts.Image, err)
		}
	}
	binds := append([]string(nil), opts.Bindings...)
	devices, cgroupRules := normalizeCreationDevices(opts.Devices, binds, opts.CgroupRules)
	// Directory-style device mappings infer major-number cgroup rules. After a
	// rootless user explicitly clears the rule field, do not silently recreate
	// those unsupported rules from /dev/bus/usb, /dev/snd, etc. Keep their bind
	// mounts; actual access remains governed by the host user's permissions.
	if IsRootlessPodman() && len(opts.CgroupRules) == 0 {
		cgroupRules = nil
	}
	binds = devices.binds
	configEnv := append([]string(nil), opts.Environment...)
	if !opts.NoX11 {
		display := os.Getenv("DISPLAY")
		// On macOS the raw DISPLAY is the XQuartz launchd socket path
		// (/var/run/.../org.xquartz:0), which is meaningless inside a container.
		// Resolve the host's en0 IP + display number (DISPLAY=<ip>:0) and open
		// the X server to it with xhost, matching the interactive CLI path. This
		// runs for every CreateContainer caller, so the Workbench GUI gets the
		// same working X display as the rfswift binary.
		if runtime.GOOS == "darwin" {
			display = strings.TrimPrefix(rfutils.GetDisplayEnv(), "DISPLAY=")
			rfutils.XHostEnable()
		}
		if display == "" {
			display = ":0"
		}
		configEnv = append(configEnv, "DISPLAY="+display)
		if runtime.GOOS == "windows" {
			// WSLg serves DISPLAY=:0 for every WSL 2 VM; mount its socket tree
			// the way the CLI does (see cli setupX11).
			for _, bind := range WSLgBindings() {
				if !bindTargets(binds, parseBindDestination(bind)) {
					binds = append(binds, bind)
				}
			}
		} else if _, err := os.Stat("/tmp/.X11-unix"); err == nil && !bindExistsByPrefix(binds, "/tmp/.X11-unix:/tmp/.X11-unix") {
			binds = append(binds, "/tmp/.X11-unix:/tmp/.X11-unix:rw")
		}
		binds, configEnv = addForwardedXAuthority(display, binds, configEnv)
	}
	// Audio: same PulseAudio target as the CLI (WSLg socket on Windows, the
	// configured TCP server elsewhere) unless the caller set PULSE_SERVER.
	if !envHasKey(configEnv, "PULSE_SERVER") {
		if pulse := resolvePulseServer(containerCfg.pulseServer); pulse != "" {
			configEnv = append(configEnv, "PULSE_SERVER="+pulse)
		}
		binds = ensureWSLgMount(binds, containerCfg.pulseServer)
	}
	if opts.Workspace != "none" {
		workspace := opts.Workspace
		if workspace == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			workspace = filepath.Join(home, "rfswift-workspace", opts.Name)
		}
		workspace, err = filepath.Abs(workspace)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			return "", err
		}
		binds = append(binds, workspace+":/workspace:rw")
	}
	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(opts.Network), Binds: binds,
		CapAdd: append([]string(nil), opts.Caps...), Privileged: opts.Privileged,
		Devices:           getDeviceMappingsFromString(strings.Join(devices.nodes, ",")),
		DeviceCgroupRules: cgroupRules,
		PortBindings:      ParseBindedPorts(opts.PortBindings), ExtraHosts: opts.ExtraHosts,
	}
	if opts.Seccomp != "" && opts.Seccomp != "default" {
		hostConfig.SecurityOpt = []string{"seccomp=" + opts.Seccomp}
	}
	if opts.Realtime {
		hostConfig.CapAdd = append(hostConfig.CapAdd, "SYS_NICE")
		hostConfig.Resources.Ulimits = append(hostConfig.Resources.Ulimits,
			&container.Ulimit{Name: "rtprio", Soft: 95, Hard: 95},
			&container.Ulimit{Name: "memlock", Soft: -1, Hard: -1})
	}
	if opts.GPUs != "" {
		applyGPUConfig(opts.GPUs, hostConfig)
	}
	var entrypoint []string
	if opts.Desktop {
		if opts.DesktopProto == "" {
			opts.DesktopProto = "http"
		}
		if opts.DesktopProto != "http" && opts.DesktopProto != "vnc" {
			return "", errors.New("desktop protocol must be http (noVNC) or vnc")
		}
		if opts.DesktopHost == "" {
			opts.DesktopHost = "127.0.0.1"
		}
		if opts.DesktopPort == "" {
			opts.DesktopPort = "6080"
			if opts.DesktopProto == "vnc" {
				opts.DesktopPort = "5900"
			}
		}
		listenHost := opts.DesktopHost
		if opts.Network != "host" {
			listenHost = "0.0.0.0"
			portProto := opts.DesktopPort + "/tcp"
			exposed := ParseExposedPorts(opts.ExposedPorts)
			for port := range ParseExposedPorts(portProto) {
				exposed[port] = struct{}{}
			}
			hostConfig.PortBindings = ParseBindedPorts(opts.PortBindings)
			for port, bindings := range ParseBindedPorts(opts.DesktopHost + ":" + opts.DesktopPort + ":" + portProto) {
				hostConfig.PortBindings[port] = bindings
			}
			opts.ExposedPorts = exposedPortsString(exposed)
		}
		sslFlag := ""
		if opts.DesktopSSL {
			sslFlag = "1"
		}
		configEnv = append(configEnv,
			"RFSWIFT_DESKTOP_PROTO="+opts.DesktopProto,
			"RFSWIFT_DESKTOP_HOST="+listenHost,
			"RFSWIFT_DESKTOP_PORT="+opts.DesktopPort,
			"RFSWIFT_DESKTOP_PASS="+opts.DesktopPassword,
			"RFSWIFT_DESKTOP_SSL="+sslFlag)
		entrypoint = []string{"/usr/sbin/rfswift-entrypoint"}
	}
	networkingConfig := &network.NetworkingConfig{}
	createdNATNetwork := false
	natNetworkName := ""
	if isNAT, natName, subnet := parseNATValue(opts.Network); isNAT {
		if natName == "" {
			createdNATNetwork = true
		} else {
			fullName := natName
			if !strings.HasPrefix(fullName, NATNetworkPrefix) {
				fullName = NATNetworkPrefix + fullName
			}
			if existing, _ := findNATNetwork(ctx, cli, fullName); existing == "" {
				createdNATNetwork = true
			}
		}
		name, _, err := createOrJoinNATNetwork(ctx, cli, opts.Name, natName, subnet)
		if err != nil {
			return "", err
		}
		natNetworkName = name
		hostConfig.NetworkMode = container.NetworkMode(name)
		networkingConfig.EndpointsConfig = map[string]*network.EndpointSettings{name: {}}
	}
	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: opts.Name,
		Config: &container.Config{
			Image: resolvedImage, Cmd: []string{opts.Shell},
			OpenStdin: true, Tty: true, Env: configEnv, Entrypoint: entrypoint,
			ExposedPorts: ParseExposedPorts(opts.ExposedPorts),
			Labels:       map[string]string{"org.container.project": "rfswift"},
		},
		HostConfig:       hostConfig,
		NetworkingConfig: networkingConfig,
	})
	if err != nil {
		if createdNATNetwork && natNetworkName != "" {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
			_, _ = cli.NetworkRemove(cleanupCtx, natNetworkName, client.NetworkRemoveOptions{})
			cleanupCancel()
		}
		return "", err
	}
	if opts.Start {
		if _, err := cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cleanupCancel()
			cleanupErr := error(nil)
			if _, removeErr := cli.ContainerRemove(cleanupCtx, resp.ID, client.ContainerRemoveOptions{Force: true}); removeErr != nil {
				cleanupErr = removeErr
			}
			if createdNATNetwork && natNetworkName != "" {
				if _, removeErr := cli.NetworkRemove(cleanupCtx, natNetworkName, client.NetworkRemoveOptions{}); removeErr != nil && cleanupErr == nil {
					cleanupErr = removeErr
				}
			}
			if cleanupErr != nil {
				return "", fmt.Errorf("container start failed: %w; automatic cleanup also failed: %v", err, cleanupErr)
			}
			return "", fmt.Errorf("container start failed and the partial container was removed: %w", err)
		}
	}
	return resp.ID, nil
}

type normalizedCreationDevices struct {
	binds []string
	nodes []string
}

// normalizeCreationDevices gives config.ini directory mappings hotplug
// semantics. A /dev subtree is a bind mount plus a major-number cgroup rule;
// individual character/block nodes remain Docker device mappings.
func normalizeCreationDevices(specs, binds, rules []string) (normalizedCreationDevices, []string) {
	result := normalizedCreationDevices{binds: append([]string(nil), binds...)}
	outRules := append([]string(nil), rules...)
	majorRules := map[string]string{"/dev/bus/usb": "c 189:* rwm", "/dev/snd": "c 116:* rwm", "/dev/dri": "c 226:* rwm", "/dev/input": "c 13:* rwm", "/dev/vhci": "c 137:* rwm"}
	contains := func(values []string, wanted string) bool {
		for _, value := range values {
			if strings.TrimSpace(value) == wanted {
				return true
			}
		}
		return false
	}
	for _, spec := range specs {
		parts := strings.SplitN(strings.TrimSpace(spec), ":", 3)
		if len(parts) < 2 {
			continue
		}
		host, target := parts[0], parts[1]
		_, knownTree := majorRules[target]
		isDir := false
		if stat, err := os.Stat(host); err == nil {
			isDir = stat.IsDir()
		}
		if knownTree || isDir {
			mount := host + ":" + target + ":rw"
			if !bindExistsByPrefix(result.binds, host+":"+target) {
				result.binds = append(result.binds, mount)
			}
			if rule := majorRules[target]; rule != "" && !contains(outRules, rule) {
				outRules = append(outRules, rule)
			}
			continue
		}
		result.nodes = append(result.nodes, spec)
	}
	return result, outRules
}

func exposedPortsString(ports network.PortSet) string {
	items := make([]string, 0, len(ports))
	for port := range ports {
		items = append(items, port.String())
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func parseNATValue(mode string) (bool, string, string) {
	if strings.EqualFold(mode, "nat") {
		return true, "", ""
	}
	if !strings.HasPrefix(strings.ToLower(mode), "nat:") {
		return false, "", ""
	}
	parts := strings.SplitN(mode[4:], ":", 2)
	if len(parts) == 1 {
		return true, parts[0], ""
	}
	return true, parts[0], parts[1]
}

// PullImage pulls an image into a specifically selected engine and reports the
// registry's per-layer progress. RF-Swift short names use the same repository
// and architecture-tag resolution as ContainerPull.
func PullImage(engineName, imageName string, progress func(PullProgress)) (string, error) {
	return PullImageContext(context.Background(), engineName, imageName, progress)
}

// PullImageContext is the cancellable variant used by the Workbench creation
// flow. Canceling closes the registry stream and prevents container creation.
func PullImageContext(ctx context.Context, engineName, imageName string, progress func(PullProgress)) (string, error) {
	SetPreferredEngine(engineName)
	engine := GetEngine()
	if engine == nil || string(engine.Type()) != strings.ToLower(engineName) {
		return "", fmt.Errorf("%s is not available", engineName)
	}
	cli, err := NewEngineClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	cleanRef := normalizeImageName(imageName)
	pullRef := cleanRef
	if IsOfficialImage(cleanRef) {
		arch := getArchitecture()
		if arch == "" {
			return "", errors.New("cannot determine system architecture")
		}
		parts := strings.SplitN(cleanRef, ":", 2)
		if len(parts) == 2 && !strings.HasSuffix(parts[1], "_"+arch) {
			pullRef = parts[0] + ":" + parts[1] + "_" + arch
		}
	}
	stream, err := cli.ImagePull(ctx, pullRef, client.ImagePullOptions{})
	if err != nil {
		return "", err
	}
	defer stream.Close()
	dec := json.NewDecoder(stream)
	for {
		var msg jsonstream.Message
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
		if msg.Error != nil {
			return "", errors.New(msg.Error.Message)
		}
		p := PullProgress{Image: cleanRef, Layer: msg.ID, Status: msg.Status}
		if msg.Progress != nil {
			p.Current, p.Total = msg.Progress.Current, msg.Progress.Total
		}
		if progress != nil {
			progress(p)
		}
	}
	if pullRef != cleanRef {
		if _, err := cli.ImageTag(ctx, client.ImageTagOptions{Source: pullRef, Target: cleanRef}); err != nil {
			return "", err
		}
	}
	return cleanRef, nil
}
