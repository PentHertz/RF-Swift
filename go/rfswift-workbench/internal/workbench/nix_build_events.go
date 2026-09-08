package workbench

// Live status of a Nix build for the UI. The nix package follows `nix build`
// through Nix's machine-readable log (rfswift/nix/progress.go) and hands the
// local engine progress snapshots and log lines; this file turns them into
// Wails events the create dialog renders:
//
//	rfswift:nix-build      {mission, progress: rfnix.BuildProgress}
//	rfswift:nix-build-log  {mission, lines: [...]}   (batched)
//
// and keeps the existing rfswift:operation-progress bar moving with a
// percentage derived from the task counts.

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	rfnix "penthertz/rfswift/nix"
)

const (
	nixBuildLogFlushEvery = 150 * time.Millisecond
	nixBuildLogBatchMax   = 500
)

// nixBuildLogBatch collects log lines between flushes so a chatty configure
// step does not become thousands of events.
type nixBuildLogBatch struct {
	mu    sync.Mutex
	lines map[string][]string
	timer *time.Timer
}

// hookEngine gives an engine the UI's build observers: the local engine
// reports its own builds, a remote engine relays what the agent's job
// reports while a creation is polled.
func (a *App) hookEngine(eng Engine) {
	switch e := eng.(type) {
	case *LocalEngine:
		e.NixBuild, e.NixBuildLog = a.emitNixBuild, a.emitNixBuildLog
		e.AuditProgress = a.emitAuditStage
	case *RemoteEngine:
		e.NixBuild, e.NixBuildLog = a.emitNixBuild, a.emitNixBuildLog
	}
}

// nixBuildOptions wires a build started outside Create (update, rebuild) to
// the same observers.
func (a *App) nixBuildOptions(mission string) rfnix.BuildOptions {
	_, eng := a.currentScope()
	if local, ok := eng.(*LocalEngine); ok {
		return local.nixBuildOptions(mission, nil)
	}
	return rfnix.BuildOptions{}
}

func (a *App) emitNixBuild(mission string, p rfnix.BuildProgress) {
	if a.ctx == nil {
		return
	}
	wruntime.EventsEmit(a.ctx, "rfswift:nix-build", map[string]any{"mission": mission, "progress": p})
	operation := "mission-create"
	if op, ok := a.nixBuildOps.Load(mission); ok {
		operation = op.(string)
	}
	a.emitOperationProgress(operation, mission, nixBuildPercent(operation, p), p.Summary())
}

// nixBuildPercent maps a build's task fraction onto the operation's bar.
// CreateMission shows 55 before the build and 100 once the mission is saved;
// update and rebuild show 10 and 100. The device/library layer built first is
// small next to the tools, so it gets the first 15% of the span.
func nixBuildPercent(operation string, p rfnix.BuildProgress) int {
	lo, hi := 55.0, 97.0
	if operation == "nix-update" {
		lo = 10
	}
	span := hi - lo
	f := p.Fraction()
	if p.Step == "prerequisites" {
		return int(lo + span*0.15*f)
	}
	return int(lo + span*(0.15+0.85*f))
}

func (a *App) emitNixBuildLog(mission, line string) {
	b := &a.nixBuildLog
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.lines == nil {
		b.lines = map[string][]string{}
	}
	b.lines[mission] = append(b.lines[mission], line)
	if b.timer == nil {
		b.timer = time.AfterFunc(nixBuildLogFlushEvery, a.flushNixBuildLog)
	}
}

func (a *App) flushNixBuildLog() {
	b := &a.nixBuildLog
	b.mu.Lock()
	lines := b.lines
	b.lines, b.timer = nil, nil
	b.mu.Unlock()
	if a.ctx == nil {
		return
	}
	for mission, ls := range lines {
		if n := len(ls) - nixBuildLogBatchMax; n > 0 {
			ls = append([]string{fmt.Sprintf("... %d lines omitted (full log: %s)", n, rfnix.BuildLogPath(mission))}, ls[n:]...)
		}
		wruntime.EventsEmit(a.ctx, "rfswift:nix-build-log", map[string]any{"mission": mission, "lines": ls})
	}
}

// withNixBuildLogHint points a failed creation at the full build log when one
// was written.
func withNixBuildLogHint(mission string, err error) error {
	if err == nil {
		return nil
	}
	path := rfnix.BuildLogPath(mission)
	if info, statErr := os.Stat(path); statErr != nil || info.Size() == 0 {
		return err
	}
	return errors.Join(err, fmt.Errorf("full build log: %s", path))
}
