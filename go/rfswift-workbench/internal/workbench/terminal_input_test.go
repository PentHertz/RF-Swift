package workbench

import (
	"bytes"
	"testing"
)

func TestTerminalColorReplyFilter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"background ST", "\x1b]11;rgb:0707/1111/0f0f\x1b\\", ""},
		{"foreground BEL", "\x1b]10;rgb:cccc/e4e4/dfdf\x07", ""},
		{"cursor mixed with key", "\x1b]12;rgb:5454/d6d6/c8c8\x1b\\ls\r", "ls\r"},
		{"ordinary input", "printf '\x1b[31mred\x1b[0m'\r", "printf '\x1b[31mred\x1b[0m'\r"},
		{"bracketed paste", "\x1b[200~hello\x1b[201~", "\x1b[200~hello\x1b[201~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := terminalColorReply.ReplaceAllString(tt.in, ""); got != tt.want {
				t.Fatalf("filtered input = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLocalTerminalInputKeepsColorReply(t *testing.T) {
	pty := &promptTestPTY{}
	a := &App{terminals: map[string]*terminalSession{"local": {id: "local", conn: pty}}}
	reply := "\x1b]11;rgb:0707/1111/0f0f\x1b\\"
	if err := a.TerminalInput("local", reply); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pty.Bytes(), []byte(reply)) {
		t.Fatalf("local input changed: %q", pty.String())
	}
}
