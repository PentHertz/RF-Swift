/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  WSL 2 as the host of the Nix engine on Windows.
*
*  Nix does not run natively on Windows. RF Swift therefore runs the Nix engine
*  inside a WSL 2 distribution: the Linux rfswift CLI and the nix daemon live in
*  the distro, the Windows rfswift.exe and the Workbench drive them through
*  wsl.exe, and the environment state is read back over the \\wsl.localhost\
*  share. This file holds the distribution-level plumbing both the nix package
*  and the doctor use: choosing a distribution, probing what it offers (nix,
*  rfswift, systemd, WSLg, GPU libraries, forwarded USB devices), running a
*  command in it through a login shell, and translating paths between the two
*  worlds. Nothing here is Windows-specific at compile time so the helpers can
*  be unit-tested everywhere; wsl.exe is simply absent elsewhere.
 */

package rfutils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// WSLNixStatus is what a WSL 2 distribution offers the Nix engine, as
// reported by one probe run inside it.
type WSLNixStatus struct {
	Distro         string `json:"distro"`
	User           string `json:"user"`           // Linux account wsl.exe runs commands as
	Home           string `json:"home"`           // its home directory (Linux path)
	Shell          string `json:"shell"`          // its login shell
	NixVersion     string `json:"nixVersion"`     // "nix (Nix) 2.x" or "" when nix is missing
	RFSwiftVersion string `json:"rfswiftVersion"` // Linux rfswift version, "unknown" when it predates --version, "" when missing
	RFSwiftPath    string `json:"rfswiftPath"`    // where the Linux rfswift was found
	Systemd        bool   `json:"systemd"`        // PID 1 is systemd (nix-daemon runs as a service)
	X11            bool   `json:"x11"`            // WSLg X11 socket present
	Audio          bool   `json:"audio"`          // WSLg PulseAudio socket present
	GPULibs        bool   `json:"gpuLibs"`        // /usr/lib/wsl/lib present (WSLg D3D12/DXCore user-space)
	Bubblewrap     bool   `json:"bubblewrap"`     // bwrap available for --isolate
	USBDevices     int    `json:"usbDevices"`     // devices under /dev/bus/usb (forwarded with usbipd)
	Kernel         string `json:"kernel"`         // /proc/sys/kernel/osrelease
}

// HasNix reports whether nix is on the login PATH of the distribution.
func (s WSLNixStatus) HasNix() bool { return s.NixVersion != "" }

// HasRFSwift reports whether the Linux rfswift CLI is on the login PATH.
func (s WSLNixStatus) HasRFSwift() bool { return s.RFSwiftVersion != "" }

// Ready reports whether the distribution can serve the Nix engine: both nix
// and the Linux rfswift are present.
func (s WSLNixStatus) Ready() bool { return s.HasNix() && s.HasRFSwift() }

// Missing names what a not-ready distribution lacks, for messages.
func (s WSLNixStatus) Missing() []string {
	var missing []string
	if !s.HasNix() {
		missing = append(missing, "nix")
	}
	if !s.HasRFSwift() {
		missing = append(missing, "the Linux rfswift CLI")
	}
	return missing
}

// ErrNoWSLNixDistro is returned when no WSL 2 distribution can host the Nix
// engine (none installed, or only the container engines' utility VMs).
var ErrNoWSLNixDistro = errors.New("no WSL 2 Linux distribution found to host the Nix engine (wsl --install -d Ubuntu, then: rfswift nix wsl setup)")

// wslNixProbeScript is run once per distribution with a login shell, so PATH
// is what the user's own terminal would have (nix.sh from /etc/profile.d).
// Every line is key=value; unknown keys are ignored by the parser.
const wslNixProbeScript = `printf 'user=%s\n' "$(id -un 2>/dev/null)"
printf 'home=%s\n' "$HOME"
printf 'shell=%s\n' "$SHELL"
if command -v nix >/dev/null 2>&1; then printf 'nix=%s\n' "$(nix --version 2>/dev/null | head -n 1)"; fi
if rf=$(command -v rfswift 2>/dev/null); then
  printf 'rfswift_path=%s\n' "$rf"
  v=$("$rf" --version 2>/dev/null </dev/null | grep -m 1 -E '(^| )version ' ) || v=""
  printf 'rfswift=%s\n' "${v:-unknown}"
fi
printf 'init=%s\n' "$(cat /proc/1/comm 2>/dev/null)"
[ -d /mnt/wslg/.X11-unix ] && echo x11=1
[ -S /mnt/wslg/PulseServer ] && echo audio=1
[ -d /usr/lib/wsl/lib ] && echo gpulibs=1
command -v bwrap >/dev/null 2>&1 && echo bwrap=1
printf 'usb=%s\n' "$(ls -1 /dev/bus/usb/*/* 2>/dev/null | wc -l)"
printf 'kernel=%s\n' "$(cat /proc/sys/kernel/osrelease 2>/dev/null)"
echo probe=ok
`

