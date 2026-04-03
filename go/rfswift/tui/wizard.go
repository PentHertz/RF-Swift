/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// RunWizardResult holds the values collected by the interactive run wizard.
type RunWizardResult struct {
	Image        string
	Name         string
	Bindings     string // comma-separated host:container volume bindings
	Devices      string // comma-separated device paths
	Network      string // network mode: "host", "nat", "bridge", or custom
	ExposedPorts string // comma-separated exposed ports (e.g., "8080/tcp,443/tcp")
	PortBindings string // comma-separated port bindings (e.g., "8080:8080/tcp,443:443/tcp")
	Caps         string // comma-separated capabilities (e.g., "NET_ADMIN,SYS_RAWIO")
	Cgroups      string // comma-separated cgroup rules (e.g., "c 189:* rwm,c 116:* rwm")
	Desktop      bool
	DesktopSSL   bool
	DesktopHost  string // host-side bind address for desktop port (e.g., "127.0.0.1" or "0.0.0.0")
	DesktopPort  string // desktop port (e.g., "6080")
	NoX11        bool
	Privileged   int
	Realtime     bool
	VPN          string // format: "type:argument"
	GPUs         string // GPU specifier: "all" or comma-separated IDs
	Workspace    string // "none" = disabled, "" = auto, path = custom
	Confirmed    bool
}

// ProfileOption represents a profile choice for the wizard.
type ProfileOption struct {
	Name         string
	Description  string
	Image        string
	Network      string
	ExposedPorts string
	PortBindings string
	Desktop      bool
	DesktopSSL   bool
	NoX11        bool
	Privileged   bool
	Realtime     bool
	Devices      string
	Bindings     string
	Caps         string
	Cgroups      string
	GPUs         string
	VPN          string
}

// RunWizardDefaults holds pre-existing CLI flag values to pre-populate the wizard.
type RunWizardDefaults struct {
	Image          string
	Name           string
	Bindings       string
	Devices        string
	Network        string
	ExposedPorts   string
	PortBindings   string
	Caps           string
	Cgroups        string
	Desktop        bool
	DesktopSSL     bool
	NoX11          bool
	Privileged     int
	Realtime       bool
	VPN            string
	GPUs           string
	Workspace      string          // "" = auto, "none" = disabled, path = custom
	WorkspaceRoot  string          // default workspace root (for display in wizard)
	Profiles       []ProfileOption // available profiles for wizard selection
}

