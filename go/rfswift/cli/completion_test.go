package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateCompletionWritesToProvidedWriter(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			var out bytes.Buffer
			if err := generateCompletion(shell, &out); err != nil {
				t.Fatal(err)
			}
			if out.Len() == 0 || !strings.Contains(out.String(), "rfswift") {
				t.Fatalf("completion output for %s is empty or invalid", shell)
			}
		})
	}
}

func TestGenerateCompletionRejectsUnknownShell(t *testing.T) {
	if err := generateCompletion("csh", &bytes.Buffer{}); err == nil {
		t.Fatal("expected unsupported shell error")
	}
}
