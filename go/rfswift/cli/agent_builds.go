/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Remote agent: target creation as a job the client can follow.
*
*  targets.create.v2 answers only when the environment is built, so a
*  Workbench connected to an agent saw nothing for the whole Nix build. These
*  methods run the creation in the background and let the client poll for the
*  build's progress snapshots (rfnix.BuildProgress, the same the local path
*  renders) and its log lines, cancel it, and collect the result:
*
*    targets.create.start  {CreateRequest}          -> {job}
*    targets.create.poll   {job, cursor}            -> {progress, progressSeq, lines, cursor, done, target, error}
*    targets.create.cancel {job}                    -> {}
*
*  Image pulls get the same treatment (the local dialog shows layer progress):
*
*    images.pull.start  {engine, image} -> {job}
*    images.pull.poll   {job}           -> {layers, progressSeq, done, resolved, error}
*    images.pull.cancel {job}           -> {}
 */

package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
)

const (
	remoteBuildKeepLines = 4000             // log lines kept for a client that polls slowly
	remoteBuildExpiry    = 10 * time.Minute // finished jobs nobody collected are dropped after this
)

type remoteBuild struct {
	mu          sync.Mutex
	progress    rfnix.BuildProgress
	progressSeq int
	lines       []string
	base        int // absolute index of lines[0]
	done        bool
	target      agentTarget
	err         string
	cancel      context.CancelFunc
	finished    time.Time
}

var remoteBuilds = struct {
	sync.Mutex
	jobs map[string]*remoteBuild
}{jobs: map[string]*remoteBuild{}}

type remoteBuildPoll struct {
	Progress    *rfnix.BuildProgress `json:"progress,omitempty"`
	ProgressSeq int                  `json:"progressSeq"`
	Lines       []string             `json:"lines"`
	Cursor      int                  `json:"cursor"`
	Done        bool                 `json:"done"`
	Target      *agentTarget         `json:"target,omitempty"`
	Error       string               `json:"error,omitempty"`
}

func newJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// expireRemoteBuilds drops finished jobs nobody collected. Called with the
// registry locked.
func expireRemoteBuilds() {
	for id, b := range remoteBuilds.jobs {
		b.mu.Lock()
		stale := b.done && time.Since(b.finished) > remoteBuildExpiry
		b.mu.Unlock()
		if stale {
			delete(remoteBuilds.jobs, id)
		}
	}
}

