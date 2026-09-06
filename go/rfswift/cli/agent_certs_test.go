package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

type cobraCommandFinder struct{ cmds []*cobra.Command }

func (f *cobraCommandFinder) find(name string) *cobra.Command {
	for _, c := range f.cmds {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestAgentCertsSubcommandsAreRegistered(t *testing.T) {
	var certs *cobraCommandFinder
	for _, c := range agentCmd.Commands() {
		if c.Name() == "certs" {
			certs = &cobraCommandFinder{c.Commands()}
		}
	}
	if certs == nil {
		t.Fatal("agent has no certs command")
	}
	want := map[string][]string{
		"init":   {"dir", "name", "host"},
		"client": {"bundle", "name", "endpoint", "out", "passphrase-file"},
		"export": {"bundle", "out", "passphrase-file"},
		"import": {"dir", "passphrase-file"},
	}
	for name, flags := range want {
		c := certs.find(name)
		if c == nil {
			t.Errorf("certs has no %s subcommand", name)
			continue
		}
		for _, f := range flags {
			if c.Flags().Lookup(f) == nil {
				t.Errorf("certs %s has no --%s flag", name, f)
			}
		}
	}
}

func TestReadPassphraseFromFileTrimsLineEnding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pass")
	if err := os.WriteFile(path, []byte("correct horse battery staple\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readPassphrase(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "correct horse battery staple" {
		t.Errorf("passphrase = %q", got)
	}
	if _, err := readPassphrase(filepath.Join(t.TempDir(), "missing"), false); err == nil {
		t.Error("a missing passphrase file must be an error")
	}
}

func TestFileSlug(t *testing.T) {
	if got := fileSlug("Alice's laptop (2026)"); got != "Alice-s-laptop--2026-" {
		t.Errorf("slug = %q", got)
	}
}
