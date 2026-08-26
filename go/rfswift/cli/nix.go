/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine CLI: the `run --engine nix` / `exec --engine nix` handlers and the
*  `rfswift nix` command group (catalog, list, info, remove).
 */

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfnix "penthertz/rfswift/nix"
	"penthertz/rfswift/tui"
)

// nixWizardResult holds the choices made in the interactive nix wizard.
type nixWizardResult struct {
	image     string
	name      string
	command   string
	workspace string // "", "none", "cwd", or a path
	pure      bool
	lazy      bool
	confirmed bool
}

// parseSize parses the binary size notation accepted by the Nix GC command.
func parseSize(value string) (int64, error) {
	s := strings.ToUpper(strings.TrimSpace(value))
	if s == "" {
		return 0, nil
	}
	multiplier := int64(1)
	for _, suffix := range []struct {
		text string
		mult int64
	}{
		{"TIB", 1 << 40}, {"TB", 1 << 40}, {"T", 1 << 40},
		{"GIB", 1 << 30}, {"GB", 1 << 30}, {"G", 1 << 30},
		{"MIB", 1 << 20}, {"MB", 1 << 20}, {"M", 1 << 20},
		{"KIB", 1 << 10}, {"KB", 1 << 10}, {"K", 1 << 10},
	} {
		if strings.HasSuffix(s, suffix.text) {
			multiplier = suffix.mult
			s = strings.TrimSuffix(s, suffix.text)
			break
		}
	}
	if s == "" || strings.ContainsAny(s, ".+- ") {
		return 0, fmt.Errorf("invalid size %q", value)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || (multiplier != 0 && n > (1<<63-1)/multiplier) {
		return 0, fmt.Errorf("invalid size %q", value)
	}
	return n * multiplier, nil
}

// runNixEnvironment handles `rfswift run --engine nix`.
func runNixEnvironment(cmd *cobra.Command) error {
	image, _ := cmd.Flags().GetString("image")
	name, _ := cmd.Flags().GetString("name")
	command, _ := cmd.Flags().GetString("command")
	workspacePath, _ := cmd.Flags().GetString("workspace")
	noWorkspace, _ := cmd.Flags().GetBool("no-workspace")
	cwdWorkspace, _ := cmd.Flags().GetBool("cwd")
	pure, _ := cmd.Flags().GetBool("pure")
	rebuild, _ := cmd.Flags().GetBool("rebuild")
	lazy, _ := cmd.Flags().GetBool("lazy")
	flakeRef, _ := cmd.Flags().GetString("flake")

	// Resolve the workspace selection into the value RunEnvironment expects.
	workspace := ""
	switch {
	case noWorkspace:
		workspace = "none"
	case cwdWorkspace:
		if cwd, err := os.Getwd(); err == nil {
			workspace = cwd
		}
	case workspacePath != "":
		workspace = workspacePath
	}

	cat, err := rfnix.LoadCatalog()
	if err != nil {
		return err
	}

	// Wizard when name or image is missing and we have a terminal.
	if (name == "" || image == "") && tui.IsInteractive() {
		res, err := nixWizard(cat, image, name)
		if err != nil {
			return fmt.Errorf("wizard cancelled: %v", err)
		}
		if !res.confirmed {
			common.PrintInfoMessage("Environment creation cancelled.")
			return nil
		}
		image = res.image
		name = res.name
		if res.command != "" {
			command = res.command
		}
		switch res.workspace {
		case "none":
			workspace = "none"
		case "cwd":
			if cwd, err := os.Getwd(); err == nil {
				workspace = cwd
			}
		case "":
			// keep whatever the flags resolved
		default:
			workspace = res.workspace
		}
		pure = pure || res.pure
		lazy = lazy || res.lazy
	} else {
		if name == "" {
			return fmt.Errorf("environment name is required (use -n)")
		}
		if image == "" {
			return fmt.Errorf("environment image is required (use -i, e.g. -i sdr_light). See: rfswift nix catalog")
		}
	}

	return rfnix.RunEnvironment(rfnix.RunOptions{
		Name:      name,
		Image:     image,
		Command:   command,
		Workspace: workspace,
		FlakeRef:  flakeRef,
		Rebuild:   rebuild,
		Pure:      pure,
		Lazy:      lazy,
	})
}

// execNixEnvironment handles `rfswift exec --engine nix`.
func execNixEnvironment(cmd *cobra.Command) error {
	name, _ := cmd.Flags().GetString("container")

	// The exec `-e` flag defaults to /bin/bash for containers; for nix an
	// unchanged flag means "interactive shell".
	command := ""
	if cmd.Flags().Changed("command") {
		command, _ = cmd.Flags().GetString("command")
	}

	return enterNixEnvironment(name, command)
}

func enterNixEnvironment(name, command string) error {
	if name == "" {
		envs, _ := rfnix.ListEnvironments()
		if len(envs) == 0 {
			return fmt.Errorf("no RF Swift nix environments found. Create one with: rfswift run --engine nix")
		}
		if tui.IsInteractive() {
			options := make([]string, len(envs))
			for i, e := range envs {
				label := fmt.Sprintf("%s  (%s)", e.Name, e.Image)
				if i == 0 {
					label += "  ← latest"
				}
				options[i] = label
			}
			selected, err := tui.SelectOne("Select an environment", options)
			if err != nil {
				return fmt.Errorf("selection cancelled")
			}
			for i, o := range options {
				if o == selected {
					name = envs[i].Name
					break
				}
			}
		} else {
			name = envs[0].Name
			common.PrintInfoMessage(fmt.Sprintf("Using latest environment: %s", name))
		}
	}
	env, err := rfnix.GetEnvironment(name)
	if err != nil {
		return err
	}
	renderNixSummary(env)
	warnInaccessibleSerialDevices()
	return rfnix.ExecEnvironment(name, command)
}

func renderNixSummary(env *rfnix.Environment) {
	mode := "eager (all up front)"
	tools := len(env.Packages)
	if env.Lazy {
		mode = "on-demand (build on first call)"
		if len(env.Commands) > 0 {
			tools = len(env.Commands)
		}
	}
	items := []tui.PropertyItem{
		{Key: "Environment", Value: env.Image, ValueColor: tui.ColorWarning},
		{Key: "Name", Value: env.Name, ValueColor: tui.ColorPrimary},
		{Key: "Tools", Value: fmt.Sprintf("%d", tools)},
		{Key: "Mode", Value: mode},
		{Key: "Workspace", Value: wsShort(env.Workspace)},
		{Key: "Flake", Value: env.FlakeRef, ValueColor: tui.ColorCyan},
		{Key: "Engine", Value: "nix (native host user)"},
	}
	tui.RenderPropertySheet("🧪 Nix Environment Summary", tui.ColorPrimary, items)
}

func warnInaccessibleSerialDevices() {
	paths, _ := filepath.Glob("/dev/ttyACM*")
	serial, _ := filepath.Glob("/dev/ttyUSB*")
	paths = append(paths, serial...)
	for _, path := range paths {
		accessible, group := serialDeviceAccess(path)
		if accessible {
			continue
		}
		common.PrintWarningMessage(fmt.Sprintf("Device access: %s needs group %s. Refresh with `newgrp %s` for this terminal, or fully log out/in before starting RF Swift or Workbench.", path, group, group))
	}
}

// nixWizard runs a short interactive flow to choose an environment to create.
func nixWizard(cat *rfnix.Catalog, image, name string) (*nixWizardResult, error) {
	res := &nixWizardResult{image: image, name: name}

	if res.image == "" {
		options := make([]string, len(cat.Environments))
		for i, e := range cat.Environments {
			options[i] = fmt.Sprintf("%s  -  %s", e.Name, e.Description)
		}
		selected, err := tui.SelectOneFilterable("Choose an environment", options)
		if err != nil {
			return nil, err
		}
		for i, o := range options {
			if o == selected {
				res.image = cat.Environments[i].Name
				break
			}
		}
	}

	if res.name == "" {
		suggested := res.image
		n, err := tui.PromptInput("Environment name", suggested)
		if err != nil {
			return nil, err
		}
		if n == "" {
			n = suggested
		}
		res.name = n
	}

	// Workspace choice.
	wsChoice, err := tui.SelectOne("Workspace (shared working directory)", []string{
		"auto  (~/rfswift-workspace/<name>)",
		"cwd  (current directory)",
		"none  (no workspace)",
	})
	if err == nil {
		switch {
		case wsChoice == "cwd  (current directory)":
			res.workspace = "cwd"
		case wsChoice == "none  (no workspace)":
			res.workspace = "none"
		default:
			res.workspace = ""
		}
	}

	// Build mode: everything up front, or each tool on first call.
	modeChoice, err := tui.SelectOne("Build mode", []string{
		"eager  (build all tools now, ready offline)",
		"on-demand  (build each tool the first time you call it)",
	})
	if err == nil && modeChoice == "on-demand  (build each tool the first time you call it)" {
		res.lazy = true
	}

	// Recap and confirm.
	entry := cat.Find(res.image)
	toolCount := 0
	if entry != nil {
		toolCount = len(entry.Packages)
	}
	mode := "eager (all up front)"
	if res.lazy {
		mode = "on-demand (build on first call)"
	}
	items := map[string]string{
		"Environment": res.image,
		"Name":        res.name,
		"Tools":       fmt.Sprintf("~%d", toolCount),
		"Mode":        mode,
		"Workspace":   wsLabel(res.name, res.workspace),
		"Engine":      "nix (native)",
	}
	tui.PrintRecap("Nix Environment", items, []string{"Environment", "Name", "Tools", "Mode", "Workspace", "Engine"})
	tui.PrintCLIEquivalent(buildNixCLI(res))

	res.confirmed = tui.Confirm("Create this environment?")
	return res, nil
}

func wsLabel(name, ws string) string {
	switch ws {
	case "", "auto":
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "auto (~/rfswift-workspace/" + name + ")"
		}
		return "auto (" + filepath.Join(home, "rfswift-workspace", name) + ")"
	case "cwd":
		if cwd, err := os.Getwd(); err == nil {
			return "current directory (" + cwd + ")"
		}
		return "current directory"
	case "none":
		return "disabled (no workspace directory)"
	default:
		if absolute, err := filepath.Abs(ws); err == nil {
			return "custom (" + absolute + ")"
		}
		return "custom (" + ws + ")"
	}
}