// RunWizard launches an interactive form to configure a new container.
// images is a list of available image tags to select from.
// defaults contains pre-existing CLI flag values to pre-populate wizard fields.
// existingNets is a list of existing NAT network names that the user can join.
// Returns the wizard result or an error if cancelled.
func RunWizard(images []string, defaults *RunWizardDefaults, existingNets []string) (*RunWizardResult, error) {
	if !IsInteractive() {
		return nil, fmt.Errorf("interactive terminal required for wizard mode")
	}

	result := &RunWizardResult{}

	// Seed result with defaults from CLI flags
	if defaults != nil {
		result.Image = defaults.Image
		result.Name = defaults.Name
		result.Bindings = defaults.Bindings
		result.Devices = defaults.Devices
		result.ExposedPorts = defaults.ExposedPorts
		result.PortBindings = defaults.PortBindings
		result.Caps = defaults.Caps
		result.Cgroups = defaults.Cgroups
		result.Desktop = defaults.Desktop
		result.DesktopSSL = defaults.DesktopSSL
		result.NoX11 = defaults.NoX11
		result.Privileged = defaults.Privileged
		result.Realtime = defaults.Realtime
		result.Network = defaults.Network
		result.VPN = defaults.VPN
		result.GPUs = defaults.GPUs
		result.Workspace = defaults.Workspace
	}

	// Step 0: Profile selection (skip if image already provided via CLI or --profile)
	profileUsed := false
	useProfileAsIs := false
	if result.Image == "" && defaults != nil && len(defaults.Profiles) > 0 {
		// Build profile options: "No profile" + all available profiles
		profileOpts := []huh.Option[string]{
			huh.NewOption("No profile (manual configuration)", ""),
		}
		for _, p := range defaults.Profiles {
			label := fmt.Sprintf("%s — %s", p.Name, p.Description)
			profileOpts = append(profileOpts, huh.NewOption(label, p.Name))
		}

		var selectedProfile string
		err := huh.NewSelect[string]().
			Title("Start from a profile?").
			Description("Profiles pre-fill image, network, and features.").
			Options(profileOpts...).
			Value(&selectedProfile).
			Run()
		if err != nil {
			return nil, err
		}

		if selectedProfile != "" {
			for _, p := range defaults.Profiles {
				if p.Name == selectedProfile {
					profileUsed = true
					result.Image = p.Image
					result.Network = p.Network
					result.Desktop = p.Desktop
					result.DesktopSSL = p.DesktopSSL
					result.NoX11 = p.NoX11
					if p.Privileged {
						result.Privileged = 1
					}
					result.Realtime = p.Realtime
					if p.Devices != "" {
						result.Devices = p.Devices
					}
					if p.Bindings != "" {
						result.Bindings = p.Bindings
					}
					if p.ExposedPorts != "" {
						result.ExposedPorts = p.ExposedPorts
					}
					if p.PortBindings != "" {
						result.PortBindings = p.PortBindings
					}
					if p.Caps != "" {
						result.Caps = p.Caps
					}
					if p.Cgroups != "" {
						result.Cgroups = p.Cgroups
					}
					if p.GPUs != "" {
						result.GPUs = p.GPUs
					}
					if p.VPN != "" {
						result.VPN = p.VPN
					}
					break
				}
			}

			// Ask: use profile as-is or customize?
			useProfileAsIs = true
			err = huh.NewConfirm().
				Title(fmt.Sprintf("Use profile '%s' as-is?", selectedProfile)).
				Description("Yes = just set a container name and go. No = customize all settings.").
				Affirmative("Yes, use as-is").
				Negative("No, let me customize").
				Value(&useProfileAsIs).
				Run()
			if err != nil {
				return nil, err
			}
		}
	}

	// Fast path: profile as-is — only ask for container name, then recap & confirm
	if profileUsed && useProfileAsIs {
		if result.Name == "" {
			err := newInput().
				Title("Container name").
				Placeholder("my_sdr").
				Value(&result.Name).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("name is required")
					}
					return nil
				}).
				Run()
			if err != nil {
				return nil, err
			}
		}

		// Recap
		items := map[string]string{
			"Image":        result.Image,
			"Name":         result.Name,
			"Bindings":     valueOrNone(result.Bindings),
			"Devices":      valueOrNone(result.Devices),
			"Ports":        valueOrNone(result.PortBindings),
			"Capabilities": valueOrNone(result.Caps),
			"Cgroups":      valueOrNone(result.Cgroups),
			"GPUs":         valueOrNone(result.GPUs),
			"Network":      result.Network,
			"Desktop":      boolStr(result.Desktop),
			"SSL/TLS":      boolStr(result.DesktopSSL),
			"X11":          boolStr(!result.NoX11),
			"Privileged":   boolStr(result.Privileged == 1),
			"Realtime":     boolStr(result.Realtime),
			"VPN":          valueOrNone(result.VPN),
		}
		keys := []string{"Image", "Name", "Bindings", "Devices", "Ports", "Capabilities", "Cgroups", "GPUs", "Network", "Desktop", "SSL/TLS", "X11", "Privileged", "Realtime", "VPN"}
		PrintRecap("Container Configuration (from profile)", items, keys)

		cmd := buildCLICommand(result)
		PrintCLIEquivalent(cmd)

		result.Confirmed = Confirm("Create this container?")
		return result, nil
	}

	// Step 1: Image selection
	// When a profile was selected, show image pre-filled but let user change it
	if profileUsed {
		// Build options with the profile image first, plus all available images
		imgOpts := []huh.Option[string]{
			huh.NewOption(fmt.Sprintf("%s (from profile)", result.Image), result.Image),
		}
		for _, img := range images {
			if img != result.Image {
				imgOpts = append(imgOpts, huh.NewOption(img, img))
			}
		}
		// Add manual input option
		imgOpts = append(imgOpts, huh.NewOption("Other (enter manually)", "__manual__"))

		err := huh.NewSelect[string]().
			Title("Image").
			Description("Profile default pre-selected. Choose a different image to keep the profile settings with another image.").
			Options(imgOpts...).
			Value(&result.Image).
			Run()
		if err != nil {
			return nil, err
		}
		if result.Image == "__manual__" {
			result.Image = ""
			err = newInput().
				Title("Image name (e.g., penthertz/rfswift:sdr_full)").
				Value(&result.Image).
				Run()
			if err != nil {
				return nil, err
			}
		}
	} else if result.Image == "" {
		if len(images) == 0 {
			// No images available, ask for manual input
			err := newInput().
				Title("Image name (e.g., penthertz/rfswift:sdr_full)").
				Value(&result.Image).
				Run()
			if err != nil {
				return nil, err
			}
		} else {
			opts := make([]huh.Option[string], len(images))
			for i, img := range images {
				opts[i] = huh.NewOption(img, img)
			}
			err := huh.NewSelect[string]().
				Title("Select an image").
				Options(opts...).
				Value(&result.Image).
				Run()
			if err != nil {
				return nil, err
			}
		}
	}

	// Step 2: Container name (skip if already provided via CLI)
	if result.Name == "" {
		err := newInput().
			Title("Container name").
			Placeholder("my_sdr").
			Value(&result.Name).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("name is required")
				}
				return nil
			}).
			Run()
		if err != nil {
			return nil, err
		}
	}

	// Step 2b: Workspace directory
	// Show workspace config unless explicitly disabled via CLI
	if result.Workspace != "none" {
		defaultWsPath := ""
		if defaults != nil && defaults.WorkspaceRoot != "" {
			defaultWsPath = defaults.WorkspaceRoot + "/" + result.Name
		}

		wsChoice := "auto"
		if result.Workspace != "" && result.Workspace != "none" {
			wsChoice = "custom"
		}

		wsOptions := []huh.Option[string]{
			huh.NewOption(fmt.Sprintf("Auto (~/rfswift-workspace/%s/)", result.Name), "auto"),
			huh.NewOption("Custom path", "custom"),
			huh.NewOption("Current directory", "cwd"),
			huh.NewOption("Disable workspace", "none"),
		}
		_ = defaultWsPath // used for display

		err := huh.NewSelect[string]().
			Title("Workspace directory").
			Description("Shared folder between host and container — files are always accessible at /workspace").
			Options(wsOptions...).
			Value(&wsChoice).
			Run()
		if err != nil {
			return nil, err
		}

		switch wsChoice {
		case "auto":
			result.Workspace = "" // auto = default
		case "none":
			result.Workspace = "none"
		case "cwd":
			// Will be resolved by the caller
			result.Workspace = "cwd"
		case "custom":
			customPath := result.Workspace
			err := newInput().
				Title("Workspace path on host").
				Description("This directory will be mounted at /workspace inside the container").
				Placeholder("/home/user/my-project").
				Value(&customPath).
				Run()
			if err != nil {
				return nil, err
			}
			result.Workspace = customPath
		}
	}

	// Step 3: Volume bindings (pre-filled if set via CLI)
	addBindings := result.Bindings != ""
	err := huh.NewConfirm().
		Title("Add volume bindings?").
		Description("Mount host directories into the container (e.g., share data, configs)").
		Affirmative("Yes").
		Negative("No").
		Value(&addBindings).
		Run()
	if err != nil {
		return nil, err
	}
	if addBindings {
		err = newInput().
			Title("Volume bindings").
			Description("host_path:container_path — separate multiple with commas").
			Placeholder("/home/user/data:/root/data,/tmp/captures:/tmp/captures").
			Value(&result.Bindings).
			Run()
		if err != nil {
			return nil, err
		}
	} else {
		result.Bindings = ""
	}

	// Step 4: Device mappings (pre-filled if set via CLI)
	addDevices := result.Devices != ""
	err = huh.NewConfirm().
		Title("Add device mappings?").
		Description("Pass host devices (SDR dongles, serial ports, etc.) into the container").
		Affirmative("Yes").
		Negative("No").
		Value(&addDevices).
		Run()
	if err != nil {
		return nil, err
	}
	if addDevices {
		err = newInput().
			Title("Device mappings").
			Description("Device paths — separate multiple with commas").
			Placeholder("/dev/ttyUSB0,/dev/bus/usb").
			Value(&result.Devices).
			Run()
		if err != nil {
			return nil, err
		}
	} else {
		result.Devices = ""
	}

	// Step 5: Port mappings (simplified — user enters host:container pairs)
	var portMappings string
	// Pre-fill from existing port bindings if set
	if result.PortBindings != "" {
		portMappings = result.PortBindings
	}
	addPorts := portMappings != ""
	err = huh.NewConfirm().
		Title("Expose ports?").
		Description("Map container ports to host ports (e.g., for web UIs, APIs)").
		Affirmative("Yes").
		Negative("No").
		Value(&addPorts).
		Run()
	if err != nil {
		return nil, err
	}
	if addPorts {
		err = newInput().
			Title("Port mappings").
			Description("hostPort:containerPort — separate multiple with commas (e.g., 8080:80,4443:443)").
			Placeholder("8080:80,4443:443").
			Value(&portMappings).
			Run()
		if err != nil {
			return nil, err
		}
		if portMappings != "" {
			result.ExposedPorts, result.PortBindings = expandPortMappings(portMappings)
		}
	} else {
		result.ExposedPorts = ""
		result.PortBindings = ""
	}

	// Step 6: Network mode selection
	if result.Network == "" {
		result.Network = "host" // default
	}

	// Build network options: host, bridge, nat (new), plus existing NAT networks
	netOptions := []huh.Option[string]{
		huh.NewOption("Host (shared network stack — default)", "host"),
		huh.NewOption("NAT (create new isolated network)", "nat"),
	}

	// Add existing NAT networks if any
	if existingNets != nil {
		for _, netName := range existingNets {
			label := fmt.Sprintf("NAT: join '%s'", netName)
			netOptions = append(netOptions, huh.NewOption(label, "nat:"+netName))
		}
	}

	netOptions = append(netOptions, huh.NewOption("Bridge (Docker default bridge)", "bridge"))

	err = huh.NewSelect[string]().
		Title("Network mode").
		Description("host: share host network | nat: isolated network | bridge: Docker default").
		Options(netOptions...).
		Value(&result.Network).
		Run()
	if err != nil {
		return nil, err
	}

	// If creating a new NAT network (not joining an existing one), offer custom subnet
	if result.Network == "nat" {
		var customSubnet string
		err = newInput().
			Title("Custom subnet (leave empty for auto-allocation)").
			Description("e.g., 10.10.0.0/24 or 192.168.100.0/28 — empty uses 172.30.x.x/28").
			Value(&customSubnet).
			Run()
		if err != nil {
			return nil, err
		}
		if customSubnet != "" {
			result.Network = "nat::" + customSubnet
		}
	}

	// Step 6: Feature toggles (pre-select features already enabled via CLI)
	var features []string
	// Pre-populate from defaults
	if result.Desktop {
		features = append(features, "desktop")
	}
	if result.DesktopSSL {
		features = append(features, "desktop-ssl")
	}
	if result.NoX11 {
		features = append(features, "no-x11")
	}
	if result.Privileged == 1 {
		features = append(features, "privileged")
	}
	if result.Realtime {
		features = append(features, "realtime")
	}
	if result.VPN != "" {
		features = append(features, "vpn")
	}
	if result.GPUs != "" {
		features = append(features, "gpus")
	}
	err = huh.NewMultiSelect[string]().
		Title("Enable features").
		Options(
			huh.NewOption("Remote Desktop (VNC/noVNC)", "desktop"),
			huh.NewOption("Desktop SSL/TLS", "desktop-ssl"),
			huh.NewOption("Disable X11 forwarding", "no-x11"),
			huh.NewOption("Privileged mode", "privileged"),
			huh.NewOption("Realtime mode (audio/SDR)", "realtime"),
			huh.NewOption("GPU passthrough", "gpus"),
			huh.NewOption("VPN (WireGuard/OpenVPN/Tailscale/Netbird)", "vpn"),
		).
		Value(&features).
		Run()
	if err != nil {
		return nil, err
	}

	// Reset all togglable fields based on wizard selection (user may have unchecked)
	result.Desktop = false
	result.DesktopSSL = false
	result.NoX11 = false
	result.Privileged = 0
	result.Realtime = false
	result.GPUs = ""
	// Preserve VPN config temporarily for pre-populating the VPN sub-form
	savedVPN := result.VPN
	result.VPN = ""

	// Check if vpn was selected
	vpnSelected := false
	for _, f := range features {
		if f == "vpn" {
			vpnSelected = true
			break
		}
	}
	if !vpnSelected {
		savedVPN = ""
	}

	for _, f := range features {
		switch f {
		case "desktop":
			result.Desktop = true

			// Follow-up: configure desktop port exposure for non-host networks
			if result.Network != "" && result.Network != "host" {
				desktopHost := "127.0.0.1"
				desktopPort := "6080"

				err = huh.NewSelect[string]().
					Title("Desktop bind address").
					Description("Which host address should the desktop port be exposed on?").
					Options(
						huh.NewOption("127.0.0.1 (localhost only — recommended)", "127.0.0.1"),
						huh.NewOption("0.0.0.0 (all interfaces — accessible from network)", "0.0.0.0"),
					).
					Value(&desktopHost).
					Run()
				if err != nil {
					return nil, err
				}

				err = newInput().
					Title("Desktop port").
					Description("Host port to map to the container's desktop service").
					Placeholder("6080").
					Value(&desktopPort).
					Run()
				if err != nil {
					return nil, err
				}
				if desktopPort == "" {
					desktopPort = "6080"
				}

				result.DesktopHost = desktopHost
				result.DesktopPort = desktopPort
			}
		case "desktop-ssl":
			result.DesktopSSL = true
		case "no-x11":
			result.NoX11 = true
		case "privileged":
			result.Privileged = 1
		case "realtime":
			result.Realtime = true
		case "gpus":
			result.GPUs = "all"
		case "vpn":
			// Follow-up: select VPN type and config
			// Pre-populate from existing VPN config if set via CLI
			var vpnType, vpnArg string
			if savedVPN != "" {
				parts := strings.SplitN(savedVPN, ":", 2)
				vpnType = parts[0]
				if len(parts) > 1 {
					vpnArg = parts[1]
				}
			}
			err = huh.NewSelect[string]().
				Title("VPN type").
				Options(
					huh.NewOption("WireGuard", "wireguard"),
					huh.NewOption("OpenVPN", "openvpn"),
					huh.NewOption("Tailscale", "tailscale"),
					huh.NewOption("Netbird", "netbird"),
				).
				Value(&vpnType).
				Run()
			if err != nil {
				return nil, err
			}

			switch vpnType {
			case "wireguard":
				err = newInput().
					Title("WireGuard config file path").
					Placeholder("./wg0.conf").
					Value(&vpnArg).
					Run()
				if err != nil {
					return nil, err
				}
			case "openvpn":
				err = newInput().
					Title("OpenVPN config file path").
					Placeholder("./client.ovpn").
					Value(&vpnArg).
					Run()
				if err != nil {
					return nil, err
				}
			case "tailscale":
				err = newInput().
					Title("Tailscale auth key (leave empty for interactive login)").
					Placeholder("tskey-auth-xxxxx or empty").
					Value(&vpnArg).
					Run()
				if err != nil {
					return nil, err
				}
			case "netbird":
				err = newInput().
					Title("Netbird setup key (leave empty for interactive login)").
					Placeholder("setup key or empty").
					Value(&vpnArg).
					Run()
				if err != nil {
					return nil, err
				}
			}
			if vpnArg != "" {
				result.VPN = vpnType + ":" + vpnArg
			} else {
				result.VPN = vpnType
			}
		}
	}

	// If desktop SSL is enabled without desktop, enable desktop
	if result.DesktopSSL && !result.Desktop {
		result.Desktop = true
	}

	// Step 8: Capabilities selection
	// Pre-populate from existing caps
	var existingCaps []string
	if result.Caps != "" {
		for _, c := range strings.Split(result.Caps, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				existingCaps = append(existingCaps, c)
			}
		}
	}

	addCaps := len(existingCaps) > 0
	err = huh.NewConfirm().
		Title("Add extra Linux capabilities?").
		Description("Capabilities grant specific privileges to the container (e.g., raw network access, hardware I/O)").
		Affirmative("Yes").
		Negative("No").
		Value(&addCaps).
		Run()
	if err != nil {
		return nil, err
	}
	if addCaps {
		// Pre-select existing caps
		selectedCaps := make([]string, len(existingCaps))
		copy(selectedCaps, existingCaps)

		err = huh.NewMultiSelect[string]().
			Title("Select capabilities").
			Description("Common capabilities for RF/hardware work. Pre-selected are from your config.").
			Options(
				huh.NewOption("NET_ADMIN — network config, monitor mode, packet capture", "NET_ADMIN"),
				huh.NewOption("NET_RAW — raw sockets, packet injection", "NET_RAW"),
				huh.NewOption("SYS_RAWIO — raw I/O port access (SDR, hardware)", "SYS_RAWIO"),
				huh.NewOption("SYS_ADMIN — mount, BPF, perf events, namespace ops", "SYS_ADMIN"),
				huh.NewOption("SYS_PTRACE — process tracing and debugging", "SYS_PTRACE"),
				huh.NewOption("SYS_NICE — set realtime scheduling priority", "SYS_NICE"),
				huh.NewOption("SYS_TTY_CONFIG — virtual terminal config", "SYS_TTY_CONFIG"),
				huh.NewOption("SYS_RESOURCE — override resource limits", "SYS_RESOURCE"),
				huh.NewOption("SYS_MODULE — load/unload kernel modules", "SYS_MODULE"),
				huh.NewOption("IPC_LOCK — lock memory (mlock, mlockall)", "IPC_LOCK"),
				huh.NewOption("DAC_OVERRIDE — bypass file permission checks", "DAC_OVERRIDE"),
				huh.NewOption("MKNOD — create special device files", "MKNOD"),
				huh.NewOption("SETUID — set UID of process", "SETUID"),
				huh.NewOption("SETGID — set GID of process", "SETGID"),
				huh.NewOption("CHOWN — change file ownership", "CHOWN"),
				huh.NewOption("FOWNER — bypass permission checks on file owner", "FOWNER"),
				huh.NewOption("KILL — send signals to any process", "KILL"),
				huh.NewOption("AUDIT_WRITE — write to the kernel audit log", "AUDIT_WRITE"),
			).
			Value(&selectedCaps).
			Run()
		if err != nil {
			return nil, err
		}
		result.Caps = strings.Join(selectedCaps, ",")
	} else {
		result.Caps = ""
	}

	// Step 9: Cgroup rules selection
	var existingCgroups []string
	if result.Cgroups != "" {
		for _, c := range strings.Split(result.Cgroups, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				existingCgroups = append(existingCgroups, c)
			}
		}
	}

	addCgroups := len(existingCgroups) > 0
	err = huh.NewConfirm().
		Title("Add device cgroup rules?").
		Description("Cgroup rules grant kernel-level access to device classes (needed for USB hotplug, sound, etc.)").
		Affirmative("Yes").
		Negative("No").
		Value(&addCgroups).
		Run()
	if err != nil {
		return nil, err
	}
	if addCgroups {
		selectedCgroups := make([]string, len(existingCgroups))
		copy(selectedCgroups, existingCgroups)

		err = huh.NewMultiSelect[string]().
			Title("Select cgroup rules").
			Description("Common device access rules for RF/hardware work. Pre-selected are from your config.").
			Options(cgroupRuleOptions()...).
			Value(&selectedCgroups).
			Run()
		if err != nil {
			return nil, err
		}
		result.Cgroups = strings.Join(selectedCgroups, ",")
	} else {
		result.Cgroups = ""
	}

	// Recap
	desktopLabel := boolStr(result.Desktop)
	if result.Desktop && result.DesktopHost != "" {
		desktopLabel = fmt.Sprintf("enabled (%s:%s)", result.DesktopHost, result.DesktopPort)
	}
	// Workspace display
	workspaceLabel := fmt.Sprintf("~/rfswift-workspace/%s/", result.Name)
	switch result.Workspace {
	case "none":
		workspaceLabel = lipgloss.NewStyle().Foreground(ColorMuted).Render("disabled")
	case "cwd":
		workspaceLabel = "current directory"
	case "":
		// default - show auto path
	default:
		workspaceLabel = result.Workspace
	}

	items := map[string]string{
		"Image":        result.Image,
		"Name":         result.Name,
		"Workspace":    workspaceLabel,
		"Bindings":     valueOrNone(result.Bindings),
		"Devices":      valueOrNone(result.Devices),
		"Ports":        valueOrNone(result.PortBindings),
		"Capabilities": valueOrNone(result.Caps),
		"Cgroups":      valueOrNone(result.Cgroups),
		"GPUs":         valueOrNone(result.GPUs),
		"Network":      result.Network,
		"Desktop":      desktopLabel,
		"SSL/TLS":      boolStr(result.DesktopSSL),
		"X11":          boolStr(!result.NoX11),
		"Privileged":   boolStr(result.Privileged == 1),
		"Realtime":     boolStr(result.Realtime),
		"VPN":          valueOrNone(result.VPN),
	}
	keys := []string{"Image", "Name", "Workspace", "Bindings", "Devices", "Ports", "Capabilities", "Cgroups", "GPUs", "Network", "Desktop", "SSL/TLS", "X11", "Privileged", "Realtime", "VPN"}
	PrintRecap("Container Configuration", items, keys)

	// Build equivalent CLI command
	cmd := buildCLICommand(result)
	PrintCLIEquivalent(cmd)

	// Confirm
	result.Confirmed = Confirm("Create this container?")
	return result, nil
}

