/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package hostsetup

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHostRulesEmbedded(t *testing.T) {
	rules := HostRules()
	if !strings.Contains(string(rules), `ATTRS{idVendor}=="1d50"`) {
		t.Fatal("embedded rules lack the HackRF vendor match")
	}
	if strings.Contains(string(rules), `MODE="0666"`) {
		t.Fatal("host rules must not make device nodes world-writable")
	}
	if got := ruleGroups(rules); len(got) != 1 || got[0] != "plugdev" {
		t.Fatalf("groups = %v, want [plugdev]", got)
	}
	if !ValidRuleFileName(HostRulesFile) {
		t.Fatalf("%q must be a valid rules file name", HostRulesFile)
	}
}

func TestUdevStatusStates(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("udev is Linux only")
	}
	dir := t.TempDir()
	udevRulesDir = dir
	defer func() { udevRulesDir = UdevRulesDir }()
	path := filepath.Join(dir, HostRulesFile)

	if st := GetUdevStatus(); st.State != UdevMissing || st.Ready {
		t.Fatalf("empty dir: state %q ready %v", st.State, st.Ready)
	}
	if err := os.WriteFile(path, append([]byte(hostHeader()), hostRules...), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := GetUdevStatus(); st.State != UdevInstalled {
		t.Fatalf("our file: state %q", st.State)
	}
	if err := os.WriteFile(path, append([]byte(hostHeader()), []byte("# older\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := GetUdevStatus(); st.State != UdevOutdated {
		t.Fatalf("older body: state %q", st.State)
	}
	if err := os.WriteFile(path, []byte("# hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := GetUdevStatus()
	if st.State != UdevForeign {
		t.Fatalf("foreign file: state %q", st.State)
	}
	if _, err := InstallUdevRules(); err == nil {
		t.Fatal("a foreign file must never be overwritten")
	}
	if _, err := RemoveUdevRules(); err == nil {
		t.Fatal("a foreign file must never be removed")
	}
	if !strings.Contains(udevRemoveScript(path), "rm -f '"+path+"'") {
		t.Fatal("remove script does not delete the file")
	}
}

func TestUdevInstallScript(t *testing.T) {
	script, err := UdevInstallScript(map[string]string{HostRulesFile: "/tmp/stage/70-rfswift.rules"}, []string{"plugdev"}, []string{"plugdev"}, "o'neil")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"set -e\n",
		"install -m 0644 '/tmp/stage/70-rfswift.rules' '/etc/udev/rules.d/70-rfswift.rules'\n",
		"getent group plugdev >/dev/null || groupadd --system plugdev\n",
		"usermod -aG plugdev 'o'\\''neil'\n",
		"udevadm control --reload-rules\nudevadm trigger\n",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script lacks %q:\n%s", want, script)
		}
	}
	if _, err := UdevInstallScript(map[string]string{"../evil.rules": "/x"}, nil, nil, ""); err == nil {
		t.Fatal("path-like file names must be refused")
	}
	if _, err := UdevInstallScript(nil, []string{"bad group;rm"}, nil, ""); err == nil {
		t.Fatal("shell metacharacters in a group must be refused")
	}
	if script, _ := UdevInstallScript(nil, nil, []string{"plugdev"}, ""); strings.Contains(script, "usermod") {
		t.Fatal("no user known: no usermod")
	}
}

func TestDockerGrantScript(t *testing.T) {
	script, err := dockerGrantScript("o'neil", "/run/docker.sock")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"getent group docker >/dev/null || groupadd --system docker\n",
		"usermod -aG docker 'o'\\''neil'\n",
		"setfacl -m u:'o'\\''neil':rw '/run/docker.sock'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script lacks %q:\n%s", want, script)
		}
	}
	if _, err := dockerGrantScript("", DockerSocket); err == nil {
		t.Fatal("empty user must be refused")
	}
}

func TestProbeSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets")
	}
	dir := t.TempDir()
	none := filepath.Join(dir, "none.sock")
	if a, r, e := probeSocket(none); a || r || e {
		t.Fatalf("missing socket: accessible %v reachable %v exists %v", a, r, e)
	}
	live := filepath.Join(dir, "live.sock")
	ln, err := net.Listen("unix", live)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	if a, r, e := probeSocket(live); !a || !r || !e {
		t.Fatalf("listening socket: accessible %v reachable %v exists %v", a, r, e)
	}
	ln.Close()
	// Listener gone: the file may be unlinked by Close; recreate a dead one.
	dead := filepath.Join(dir, "dead.sock")
	dl, err := net.Listen("unix", dead)
	if err != nil {
		t.Fatal(err)
	}
	dl.(*net.UnixListener).SetUnlinkOnClose(false)
	dl.Close()
	if a, r, e := probeSocket(dead); !a || r || !e {
		t.Fatalf("dead socket: accessible %v reachable %v exists %v", a, r, e)
	}
}

func TestDockerAccessStatusOffLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		dockerSocket = filepath.Join(t.TempDir(), "docker.sock")
		defer func() { dockerSocket = DockerSocket }()
		st := GetDockerAccess()
		if !st.Supported || st.SocketFound || st.Accessible {
			t.Fatalf("no socket: %+v", st)
		}
		return
	}
	if st := GetDockerAccess(); st.Supported {
		t.Fatal("must be unsupported off Linux")
	}
}

func TestParseOSRelease(t *testing.T) {
	cases := []struct{ content, id, pm string }{
		{"ID=ubuntu\nID_LIKE=debian\nPRETTY_NAME=\"Ubuntu 24.04\"\n", "ubuntu", "apt"},
		{"ID=debian\nNAME=\"Debian GNU/Linux\"\n", "debian", "apt"},
		{"ID=kali\nID_LIKE=debian\n", "kali", "apt"},
		{"ID=fedora\n", "fedora", "dnf"},
		{"ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\n", "rocky", "dnf"},
		{"ID=arch\n", "arch", "pacman"},
		{"ID=endeavouros\nID_LIKE=arch\n", "endeavouros", "pacman"},
		{"ID=opensuse-tumbleweed\nID_LIKE=\"opensuse suse\"\n", "opensuse-tumbleweed", "zypper"},
		{"ID=alpine\n", "alpine", "apk"},
		{"ID=nixos\n", "nixos", "nix"},
		{"ID=gentoo\n", "gentoo", ""},
	}
	for _, c := range cases {
		d := parseOSRelease(c.content)
		if d.ID != c.id {
			t.Errorf("%q: id %q want %q", c.content, d.ID, c.id)
		}
		if d.PackageManager != c.pm && !(c.pm == "dnf" && d.PackageManager == "yum") {
			t.Errorf("%q: pm %q want %q", c.content, d.PackageManager, c.pm)
		}
	}
}

func TestEnginePackagesAndPlan(t *testing.T) {
	if pkgs, err := enginePackages(Distro{ID: "ubuntu", PackageManager: "apt"}, EngineDocker); err != nil || pkgs[0] != "docker.io" {
		t.Fatalf("ubuntu docker: %v %v", pkgs, err)
	}
	if _, err := enginePackages(Distro{ID: "rocky", Name: "Rocky Linux", PackageManager: "dnf"}, EngineDocker); err == nil {
		t.Fatal("RHEL family must refuse Docker (not packaged)")
	}
	if pkgs, err := enginePackages(Distro{ID: "fedora", PackageManager: "dnf"}, EngineDocker); err != nil || pkgs[0] != "moby-engine" {
		t.Fatalf("fedora docker: %v %v", pkgs, err)
	}
	if _, err := enginePackages(Distro{ID: "gentoo"}, EnginePodman); err == nil {
		t.Fatal("unknown distribution must be refused")
	}
	if c, err := ParseEngineChoice("Both"); err != nil || c != EngineBoth {
		t.Fatalf("parse: %v %v", c, err)
	}
	if _, err := ParseEngineChoice("lima"); err == nil {
		t.Fatal("lima is not an installable engine here")
	}
	if runtime.GOOS != "linux" {
		return
	}
	// Plan on a fake Ubuntu with a fixed user.
	dir := t.TempDir()
	osReleasePath = filepath.Join(dir, "os-release")
	defer func() { osReleasePath = "/etc/os-release" }()
	if err := os.WriteFile(osReleasePath, []byte("ID=ubuntu\nID_LIKE=debian\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUDO_USER", "alice")
	plan, err := PlanEngineInstall(EngineBoth)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"apt-get install -y docker.io podman uidmap slirp4netns fuse-overlayfs\n",
		"systemctl enable --now docker",
		"usermod -aG docker 'alice'\n",
		"setfacl -m u:'alice':rw '/var/run/docker.sock'",
		"usermod --add-subuids 100000-165535 'alice'\n",
		"loginctl enable-linger 'alice'",
	} {
		if !strings.Contains(plan.Script, want) {
			t.Fatalf("plan lacks %q:\n%s", want, plan.Script)
		}
	}
	if len(plan.Steps) != 4 {
		t.Fatalf("steps = %v", plan.Steps)
	}
	t.Setenv("SUDO_USER", "bad user;id")
	if _, err := PlanEngineInstall(EnginePodman); err == nil {
		t.Fatal("unsafe user names must be refused")
	}
	if _, err := PlanEngineInstall(EngineNone); err == nil {
		t.Fatal("none must not plan anything")
	}
}
