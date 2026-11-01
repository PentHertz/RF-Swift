/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package hostsetup

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// fakeQube points every Qubes probe at a temporary tree and returns it.
// vmType/persistence are what qubesdb would answer; binaries lists what
// LookPath finds.
type fakeQube struct {
	root string
	t    *testing.T
}

func newFakeQube(t *testing.T, vmType, persistence string, binaries ...string) *fakeQube {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Qubes OS qubes are Linux")
	}
	root := t.TempDir()
	q := &fakeQube{root: root, t: t}
	saved := struct {
		marker, usbImport, sudoers, polkit, custom, mountinfo, sysusb, bususb, dri, boot, dropin, modules, socket string
		nixBin, nixDaemon, nixDropIn                                                                              string
		systemctl                                                                                                 func(...string) (string, error)
		bindDirs                                                                                                  []string
		qdb                                                                                                       func(string) (string, error)
		look                                                                                                      func(string) (string, error)
		euid                                                                                                      func() int
		sudo                                                                                                      func() bool
	}{qubesMarkerFile, qubesUSBImport, qubesSudoersFile, qubesPolkitAllowFile, qubesCustomPersist, qubesMountInfo, qubesSysUSBDevices, qubesDevBusUSB, qubesDevDRI, qubesBootScriptPath, qubesPodmanDropIn, qubesModulesLoad, rootfulPodmanSocket,
		qubesNixBinary, nixProfileDaemon, nixDaemonDropIn, systemctlOutput,
		qubesBindDirsDirs, qubesdbRead, qubesLookPath, qubesEUID, sudoNonInteractive}
	t.Cleanup(func() {
		qubesMarkerFile, qubesUSBImport, qubesSudoersFile, qubesPolkitAllowFile, qubesCustomPersist = saved.marker, saved.usbImport, saved.sudoers, saved.polkit, saved.custom
		qubesMountInfo, qubesSysUSBDevices, qubesDevBusUSB, qubesDevDRI = saved.mountinfo, saved.sysusb, saved.bususb, saved.dri
		qubesBootScriptPath, qubesPodmanDropIn, qubesModulesLoad, rootfulPodmanSocket = saved.boot, saved.dropin, saved.modules, saved.socket
		qubesBindDirsDirs, qubesdbRead, qubesLookPath, qubesEUID, sudoNonInteractive = saved.bindDirs, saved.qdb, saved.look, saved.euid, saved.sudo
		qubesNixBinary, nixProfileDaemon, nixDaemonDropIn, systemctlOutput = saved.nixBin, saved.nixDaemon, saved.nixDropIn, saved.systemctl
		qubesOnce = sync.Once{}
	})
	savedRTL, savedModprobe, savedBindRoot := qubesRTLBlacklist, qubesModprobeDirs, qubesBindDirsRoot
	t.Cleanup(func() {
		qubesRTLBlacklist, qubesModprobeDirs, qubesBindDirsRoot = savedRTL, savedModprobe, savedBindRoot
	})
	savedDockerDropIn, savedDiskFree, savedDockerSocket := qubesDockerDropIn, qubesDiskFree, dockerSocket
	t.Cleanup(func() {
		qubesDockerDropIn, qubesDiskFree, dockerSocket = savedDockerDropIn, savedDiskFree, savedDockerSocket
	})
	p := func(rel string) string { return filepath.Join(root, rel) }
	qubesMarkerFile = p("usr/share/qubes/marker-vm")
	q.write("usr/share/qubes/marker-vm", "")
	qubesUSBImport = p("usr/lib/qubes/usb-import")
	q.write("usr/lib/qubes/usb-import", "")
	qubesSudoersFile = p("etc/sudoers.d/qubes")
	q.write("etc/sudoers.d/qubes", "user ALL=(ALL) NOPASSWD: ALL\n")
	qubesPolkitAllowFile = p("etc/polkit-1/rules.d/00-qubes-allow-all.rules")
	qubesCustomPersist = p("run/qubes-service/custom-persist")
	qubesMountInfo = p("proc/mountinfo")
	q.write("proc/mountinfo", "")
	qubesSysUSBDevices = p("sys/bus/usb/devices")
	qubesDevBusUSB = p("dev/bus/usb")
	qubesDevDRI = p("dev/dri")
	qubesBootScriptPath = p("rw/config/rc.local.d/50-rfswift.rc")
	qubesPodmanDropIn = p("etc/systemd/system/podman.socket.d/50-rfswift.conf")
	qubesModulesLoad = p("etc/modules-load.d/rfswift-vhci.conf")
	rootfulPodmanSocket = p("run/podman/podman.sock")
	qubesNixBinary = p("nix/var/nix/profiles/default/bin/nix")
	nixProfileDaemon = p("nix/var/nix/profiles/default/bin/nix-daemon")
	nixDaemonDropIn = p("etc/systemd/system/nix-daemon.service.d/50-rfswift.conf")
	systemctlOutput = func(args ...string) (string, error) { return "not-found", nil }
	qubesRTLBlacklist = p("etc/modprobe.d/rfswift-rtlsdr.conf")
	qubesModprobeDirs = []string{p("etc/modprobe.d"), p("usr/lib/modprobe.d")}
	qubesBindDirsRoot = p("rw/bind-dirs")
	qubesDockerDropIn = p("etc/systemd/system/docker.socket.d/50-rfswift.conf")
	dockerSocket = p("run/docker.sock")
	qubesDiskFree = func(string) (uint64, error) { return 100 << 30, nil }
	qubesBindDirsDirs = []string{p("usr/lib/qubes-bind-dirs.d"), p("rw/config/qubes-bind-dirs.d")}
	qubesdbRead = func(key string) (string, error) {
		switch key {
		case "/type", "/qubes-vm-type":
			return vmType, nil
		case "/name":
			return "sdr-lab", nil
		case "/qubes-vm-persistence":
			return persistence, nil
		}
		return "", errors.New("no such key")
	}
	found := map[string]bool{}
	for _, b := range binaries {
		found[b] = true
	}
	qubesLookPath = func(name string) (string, error) {
		if found[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	qubesEUID = func() int { return 1000 }
	sudoNonInteractive = func() bool { return false }
	qubesOnce = sync.Once{}
	return q
}

func (q *fakeQube) path(rel string) string { return filepath.Join(q.root, rel) }

func (q *fakeQube) write(rel, content string) {
	q.t.Helper()
	full := q.path(rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		q.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		q.t.Fatal(err)
	}
}

// listen makes the fake rootful Podman socket accessible and answering.
func (q *fakeQube) listen() {
	q.t.Helper()
	if err := os.MkdirAll(filepath.Dir(rootfulPodmanSocket), 0o755); err != nil {
		q.t.Fatal(err)
	}
	l, err := net.Listen("unix", rootfulPodmanSocket)
	if err != nil {
		q.t.Fatal(err)
	}
	q.t.Cleanup(func() { l.Close() })
}

func currentUser(t *testing.T) (string, string) {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Skip("no current user")
	}
	return u.Username, primaryGroup(u.Username)
}