func boolStr(b bool) string {
	if b {
		return lipgloss.NewStyle().Foreground(ColorSuccess).Render("enabled")
	}
	return lipgloss.NewStyle().Foreground(ColorMuted).Render("disabled")
}

func valueOrNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return lipgloss.NewStyle().Foreground(ColorMuted).Render("none")
	}
	return s
}

// expandPortMappings converts simplified port mappings (e.g., "8080:80,4443:443")
// into the exposedPorts and portBindings format expected by the container engine.
//
// Input format: "hostPort:containerPort" pairs separated by commas.
// Supports optional protocol suffix: "8080:80/udp" (defaults to tcp).
// Supports optional bind address: "0.0.0.0:8080:80/tcp".
//
// Returns:
//   - exposedPorts: "80/tcp,443/tcp" (container-side)
//   - portBindings: "8080:80/tcp,4443:443/tcp" (host:container, passed to -w flag)
func expandPortMappings(input string) (exposedPorts string, portBindings string) {
	var exposed []string
	var bindings []string

	for _, mapping := range strings.Split(input, ",") {
		mapping = strings.TrimSpace(mapping)
		if mapping == "" {
			continue
		}

		parts := strings.Split(mapping, ":")
		var hostAddr, hostPort, containerPortProto string

		switch len(parts) {
		case 1:
			// Just a port: "8080" → expose and bind same port
			containerPortProto = ensureProto(parts[0])
			hostPort = stripProto(parts[0])
		case 2:
			// "hostPort:containerPort"
			hostPort = strings.TrimSpace(parts[0])
			containerPortProto = ensureProto(parts[1])
		case 3:
			// "bindAddr:hostPort:containerPort"
			hostAddr = strings.TrimSpace(parts[0])
			hostPort = strings.TrimSpace(parts[1])
			containerPortProto = ensureProto(parts[2])
		default:
			continue
		}

		exposed = append(exposed, containerPortProto)

		if hostAddr != "" {
			bindings = append(bindings, fmt.Sprintf("%s:%s:%s", hostAddr, hostPort, containerPortProto))
		} else {
			bindings = append(bindings, fmt.Sprintf("%s:%s", hostPort, containerPortProto))
		}
	}

	return strings.Join(exposed, ","), strings.Join(bindings, ",")
}

