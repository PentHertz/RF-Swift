/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  CLI organization: group `rfswift --help` into readable sections and add a few
*  convenience parent commands (config / system / usb) that gather related
*  subcommands. Every original top-level path is preserved (the commands are
*  registered under the new parents in addition to their original location), so
*  existing scripts and muscle memory keep working.
*
*  Must run AFTER all register*Commands(), so every command already exists.
 */

package cli

import (
	"runtime"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
)

// group IDs
const (
	grpWorkspace = "workspace"
	grpContainer = "container"
	grpImages    = "images"
	grpNix       = "nix"
	grpConfig    = "cconfig"
	grpNet       = "cnet"
	grpDevices   = "devices"
	grpSecurity  = "security"
	grpRemote    = "remote"
	grpSystem    = "system"
)

// usbCmd is a cross-platform entry point that dispatches to the macOS or Windows
// USB backend for the current host (on Linux, USB is passed at container
// creation, so it just points there).
var usbCmd = &cobra.Command{
	Use:   "usb",
	Short: "Attach/detach USB devices to a container (macOS/Windows)",
	Long: `Attach and detach USB devices for RF Swift containers. This is the
cross-platform front door: on macOS it drives the Lima VM USB passthrough, on
Windows it drives usbipd. (On Linux, pass devices at container creation with
'run --usb'/device flags; there is no separate attach step.)`,
}

// configCmd gathers the container runtime-configuration groups.
var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Configure container devices, privileges, ports and resources",
	Aliases: []string{"cfg"},
	Long: `Adjust a container's runtime configuration. Groups: bindings, capabilities,
cgroups, gpus, ports, ulimits. Each is also available at the top level (e.g.
'rfswift ports ...') for backward compatibility.`,
}

// systemCmd gathers maintenance/diagnostic commands.
var systemCmd = &cobra.Command{
	Use:   "system",
	Short: "Maintain and diagnose the RF Swift host",
	Long: `Housekeeping for RF Swift itself. Each subcommand is also available at the
top level (e.g. 'rfswift doctor') for backward compatibility.`,
}

var containerCmd = &cobra.Command{Use: "container", Short: "Create and manage RF Swift containers"}

func setGroup(id string, cmds ...*cobra.Command) {
	for _, c := range cmds {
		if c != nil {
			c.GroupID = id
		}
	}
}

