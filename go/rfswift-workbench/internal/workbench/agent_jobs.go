/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Background jobs and interactive sessions for MCP agents.
//
// execute_command used to block until the command ended and to hold all its
// output in memory, so an agent starting rtl_433, airodump-ng or a capture
// never got an answer and the Workbench kept growing. Every agent command now
// runs as a job: the call waits a bounded time (wait_seconds), then returns
// what the command printed so far and a job_id while the command keeps
// running; job_output, cancel_job and list_jobs follow it up. Output is cleaned
// of terminal escapes, written to the mission's agent-workspace/jobs/ folder
// (capped) and returned to the agent as a bounded excerpt with the log path.
// Sessions are persistent login shells the agent types into, for the
// interactive tools hardware work depends on (pm3, bluetoothctl, serial
// consoles, OpenOCD, gdb). Everything an agent started is stopped when its MCP
// server ends, so nothing (a transmission in particular) outlives the agent.

const (
	agentDefaultWait     = 60 * time.Second
	agentMaxWait         = 300 * time.Second
	agentMaxRunningJobs  = 8
	agentMaxSessions     = 6
	agentLogLimit        = 32 << 20
	agentInlineLimit     = 24 << 10
	agentReadDefault     = 16 << 10
	agentReadMax         = 64 << 10
	agentSessionIdle     = 1500 * time.Millisecond
	agentSessionMaxWait  = 120 * time.Second
	agentStopGrace       = 6 * time.Second
	agentKeptJobs        = 200
	agentExpectWindow    = 64 << 10
	agentPollInterval    = 100 * time.Millisecond
	agentDefaultSendWait = 3 * time.Second
)

var sessionNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

type agentJob struct {
	id, kind, name, mission, command, purpose string
	transmits                                 bool
	started                                   time.Time
	logPath                                   string
	proc                                      agentProcess
	events                                    *agentEventWriter
	done                                      chan struct{}

	mu         sync.Mutex
	log        *os.File
	cleaner    termCleaner
	size       int64
	limitHit   bool
	lastOutput time.Time
	ended      time.Time
	exitCode   int
	runErr     error
	stoppedBy  string
	readOff    int64
	changed    chan struct{}
}

type agentJobRegistry struct {
	mu       sync.Mutex
	stamp    string
	seq      int
	jobs     []*agentJob
	byID     map[string]*agentJob
	sessions map[string]*agentJob
}

func newAgentJobRegistry() *agentJobRegistry {
	return &agentJobRegistry{stamp: time.Now().UTC().Format("20060102T150405Z"), byID: map[string]*agentJob{}, sessions: map[string]*agentJob{}}
}

func (s *mcpServer) registry() *agentJobRegistry {
	s.jobsOnce.Do(func() {
		if s.jobs == nil {
			s.jobs = newAgentJobRegistry()
		}
	})
	return s.jobs
}

func (s *mcpServer) processStarter() (agentProcessStarter, error) {
	if s.procs != nil {
		return s.procs, nil
	}
	if starter, ok := s.eng.(agentProcessStarter); ok {
		return starter, nil
	}
	return nil, errors.New("command execution is unavailable for this engine")
}

