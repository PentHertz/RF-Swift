/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Profile system for quick container presets (YAML-based)
 */
package dock

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	GPUs         string `yaml:"gpus,omitempty"`
	VPN          string `yaml:"vpn,omitempty"`
}

// Building blocks shared by the default profiles.
const (
	// usbTreeBinding bind-mounts the whole USB device tree instead of mapping
	// individual device nodes. Individual nodes are snapshots taken at creation
	// time, so a dongle unplugged and replugged gets a new node the container
	// cannot see; the bind mount (plus the cgroup rule injected at run time)
	// keeps hotplug working.
	usbTreeBinding = "/dev/bus/usb:/dev/bus/usb"

	// tunDevice is needed to create tunnel interfaces inside the container
	// (VPN clients, srsRAN/UERANSIM/Open5GS GTP tunnels, tun/tap tooling).
	// It requires NET_ADMIN to configure the interface once created.
	tunDevice = "/dev/net/tun:/dev/net/tun"

	// captureCaps is the minimum for raw-socket capture and interface control
	// (Wireshark, tcpdump, Kismet, Bettercap, monitor mode, tun/tap setup).
	// NET_RAW is in Docker's default set but is listed explicitly so the intent
	// survives a restrictive host config.
	captureCaps = "NET_ADMIN,NET_RAW"

	// serviceCaps adds the right to bind privileged ports (<1024) for services
	// run inside the container: DNS, DHCP, HTTP, SMB, SIP, core network ...
	serviceCaps = "NET_ADMIN,NET_RAW,NET_BIND_SERVICE"
)

// officialImage builds the reference of an official RF Swift image for the
// current Ubuntu base, e.g. officialImage("sdr_full") ->
// "penthertz/rfswift_resolute:sdr_full".
//
//	in(1): string tag image tag (image family)
//	out: string fully qualified image reference
func officialImage(tag string) string {
	return fmt.Sprintf("penthertz/rfswift_%s:%s", common.CurrentImageCodename, tag)
}

