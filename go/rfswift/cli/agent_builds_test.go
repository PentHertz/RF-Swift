package cli

import (
	"strings"
	"testing"
	"time"

	"penthertz/rfswift/remote"
)

// A creation job must be startable, pollable to its end, and gone from the
// registry once expired; an impossible request ends as an error the client
// can show, not as a hung job.
func TestRemoteCreationJobLifecycle(t *testing.T) {
	start, err := agentCreateStart(agentCreate(remote.CreateRequest{Name: "job-test", Engine: "nix", Image: "no-such-environment-xyz"}))
	if err != nil {
		t.Fatal(err)
	}
	job, _ := start["job"].(string)
	if job == "" {
		t.Fatal("start returned no job id")
	}
	deadline := time.Now().Add(60 * time.Second)
	var last remoteBuildPoll
	for {
		last, err = agentCreatePoll(job, 0)
		if err != nil {
			t.Fatal(err)
		}
		if last.Done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("creation job never finished")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if last.Error == "" || last.Target != nil {
		t.Errorf("an unknown environment must end the job with an error, got %+v", last)
	}
	if last.Lines == nil {
		t.Error("lines must be an array, not null")
	}
	if err := agentCreateCancel(job); err != nil {
		t.Errorf("cancel after the end must be harmless: %v", err)
	}
	if _, err := agentCreatePoll("no-such-job", 0); err == nil || !strings.Contains(err.Error(), "unknown creation job") {
		t.Errorf("polling an unknown job: %v", err)
	}

	// Expiry: a finished job older than the retention is dropped on the next
	// registry access.
	b, err := lookupRemoteBuild(job)
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	b.finished = time.Now().Add(-remoteBuildExpiry - time.Minute)
	b.mu.Unlock()
	if _, err := agentCreateStart(agentCreate(remote.CreateRequest{Name: "job-test-2", Engine: "nix", Image: "no-such-environment-xyz"})); err != nil {
		t.Fatal(err)
	}
	if _, err := lookupRemoteBuild(job); err == nil {
		t.Error("expired job still in the registry")
	}
	if _, err := agentCreateStart(agentCreate(remote.CreateRequest{Engine: "nix"})); err == nil {
		t.Error("a start without a name must be refused")
	}
}

func TestRemoteCreationPollKeepsOnlyRecentLines(t *testing.T) {
	b := &remoteBuild{}
	for i := 0; i < remoteBuildKeepLines+10; i++ {
		b.lines = append(b.lines, "line")
	}
	if drop := len(b.lines) - remoteBuildKeepLines; drop > 0 {
		b.lines = b.lines[drop:]
		b.base += drop
	}
	remoteBuilds.Lock()
	remoteBuilds.jobs["ring"] = b
	remoteBuilds.Unlock()
	defer func() {
		remoteBuilds.Lock()
		delete(remoteBuilds.jobs, "ring")
		remoteBuilds.Unlock()
	}()
	p, err := agentCreatePoll("ring", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Lines) != remoteBuildKeepLines+1 || !strings.HasPrefix(p.Lines[0], "... 10 lines not kept") {
		t.Errorf("a slow client must learn what it missed: %d lines, first %q", len(p.Lines), p.Lines[0])
	}
	if p.Cursor != remoteBuildKeepLines+10 {
		t.Errorf("cursor = %d", p.Cursor)
	}
	p, _ = agentCreatePoll("ring", p.Cursor)
	if len(p.Lines) != 0 {
		t.Errorf("nothing new must yield no lines, got %d", len(p.Lines))
	}
}

func TestRemotePullJobEndsWithAnErrorForAMissingEngine(t *testing.T) {
	if _, err := agentPullStart("", "x"); err == nil {
		t.Error("a start without engine or image must be refused")
	}
	start, err := agentPullStart("no-such-engine", "rfswift-sdr:latest")
	if err != nil {
		t.Fatal(err)
	}
	job := start["job"].(string)
	deadline := time.Now().Add(60 * time.Second)
	for {
		p, err := agentPullPoll(job)
		if err != nil {
			t.Fatal(err)
		}
		if p.Layers == nil {
			t.Fatal("layers must be an array, not null")
		}
		if p.Done {
			if p.Error == "" {
				t.Errorf("a pull on a missing engine must fail, got %+v", p)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pull job never finished")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := agentPullCancel(job); err != nil {
		t.Errorf("cancel after the end must be harmless: %v", err)
	}
}