// ensureProto adds "/tcp" if no protocol suffix is present.
func ensureProto(port string) string {
	port = strings.TrimSpace(port)
	if !strings.Contains(port, "/") {
		return port + "/tcp"
	}
	return port
}

// stripProto removes the protocol suffix from a port string.
func stripProto(port string) string {
	port = strings.TrimSpace(port)
	if idx := strings.Index(port, "/"); idx != -1 {
		return port[:idx]
	}
	return port
}

func buildCLICommand(r *RunWizardResult) string {
	parts := []string{"rfswift run"}
	parts = append(parts, fmt.Sprintf("-i %s", r.Image))
	parts = append(parts, fmt.Sprintf("-n %s", r.Name))
	if r.Bindings != "" {
		parts = append(parts, fmt.Sprintf("-b %s", r.Bindings))
	}
	if r.Devices != "" {
		parts = append(parts, fmt.Sprintf("-s %s", r.Devices))
	}
	if r.ExposedPorts != "" {
		parts = append(parts, fmt.Sprintf("-z %s", r.ExposedPorts))
	}
	if r.PortBindings != "" {
		parts = append(parts, fmt.Sprintf("-w %s", r.PortBindings))
	}
	if r.Caps != "" {
		parts = append(parts, fmt.Sprintf("-a %s", r.Caps))
	}
	if r.Cgroups != "" {
		parts = append(parts, fmt.Sprintf("-g %s", r.Cgroups))
	}
	if r.Network != "" && r.Network != "host" {
		parts = append(parts, fmt.Sprintf("-t %s", r.Network))
	}
	if r.Desktop {
		parts = append(parts, "--desktop")
		if r.DesktopHost != "" || r.DesktopPort != "" {
			host := r.DesktopHost
			if host == "" {
				host = "127.0.0.1"
			}
			port := r.DesktopPort
			if port == "" {
				port = "6080"
			}
			parts = append(parts, fmt.Sprintf("--desktop-config http:%s:%s", host, port))
		}
	}
	if r.DesktopSSL {
		parts = append(parts, "--desktop-ssl")
	}
	if r.NoX11 {
		parts = append(parts, "--no-x11")
	}
	if r.Privileged == 1 {
		parts = append(parts, "-u 1")
	}
	if r.Realtime {
		parts = append(parts, "--realtime")
	}
	if r.GPUs != "" {
		parts = append(parts, fmt.Sprintf("--gpus %s", r.GPUs))
	}
	if r.VPN != "" {
		parts = append(parts, fmt.Sprintf("--vpn %s", r.VPN))
	}
	return strings.Join(parts, " ")
}

