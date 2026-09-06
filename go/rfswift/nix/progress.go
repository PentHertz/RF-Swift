/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - live progress of a `nix build`.
*
*  A terminal user sees Nix's own progress bar. A GUI (the Workbench) runs the
*  build without a terminal and used to see nothing until it ended. This file
*  reads Nix's machine-readable log stream (`--log-format internal-json`, the
*  same stream Nix's progress bar is drawn from) and turns it into
*  BuildProgress snapshots: how many derivations are built and still to build,
*  how many store paths are fetched from the binary cache and how many remain,
*  what is being compiled right now and in which phase, and the log lines.
 */

package nix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Activity and result types of Nix's logger (src/libutil/logging.hh). Only
// the ones the monitor reacts to are named.
const (
	nixActCopyPath     = 100
	nixActFileTransfer = 101
	nixActRealise      = 102
	nixActCopyPaths    = 103
	nixActBuilds       = 104
	nixActBuild        = 105
	nixActSubstitute   = 108
	nixActFetchTree    = 112

	nixResBuildLogLine     = 101
	nixResSetPhase         = 104
	nixResProgress         = 105
	nixResSetExpected      = 106
	nixResPostBuildLogLine = 107

	// Nix log levels: 0 error, 1 warn, 2 notice, 3 info, 4 talkative, ...
	nixLevelInfo = 3
)

// BuildCounter counts one kind of work (derivations to build, store paths to
// fetch) the way Nix's own progress bar does: Expected is the total known so
// far and grows while Nix still discovers dependencies.
type BuildCounter struct {
	Done     int `json:"done"`
	Expected int `json:"expected"`
	Running  int `json:"running"`
	Failed   int `json:"failed"`
}

// BuildTask is one derivation being built right now.
type BuildTask struct {
	Name  string `json:"name"`  // package name, from the derivation's store name
	Phase string `json:"phase"` // unpackPhase, buildPhase, checkPhase, ... ("" until Nix says)
	Since int64  `json:"since"` // Unix milliseconds when the build started
}

// BuildProgress is a snapshot of a running `nix build`, in the shape the
// Workbench renders. Counts are Nix's own; they are exact for what Nix has
// scheduled so far, and the totals can still grow early in a build.
type BuildProgress struct {
	Step  string `json:"step"`  // which build: "prerequisites" (device/library layer) or "environment" (the tools)
	Stage string `json:"stage"` // evaluating, planning, fetching, building, done, failed, cancelled

	Builds  BuildCounter `json:"builds"`
	Fetches BuildCounter `json:"fetches"`

	// Bytes fetched from binary caches so far and the total Nix expects.
	DownloadedBytes    int64 `json:"downloadedBytes"`
	DownloadTotalBytes int64 `json:"downloadTotalBytes"`

	// Nix's up-front plan ("these N derivations will be built", "these M paths
	// will be fetched (X MiB download, Y MiB unpacked)"), when it printed one.
	PlannedBuilds        int   `json:"plannedBuilds"`
	PlannedFetches       int   `json:"plannedFetches"`
	PlannedDownloadBytes int64 `json:"plannedDownloadBytes"`
	PlannedUnpackedBytes int64 `json:"plannedUnpackedBytes"`

	Building []BuildTask `json:"building"` // oldest first
	Fetching []string    `json:"fetching"` // store names being downloaded, oldest first
	LastLine string      `json:"lastLine"` // last build output line, "<package>> <line>"
	Error    string      `json:"error,omitempty"`
	Done     bool        `json:"done"`
}

// BuildObserver receives BuildProgress snapshots while a build runs. It is
// called from the goroutine reading Nix's output, at most a few times per
// second, and once more with Done set when the build ends.
type BuildObserver func(BuildProgress)

// BuildOptions carries what a front end needs to follow and stop a build.
// The zero value means: stream to the terminal as before.
type BuildOptions struct {
	// Context, when set, cancels the build: Nix gets an interrupt so it can
	// clean up, then is killed if it lingers.
	Context context.Context
	// Progress receives live snapshots. When set (or BuildLog is), Nix runs
	// with its machine-readable log instead of the terminal output, and the
	// full human-readable log is also written to BuildLogPath.
	Progress BuildObserver
	// BuildLog receives the human-readable log, one line per Write.
	BuildLog io.Writer
}

