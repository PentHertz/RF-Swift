//go:build !windows

package workbench

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// mcpPipe drives serve() over real pipes, the way a coding-agent CLI does.
type mcpPipe struct {
	t     *testing.T
	in    *io.PipeWriter
	lines chan map[string]any
	done  chan error
}

func startMCPPipe(t *testing.T, s *mcpServer) *mcpPipe {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	p := &mcpPipe{t: t, in: inW, lines: make(chan map[string]any, 64), done: make(chan error, 1)}
	go func() {
		p.done <- s.serve(inR, outW)
		_ = outW.Close()
	}()
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 64*1024), 4<<20)
		for sc.Scan() {
			var m map[string]any
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				p.lines <- m
			}
		}
		close(p.lines)
	}()
	return p
}

func (p *mcpPipe) send(v map[string]any) {
	p.t.Helper()
	v["jsonrpc"] = "2.0"
	b, _ := json.Marshal(v)
	if _, err := p.in.Write(append(b, '\n')); err != nil {
		p.t.Fatal(err)
	}
}

// expect waits for the response with the given id; any other response seen
// meanwhile is returned in others.
func (p *mcpPipe) expect(id float64, within time.Duration) (map[string]any, []map[string]any) {
	p.t.Helper()
	var others []map[string]any
	deadline := time.After(within)
	for {
		select {
		case m, ok := <-p.lines:
			if !ok {
				p.t.Fatalf("transport closed before response %v", id)
			}
			if m["id"] == id {
				return m, others
			}
			others = append(others, m)
		case <-deadline:
			p.t.Fatalf("no response %v within %s (others: %v)", id, within, others)
		}
	}
}

func toolText(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	result, _ := resp["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("no tool content: %#v", resp)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("tool text is not JSON: %q", text)
	}
	return m
}

func TestMCPStdioSmoke(t *testing.T) {
	s := newJobTestServer(t)
	p := startMCPPipe(t, s)
	p.send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}})
	if init, _ := p.expect(1, 5*time.Second); init["result"].(map[string]any)["serverInfo"] == nil {
		t.Fatalf("bad initialize response: %#v", init)
	}
	p.send(map[string]any{"method": "notifications/initialized"})
	p.send(map[string]any{"id": 2, "method": "tools/list"})
	list, _ := p.expect(2, 5*time.Second)
	if tools := list["result"].(map[string]any)["tools"].([]any); len(tools) < 15 {
		t.Fatalf("only %d tools listed", len(tools))
	}
	p.send(map[string]any{"id": 3, "method": "tools/call", "params": map[string]any{"name": "execute_command", "arguments": map[string]any{"command": "echo stdio-ok"}}})
	res, _ := p.expect(3, 10*time.Second)
	if out := toolText(t, res)["output"]; !strings.Contains(out.(string), "stdio-ok") {
		t.Fatalf("command output missing: %#v", res)
	}
	_ = p.in.Close()
	select {
	case err := <-p.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not return after the input closed")
	}
}

func TestMCPCancellationStopsTheCallAndOthersKeepFlowing(t *testing.T) {
	s := newJobTestServer(t)
	p := startMCPPipe(t, s)
	p.send(map[string]any{"id": 10, "method": "tools/call", "params": map[string]any{"name": "execute_command", "arguments": map[string]any{"command": "sleep 60", "wait_seconds": 60}}})
	time.Sleep(300 * time.Millisecond)
	// While call 10 waits, another call is answered.
	start := time.Now()
	p.send(map[string]any{"id": 11, "method": "tools/call", "params": map[string]any{"name": "list_jobs", "arguments": map[string]any{}}})
	listed, _ := p.expect(11, 5*time.Second)
	if time.Since(start) > 3*time.Second {
		t.Fatal("list_jobs waited for the running execute_command")
	}
	jobs := toolText(t, listed)["jobs"].([]any)
	if len(jobs) != 1 || jobs[0].(map[string]any)["status"] != "running" {
		t.Fatalf("expected one running job: %#v", jobs)
	}
	// The client cancels call 10: the command stops and no response is sent.
	p.send(map[string]any{"method": "notifications/cancelled", "params": map[string]any{"requestId": 10, "reason": "user pressed Esc"}})
	var after map[string]any
	var others []map[string]any
	for i := 0; i < 50; i++ {
		time.Sleep(200 * time.Millisecond)
		p.send(map[string]any{"id": 100 + i, "method": "tools/call", "params": map[string]any{"name": "list_jobs", "arguments": map[string]any{}}})
		var o []map[string]any
		after, o = p.expect(float64(100+i), 5*time.Second)
		others = append(others, o...)
		if j := toolText(t, after)["jobs"].([]any)[0].(map[string]any); j["status"] != "running" {
			if j["stopped_by"] != "cancelled by the client" {
				t.Fatalf("job ended for another reason: %#v", j)
			}
			break
		}
	}
	if j := toolText(t, after)["jobs"].([]any)[0].(map[string]any); j["status"] == "running" {
		t.Fatal("the cancelled command is still running")
	}
	for _, o := range others {
		if o["id"] == float64(10) {
			t.Fatalf("a response was sent for the cancelled call: %#v", o)
		}
	}
	// Ending the transport stops whatever is still running.
	p.send(map[string]any{"id": 20, "method": "tools/call", "params": map[string]any{"name": "execute_command", "arguments": map[string]any{"command": "sleep 60", "wait_seconds": 0}}})
	p.expect(20, 5*time.Second)
	_ = p.in.Close()
	select {
	case <-p.done:
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not return after the input closed")
	}
	for _, j := range s.registry().jobs {
		if j.isRunning() {
			t.Fatalf("job %s still running after the transport closed", j.id)
		}
	}
}
