package rfutils

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// limaBootLog is a warm boot of the rfswift instance (Lima 2.2, qemu) as
// `limactl --log-format json start` writes it on stderr: client lines, and
// hostagent lines relayed with the "[hostagent] " prefix.
var limaBootLog = []string{
	"Using the existing instance `rfswift`",
	"Starting the instance `rfswift` with internal VM driver `qemu`",
	"QEMU binary \"/opt/homebrew/bin/qemu-system-aarch64\" seems properly signed with the \"com.apple.security.hypervisor\" entitlement",
	"[hostagent] hostagent socket created at /Users/u/.lima/rfswift/ha.sock",
	"[hostagent] Starting QEMU (hint: to watch the boot progress, see `/Users/u/.lima/rfswift/serial*.log`)",
	"SSH Local Port: 59834",
	"[hostagent] Waiting for the essential requirement 1 of 3: `ssh`",
	"[hostagent] The essential requirement 1 of 3 is satisfied",
	"[hostagent] Waiting for the essential requirement 2 of 3: `user session is ready for ssh`",
	"[hostagent] The essential requirement 2 of 3 is satisfied",
	"[hostagent] Waiting for the essential requirement 3 of 3: `Explicitly start ssh ControlMaster`",
	"[hostagent] The essential requirement 3 of 3 is satisfied",
	"[hostagent] Waiting for the optional requirement 1 of 2: `systemd must be available`",
	"[hostagent] Forwarding `/run/docker.sock` (guest) to `/Users/u/.lima/rfswift/sock/docker.sock` (host)",
	"[hostagent] The optional requirement 1 of 2 is satisfied",
	"[hostagent] Waiting for the optional requirement 2 of 2: `containerd binaries to be installed`",
	"[hostagent] The optional requirement 2 of 2 is satisfied",
	"[hostagent] Waiting for the final requirement 1 of 1: `boot scripts must have finished`",
	"[hostagent] Not forwarding TCP 0.0.0.0:22",
	"[hostagent] The final requirement 1 of 1 is satisfied",
	"READY. Run `limactl shell rfswift` to open the shell.",
}

func jsonLogLine(level, msg string) string {
	b, _ := json.Marshal(map[string]string{"level": level, "msg": msg, "time": "2026-11-10T21:51:49+01:00"})
	return string(b)
}

func TestLimaBootTrackerFollowsTheBootSteps(t *testing.T) {
	tr := newLimaBootTracker()
	last := -1
	stages := map[string]int{}
	for _, msg := range limaBootLog {
		tr.observe(jsonLogLine("info", msg))
		tr.mu.Lock()
		pct, stage := int(tr.cur), tr.stage
		tr.mu.Unlock()
		if pct < last {
			t.Fatalf("progress went back from %d to %d at %q", last, pct, msg)
		}
		last = pct
		if _, seen := stages[stage]; !seen {
			stages[stage] = pct
		}
	}
	if last != limaPctReady {
		t.Fatalf("READY left the bar at %d, want %d", last, limaPctReady)
	}
	for _, want := range []string{
		"Preparing the VM",
		"Launching the VM",
		"Booting the VM",
		"Booting: waiting for SSH (essential check 1/3)",
		"Booting: waiting for systemd (optional check 1/2)",
		"Booting: running the boot scripts (final check 1/1)",
		"VM is up",
	} {
		if _, ok := stages[want]; !ok {
			t.Errorf("stage %q never shown; got %v", want, stages)
		}
	}
	// The boot-script check is where a warm boot spends most of its time,
	// so it starts well before the end of the bar.
	if p := stages["Booting: running the boot scripts (final check 1/1)"]; p < 50 || p > 60 {
		t.Errorf("final check starts at %d%%, want 50-60", p)
	}
}

func TestLimaBootTrackerCreepsButStopsBeforeTheNextStep(t *testing.T) {
	tr := newLimaBootTracker()
	tr.observe(jsonLogLine("info", "[hostagent] Waiting for the final requirement 1 of 1: `boot scripts must have finished`"))
	var pct int
	for i := 0; i < 300; i++ {
		pct, _ = tr.tick()
	}
	if pct <= int(limaRequirementBands["final"][0]) {
		t.Fatalf("no creep while waiting: %d%%", pct)
	}
	if pct >= int(limaRequirementBands["final"][1]) {
		t.Fatalf("creep reached the next step: %d%%", pct)
	}
}