func (o BuildOptions) observed() bool { return o.Progress != nil || o.BuildLog != nil }

// BuildLogPath is where the environment's last observed build wrote its full
// log (only builds started with a Progress or BuildLog observer, i.e. from the
// Workbench; a terminal user already saw it).
func BuildLogPath(name string) string { return filepath.Join(EnvDir(name), "build.log") }

// TotalTasks is the number of build and fetch tasks Nix knows about.
func (p BuildProgress) TotalTasks() int {
	return max(p.Builds.Expected, p.PlannedBuilds) + max(p.Fetches.Expected, p.PlannedFetches)
}

// DoneTasks is the number of those tasks that finished.
func (p BuildProgress) DoneTasks() int { return p.Builds.Done + p.Fetches.Done }

// Fraction is the share of known tasks that finished, 0..1. It can go down
// while Nix is still discovering work; front ends keep their bar monotonic.
func (p BuildProgress) Fraction() float64 {
	total := p.TotalTasks()
	if p.Done && p.Error == "" {
		return 1
	}
	if total == 0 {
		return 0
	}
	return min(1, float64(p.DoneTasks())/float64(total))
}

// Summary is a one-line, human-readable state for a progress label.
func (p BuildProgress) Summary() string {
	what := "tools"
	if p.Step == "prerequisites" {
		what = "device and library layer"
	}
	switch p.Stage {
	case "evaluating":
		return "Evaluating the " + what
	case "planning":
		return fmt.Sprintf("Planned: %d to build, %d to download (%s)", p.PlannedBuilds, p.PlannedFetches, formatBytes(p.PlannedDownloadBytes))
	case "done":
		return "Built the " + what
	case "failed":
		return "Build failed"
	case "cancelled":
		return "Build cancelled"
	}
	var parts []string
	if t := max(p.Builds.Expected, p.PlannedBuilds); t > 0 || p.Builds.Done > 0 {
		s := fmt.Sprintf("built %d/%d", p.Builds.Done, t)
		if p.Builds.Running > 0 {
			s += fmt.Sprintf(" (%d running)", p.Builds.Running)
		}
		parts = append(parts, s)
	}
	if t := max(p.Fetches.Expected, p.PlannedFetches); t > 0 || p.Fetches.Done > 0 {
		s := fmt.Sprintf("fetched %d/%d", p.Fetches.Done, t)
		if p.DownloadTotalBytes > 0 {
			s += fmt.Sprintf(" (%s of %s)", formatBytes(p.DownloadedBytes), formatBytes(p.DownloadTotalBytes))
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "Preparing the " + what
	}
	s := strings.Join(parts, ", ")
	return strings.ToUpper(s[:1]) + s[1:]
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%d KiB", (n+512)/1024)
	}
}

// storeName strips the /nix/store/<hash>- prefix and a .drv suffix:
// "/nix/store/abc...-gnuradio-3.10.9.drv" -> "gnuradio-3.10.9".
func storeName(p string) string {
	base := path.Base(strings.TrimSuffix(strings.TrimSpace(p), ".drv"))
	if i := strings.IndexByte(base, '-'); i == 32 {
		return base[i+1:]
	}
	return base
}

// Build output is meant for a terminal: test runners colour their dots,
// downloaders redraw a bar with carriage returns, configure scripts print
// thousands of columns. The log and the panel get the plain text.
const (
	maxLogLineLength  = 2000
	maxLastLineLength = 240
)

var otherEscapeRe = regexp.MustCompile("\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|\x1b[@-_]")

// cleanLogLine strips terminal escape sequences and control characters, keeps
// what a carriage-return redraw would have left visible, and caps the length.
func cleanLogLine(line string) string {
	if i := strings.LastIndexByte(line, '\r'); i >= 0 {
		line = line[i+1:]
	}
	if strings.IndexByte(line, 0x1b) >= 0 {
		line = ansiEscapeRe.ReplaceAllString(line, "")
		line = otherEscapeRe.ReplaceAllString(line, "")
	}
	line = strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, line)
	if len(line) > maxLogLineLength {
		line = line[:maxLogLineLength] + " ..."
	}
	return line
}

