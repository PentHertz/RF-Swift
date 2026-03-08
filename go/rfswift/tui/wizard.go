/* This code is part of RF Switch by @Penthertz
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
	Image       string
	Name        string
	Bindings    string // comma-separated host:container volume bindings
	Devices     string // comma-separated device paths
	Desktop     bool
	DesktopSSL  bool
	NoX11       bool
	Privileged  int
	Realtime    bool
	VPN         string // format: "type:argument"
	Confirmed   bool
}

// RunWizardDefaults holds pre-existing CLI flag values to pre-populate the wizard.
type RunWizardDefaults struct {
	Image      string
	Name       string
	Bindings   string
	Devices    string
	Desktop    bool
	DesktopSSL bool
	NoX11      bool
	Privileged int
	Realtime   bool
	VPN        string
}

// RunWizard launches an interactive form to configure a new container.
// images is a list of available image tags to select from.
// defaults contains pre-existing CLI flag values to pre-populate wizard fields.
// Returns the wizard result or an error if cancelled.
func RunWizard(images []string, defaults *RunWizardDefaults) (*RunWizardResult, error) {
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
		result.Desktop = defaults.Desktop
		result.DesktopSSL = defaults.DesktopSSL
		result.NoX11 = defaults.NoX11
		result.Privileged = defaults.Privileged
		result.Realtime = defaults.Realtime
		result.VPN = defaults.VPN
	}

	// Step 1: Image selection (skip if already provided via CLI)
	if result.Image == "" {
		if len(images) == 0 {
			// No images available, ask for manual input
			err := huh.NewInput().
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
		err := huh.NewInput().
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
		err = huh.NewInput().
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
		err = huh.NewInput().
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

	// Step 5: Feature toggles (pre-select features already enabled via CLI)
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
	err = huh.NewMultiSelect[string]().
		Title("Enable features").
		Options(
			huh.NewOption("Remote Desktop (VNC/noVNC)", "desktop"),
			huh.NewOption("Desktop SSL/TLS", "desktop-ssl"),
			huh.NewOption("Disable X11 forwarding", "no-x11"),
			huh.NewOption("Privileged mode", "privileged"),
			huh.NewOption("Realtime mode (audio/SDR)", "realtime"),
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
		case "desktop-ssl":
			result.DesktopSSL = true
		case "no-x11":
			result.NoX11 = true
		case "privileged":
			result.Privileged = 1
		case "realtime":
			result.Realtime = true
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
				err = huh.NewInput().
					Title("WireGuard config file path").
					Placeholder("./wg0.conf").
					Value(&vpnArg).
					Run()
				if err != nil {
					return nil, err
				}
			case "openvpn":
				err = huh.NewInput().
					Title("OpenVPN config file path").
					Placeholder("./client.ovpn").
					Value(&vpnArg).
					Run()
				if err != nil {
					return nil, err
				}
			case "tailscale":
				err = huh.NewInput().
					Title("Tailscale auth key (leave empty for interactive login)").
					Placeholder("tskey-auth-xxxxx or empty").
					Value(&vpnArg).
					Run()
				if err != nil {
					return nil, err
				}
			case "netbird":
				err = huh.NewInput().
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

	// Recap
	items := map[string]string{
		"Image":      result.Image,
		"Name":       result.Name,
		"Bindings":   valueOrNone(result.Bindings),
		"Devices":    valueOrNone(result.Devices),
		"Desktop":    boolStr(result.Desktop),
		"SSL/TLS":    boolStr(result.DesktopSSL),
		"X11":        boolStr(!result.NoX11),
		"Privileged": boolStr(result.Privileged == 1),
		"Realtime":   boolStr(result.Realtime),
		"VPN":        valueOrNone(result.VPN),
	}
	keys := []string{"Image", "Name", "Bindings", "Devices", "Desktop", "SSL/TLS", "X11", "Privileged", "Realtime", "VPN"}
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
	if r.Desktop {
		parts = append(parts, "--desktop")
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
	if r.VPN != "" {
		parts = append(parts, fmt.Sprintf("--vpn %s", r.VPN))
	}
	return strings.Join(parts, " ")
}
