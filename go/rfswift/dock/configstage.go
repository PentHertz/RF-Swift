/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Steps of a container configuration change (stopping the container,
*  rewriting its files, restarting the engine service, re-creating it) for a
*  progress display such as the Workbench's Configure dialog. When the change
*  runs in a separate root process, it prints each step as a line the parent
*  reads while it runs.
 */

package dock

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

// configStageLinePrefix starts the lines a root process prints for each step.
const configStageLinePrefix = "rfswift-config-stage:"

var configStageReporter atomic.Pointer[func(stage string, percent int)]

// SetConfigStageReporter installs fn to receive the steps of configuration
// changes, with a rough percentage; nil removes it.
func SetConfigStageReporter(fn func(stage string, percent int)) {
	if fn == nil {
		configStageReporter.Store(nil)
		return
	}
	configStageReporter.Store(&fn)
}

// reportConfigStage tells the reporter, if any, that a change reached stage.
func reportConfigStage(stage string, percent int) {
	if fn := configStageReporter.Load(); fn != nil {
		(*fn)(stage, percent)
	}
}

// ConfigStageLine is the line a root process prints for a step.
func ConfigStageLine(stage string, percent int) string {
	return fmt.Sprintf("%s%d:%s", configStageLinePrefix, percent, strings.ReplaceAll(stage, "\n", " "))
}

// ParseConfigStageLine reads a line printed by ConfigStageLine.
func ParseConfigStageLine(line string) (stage string, percent int, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(line), configStageLinePrefix)
	if !found {
		return "", 0, false
	}
	number, stage, found := strings.Cut(rest, ":")
	percent, err := strconv.Atoi(number)
	if !found || err != nil || percent < 0 || percent > 100 || stage == "" {
		return "", 0, false
	}
	return stage, percent, true
}