func checkByID(st QubesStatus, id string) (QubesCheck, bool) {
	for _, c := range st.Checks {
		if c.ID == id {
			return c, true
		}
	}
	return QubesCheck{}, false
}

func TestDetectQubesOutsideQubes(t *testing.T) {
	newFakeQube(t, "AppVM", "rw-only")
	qubesMarkerFile = filepath.Join(t.TempDir(), "absent")
	qubesdbRead = func(string) (string, error) { return "", errors.New("qubesdb-read: not found") }
	if info := DetectQubes(); info.IsQubes {
		t.Fatalf("detected a qube without marker or qubesdb: %+v", info)
	}
	if st := GetQubesStatus(QubesStatusInput{}); len(st.Checks) != 0 || st.CanSetup {
		t.Fatalf("status outside Qubes must be empty: %+v", st)
	}
}

func TestDetectQubesAppVM(t *testing.T) {
	newFakeQube(t, "AppVM", "rw-only", "podman")
	info := DetectQubes()
	if !info.IsQubes || info.Name != "sdr-lab" || info.Type != "AppVM" || !info.TemplateBased() || info.Persistent() {
		t.Fatalf("unexpected info: %+v", info)
	}
	if !info.USBProxy {
		t.Fatalf("usb-import is present: %+v", info)
	}
	if st := GetQubesStatus(QubesStatusInput{User: "user"}); !st.Info.PasswordlessRoot {
		t.Fatalf("the sudoers file is present, root is passwordless: %+v", st.Info)
	}
}