// nixInstallWizard collects a package and installation scope for the guided
// installer shared by `rfswift install --engine nix` and the Workbench-facing
// command tree. The returned target is empty for the shared extras profile.
func nixInstallWizard(initialTarget, initialQuery string) ([]string, string, error) {
	target := strings.TrimSpace(initialTarget)
	if target == "" {
		envs, err := rfnix.ListEnvironments()
		if err != nil {
			return nil, "", err
		}
		options := []string{"shared  (available in every Nix environment)"}
		for _, env := range envs {
			options = append(options, fmt.Sprintf("%s  (%s only)", env.Name, env.Image))
		}
		selected, err := tui.SelectOne("Install scope", options)
		if err != nil {
			return nil, "", err
		}
		if selected != options[0] {
			for _, env := range envs {
				if strings.HasPrefix(selected, env.Name+"  (") {
					target = env.Name
					break
				}
			}
		}
	}

	query := strings.TrimSpace(initialQuery)
	if query == "" {
		var err error
		query, err = tui.PromptInput("Tool name or search term", "")
		if err != nil {
			return nil, "", err
		}
	}
	hits := rfnix.SearchPackages(query)
	if len(hits) == 0 {
		// A package outside the curated RF Swift catalog is still valid: the
		// installer resolves it through the flake's pinned nixpkgs set.
		if query == "" {
			return nil, "", fmt.Errorf("a package name is required")
		}
		return []string{query}, target, nil
	}
	options := make([]string, len(hits))
	for i, hit := range hits {
		options[i] = fmt.Sprintf("%s  (%s)", hit.Name, strings.Join(hit.Envs, ", "))
	}
	selected, err := tui.SelectOne("Choose a tool", options)
	if err != nil {
		return nil, "", err
	}
	for i, option := range options {
		if selected == option {
			return []string{hits[i].Name}, target, nil
		}
	}
	return nil, "", fmt.Errorf("no package selected")
}