// organizeCommands wires help groups and the convenience parents. It is
// defensive (nil-checks) so a command that is only registered on some platforms
// does not break the others.
func organizeCommands() {
	groups := []*cobra.Group{
		{ID: grpContainer, Title: "Containers:"},
		{ID: grpImages, Title: "Images and portability:"},
		{ID: grpNix, Title: "Native Nix environments:"},
		{ID: grpConfig, Title: "Runtime configuration:"},
		{ID: grpNet, Title: "Networking:"},
		{ID: grpDevices, Title: "Devices (USB / host):"},
		{ID: grpSecurity, Title: "Security:"},
		{ID: grpRemote, Title: "Remote access:"},
		{ID: grpSystem, Title: "System & maintenance:"},
	}
	rootCmd.AddGroup(groups...)

	// Resource-first commands are the canonical interface. The implementation
	// commands are cloned so legacy paths can keep their exact flags and behavior
	// while emitting a deprecation notice.
	containerCmd.AddCommand(
		cloneCommand(runCmd, "create"), cloneCommand(execCmd, "shell"), cloneCommand(lastCmd, "last"),
		cloneCommand(installCmd, "install"), cloneCommand(stopCmd, "stop"), cloneCommand(removeCmd, "rm"),
		cloneCommand(renameCmd, "rename"), cloneCommand(commitCmd, "commit"), cloneCommand(upgradeCmd, "upgrade"),
	)
	imageCmd := cloneCommand(ImagesCmd, "image")
	imageCmd.Short = "Manage container images"
	imageCmd.AddCommand(cloneCommand(buildCmd, "build"), cloneCommand(DeleteCmd, "rm"), cloneCommand(retagCmd, "tag"), cloneCommand(DownloadCmd, "download"), cloneCommand(ExportCmd, "export"), cloneCommand(ImportCmd, "import"))
	envCmd := cloneCommand(nixCmd, "env")
	envCmd.Short = "Create and manage native Nix environments"
	setGroup(grpContainer, containerCmd)
	setGroup(grpImages, imageCmd)
	setGroup(grpNix, envCmd)
	rootCmd.AddCommand(containerCmd, imageCmd, envCmd)

	deprecate(runCmd, "rfswift container create (containers) or rfswift create --engine nix", execCmd, "rfswift container shell (containers) or rfswift env shell (Nix)", lastCmd, "rfswift container last", installCmd, "rfswift container install", stopCmd, "rfswift container stop", removeCmd, "rfswift container rm", renameCmd, "rfswift container rename", commitCmd, "rfswift container commit", upgradeCmd, "rfswift container upgrade", ImagesCmd, "rfswift image", buildCmd, "rfswift image build", DeleteCmd, "rfswift image rm", retagCmd, "rfswift image tag", DownloadCmd, "rfswift image download", ExportCmd, "rfswift image export", ImportCmd, "rfswift image import", nixCmd, "rfswift env")
	deprecateDescendants(ImagesCmd, "rfswift image")
	deprecateDescendants(nixCmd, "rfswift env")
	setGroup(grpContainer, runCmd, execCmd, lastCmd, installCmd)
	setGroup(grpContainer, stopCmd, removeCmd, renameCmd, commitCmd)
	setGroup(grpImages, ImagesCmd, buildCmd, DeleteCmd, retagCmd, DownloadCmd, ExportCmd, ImportCmd, upgradeCmd)
	setGroup(grpNix, nixCmd)
	setGroup(grpConfig, BindingsCmd, CapabilitiesCmd, CgroupsCmd, GPUsCmd, PortsCmd, UlimitsCmd, configCmd)
	setGroup(grpNet, networkCmd)
	setGroup(grpDevices, HostCmd, macusbCmd, winusbCmd, usbCmd)
	setGroup(grpSecurity, auditCmd, reportCmd)
	setGroup(grpRemote, agentCmd)
	setGroup(grpSystem, doctorCmd, CleanupCmd, UpdateCmd, LogCmd, engineCmd, profileCmd, RealtimeCmd, systemCmd, completionCmd)

	// Predictable vocabulary for new users. Existing command names remain the
	// canonical paths, so scripts and generated completions remain compatible.
	runCmd.Aliases = appendUnique(runCmd.Aliases, "create", "new")
	execCmd.Aliases = appendUnique(execCmd.Aliases, "shell", "enter")
	removeCmd.Aliases = appendUnique(removeCmd.Aliases, "rm")
	stopCmd.Aliases = appendUnique(stopCmd.Aliases, "halt")
	agentCmd.Aliases = appendUnique(agentCmd.Aliases, "remote")
	systemCmd.Aliases = appendUnique(systemCmd.Aliases, "admin")

	runCmd.Example = "  rfswift create -n lab -i rfid\n  rfswift create --engine nix -n radio -i sdr_light"
	execCmd.Example = "  rfswift shell -c lab -e /bin/zsh\n  rfswift --engine nix enter -c radio"
	agentCmd.Example = "  rfswift agent certs init --dir ./agent-certs --host 127.0.0.1\n  rfswift agent --bind 127.0.0.1:8443 --cert agent-certs/server.pem --key agent-certs/server-key.pem --client-ca agent-certs/ca.pem"

	// New convenience parents. Re-register the SAME command objects under them
	// (cobra resolves subcommands via each parent's list, so the original paths
	// keep working). Give each parent the matching group so cobra's group check
	// is satisfied at both parents.
	configCmd.AddGroup(&cobra.Group{ID: grpConfig, Title: "Container configuration:"})
	configCmd.AddCommand(BindingsCmd, CapabilitiesCmd, CgroupsCmd, GPUsCmd, PortsCmd, UlimitsCmd)
	rootCmd.AddCommand(configCmd)

	systemCmd.AddGroup(&cobra.Group{ID: grpSystem, Title: "System & maintenance:"})
	systemCmd.AddGroup(&cobra.Group{ID: grpImages, Title: "Images and portability:"})
	systemCmd.AddGroup(&cobra.Group{ID: grpSecurity, Title: "Security:"})
	systemCmd.AddCommand(doctorCmd, CleanupCmd, UpdateCmd, upgradeCmd, LogCmd, reportCmd)
	rootCmd.AddCommand(systemCmd)

	// Cross-platform usb parent: attach the current OS's backend subcommands.
	switch runtime.GOOS {
	case "darwin":
		usbCmd.AddCommand(macusbListCmd, macusbAttachCmd, macusbDetachCmd, macusbStatusCmd, macusbVMDevicesCmd)
	case "windows":
		usbCmd.AddCommand(winusblistCmd, winusbattachCmd, winusbdetachCmd)
	default:
		usbCmd.Run = func(cmd *cobra.Command, args []string) {
			common.PrintInfoMessage("On Linux, USB devices are passed when the container is created (see the device flags on 'rfswift run'); there is no separate attach step.")
		}
	}
	rootCmd.AddCommand(usbCmd)
}

// cloneCommand shares the tested command handler and flag values but owns a
// distinct Cobra node/path. RF Swift runs one command per process, so sharing
// pflag values cannot create cross-invocation state.
func cloneCommand(source *cobra.Command, use string) *cobra.Command {
	clone := &cobra.Command{Use: use, Aliases: append([]string(nil), source.Aliases...), SuggestFor: append([]string(nil), source.SuggestFor...), Short: source.Short, Long: source.Long, Example: source.Example, Args: source.Args, ArgAliases: append([]string(nil), source.ArgAliases...), ValidArgs: append([]string(nil), source.ValidArgs...), ValidArgsFunction: source.ValidArgsFunction, Run: source.Run, RunE: source.RunE, PreRun: source.PreRun, PreRunE: source.PreRunE, PostRun: source.PostRun, PostRunE: source.PostRunE, SilenceErrors: source.SilenceErrors, SilenceUsage: source.SilenceUsage, DisableFlagParsing: source.DisableFlagParsing, DisableAutoGenTag: source.DisableAutoGenTag}
	clone.Flags().AddFlagSet(source.LocalNonPersistentFlags())
	clone.PersistentFlags().AddFlagSet(source.PersistentFlags())
	for _, child := range source.Commands() {
		clone.AddCommand(cloneCommand(child, child.Use))
	}
	return clone
}

func deprecate(values ...any) {
	for i := 0; i+1 < len(values); i += 2 {
		command, ok := values[i].(*cobra.Command)
		replacement, rok := values[i+1].(string)
		if ok && rok {
			command.Deprecated = "use '" + replacement + "' instead"
		}
	}
}

func deprecateDescendants(parent *cobra.Command, replacement string) {
	for _, child := range parent.Commands() {
		name := child.Name()
		child.Deprecated = "use '" + replacement + " " + name + "' instead"
		deprecateDescendants(child, replacement+" "+name)
	}
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}
