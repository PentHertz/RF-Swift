/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package tui

import (
	"fmt"

	huh "charm.land/huh/v2"
)

// EngineChoice is one installed engine offered by PickEngine.
type EngineChoice struct {
	Value string // docker, podman, lima or nix
	Label string
}

// PickEngine is the first step of the run wizard when several engines are
// installed and none was chosen: it asks which engine to use, with current
// (the auto-detected one) preselected, then whether to keep it as the default.
//
//	in(1): []EngineChoice choices installed engines
//	in(2): string current value preselected in the list
//	out: string chosen engine value
//	out: bool true when the choice should be stored as the default
//	out: error non-nil when the form was cancelled
func PickEngine(choices []EngineChoice, current string) (string, bool, error) {
	if !IsInteractive() {
		return current, false, fmt.Errorf("interactive terminal required for wizard mode")
	}
	opts := make([]huh.Option[string], 0, len(choices))
	for _, c := range choices {
		opts = append(opts, huh.NewOption(c.Label, c.Value))
	}
	engine := current
	err := huh.NewSelect[string]().
		Title("Which engine?").
		Description("Several engines are installed. --engine or 'rfswift engine set <engine>' skips this question.").
		Options(opts...).
		Value(&engine).
		Run()
	if err != nil {
		return "", false, err
	}
	label := engine
	for _, c := range choices {
		if c.Value == engine {
			label = c.Label
		}
	}
	remember := false
	err = huh.NewConfirm().
		Title(fmt.Sprintf("Use %s by default from now on?", label)).
		Description("'rfswift engine set auto' brings this question back.").
		Affirmative("Yes").
		Negative("No").
		Value(&remember).
		Run()
	if err != nil {
		return "", false, err
	}
	return engine, remember, nil
}