// DefaultProfiles returns the profiles written to the profiles directory on
// `rfswift profile init`. They are starter templates users can edit or delete.
//
// Two families are provided:
//
//   - Role profiles (yolo, network-host, network-nat) describe *how* the
//     container is wired to the host rather than which toolset it ships.
//   - Toolset profiles (sdr-full, wifi, telecom ...) pair an image family with
//     the devices, capabilities and limits that family actually needs.
//
// The guiding rule is least privilege: capabilities and device mappings are
// preferred over `privileged: true`, which is reserved for the yolo profile.
//
//	out: []Profile default profile set
func DefaultProfiles() []Profile {
	return []Profile{
		// ── Role profiles ──────────────────────────────────────────────
		{
			Name:        "yolo",
			Description: "Everything on: privileged, USB, realtime, GPU",
			Image:       officialImage("sdr_full"),
			Network:     "host",
			Privileged:  true,
			Realtime:    true,
			GPUs:        "all",
			Bindings:    usbTreeBinding,
		},
		{
			Name:        "network-host",
			Description: "Capture and run services on the host network",
			Image:       officialImage("network"),
			Network:     "host",
			Caps:        serviceCaps,
			Devices:     tunDevice,
			Bindings:    usbTreeBinding,
		},
		{
			Name:         "network-nat",
			Description:  "Capture and run services inside an isolated NAT",
			Image:        officialImage("network"),
			Network:      "nat",
			Caps:         serviceCaps,
			Devices:      tunDevice,
			Bindings:     usbTreeBinding,
			ExposedPorts: "8080/tcp,8000/tcp,4444/tcp",
			PortBindings: "127.0.0.1:8080:8080/tcp,127.0.0.1:8000:8000/tcp,127.0.0.1:4444:4444/tcp",
		},

		// ── Toolset profiles ───────────────────────────────────────────
		{
			Name:        "sdr-full",
			Description: "Full SDR suite, realtime and USB hotplug",
			Image:       officialImage("sdr_full"),
			Network:     "host",
			Realtime:    true,
			Bindings:    usbTreeBinding,
		},
		{
			Name:        "sdr-light",
			Description: "Lightweight SDR tools, realtime and USB hotplug",
			Image:       officialImage("sdr_light"),
			Network:     "host",
			Realtime:    true,
			Bindings:    usbTreeBinding,
		},
		{
			Name:        "wifi",
			Description: "Wi-Fi monitor mode and injection, unprivileged",
			Image:       officialImage("wifi"),
			Network:     "host",
			Caps:        captureCaps,
			Devices:     "/dev/rfkill:/dev/rfkill",
			Bindings:    usbTreeBinding,
		},
		{
			Name:        "bluetooth",
			Description: "Bluetooth sniffing and assessment (HCI, vhci)",
			Image:       officialImage("bluetooth"),
			Network:     "host",
			Caps:        captureCaps,
			Devices:     "/dev/rfkill:/dev/rfkill",
			Bindings:    usbTreeBinding,
		},
		{
			Name:        "telecom",
			Description: "4G/5G NSA core and RAN, GTP tunnels, realtime",
			Image:       officialImage("telecom_4G_5GNSA"),
			Network:     "host",
			Realtime:    true,
			Caps:        serviceCaps,
			Devices:     tunDevice,
			Bindings:    usbTreeBinding,
		},
		{
			Name:        "telecom-5g",
			Description: "5G SA core and RAN, GTP tunnels, realtime",
			Image:       officialImage("telecom_5G"),
			Network:     "host",
			Realtime:    true,
			Caps:        serviceCaps,
			Devices:     tunDevice,
			Bindings:    usbTreeBinding,
		},
		{
			// No /dev/ttyACM0 mapping on purpose: a Proxmark3 or other CDC-ACM
			// reader is reached through the USB tree bind + cgroup rule below
			// (hotplug-safe), while a fixed node mapping only exists when the
			// device is plugged in at creation time and otherwise makes the
			// container fail to start. Add it explicitly when a tool insists
			// on the serial node (`--devices /dev/ttyACM0:/dev/ttyACM0`, or the
			// Workbench's Proxmark shortcut).
			Name:        "rfid",
			Description: "RFID/NFC tools (Proxmark3, libnfc) over USB",
			Image:       officialImage("rfid"),
			Network:     "host",
			Bindings:    usbTreeBinding,
			Cgroups:     "c 189:* rwm",
		},
		{
			Name:        "automotive",
			Description: "Automotive RF and CAN bus, realtime and CAN setup",
			Image:       officialImage("automotive"),
			Network:     "host",
			Realtime:    true,
			Caps:        captureCaps,
			Bindings:    usbTreeBinding,
		},
		{
			Name:        "hardware",
			Description: "JTAG/SWD/UART and flashing tools, raw I/O access",
			Image:       officialImage("hardware"),
			Network:     "host",
			Caps:        "SYS_RAWIO",
			Bindings:    usbTreeBinding,
		},
		{
			Name:        "reversing",
			Description: "RE desktop (Ghidra, Cutter) with ptrace, in NAT",
			Image:       officialImage("reversing"),
			Network:     "nat",
			Desktop:     true,
			Caps:        "SYS_PTRACE",
		},
		{
			Name:        "headless",
			Description: "Headless SDR, no GUI, isolated NAT network",
			Image:       officialImage("sdr_light"),
			Network:     "nat",
			NoX11:       true,
			Realtime:    true,
			Bindings:    usbTreeBinding,
		},
	}
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
// If a permission error occurs, the user is prompted to retry with elevated privileges.
func SaveProfile(p *Profile) error {
	dir := ProfilesDirByPlatform()
	if err := os.MkdirAll(dir, 0755); err != nil {
		if !errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("failed to create profiles directory: %w", err)
		}
		if !promptProfileElevation("create profiles directory") {
			return fmt.Errorf("cannot create profiles directory %s: permission denied", dir)
		}
		return writeProfileElevated(p, dir)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	filename := profileFilename(p.Name)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		if !errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("failed to write profile: %w", err)
		}
		if !promptProfileElevation("write profile file") {
			return fmt.Errorf("cannot write profile %s: permission denied", path)
		}
		return writeProfileElevated(p, "")
	}

	return nil
}

// promptProfileElevation asks the user whether to retry with sudo.
func promptProfileElevation(action string) bool {
	fmt.Printf("\033[38;5;208mPermission denied to %s. Retry with elevated privileges (sudo)? (y/n)\033[0m ", action)
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	return strings.ToLower(strings.TrimSpace(response)) == "y"
}

