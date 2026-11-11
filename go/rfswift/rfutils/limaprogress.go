/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 *
 * Progress of a Lima VM boot, for front-ends that cannot show limactl's
 * terminal output (the Workbench).
 *
 * `limactl --log-format json start` logs each boot step as a JSON line on
 * stderr: launching the VM driver, then the hostagent's requirement checks
 * ("Waiting for the essential requirement 1 of 3: `ssh`", "The final
 * requirement 1 of 1 is satisfied", ...), then "READY". A warm boot takes
 * ~30 s and most of it is the final requirement (boot scripts), so the steps
 * are weighted rather than spread evenly. Between two steps the bar creeps
 * towards the next one so it never looks frozen. It never passes that step.
 *
 * Without a reporter (the CLI), limactl keeps writing straight to the
 * terminal as before.
 */

package rfutils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LimaProgress is one update of a Lima VM boot.
type LimaProgress struct {
	Instance string `json:"instance"`
	Percent  int    `json:"percent"` // 0-100
	Stage    string `json:"stage"`
	Elapsed  int    `json:"elapsed"` // seconds since the boot began
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// LimaProgressReporter receives Lima boot progress.
type LimaProgressReporter func(LimaProgress)

var (
	limaProgressMu sync.Mutex
	limaReporter   LimaProgressReporter
	limaSession    *limaBootSession
)

// limaBootSession spans one boot, from the first limactl call to Docker being
// reachable, so the bar and elapsed time carry over between those steps.
type limaBootSession struct {
	instance string
	start    time.Time
	percent  int
}

// SetLimaProgressReporter routes Lima boot progress to f. With a reporter,
// limactl output is parsed instead of copied to the terminal. nil restores
// the terminal output.
func SetLimaProgressReporter(f LimaProgressReporter) {
	limaProgressMu.Lock()
	defer limaProgressMu.Unlock()
	limaReporter = f
	limaSession = nil
}

func limaProgressReporter() LimaProgressReporter {
	limaProgressMu.Lock()
	defer limaProgressMu.Unlock()
	return limaReporter
}

// ReportLimaProgress sends a boot step. The bar never moves backwards within
// a boot. It is a no-op without a reporter.
//
//	in(1): string instance Lima instance name
//	in(2): int percent progress 0-100
//	in(3): string stage what is happening
func ReportLimaProgress(instance string, percent int, stage string) {
	limaProgressMu.Lock()
	f := limaReporter
	if f == nil {
		limaProgressMu.Unlock()
		return
	}
	s := limaSession
	if s == nil || s.instance != instance {
		s = &limaBootSession{instance: instance, start: time.Now()}
		limaSession = s
	}
	if percent > 99 {
		percent = 99 // 100 is FinishLimaProgress
	}
	if percent < s.percent {
		percent = s.percent
	}
	s.percent = percent
	p := LimaProgress{Instance: instance, Percent: percent, Stage: stage, Elapsed: int(time.Since(s.start).Seconds())}
	limaProgressMu.Unlock()
	f(p)
}

// FinishLimaProgress ends the boot: done at 100% on success, or the error.
//
//	in(1): string instance Lima instance name
//	in(2): error err nil once the engine is reachable
func FinishLimaProgress(instance string, err error) {
	limaProgressMu.Lock()
	f := limaReporter
	s := limaSession
	limaSession = nil
	limaProgressMu.Unlock()
	if f == nil {
		return
	}
	p := LimaProgress{Instance: instance, Percent: 100, Stage: "Lima VM is ready", Done: true}
	if s != nil {
		p.Elapsed = int(time.Since(s.start).Seconds())
	}
	if err != nil {
		p.Stage = "Lima VM failed to start"
		p.Error = err.Error()
		if s != nil {
			p.Percent = s.percent
		}
	}
	f(p)
}

// Bands of the bar, in percent. The final requirement band is the largest:
// on a warm boot it is where most of the time goes.
const (
	limaPctPrepare  = 2
	limaPctLaunch   = 5
	limaPctDownload = 6
	limaPctBoot     = 12
	limaPctReady    = 90
)

var limaRequirementBands = map[string][2]float64{
	"essential": {15, 40},
	"optional":  {40, 52},
	"final":     {52, 88},
}

var (
	limaWaitRe = regexp.MustCompile("Waiting for the (essential|optional|final) requirement (\\d+) of (\\d+): [`\"]?(.*?)[`\"]?$")
	limaDoneRe = regexp.MustCompile(`The (essential|optional|final) requirement (\d+) of (\d+) is satisfied`)
)

// limaBootTracker turns limactl log messages into a percentage.
type limaBootTracker struct {
	mu      sync.Mutex
	cur     float64 // shown percentage (creeps)
	ceil    float64 // creeping stops here, just short of the next step
	stage   string
	lastErr string
}

func newLimaBootTracker() *limaBootTracker {
	return &limaBootTracker{stage: "Starting the Lima VM"}
}

// at moves to a step reached at floor percent, creeping up to ceil.
func (t *limaBootTracker) at(floor, ceil float64, stage string) {
	if floor > t.cur {
		t.cur = floor
	}
	if ceil < t.cur {
		ceil = t.cur
	}
	t.ceil = ceil
	if stage != "" {
		t.stage = stage
	}
}

// observe reads one stderr line of `limactl --log-format json`. Plain text
// lines (older limactl, or output not from the logger) are matched as is.
//
//	out: bool true when the step changed
func (t *limaBootTracker) observe(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	msg, level := line, ""
	var entry struct {
		Level string `json:"level"`
		Msg   string `json:"msg"`
	}
	if strings.HasPrefix(line, "{") && json.Unmarshal([]byte(line), &entry) == nil {
		msg, level = entry.Msg, entry.Level
	}
	msg = strings.TrimSpace(strings.TrimPrefix(msg, "[hostagent] "))

	t.mu.Lock()
	defer t.mu.Unlock()
	before, beforePct := t.stage, int(t.cur)
	if level == "fatal" || level == "error" {
		t.lastErr = msg
	}
	switch {
	case limaWaitRe.MatchString(msg):
		m := limaWaitRe.FindStringSubmatch(msg)
		lo, hi, i, n := limaRequirementStep(m[1], m[2], m[3])
		t.at(lo+(hi-lo)*(i-1)/n, lo+(hi-lo)*i/n-1, fmt.Sprintf("Booting: %s (%s check %d/%d)", limaRequirementLabel(m[4]), m[1], int(i), int(n)))
	case limaDoneRe.MatchString(msg):
		m := limaDoneRe.FindStringSubmatch(msg)
		lo, hi, i, n := limaRequirementStep(m[1], m[2], m[3])
		t.at(lo+(hi-lo)*i/n, 0, "")
	case strings.HasPrefix(msg, "READY"):
		t.at(limaPctReady, limaPctReady, "VM is up")
	case strings.Contains(msg, "Starting QEMU"), strings.Contains(msg, "Starting VZ"),
		strings.Contains(msg, "Starting krunkit"), strings.Contains(msg, "hostagent socket created"):
		t.at(limaPctBoot, limaRequirementBands["essential"][0]-1, "Booting the VM")
	case strings.Contains(msg, "Attempting to download"):
		t.at(limaPctDownload, limaPctBoot-1, "Downloading the VM image (first start only)")
	case strings.HasPrefix(msg, "Starting the instance"):
		t.at(limaPctLaunch, limaPctBoot-1, "Launching the VM")
	case strings.HasPrefix(msg, "Using the existing instance"):
		t.at(limaPctPrepare, limaPctLaunch, "Preparing the VM")
	}
	return t.stage != before || int(t.cur) != beforePct
}

// limaRequirementStep returns the band of a requirement kind and i, n as floats.
func limaRequirementStep(kind, is, ns string) (lo, hi, i, n float64) {
	band := limaRequirementBands[kind]
	iv, _ := strconv.Atoi(is)
	nv, _ := strconv.Atoi(ns)
	if nv < 1 {
		nv = 1
	}
	if iv < 1 {
		iv = 1
	}
	if iv > nv {
		iv = nv
	}
	return band[0], band[1], float64(iv), float64(nv)
}

// limaRequirementLabel turns the probe names Lima uses into readable steps.
func limaRequirementLabel(desc string) string {
	switch strings.ToLower(strings.TrimSpace(desc)) {
	case "ssh":
		return "waiting for SSH"
	case "user session is ready for ssh":
		return "waiting for the user session"
	case "boot scripts must have finished", "boot scripts must have exited":
		return "running the boot scripts"
	case "systemd must be available":
		return "waiting for systemd"
	}
	return desc
}

// tick advances the creep and returns the percentage and step to show.
func (t *limaBootTracker) tick() (int, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ceil > t.cur {
		t.cur += (t.ceil - t.cur) * 0.08
	}
	return int(t.cur), t.stage
}

// runLimactlWithProgress runs limactl with JSON logs, reporting the boot of
// instance as it goes.
//
//	in(1): string instance Lima instance name
//	in(2): []string args limactl arguments after the global flags
//	out: error the limactl failure, with its last error message
func runLimactlWithProgress(instance string, args ...string) error {
	cmd := exec.Command(LimaCtl(), append([]string{"--log-format", "json"}, args...)...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	tracker := newLimaBootTracker()
	report := func() {
		percent, stage := tracker.tick()
		ReportLimaProgress(instance, percent, stage)
	}
	report()

	stop := make(chan struct{})
	ticked := make(chan struct{})
	go func() {
		defer close(ticked)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				report()
			}
		}
	}()

	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if tracker.observe(scanner.Text()) {
			report()
		}
	}
	err = cmd.Wait()
	close(stop)
	<-ticked
	if err != nil {
		tracker.mu.Lock()
		detail := tracker.lastErr
		tracker.mu.Unlock()
		if detail != "" {
			err = fmt.Errorf("limactl %s: %s", args[0], detail)
		} else {
			err = fmt.Errorf("limactl %s: %w", args[0], err)
		}
		FinishLimaProgress(instance, err)
		return err
	}
	report()
	return nil
}
