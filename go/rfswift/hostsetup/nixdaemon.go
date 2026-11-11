/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Host setup - the Nix daemon of a multi-user Nix install.
*
*  The Determinate Systems installer runs the daemon from
*  /usr/local/bin/determinate-nixd. Installed in a Qubes OS template, that
*  binary lands in the template's /usr/local, which no AppVM sees (an AppVM's
*  /usr/local is its own, from /rw/usrlocal): nix-daemon.service fails at
*  boot with 203/EXEC, the socket accepts a connection and drops it, and
*  every nix command that needs the store ends with "cannot open connection
*  to remote store 'daemon'". The same Nix install ships the standard daemon
*  in its default profile (/nix/var/nix/profiles/default/bin/nix-daemon),
*  which the AppVM does see; a systemd drop-in pointing the unit at it brings
*  the store back. `rfswift host qubes setup` applies it at every boot of an
*  AppVM, only when the unit's own binary is missing.
 */

package hostsetup

import (
	"os/exec"
	"regexp"
	"strings"
)

// NixProfileDaemon is the daemon shipped in the default Nix profile.
const NixProfileDaemon = "/nix/var/nix/profiles/default/bin/nix-daemon"

// NixDaemonDropIn is the drop-in RF Swift writes when the unit's binary is
// missing.
const NixDaemonDropIn = "/etc/systemd/system/nix-daemon.service.d/50-rfswift.conf"

// Overridable in tests.
var (
	nixProfileDaemon = NixProfileDaemon
	nixDaemonDropIn  = NixDaemonDropIn
	systemctlOutput  = func(args ...string) (string, error) {
		if _, err := exec.LookPath("systemctl"); err != nil {
			return "", err
		}
		out, err := exec.Command("systemctl", args...).Output()
		return strings.TrimSpace(string(out)), err
	}
)

// NixDaemonState describes the nix-daemon systemd unit.
type NixDaemonState struct {
	Unit        bool   `json:"unit"`        // nix-daemon.service exists (multi-user install)
	ExecStart   string `json:"execStart"`   // binary the unit runs
	ExecMissing bool   `json:"execMissing"` // that binary does not exist on this machine
	Active      string `json:"active"`      // systemctl is-active (active, failed, inactive, ...)
	Fallback    bool   `json:"fallback"`    // the default profile's nix-daemon exists
	DropIn      bool   `json:"dropIn"`      // RF Swift's drop-in is in place
}

var execStartPathRe = regexp.MustCompile(`path=([^ ;]+)`)

// parseExecStartPath extracts the binary from `systemctl show -p ExecStart
// --value` ("{ path=/usr/local/bin/determinate-nixd ; argv[]=... }").
func parseExecStartPath(s string) string {
	if m := execStartPathRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// GetNixDaemonState inspects the nix-daemon unit; never changes anything.
func GetNixDaemonState() NixDaemonState {
	var st NixDaemonState
	st.Fallback = pathExists(nixProfileDaemon)
	st.DropIn = pathExists(nixDaemonDropIn)
	load, err := systemctlOutput("show", "-p", "LoadState", "--value", "nix-daemon.service")
	if err != nil || load != "loaded" {
		return st
	}
	st.Unit = true
	exec, _ := systemctlOutput("show", "-p", "ExecStart", "--value", "nix-daemon.service")
	st.ExecStart = parseExecStartPath(exec)
	st.ExecMissing = st.ExecStart != "" && !pathExists(st.ExecStart)
	st.Active, _ = systemctlOutput("is-active", "nix-daemon.service")
	return st
}

// NixDaemonDropInContent points nix-daemon.service at the default profile's
// daemon.
func NixDaemonDropInContent() string {
	return "# Written by 'rfswift host qubes setup': the unit's own daemon binary is not\n" +
		"# in this qube (installed in the template's /usr/local), run Nix's own.\n" +
		"[Service]\n" +
		"ExecStart=\n" +
		"ExecStart=@" + nixProfileDaemon + " nix-daemon --daemon\n"
}

// nixDaemonBootScript re-applies the drop-in at boot, only when the unit's
// binary is missing (in a TemplateVM or StandaloneVM it is there and nothing
// changes). Idempotent; no set -e.
func nixDaemonBootScript() string {
	var b strings.Builder
	b.WriteString("# Nix daemon: Determinate Nix installed in the template runs\n")
	b.WriteString("# /usr/local/bin/determinate-nixd, but an AppVM's /usr/local is its own.\n")
	b.WriteString("# When the unit's binary is missing, run the daemon of the Nix profile.\n")
	b.WriteString("if [ -x " + nixProfileDaemon + " ] && systemctl cat nix-daemon.service >/dev/null 2>&1; then\n")
	b.WriteString("  nixd=$(systemctl show -p ExecStart --value nix-daemon.service | sed -n 's/.*path=\\([^ ;]*\\).*/\\1/p')\n")
	b.WriteString("  if [ -n \"$nixd\" ] && [ ! -e \"$nixd\" ]; then\n")
	b.WriteString("    mkdir -p " + dirOf(nixDaemonDropIn) + "\n")
	b.WriteString("    cat > " + nixDaemonDropIn + " <<'RFSWIFT_EOF'\n" + NixDaemonDropInContent() + "RFSWIFT_EOF\n")
	b.WriteString("    systemctl daemon-reload\n")
	b.WriteString("    systemctl reset-failed nix-daemon.socket nix-daemon.service 2>/dev/null\n")
	b.WriteString("    systemctl restart nix-daemon.socket 2>/dev/null\n")
	b.WriteString("    systemctl restart nix-daemon.service\n")
	b.WriteString("  fi\n")
	b.WriteString("fi\n")
	return b.String()
}

func dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return "/"
}

// NixDaemonHint says how to bring back a Nix store that does not answer.
func NixDaemonHint() string {
	st := GetNixDaemonState()
	switch {
	case st.Unit && st.ExecMissing && IsQubes():
		return "nix-daemon.service runs " + st.ExecStart + ", which this qube does not have (Nix was installed in the template, and an AppVM's /usr/local is its own): run 'rfswift host qubes setup'"
	case st.Unit && st.ExecMissing && st.Fallback:
		return "nix-daemon.service runs " + st.ExecStart + ", which is missing: reinstall Nix, or point the unit at " + nixProfileDaemon + " (systemctl edit nix-daemon.service)"
	case st.Unit && st.Active != "active":
		return "the Nix daemon is " + st.Active + ": sudo systemctl restart nix-daemon.service (see journalctl -u nix-daemon)"
	}
	return "check the Nix daemon: sudo systemctl status nix-daemon.service"
}
