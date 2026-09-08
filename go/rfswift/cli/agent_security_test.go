package cli

import (
	"strings"
	"testing"
)

func TestCappedAgentOutputBoundsMemory(t *testing.T) {
	w := &cappedAgentOutput{}
	payload := []byte(strings.Repeat("x", maxAgentCommandOutput+1024))
	n, err := w.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("writer must consume subprocess output: n=%d err=%v", n, err)
	}
	got := w.String()
	if len(got) > maxAgentCommandOutput+128 {
		t.Fatalf("bounded output grew unexpectedly: %d bytes", len(got))
	}
	if !strings.Contains(got, "output truncated") {
		t.Fatal("truncated output omitted its operator warning")
	}
}