// ProfileCreateResult holds the values collected by the profile creation wizard.
type ProfileCreateResult struct {
	Name         string
	Description  string
	Image        string
	Network      string
	ExposedPorts string
	PortBindings string
	Desktop      bool
	DesktopSSL   bool
	NoX11        bool
	Privileged   bool
	Realtime     bool
	Devices      string
	Bindings     string
	Caps         string
	Cgroups      string
	GPUs         string
	VPN          string
}

// ProfileCreateWizard launches an interactive form to create a new profile.
// images is a list of available image tags to select from.
// existingNets is a list of existing NAT network names.
func ProfileCreateWizard(images []string, existingNets []string) (*ProfileCreateResult, error) {
	if !IsInteractive() {
		return nil, fmt.Errorf("interactive terminal required for wizard mode")
	}

	result := &ProfileCreateResult{}

	// Step 1: Profile name
	err := newInput().
		Title("Profile name").
		Placeholder("my-sdr-setup").
		Value(&result.Name).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("name is required")
			}
			return nil
		}).
		Run()
	if err != nil {
		return nil, err
	}

	// Step 2: Description
	err = newInput().
		Title("Description").
		Placeholder("Brief description of what this profile is for").
		Value(&result.Description).
		Run()
	if err != nil {
		return nil, err
	}

	// Step 3: Image selection
	if len(images) == 0 {
		err = newInput().
			Title("Image name (e.g., penthertz/rfswift_noble:sdr_full)").
			Value(&result.Image).
			Run()
		if err != nil {
			return nil, err
		}
	} else {
		imgOpts := make([]huh.Option[string], 0, len(images)+1)
		for _, img := range images {
			imgOpts = append(imgOpts, huh.NewOption(img, img))
		}
		imgOpts = append(imgOpts, huh.NewOption("Other (enter manually)", "__manual__"))

		err = huh.NewSelect[string]().
			Title("Select an image").
			Options(imgOpts...).
			Value(&result.Image).
			Run()
		if err != nil {
			return nil, err
		}
		if result.Image == "__manual__" {
			result.Image = ""
			err = newInput().
				Title("Image name (e.g., penthertz/rfswift_noble:sdr_full)").
				Value(&result.Image).
				Run()
			if err != nil {
				return nil, err
			}
		}
	}

	// Step 4: Network mode
	result.Network = "host"
	netOptions := []huh.Option[string]{
		huh.NewOption("Host (shared network stack — default)", "host"),
		huh.NewOption("NAT (isolated network)", "nat"),
	}
	if existingNets != nil {
		for _, netName := range existingNets {
			label := fmt.Sprintf("NAT: join '%s'", netName)
			netOptions = append(netOptions, huh.NewOption(label, "nat:"+netName))
		}
	}
	netOptions = append(netOptions, huh.NewOption("Bridge (Docker default bridge)", "bridge"))

	err = huh.NewSelect[string]().
		Title("Network mode").
		Options(netOptions...).
		Value(&result.Network).
		Run()
	if err != nil {
		return nil, err
	}

	// If creating a new NAT network (not joining an existing one), offer custom subnet
	if result.Network == "nat" {
		var customSubnet string
		err = newInput().
			Title("Custom subnet (leave empty for auto-allocation)").
			Description("e.g., 10.10.0.0/24 or 192.168.100.0/28 — empty uses 172.30.x.x/28").
			Value(&customSubnet).
			Run()
		if err != nil {
			return nil, err
		}
		if customSubnet != "" {
			result.Network = "nat::" + customSubnet
		}
	}

	// Step 5: Feature toggles
	var features []string
	err = huh.NewMultiSelect[string]().
		Title("Enable features").
		Options(
			huh.NewOption("Remote Desktop (VNC/noVNC)", "desktop"),
			huh.NewOption("Desktop SSL/TLS", "desktop-ssl"),
			huh.NewOption("Disable X11 forwarding", "no-x11"),
			huh.NewOption("Privileged mode", "privileged"),
			huh.NewOption("Realtime mode (audio/SDR)", "realtime"),
			huh.NewOption("GPU passthrough", "gpus"),
		).
		Value(&features).
		Run()
	if err != nil {
		return nil, err
	}

	for _, f := range features {
		switch f {
		case "desktop":
			result.Desktop = true
		case "desktop-ssl":
			result.DesktopSSL = true
		case "no-x11":
			result.NoX11 = true
		case "privileged":
			result.Privileged = true
		case "realtime":
			result.Realtime = true
		case "gpus":
			result.GPUs = "all"
		}
	}

	if result.DesktopSSL && !result.Desktop {
		result.Desktop = true
	}

	// Step 6: Optional device mappings
	addDevices := false
	err = huh.NewConfirm().
		Title("Add default device mappings?").
		Affirmative("Yes").
		Negative("No").
		Value(&addDevices).
		Run()
	if err != nil {
		return nil, err
	}
	if addDevices {
		err = newInput().
			Title("Device mappings").
			Description("Device paths — separate multiple with commas").
			Placeholder("/dev/ttyUSB0,/dev/bus/usb").
			Value(&result.Devices).
			Run()
		if err != nil {
			return nil, err
		}
	}

	// Step 7: Optional volume bindings
	addBindings := false
	err = huh.NewConfirm().
		Title("Add default volume bindings?").
		Affirmative("Yes").
		Negative("No").
		Value(&addBindings).
		Run()
	if err != nil {
		return nil, err
	}
	if addBindings {
		err = newInput().
			Title("Volume bindings").
			Description("host_path:container_path — separate multiple with commas").
			Placeholder("/home/user/data:/root/data").
			Value(&result.Bindings).
			Run()
		if err != nil {
			return nil, err
		}
	}

	// Step 8: Optional port mappings
	var portMappings string
	addPorts := false
	err = huh.NewConfirm().
		Title("Expose ports?").
		Description("Map container ports to host ports (e.g., for web UIs, APIs)").
		Affirmative("Yes").
		Negative("No").
		Value(&addPorts).
		Run()
	if err != nil {
		return nil, err
	}
	if addPorts {
		err = newInput().
			Title("Port mappings").
			Description("hostPort:containerPort — separate multiple with commas (e.g., 8080:80,4443:443)").
			Placeholder("8080:80,4443:443").
			Value(&portMappings).
			Run()
		if err != nil {
			return nil, err
		}
		if portMappings != "" {
			result.ExposedPorts, result.PortBindings = expandPortMappings(portMappings)
		}
	}

	// Step 9: Capabilities selection
	addCaps := false
	err = huh.NewConfirm().
		Title("Add extra Linux capabilities?").
		Description("Capabilities grant specific privileges (e.g., raw network access, hardware I/O)").
		Affirmative("Yes").
		Negative("No").
		Value(&addCaps).
		Run()
	if err != nil {
		return nil, err
	}
	if addCaps {
		var selectedCaps []string
		err = huh.NewMultiSelect[string]().
			Title("Select capabilities").
			Options(
				huh.NewOption("NET_ADMIN — network config, monitor mode, packet capture", "NET_ADMIN"),
				huh.NewOption("NET_RAW — raw sockets, packet injection", "NET_RAW"),
				huh.NewOption("SYS_RAWIO — raw I/O port access (SDR, hardware)", "SYS_RAWIO"),
				huh.NewOption("SYS_ADMIN — mount, BPF, perf events, namespace ops", "SYS_ADMIN"),
				huh.NewOption("SYS_PTRACE — process tracing and debugging", "SYS_PTRACE"),
				huh.NewOption("SYS_NICE — set realtime scheduling priority", "SYS_NICE"),
				huh.NewOption("SYS_TTY_CONFIG — virtual terminal config", "SYS_TTY_CONFIG"),
				huh.NewOption("SYS_RESOURCE — override resource limits", "SYS_RESOURCE"),
				huh.NewOption("SYS_MODULE — load/unload kernel modules", "SYS_MODULE"),
				huh.NewOption("IPC_LOCK — lock memory (mlock, mlockall)", "IPC_LOCK"),
				huh.NewOption("DAC_OVERRIDE — bypass file permission checks", "DAC_OVERRIDE"),
				huh.NewOption("MKNOD — create special device files", "MKNOD"),
				huh.NewOption("SETUID — set UID of process", "SETUID"),
				huh.NewOption("SETGID — set GID of process", "SETGID"),
				huh.NewOption("CHOWN — change file ownership", "CHOWN"),
				huh.NewOption("FOWNER — bypass permission checks on file owner", "FOWNER"),
				huh.NewOption("KILL — send signals to any process", "KILL"),
				huh.NewOption("AUDIT_WRITE — write to the kernel audit log", "AUDIT_WRITE"),
			).
			Value(&selectedCaps).
			Run()
		if err != nil {
			return nil, err
		}
		result.Caps = strings.Join(selectedCaps, ",")
	}

	// Step 10: Cgroup rules selection
	addCgroups := false
	err = huh.NewConfirm().
		Title("Add device cgroup rules?").
		Description("Cgroup rules grant kernel-level access to device classes (needed for USB hotplug, sound, etc.)").
		Affirmative("Yes").
		Negative("No").
		Value(&addCgroups).
		Run()
	if err != nil {
		return nil, err
	}
	if addCgroups {
		var selectedCgroups []string
		err = huh.NewMultiSelect[string]().
			Title("Select cgroup rules").
			Options(cgroupRuleOptions()...).
			Value(&selectedCgroups).
			Run()
		if err != nil {
			return nil, err
		}
		result.Cgroups = strings.Join(selectedCgroups, ",")
	}

	// Recap
	items := map[string]string{
		"Name":         result.Name,
		"Description":  result.Description,
		"Image":        result.Image,
		"Network":      result.Network,
		"Ports":        valueOrNone(result.PortBindings),
		"Capabilities": valueOrNone(result.Caps),
		"Cgroups":      valueOrNone(result.Cgroups),
		"GPUs":         valueOrNone(result.GPUs),
		"Desktop":      boolStr(result.Desktop),
		"SSL/TLS":      boolStr(result.DesktopSSL),
		"X11":          boolStr(!result.NoX11),
		"Privileged":   boolStr(result.Privileged),
		"Realtime":     boolStr(result.Realtime),
		"Devices":      valueOrNone(result.Devices),
		"Bindings":     valueOrNone(result.Bindings),
	}
	keys := []string{"Name", "Description", "Image", "Network", "Ports", "Capabilities", "Cgroups", "GPUs", "Desktop", "SSL/TLS", "X11", "Privileged", "Realtime", "Devices", "Bindings"}
	PrintRecap("New Profile", items, keys)

	if !Confirm("Save this profile?") {
		return nil, fmt.Errorf("cancelled")
	}

	return result, nil
}

// cgroupRuleOptions returns the huh options for common device cgroup rules.
func cgroupRuleOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("c 189:* rwm — USB devices (SDR dongles, serial adapters)", "c 189:* rwm"),
		huh.NewOption("c 188:* rwm — USB serial (ttyUSB)", "c 188:* rwm"),
		huh.NewOption("c 166:* rwm — ACM modems (ttyACM)", "c 166:* rwm"),
		huh.NewOption("c 116:* rwm — ALSA sound devices", "c 116:* rwm"),
		huh.NewOption("c 226:* rwm — DRI/GPU rendering", "c 226:* rwm"),
		huh.NewOption("c 13:* rwm — Input devices (HID, joystick)", "c 13:* rwm"),
		huh.NewOption("c 137:* rwm — VHCI (virtual HCI for Bluetooth)", "c 137:* rwm"),
		huh.NewOption("c 180:* rwm — USB character devices", "c 180:* rwm"),
		huh.NewOption("c 204:* rwm — Low-density serial ports", "c 204:* rwm"),
		huh.NewOption("c 29:* rwm — Framebuffer devices", "c 29:* rwm"),
		huh.NewOption("c *:* rwm — All character devices (wildcard)", "c *:* rwm"),
	}
}
