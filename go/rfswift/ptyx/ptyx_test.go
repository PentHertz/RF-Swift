package ptyx

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestStartEchoes runs a trivial shell under a PTY on the host platform and
// checks that output flows back, that the process exit ends the stream, and
// that Resize is accepted.
func TestStartEchoes(t *testing.T) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/c", "echo PTYX-MARKER-OK")
	} else {
		cmd = exec.Command("/bin/sh", "-c", "echo PTYX-MARKER-OK")
	}
	term, err := Start(cmd, 100, 30)
	if errors.Is(err, ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	if term.Pid() <= 0 {
		t.Fatal("pid not reported")
	}
	if err := term.Resize(120, 40); err != nil {
		t.Fatalf("resize: %v", err)
	}
	var out strings.Builder
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := term.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("terminal did not end after the process exited; output so far: %q", out.String())
	}
	if !strings.Contains(out.String(), "PTYX-MARKER-OK") {
		t.Fatalf("marker missing from terminal output: %q", out.String())
	}
	if err := term.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestStartRejectsNilCommand(t *testing.T) {
	if _, err := Start(nil, 80, 24); err == nil {
		t.Fatal("nil command accepted")
	}
}
