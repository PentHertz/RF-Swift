/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * CLI commands for profile management
 */

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	"penthertz/rfswift/tui"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage container profiles",
	Long: `Manage RF Swift profiles — YAML presets for quick container creation.

Profiles bundle image, network mode, features (desktop, realtime, privileged),
and device mappings into a single named preset.

Profiles are stored as YAML files in:
  Linux:   ~/.config/rfswift/profiles/
  macOS:   ~/Library/Application Support/rfswift/profiles/
  Windows: %APPDATA%\rfswift\profiles\

Use 'rfswift profile init' to generate default profiles.
Use 'rfswift profile create' to create a new profile interactively.`,
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	Long:  `List all profiles from the profiles directory`,
	Run: func(cmd *cobra.Command, args []string) {
		profiles := rfdock.GetAllProfiles()
		if len(profiles) == 0 {
			common.PrintInfoMessage("No profiles found. Run 'rfswift profile init' to generate defaults or 'rfswift profile create' to create one.")
			return
		}

		var rows [][]string
		for _, p := range profiles {
			features := profileFeatures(p)
			rows = append(rows, []string{
				p.Name,
				p.Description,
				p.Image,
				networkLabel(p.Network),
				features,
			})
		}

		tui.RenderTable(tui.TableConfig{
			Title:   fmt.Sprintf("Profiles (%s)", rfdock.ProfilesDirByPlatform()),
			Headers: []string{"Name", "Description", "Image", "Network", "Features"},
			Rows:    rows,
		})
	},
}

var profileShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show profile details",
	Long:  `Display detailed configuration for a specific profile`,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")

		if name == "" && len(args) > 0 {
			name = args[0]
		}

		if name == "" && tui.IsInteractive() {
			names := rfdock.GetProfileNames()
			if len(names) == 0 {
				common.PrintInfoMessage("No profiles available")
				return
			}
			var err error
			name, err = tui.SelectOne("Select a profile", names)
			if err != nil {
				return
			}
		}

		if name == "" {
			common.PrintErrorMessage(fmt.Errorf("profile name is required"))
			return
		}

		p, err := rfdock.GetProfileByName(name)
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}

		network := p.Network
		if network == "" {
			network = "host"
		}

		items := map[string]string{
			"Name":         p.Name,
			"Description":  p.Description,
			"Image":        p.Image,
			"Network":      networkLabel(network),
			"Ports":        valueOrDash(p.PortBindings),
			"Capabilities": valueOrDash(p.Caps),
			"Cgroups":      valueOrDash(p.Cgroups),
			"Desktop":      enabledStr(p.Desktop),
			"Desktop SSL":  enabledStr(p.DesktopSSL),
			"X11":          enabledStr(!p.NoX11),
			"Privileged":   enabledStr(p.Privileged),
			"Realtime":     enabledStr(p.Realtime),
			"Devices":      valueOrDash(p.Devices),
			"Bindings":     valueOrDash(p.Bindings),
			"GPUs":         valueOrDash(p.GPUs),
			"VPN":          valueOrDash(p.VPN),
		}
		keys := []string{"Name", "Description", "Image", "Network", "Ports", "Capabilities", "Cgroups", "GPUs", "Desktop", "Desktop SSL", "X11", "Privileged", "Realtime", "Devices", "Bindings", "VPN"}

		tui.PrintRecap(fmt.Sprintf("Profile: %s", p.Name), items, keys)

		// Show equivalent run command
		cmdStr := profileToCLICommand(p)
		tui.PrintCLIEquivalent(cmdStr)
	},
}

var profileCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new profile interactively",
	Long:  `Launch an interactive wizard to create a new profile and save it as a YAML file`,
	Run: func(cmd *cobra.Command, args []string) {
		if !tui.IsInteractive() {
			common.PrintErrorMessage(fmt.Errorf("interactive terminal required. Create profiles manually in %s", rfdock.ProfilesDirByPlatform()))
			return
		}

		availableImages := rfdock.ListImageTags("org.container.project", "rfswift")
		existingNets := rfdock.ListNATNetworkNames()

		p, err := tui.ProfileCreateWizard(availableImages, existingNets)
		if err != nil {
			common.PrintErrorMessage(fmt.Errorf("profile creation cancelled: %v", err))
			return
		}

		profile := &rfdock.Profile{
			Name:         p.Name,
			Description:  p.Description,
			Image:        p.Image,
			Network:      p.Network,
			ExposedPorts: p.ExposedPorts,
			PortBindings: p.PortBindings,
			Desktop:      p.Desktop,
			DesktopSSL:   p.DesktopSSL,
			NoX11:        p.NoX11,
			Privileged:   p.Privileged,
			Realtime:     p.Realtime,
			Devices:      p.Devices,
			Bindings:     p.Bindings,
			Caps:         p.Caps,
			Cgroups:      p.Cgroups,
			GPUs:         p.GPUs,
			VPN:          p.VPN,
		}

		if err := rfdock.SaveProfile(profile); err != nil {
			common.PrintErrorMessage(err)
			return
		}

		common.PrintSuccessMessage(fmt.Sprintf("Profile '%s' saved to %s", profile.Name, rfdock.ProfilesDirByPlatform()))
	},
}

var profileInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate default profiles",
	Long: `Generate default RF Swift profile YAML files in the profiles directory.
Existing profiles are not overwritten unless --force is used.`,
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")
		created, skipped := rfdock.InitDefaultProfiles(force)

		if created > 0 {
			common.PrintSuccessMessage(fmt.Sprintf("%d profile(s) created in %s", created, rfdock.ProfilesDirByPlatform()))
		}
		if skipped > 0 {
			common.PrintInfoMessage(fmt.Sprintf("%d profile(s) already exist (use --force to overwrite)", skipped))
		}
		if created == 0 && skipped == 0 {
			common.PrintInfoMessage("No profiles to create")
		}
	},
}

var profileDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a profile",
	Long:  `Delete a profile YAML file from the profiles directory`,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")

		if name == "" && len(args) > 0 {
			name = args[0]
		}

		if name == "" && tui.IsInteractive() {
			profiles := rfdock.GetAllProfiles()
			if len(profiles) == 0 {
				common.PrintInfoMessage("No profiles to delete")
				return
			}
			names := make([]string, len(profiles))
			for i, p := range profiles {
				names[i] = p.Name
			}
			var err error
			name, err = tui.SelectOne("Select a profile to delete", names)
			if err != nil {
				return
			}
		}

		if name == "" {
			common.PrintErrorMessage(fmt.Errorf("profile name is required"))
			return
		}

		if tui.IsInteractive() && !tui.Confirm(fmt.Sprintf("Delete profile '%s'?", name)) {
			common.PrintInfoMessage("Deletion cancelled.")
			return
		}

		if err := rfdock.DeleteProfile(name); err != nil {
			common.PrintErrorMessage(err)
		}
	},
}

func registerProfileCommands() {
	rootCmd.AddCommand(profileCmd)
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileCreateCmd)
	profileCmd.AddCommand(profileInitCmd)
	profileCmd.AddCommand(profileDeleteCmd)

	profileShowCmd.Flags().StringP("name", "n", "", "Profile name")
	profileDeleteCmd.Flags().StringP("name", "n", "", "Profile name")
	profileInitCmd.Flags().Bool("force", false, "Overwrite existing profile files")
}

// profileFeatures returns a comma-separated list of enabled features.
func profileFeatures(p rfdock.Profile) string {
	var feats []string
	if p.Desktop {
		feats = append(feats, "desktop")
	}
	if p.Privileged {
		feats = append(feats, "privileged")
	}
	if p.Realtime {
		feats = append(feats, "realtime")
	}
	if p.NoX11 {
		feats = append(feats, "no-x11")
	}
	if p.VPN != "" {
		feats = append(feats, "vpn")
	}
	if len(feats) == 0 {
		return "-"
	}
	return strings.Join(feats, ", ")
}

// networkLabel returns a human-friendly label for a network mode.
func networkLabel(net string) string {
	switch net {
	case "", "host":
		return "host"
	case "nat":
		return "nat (isolated)"
	case "bridge":
		return "bridge"
	default:
		if strings.HasPrefix(net, "nat:") {
			return fmt.Sprintf("nat: %s", strings.TrimPrefix(net, "nat:"))
		}
		return net
	}
}

func enabledStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func valueOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// profileToCLICommand generates the equivalent rfswift run command for a profile.
func profileToCLICommand(p *rfdock.Profile) string {
	parts := []string{"rfswift run"}
	parts = append(parts, fmt.Sprintf("-i %s", p.Image))
	parts = append(parts, "-n <container_name>")
	if p.Bindings != "" {
		parts = append(parts, fmt.Sprintf("-b %s", p.Bindings))
	}
	if p.Devices != "" {
		parts = append(parts, fmt.Sprintf("-s %s", p.Devices))
	}
	if p.Network != "" && p.Network != "host" {
		parts = append(parts, fmt.Sprintf("-t %s", p.Network))
	}
	if p.Desktop {
		parts = append(parts, "--desktop")
	}
	if p.DesktopSSL {
		parts = append(parts, "--desktop-ssl")
	}
	if p.NoX11 {
		parts = append(parts, "--no-x11")
	}
	if p.Privileged {
		parts = append(parts, "-u 1")
	}
	if p.Realtime {
		parts = append(parts, "--realtime")
	}
	if p.ExposedPorts != "" {
		parts = append(parts, fmt.Sprintf("-z %s", p.ExposedPorts))
	}
	if p.PortBindings != "" {
		parts = append(parts, fmt.Sprintf("-w %s", p.PortBindings))
	}
	if p.Caps != "" {
		parts = append(parts, fmt.Sprintf("-a %s", p.Caps))
	}
	if p.Cgroups != "" {
		parts = append(parts, fmt.Sprintf("-g %s", p.Cgroups))
	}
	if p.GPUs != "" {
		parts = append(parts, fmt.Sprintf("--gpus %s", p.GPUs))
	}
	if p.VPN != "" {
		parts = append(parts, fmt.Sprintf("--vpn %s", p.VPN))
	}
	return strings.Join(parts, " ")
}
