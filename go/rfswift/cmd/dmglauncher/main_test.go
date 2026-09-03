// This code is part of RF Swift by @Penthertz
// Author(s): Sébastien Dudek (@FlUxIuS)

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/Volumes/RF Swift v4.0.0": `'/Volumes/RF Swift v4.0.0'`,
		"plain":                    `'plain'`,
		"it's":                     `'it'\''s'`,
		`back\slash and "double"`:  `'back\slash and "double"'`,
		"$HOME/`whoami`/$(id)":     `'$HOME/` + "`whoami`" + `/$(id)'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestTrampolineShape(t *testing.T) {
	got := trampoline("/Volumes/RF Swift v4.0.0", "/Volumes/RF Swift v4.0.0/RF Swift Setup.app/Contents/Resources/main.sh")
	for _, want := range []string{
		"#!/bin/bash\n",
		`rm -f -- "$0"`,
		"export RFSWIFT_DMG_ROOT='/Volumes/RF Swift v4.0.0'\n",
		"cd '/Volumes/RF Swift v4.0.0' || exit 1\n",
		"exec /bin/bash '/Volumes/RF Swift v4.0.0/RF Swift Setup.app/Contents/Resources/main.sh'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("trampoline lacks %q:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, "#!/bin/bash\n") {
		t.Errorf("trampoline must start with the bash shebang:\n%s", got)
	}
}

// TestTrampolineRuns executes a generated trampoline with bash the way
// Terminal does and checks that the bundled script sees the image root as
// RFSWIFT_DMG_ROOT and as its working directory, with the trampoline gone.
func TestTrampolineRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	root := filepath.Join(t.TempDir(), "RF Swift v4.0.0 (it's a test)")
	script := filepath.Join(root, "RF Swift Setup.app", "Contents", "Resources", scriptName)
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("printf '%s|%s|%s\\n' \"$RFSWIFT_DMG_ROOT\" \"$PWD\" \"$0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	tramp := filepath.Join(tmp, "RF Swift Setup.command")
	if err := os.WriteFile(tramp, []byte(trampoline(root, script)), 0o700); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(bash, tramp).CombinedOutput()
	if err != nil {
		t.Fatalf("trampoline failed: %v\n%s", err, out)
	}
	want := root + "|" + root + "|" + script + "\n"
	if string(out) != want {
		t.Errorf("script saw %q, want %q", out, want)
	}
	if _, err := os.Stat(tramp); !os.IsNotExist(err) {
		t.Errorf("trampoline should remove itself, stat err = %v", err)
	}
}

func TestAppleScriptString(t *testing.T) {
	got := appleScriptString(`say "hi" \ bye`)
	want := `"say \"hi\" \\ bye"`
	if got != want {
		t.Errorf("appleScriptString = %s, want %s", got, want)
	}
}