func buildNixCLI(res *nixWizardResult) string {
	s := fmt.Sprintf("rfswift run --engine nix -i %s -n %s", res.image, res.name)
	switch res.workspace {
	case "cwd":
		s += " --cwd"
	case "none":
		s += " --no-workspace"
	}
	if res.pure {
		s += " --pure"
	}
	if res.lazy {
		s += " --lazy"
	}
	return s
}

// ---------------------------------------------------------------------------
// `rfswift nix` command group
// ---------------------------------------------------------------------------

var nixCmd = &cobra.Command{
	Use:   "nix",
	Short: "Manage RF Swift Nix environments",
	Long: `Create and manage RF Swift tool sets as native Nix environments.

Run one with:  rfswift run --engine nix -i sdr_light -n mysdr
Re-enter it:   rfswift exec --engine nix -c mysdr

The subcommands here browse the catalog and manage created environments.`,
}

var nixCatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "List available Nix environments (images)",
	Run: func(cmd *cobra.Command, args []string) {
		cat, err := rfnix.LoadCatalog()
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		rows := make([][]string, 0, len(cat.Environments))
		for _, e := range cat.Environments {
			rows = append(rows, []string{
				e.Name,
				e.Category,
				fmt.Sprintf("%d", len(e.Packages)),
				truncate(e.Description, 60),
			})
		}
		tui.RenderTable(tui.TableConfig{
			Title:      "📦 RF Swift Nix environments",
			TitleColor: tui.ColorPrimary,
			Headers:    []string{"Name", "Category", "Tools", "Description"},
			Rows:       rows,
			BorderRow:  false,
		})
		fmt.Println()
		common.PrintInfoMessage("Create one with: rfswift run --engine nix -i <name> -n <env-name>")
	},
}

