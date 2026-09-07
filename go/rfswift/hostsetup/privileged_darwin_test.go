/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package hostsetup

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestAdminShellScriptQuoting(t *testing.T) {
	got := adminShellScript([]string{"/nix/bin/nix", "copy", "--from", "file:///tmp/a b?x=1", `it's "q" \ end`})
	want := `do shell script "'/nix/bin/nix' 'copy' '--from' 'file:///tmp/a b?x=1' 'it'\\''s \"q\" \\ end'" with administrator privileges`
	if got != want {
		t.Fatalf("adminShellScript =\n%s\nwant\n%s", got, want)
	}
}

// The unelevated form of the script must hand every argument to the program
// unchanged, whatever shell and AppleScript metacharacters it holds.
func TestShellScriptSourceRoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is macOS only")
	}
	args := []string{"plain", "with space", "it's", `dq"uote`, `back\slash`, "$HOME `id`", "file:///tmp/x?compression=none", "--extra-experimental-features", "nix-command flakes"}
	out, err := exec.Command(osascriptBinary, "-e", shellScriptSource(append([]string{"/usr/bin/printf", `%s\n`}, args...))).Output()
	if err != nil {
		t.Fatalf("osascript: %v", err)
	}
	// osascript reports "\r" line breaks and drops the final one.
	got := strings.Split(strings.ReplaceAll(strings.TrimRight(string(out), "\r\n"), "\r", "\n"), "\n")
	if strings.Join(got, "\x00") != strings.Join(args, "\x00") {
		t.Fatalf("arguments changed through osascript:\n got %q\nwant %q", got, args)
	}
}