func TestQubesSetupScriptAppVM(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only", "podman")
	name, group := currentUser(t)
	in := QubesStatusInput{User: name, WorkspaceRoot: q.path("home/ws")}
	o := DefaultQubesSetupOptions(in)
	if !o.PodmanSocket || !o.VHCI || !o.PersistStore || o.DockerGroup {
		t.Fatalf("defaults for a passwordless AppVM with Podman: %+v", o)
	}
	script, err := QubesSetupScript(in, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"binds+=( '/var/lib/containers' )",
		QubesBindDirsFile,
		"modprobe vhci-hcd",
		"SocketGroup=" + group,
		"chmod 0710 /run/podman",
		"systemctl restart podman.socket",
		"chmod 0755 " + qubesBootScriptPath,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "/var/lib/docker") || strings.Contains(script, "usermod") {
		t.Errorf("Docker is not installed, the script must not touch it:\n%s", script)
	}
	if strings.Contains(script, "/etc/modules-load.d") {
		t.Errorf("an AppVM's /etc is not persistent; vhci-hcd belongs in the boot script:\n%s", script)
	}
	// The boot script must be runnable on its own: shebang first, no set -e.
	boot := QubesBootScriptContent(o)
	if !strings.HasPrefix(boot, "#!/bin/sh\n") || strings.Contains(boot, "set -e") {
		t.Errorf("boot script must start with a shebang and not stop on the first error:\n%s", boot)
	}
}

func TestQubesSetupScriptSkipsConfiguredBind(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only", "podman")
	q.write("rw/config/qubes-bind-dirs.d/50_user.conf", "binds+=( '/var/lib/containers' )\n")
	name, _ := currentUser(t)
	in := QubesStatusInput{User: name}
	script, err := QubesSetupScript(in, DefaultQubesSetupOptions(in))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, QubesBindDirsFile) {
		t.Errorf("/var/lib/containers is already in bind-dirs, no second entry expected:\n%s", script)
	}
}

