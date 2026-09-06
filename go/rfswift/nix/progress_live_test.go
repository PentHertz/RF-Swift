package nix

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// Drives a real `nix build` through runNixBuild: the monitor's wiring to the
// process (stdout, stderr, cancellation) is what the unit tests cannot cover.
// Opt in with RFSWIFT_TEST_NIX_BUILD=1 on a host with nix and a nixpkgs source
// in RFSWIFT_TEST_NIXPKGS (a store path or checkout, so no network is needed).
func liveNixBuildSetup(t *testing.T) string {
	t.Helper()
	if os.Getenv("RFSWIFT_TEST_NIX_BUILD") == "" {
		t.Skip("set RFSWIFT_TEST_NIX_BUILD=1 to run a real nix build")
	}
	if _, err := exec.LookPath("nix"); err != nil {
		t.Skip("nix is not on PATH")
	}
	nixpkgs := os.Getenv("RFSWIFT_TEST_NIXPKGS")
	if nixpkgs == "" {
		t.Skip("set RFSWIFT_TEST_NIXPKGS to a nixpkgs source path")
	}
	return nixpkgs
}

func TestRunNixBuildLive(t *testing.T) {
	nixpkgs := liveNixBuildSetup(t)
	expr := `with import ` + nixpkgs + ` {}; runCommand "rfswift-progress-live-` + time.Now().Format("150405") + `" {} "echo compiling live sample; mkdir -p $out"`
	var mu sync.Mutex
	var snaps []BuildProgress
	var log strings.Builder
	opts := BuildOptions{
		Progress: func(p BuildProgress) {
			mu.Lock()
			defer mu.Unlock()
			snaps = append(snaps, p)
		},
		BuildLog: NewLineWriter(func(l string) {
			mu.Lock()
			defer mu.Unlock()
			log.WriteString(l + "\n")
		}),
	}
	args := append(experimentalArgs(), "build", "--no-link", "--impure", "--expr", expr)
	if err := runNixBuild(opts, "environment", "live sample", args...); err != nil {
		t.Fatalf("runNixBuild: %v\nlog:\n%s", err, log.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(snaps) == 0 {
		t.Fatal("no progress snapshots")
	}
	last := snaps[len(snaps)-1]
	if !last.Done || last.Stage != "done" || last.Builds.Done != 1 {
		t.Errorf("final snapshot = %+v", last)
	}
	if !strings.Contains(log.String(), "rfswift-progress-live") || !strings.Contains(log.String(), "> compiling live sample") {
		t.Errorf("log lacks the build's own output:\n%s", log.String())
	}
	sawBuilding := false
	for _, s := range snaps {
		if len(s.Building) > 0 && strings.HasPrefix(s.Building[0].Name, "rfswift-progress-live") {
			sawBuilding = true
		}
	}
	if !sawBuilding {
		t.Errorf("no snapshot showed the derivation being built: %+v", snaps)
	}
}

func TestRunNixBuildLiveCancel(t *testing.T) {
	nixpkgs := liveNixBuildSetup(t)
	expr := `with import ` + nixpkgs + ` {}; runCommand "rfswift-progress-cancel-` + time.Now().Format("150405") + `" {} "echo started; sleep 60; mkdir -p $out"`
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var last BuildProgress
	opts := BuildOptions{Context: ctx, Progress: func(p BuildProgress) {
		mu.Lock()
		last = p
		mu.Unlock()
		if len(p.Building) > 0 {
			once.Do(func() { close(started) })
		}
	}}
	go func() {
		select {
		case <-started:
		case <-time.After(60 * time.Second):
		}
		cancel()
	}()
	args := append(experimentalArgs(), "build", "--no-link", "--impure", "--expr", expr)
	begin := time.Now()
	err := runNixBuild(opts, "environment", "cancel sample", args...)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected a cancellation error, got %v", err)
	}
	if time.Since(begin) > 40*time.Second {
		t.Errorf("cancellation took %s; the build should have been interrupted", time.Since(begin))
	}
	mu.Lock()
	defer mu.Unlock()
	if last.Stage != "cancelled" || !last.Done {
		t.Errorf("final snapshot = %+v", last)
	}
}
