package rfutils

import (
	"reflect"
	"strings"
	"testing"
)

// Captured from the probe script in an Ubuntu 24.04 distribution with WSLg,
// systemd, the WSLg GPU libraries, no Nix and no rfswift yet.
const sampleWSLProbeBare = `user=fluxius
home=/home/fluxius
shell=/bin/bash
init=systemd
x11=1
audio=1
gpulibs=1
usb=0
kernel=6.6.87.2-microsoft-standard-WSL2
probe=ok
`

// The same distribution once provisioned, with two forwarded USB devices.
const sampleWSLProbeReady = `user=fluxius
home=/home/fluxius
shell=/bin/zsh
nix=nix (Nix) 2.31.2
rfswift_path=/usr/local/bin/rfswift
rfswift=rfswift version 4.0.1-dev
init=systemd
x11=1
audio=1
gpulibs=1
bwrap=1
usb=2
kernel=6.6.87.2-microsoft-standard-WSL2
probe=ok
`

func TestParseWSLNixProbeBare(t *testing.T) {
	st := ParseWSLNixProbe("Ubuntu-24.04", sampleWSLProbeBare)
	if st.Distro != "Ubuntu-24.04" || st.User != "fluxius" || st.Home != "/home/fluxius" || st.Shell != "/bin/bash" {
		t.Fatalf("identity fields: %#v", st)
	}
	if st.HasNix() || st.HasRFSwift() || st.Ready() {
		t.Fatalf("a bare distribution must not look provisioned: %#v", st)
	}
	if !st.Systemd || !st.X11 || !st.Audio || !st.GPULibs || st.Bubblewrap {
		t.Fatalf("host features: %#v", st)
	}
	if st.USBDevices != 0 || st.Kernel != "6.6.87.2-microsoft-standard-WSL2" {
		t.Fatalf("usb/kernel: %#v", st)
	}
	if got := st.Missing(); !reflect.DeepEqual(got, []string{"nix", "the Linux rfswift CLI"}) {
		t.Fatalf("Missing() = %v", got)
	}
}

func TestParseWSLNixProbeReady(t *testing.T) {
	st := ParseWSLNixProbe("Ubuntu-24.04", sampleWSLProbeReady)
	if !st.Ready() {
		t.Fatalf("provisioned distribution must be ready: %#v", st)
	}
	if st.NixVersion != "nix (Nix) 2.31.2" {
		t.Fatalf("nix version = %q", st.NixVersion)
	}
	if st.RFSwiftVersion != "4.0.1-dev" || st.RFSwiftPath != "/usr/local/bin/rfswift" {
		t.Fatalf("rfswift = %q at %q", st.RFSwiftVersion, st.RFSwiftPath)
	}
	if !st.Bubblewrap || st.USBDevices != 2 {
		t.Fatalf("bwrap/usb: %#v", st)
	}
	if got := st.Missing(); len(got) != 0 {
		t.Fatalf("Missing() = %v, want none", got)
	}
}