func TestQubesSetupScriptTemplateVM(t *testing.T) {
	newFakeQube(t, "TemplateVM", "full", "podman", "docker")
	name, _ := currentUser(t)
	in := QubesStatusInput{User: name}
	script, err := QubesSetupScript(in, DefaultQubesSetupOptions(in))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{qubesModulesLoad, "systemctl enable podman.socket", "usermod -aG docker"} {
		if !strings.Contains(script, want) {
			t.Errorf("TemplateVM script lacks %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "/rw/config") {
		t.Errorf("a TemplateVM writes /etc directly, not /rw:\n%s", script)
	}
}

func TestQubesSetupRejectsUnsafeNames(t *testing.T) {
	newFakeQube(t, "AppVM", "rw-only", "podman")
	in := QubesStatusInput{User: "user"}
	for _, o := range []QubesSetupOptions{
		{User: "user", Group: "user;reboot", PodmanSocket: true},
		{User: "root", Group: "root"},
		{User: "us er", Group: "user"},
		{User: "user", Group: "user", FixOwnership: []string{"relative/path"}},
		{User: "user", Group: "user", FixOwnership: []string{"/home/user/../../etc"}},
	} {
		if _, err := QubesSetupScript(in, o); err == nil {
			t.Errorf("options %+v must be refused", o)
		}
	}
}

func TestQubesNoSocketGrantWhenRootNeedsPassword(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only", "podman")
	os.Remove(q.path("etc/sudoers.d/qubes"))
	qubesOnce = sync.Once{}
	name, _ := currentUser(t)
	o := DefaultQubesSetupOptions(QubesStatusInput{User: name})
	if o.PodmanSocket {
		t.Fatal("the rootful socket must not be granted by default where root needs a password")
	}
	st := GetQubesStatus(QubesStatusInput{User: name})
	if c, _ := checkByID(st, "root"); c.Status != "warn" {
		t.Fatalf("root check = %+v", c)
	}
}

func TestQubesStatusReadyAfterSetup(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only", "podman")
	name, group := currentUser(t)
	q.write("dev/bus/usb/001/001", "")
	q.write("rw/config/qubes-bind-dirs.d/50_rfswift.conf", "binds+=( '/var/lib/containers' )\n")
	q.write("proc/mountinfo", "100 30 202:16 /bind-dirs/var/lib/containers /var/lib/containers rw,relatime shared:1 - ext4 /dev/xvdb rw\n")
	q.write("etc/systemd/system/podman.socket.d/50-rfswift.conf", QubesPodmanDropInContent(group))
	q.write("etc/modprobe.d/rfswift-rtlsdr.conf", "blacklist dvb_usb_rtl28xxu\n")
	q.listen()
	in := QubesStatusInput{User: name, PodmanMode: "rootful", WorkspaceRoot: q.path("home/ws"), Devices: "/dev/bus/usb:/dev/bus/usb"}
	o := DefaultQubesSetupOptions(in)
	q.write("rw/config/rc.local.d/50-rfswift.rc", QubesBootScriptContent(o))
	st := GetQubesStatus(in)
	if !st.Ready {
		t.Fatalf("everything is in place, status should be ready; plan=%v checks=%+v", st.Plan, st.Checks)
	}
	if c, _ := checkByID(st, "storage-podman"); c.Status != "ok" {
		t.Errorf("storage check = %+v", c)
	}
	if c, _ := checkByID(st, "podman-access"); c.Status != "ok" {
		t.Errorf("podman access check = %+v", c)
	}
	if _, ok := checkByID(st, "usb-streaming"); ok {
		t.Error("no device attached: no streaming tip expected")
	}
	q.write("sys/bus/usb/devices/1-1/idVendor", "0bda\n")
	q.write("sys/bus/usb/devices/1-1/idProduct", "2838\n")
	if c, ok := checkByID(GetQubesStatus(in), "usb-streaming"); !ok || !strings.Contains(c.Detail, "buflen=16384") {
		t.Errorf("an attached device must bring the streaming tip: %+v", c)
	}
	if script, _ := QubesSetupScript(in, o); strings.TrimSpace(script) != "" {
		t.Errorf("nothing left to do, but the script is:\n%s", script)
	}
}

func TestQubesStatusPlansMissingPieces(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only", "podman")
	name, _ := currentUser(t)
	in := QubesStatusInput{User: name, PodmanMode: "auto", WorkspaceRoot: q.path("home/ws"), Devices: "/dev/bus/usb:/dev/bus/usb,/dev/dri:/dev/dri"}
	st := GetQubesStatus(in)
	if st.Ready {
		t.Fatal("a fresh AppVM is not ready")
	}
	joined := strings.Join(st.Plan, "\n")
	for _, want := range []string{"vhci-hcd", "/var/lib/containers", "podman_mode = rootful", qubesBootScriptPath, "/dev/dri"} {
		if !strings.Contains(joined, want) {
			t.Errorf("plan lacks %q:\n%s", want, joined)
		}
	}
	if !st.SetMode || len(st.DropDevices) != 1 || st.DropDevices[0] != "/dev/dri:/dev/dri" {
		t.Errorf("SetMode=%v DropDevices=%v", st.SetMode, st.DropDevices)
	}
	if c, _ := checkByID(st, "usb-bus"); c.Status != "warn" || !strings.Contains(c.Fix, "qvm-usb attach sdr-lab") {
		t.Errorf("usb-bus check = %+v", c)
	}
}

func TestQubesUSBDevices(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only")
	q.write("sys/bus/usb/devices/1-1/idVendor", "1d50\n")
	q.write("sys/bus/usb/devices/1-1/idProduct", "6089\n")
	q.write("sys/bus/usb/devices/1-1/manufacturer", "Great Scott Gadgets\n")
	q.write("sys/bus/usb/devices/1-1/product", "HackRF One\n")
	q.write("sys/bus/usb/devices/usb1/idVendor", "1d6b\n")       // root hub
	q.write("sys/bus/usb/devices/1-1:1.0/bInterfaceClass", "ff") // interface
	got := QubesUSBDevices()
	if len(got) != 1 || got[0].BusID != "1-1" || got[0].Label() != "1d50:6089 Great Scott Gadgets HackRF One" {
		t.Fatalf("devices = %+v", got)
	}
}

func TestBindDirsConfigured(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only")
	q.write("usr/lib/qubes-bind-dirs.d/30_cron.conf", "binds+=( '/var/spool/cron' )\n")
	q.write("rw/config/qubes-bind-dirs.d/50_user.conf", "# binds+=( '/commented' )\nbinds+=( \"/var/lib/docker\" '/etc/foo/' )\n")
	got := bindDirsConfigured()
	for _, want := range []string{"/var/spool/cron", "/var/lib/docker", "/etc/foo"} {
		if got[want] == "" {
			t.Errorf("%s not found in %v", want, got)
		}
	}
	if got["/commented"] != "" {
		t.Error("commented entries must be ignored")
	}
}

func TestDropDevices(t *testing.T) {
	in := "/dev/bus/usb:/dev/bus/usb, /dev/dri:/dev/dri,/dev/snd:/dev/snd"
	if got := DropDevices(in, []string{"/dev/dri:/dev/dri"}); got != "/dev/bus/usb:/dev/bus/usb,/dev/snd:/dev/snd" {
		t.Fatalf("DropDevices = %q", got)
	}
}

func TestQubesPodmanDropIn(t *testing.T) {
	got := QubesPodmanDropInContent("user")
	for _, want := range []string{"[Socket]", "SocketMode=0660", "SocketGroup=user", "ExecStartPost=/bin/sh -c 'chgrp user /run/podman && chmod 0710 /run/podman'"} {
		if !strings.Contains(got, want) {
			t.Errorf("drop-in lacks %q:\n%s", want, got)
		}
	}
}

// withNix gives the fake qube a template-installed Determinate Nix whose
// daemon binary is not visible (execStart missing) or is (present).
func (q *fakeQube) withNix(execStart string, active string) {
	q.t.Helper()
	q.write("nix/var/nix/profiles/default/bin/nix", "")
	q.write("nix/var/nix/profiles/default/bin/nix-daemon", "")
	systemctlOutput = func(args ...string) (string, error) {
		switch {
		case len(args) > 2 && args[2] == "LoadState":
			return "loaded", nil
		case len(args) > 2 && args[2] == "ExecStart":
			return "{ path=" + execStart + " ; argv[]=determinate-nixd daemon ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }", nil
		case len(args) > 0 && args[0] == "is-active":
			return active, nil
		}
		return "", nil
	}
}

func TestParseExecStartPath(t *testing.T) {
	in := "{ path=/usr/local/bin/determinate-nixd ; argv[]=determinate-nixd daemon ; ignore_errors=no ; start_time=[n/a] }"
	if got := parseExecStartPath(in); got != "/usr/local/bin/determinate-nixd" {
		t.Fatalf("parseExecStartPath = %q", got)
	}
	if got := parseExecStartPath(""); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

// The boot script parses `systemctl show -p ExecStart --value` with sed; run
// that exact expression on real output to make sure it extracts the path.
func TestNixDaemonBootScriptSedExtractsPath(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	script := nixDaemonBootScript()
	i := strings.Index(script, "sed -n '")
	if i < 0 {
		t.Fatalf("no sed expression in:\n%s", script)
	}
	expr := script[i+len("sed -n '"):]
	expr = expr[:strings.Index(expr, "'")]
	out, err := exec.Command("/bin/sh", "-c", "printf '%s\\n' \"$1\" | sed -n \"$2\"", "sh",
		"{ path=/usr/local/bin/determinate-nixd ; argv[]=determinate-nixd daemon ; ignore_errors=no }", expr).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "/usr/local/bin/determinate-nixd" {
		t.Fatalf("sed extracted %q", got)
	}
}

func TestQubesNixDaemonRepair(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only", "podman")
	q.withNix("/usr/local/bin/determinate-nixd-not-here", "failed")
	name, _ := currentUser(t)
	in := QubesStatusInput{User: name, PodmanMode: "rootful"}
	st := GetQubesStatus(in)
	if c, _ := checkByID(st, "nix-daemon"); c.Status != "warn" || !strings.Contains(c.Detail, "determinate-nixd-not-here") {
		t.Fatalf("nix-daemon check = %+v", c)
	}
	if c, _ := checkByID(st, "nix-store"); c.Status != "warn" {
		t.Fatalf("nix-store check = %+v", c)
	}
	joined := strings.Join(st.Plan, "\n")
	if !strings.Contains(joined, "run the Nix daemon from") || !strings.Contains(joined, "keep /nix across restarts") {
		t.Fatalf("plan lacks the Nix steps:\n%s", joined)
	}
	o := DefaultQubesSetupOptions(in)
	if !o.NixFound || !o.NixDaemon {
		t.Fatalf("options: %+v", o)
	}
	script, err := QubesSetupScript(in, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"binds+=( '/nix' )", "ExecStart=@" + nixProfileDaemon + " nix-daemon --daemon", "systemctl restart nix-daemon.service"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
}

func TestQubesNixDaemonHealthy(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only")
	q.withNix(q.path("nix/var/nix/profiles/default/bin/nix-daemon"), "active")
	q.write("rw/config/qubes-bind-dirs.d/50_rfswift.conf", "binds+=( '/nix' )\n")
	q.write("proc/mountinfo", "101 30 202:16 /bind-dirs/nix /nix rw,relatime shared:1 - ext4 /dev/xvdb rw\n")
	name, _ := currentUser(t)
	st := GetQubesStatus(QubesStatusInput{User: name})
	if c, _ := checkByID(st, "nix-daemon"); c.Status != "ok" {
		t.Fatalf("nix-daemon check = %+v", c)
	}
	if c, _ := checkByID(st, "nix-store"); c.Status != "ok" {
		t.Fatalf("nix-store check = %+v", c)
	}
	for _, p := range st.Plan {
		if strings.Contains(p, "run the Nix daemon") || strings.Contains(p, "keep /nix") {
			t.Errorf("nothing Nix-related left to do, plan has %q", p)
		}
	}
}

func TestNoNixNoNixChecks(t *testing.T) {
	newFakeQube(t, "AppVM", "rw-only", "podman")
	name, _ := currentUser(t)
	st := GetQubesStatus(QubesStatusInput{User: name})
	if _, ok := checkByID(st, "nix-daemon"); ok {
		t.Fatal("no Nix installed: no Nix checks expected")
	}
}

func TestQubesRTLBlacklist(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only")
	name, _ := currentUser(t)
	in := QubesStatusInput{User: name}
	st := GetQubesStatus(in)
	if c, _ := checkByID(st, "rtl-dvb"); c.Status != "warn" {
		t.Fatalf("no blacklist: rtl-dvb check = %+v", c)
	}
	if !strings.Contains(strings.Join(st.Plan, "\n"), "dvb_usb_rtl28xxu") {
		t.Fatalf("plan lacks the blacklist: %v", st.Plan)
	}
	script, err := QubesSetupScript(in, DefaultQubesSetupOptions(in))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "blacklist dvb_usb_rtl28xxu") || !strings.Contains(script, "modprobe -r dvb_usb_rtl28xxu") {
		t.Fatalf("script lacks the blacklist:\n%s", script)
	}
	o := DefaultQubesSetupOptions(in)
	o.RTLBlacklist = false
	if script, _ := QubesSetupScript(in, o); strings.Contains(script, "dvb_usb_rtl28xxu") {
		t.Fatalf("--no-rtl-blacklist must leave the driver alone:\n%s", script)
	}
	// A blacklist from anywhere (distribution package, the user) counts.
	q.write("usr/lib/modprobe.d/rtl-sdr-blacklist.conf", "# rtl-sdr package\nblacklist dvb_usb_rtl28xxu\n")
	if c, _ := checkByID(GetQubesStatus(in), "rtl-dvb"); c.Status != "ok" {
		t.Fatalf("with a blacklist: rtl-dvb check = %+v", c)
	}
}

func TestQubesLiveStores(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only", "docker")
	q.withNix(q.path("nix/var/nix/profiles/default/bin/nix-daemon"), "active")
	name, _ := currentUser(t)
	in := QubesStatusInput{User: name}
	script, err := QubesSetupScript(in, DefaultQubesSetupOptions(in))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"binds+=( '/var/lib/containerd' )",
		"binds+=( '/var/lib/docker' )",
		"binds+=( '/nix' )",
		"rfswift_bind_now() {",
		"rfswift_bind_now /nix 'nix-daemon.socket nix-daemon.service' 'nix-daemon.socket'",
		"rfswift_bind_now /var/lib/containerd 'docker.socket docker.service containerd.service' 'docker.socket'",
		`if [ -n "$(docker ps -q 2>/dev/null)" ]; then`,
		"copy=" + q.path("rw/bind-dirs") + "$1",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
	// Already on the persistent copy: nothing to switch.
	q.write("proc/mountinfo", "1 2 202:16 /bind-dirs/nix /nix rw - ext4 /dev/xvdb rw\n3 2 202:16 /bind-dirs/var/lib/docker /var/lib/docker rw - ext4 /dev/xvdb rw\n4 2 202:16 /bind-dirs/var/lib/containerd /var/lib/containerd rw - ext4 /dev/xvdb rw\n")
	if script, _ := QubesSetupScript(in, DefaultQubesSetupOptions(in)); strings.Contains(script, "rfswift_bind_now /") {
		t.Errorf("mounted already, no switch expected:\n%s", script)
	}
}

// Re-running the setup after another engine was installed must keep the
// entries RF Swift wrote before (the file is rewritten whole).
func TestQubesBindDirsFileKeepsOwnEntries(t *testing.T) {
	q := newFakeQube(t, "AppVM", "rw-only", "docker")
	q.write("rw/config/qubes-bind-dirs.d/50_rfswift.conf", "# Written by 'rfswift host qubes setup'\nbinds+=( '/nix' )\n")
	name, _ := currentUser(t)
	in := QubesStatusInput{User: name}
	script, err := QubesSetupScript(in, DefaultQubesSetupOptions(in))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"binds+=( '/nix' )", "binds+=( '/var/lib/docker' )", "binds+=( '/var/lib/containerd' )"} {
		if !strings.Contains(script, want) {
			t.Errorf("rewritten bind-dirs file lacks %q:\n%s", want, script)
		}
	}
}

func TestQubesDockerSetup(t *testing.T) {
	newFakeQube(t, "AppVM", "rw-only", "docker")
	name, _ := currentUser(t)
	in := QubesStatusInput{User: name}
	st := GetQubesStatus(in)
	if c, _ := checkByID(st, "docker-service"); c.Status != "warn" {
		t.Fatalf("docker not running: docker-service check = %+v", c)
	}
	joined := strings.Join(st.Plan, "\n")
	for _, want := range []string{"start Docker", "keep /var/lib/docker across restarts", "access to the Docker socket"} {
		if !strings.Contains(joined, want) {
			t.Errorf("plan lacks %q:\n%s", want, joined)
		}
	}
	o := DefaultQubesSetupOptions(in)
	if !o.DockerStart || !o.DockerGroup {
		t.Fatalf("options: %+v", o)
	}
	script, err := QubesSetupScript(in, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"binds+=( '/var/lib/docker' )", "ExecStartPost=/bin/sh -c 'command -v setfacl", "systemctl start docker.socket", "usermod -aG docker"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
	// Running containers must survive a re-run: never restart Docker.
	if strings.Contains(script, "restart docker") {
		t.Errorf("the setup must never restart Docker:\n%s", script)
	}
	// The socket must be started before the ACL is applied to it.
	direct := strings.Index(script, "if command -v setfacl >/dev/null 2>&1 && [ -S")
	if direct < 0 || strings.Index(script, "systemctl start docker.socket") > direct {
		t.Errorf("the ACL is applied before the socket exists:\n%s", script)
	}
}

func TestQubesDockerRunning(t *testing.T) {
	newFakeQube(t, "AppVM", "rw-only", "docker")
	systemctlOutput = func(args ...string) (string, error) {
		if len(args) == 2 && args[0] == "is-active" && args[1] == "docker.socket" {
			return "active", nil
		}
		return "not-found", nil
	}
	name, _ := currentUser(t)
	st := GetQubesStatus(QubesStatusInput{User: name})
	if c, _ := checkByID(st, "docker-service"); c.Status != "ok" {
		t.Fatalf("docker.socket active: docker-service check = %+v", c)
	}
	for _, p := range st.Plan {
		if strings.Contains(p, "start Docker") {
			t.Errorf("Docker runs, plan still has %q", p)
		}
	}
}

func TestQubesDiskSpace(t *testing.T) {
	newFakeQube(t, "AppVM", "rw-only", "podman")
	name, _ := currentUser(t)
	qubesDiskFree = func(string) (uint64, error) { return 5 << 30, nil }
	c, ok := checkByID(GetQubesStatus(QubesStatusInput{User: name}), "disk")
	if !ok || c.Status != "warn" || !strings.Contains(c.Fix, "qvm-volume extend sdr-lab:private") {
		t.Fatalf("5 GiB free: disk check = %+v", c)
	}
	qubesDiskFree = func(string) (uint64, error) { return 80 << 30, nil }
	if c, _ := checkByID(GetQubesStatus(QubesStatusInput{User: name}), "disk"); c.Status != "ok" {
		t.Fatalf("80 GiB free: disk check = %+v", c)
	}
}