// preview shortens a line for the single-line "now" display.
func preview(line string) string {
	if len(line) <= maxLastLineLength {
		return line
	}
	return line[:maxLastLineLength] + " ..."
}

// LineWriter turns writes into whole lines for fn (no trailing newline).
// Partial lines are kept until the newline arrives; Flush delivers a last
// unterminated line.
type LineWriter struct {
	fn      func(string)
	pending []byte
	mu      sync.Mutex
}

func NewLineWriter(fn func(string)) *LineWriter { return &LineWriter{fn: fn} }

func (w *LineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.pending[:i]), "\r")
		w.pending = w.pending[i+1:]
		w.fn(line)
	}
	return len(p), nil
}

// Flush delivers a trailing line that had no newline.
func (w *LineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		w.fn(strings.TrimRight(string(w.pending), "\r"))
		w.pending = nil
	}
}

// ---------------------------------------------------------------------------
// The monitor: Nix's internal-json stream in, BuildProgress out
// ---------------------------------------------------------------------------

const buildEmitInterval = 250 * time.Millisecond

var (
	plannedBuildsRe = regexp.MustCompile(`^(?:these (\d+) derivations|this derivation) will be built`)
	plannedFetchRe  = regexp.MustCompile(`^(?:these (\d+) paths|this path) will be fetched \(([\d.]+) MiB download, ([\d.]+) MiB unpacked\)`)
)

type nixActivity struct {
	typ   int
	name  string
	phase string
	since time.Time
	// bytes, for file transfers
	done, expected int64
}

// buildMonitor is the io.Writer Nix's stderr is attached to. It keeps one
// BuildProgress up to date, forwards a human-readable log and notifies the
// observer, rate-limited, with a trailing flush so the last state always
// reaches the front end.
type buildMonitor struct {
	mu       sync.Mutex
	observe  BuildObserver
	log      io.Writer
	progress BuildProgress
	pending  []byte
	acts     map[int64]*nixActivity
	// bytes of file transfers that already ended
	finishedBytes int64
	lastEmit      time.Time
	flush         *time.Timer
	closed        bool
	lastError     string
	sawActivity   bool
}

func newBuildMonitor(step string, observe BuildObserver, log io.Writer) *buildMonitor {
	return &buildMonitor{
		observe:  observe,
		log:      log,
		progress: BuildProgress{Step: step, Stage: "evaluating", Building: []BuildTask{}, Fetching: []string{}},
		acts:     map[int64]*nixActivity{},
	}
}

// Write receives Nix's stderr: "@nix {json}" lines and, rarely, plain text.
func (m *buildMonitor) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = append(m.pending, p...)
	for {
		i := bytes.IndexByte(m.pending, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(m.pending[:i]), "\r")
		m.pending = m.pending[i+1:]
		m.handleLine(line)
	}
	return len(p), nil
}

// plainWriter returns a writer for the build's stdout: plain lines that go to
// the log only.
func (m *buildMonitor) plainWriter() io.Writer {
	return NewLineWriter(func(line string) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.logLine(line)
	})
}

// handleLine is called with the lock held.
func (m *buildMonitor) handleLine(line string) {
	if !strings.HasPrefix(line, "@nix ") {
		line = cleanLogLine(line)
		if line == "" {
			return
		}
		m.logLine(line)
		if strings.HasPrefix(line, "error:") {
			m.lastError = line
		}
		return
	}
	var ev struct {
		Action string `json:"action"`
		ID     int64  `json:"id"`
		Type   int    `json:"type"`
		Level  int    `json:"level"`
		Text   string `json:"text"`
		Msg    string `json:"msg"`
		Fields []any  `json:"fields"`
	}
	if err := json.Unmarshal([]byte(line[len("@nix "):]), &ev); err != nil {
		m.logLine(line)
		return
	}
	notable := false
	switch ev.Action {
	case "msg":
		m.message(ev.Level, ev.Msg)
		notable = true
	case "start":
		notable = m.start(ev.ID, ev.Type, ev.Level, ev.Text, ev.Fields)
	case "result":
		notable = m.result(ev.ID, ev.Type, ev.Fields)
	case "stop":
		notable = m.stop(ev.ID)
	default:
		return
	}
	m.emit(notable)
}

