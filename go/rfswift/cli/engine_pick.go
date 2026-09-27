/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
	"penthertz/rfswift/rfutils"
	"penthertz/rfswift/tui"
)

// Engine choice in the run wizard. When several engines are installed and
// none was chosen (--engine, --gpu, RFSWIFT_ENGINE or [general] engine in the
// config), `rfswift run` asks which one to use before the rest of the wizard,
// and can store the answer as the default. `rfswift engine set` sets or
// clears that default directly.

// defaultEngineValues are the values `[general] engine` accepts.
var defaultEngineValues = []string{"auto", "docker", "podman", "lima", "nix"}

// installedEngines lists the engines usable on this host, in the order the
// auto-detection tries them. Nix is left out on Windows: it runs in the WSL
// backend, which the wizard cannot switch to mid-command (--engine nix does).
func installedEngines() []tui.EngineChoice {
	var out []tui.EngineChoice
	if (&rfdock.DockerEngine{}).IsAvailable() {
		out = append(out, tui.EngineChoice{Value: "docker", Label: "Docker"})
	}
	if (&rfdock.PodmanEngine{}).IsAvailable() {
		out = append(out, tui.EngineChoice{Value: "podman", Label: "Podman"})
	}
	if rfdock.IsLimaEngineCandidate() && (&rfdock.LimaEngine{}).IsAvailable() {
		out = append(out, tui.EngineChoice{Value: "lima", Label: "Docker in the Lima VM (USB passthrough)"})
	}
	if runtime.GOOS != "windows" && rfnix.IsAvailable() {
		out = append(out, tui.EngineChoice{Value: "nix", Label: "Nix (native tools, no container)"})
	}
	return out
}

// engineChosen reports whether the engine was already decided for this run.
func engineChosen(cmd *cobra.Command) bool {
	if f := cmd.Flags().Lookup("engine"); f != nil && f.Changed {
		return true
	}
	if gpu, _ := cmd.Flags().GetBool("gpu"); gpu {
		return true
	}
	if strings.TrimSpace(os.Getenv("RFSWIFT_ENGINE")) != "" {
		return true
	}
	e := rfutils.ConfiguredEngine(common.ConfigFileByPlatform())
	return e != "" && e != "auto"
}

// pickRunEngine runs the wizard's engine step when it applies and switches
// to the chosen engine; picking Nix selects the environment path, which the
// caller then takes (rfnix.IsSelected).
func pickRunEngine(cmd *cobra.Command) error {
	if engineChosen(cmd) || !tui.IsInteractive() {
		return nil
	}
	choices := installedEngines()
	if len(choices) < 2 {
		return nil
	}
	current := choices[0].Value
	if eng := rfdock.GetEngine(); eng != nil {
		current = string(eng.Type())
	}
	choice, remember, err := tui.PickEngine(choices, current)
	if err != nil {
		return err
	}
	if remember {
		if err := rfutils.SetConfigValue(common.ConfigFileByPlatform(), "general", "engine", choice); err != nil {
			common.PrintWarningMessage(fmt.Sprintf("Could not save %s as the default engine: %v", choice, err))
		} else {
			common.PrintSuccessMessage(fmt.Sprintf("Default engine set to %s ('rfswift engine set auto' asks again).", choice))
		}
	}
	if choice == "nix" {
		rfnix.SetSelected(true)
		return nil
	}
	if choice != current {
		rfdock.SetPreferredEngine(choice)
		rfdock.GetEngine()
	}
	return nil
}

var engineSetDefaultCmd = &cobra.Command{
	Use:       "set <auto|docker|podman|lima|nix>",
	Short:     "Set the default engine ('auto' asks in the run wizard when several are installed)",
	Args:      cobra.ExactArgs(1),
	ValidArgs: defaultEngineValues,
	Run: func(cmd *cobra.Command, args []string) {
		value := strings.ToLower(strings.TrimSpace(args[0]))
		valid := false
		for _, v := range defaultEngineValues {
			if v == value {
				valid = true
			}
		}
		if !valid {
			common.PrintErrorMessage(fmt.Errorf("unknown engine %q: use one of %s", args[0], strings.Join(defaultEngineValues, ", ")))
			os.Exit(1)
		}
		if err := rfutils.SetConfigValue(common.ConfigFileByPlatform(), "general", "engine", value); err != nil {
			common.PrintErrorMessage(err)
			os.Exit(1)
		}
		if value == "auto" {
			common.PrintSuccessMessage("Default engine cleared: 'rfswift run' asks which engine to use when several are installed.")
			return
		}
		common.PrintSuccessMessage(fmt.Sprintf("Default engine set to %s (--engine still overrides it).", value))
	},
}