var nixListCmd = &cobra.Command{
	Use:   "list",
	Short: "List created Nix environments",
	Run: func(cmd *cobra.Command, args []string) {
		envs, err := rfnix.ListEnvironments()
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		if len(envs) == 0 {
			common.PrintInfoMessage("No Nix environments yet. Create one with: rfswift run --engine nix")
			return
		}
		rows := make([][]string, 0, len(envs))
		for _, e := range envs {
			rows = append(rows, []string{
				e.Name,
				e.Image,
				e.Created.Format("2006-01-02 15:04"),
				realisedLabel(e),
				wsShort(e.Workspace),
			})
		}
		tui.RenderTable(tui.TableConfig{
			Title:      "🧪 RF Swift Nix environments (local)",
			TitleColor: tui.ColorPrimary,
			Headers:    []string{"Name", "Image", "Created", "State", "Workspace"},
			Rows:       rows,
			BorderRow:  false,
		})
	},
}

var nixInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show details for a Nix environment",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		if name == "" {
			common.PrintErrorMessage(fmt.Errorf("environment name required: rfswift nix info <name>"))
			return
		}
		env, err := rfnix.GetEnvironment(name)
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		items := []tui.PropertyItem{
			{Key: "Name", Value: env.Name, ValueColor: tui.ColorPrimary},
			{Key: "Image", Value: env.Image, ValueColor: tui.ColorWarning},
			{Key: "Created", Value: env.Created.Format("2006-01-02 15:04:05")},
			{Key: "Flake", Value: env.FlakeRef, ValueColor: tui.ColorCyan},
			{Key: "State", Value: realisedLabel(env)},
			{Key: "Workspace", Value: wsShort(env.Workspace)},
			{Key: "Tools", Value: fmt.Sprintf("%d packages", len(env.Packages))},
		}
		tui.RenderPropertySheet("🧪 Nix Environment", tui.ColorPrimary, items)
		if len(env.Packages) > 0 {
			fmt.Println()
			common.PrintInfoMessage("Packages:")
			for _, p := range env.Packages {
				fmt.Printf("  - %s\n", p)
			}
		}
	},
}

var nixRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a Nix environment",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		if name == "" && tui.IsInteractive() {
			envs, _ := rfnix.ListEnvironments()
			if len(envs) == 0 {
				common.PrintInfoMessage("No Nix environments to remove.")
				return
			}
			options := make([]string, len(envs))
			for i, e := range envs {
				options[i] = fmt.Sprintf("%s  (%s)", e.Name, e.Image)
			}
			selected, err := tui.SelectOne("Select an environment to remove", options)
			if err != nil {
				return
			}
			for i, o := range options {
				if o == selected {
					name = envs[i].Name
					break
				}
			}
			if !tui.Confirm(fmt.Sprintf("Remove environment '%s'?", name)) {
				common.PrintInfoMessage("Removal cancelled.")
				return
			}
		}
		if name == "" {
			common.PrintErrorMessage(fmt.Errorf("environment name required: rfswift nix remove <name>"))
			return
		}
		if err := rfnix.RemoveEnvironment(name); err != nil {
			common.PrintErrorMessage(err)
		}
	},
}

func realisedLabel(e *rfnix.Environment) string {
	if e.Lazy {
		return "on-demand"
	}
	if e.ProfilePath == "" {
		return "pure"
	}
	if e.Realised() {
		return "realised"
	}
	return "not built"
}

func wsShort(ws string) string {
	if ws == "" {
		return "none"
	}
	return ws
}

// resolveFlakeForTarget returns the flake reference to use for a `nix run`
// target: a created environment's pinned flake if the target names one,
// otherwise the resolved default.
func resolveFlakeForTarget(target string) string {
	if env, err := rfnix.GetEnvironment(target); err == nil && env.FlakeRef != "" {
		return env.FlakeRef
	}
	return rfnix.ResolveFlakeRef("")
}

var nixInstallCmd = &cobra.Command{
	Use:   "install [package...]",
	Short: "Install extra tools into a Nix environment or the shared profile",
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		target, _ := cmd.Flags().GetString("env")
		flake, _ := cmd.Flags().GetString("flake")
		packages := args
		if len(packages) == 0 && tui.IsInteractive() {
			var err error
			packages, target, err = nixInstallWizard(target, "")
			if err != nil {
				common.PrintErrorMessage(err)
				return
			}
		}
		if len(packages) == 0 {
			common.PrintErrorMessage(fmt.Errorf("package name required"))
			return
		}
		if flake == "" {
			flake = resolveFlakeForTarget(target)
		}
		if err := rfnix.InstallPackages(flake, packages, target); err != nil {
			common.PrintErrorMessage(err)
		}
	},
}

var nixAuditCmd = &cobra.Command{
	Use:   "audit [environment]",
	Short: "Audit a Nix tool environment for security warnings",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("env")
		if len(args) == 1 {
			name = args[0]
		}
		format, _ := cmd.Flags().GetString("format")
		failOn, _ := cmd.Flags().GetString("fail-on")
		out, _ := cmd.Flags().GetString("out")
		auditNixEnv(name, format, failOn, out)
	},
}

var nixGCCmd = &cobra.Command{
	Use:   "gc",
	Short: "Reclaim unreferenced Nix store paths",
	Run: func(cmd *cobra.Command, _ []string) {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		maxText, _ := cmd.Flags().GetString("max-free")
		maxFree, err := parseSize(maxText)
		if err != nil {
			common.PrintErrorMessage(err)
			return
		}
		if err := rfnix.GarbageCollect(rfnix.GCOptions{DryRun: dryRun, MaxFree: maxFree}); err != nil {
			common.PrintErrorMessage(err)
		}
	},
}

var nixUpdateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Check for updates, update the flake lock, and safely rebuild an environment",
	Long: `Update a named eager environment. RF Swift first updates the writable local
flake.lock (or refreshes a remote flake), builds a candidate closure, and only
then switches the active profile. The previous closure remains GC-rooted and
can be restored with 'rfswift env rollback <name>'.

Use --check to preview lock changes without writing or building. Use --input to
update only one local flake input, for example nixpkgs.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		check, _ := cmd.Flags().GetBool("check")
		input, _ := cmd.Flags().GetString("input")
		yes, _ := cmd.Flags().GetBool("yes")
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		if name == "" {
			if !tui.IsInteractive() {
				return fmt.Errorf("environment name required in non-interactive mode")
			}
			var confirmed bool
			var err error
			name, check, input, confirmed, err = nixUpdateWizard(cmd, check, input)
			if err != nil {
				return err
			}
			if !confirmed {
				common.PrintInfoMessage("Update cancelled.")
				return nil
			}
			yes = true // the wizard already showed the recap and confirmed it
		}
		if check {
			return rfnix.CheckEnvironmentUpdate(name, input)
		}
		if !yes {
			if !tui.IsInteractive() {
				return fmt.Errorf("update requires confirmation; rerun with --yes")
			}
			label := "all flake inputs"
			if input != "" {
				label = "flake input " + input
			}
			if !tui.Confirm(fmt.Sprintf("Update %s and rebuild environment '%s'?", label, name)) {
				common.PrintInfoMessage("Update cancelled.")
				return nil
			}
		}
		return rfnix.UpdateEnvironment(name, rfnix.UpdateOptions{Input: input})
	},
}

func nixUpdateWizard(cmd *cobra.Command, checkFlag bool, inputFlag string) (string, bool, string, bool, error) {
	envs, err := rfnix.ListEnvironments()
	if err != nil {
		return "", false, "", false, err
	}
	eligible := make([]*rfnix.Environment, 0, len(envs))
	options := make([]string, 0, len(envs))
	for _, env := range envs {
		if env.Lazy || env.ProfilePath == "" || !env.Realised() {
			continue
		}
		eligible = append(eligible, env)
		options = append(options, fmt.Sprintf("%s  (%s · %d tools)", env.Name, env.Image, len(env.Packages)))
	}
	if len(eligible) == 0 {
		return "", false, "", false, fmt.Errorf("no realised eager environments are available; create one without --lazy or --pure")
	}
	selected, err := tui.SelectOneFilterable("Select an environment to update", options)
	if err != nil {
		return "", false, "", false, fmt.Errorf("environment selection cancelled")
	}
	var env *rfnix.Environment
	for i, option := range options {
		if selected == option {
			env = eligible[i]
			break
		}
	}
	if env == nil {
		return "", false, "", false, fmt.Errorf("environment selection failed")
	}
	if inputFlag != "" && !rfnix.EnvironmentUsesLocalFlake(env.Name) {
		return "", false, "", false, fmt.Errorf("--input requires a writable local flake; selected environment uses %s", env.FlakeRef)
	}

	check, input := checkFlag, inputFlag
	action := ""
	if cmd.Flags().Changed("check") {
		action = "Check only (no changes)"
		if input != "" {
			action = "Check input " + input + " only (no changes)"
		}
	} else if cmd.Flags().Changed("input") {
		action = "Update one input and rebuild"
	}
	if action == "" {
		actions := []string{"Check only (no changes)", "Update all inputs and rebuild"}
		if rfnix.EnvironmentUsesLocalFlake(env.Name) {
			actions = append(actions, "Update one input and rebuild")
		}
		action, err = tui.SelectOne("Choose update operation", actions)
		if err != nil {
			return "", false, "", false, fmt.Errorf("update operation selection cancelled")
		}
		check = action == "Check only (no changes)"
	}
	if action == "Update one input and rebuild" && input == "" {
		inputs, inputErr := rfnix.EnvironmentFlakeInputs(env.Name)
		if inputErr != nil {
			return "", false, "", false, inputErr
		}
		if len(inputs) == 0 {
			return "", false, "", false, fmt.Errorf("the flake lock exposes no selectable inputs")
		}
		input, err = tui.SelectOneFilterable("Select the flake input to update", inputs)
		if err != nil {
			return "", false, "", false, fmt.Errorf("flake input selection cancelled")
		}
	}
	if action == "" {
		if check {
			action = "Check only (no changes)"
		} else if input != "" {
			action = "Update input " + input + " and rebuild"
		} else {
			action = "Update all inputs and rebuild"
		}
	}
	gens, _ := rfnix.ListGenerations(env.Name)
	tui.PrintRecap("Nix Environment Update", map[string]string{
		"Environment":     env.Name,
		"Profile":         env.Image,
		"Flake":           env.FlakeRef,
		"Operation":       action,
		"Rollback points": fmt.Sprintf("%d", len(gens)),
	}, []string{"Environment", "Profile", "Flake", "Operation", "Rollback points"})
	if check {
		return env.Name, true, input, true, nil
	}
	return env.Name, false, input, tui.Confirm("Apply this update and rebuild the environment?"), nil
}

var nixRebuildCmd = &cobra.Command{
	Use:          "rebuild <name>",
	Short:        "Rebuild an environment using its currently pinned flake lock",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         func(cmd *cobra.Command, args []string) error { return rfnix.RebuildEnvironment(args[0]) },
}

var nixGenerationsCmd = &cobra.Command{
	Use:          "generations <name>",
	Short:        "List rollback generations for an environment",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		gens, err := rfnix.ListGenerations(args[0])
		if err != nil {
			return err
		}
		if len(gens) == 0 {
			common.PrintInfoMessage("No previous generations yet.")
			return nil
		}
		rows := make([][]string, 0, len(gens))
		for _, g := range gens {
			rows = append(rows, []string{g.Name, g.Created.Local().Format("2006-01-02 15:04:05"), g.StorePath})
		}
		tui.RenderTable(tui.TableConfig{Title: "Nix environment generations", TitleColor: tui.ColorPrimary, Headers: []string{"Generation", "Created", "Store path"}, Rows: rows})
		return nil
	},
}

var nixRollbackCmd = &cobra.Command{
	Use:          "rollback <name> [generation]",
	Short:        "Restore the newest or a selected previous environment generation",
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		generation := ""
		if len(args) == 2 {
			generation = args[1]
		}
		return rfnix.RollbackEnvironment(args[0], generation)
	},
}

var nixRunCmd = &cobra.Command{
	Use:   "run <environment|image> <tool> [-- args...]",
	Short: "Build and run a single tool on demand (nothing else is built)",
	Long: `Build (only if needed) and run one tool from the pinned package set. Only that