func (m *buildMonitor) message(level int, msg string) {
	msg = cleanLogLine(msg)
	if level <= nixLevelInfo && msg != "" {
		m.logLine(msg)
	}
	if level == 0 {
		// Nix reports failures as multi-line error messages; keep the message
		// so the caller's error names the cause instead of "exit status 1".
		m.lastError = msg
	}
	if mt := plannedBuildsRe.FindStringSubmatch(msg); mt != nil {
		m.progress.PlannedBuilds = 1
		if mt[1] != "" {
			m.progress.PlannedBuilds, _ = strconv.Atoi(mt[1])
		}
		if !m.sawActivity {
			m.progress.Stage = "planning"
		}
	}
	if mt := plannedFetchRe.FindStringSubmatch(msg); mt != nil {
		m.progress.PlannedFetches = 1
		if mt[1] != "" {
			m.progress.PlannedFetches, _ = strconv.Atoi(mt[1])
		}
		if mib, err := strconv.ParseFloat(mt[2], 64); err == nil {
			m.progress.PlannedDownloadBytes = int64(mib * (1 << 20))
		}
		if mib, err := strconv.ParseFloat(mt[3], 64); err == nil {
			m.progress.PlannedUnpackedBytes = int64(mib * (1 << 20))
		}
		if !m.sawActivity {
			m.progress.Stage = "planning"
		}
	}
}

func fieldString(fields []any, i int) string {
	if i < len(fields) {
		if s, ok := fields[i].(string); ok {
			return s
		}
	}
	return ""
}

func fieldInt(fields []any, i int) int64 {
	if i < len(fields) {
		if f, ok := fields[i].(float64); ok {
			return int64(f)
		}
	}
	return 0
}

func (m *buildMonitor) start(id int64, typ, level int, text string, fields []any) bool {
	act := &nixActivity{typ: typ, since: time.Now()}
	m.acts[id] = act
	notable := false
	switch typ {
	case nixActBuild:
		act.name = storeName(fieldString(fields, 0))
		m.progress.Building = append(m.progress.Building, BuildTask{Name: act.name, Since: act.since.UnixMilli()})
		m.sawActivity, notable = true, true
	case nixActSubstitute:
		act.name = storeName(fieldString(fields, 0))
		m.progress.Fetching = append(m.progress.Fetching, act.name)
		m.sawActivity, notable = true, true
	case nixActFileTransfer, nixActCopyPath:
		act.name = fieldString(fields, 0)
	case nixActRealise, nixActBuilds, nixActCopyPaths:
		if m.progress.Stage == "evaluating" {
			m.progress.Stage = "planning"
		}
	case nixActFetchTree:
		m.sawActivity = true
	}
	if text = cleanLogLine(text); text != "" && level <= nixLevelInfo {
		m.logLine(text)
	}
	m.refreshStage()
	return notable
}

func (m *buildMonitor) result(id int64, typ int, fields []any) bool {
	act := m.acts[id]
	if act == nil {
		return false
	}
	switch typ {
	case nixResProgress:
		done, expected, running, failed := fieldInt(fields, 0), fieldInt(fields, 1), fieldInt(fields, 2), fieldInt(fields, 3)
		switch act.typ {
		case nixActBuilds:
			m.progress.Builds = BuildCounter{Done: int(done), Expected: int(expected), Running: int(running), Failed: int(failed)}
		case nixActCopyPaths:
			m.progress.Fetches = BuildCounter{Done: int(done), Expected: int(expected), Running: int(running), Failed: int(failed)}
		case nixActFileTransfer:
			act.done, act.expected = done, expected
		default:
			return false
		}
		m.refreshBytes()
		m.refreshStage()
		return false
	case nixResSetExpected:
		if fieldInt(fields, 0) == nixActFileTransfer {
			m.progress.DownloadTotalBytes = fieldInt(fields, 1)
		}
		return false
	case nixResSetPhase:
		act.phase = fieldString(fields, 0)
		for i := range m.progress.Building {
			if m.progress.Building[i].Name == act.name {
				m.progress.Building[i].Phase = act.phase
			}
		}
		return true
	case nixResBuildLogLine, nixResPostBuildLogLine:
		line := cleanLogLine(fieldString(fields, 0))
		if line == "" {
			return false
		}
		if act.name != "" {
			line = act.name + "> " + line
		}
		m.progress.LastLine = preview(line)
		m.logLine(line)
		return false
	}
	return false
}