// writeProfileElevated writes a profile file using sudo.
func writeProfileElevated(p *Profile, dir string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("elevated write not supported on Windows; please run as Administrator")
	}
	if dir != "" {
		cmd := exec.Command("sudo", "mkdir", "-p", dir)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create directory with sudo: %w", err)
		}
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	profileDir := ProfilesDirByPlatform()
	filename := profileFilename(p.Name)
	path := filepath.Join(profileDir, filename)

	cmd := exec.Command("sudo", "tee", path)
	cmd.Stdin = strings.NewReader(string(data))
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write profile with sudo: %w", err)
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
// Existing files are not overwritten unless force is true. If a permission error
// occurs, the user is prompted to retry with elevated privileges.
//
//	in(1): bool force overwrite profiles that already exist
//	out: int number of profiles written
//	out: int number of profiles left untouched
//	out: []string names of untouched profiles whose content differs from the
//	     current default (edited locally, or shipped by an older release)
func InitDefaultProfiles(force bool) (created int, skipped int, stale []string) {
	dir := ProfilesDirByPlatform()
	elevated := false

	if err := os.MkdirAll(dir, 0755); err != nil {
		if !errors.Is(err, os.ErrPermission) {
			common.PrintErrorMessage(fmt.Errorf("failed to create profiles directory: %w", err))
			return 0, 0, nil
		}
		if !promptProfileElevation("create profiles directory") {
			common.PrintErrorMessage(fmt.Errorf("cannot create profiles directory %s: permission denied", dir))
			return 0, 0, nil
		}
		cmd := exec.Command("sudo", "mkdir", "-p", dir)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			common.PrintErrorMessage(fmt.Errorf("failed to create directory with sudo: %w", err))
			return 0, 0, nil
		}
		elevated = true
	}

	for _, p := range DefaultProfiles() {
		filename := profileFilename(p.Name)
		path := filepath.Join(dir, filename)

		if !force {
			if _, err := os.Stat(path); err == nil {
				skipped++
				if differsFromDefault(path, p) {
					stale = append(stale, p.Name)
				}
				continue
			}
		}

		data, err := yaml.Marshal(&p)
		if err != nil {
			common.PrintWarningMessage(fmt.Sprintf("Failed to marshal profile '%s': %v", p.Name, err))
			continue
		}

		if elevated {
			cmd := exec.Command("sudo", "tee", path)
			cmd.Stdin = strings.NewReader(string(data))
			cmd.Stdout = nil
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				common.PrintWarningMessage(fmt.Sprintf("Failed to write profile '%s' with sudo: %v", p.Name, err))
				continue
			}
		} else {
			if err := os.WriteFile(path, data, 0644); err != nil {
				if errors.Is(err, os.ErrPermission) && !elevated {
					if promptProfileElevation(fmt.Sprintf("write profile '%s'", p.Name)) {
						elevated = true
						cmd := exec.Command("sudo", "tee", path)
						cmd.Stdin = strings.NewReader(string(data))
						cmd.Stdout = nil
						cmd.Stderr = os.Stderr
						if err := cmd.Run(); err != nil {
							common.PrintWarningMessage(fmt.Sprintf("Failed to write profile '%s' with sudo: %v", p.Name, err))
							continue
						}
					} else {
						common.PrintWarningMessage(fmt.Sprintf("Failed to write profile '%s': permission denied", p.Name))
						continue
					}
				} else {
					common.PrintWarningMessage(fmt.Sprintf("Failed to write profile '%s': %v", p.Name, err))
					continue
				}
			}
		}
		created++
	}
	return created, skipped, stale
}

// differsFromDefault reports whether the profile stored at path no longer
// matches the default RF Swift ships under that name. Unreadable or malformed
// files count as different: refreshing them is the useful advice.
//
//	in(1): string path profile YAML file to compare
//	in(2): Profile def the current default with the same name
//	out: bool true when the stored profile differs from the default
func differsFromDefault(path string, def Profile) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	var stored Profile
	if err := yaml.Unmarshal(data, &stored); err != nil {
		return true
	}
	return stored != def
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