func TestLimaBootTrackerPlainTextAndErrors(t *testing.T) {
	tr := newLimaBootTracker()
	// Older limactl / text logs: matched on the raw line.
	if !tr.observe(`time="2026-11-10T21:51:49+01:00" level=info msg="[hostagent] Waiting for the essential requirement 1 of 2: \"ssh\""`) {
		t.Fatal("text log line not recognised")
	}
	tr.mu.Lock()
	stage := tr.stage
	tr.mu.Unlock()
	if !strings.Contains(stage, "essential check 1/2") {
		t.Fatalf("stage = %q", stage)
	}
	tr.observe(jsonLogLine("fatal", "instance \"rfswift\" is in an inconsistent state"))
	if tr.lastErr != "instance \"rfswift\" is in an inconsistent state" {
		t.Fatalf("lastErr = %q", tr.lastErr)
	}
	if tr.observe("") || tr.observe("{not json") {
		t.Fatal("noise changed the step")
	}
}

func TestReportLimaProgressSession(t *testing.T) {
	var got []LimaProgress
	SetLimaProgressReporter(func(p LimaProgress) { got = append(got, p) })
	defer SetLimaProgressReporter(nil)

	ReportLimaProgress("rfswift", 40, "a")
	ReportLimaProgress("rfswift", 20, "b") // never backwards
	ReportLimaProgress("rfswift", 150, "c")
	FinishLimaProgress("rfswift", nil)
	ReportLimaProgress("rfswift", 10, "next boot")
	FinishLimaProgress("rfswift", errors.New("boom"))

	want := []int{40, 40, 99, 100, 10, 10}
	if len(got) != len(want) {
		t.Fatalf("got %d updates: %+v", len(got), got)
	}
	for i, p := range got {
		if p.Percent != want[i] {
			t.Errorf("update %d: percent %d, want %d (%+v)", i, p.Percent, want[i], p)
		}
	}
	if !got[3].Done || got[3].Error != "" {
		t.Errorf("success not reported as done: %+v", got[3])
	}
	if !got[5].Done || got[5].Error != "boom" {
		t.Errorf("failure not reported: %+v", got[5])
	}

	// Without a reporter nothing is sent and limactl output is untouched.
	SetLimaProgressReporter(nil)
	if limaProgressReporter() != nil {
		t.Fatal("reporter still set")
	}
	ReportLimaProgress("rfswift", 50, "ignored")
}

// Live check of the limactl pipeline (RFSWIFT_TEST_LIMA=1, rfswift instance
// running): `start` on a running instance returns at once, and a limactl
// failure comes back with limactl's own message instead of "exit status 1".
func TestRunLimactlWithProgressLive(t *testing.T) {
	if os.Getenv("RFSWIFT_TEST_LIMA") != "1" || !IsLimaInstalled() || !IsLimaInstanceRunning("rfswift") {
		t.Skip("set RFSWIFT_TEST_LIMA=1 with the rfswift Lima instance running")
	}
	var got []LimaProgress
	SetLimaProgressReporter(func(p LimaProgress) { got = append(got, p) })
	defer SetLimaProgressReporter(nil)

	if err := StartLimaInstance("rfswift"); err != nil {
		t.Fatalf("start on a running instance: %v", err)
	}
	if len(got) == 0 || got[len(got)-1].Stage != "Preparing the VM" {
		t.Fatalf("progress = %+v", got)
	}

	got = nil
	missing := "rfswift-no-such-instance-test"
	err := runLimactlWithProgress(missing, "stop", "--tty=false", missing)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("error = %v, want limactl's message naming %s", err, missing)
	}
	if last := got[len(got)-1]; !last.Done || last.Error == "" {
		t.Fatalf("failure not reported as finished: %+v", last)
	}
}