func (m *buildMonitor) stop(id int64) bool {
	act := m.acts[id]
	if act == nil {
		return false
	}
	delete(m.acts, id)
	switch act.typ {
	case nixActBuild:
		for i, t := range m.progress.Building {
			if t.Name == act.name {
				m.progress.Building = append(m.progress.Building[:i], m.progress.Building[i+1:]...)
				break
			}
		}
	case nixActSubstitute:
		for i, n := range m.progress.Fetching {
			if n == act.name {
				m.progress.Fetching = append(m.progress.Fetching[:i], m.progress.Fetching[i+1:]...)
				break
			}
		}
	case nixActFileTransfer:
		m.finishedBytes += act.done
		m.refreshBytes()
		return false
	default:
		return false
	}
	m.refreshStage()
	return true
}

func (m *buildMonitor) refreshBytes() {
	var active, expected int64
	for _, a := range m.acts {
		if a.typ == nixActFileTransfer {
			active += a.done
			expected += a.expected
		}
	}
	m.progress.DownloadedBytes = m.finishedBytes + active
	if total := m.finishedBytes + expected; total > m.progress.DownloadTotalBytes {
		m.progress.DownloadTotalBytes = total
	}
}

func (m *buildMonitor) refreshStage() {
	switch {
	case len(m.progress.Building) > 0 || m.progress.Builds.Running > 0:
		m.progress.Stage = "building"
	case len(m.progress.Fetching) > 0 || m.progress.Fetches.Running > 0:
		m.progress.Stage = "fetching"
	case m.sawActivity:
		// Between tasks: keep whichever kind of work is still pending.
		if m.progress.Builds.Done < max(m.progress.Builds.Expected, m.progress.PlannedBuilds) {
			m.progress.Stage = "building"
		} else if m.progress.Fetches.Done < max(m.progress.Fetches.Expected, m.progress.PlannedFetches) {
			m.progress.Stage = "fetching"
		}
	}
}

func (m *buildMonitor) logLine(line string) {
	if m.log != nil {
		_, _ = io.WriteString(m.log, line+"\n")
	}
}

// snapshot copies the progress so the observer can keep it.
func (m *buildMonitor) snapshot() BuildProgress {
	p := m.progress
	p.Building = append([]BuildTask(nil), m.progress.Building...)
	sort.SliceStable(p.Building, func(i, j int) bool { return p.Building[i].Since < p.Building[j].Since })
	p.Fetching = append([]string(nil), m.progress.Fetching...)
	return p
}

// emit notifies the observer, at most once per buildEmitInterval; a change
// inside the window is delivered by a trailing timer. Called with the lock
// held.
func (m *buildMonitor) emit(notable bool) {
	if m.observe == nil || m.closed {
		return
	}
	now := time.Now()
	if wait := buildEmitInterval - now.Sub(m.lastEmit); wait > 0 && !(notable && wait < buildEmitInterval/2) {
		if m.flush == nil {
			m.flush = time.AfterFunc(wait, func() {
				m.mu.Lock()
				defer m.mu.Unlock()
				m.flush = nil
				if m.closed {
					return
				}
				m.lastEmit = time.Now()
				m.observe(m.snapshot())
			})
		}
		return
	}
	if m.flush != nil {
		m.flush.Stop()
		m.flush = nil
	}
	m.lastEmit = now
	m.observe(m.snapshot())
}

// finish delivers the final snapshot and returns Nix's own reason when the
// build failed ("" when it succeeded or said nothing).
func (m *buildMonitor) finish(err error, cancelled bool) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pending) > 0 {
		m.handleLine(strings.TrimRight(string(m.pending), "\r"))
		m.pending = nil
	}
	if m.flush != nil {
		m.flush.Stop()
		m.flush = nil
	}
	m.progress.Done = true
	m.progress.Building = m.progress.Building[:0]
	m.progress.Fetching = m.progress.Fetching[:0]
	reason := ""
	switch {
	case cancelled:
		m.progress.Stage = "cancelled"
		m.progress.Error = "cancelled"
	case err != nil:
		m.progress.Stage = "failed"
		reason = strings.TrimSpace(m.lastError)
		if reason == "" {
			reason = err.Error()
		}
		m.progress.Error = reason
	default:
		m.progress.Stage = "done"
	}
	m.closed = true
	if m.observe != nil {
		m.observe(m.snapshot())
	}
	return reason
}

