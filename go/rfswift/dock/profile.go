/* This code is part of RF Switch by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Profile system for quick container presets (YAML-based)
 */
package dock

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	common "penthertz/rfswift/common"
)

// Profile defines a preset configuration for quick container creation.
// Profiles are stored as YAML files in the user's profiles directory.
type Profile struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	Image        string `yaml:"image"`
	Network      string `yaml:"network,omitempty"`
	ExposedPorts string `yaml:"exposed_ports,omitempty"`
	PortBindings string `yaml:"port_bindings,omitempty"`
	Desktop      bool   `yaml:"desktop,omitempty"`
	DesktopSSL   bool   `yaml:"desktop_ssl,omitempty"`
	NoX11        bool   `yaml:"no_x11,omitempty"`
	Privileged   bool   `yaml:"privileged,omitempty"`
	Realtime     bool   `yaml:"realtime,omitempty"`
	Devices      string `yaml:"devices,omitempty"`
	Bindings     string `yaml:"bindings,omitempty"`
	Caps         string `yaml:"caps,omitempty"`
	Cgroups      string `yaml:"cgroups,omitempty"`
	VPN          string `yaml:"vpn,omitempty"`
}

// defaultProfiles are written to the profiles directory on `rfswift profile init`.
// They serve as starter templates that users can edit or delete.
var defaultProfiles = []Profile{
	{
		Name:        "sdr-full",
		Description: "Full SDR suite with all tools and device support",
		Image:       "penthertz/rfswift_noble:sdr_full",
		Network:     "host",
		Realtime:    true,
	},
	{
		Name:        "sdr-light",
		Description: "Lightweight SDR tools for quick analysis",
		Image:       "penthertz/rfswift_noble:sdr_light",
		Network:     "host",
	},
	{
		Name:        "wifi",
		Description: "WiFi pentesting and assessment tools",
		Image:       "penthertz/rfswift_noble:wifi",
		Network:     "host",
		Privileged:  true,
	},
	{
		Name:        "bluetooth",
		Description: "Bluetooth pentesting and sniffing tools",
		Image:       "penthertz/rfswift_noble:bluetooth",
		Network:     "host",
	},
	{
		Name:        "telecom",
		Description: "Telecom (2G-5G) analysis and testing tools",
		Image:       "penthertz/rfswift_noble:telecom_4Gto5G",
		Network:     "host",
		Realtime:    true,
	},
	{
		Name:        "rfid",
		Description: "RFID/NFC analysis and cloning tools",
		Image:       "penthertz/rfswift_noble:rfid",
		Network:     "host",
	},
	{
		Name:        "automotive",
		Description: "Automotive RF and CAN bus tools",
		Image:       "penthertz/rfswift_noble:automotive",
		Network:     "host",
		Realtime:    true,
	},
	{
		Name:        "hardware",
		Description: "Hardware hacking and debugging tools (JTAG, SWD, UART)",
		Image:       "penthertz/rfswift_noble:hardware",
		Network:     "host",
	},
	{
		Name:        "reversing",
		Description: "Reverse engineering and firmware analysis tools",
		Image:       "penthertz/rfswift_noble:reversing",
		Network:     "host",
		Desktop:     true,
	},
	{
		Name:        "network",
		Description: "Network security and analysis tools",
		Image:       "penthertz/rfswift_noble:network",
		Network:     "nat",
	},
	{
		Name:        "pentest-full",
		Description: "Full pentest setup: SDR + desktop + NAT isolation",
		Image:       "penthertz/rfswift_noble:sdr_full",
		Network:     "nat",
		Desktop:     true,
		Privileged:  true,
		Realtime:    true,
	},
	{
		Name:        "headless",
		Description: "Minimal headless SDR — no GUI, isolated network",
		Image:       "penthertz/rfswift_noble:sdr_light",
		Network:     "nat",
		NoX11:       true,
	},
}

// ProfilesDirByPlatform returns the platform-specific profiles directory path.
func ProfilesDirByPlatform() string {
	homeDir := os.Getenv("HOME")
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			homeDir = u.HomeDir
		}
	}

	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "rfswift", "profiles")
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "rfswift", "profiles")
	default:
		return filepath.Join(homeDir, ".config", "rfswift", "profiles")
	}
}

// LoadProfiles loads all profiles from the user's profiles directory.
func LoadProfiles() []Profile {
	dir := ProfilesDirByPlatform()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var profiles []Profile
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var p Profile
		if err := yaml.Unmarshal(data, &p); err != nil {
			continue
		}
		if p.Name == "" {
			p.Name = strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".yaml"), ".yml")
		}
		profiles = append(profiles, p)
	}
	return profiles
}

// GetAllProfiles returns all profiles from the YAML directory.
func GetAllProfiles() []Profile {
	return LoadProfiles()
}

// GetProfileByName finds a profile by name from the YAML directory.
func GetProfileByName(name string) (*Profile, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range LoadProfiles() {
		if strings.ToLower(p.Name) == name {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("profile '%s' not found. Run 'rfswift profile init' to generate default profiles or 'rfswift profile create' to create one", name)
}

// SaveProfile saves a profile as a YAML file in the profiles directory.
func SaveProfile(p *Profile) error {
	dir := ProfilesDirByPlatform()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create profiles directory: %w", err)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	filename := profileFilename(p.Name)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}

	return nil
}

// DeleteProfile removes a profile YAML file by name.
func DeleteProfile(name string) error {
	dir := ProfilesDirByPlatform()

	// Try exact filename match
	filename := profileFilename(name)
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' not found at %s", name, path)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to remove profile: %w", err)
	}

	common.PrintSuccessMessage(fmt.Sprintf("Profile '%s' removed", name))
	return nil
}

// InitDefaultProfiles writes the default profile YAML files to the profiles directory.
// Existing files are not overwritten unless force is true.
func InitDefaultProfiles(force bool) (created int, skipped int) {
	dir := ProfilesDirByPlatform()
	if err := os.MkdirAll(dir, 0755); err != nil {
		common.PrintErrorMessage(fmt.Errorf("failed to create profiles directory: %w", err))
		return 0, 0
	}

	for _, p := range defaultProfiles {
		filename := profileFilename(p.Name)
		path := filepath.Join(dir, filename)

		if !force {
			if _, err := os.Stat(path); err == nil {
				skipped++
				continue
			}
		}

		data, err := yaml.Marshal(&p)
		if err != nil {
			common.PrintWarningMessage(fmt.Sprintf("Failed to marshal profile '%s': %v", p.Name, err))
			continue
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			common.PrintWarningMessage(fmt.Sprintf("Failed to write profile '%s': %v", p.Name, err))
			continue
		}
		created++
	}
	return created, skipped
}

// GetProfileNames returns just the names of all available profiles.
func GetProfileNames() []string {
	profiles := LoadProfiles()
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	return names
}

// profileFilename converts a profile name to a safe filename.
func profileFilename(name string) string {
	safe := strings.ToLower(strings.TrimSpace(name))
	safe = strings.ReplaceAll(safe, " ", "-")
	return safe + ".yaml"
}