func TestRFSwiftVersionFromBanner(t *testing.T) {
	cases := map[string]string{
		"rfswift version 4.0.1-dev": "4.0.1-dev",
		"rfswift version v4.0.0":    "4.0.0",
		"unknown":                   "unknown",
		"  ":                        "",
		// A first-run prompt printed instead of the version (seen on a fresh
		// distribution before the config file exists) must not become a version.
		"\x1b[38;5;208mConfig file not found in your user profile. Would you like to create one with default values? (y/n)\x1b[0m": "unknown",
		"Error: unknown flag: --version": "unknown",
	}
	for in, want := range cases {
		if got := rfswiftVersionFromBanner(in); got != want {
			t.Errorf("rfswiftVersionFromBanner(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWSLNixCandidatesSkipUtilityVMsAndWSL1(t *testing.T) {
	status := WSLStatus{Installed: true, Distros: []WSLDistro{
		{Name: "docker-desktop", State: "Running", Version: 2},
		{Name: "Debian", State: "Stopped", Version: 2},
		{Name: "Ubuntu-24.04", State: "Running", Version: 2, Default: true},
		{Name: "Legacy", State: "Stopped", Version: 1},
		{Name: "podman-machine-default", State: "Running", Version: 2},
		{Name: "Arch", State: "Stopped", Version: 2},
	}}
	got := WSLNixCandidates(status)
	names := make([]string, 0, len(got))
	for _, d := range got {
		names = append(names, d.Name)
	}
	if want := []string{"Ubuntu-24.04", "Arch", "Debian"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("candidates = %v, want %v (default first, then alphabetical, no utility VMs, no WSL 1)", names, want)
	}
	for _, name := range []string{"docker-desktop", "docker-desktop-data", "Podman-Machine-Default", "rancher-desktop"} {
		if !IsWSLUtilityDistro(name) {
			t.Errorf("%s should be a utility distribution", name)
		}
	}
	if IsWSLUtilityDistro("Ubuntu") {
		t.Error("Ubuntu is a real distribution")
	}
}

func TestSplitWSLUNC(t *testing.T) {
	cases := []struct {
		in, distro, linux string
		ok                bool
	}{
		{`\\wsl.localhost\Ubuntu-24.04\home\fluxius\.rfswift`, "Ubuntu-24.04", "/home/fluxius/.rfswift", true},
		{`\\wsl$\Debian\etc\wsl.conf`, "Debian", "/etc/wsl.conf", true},
		{`\\WSL.LOCALHOST\Ubuntu`, "Ubuntu", "/", true},
		{`//wsl.localhost/Ubuntu/tmp`, "Ubuntu", "/tmp", true},
		{`C:\Users\x`, "", "", false},
		{`\\server\share\x`, "", "", false},
		{`\\wsl.localhost\`, "", "", false},
	}
	for _, c := range cases {
		distro, linux, ok := SplitWSLUNC(c.in)
		if ok != c.ok || distro != c.distro || linux != c.linux {
			t.Errorf("SplitWSLUNC(%q) = (%q, %q, %v), want (%q, %q, %v)", c.in, distro, linux, ok, c.distro, c.linux, c.ok)
		}
	}
}

func TestWindowsPathToWSL(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{`C:\Users\fluxius\lab`, "/mnt/c/Users/fluxius/lab", true},
		{`d:/captures/`, "/mnt/d/captures", true},
		{`C:`, "/mnt/c", true},
		{`C:\`, "/mnt/c", true},
		{`\\wsl.localhost\Ubuntu-24.04\home\fluxius\rfswift-workspace\lab`, "/home/fluxius/rfswift-workspace/lab", true},
		{`/home/fluxius`, "/home/fluxius", true},
		{`github:PentHertz/RF-Swift-nix`, "github:PentHertz/RF-Swift-nix", false},
		{`mysdr`, "mysdr", false},
		{`--lazy`, "--lazy", false},
		{`c 189:* rwm`, "c 189:* rwm", false},
	}
	for _, c := range cases {
		got, ok := WindowsPathToWSL(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("WindowsPathToWSL(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
	for _, p := range []string{`C:\x`, `c:/x`, `Z:`} {
		if !IsWindowsAbsPath(p) {
			t.Errorf("%q should be a Windows absolute path", p)
		}
	}
	for _, p := range []string{`/x`, `x:y`, `1:\x`, `C`, `Cx\y`} {
		if IsWindowsAbsPath(p) {
			t.Errorf("%q should not be a Windows absolute path", p)
		}
	}
}

func TestWSLPathToWindows(t *testing.T) {
	if got := WSLPathToWindows("Ubuntu-24.04", "/mnt/c/Users/fluxius/lab"); got != `C:\Users\fluxius\lab` {
		t.Fatalf("drive path = %q", got)
	}
	if got := WSLPathToWindows("Ubuntu-24.04", "/mnt/d"); got != `D:\` {
		t.Fatalf("drive root = %q", got)
	}
	got := WSLPathToWindows("Ubuntu-24.04", "/home/fluxius/.rfswift/nix")
	if !strings.HasPrefix(got, `\\wsl`) || !strings.HasSuffix(got, `\Ubuntu-24.04\home\fluxius\.rfswift\nix`) {
		t.Fatalf("share path = %q", got)
	}
	if got := WSLPathToWindows("Ubuntu-24.04", "relative/path"); got != "relative/path" {
		t.Fatalf("relative paths must pass through, got %q", got)
	}
	// Round trip through the share stays consistent.
	if _, linux, ok := SplitWSLUNC(got); !ok || linux != "/home/fluxius/.rfswift/nix" && got != "relative/path" {
		t.Fatalf("round trip = %q %v", linux, ok)
	}
}

func TestWSLLoginArgsRunThroughLoginShell(t *testing.T) {
	args := wslLoginArgs("Ubuntu", "root", []string{"rfswift", "-q", "nix", "list", "--json"})
	want := []string{"-d", "Ubuntu", "-u", "root", "-e", "sh", "-l", "-c", `exec "$0" "$@"`, "rfswift", "-q", "nix", "list", "--json"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %q, want %q", args, want)
	}
	if got := wslLoginArgs("", "", []string{"nix", "--version"}); got[0] != "-e" {
		t.Fatalf("no distro/user must omit -d/-u: %q", got)
	}
}
