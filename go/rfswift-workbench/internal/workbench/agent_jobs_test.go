//go:build !windows

package workbench

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"penthertz/rfswift/ptyx"
)

// hostTestStarter runs agent processes on the test host with sh, through the
// same PTY process type Nix missions use.
type hostTestStarter struct{}

func (hostTestStarter) startAgentProcess(mission, command string, interactive bool) (agentProcess, error) {
	cmd := exec.Command("sh", "-c", command)
	if interactive {
		cmd = exec.Command("sh", "-i")
	}
	cmd.Env = append(os.Environ(), "PS1=$ ", "ENV=")
	term, err := ptyx.Start(cmd, agentTermCols, agentTermRows)
	if err != nil {
		return nil, err
	}
	return &hostPTYProcess{term: term}, nil
}

func newJobTestServer(t *testing.T) *mcpServer {
	t.Helper()
	store := NewStore(t.TempDir())
	if err := store.SaveMission("default", Mission{ID: "lab"}); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{store: store, eng: &fakeEngine{}, procs: hostTestStarter{},
		opts: MCPOptions{Workspace: "default", Mission: "lab", AllowWrite: true, AllowExec: true}}
	t.Cleanup(func() { s.registry().stopAll("test end") })
	return s
}

func callMap(t *testing.T, s *mcpServer, name string, args map[string]any) map[string]any {
	t.Helper()
	got, err := s.call(name, args)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("%s returned %T", name, got)
	}
	return m
}

func TestExecuteCommandReturnsCleanOutputAndExitCode(t *testing.T) {
	s := newJobTestServer(t)
	res := callMap(t, s, "execute_command", map[string]any{"command": "printf '\\033[31mred\\033[0m\\n'; exit 3", "purpose": "test"})
	if res["status"] != "failed" || res["exit_code"] != 3 {
		t.Fatalf("unexpected result: %#v", res)
	}
	if out, _ := res["output"].(string); out != "red\n" {
		t.Fatalf("output not cleaned: %q", out)
	}
	if _, err := os.Stat(res["log"].(string)); err != nil {
		t.Fatalf("log file missing: %v", err)
	}
}

func TestExecuteCommandBecomesBackgroundJob(t *testing.T) {
	s := newJobTestServer(t)
	res := callMap(t, s, "execute_command", map[string]any{"command": "echo started; sleep 1; echo finished", "wait_seconds": 0.2})
	if res["status"] != "running" || res["job_id"] == "" {
		t.Fatalf("expected a running job: %#v", res)
	}
	out := callMap(t, s, "job_output", map[string]any{"job_id": res["job_id"], "wait_seconds": 5})
	if out["status"] != "completed" || !strings.Contains(out["output"].(string), "finished") {
		t.Fatalf("job did not complete: %#v", out)
	}
	next := out["next_offset"].(int64)
	tail := callMap(t, s, "job_output", map[string]any{"job_id": res["job_id"], "offset": float64(next)})
	if tail["output"] != "" || tail["more"] != false {
		t.Fatalf("reading past the end returned data: %#v", tail)
	}
}

func TestCancelJobAndMaxSecondsStopCommands(t *testing.T) {
	s := newJobTestServer(t)
	res := callMap(t, s, "execute_command", map[string]any{"command": "sleep 60", "wait_seconds": 0.1})
	start := time.Now()
	stopped := callMap(t, s, "cancel_job", map[string]any{"job_id": res["job_id"]})
	if stopped["status"] != "stopped" || stopped["stopped_by"] != "cancel_job" || time.Since(start) > 5*time.Second {
		t.Fatalf("cancel_job did not stop the job: %#v", stopped)
	}
	timed := callMap(t, s, "execute_command", map[string]any{"command": "sleep 60", "max_seconds": 0.5, "wait_seconds": 10})
	if timed["status"] != "stopped" || timed["stopped_by"] != "max_seconds" {
		t.Fatalf("max_seconds did not stop the job: %#v", timed)
	}
}

func TestLongOutputIsExcerptedInline(t *testing.T) {
	s := newJobTestServer(t)
	res := callMap(t, s, "execute_command", map[string]any{"command": "i=0; while [ $i -lt 3000 ]; do echo line-$i-xxxxxxxxxxxxxxxxxxxx; i=$((i+1)); done"})
	out := res["output"].(string)
	if res["output_truncated"] != true || len(out) > agentInlineLimit+200 {
		t.Fatalf("output was not excerpted: %d bytes, %#v", len(out), res["output_truncated"])
	}
	if !strings.Contains(out, "line-0-") || !strings.Contains(out, "line-2999-") || !strings.Contains(out, "bytes omitted") {
		t.Fatalf("excerpt lost its start or end")
	}
}

func TestSessionsKeepStateAndStopOnShutdown(t *testing.T) {
	s := newJobTestServer(t)
	start := callMap(t, s, "session_start", map[string]any{"name": "shell", "command": "cd /tmp && MARK=rfswift", "expect": `\$ $`})
	if start["status"] != "running" {
		t.Fatalf("session did not start: %#v", start)
	}
	res := callMap(t, s, "session_send", map[string]any{"name": "shell", "input": "echo $MARK in $(pwd)", "expect": "rfswift in /tmp", "wait_seconds": 5})
	if res["expect_matched"] != true || !strings.Contains(res["output"].(string), "rfswift in /tmp") {
		t.Fatalf("session state not kept: %#v", res)
	}
	again := callMap(t, s, "session_send", map[string]any{"name": "shell", "input": "", "enter": false, "wait_seconds": 0.3})
	if strings.Contains(again["output"].(string), "rfswift in /tmp") {
		t.Fatalf("output was returned twice: %q", again["output"])
	}
	if _, err := s.call("session_start", map[string]any{"name": "shell"}); err == nil {
		t.Fatal("a second session with the same name was opened")
	}
	if _, err := s.call("session_start", map[string]any{"name": "Bad Name"}); err == nil {
		t.Fatal("an invalid session name was accepted")
	}
	job := callMap(t, s, "execute_command", map[string]any{"command": "sleep 60", "wait_seconds": 0.1})
	s.registry().stopAll("agent disconnected")
	list := callMap(t, s, "list_jobs", map[string]any{})
	for _, j := range list["jobs"].([]map[string]any) {
		if j["status"] == "running" {
			t.Fatalf("job still running after shutdown: %#v (job %v)", j, job["job_id"])
		}
	}
}

func TestExecuteCommandRefusesTransmissionWithoutPermission(t *testing.T) {
	s := newJobTestServer(t)
	_, err := s.call("execute_command", map[string]any{"command": "hackrf_transfer -t x.cs8 -f 433920000 -s 2000000"})
	if err == nil || !strings.Contains(err.Error(), "RF transmission is disabled") {
		t.Fatalf("transmission was not refused: %v", err)
	}
	if list := callMap(t, s, "list_jobs", map[string]any{}); len(list["jobs"].([]map[string]any)) != 0 {
		t.Fatal("a refused command was started")
	}
}
