package nix

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// Lines as Nix 2.35 prints them with --log-format internal-json for a two-
// derivation build (captured), plus a substitution and a download the small
// build did not exercise.
const sampleNixJSON = `@nix {"action":"msg","level":3,"msg":"these 2 derivations will be built:"}
@nix {"action":"msg","level":3,"msg":"  /nix/store/gkgczv196yqdy34zzpahns455gdbfh7x-rfswift-progress-sample-a.drv"}
@nix {"action":"msg","level":3,"msg":"  /nix/store/zxzv8hiwzpqdqnsyrdcwjn6z4y8mhkdv-rfswift-progress-sample-b.drv"}
@nix {"action":"msg","level":3,"msg":"these 3 paths will be fetched (12.50 MiB download, 40.00 MiB unpacked):"}
@nix {"action":"start","id":1,"level":0,"parent":0,"text":"","type":102}
@nix {"action":"start","id":2,"level":0,"parent":0,"text":"","type":104}
@nix {"action":"start","id":3,"level":0,"parent":0,"text":"","type":103}
@nix {"action":"result","fields":[101,13107200],"id":1,"type":106}
@nix {"action":"start","fields":["/nix/store/abcdefghijklmnopqrstuvwxyz012345-numpy-2.1.0","https://cache.nixos.org"],"id":10,"level":3,"parent":0,"text":"copying path '/nix/store/abcdefghijklmnopqrstuvwxyz012345-numpy-2.1.0' from 'https://cache.nixos.org'","type":108}
@nix {"action":"start","fields":["https://cache.nixos.org/nar/x.nar.xz"],"id":11,"level":4,"parent":10,"text":"downloading 'https://cache.nixos.org/nar/x.nar.xz'","type":101}
@nix {"action":"result","fields":[0,3,1,0],"id":3,"type":105}
@nix {"action":"result","fields":[4194304,8388608,0,0],"id":11,"type":105}
@nix {"action":"stop","id":11}
@nix {"action":"result","fields":[1,3,0,0],"id":3,"type":105}
@nix {"action":"stop","id":10}
@nix {"action":"start","fields":["/nix/store/gkgczv196yqdy34zzpahns455gdbfh7x-rfswift-progress-sample-a.drv","",1,1],"id":20,"level":3,"parent":0,"text":"building '/nix/store/gkgczv196yqdy34zzpahns455gdbfh7x-rfswift-progress-sample-a.drv'","type":105}
@nix {"action":"result","fields":[0,2,1,0],"id":2,"type":105}
@nix {"action":"result","fields":["buildPhase"],"id":20,"type":104}
@nix {"action":"result","fields":["hello from a"],"id":20,"type":101}
@nix {"action":"result","fields":["second line"],"id":20,"type":101}
`

func feed(t *testing.T, m *buildMonitor, text string, chunk int) {
	t.Helper()
	data := []byte(text)
	for len(data) > 0 {
		n := min(chunk, len(data))
		if _, err := m.Write(data[:n]); err != nil {
			t.Fatal(err)
		}
		data = data[n:]
	}
}