// WSLExePath locates wsl.exe: PATH first, then System32 (a GUI started with a
// minimal PATH still finds it).
//
//	out(1): string absolute path of wsl.exe
//	out(2): error when WSL is not installed
func WSLExePath() (string, error) {
	if p, err := exec.LookPath("wsl.exe"); err == nil {
		return p, nil
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		p := filepath.Join(root, "System32", "wsl.exe")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("wsl.exe not found: WSL 2 is required for the Nix engine on Windows (wsl --install)")
}

// wslLoginArgs builds the wsl.exe argument vector that runs argv inside distro
// through a POSIX login shell. The login shell matters: nix is put on PATH by
// /etc/profile.d/nix.sh, which a plain `wsl.exe -- cmd` (non-login) never
// reads. `-e` bypasses the user's shell so argv reaches the program verbatim,
// with no re-parsing; the tiny sh wrapper only turns the login environment on.
//
//	in(1): string distro distribution name ("" = default)
//	in(2): string user Linux account ("" = the distribution's default user)
//	in(3): []string argv program and arguments, as the Linux side should see them
func wslLoginArgs(distro, user string, argv []string) []string {
	args := []string{}
	if distro != "" {
		args = append(args, "-d", distro)
	}
	if user != "" {
		args = append(args, "-u", user)
	}
	args = append(args, "-e", "sh", "-l", "-c", `exec "$0" "$@"`)
	return append(args, argv...)
}

// WSLExec prepares (does not start) argv inside distro as its default user,
// with a login environment. Callers wire stdio and environment.
func WSLExec(distro string, argv ...string) *exec.Cmd {
	return WSLExecAs(distro, "", argv...)
}

// WSLExecAs is WSLExec running as a specific Linux user (e.g. root for
// provisioning). When wsl.exe cannot be found the returned command fails at
// Start with a clear error instead of a bare "file not found".
func WSLExecAs(distro, user string, argv ...string) *exec.Cmd {
	wsl, err := WSLExePath()
	if err != nil {
		cmd := exec.Command("wsl.exe")
		cmd.Err = err
		return cmd
	}
	return exec.Command(wsl, wslLoginArgs(distro, user, argv)...)
}

// ProbeWSLNix asks a WSL 2 distribution what it offers the Nix engine. It
// starts the distribution when needed, which is why a generous timeout is
// used; the console window is hidden so a GUI caller does not flash one.
//
//	in(1): string distro distribution name ("" = default)
//	out(1): WSLNixStatus
//	out(2): error when wsl.exe is missing, the distribution does not exist or
//	        the probe did not run
func ProbeWSLNix(distro string) (WSLNixStatus, error) {
	wsl, err := WSLExePath()
	if err != nil {
		return WSLNixStatus{Distro: distro}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, wsl, wslLoginArgs(distro, "", []string{"sh", "-c", wslNixProbeScript})...)
	hideConsoleWindow(cmd)
	raw, err := cmd.CombinedOutput()
	text := DecodeConsoleOutput(raw)
	status := ParseWSLNixProbe(distro, text)
	if !strings.Contains(text, "probe=ok") {
		msg := strings.TrimSpace(text)
		if msg == "" && err != nil {
			msg = err.Error()
		}
		if msg == "" {
			msg = "no answer"
		}
		name := distro
		if name == "" {
			name = "the default distribution"
		}
		return status, fmt.Errorf("could not probe WSL distribution %s: %s", name, firstLine(msg))
	}
	return status, nil
}

// ParseWSLNixProbe decodes the key=value lines printed by the probe script.
//
//	in(1): string distro the distribution the text came from
//	in(2): string text probe output
//	out: WSLNixStatus
func ParseWSLNixProbe(distro, text string) WSLNixStatus {
	status := WSLNixStatus{Distro: distro}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "user":
			status.User = value
		case "home":
			status.Home = value
		case "shell":
			status.Shell = value
		case "nix":
			status.NixVersion = value
		case "rfswift_path":
			status.RFSwiftPath = value
		case "rfswift":
			status.RFSwiftVersion = rfswiftVersionFromBanner(value)
		case "init":
			status.Systemd = value == "systemd"
		case "x11":
			status.X11 = value == "1"
		case "audio":
			status.Audio = value == "1"
		case "gpulibs":
			status.GPULibs = value == "1"
		case "bwrap":
			status.Bubblewrap = value == "1"
		case "usb":
			fmt.Sscanf(value, "%d", &status.USBDevices)
		case "kernel":
			status.Kernel = value
		}
	}
	return status
}