// agentCreateStart begins a creation in the background and returns its job id.
func agentCreateStart(p agentCreate) (map[string]any, error) {
	if p.Name == "" {
		return nil, errors.New("target name is required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &remoteBuild{cancel: cancel}
	p.Context = ctx
	build := rfnix.BuildOptions{
		Context: ctx,
		Progress: func(snapshot rfnix.BuildProgress) {
			b.mu.Lock()
			b.progress = snapshot
			b.progressSeq++
			b.mu.Unlock()
		},
		BuildLog: rfnix.NewLineWriter(func(line string) {
			b.mu.Lock()
			b.lines = append(b.lines, line)
			if drop := len(b.lines) - remoteBuildKeepLines; drop > 0 {
				b.lines = append([]string(nil), b.lines[drop:]...)
				b.base += drop
			}
			b.mu.Unlock()
		}),
	}
	id := newJobID()
	remoteBuilds.Lock()
	expireRemoteBuilds()
	remoteBuilds.jobs[id] = b
	remoteBuilds.Unlock()
	go func() {
		target, err := agentCreateTargetWith(p, build)
		b.mu.Lock()
		b.done, b.target, b.finished = true, target, time.Now()
		if err != nil {
			b.err = err.Error()
		}
		b.mu.Unlock()
		cancel()
	}()
	return map[string]any{"job": id}, nil
}

func lookupRemoteBuild(job string) (*remoteBuild, error) {
	remoteBuilds.Lock()
	defer remoteBuilds.Unlock()
	b := remoteBuilds.jobs[job]
	if b == nil {
		return nil, fmt.Errorf("unknown creation job %q (it may have expired)", job)
	}
	return b, nil
}

// agentCreatePoll returns what happened since cursor: the latest progress
// snapshot, the new log lines, and the result once the creation ended.
func agentCreatePoll(job string, cursor int) (remoteBuildPoll, error) {
	b, err := lookupRemoteBuild(job)
	if err != nil {
		return remoteBuildPoll{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := remoteBuildPoll{ProgressSeq: b.progressSeq, Lines: []string{}, Done: b.done, Error: b.err}
	if b.progressSeq > 0 {
		snapshot := b.progress
		out.Progress = &snapshot
	}
	if cursor < b.base {
		out.Lines = append(out.Lines, fmt.Sprintf("... %d lines not kept on the agent (full log: %s)", b.base-cursor, rfnix.BuildLogPath(b.target.ID)))
		cursor = b.base
	}
	if i := cursor - b.base; i >= 0 && i < len(b.lines) {
		out.Lines = append(out.Lines, b.lines[i:]...)
	}
	out.Cursor = b.base + len(b.lines)
	if b.done && b.err == "" {
		target := b.target
		out.Target = &target
	}
	return out, nil
}

// agentCreateCancel interrupts a running creation; its poll then reports the
// cancellation as the error.
func agentCreateCancel(job string) error {
	b, err := lookupRemoteBuild(job)
	if err != nil {
		return err
	}
	b.cancel()
	return nil
}

// ---------------------------------------------------------------------------
// Image pulls
// ---------------------------------------------------------------------------

type remotePull struct {
	mu       sync.Mutex
	layers   map[string]rfdock.PullProgress
	order    []string
	seq      int
	done     bool
	resolved string
	err      string
	cancel   context.CancelFunc
	finished time.Time
}

var remotePulls = struct {
	sync.Mutex
	jobs map[string]*remotePull
}{jobs: map[string]*remotePull{}}

type remotePullPoll struct {
	Layers      []rfdock.PullProgress `json:"layers"`
	ProgressSeq int                   `json:"progressSeq"`
	Done        bool                  `json:"done"`
	Resolved    string                `json:"resolved"`
	Error       string                `json:"error,omitempty"`
}

func expireRemotePulls() {
	for id, j := range remotePulls.jobs {
		j.mu.Lock()
		stale := j.done && time.Since(j.finished) > remoteBuildExpiry
		j.mu.Unlock()
		if stale {
			delete(remotePulls.jobs, id)
		}
	}
}

// agentPullStart begins an image pull in the background; the latest state of
// every layer is kept for the client's polls.
func agentPullStart(engine, image string) (map[string]any, error) {
	if engine == "" || image == "" {
		return nil, errors.New("engine and image are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &remotePull{layers: map[string]rfdock.PullProgress{}, cancel: cancel}
	id := newJobID()
	remotePulls.Lock()
	expireRemotePulls()
	remotePulls.jobs[id] = j
	remotePulls.Unlock()
	go func() {
		resolved, err := rfdock.PullImageContext(ctx, engine, image, func(p rfdock.PullProgress) {
			j.mu.Lock()
			if _, seen := j.layers[p.Layer]; !seen {
				j.order = append(j.order, p.Layer)
			}
			j.layers[p.Layer] = p
			j.seq++
			j.mu.Unlock()
		})
		j.mu.Lock()
		j.done, j.resolved, j.finished = true, resolved, time.Now()
		if err != nil {
			j.err = err.Error()
		}
		j.mu.Unlock()
		cancel()
	}()
	return map[string]any{"job": id}, nil
}

func lookupRemotePull(job string) (*remotePull, error) {
	remotePulls.Lock()
	defer remotePulls.Unlock()
	j := remotePulls.jobs[job]
	if j == nil {
		return nil, fmt.Errorf("unknown pull job %q (it may have expired)", job)
	}
	return j, nil
}

func agentPullPoll(job string) (remotePullPoll, error) {
	j, err := lookupRemotePull(job)
	if err != nil {
		return remotePullPoll{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := remotePullPoll{Layers: make([]rfdock.PullProgress, 0, len(j.order)), ProgressSeq: j.seq, Done: j.done, Resolved: j.resolved, Error: j.err}
	for _, layer := range j.order {
		out.Layers = append(out.Layers, j.layers[layer])
	}
	return out, nil
}

func agentPullCancel(job string) error {
	j, err := lookupRemotePull(job)
	if err != nil {
		return err
	}
	j.cancel()
	return nil
}