func TestBuildMonitorFollowsNixStream(t *testing.T) {
	var log bytes.Buffer
	m := newBuildMonitor("environment", nil, &log)
	// Chunks of 7 bytes exercise the partial-line buffering.
	feed(t, m, sampleNixJSON, 7)

	m.mu.Lock()
	p := m.snapshot()
	m.mu.Unlock()

	if p.PlannedBuilds != 2 || p.PlannedFetches != 3 {
		t.Errorf("planned = %d builds, %d fetches; want 2, 3", p.PlannedBuilds, p.PlannedFetches)
	}
	if p.PlannedDownloadBytes != int64(12.5*(1<<20)) || p.PlannedUnpackedBytes != 40<<20 {
		t.Errorf("planned bytes = %d/%d", p.PlannedDownloadBytes, p.PlannedUnpackedBytes)
	}
	if p.Builds != (BuildCounter{Done: 0, Expected: 2, Running: 1}) {
		t.Errorf("builds = %+v", p.Builds)
	}
	if p.Fetches != (BuildCounter{Done: 1, Expected: 3}) {
		t.Errorf("fetches = %+v", p.Fetches)
	}
	if p.DownloadedBytes != 4194304 || p.DownloadTotalBytes != 13107200 {
		t.Errorf("download = %d of %d", p.DownloadedBytes, p.DownloadTotalBytes)
	}
	if len(p.Building) != 1 || p.Building[0].Name != "rfswift-progress-sample-a" || p.Building[0].Phase != "buildPhase" {
		t.Errorf("building = %+v", p.Building)
	}
	if len(p.Fetching) != 0 {
		t.Errorf("fetching should be empty after the stop, got %v", p.Fetching)
	}
	if p.Stage != "building" {
		t.Errorf("stage = %q, want building", p.Stage)
	}
	if p.LastLine != "rfswift-progress-sample-a> second line" {
		t.Errorf("last line = %q", p.LastLine)
	}
	if p.TotalTasks() != 5 || p.DoneTasks() != 1 {
		t.Errorf("tasks = %d/%d, want 1/5", p.DoneTasks(), p.TotalTasks())
	}
	for _, want := range []string{
		"these 2 derivations will be built:",
		"copying path '/nix/store/abcdefghijklmnopqrstuvwxyz012345-numpy-2.1.0' from 'https://cache.nixos.org'",
		"building '/nix/store/gkgczv196yqdy34zzpahns455gdbfh7x-rfswift-progress-sample-a.drv'",
		"rfswift-progress-sample-a> hello from a",
	} {
		if !strings.Contains(log.String(), want+"\n") {
			t.Errorf("log lacks %q:\n%s", want, log.String())
		}
	}
	if strings.Contains(log.String(), "downloading 'https://cache.nixos.org/nar/x.nar.xz'") {
		t.Error("talkative (level 4) activity leaked into the log")
	}

	// A stop for the build, then success.
	feed(t, m, `@nix {"action":"stop","id":20}`+"\n"+`@nix {"action":"result","fields":[1,2,0,0],"id":2,"type":105}`+"\n", 1000)
	if reason := m.finish(nil, false); reason != "" {
		t.Errorf("finish on success returned reason %q", reason)
	}
	m.mu.Lock()
	p = m.snapshot()
	m.mu.Unlock()
	if !p.Done || p.Stage != "done" || len(p.Building) != 0 || p.Builds.Done != 1 {
		t.Errorf("final = %+v", p)
	}
}

func TestBuildMonitorReportsNixReason(t *testing.T) {
	m := newBuildMonitor("environment", nil, nil)
	feed(t, m, `@nix {"action":"msg","level":0,"msg":"error: builder for '/nix/store/abc-tailcat.drv' failed with exit code 1"}`+"\n", 1000)
	reason := m.finish(errors.New("exit status 1"), false)
	if !strings.Contains(reason, "tailcat.drv") {
		t.Errorf("reason = %q, want Nix's own message", reason)
	}
	m.mu.Lock()
	p := m.snapshot()
	m.mu.Unlock()
	if p.Stage != "failed" || p.Error != reason || !p.Done {
		t.Errorf("failed snapshot = %+v", p)
	}

	m = newBuildMonitor("environment", nil, nil)
	// Nothing from Nix: the process error is all there is. A trailing line
	// without newline is still parsed.
	feed(t, m, `error: interrupted by the user`, 1000)
	if reason := m.finish(errors.New("exit status 1"), false); reason != "error: interrupted by the user" {
		t.Errorf("reason = %q", reason)
	}
	m = newBuildMonitor("environment", nil, nil)
	if reason := m.finish(errors.New("exit status 1"), false); reason != "exit status 1" {
		t.Errorf("reason without Nix message = %q", reason)
	}
	m = newBuildMonitor("environment", nil, nil)
	m.finish(errors.New("signal: killed"), true)
	if m.progress.Stage != "cancelled" {
		t.Errorf("cancelled stage = %q", m.progress.Stage)
	}
}