tool's closure is realised, so you can bring tools up step by step.

Examples:
  rfswift nix run sdr_light gqrx
  rfswift nix run mysdr inspectrum -- recording.iq`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		flakeOverride, _ := cmd.Flags().GetString("flake")
		target := args[0]
		tool := args[1]
		toolArgs := args[2:]

		flakeRef := flakeOverride
		if flakeRef == "" {
			flakeRef = resolveFlakeForTarget(target)
		}
		if err := rfnix.RunTool(flakeRef, tool, toolArgs); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var nixShellCmd = &cobra.Command{
	Use:     "shell [name]",
	Aliases: []string{"enter"},
	Short:   "Enter a native Nix environment",
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		if err := enterNixEnvironment(name, ""); err != nil {
			common.PrintErrorMessage(err)
		}
	},
}

var nixExportCmd = &cobra.Command{
	Use:   "export <name> [-o file.rfenv]",
	Short: "Export an environment (its closure + workspace) to a compressed archive",
	Long: `Realise an environment and pack its entire Nix closure together with its
workspace into a single compressed .rfenv archive, ready to move to another
machine and import with 'rfswift nix import'.

Example:
  rfswift nix export mysdr -o mysdr.rfenv`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, _ := cmd.Flags().GetString("output")
		if err := rfnix.ExportEnvironment(args[0], out); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

var nixImportCmd = &cobra.Command{
	Use:   "import <file.rfenv>",
	Short: "Import an environment from a .rfenv archive",
	Long: `Import an environment archive created by 'rfswift nix export': its closure is
