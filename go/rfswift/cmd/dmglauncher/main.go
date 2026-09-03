// This code is part of RF Swift by @Penthertz
// Author(s): Sébastien Dudek (@FlUxIuS)

// Command dmglauncher is the executable behind the two double-clickable
// helpers shipped in the macOS disk image, "Install RF Swift CLI.app" and
// "RF Swift Setup.app" (built by scripts/macos-launcher-app.sh, wired in
// .github/workflows/macos-dmg.yml).
//
// Since macOS 15 Gatekeeper assesses whatever a user double-clicks out of a
// downloaded (quarantined) image, and it only accepts code that a
// notarization ticket lists. Apple's notary service records Mach-O
// executables and nothing else: a bare .command script, even one carrying a
// Developer ID signature, stays "Unnotarized Developer ID" and is refused
// with "Apple could not verify ... is free of malware". The supported way
// out is an app bundle: this small signed binary is what the image's
// notarization lists, so Gatekeeper accepts the app, and the script rides
// along as a sealed bundle resource (Contents/Resources/main.sh).
//
// The launcher opens Terminal on a tiny trampoline written to a private
// temporary directory instead of on the bundled script itself: every file
// on a quarantined image is quarantined too, and Terminal would hand it to
// the same Gatekeeper check. The trampoline execs /bin/bash on the script,
// and bash reading a file is not a Launch Services "open", so nothing is
// assessed. Terminal is driven through `open`, not AppleScript, so macOS
// asks for no "wants to control Terminal" automation consent either.
//
// The image root (the directory holding the .app, where the rfswift binary
// sits) is exported as RFSWIFT_DMG_ROOT and made the working directory, so
// the scripts find the CLI to install without a copy of it in each bundle.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// scriptName is the bundled script, sealed by the bundle's code signature.
const scriptName = "main.sh"

func main() {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		fail("Cannot locate the launcher executable: " + err.Error())
	}

	// <name>.app/Contents/MacOS/<exe>
	contents := filepath.Dir(filepath.Dir(exe))
	bundle := filepath.Dir(contents)
	script := filepath.Join(contents, "Resources", scriptName)
	if st, err := os.Stat(script); err != nil || !st.Mode().IsRegular() {
		fail(fmt.Sprintf("The bundled script is missing (%s). The app bundle looks damaged; download the disk image again.", script))
	}

	root := filepath.Dir(bundle)
	name := strings.TrimSuffix(filepath.Base(bundle), ".app")

	dir, err := os.MkdirTemp("", "rfswift-launcher-")
	if err != nil {
		fail("Cannot create a temporary directory: " + err.Error())
	}
	tramp := filepath.Join(dir, name+".command")
	if err := os.WriteFile(tramp, []byte(trampoline(root, script)), 0o700); err != nil {
		fail("Cannot write the Terminal launcher: " + err.Error())
	}

	out, err := exec.Command("/usr/bin/open", "-a", "Terminal", tramp).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		fail("Could not open Terminal: " + msg)
	}
}

// trampoline is the .command Terminal runs: it removes itself, moves to the
// image root and hands over to bash on the bundled script.
func trampoline(root, script string) string {
	return "#!/bin/bash\n" +
		"rm -f -- \"$0\"; rmdir -- \"$(dirname -- \"$0\")\" 2>/dev/null\n" +
		"export RFSWIFT_DMG_ROOT=" + shellQuote(root) + "\n" +
		"cd " + shellQuote(root) + " || exit 1\n" +
		"exec /bin/bash " + shellQuote(script) + "\n"
}

// shellQuote wraps s in single quotes for a POSIX shell, so a volume name
// with spaces or quotes ("RF Swift v4.0.0") survives verbatim.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// fail reports msg to the user and exits. Launched from the Finder there is
// no terminal behind stderr, so an alert is the only channel that reaches
// them; stderr is kept for a launch from a shell.
func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	_ = exec.Command("/usr/bin/osascript", "-e",
		"display alert \"RF Swift\" message "+appleScriptString(msg)+" as critical").Run()
	os.Exit(1)
}

// appleScriptString renders s as an AppleScript string literal.
func appleScriptString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