func TestBuildMonitorDeliversLastStateUnderThrottle(t *testing.T) {
	var mu sync.Mutex
	var got []BuildProgress
	m := newBuildMonitor("environment", func(p BuildProgress) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, p)
	}, nil)
	// A burst far denser than the emit interval: only a few callbacks may
	// happen, but the last state must arrive without another write.
	var burst strings.Builder
	burst.WriteString(`@nix {"action":"start","id":2,"level":0,"parent":0,"text":"","type":104}` + "\n")
	for i := 1; i <= 200; i++ {
		burst.WriteString(`@nix {"action":"result","fields":[` + itoa(i) + `,200,0,0],"id":2,"type":105}` + "\n")
	}
	feed(t, m, burst.String(), 1<<20)
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		last := BuildProgress{}
		if n > 0 {
			last = got[n-1]
		}
		mu.Unlock()
		if last.Builds.Done == 200 {
			if n > 20 {
				t.Errorf("%d callbacks for a burst; the observer should be rate-limited", n)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("last state never delivered; %d callbacks, last %+v", n, last.Builds)
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.finish(nil, false)
	mu.Lock()
	defer mu.Unlock()
	if !got[len(got)-1].Done {
		t.Error("finish did not deliver the final snapshot")
	}
}

func itoa(i int) string {
	var b [8]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

func TestProgressStreamRoundTrip(t *testing.T) {
	var stream bytes.Buffer
	observe := JSONProgressObserver(&stream)
	want := BuildProgress{Step: "prerequisites", Stage: "building", Builds: BuildCounter{Done: 3, Expected: 9, Running: 2},
		Building: []BuildTask{{Name: "hackrf-2024.02.1", Phase: "buildPhase", Since: 1700000000000}}, Fetching: []string{}, LastLine: "hackrf> [ 50%] Building C object"}
	observe(want)
	stream.WriteString("\x1b[34m[i]\x1b[0m Realising device/library prerequisites for 'sdr_light' ...\n")
	stream.WriteString("plain trailing line")

	var got []BuildProgress
	var log bytes.Buffer
	w := ProgressStreamWriter(func(p BuildProgress) { got = append(got, p) }, &log)
	if _, err := w.Write(stream.Bytes()); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if len(got) != 1 {
		t.Fatalf("decoded %d snapshots, want 1", len(got))
	}
	if got[0].Step != want.Step || got[0].Builds != want.Builds || len(got[0].Building) != 1 || got[0].Building[0] != want.Building[0] || got[0].LastLine != want.LastLine {
		t.Errorf("round trip changed the snapshot: %+v", got[0])
	}
	if log.String() != "[i] Realising device/library prerequisites for 'sdr_light' ...\nplain trailing line\n" {
		t.Errorf("log = %q", log.String())
	}
}

func TestBuildProgressSummaryAndFraction(t *testing.T) {
	p := BuildProgress{Step: "environment", Stage: "building", Builds: BuildCounter{Done: 12, Expected: 57, Running: 3}, Fetches: BuildCounter{Done: 130, Expected: 300}, DownloadedBytes: 1288490188, DownloadTotalBytes: 4831838208}
	if s := p.Summary(); s != "Built 12/57 (3 running), fetched 130/300 (1.20 GiB of 4.50 GiB)" {
		t.Errorf("summary = %q", s)
	}
	if f := p.Fraction(); f < 0.397 || f > 0.398 {
		t.Errorf("fraction = %v, want 142/357", f)
	}
	// The plan counts more than Nix has scheduled so far: the larger total wins.
	p.PlannedBuilds = 80
	if p.TotalTasks() != 380 {
		t.Errorf("total with plan = %d", p.TotalTasks())
	}
	if (BuildProgress{Stage: "evaluating"}).Fraction() != 0 {
		t.Error("nothing known yet must be 0")
	}
	if (BuildProgress{Stage: "done", Done: true}).Fraction() != 1 {
		t.Error("a finished build is 1")
	}
	if s := (BuildProgress{Step: "prerequisites", Stage: "planning", PlannedBuilds: 1, PlannedFetches: 4, PlannedDownloadBytes: 3 << 20}).Summary(); s != "Planned: 1 to build, 4 to download (3.0 MiB)" {
		t.Errorf("planning summary = %q", s)
	}
	if s := (BuildProgress{Step: "prerequisites", Stage: "evaluating"}).Summary(); s != "Evaluating the device and library layer" {
		t.Errorf("evaluating summary = %q", s)
	}
}

func TestStoreName(t *testing.T) {
	for in, want := range map[string]string{
		"/nix/store/gkgczv196yqdy34zzpahns455gdbfh7x-rfswift-progress-sample-a.drv": "rfswift-progress-sample-a",
		"/nix/store/abcdefghijklmnopqrstuvwxyz012345-numpy-2.1.0":                   "numpy-2.1.0",
		"not-a-store-path": "not-a-store-path",
		"":                 ".",
	} {
		if got := storeName(in); got != want {
			t.Errorf("storeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLineWriterSplitsAndFlushes(t *testing.T) {
	var lines []string
	w := NewLineWriter(func(l string) { lines = append(lines, l) })
	w.Write([]byte("one\r\ntw"))
	w.Write([]byte("o\nthree"))
	if len(lines) != 2 || lines[0] != "one" || lines[1] != "two" {
		t.Fatalf("lines = %q", lines)
	}
	w.Flush()
	if len(lines) != 3 || lines[2] != "three" {
		t.Errorf("after flush = %q", lines)
	}
}