// startAgentJob starts a job (sh -lc command) or, with interactive, a session
// shell named name. It does not wait.
func (s *mcpServer) startAgentJob(mission, kind, name, command, purpose string, transmits, interactive bool) (*agentJob, error) {
	starter, err := s.processStarter()
	if err != nil {
		return nil, err
	}
	reg := s.registry()
	reg.mu.Lock()
	running, sessions := 0, 0
	for _, j := range reg.jobs {
		if j.isRunning() {
			if j.kind == "session" {
				sessions++
			} else {
				running++
			}
		}
	}
	if kind == "session" && sessions >= agentMaxSessions {
		reg.mu.Unlock()
		return nil, fmt.Errorf("too many open sessions (%d): close one with session_close first", agentMaxSessions)
	}
	if kind != "session" && running >= agentMaxRunningJobs {
		reg.mu.Unlock()
		return nil, fmt.Errorf("too many running jobs (%d): wait for one with job_output or stop one with cancel_job first", agentMaxRunningJobs)
	}
	if kind == "session" {
		if old := reg.sessions[name]; old != nil && old.isRunning() {
			reg.mu.Unlock()
			return nil, fmt.Errorf("session %q is already open: use session_send, or session_close it first", name)
		}
	}
	reg.seq++
	id := fmt.Sprintf("job-%d", reg.seq)
	stamp := reg.stamp
	reg.mu.Unlock()

	dir := filepath.Join(s.store.missionDir(s.opts.Workspace, mission), "agent-workspace", "jobs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	logPath := filepath.Join(dir, stamp+"-"+id+".log")
	if kind == "session" {
		logPath = filepath.Join(dir, stamp+"-session-"+name+".log")
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	label := command
	if kind == "session" {
		label = "session " + name
		if command != "" {
			label += ": " + command
		}
	}
	if purpose != "" {
		label += "  # " + purpose
	}
	if transmits {
		label = "[RF TX] " + label
	}
	events, err := newAgentEventWriter(s.store, s.opts.Workspace, mission, label)
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("open live terminal stream: %w", err)
	}
	proc, err := starter.startAgentProcess(mission, command, interactive)
	if err != nil {
		events.finish(err)
		_ = logFile.Close()
		return nil, err
	}
	now := time.Now()
	job := &agentJob{id: id, kind: kind, name: name, mission: mission, command: command, purpose: purpose, transmits: transmits,
		started: now, logPath: logPath, proc: proc, events: events, done: make(chan struct{}), log: logFile, lastOutput: now, changed: make(chan struct{})}
	reg.mu.Lock()
	reg.jobs = append(reg.jobs, job)
	reg.byID[id] = job
	if kind == "session" {
		reg.sessions[name] = job
	}
	// Forget the oldest finished jobs so a long agent session stays bounded.
	for len(reg.jobs) > agentKeptJobs {
		dropped := false
		for i, j := range reg.jobs {
			if !j.isRunning() {
				delete(reg.byID, j.id)
				if j.kind == "session" && reg.sessions[j.name] == j {
					delete(reg.sessions, j.name)
				}
				reg.jobs = append(reg.jobs[:i], reg.jobs[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			break
		}
	}
	reg.mu.Unlock()
	go job.pump()
	return job, nil
}

func (j *agentJob) isRunning() bool {
	select {
	case <-j.done:
		return false
	default:
		return true
	}
}

func (j *agentJob) pump() {
	buf := make([]byte, 32<<10)
	for {
		n, err := j.proc.Read(buf)
		if n > 0 {
			_, _ = j.events.Write(buf[:n])
			j.mu.Lock()
			j.appendLocked(j.cleaner.Write(buf[:n]))
			j.lastOutput = time.Now()
			j.notifyLocked()
			j.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	code, waitErr := j.proc.Wait()
	_ = j.proc.Close()
	j.mu.Lock()
	j.appendLocked(j.cleaner.Flush())
	j.exitCode, j.runErr, j.ended = code, waitErr, time.Now()
	_ = j.log.Close()
	j.notifyLocked()
	stoppedBy := j.stoppedBy
	j.mu.Unlock()
	switch {
	case waitErr != nil:
		j.events.finish(waitErr)
	case stoppedBy != "":
		j.events.finish(fmt.Errorf("stopped (%s)", stoppedBy))
	case code != 0:
		j.events.finish(fmt.Errorf("command exited with status %d", code))
	default:
		j.events.finish(nil)
	}
	close(j.done)
}

func (j *agentJob) appendLocked(b []byte) {
	if len(b) == 0 || j.limitHit {
		return
	}
	if j.size+int64(len(b)) > agentLogLimit {
		b = b[:agentLogLimit-j.size]
		j.limitHit = true
	}
	n, _ := j.log.Write(b)
	j.size += int64(n)
	if j.limitHit {
		note := "\n[rfswift: output limit of 32 MiB reached; further output was discarded. Redirect large output to a file in the workspace.]\n"
		n, _ = j.log.WriteString(note)
		j.size += int64(n)
	}
}

func (j *agentJob) notifyLocked() {
	close(j.changed)
	j.changed = make(chan struct{})
}

// commit makes the current unterminated line (a prompt, a progress line) part
// of the log so readers see it.
func (j *agentJob) commit() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.ended.IsZero() {
		j.appendLocked(j.cleaner.Flush())
	}
}

// readRange returns log bytes [from, to).
func (j *agentJob) readRange(from, to int64) string {
	if to <= from {
		return ""
	}
	f, err := os.Open(j.logPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, to-from)
	n, _ := f.ReadAt(buf, from)
	return string(buf[:n])
}

// readExcerpt returns [from, to) shortened to about limit bytes, keeping the
// beginning and the end (where errors and results usually are).
func (j *agentJob) readExcerpt(from, to int64, limit int) (string, bool) {
	if to-from <= int64(limit) {
		return j.readRange(from, to), false
	}
	head := int64(limit / 5)
	tail := int64(limit) - head
	headText := trimToRunes(j.readRange(from, from+head), true)
	tailText := trimToRunes(j.readRange(to-tail, to), false)
	omitted := (to - from) - int64(len(headText)) - int64(len(tailText))
	return headText + fmt.Sprintf("\n[... %d bytes omitted: read them with job_output and an offset, or open the log file ...]\n", omitted) + tailText, true
}

func (j *agentJob) status() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.statusLocked()
}

func (j *agentJob) statusLocked() string {
	switch {
	case j.ended.IsZero():
		return "running"
	case j.stoppedBy != "":
		return "stopped"
	case j.runErr != nil || j.exitCode != 0:
		return "failed"
	default:
		return "completed"
	}
}

// stop terminates the job and waits for it to end.
func (j *agentJob) stop(reason string) {
	j.mu.Lock()
	if !j.ended.IsZero() {
		j.mu.Unlock()
		return
	}
	if j.stoppedBy == "" {
		j.stoppedBy = reason
	}
	j.mu.Unlock()
	_ = j.proc.Stop()
	select {
	case <-j.done:
	case <-time.After(agentStopGrace):
		_ = j.proc.Close()
		select {
		case <-j.done:
		case <-time.After(2 * time.Second):
		}
	}
}

// waitDone waits for the job to end, at most d.
func (j *agentJob) waitDone(d time.Duration) bool {
	if d <= 0 {
		return !j.isRunning()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-j.done:
		return true
	case <-timer.C:
		return false
	}
}

// waitOutput waits until expect matches the output written since from, or,
// without expect, until output arrived after since and then stayed quiet for
// idle, or until the job ends or maxWait passes. It reports whether expect
// matched.
func (j *agentJob) waitOutput(from int64, since time.Time, expect *regexp.Regexp, idle, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for {
		j.mu.Lock()
		ch, ended, last, size := j.changed, !j.ended.IsZero(), j.lastOutput, j.size
		partial := j.cleaner.Partial()
		j.mu.Unlock()
		if expect != nil {
			start := from
			if size-start > agentExpectWindow {
				start = size - agentExpectWindow
			}
			if expect.MatchString(j.readRange(start, size) + string(partial)) {
				return true
			}
		} else if last.After(since) && time.Since(last) >= idle {
			return false
		}
		if ended || time.Now().After(deadline) {
			return false
		}
		wait := agentPollInterval
		if expect == nil && last.After(since) {
			if rest := idle - time.Since(last); rest > 0 && rest < wait {
				wait = rest
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ch:
		case <-timer.C:
		}
		timer.Stop()
	}
}

func (j *agentJob) summary() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := map[string]any{"job_id": j.id, "kind": j.kind, "status": j.statusLocked(), "command": j.command, "started": j.started.UTC().Format(time.RFC3339),
		"elapsed_seconds": math.Round(j.elapsedLocked().Seconds()*10) / 10, "log": j.logPath, "output_bytes": j.size}
	if j.kind == "session" {
		out["name"] = j.name
	}
	if j.purpose != "" {
		out["purpose"] = j.purpose
	}
	if j.transmits {
		out["transmits"] = true
	}
	if !j.ended.IsZero() {
		out["exit_code"] = j.exitCode
		if j.runErr != nil {
			out["error"] = j.runErr.Error()
		}
		if j.stoppedBy != "" {
			out["stopped_by"] = j.stoppedBy
		}
	}
	if j.limitHit {
		out["output_limit_reached"] = true
	}
	return out
}

func (j *agentJob) elapsedLocked() time.Duration {
	if j.ended.IsZero() {
		return time.Since(j.started)
	}
	return j.ended.Sub(j.started)
}

// stopAll stops every running job and session (agent disconnect, shutdown).
func (r *agentJobRegistry) stopAll(reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	running := []*agentJob{}
	for _, j := range r.jobs {
		if j.isRunning() {
			running = append(running, j)
		}
	}
	r.mu.Unlock()
	var wg sync.WaitGroup
	for _, j := range running {
		wg.Add(1)
		go func(j *agentJob) {
			defer wg.Done()
			j.stop(reason)
		}(j)
	}
	wg.Wait()
}

func (r *agentJobRegistry) get(id string) (*agentJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j := r.byID[strings.TrimSpace(id)]; j != nil {
		return j, nil
	}
	return nil, fmt.Errorf("unknown job %q: list_jobs shows the jobs of this agent session", id)
}

func (r *agentJobRegistry) session(name string) (*agentJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j := r.sessions[strings.TrimSpace(name)]; j != nil {
		return j, nil
	}
	return nil, fmt.Errorf("unknown session %q: open it with session_start", name)
}

// --- MCP tool handlers -------------------------------------------------------

// argNumber reads a numeric argument (JSON numbers decode as float64; callers
// inside the process may pass integers).
func argNumber(args map[string]any, key string) (float64, bool) {
	switch v := args[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	}
	return 0, false
}

func argSeconds(args map[string]any, key string, def, max time.Duration) time.Duration {
	v, ok := argNumber(args, key)
	if !ok {
		return def
	}
	d := time.Duration(v * float64(time.Second))
	if d < 0 {
		d = 0
	}
	if d > max {
		d = max
	}
	return d
}

func argBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}

func argString(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

// runAgentCommand is execute_command and run_tool: start the command as a job,
// wait for it a bounded time, and report.
func (s *mcpServer) runAgentCommand(mission, command, purpose string, transmits bool, args map[string]any) (any, error) {
	if strings.TrimSpace(command) == "" || len(command) > 16384 {
		return nil, errors.New("a command of at most 16384 bytes is required")
	}
	tx, err := s.checkTransmit(command, transmits, purpose)
	if err != nil {
		return nil, err
	}
	txFreq, _ := argNumber(args, "tx_frequency_hz")
	if err := s.checkScope(mission, command, tx, txFreq); err != nil {
		return nil, err
	}
	wait := argSeconds(args, "wait_seconds", agentDefaultWait, agentMaxWait)
	maxRun := argSeconds(args, "max_seconds", 0, 7*24*time.Hour)
	job, err := s.startAgentJob(mission, "job", "", command, purpose, tx, false)
	if err != nil {
		return nil, err
	}
	if maxRun > 0 {
		timer := time.AfterFunc(maxRun, func() { job.stop("max_seconds") })
		go func() {
			<-job.done
			timer.Stop()
		}()
	}
	finished := job.waitDone(wait)
	if !finished {
		job.commit()
	}
	result := job.summary()
	job.mu.Lock()
	size := job.size
	job.mu.Unlock()
	output, truncated := job.readExcerpt(0, size, agentInlineLimit)
	result["output"] = output
	result["next_offset"] = size
	if truncated {
		result["output_truncated"] = true
	}
	if !finished {
		result["note"] = fmt.Sprintf("Still running after %s; it continues in the background. Follow it with job_output {job_id: %q, offset: %d}, stop it with cancel_job.", wait.Round(time.Second), job.id, size)
	}
	return result, nil
}

func (s *mcpServer) callJobOutput(args map[string]any) (any, error) {
	job, err := s.registry().get(argString(args, "job_id"))
	if err != nil {
		return nil, err
	}
	if wait := argSeconds(args, "wait_seconds", 0, agentMaxWait); wait > 0 {
		job.waitDone(wait)
	}
	if job.isRunning() {
		job.commit()
	}
	offset := int64(0)
	if v, ok := argNumber(args, "offset"); ok && v > 0 {
		offset = int64(v)
	}
	limit := agentReadDefault
	if v, ok := argNumber(args, "max_bytes"); ok && v > 0 {
		limit = int(math.Min(v, agentReadMax))
	}
	result := job.summary()
	job.mu.Lock()
	size := job.size
	job.mu.Unlock()
	if offset > size {
		offset = size
	}
	end := size
	if end-offset > int64(limit) {
		end = offset + int64(limit)
	}
	chunk := trimToRunes(job.readRange(offset, end), true)
	next := offset + int64(len(chunk))
	result["output"] = chunk
	result["offset"] = offset
	result["next_offset"] = next
	result["more"] = next < size
	return result, nil
}

func (s *mcpServer) callCancelJob(args map[string]any) (any, error) {
	job, err := s.registry().get(argString(args, "job_id"))
	if err != nil {
		return nil, err
	}
	job.stop("cancel_job")
	return job.summary(), nil
}

func (s *mcpServer) callListJobs() (any, error) {
	reg := s.registry()
	reg.mu.Lock()
	jobs := append([]*agentJob(nil), reg.jobs...)
	reg.mu.Unlock()
	sort.SliceStable(jobs, func(a, b int) bool { return jobs[a].started.After(jobs[b].started) })
	out := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, j.summary())
	}
	return map[string]any{"jobs": out}, nil
}

func (s *mcpServer) callSessionStart(mission string, args map[string]any) (any, error) {
	name := strings.TrimSpace(argString(args, "name"))
	if !sessionNamePattern.MatchString(name) {
		return nil, errors.New("name must be 1-32 characters: lowercase letters, digits, '-' or '_' (for example pm3, uart, ble)")
	}
	command := argString(args, "command")
	purpose := argString(args, "purpose")
	tx := false
	if command != "" {
		var err error
		if tx, err = s.checkTransmit(command, argBool(args, "transmits", false), purpose); err != nil {
			return nil, err
		}
		txFreq, _ := argNumber(args, "tx_frequency_hz")
		if err := s.checkScope(mission, command, tx, txFreq); err != nil {
			return nil, err
		}
	}
	job, err := s.startAgentJob(mission, "session", name, command, purpose, tx, true)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	job.waitOutput(0, start.Add(-time.Second), nil, 800*time.Millisecond, 5*time.Second)
	matched := false
	if command != "" {
		expect, err := compileExpect(args)
		if err != nil {
			return nil, err
		}
		job.mu.Lock()
		from := job.size
		job.mu.Unlock()
		sent := time.Now()
		if err := job.proc.Input([]byte(command + "\r")); err != nil {
			return nil, err
		}
		matched = job.waitOutput(from, sent, expect, agentSessionIdle, argSeconds(args, "wait_seconds", 5*time.Second, agentSessionMaxWait))
	}
	return s.sessionResult(job, matched), nil
}

func (s *mcpServer) callSessionSend(args map[string]any) (any, error) {
	job, err := s.registry().session(argString(args, "name"))
	if err != nil {
		return nil, err
	}
	if !job.isRunning() {
		result := s.sessionResult(job, false)
		result["note"] = "The session has ended; open a new one with session_start."
		return result, nil
	}
	input := argString(args, "input")
	enter := argBool(args, "enter", true)
	if strings.TrimSpace(input) != "" {
		tx, err := s.checkTransmit(input, argBool(args, "transmits", false), argString(args, "purpose"))
		if err != nil {
			return nil, err
		}
		txFreq, _ := argNumber(args, "tx_frequency_hz")
		if err := s.checkScope(job.mission, input, tx, txFreq); err != nil {
			return nil, err
		}
		if tx {
			_ = job.events.event(AgentTerminalEvent{Type: "output", Data: "\r\n[RF TX declared] " + argString(args, "purpose") + "\r\n"})
		}
	}
	expect, err := compileExpect(args)
	if err != nil {
		return nil, err
	}
	data := input
	if enter {
		data += "\r"
	}
	job.mu.Lock()
	from := job.size
	job.mu.Unlock()
	sent := time.Now()
	if data != "" {
		if err := job.proc.Input([]byte(data)); err != nil {
			return nil, err
		}
	}
	wait := argSeconds(args, "wait_seconds", agentDefaultSendWait, agentSessionMaxWait)
	matched := job.waitOutput(from, sent, expect, agentSessionIdle, wait)
	return s.sessionResult(job, matched), nil
}

func (s *mcpServer) callSessionClose(args map[string]any) (any, error) {
	job, err := s.registry().session(argString(args, "name"))
	if err != nil {
		return nil, err
	}
	job.stop("session_close")
	return s.sessionResult(job, false), nil
}

// sessionResult returns the output the agent has not seen yet and advances
// the session's read position.
func (s *mcpServer) sessionResult(job *agentJob, matched bool) map[string]any {
	job.commit()
	result := job.summary()
	job.mu.Lock()
	from, size := job.readOff, job.size
	job.readOff = size
	job.mu.Unlock()
	output, truncated := job.readExcerpt(from, size, agentInlineLimit)
	result["output"] = output
	if truncated {
		result["output_truncated"] = true
	}
	if matched {
		result["expect_matched"] = true
	}
	return result
}

func compileExpect(args map[string]any) (*regexp.Regexp, error) {
	pattern := argString(args, "expect")
	if pattern == "" {
		return nil, nil
	}
	if len(pattern) > 512 {
		return nil, errors.New("expect is limited to 512 characters")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("expect is not a valid RE2 regular expression: %w", err)
	}
	return re, nil
}

// trimToRunes drops an incomplete UTF-8 sequence at the end (fromStart) or at
// the start (!fromStart) of a byte range cut from the log.
func trimToRunes(s string, fromStart bool) string {
	if fromStart {
		for i := len(s) - 1; i >= 0 && i >= len(s)-4; i-- {
			c := s[i]
			if c < 0x80 {
				return s
			}
			if c >= 0xc0 { // lead byte: complete only if the sequence fits
				need := 2
				if c >= 0xf0 {
					need = 4
				} else if c >= 0xe0 {
					need = 3
				}
				if len(s)-i < need {
					return s[:i]
				}
				return s
			}
		}
		return s
	}
	i := 0
	for i < len(s) && i < 4 && s[i] >= 0x80 && s[i] < 0xc0 {
		i++
	}
	return s[i:]
}