added to the local Nix store, its workspace is restored, and it is registered
so you can enter it directly.

Examples:
  rfswift nix import mysdr.rfenv
  rfswift nix import mysdr.rfenv --name mysdr2 --workspace ~/work/mysdr2`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		ws, _ := cmd.Flags().GetString("workspace")
		if err := rfnix.ImportEnvironment(args[0], name, ws); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
	},
}

func registerNixCommands() {
	rootCmd.AddCommand(nixCmd)
	nixCmd.AddCommand(nixCatalogCmd)
	nixCmd.AddCommand(nixListCmd)
	nixCmd.AddCommand(nixInfoCmd)
	nixCmd.AddCommand(nixRemoveCmd)
	nixCmd.AddCommand(nixRunCmd)
	nixCmd.AddCommand(nixShellCmd)
	nixCmd.AddCommand(nixExportCmd)
	nixCmd.AddCommand(nixImportCmd)
	nixCmd.AddCommand(nixInstallCmd)
	nixCmd.AddCommand(nixAuditCmd)
	nixCmd.AddCommand(nixGCCmd)
	nixCmd.AddCommand(nixUpdateCmd)
	nixCmd.AddCommand(nixRebuildCmd)
	nixCmd.AddCommand(nixGenerationsCmd)
	nixCmd.AddCommand(nixRollbackCmd)
	nixRunCmd.Flags().String("flake", "", "flake reference override")
	nixInstallCmd.Flags().String("env", "", "environment receiving the package (default: shared profile)")
	nixInstallCmd.Flags().String("flake", "", "flake reference override")
	nixAuditCmd.Flags().String("env", "", "environment to audit")
	nixAuditCmd.Flags().String("format", "stdout,txt,json", "report formats: stdout,txt,json,html,pdf")
	nixAuditCmd.Flags().String("fail-on", "none", "exit non-zero at none|low|medium|high|critical")
	nixAuditCmd.Flags().String("out", "", "report output directory")
	nixGCCmd.Flags().Bool("dry-run", false, "show paths without deleting them")
	nixGCCmd.Flags().String("max-free", "", "stop after freeing this size (for example 5G)")
	nixUpdateCmd.Flags().Bool("check", false, "preview available flake lock updates without changing anything")
	nixUpdateCmd.Flags().String("input", "", "update only this flake input (for example nixpkgs)")
	nixUpdateCmd.Flags().BoolP("yes", "y", false, "confirm the update non-interactively")
	nixExportCmd.Flags().StringP("output", "o", "", "output archive path (default: <name>.rfenv)")
	nixImportCmd.Flags().String("name", "", "name for the imported environment (default: archived name)")
	nixImportCmd.Flags().String("workspace", "", "path to restore the workspace to (default: ~/rfswift-workspace/<name>)")

	// Nix-specific flags on the shared run/exec commands.
	runCmd.Flags().Bool("pure", false, "Nix engine: enter a pure environment (nix develop --ignore-environment)")
	runCmd.Flags().Bool("rebuild", false, "Nix engine: force re-realisation of the environment closure")
	runCmd.Flags().Bool("lazy", false, "Nix engine: build each tool on first call instead of all up front")
	runCmd.Flags().String("flake", "", "Nix engine: flake reference (default: local RF-Swift-nix checkout or github:PentHertz/RF-Swift-nix)")
}