// ---------------------------------------------------------------------------
// Running a build under the monitor
// ---------------------------------------------------------------------------

// runNixBuild runs `nix <args>` (a build) as the terminal would, or, when
// opts asks for it, under the monitor: Nix's machine-readable log is parsed
// into progress snapshots and a human-readable log. installable only names
// the build in error messages.
func runNixBuild(opts BuildOptions, step, installable string, args ...string) error {
	if !opts.observed() {
		cmd := nixCommand(append(args, "--print-build-logs")...)
		cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
		if err := runWithContext(opts.Context, cmd); err != nil {
			if opts.Context != nil && opts.Context.Err() != nil {
				return fmt.Errorf("build of %s cancelled", installable)
			}
			return fmt.Errorf("nix build failed for %s: %w", installable, err)
		}
		return nil
	}
	mon := newBuildMonitor(step, opts.Progress, opts.BuildLog)
	cmd := nixCommand(append(args, "--log-format", "internal-json")...)
	cmd.Stdout = mon.plainWriter()
	cmd.Stderr = mon
	cmd.Stdin = nil
	err := runWithContext(opts.Context, cmd)
	cancelled := opts.Context != nil && opts.Context.Err() != nil
	reason := mon.finish(err, cancelled)
	if err == nil {
		return nil
	}
	if cancelled {
		return fmt.Errorf("build of %s cancelled", installable)
	}
	return fmt.Errorf("nix build failed for %s: %s", installable, reason)
}

// runWithContext runs cmd and, when ctx ends first, interrupts Nix (so it
// can release its build locks and temporary directories) and kills it if it
// is still there a few seconds later.
func runWithContext(ctx context.Context, cmd *exec.Cmd) error {
	if ctx == nil {
		return cmd.Run()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			interruptProcess(cmd.Process, done)
		case <-done:
		}
	}()
	err := cmd.Wait()
	close(done)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func interruptProcess(p *os.Process, done <-chan struct{}) {
	if p == nil {
		return
	}
	if err := p.Signal(os.Interrupt); err != nil {
		// Windows has no interrupt for a child; wsl.exe and the build behind
		// it end with the kill.
		_ = p.Kill()
		return
	}
	select {
	case <-time.After(10 * time.Second):
		_ = p.Kill()
	case <-done:
	}
}

// ---------------------------------------------------------------------------
// Progress over a text stream (the Windows front end drives the Linux CLI)
// ---------------------------------------------------------------------------

// ProgressLinePrefix marks a BuildProgress snapshot on stdout when the CLI
// runs with --progress-json; every other line is log output.
const ProgressLinePrefix = "@rfswift-progress "

// JSONProgressObserver returns an observer that prints each snapshot as one
// ProgressLinePrefix line on w.
func JSONProgressObserver(w io.Writer) BuildObserver {
	var mu sync.Mutex
	return func(p BuildProgress) {
		data, err := json.Marshal(p)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		_, _ = fmt.Fprintf(w, "%s%s\n", ProgressLinePrefix, data)
	}
}

// ProgressStreamWriter is the reverse: an io.Writer that splits a stream
// written by JSONProgressObserver back into snapshots for observe and plain
// lines for log (ANSI colours stripped).
func ProgressStreamWriter(observe BuildObserver, log io.Writer) *LineWriter {
	return NewLineWriter(func(line string) {
		if strings.HasPrefix(line, ProgressLinePrefix) {
			var p BuildProgress
			if err := json.Unmarshal([]byte(line[len(ProgressLinePrefix):]), &p); err == nil {
				if observe != nil {
					observe(p)
				}
				return
			}
		}
		if log != nil {
			_, _ = io.WriteString(log, stripANSI(line)+"\n")
		}
	})
}
