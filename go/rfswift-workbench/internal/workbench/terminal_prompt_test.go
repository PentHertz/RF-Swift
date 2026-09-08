package workbench

import (
	"bytes"
	"io"
	"testing"
)

type promptTestPTY struct{ bytes.Buffer }

func (p *promptTestPTY) Close() error { return nil }

var _ io.ReadWriteCloser = (*promptTestPTY)(nil)

func TestSubmitAgentPromptUsesBracketedPasteAndSeparateEnter(t *testing.T) {
	pty := &promptTestPTY{}
	a := &App{terminals: map[string]*terminalSession{
		"agent-1": {id: "agent-1", agent: true, conn: pty},
	}}
	if err := a.SubmitAgentPrompt("agent-1", "review evidence\nthen list findings"); err != nil {
		t.Fatal(err)
	}
	want := "\x1b[200~review evidence\nthen list findings\x1b[201~\r"
	if got := pty.String(); got != want {
		t.Fatalf("PTY input = %q, want %q", got, want)
	}
}

func TestSubmitAgentPromptRejectsOrdinaryTerminal(t *testing.T) {
	pty := &promptTestPTY{}
	a := &App{terminals: map[string]*terminalSession{
		"shell-1": {id: "shell-1", conn: pty},
	}}
	if err := a.SubmitAgentPrompt("shell-1", "whoami"); err == nil {
		t.Fatal("ordinary terminal accepted an agent prompt")
	}
	if pty.Len() != 0 {
		t.Fatalf("ordinary terminal received %q", pty.String())
	}
}