var (
	ansiSequenceRe    = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")
	versionTokenRe    = regexp.MustCompile(`^v?[0-9]+\.[0-9]+(\.[0-9]+)?([-+][0-9A-Za-z.-]+)?$`)
	rfswiftBannerWord = "version"
)

// rfswiftVersionFromBanner reduces "rfswift version 4.0.1-dev" to "4.0.1-dev".
// Anything that is not a version - an older rfswift without --version, or a
// first-run prompt the CLI printed instead - reads as "unknown", so a present
// but unidentifiable rfswift is never mistaken for an absent one.
func rfswiftVersionFromBanner(s string) string {
	s = strings.TrimSpace(ansiSequenceRe.ReplaceAllString(s, ""))
	if s == "" {
		return ""
	}
	fields := strings.Fields(s)
	for i, f := range fields {
		if f == rfswiftBannerWord && i+1 < len(fields) && versionTokenRe.MatchString(fields[i+1]) {
			return strings.TrimPrefix(fields[i+1], "v")
		}
	}
	if last := fields[len(fields)-1]; versionTokenRe.MatchString(last) {
		return strings.TrimPrefix(last, "v")
	}
	return "unknown"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// wslUtilityDistroPrefixes are the WSL distributions that belong to container
// engines and desktop tools, not to the user: they must never host the Nix
// engine (Docker Desktop's even lacks a package manager and a real user).
var wslUtilityDistroPrefixes = []string{"docker-desktop", "podman-machine", "rancher-desktop"}

// IsWSLUtilityDistro reports whether a distribution name is one of the
// container engines' utility VMs rather than a Linux distribution for the user.
func IsWSLUtilityDistro(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, prefix := range wslUtilityDistroPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// WSLNixCandidates keeps the WSL 2 distributions that can host the Nix engine
// (no utility VMs, no WSL 1), the default distribution first, then the others
// in alphabetical order.
func WSLNixCandidates(status WSLStatus) []WSLDistro {
	var out []WSLDistro
	for _, d := range status.Distros {
		if d.Version != 2 || IsWSLUtilityDistro(d.Name) {
			continue
		}
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Default != out[j].Default {
			return out[i].Default
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// ResolveWSLNix picks the WSL 2 distribution that hosts the Nix engine and
// probes it. Precedence: RFSWIFT_WSL_DISTRO, then `[nix] wsl_distro` in the
// config file, then the installed candidates - the default distribution first
// - keeping the first one that is fully provisioned, else the first that has
// nix, else the first candidate (so the caller can offer to set it up).
//
//	in(1): string configFile RF Swift config.ini path
//	out(1): WSLNixStatus the chosen distribution and what it offers
//	out(2): error when WSL is missing or no distribution qualifies
func ResolveWSLNix(configFile string) (WSLNixStatus, error) {
	if d := strings.TrimSpace(os.Getenv("RFSWIFT_WSL_DISTRO")); d != "" {
		return ProbeWSLNix(d)
	}
	if d := ConfiguredNixWSLDistro(configFile); d != "" {
		return ProbeWSLNix(d)
	}
	list, err := WSLDistributions()
	if err != nil {
		return WSLNixStatus{}, err
	}
	candidates := WSLNixCandidates(list)
	if len(candidates) == 0 {
		return WSLNixStatus{}, ErrNoWSLNixDistro
	}
	var fallback *WSLNixStatus
	var firstErr error
	for _, c := range candidates {
		st, err := ProbeWSLNix(c.Name)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if st.Ready() {
			return st, nil
		}
		if fallback == nil || (st.HasNix() && !fallback.HasNix()) {
			copied := st
			fallback = &copied
		}
	}
	if fallback != nil {
		return *fallback, nil
	}
	return WSLNixStatus{Distro: candidates[0].Name}, firstErr
}

// WSLTerminate stops a distribution so the next start picks up /etc/wsl.conf
// changes (systemd, automount options).
func WSLTerminate(distro string) error {
	wsl, err := WSLExePath()
	if err != nil {
		return err
	}
	cmd := exec.Command(wsl, "--terminate", distro)
	hideConsoleWindow(cmd)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wsl --terminate %s: %s", distro, firstLine(strings.TrimSpace(DecodeConsoleOutput(raw))))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Paths: Windows <-> WSL
// ---------------------------------------------------------------------------

// wslUNCPrefixes are the shares through which Windows sees a distribution's
// root filesystem. wsl.localhost is the current name; wsl$ the older alias.
var wslUNCPrefixes = []string{`\\wsl.localhost\`, `\\wsl$\`, `//wsl.localhost/`, `//wsl$/`}

// SplitWSLUNC splits \\wsl.localhost\<distro>\a\b into ("<distro>", "/a/b").
//
//	out(3): bool false when path is not a WSL share path
func SplitWSLUNC(path string) (distro, linuxPath string, ok bool) {
	for _, prefix := range wslUNCPrefixes {
		if len(path) <= len(prefix) || !strings.EqualFold(path[:len(prefix)], prefix) {
			continue
		}
		rest := strings.ReplaceAll(path[len(prefix):], `\`, "/")
		distro, sub, _ := strings.Cut(rest, "/")
		if distro == "" {
			return "", "", false
		}
		return distro, "/" + strings.TrimLeft(sub, "/"), true
	}
	return "", "", false
}

// IsWindowsAbsPath reports whether s is a drive-letter absolute Windows path
// (C:\x, C:/x, or the bare root C:).
func IsWindowsAbsPath(s string) bool {
	if len(s) < 2 || s[1] != ':' {
		return false
	}
	c := s[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return false
	}
	return len(s) == 2 || s[2] == '\\' || s[2] == '/'
}

// WindowsPathToWSL translates a path the Windows side uses into the path the
// Linux side sees: a drive path through WSL's default /mnt/<drive> automount,
// a \\wsl.localhost share path into the distribution's own path. Linux
// absolute paths pass through unchanged. Anything else (relative paths,
// flake references, option values) is reported as not a Windows path.
//
//	out(2): bool true when the input was translated (or was already a Linux path)
func WindowsPathToWSL(path string) (string, bool) {
	p := strings.TrimSpace(path)
	if _, linux, ok := SplitWSLUNC(p); ok {
		return linux, true
	}
	if IsWindowsAbsPath(p) {
		rest := strings.ReplaceAll(p[2:], `\`, "/")
		rest = strings.TrimRight(rest, "/")
		return "/mnt/" + strings.ToLower(p[:1]) + rest, true
	}
	if strings.HasPrefix(p, "/") {
		return p, true
	}
	return path, false
}

var (
	wslUNCRootMu    sync.Mutex
	wslUNCRootCache = map[string]string{}
)

// WSLUNCRoot returns the share root through which Windows reads a
// distribution's filesystem, preferring \\wsl.localhost and falling back to
// the older \\wsl$ alias when the former is not served.
func WSLUNCRoot(distro string) string {
	wslUNCRootMu.Lock()
	defer wslUNCRootMu.Unlock()
	if root, ok := wslUNCRootCache[distro]; ok {
		return root
	}
	root := `\\wsl.localhost\` + distro
	if _, err := os.Stat(root); err != nil {
		alias := `\\wsl$\` + distro
		if _, err := os.Stat(alias); err == nil {
			root = alias
		}
	}
	wslUNCRootCache[distro] = root
	return root
}

// WSLPathToWindows translates a Linux path of distro into a path this Windows
// process can open: /mnt/<drive>/... back to the drive, everything else
// through the distribution's share.
func WSLPathToWindows(distro, linuxPath string) string {
	p := strings.TrimSpace(linuxPath)
	if !strings.HasPrefix(p, "/") {
		return linuxPath
	}
	if strings.HasPrefix(p, "/mnt/") && len(p) >= 6 {
		drive := p[5]
		if (drive >= 'a' && drive <= 'z') || (drive >= 'A' && drive <= 'Z') {
			if len(p) == 6 {
				return strings.ToUpper(string(drive)) + `:\`
			}
			if p[6] == '/' {
				return strings.ToUpper(string(drive)) + ":" + strings.ReplaceAll(p[6:], "/", `\`)
			}
		}
	}
	return WSLUNCRoot(distro) + strings.ReplaceAll(p, "/", `\`)
}
