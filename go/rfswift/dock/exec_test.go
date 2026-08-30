package dock

import (
	"errors"
	"strings"
	"testing"
)

func TestTailWriterKeepsOnlyTheEnd(t *testing.T) {
	w := &tailWriter{max: 8}
	for _, chunk := range []string{"abcdef", "ghij", "kl\n"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if got := w.String(); got != "fghijkl" {
		t.Fatalf("tail = %q", got)
	}
}

func TestExecExitErrorCarriesStatusAndTail(t *testing.T) {
	var err error = &execExitError{Cmd: "/bin/bash -c ./entrypoint.sh sdrpp_soft_fromsource_install", ExitCode: 1, Tail: "CMake Error: libhydrasdr not found"}
	var exitErr *execExitError
	if !errors.As(err, &exitErr) {
		t.Fatal("errors.As must recognise the exit error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "exited with status 1") || !strings.Contains(msg, "libhydrasdr not found") {
		t.Fatalf("message = %q", msg)
	}
}
