/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Host setup - Qubes OS integration.
*
*  RF Swift inside a Qubes OS qube meets a few things a plain Linux host
*  does not have:
*
*  - USB devices come from sys-usb through qvm-usb (usbip over vchan). The
*    qube has no USB controller of its own, so /dev/bus/usb only exists once
*    the first device is attached, when qubes-usb-proxy loads vhci-hcd. A
*    container created or started before that fails on its /dev/bus/usb
*    mapping, and one started without it never sees devices attached later.
*    Loading vhci-hcd at boot creates the virtual bus up front: containers
*    start, and devices attached afterwards appear inside them (with the
*    "c 189:* rwm" cgroup rule RF Swift sets by default).
*  - In a template-based AppVM only /home, /rw and /usr/local survive a
*    restart. The rootful engines keep their images and containers in
*    /var/lib/containers (Podman) and /var/lib/docker, and changes to /etc
*    vanish as well, so every reboot throws away what was pulled and how the
*    engine was configured. bind-dirs (/rw/config/qubes-bind-dirs.d) keeps
*    the storage, and a boot script in /rw/config/rc.local.d re-applies the
*    rest.
*  - Rootless Podman cannot map root-only device nodes, so SDR/USB work
*    wants the rootful service, and `sudo rfswift` used to lose the user's
*    audio server and home on the way. Inside a qube the user already has
*    passwordless root (qubes-core-agent-passwordless-root): the qube, not
*    the account, is the security boundary. Making the rootful Podman socket
*    usable by the user therefore grants nothing the user does not already
*    have, and lets RF Swift and the Workbench run as the user against the
*    rootful service (podman_mode = rootful).
*
*  Status never changes anything; the setup builds one root script, shows it,
*  and runs it through RunPrivileged (sudo on a terminal, polkit from the
*  Workbench). Like the other host-setup scripts it only ever holds names
*  validated against strict patterns or single-quoted values.
 */

package hostsetup

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Paths used by the Qubes integration, overridable in tests.
var (
	qubesMarkerFile      = "/usr/share/qubes/marker-vm"
	qubesUSBImport       = "/usr/lib/qubes/usb-import"
	qubesSudoersFile     = "/etc/sudoers.d/qubes"
	qubesPolkitAllowFile = "/etc/polkit-1/rules.d/00-qubes-allow-all.rules"
	qubesCustomPersist   = "/var/run/qubes-service/custom-persist"
	qubesBindDirsDirs    = []string{"/usr/lib/qubes-bind-dirs.d", "/etc/qubes-bind-dirs.d", "/rw/config/qubes-bind-dirs.d"}
	qubesMountInfo       = "/proc/self/mountinfo"
	qubesSysUSBDevices   = "/sys/bus/usb/devices"
	qubesDevBusUSB       = "/dev/bus/usb"
	qubesDevDRI          = "/dev/dri"
	qubesBootScriptPath  = QubesBootScript
	qubesPodmanDropIn    = QubesPodmanDropIn
	qubesModulesLoad     = QubesModulesLoad
	qubesRTLBlacklist    = QubesRTLBlacklist
	qubesBindDirsRoot    = "/rw/bind-dirs"
	qubesDockerDropIn    = QubesDockerDropIn
	qubesDiskFree        = diskFreeBytes
	qubesNixBinary       = "/nix/var/nix/profiles/default/bin/nix"
	qubesModprobeDirs    = []string{"/etc/modprobe.d", "/usr/lib/modprobe.d", "/run/modprobe.d", "/lib/modprobe.d"}
	qubesdbRead          = func(key string) (string, error) {
		out, err := exec.Command("qubesdb-read", key).Output()
		return strings.TrimSpace(string(out)), err
	}
	qubesLookPath = exec.LookPath
	qubesEUID     = os.Geteuid
)

// Files the setup writes.
const (
	QubesBindDirsFile = "/rw/config/qubes-bind-dirs.d/50_rfswift.conf"
	QubesBootScript   = "/rw/config/rc.local.d/50-rfswift.rc"
	QubesPodmanDropIn = "/etc/systemd/system/podman.socket.d/50-rfswift.conf"
	QubesModulesLoad  = "/etc/modules-load.d/rfswift-vhci.conf"
	QubesRTLBlacklist = "/etc/modprobe.d/rfswift-rtlsdr.conf"
	QubesDockerDropIn = "/etc/systemd/system/docker.socket.d/50-rfswift.conf"
	// qubesMinFreeGiB is the free space below which the status warns: RF
	// Swift images take 8 to 18 GB each, and in an AppVM bind-dirs keeps
	// them on the private volume.
	qubesMinFreeGiB    = 20
	qubesSetupMarker   = "# Written by 'rfswift host qubes setup'"
	podmanRootfulStore = "/var/lib/containers"
	dockerRootfulStore = "/var/lib/docker"
	// Docker 25+ with the containerd image store (the default on current
	// distributions) keeps images in containerd's content store and
	// snapshots, not under /var/lib/docker.
	containerdStore = "/var/lib/containerd"
	nixStore        = "/nix"
)

// QubesInfo describes the qube RF Swift runs in.
type QubesInfo struct {
	IsQubes          bool   `json:"isQubes"`
	Name             string `json:"name"`             // qube name (qubesdb /name)
	Type             string `json:"type"`             // AppVM, DispVM, TemplateVM, StandaloneVM, ...
	Persistence      string `json:"persistence"`      // full | rw-only | none
	CustomPersist    bool   `json:"customPersist"`    // the custom-persist feature replaces bind-dirs and rc.local
	PasswordlessRoot bool   `json:"passwordlessRoot"` // root without a password (filled by the status, not by DetectQubes)
	USBProxy         bool   `json:"usbProxy"`         // qubes-usb-proxy is installed (qvm-usb can attach here)
}

// Persistent reports whether changes outside /home, /rw and /usr/local stay.
func (i QubesInfo) Persistent() bool { return i.Persistence == "full" }

// TemplateBased reports whether this is an AppVM whose /rw can carry the
// setup across restarts (bind-dirs and rc.local).
func (i QubesInfo) TemplateBased() bool { return i.Persistence == "rw-only" && !i.CustomPersist }

var (
	qubesOnce   sync.Once
	qubesCached QubesInfo
)

// DetectQubes reports whether this is a Qubes OS qube and what kind. The
// result is cached for the process lifetime.
func DetectQubes() QubesInfo {
	qubesOnce.Do(func() { qubesCached = detectQubes() })
	return qubesCached
}

// IsQubes is DetectQubes().IsQubes.
func IsQubes() bool { return DetectQubes().IsQubes }

func detectQubes() QubesInfo {
	var info QubesInfo
	if runtime.GOOS != "linux" {
		return info
	}
	_, markerErr := os.Stat(qubesMarkerFile)
	vmType, typeErr := qubesdbRead("/type")
	if markerErr != nil && (typeErr != nil || vmType == "") {
		return info
	}
	info.IsQubes = true
	info.Type = vmType
	if info.Type == "" {
		info.Type, _ = qubesdbRead("/qubes-vm-type")
	}
	info.Name, _ = qubesdbRead("/name")
	info.Persistence, _ = qubesdbRead("/qubes-vm-persistence")
	if info.Persistence == "" {
		switch info.Type {
		case "TemplateVM", "StandaloneVM":
			info.Persistence = "full"
		case "DispVM":
			info.Persistence = "none"
		default:
			info.Persistence = "rw-only"
		}
	}
	info.CustomPersist = pathExists(qubesCustomPersist)
	info.USBProxy = pathExists(qubesUSBImport)
	return info
}

// passwordlessRoot reports whether the user has root without a password:
// qubes-core-agent-passwordless-root's sudoers and polkit files when they can
// be seen (/etc/sudoers.d is not readable by a user), otherwise a
// non-interactive sudo, which never prompts.
func passwordlessRoot() bool {
	if pathExists(qubesSudoersFile) || pathExists(qubesPolkitAllowFile) {
		return true
	}
	if qubesEUID() == 0 {
		return false
	}
	return sudoNonInteractive()
}

// sudoNonInteractive runs `sudo -n true`; overridable in tests.
var sudoNonInteractive = func() bool {
	if _, err := qubesLookPath("sudo"); err != nil {
		return false
	}
	return exec.Command("sudo", "-n", "true").Run() == nil
}

// pathExists reports whether a file or directory exists.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// QubesUSBDevice is a device attached to this qube (from sys-usb).
type QubesUSBDevice struct {
	BusID   string `json:"busId"`   // kernel name, e.g. 1-1
	Vendor  string `json:"vendor"`  // idVendor
	Product string `json:"product"` // idProduct
	Name    string `json:"name"`    // manufacturer + product strings, when the device has them
}

// Label is "vvvv:pppp Name".
func (d QubesUSBDevice) Label() string {
	label := d.Vendor + ":" + d.Product
	if d.Name != "" {
		label += " " + d.Name
	}
	return label
}

var usbDeviceNameRe = regexp.MustCompile(`^[0-9]+-[0-9]+(\.[0-9]+)*$`)

// QubesUSBDevices lists the USB devices currently attached to this qube
// (root hubs and interfaces excluded).
func QubesUSBDevices() []QubesUSBDevice {
	entries, err := os.ReadDir(qubesSysUSBDevices)
	if err != nil {
		return nil
	}
	read := func(dir, name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	var out []QubesUSBDevice
	for _, e := range entries {
		if !usbDeviceNameRe.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(qubesSysUSBDevices, e.Name())
		d := QubesUSBDevice{BusID: e.Name(), Vendor: read(dir, "idVendor"), Product: read(dir, "idProduct")}
		d.Name = strings.TrimSpace(read(dir, "manufacturer") + " " + read(dir, "product"))
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BusID < out[j].BusID })
	return out
}

// QubesStreamingTip is the advice for high-rate SDR streaming through
// sys-usb's USB/IP link.
const QubesStreamingTip = "through sys-usb keep USB transfers small: large async buffers (librtlsdr's default 15 x 256 KiB) make the USB proxy drop the device. " +
	"Use rtl_sdr -b 16384, gr-osmosdr/GQRX device string rtl=0,buflen=16384, or sync mode (-S). If it still drops, give sys-usb more memory (dom0: qvm-prefs sys-usb memory/maxmem, then restart it), or assign a USB controller to this qube (qvm-pci) for sustained high rates"

// QubesAttachHint tells how to bring a device into this qube.
func QubesAttachHint() string {
	name := DetectQubes().Name
	if name == "" {
		name = "<this-qube>"
	}
	return fmt.Sprintf("attach it from the Qubes devices tray icon, or in dom0: qvm-usb attach %s sys-usb:<busid> (qvm-usb list shows the bus ids)", name)
}

// QubesCheck is one line of the Qubes status.
type QubesCheck struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"` // what to do, when not ok
}

// QubesStatusInput is what the caller knows that this package cannot look
// up itself (it sits below the engine and config packages).
type QubesStatusInput struct {
	User          string // account RF Swift acts for (InvokingUser when empty)
	PodmanMode    string // configured podman_mode (auto | rootless | rootful)
	WorkspaceRoot string // ~/rfswift-workspace of that user
	Devices       string // configured default devices (config.ini [container] devices)
}

// QubesStatus is the full picture `rfswift host qubes` and the Workbench show.
type QubesStatus struct {
	Info        QubesInfo        `json:"info"`
	Checks      []QubesCheck     `json:"checks"`
	USB         []QubesUSBDevice `json:"usb"`
	Plan        []string         `json:"plan"`        // what the setup would do, in plain words
	Ready       bool             `json:"ready"`       // nothing left for the setup to do
	CanSetup    bool             `json:"canSetup"`    // the setup applies here
	RootOwned   []string         `json:"rootOwned"`   // workspace directories owned by root
	DropDevices []string         `json:"dropDevices"` // default devices this qube does not have
	SetMode     bool             `json:"setMode"`     // the setup would set podman_mode = rootful
	User        string           `json:"user"`
	Detail      string           `json:"detail"`
}

// qubesState is everything the checks and the plan are derived from.
type qubesState struct {
	info            QubesInfo
	in              QubesStatusInput
	group           string
	podman, docker  bool
	binds           map[string]string // configured bind-dirs entry -> file
	mounted         map[string]bool
	busExists       bool
	usb             []QubesUSBDevice
	sock            PodmanSocketAccess
	dropInCurrent   bool
	modulesLoad     bool
	dockerAccess    DockerAccess
	bootExists      bool
	bootCurrent     bool
	rootOwned       []string
	dropDevices     []string
	mode            string
	podmanSocketOpt bool
	nix             bool           // a Nix install is present (/nix)
	nixDaemon       NixDaemonState // its nix-daemon unit
	rtlBlacklisted  bool           // dvb_usb_rtl28xxu is blacklisted
	dockerRunning   bool           // docker.socket or docker.service is active
}

func gatherQubesState(in QubesStatusInput) qubesState {
	if in.User == "" {
		in.User = InvokingUser()
	}
	s := qubesState{info: DetectQubes(), in: in, mounted: map[string]bool{}}
	// Probed here rather than in DetectQubes: it may run `sudo -n true`,
	// which only the status and the setup need (not every device check).
	s.info.PasswordlessRoot = passwordlessRoot()
	s.group = primaryGroup(in.User)
	_, perr := qubesLookPath("podman")
	_, derr := qubesLookPath("docker")
	s.podman, s.docker = perr == nil, derr == nil
	s.nix = pathExists(qubesNixBinary)
	if s.nix {
		s.nixDaemon = GetNixDaemonState()
	}
	s.rtlBlacklisted = dvbBlacklisted()
	s.binds = bindDirsConfigured()
	for _, p := range []string{podmanRootfulStore, dockerRootfulStore, containerdStore, nixStore} {
		s.mounted[p] = isMountPoint(p)
	}
	s.busExists = pathExists(qubesDevBusUSB)
	s.usb = QubesUSBDevices()
	s.sock = ProbeRootfulPodmanSocket()
	if b, err := os.ReadFile(qubesPodmanDropIn); err == nil && s.group != "" {
		s.dropInCurrent = string(b) == QubesPodmanDropInContent(s.group)
	}
	s.modulesLoad = pathExists(qubesModulesLoad)
	if s.docker {
		s.dockerAccess = GetDockerAccess()
		for _, unit := range []string{"docker.socket", "docker.service"} {
			if st, _ := systemctlOutput("is-active", unit); st == "active" {
				s.dockerRunning = true
			}
		}
	}
	s.mode = strings.ToLower(strings.TrimSpace(in.PodmanMode))
	s.podmanSocketOpt = s.podman && s.info.PasswordlessRoot
	opts := s.defaultOptions()
	if b, err := os.ReadFile(qubesBootScriptPath); err == nil {
		s.bootExists = true
		s.bootCurrent = string(b) == QubesBootScriptContent(opts)
	}
	s.rootOwned = rootOwnedWorkspaceDirs(in.WorkspaceRoot)
	s.dropDevices = missingDefaultDevices(in.Devices)
	return s
}

func (s qubesState) defaultOptions() QubesSetupOptions {
	return QubesSetupOptions{
		User:         s.in.User,
		Group:        s.group,
		PodmanSocket: s.podmanSocketOpt,
		DockerGroup:  s.docker,
		DockerStart:  s.docker,
		PersistStore: s.podman || s.docker || s.nix,
		VHCI:         true,
		FixOwnership: s.rootOwned,
		PodmanFound:  s.podman,
		DockerFound:  s.docker,
		NixFound:     s.nix,
		NixDaemon:    s.nix && s.nixDaemon.Unit,
		RTLBlacklist: true,
	}
}

// dvbBlacklisted reports whether a modprobe.d file blacklists the DVB-T
// driver that grabs RTL2832U dongles.
func dvbBlacklisted() bool {
	for _, dir := range qubesModprobeDirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(b), "\n") {
				if f := strings.Fields(line); len(f) == 2 && f[0] == "blacklist" && f[1] == "dvb_usb_rtl28xxu" {
					return true
				}
			}
		}
	}
	return false
}

// nixDaemonUsable reports whether the nix-daemon unit can start: its binary
// exists, or RF Swift's drop-in points it at the Nix profile's daemon.
func (s qubesState) nixDaemonUsable() bool {
	return !s.nixDaemon.Unit || !s.nixDaemon.ExecMissing
}

// podmanUsable reports whether the user can drive the rootful service.
// As the user, the socket probe is the truth; as root (sudo rfswift host
// qubes ...), the drop-in RF Swift installs stands for it.
func (s qubesState) podmanUsable() bool {
	if qubesEUID() == 0 {
		return s.dropInCurrent && pathExists(filepath.Dir(s.sock.Socket))
	}
	return s.sock.Accessible
}

// vhciAtBoot reports whether vhci-hcd is loaded at every start.
func (s qubesState) vhciAtBoot() bool {
	switch {
	case s.info.Persistent():
		return s.modulesLoad
	case s.info.TemplateBased():
		return s.bootCurrent
	}
	return s.busExists // DispVM / custom-persist: this session only
}

// missingBinds are the rootful storage paths not configured in any
// bind-dirs file yet (template-based AppVMs only).
func (s qubesState) missingBinds(o QubesSetupOptions) []string {
	if !s.info.TemplateBased() || !o.PersistStore {
		return nil
	}
	var out []string
	if o.PodmanFound && s.binds[podmanRootfulStore] == "" {
		out = append(out, podmanRootfulStore)
	}
	if o.DockerFound && s.binds[dockerRootfulStore] == "" {
		out = append(out, dockerRootfulStore)
	}
	if o.DockerFound && s.binds[containerdStore] == "" {
		out = append(out, containerdStore)
	}
	if o.NixFound && s.binds[nixStore] == "" {
		out = append(out, nixStore)
	}
	return out
}

// liveStore is a storage path the setup moves to its persistent copy right
// away, with the units to stop while it does and a busy check.
type liveStore struct {
	path  string
	stop  string // units stopped during the switch
	start string // units started again afterwards
	busy  string // shell test that is true when containers run ("" = never busy)
}

// liveStores are the storage paths of this qube that are not on their
// persistent copy yet. Until the first restart with the bind-dirs entry they
// would sit on the root volume of a template-based AppVM, which is the size
// of the template's root (often 20 GB, too small for RF Swift images) and is
// discarded at the restart. Switching now keeps what was pulled or built.
func (s qubesState) liveStores(o QubesSetupOptions) []liveStore {
	if !s.info.TemplateBased() || !o.PersistStore {
		return nil
	}
	var out []liveStore
	if o.PodmanFound && !s.mounted[podmanRootfulStore] {
		out = append(out, liveStore{podmanRootfulStore, "podman.socket podman.service", "podman.socket", "[ -n \"$(podman ps -q 2>/dev/null)\" ]"})
	}
	if o.DockerFound {
		for _, p := range []string{dockerRootfulStore, containerdStore} {
			if !s.mounted[p] {
				out = append(out, liveStore{p, "docker.socket docker.service containerd.service", "docker.socket", "[ -n \"$(docker ps -q 2>/dev/null)\" ]"})
			}
		}
	}
	if o.NixFound && !s.mounted[nixStore] {
		out = append(out, liveStore{nixStore, "nix-daemon.socket nix-daemon.service", "nix-daemon.socket", ""})
	}
	return out
}

// liveBindFunction is the shell function the setup uses to move a storage
// path to bind-dirs' persistent location now: seed the copy from the live
// path (unless it exists), bind-mount it, restart the engine's socket. A
// path whose engine runs containers, or that does not fit in /rw, is left
// for the next restart (bind-dirs then mounts it).
func liveBindFunction() string {
	root := qubesBindDirsRoot
	rw := filepath.Dir(root)
	return "# Switch a storage path to its persistent copy now (what bind-dirs does at\n" +
		"# boot). $1 path, $2 units to stop, $3 units to start again.\n" +
		"rfswift_bind_now() {\n" +
		"  p=$1; copy=" + root + "$1\n" +
		"  if mountpoint -q \"$p\"; then return 0; fi\n" +
		"  if [ ! -e \"$copy\" ] && [ -e \"$p\" ]; then\n" +
		"    need=$(du -sk \"$p\" | cut -f1); free=$(df -Pk " + rw + " | awk 'NR==2{print $4}')\n" +
		"    if [ \"$need\" -ge \"$free\" ]; then echo \"rfswift: not enough room in " + rw + " to move $p now; it switches at the next restart\" >&2; return 0; fi\n" +
		"  fi\n" +
		"  systemctl stop $2 2>/dev/null\n" +
		"  fresh=\n" +
		"  if [ ! -e \"$copy\" ]; then\n" +
		"    mkdir -p \"$(dirname \"$copy\")\"\n" +
		"    if [ -e \"$p\" ]; then cp -a \"$p\" \"$copy.tmp\" && mv \"$copy.tmp\" \"$copy\"; else mkdir -p \"$copy\"; fresh=1; fi\n" +
		"  fi\n" +
		"  mkdir -p \"$p\" && mount --bind \"$copy\" \"$p\"\n" +
		"  if [ -n \"$fresh\" ] && command -v restorecon >/dev/null 2>&1; then restorecon -R \"$p\" 2>/dev/null; fi\n" +
		"  systemctl start $3 2>/dev/null\n" +
		"}\n"
}

// liveBindScript calls liveBindFunction for each store, skipping busy ones.
func liveBindScript(stores []liveStore) string {
	if len(stores) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(liveBindFunction())
	for _, st := range stores {
		call := fmt.Sprintf("rfswift_bind_now %s '%s' '%s'", st.path, st.stop, st.start)
		if st.busy != "" {
			fmt.Fprintf(&b, "if %s; then echo 'rfswift: containers are running, %s switches to its persistent copy at the next restart' >&2; else %s; fi\n", st.busy, st.path, call)
		} else {
			b.WriteString(call + "\n")
		}
	}
	return b.String()
}

// ownBinds are the entries of RF Swift's own bind-dirs file, kept when the
// file is written again (it is rewritten whole).
func (s qubesState) ownBinds() []string {
	var out []string
	for p, f := range s.binds {
		if filepath.Base(f) == filepath.Base(QubesBindDirsFile) {
			out = append(out, p)
		}
	}
	return out
}

// needsBootScript reports whether the AppVM boot script must be written.
func (s qubesState) needsBootScript(o QubesSetupOptions) bool {
	if !s.info.TemplateBased() || !(o.VHCI || o.PodmanSocket || o.DockerGroup || o.DockerStart || o.NixDaemon || o.RTLBlacklist) {
		return false
	}
	b, err := os.ReadFile(qubesBootScriptPath)
	return err != nil || string(b) != QubesBootScriptContent(o)
}

// plan lists, in plain words, what the setup would change.
func (s qubesState) plan(o QubesSetupOptions) []string {
	var plan []string
	if o.VHCI && (!s.busExists || !s.vhciAtBoot()) {
		plan = append(plan, "load vhci-hcd so /dev/bus/usb exists before any device is attached"+s.everyBoot())
	}
	for _, p := range s.missingBinds(o) {
		plan = append(plan, "keep "+p+" across restarts (bind-dirs)")
	}
	if live := s.liveStores(o); len(live) > 0 {
		paths := make([]string, 0, len(live))
		for _, l := range live {
			paths = append(paths, l.path)
		}
		plan = append(plan, "switch "+strings.Join(paths, ", ")+" to the persistent copy now, keeping what is there (left for the next restart while containers run)")
	}
	if o.PodmanSocket && (!s.podmanUsable() || (s.info.Persistent() && !s.dropInCurrent)) {
		plan = append(plan, "make "+s.sock.Socket+" usable by group "+o.Group+s.everyBoot())
	}
	if o.PodmanSocket && s.mode != "rootful" {
		plan = append(plan, "set podman_mode = rootful in config.ini: rfswift and the Workbench run as "+o.User+" against the rootful service")
	}
	if o.DockerStart && s.docker && !s.dockerRunning {
		plan = append(plan, "start Docker (docker.socket, the daemon starts on first use)"+s.everyBoot())
	}
	if o.DockerGroup && s.docker && !s.dockerAccess.Ready {
		plan = append(plan, "give "+o.User+" access to the Docker socket (docker group + an ACL re-applied whenever the socket is created)"+s.everyBoot())
	}
	if o.RTLBlacklist && !s.rtlBlacklisted {
		plan = append(plan, "keep the DVB-T driver (dvb_usb_rtl28xxu) off RTL-SDR dongles"+s.everyBoot())
	}
	if o.NixDaemon && !s.nixDaemonUsable() {
		plan = append(plan, "run the Nix daemon from "+nixProfileDaemon+" ("+s.nixDaemon.ExecStart+" is not in this qube)"+s.everyBoot())
	}
	if s.needsBootScript(o) {
		plan = append(plan, "install "+qubesBootScriptPath+" to re-apply the above at every start")
	}
	if len(o.FixOwnership) > 0 {
		plan = append(plan, fmt.Sprintf("give %d root-owned workspace folder(s) back to %s", len(o.FixOwnership), o.User))
	}
	if len(s.dropDevices) > 0 {
		plan = append(plan, "drop "+strings.Join(s.dropDevices, ", ")+" from the default devices in config.ini (no GPU in this qube)")
	}
	return plan
}

func (s qubesState) everyBoot() string {
	if s.info.TemplateBased() {
		return " (at every boot)"
	}
	if !s.info.Persistent() {
		return " (this session only)"
	}
	return ""
}

// bindDirsConfigured returns the bind-dirs entries configured in any
// bind-dirs.d file (Qubes' own, the template's and this qube's).
func bindDirsConfigured() map[string]string {
	found := map[string]string{}
	re := regexp.MustCompile(`binds\+=\(\s*(.*?)\s*\)`)
	for _, dir := range qubesBindDirsDirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
		for _, f := range files {
			fh, err := os.Open(f)
			if err != nil {
				continue
			}
			sc := bufio.NewScanner(fh)
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if strings.HasPrefix(line, "#") {
					continue
				}
				for _, m := range re.FindAllStringSubmatch(line, -1) {
					for _, p := range strings.Fields(m[1]) {
						p = filepath.Clean(strings.Trim(p, `'"`))
						if strings.HasPrefix(p, "/") && found[p] == "" {
							found[p] = f
						}
					}
				}
			}
			fh.Close()
		}
	}
	return found
}

// isMountPoint reports whether path is a mount point (bind-dirs mounts the
// /rw copy over it at boot).
func isMountPoint(path string) bool {
	b, err := os.ReadFile(qubesMountInfo)
	if err != nil {
		return false
	}
	path = filepath.Clean(path)
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) > 4 && unescapeMountPath(f[4]) == path {
			return true
		}
	}
	return false
}

func unescapeMountPath(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}

// rootOwnedWorkspaceDirs lists the workspace root and its first-level
// folders that belong to root: created by a `sudo rfswift run` of an older
// release. The user cannot drop files into them from the host.
func rootOwnedWorkspaceDirs(root string) []string {
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	consider := []string{root}
	for _, e := range entries {
		if e.IsDir() {
			consider = append(consider, filepath.Join(root, e.Name()))
		}
	}
	var out []string
	for _, p := range consider {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() {
			continue
		}
		if uid, ok := fileUID(info); ok && uid == 0 {
			out = append(out, p)
		}
	}
	return out
}

// missingDefaultDevices returns the configured default device mappings this
// qube does not have and never will without a dom0 passthrough (/dev/dri:
// qubes have no GPU unless one is assigned). /dev/bus/usb is not one of
// them: the setup makes it exist.
func missingDefaultDevices(devices string) []string {
	var out []string
	for _, spec := range strings.Split(devices, ",") {
		spec = strings.TrimSpace(spec)
		host := strings.SplitN(spec, ":", 2)[0]
		if host == "/dev/dri" && !pathExists(qubesDevDRI) {
			out = append(out, spec)
		}
	}
	return out
}

// DropDevices removes the given mappings from a comma-separated device list.
func DropDevices(devices string, drop []string) string {
	skip := map[string]bool{}
	for _, d := range drop {
		skip[strings.TrimSpace(d)] = true
	}
	var kept []string
	for _, spec := range strings.Split(devices, ",") {
		spec = strings.TrimSpace(spec)
		if spec == "" || skip[spec] {
			continue
		}
		kept = append(kept, spec)
	}
	return strings.Join(kept, ",")
}

// primaryGroup returns the name of a user's primary group.
func primaryGroup(username string) string {
	u, err := user.Lookup(username)
	if err != nil {
		return ""
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		return ""
	}
	return g.Name
}

// GetQubesStatus inspects the qube; never changes anything.
func GetQubesStatus(in QubesStatusInput) QubesStatus {
	info := DetectQubes()
	st := QubesStatus{Info: info, Checks: []QubesCheck{}, Plan: []string{}, USB: []QubesUSBDevice{}, RootOwned: []string{}, DropDevices: []string{}}
	if !info.IsQubes {
		st.Detail = "not a Qubes OS qube"
		return st
	}
	s := gatherQubesState(in)
	info = s.info
	st.Info = info
	st.User = s.in.User
	add := func(c QubesCheck) { st.Checks = append(st.Checks, c) }

	// The qube itself.
	persistNote := map[string]string{
		"full":    "fully persistent: changes to the root filesystem stay",
		"rw-only": "template-based: only /home, /rw and /usr/local survive a restart",
		"none":    "disposable: everything is discarded when it shuts down",
	}[info.Persistence]
	add(QubesCheck{ID: "qube", Name: "Qubes OS qube", Status: "ok", Detail: fmt.Sprintf("%s (%s, %s)", info.Name, info.Type, persistNote)})
	if info.CustomPersist {
		add(QubesCheck{ID: "custom-persist", Name: "custom-persist", Status: "warn",
			Detail: "the custom-persist feature is on: /rw/config (bind-dirs, rc.local) is ignored, so nothing can be made persistent from inside the qube",
			Fix:    fmt.Sprintf("in dom0: qvm-features %s custom-persist.rfswift-podman /var/lib/containers (and custom-persist.rfswift-docker /var/lib/docker), and run the setup again after each start", info.Name)})
	}
	if info.PasswordlessRoot {
		add(QubesCheck{ID: "root", Name: "Root in the qube", Status: "ok", Detail: "passwordless (qubes-core-agent-passwordless-root): the qube is the security boundary"})
	} else {
		add(QubesCheck{ID: "root", Name: "Root in the qube", Status: "warn",
			Detail: "root needs a password in this qube: granting the rootful Podman socket would give " + s.in.User + " root without one",
			Fix:    "the setup leaves the socket alone here unless you pass --podman-socket"})
	}

	// USB.
	st.USB = s.usb
	if st.USB == nil {
		st.USB = []QubesUSBDevice{}
	}
	if info.USBProxy {
		add(QubesCheck{ID: "usb-proxy", Name: "USB proxy", Status: "ok", Detail: "qubes-usb-proxy installed: sys-usb can attach devices to this qube"})
	} else {
		add(QubesCheck{ID: "usb-proxy", Name: "USB proxy", Status: "fail", Detail: "qubes-usb-proxy is not installed: qvm-usb cannot attach SDR/USB devices to this qube",
			Fix: "install qubes-usb-proxy in the template (dnf install qubes-usb-proxy / apt install qubes-usb-proxy), then restart this qube"})
	}
	switch {
	case s.busExists && len(s.usb) > 0:
		labels := make([]string, 0, len(s.usb))
		for _, d := range s.usb {
			labels = append(labels, d.Label())
		}
		add(QubesCheck{ID: "usb-bus", Name: "USB devices", Status: "ok", Detail: fmt.Sprintf("%d attached: %s", len(s.usb), strings.Join(labels, ", "))})
	case s.busExists:
		add(QubesCheck{ID: "usb-bus", Name: "USB devices", Status: "ok", Detail: "virtual USB bus ready (vhci-hcd), nothing attached yet: " + QubesAttachHint()})
	default:
		add(QubesCheck{ID: "usb-bus", Name: "USB devices", Status: "warn",
			Detail: "/dev/bus/usb does not exist until a first device is attached: containers mapping it fail to start, and devices attached later never appear in containers started before",
			Fix:    "rfswift host qubes setup loads vhci-hcd (the virtual bus sys-usb attaches through) at boot; or " + QubesAttachHint()})
	}
	if len(s.usb) > 0 {
		// Measured with an RTL2832U through sys-usb: sync reads and async
		// reads with 16 KiB transfers stream fine; librtlsdr's default async
		// setup (15 x 256 KiB zero-copy transfers in flight) makes the USB
		// proxy close the connection and the device is detached.
		add(QubesCheck{ID: "usb-streaming", Name: "SDR streaming tip", Status: "ok",
			Detail: QubesStreamingTip})
	}
	if s.busExists && !s.vhciAtBoot() && info.Persistence != "none" {
		add(QubesCheck{ID: "usb-boot", Name: "USB bus at boot", Status: "warn",
			Detail: "/dev/bus/usb exists in this session only (vhci-hcd loaded by a device attach or by hand); after a restart it is gone again until a device is attached",
			Fix:    "rfswift host qubes setup loads vhci-hcd at every start"})
	}

	// RTL-SDR dongles and the DVB-T driver.
	if s.rtlBlacklisted {
		add(QubesCheck{ID: "rtl-dvb", Name: "RTL-SDR kernel driver", Status: "ok", Detail: "dvb_usb_rtl28xxu is blacklisted: RTL-SDR dongles stay free for librtlsdr"})
	} else {
		add(QubesCheck{ID: "rtl-dvb", Name: "RTL-SDR kernel driver", Status: "warn",
			Detail: "the DVB-T driver (dvb_usb_rtl28xxu) claims RTL-SDR dongles on every attach: native tools (Nix environments) fail with 'usb_claim_interface error -6' (container images detach it themselves)",
			Fix:    "rfswift host qubes setup blacklists it" + s.everyBoot() + " (--no-rtl-blacklist keeps it for DVB-T TV use)"})
	}

	// Engine storage persistence.
	type store struct{ id, engine, path string }
	var stores []store
	if s.podman {
		stores = append(stores, store{"storage-podman", "Podman (rootful)", podmanRootfulStore})
	}
	if s.docker {
		stores = append(stores, store{"storage-docker", "Docker", dockerRootfulStore}, store{"storage-containerd", "Docker images (containerd)", containerdStore})
	}
	for _, x := range stores {
		name := x.engine + " storage"
		switch {
		case info.Persistent():
			add(QubesCheck{ID: x.id, Name: name, Status: "ok", Detail: x.path + " is persistent in a " + info.Type})
		case info.Persistence == "none":
			add(QubesCheck{ID: x.id, Name: name, Status: "warn", Detail: x.path + " is discarded with this disposable qube", Fix: "pull images in its disposable template's AppVM instead"})
		case s.mounted[x.path]:
			add(QubesCheck{ID: x.id, Name: name, Status: "ok", Detail: x.path + " persists (bind-dirs: " + s.binds[x.path] + ")"})
		case s.binds[x.path] != "":
			add(QubesCheck{ID: x.id, Name: name, Status: "warn", Detail: x.path + " is listed in " + s.binds[x.path] + " but still on the root volume", Fix: "rfswift host qubes setup switches it now (when no container runs), or restart this qube"})
		default:
			add(QubesCheck{ID: x.id, Name: name, Status: "warn", Detail: x.path + " is on the root volume: images and containers are discarded at every restart of this qube (and the root volume is as small as the template's)",
				Fix: "rfswift host qubes setup lists it in " + QubesBindDirsFile + " and switches to the persistent copy right away"})
		}
	}

	// Rootful Podman for the desktop user.
	if s.podman {
		usable := s.podmanUsable()
		switch {
		case qubesEUID() == 0 && s.in.User == "root":
			add(QubesCheck{ID: "podman-access", Name: "Rootful Podman", Status: "ok", Detail: "running as root"})
		case usable && s.mode == "rootful":
			add(QubesCheck{ID: "podman-access", Name: "Rootful Podman", Status: "ok", Detail: "podman_mode = rootful and " + s.sock.Socket + " is usable by " + s.in.User + ": rfswift and the Workbench run without sudo and see the rootful containers"})
		case usable:
			add(QubesCheck{ID: "podman-access", Name: "Rootful Podman", Status: "warn", Detail: s.sock.Socket + " is usable by " + s.in.User + " but podman_mode is " + orAuto(s.mode) + ": RF Swift still drives rootless Podman",
				Fix: "rfswift host qubes setup (or set podman_mode = rootful in config.ini)"})
		default:
			add(QubesCheck{ID: "podman-access", Name: "Rootful Podman", Status: "warn", Detail: s.sock.Socket + " is root-only: USB work needs 'sudo rfswift', and the Workbench only sees rootless containers",
				Fix: "rfswift host qubes setup makes the socket usable by " + s.in.User + " and sets podman_mode = rootful"})
		}
	}
	if s.docker {
		if s.dockerRunning {
			add(QubesCheck{ID: "docker-service", Name: "Docker service", Status: "ok", Detail: "docker.socket is listening (the daemon starts on first use)"})
		} else {
			add(QubesCheck{ID: "docker-service", Name: "Docker service", Status: "warn",
				Detail: "Docker is installed but neither docker.socket nor docker.service runs (not enabled in the template, or installed in this qube only)",
				Fix:    "rfswift host qubes setup starts it" + s.everyBoot()})
		}
		if s.dockerAccess.Ready {
			add(QubesCheck{ID: "docker-access", Name: "Docker access", Status: "ok", Detail: s.dockerAccess.Detail})
		} else {
			add(QubesCheck{ID: "docker-access", Name: "Docker access", Status: "warn", Detail: s.dockerAccess.Detail,
				Fix: "rfswift host qubes setup gives " + s.in.User + " the Docker socket (docker group + ACL)" + s.everyBoot()})
		}
	}

	// Nix engine (native environments).
	if s.nix {
		d := s.nixDaemon
		switch {
		case !d.Unit:
			add(QubesCheck{ID: "nix-daemon", Name: "Nix daemon", Status: "ok", Detail: "single-user Nix install (no daemon)"})
		case d.ExecMissing:
			add(QubesCheck{ID: "nix-daemon", Name: "Nix daemon", Status: "warn",
				Detail: "nix-daemon.service runs " + d.ExecStart + ", which this qube does not have (Nix was installed in the template, and an AppVM's /usr/local is its own): the Nix engine cannot reach its store",
				Fix:    "rfswift host qubes setup runs the daemon of the Nix profile (" + nixProfileDaemon + ") instead" + s.everyBoot()})
		case d.Active == "failed":
			add(QubesCheck{ID: "nix-daemon", Name: "Nix daemon", Status: "warn", Detail: "nix-daemon.service failed", Fix: "sudo systemctl restart nix-daemon.service (see journalctl -u nix-daemon)"})
		case d.DropIn:
			add(QubesCheck{ID: "nix-daemon", Name: "Nix daemon", Status: "ok", Detail: "runs " + nixProfileDaemon + " (the template's " + "determinate-nixd is not visible in this AppVM)"})
		default:
			add(QubesCheck{ID: "nix-daemon", Name: "Nix daemon", Status: "ok", Detail: "nix-daemon.service: " + orUnknown(d.Active) + " (" + d.ExecStart + ")"})
		}
		switch {
		case s.info.Persistent():
			add(QubesCheck{ID: "nix-store", Name: "Nix store", Status: "ok", Detail: nixStore + " is persistent in a " + s.info.Type})
		case s.info.Persistence == "none":
			add(QubesCheck{ID: "nix-store", Name: "Nix store", Status: "warn", Detail: nixStore + " is discarded with this disposable qube", Fix: "create Nix environments in its disposable template's AppVM instead"})
		case s.mounted[nixStore]:
			add(QubesCheck{ID: "nix-store", Name: "Nix store", Status: "ok", Detail: nixStore + " persists (bind-dirs: " + s.binds[nixStore] + ")"})
		case s.binds[nixStore] != "":
			add(QubesCheck{ID: "nix-store", Name: "Nix store", Status: "warn", Detail: nixStore + " is listed in " + s.binds[nixStore] + " but still on the root volume", Fix: "rfswift host qubes setup switches it to the persistent copy now, keeping the environments built so far"})
		default:
			add(QubesCheck{ID: "nix-store", Name: "Nix store", Status: "warn",
				Detail: nixStore + " is on the root volume: Nix environments built in this AppVM are discarded at every restart (their manifests in your home then point at nothing)",
				Fix:    "rfswift host qubes setup lists it in " + QubesBindDirsFile + " and switches to the persistent copy right away, keeping the environments built so far (the template's later Nix upgrades then stop reaching this qube)"})
		}
	}

	// Boot script (AppVM).
	if info.TemplateBased() {
		switch {
		case s.bootCurrent:
			add(QubesCheck{ID: "boot", Name: "Boot script", Status: "ok", Detail: qubesBootScriptPath + " re-applies the setup at every start"})
		case s.bootExists:
			add(QubesCheck{ID: "boot", Name: "Boot script", Status: "warn", Detail: qubesBootScriptPath + " differs from what this release writes (older RF Swift, or edited)", Fix: "rfswift host qubes setup rewrites it"})
		default:
			add(QubesCheck{ID: "boot", Name: "Boot script", Status: "warn", Detail: "none: whatever is set up outside /home, /rw and /usr/local is lost at the next restart", Fix: "rfswift host qubes setup installs " + qubesBootScriptPath})
		}
	}

	// Room for images: in an AppVM bind-dirs keeps the engines' storage and
	// the Nix store on the private volume, which Qubes creates small.
	if (s.podman || s.docker || s.nix) && info.Persistence != "none" {
		path, volume := "/", "root"
		if info.TemplateBased() {
			path, volume = filepath.Dir(qubesBindDirsRoot), "private"
		}
		if free, err := qubesDiskFree(path); err == nil {
			gib := float64(free) / (1 << 30)
			fix := fmt.Sprintf("in dom0: qvm-volume extend %s:%s 60G (or Qube Settings > Disk storage)", info.Name, volume)
			if gib < qubesMinFreeGiB {
				add(QubesCheck{ID: "disk", Name: "Disk space", Status: "warn",
					Detail: fmt.Sprintf("%.1f GiB free on the %s volume, where the engines' images and the Nix store live: RF Swift images take 8 to 18 GB each", gib, volume),
					Fix:    fix})
			} else {
				add(QubesCheck{ID: "disk", Name: "Disk space", Status: "ok", Detail: fmt.Sprintf("%.0f GiB free on the %s volume (images and Nix store)", gib, volume)})
			}
		}
	}

	// Workspaces.
	st.RootOwned = s.rootOwned
	if st.RootOwned == nil {
		st.RootOwned = []string{}
	}
	switch {
	case s.in.WorkspaceRoot == "":
	case len(s.rootOwned) > 0:
		add(QubesCheck{ID: "workspace", Name: "Workspaces", Status: "warn",
			Detail: fmt.Sprintf("%d workspace folder(s) belong to root (an earlier sudo run): %s", len(s.rootOwned), strings.Join(shortList(s.rootOwned, 4), ", ")),
			Fix:    "rfswift host qubes setup gives them back to " + s.in.User})
	default:
		add(QubesCheck{ID: "workspace", Name: "Workspaces", Status: "ok", Detail: s.in.WorkspaceRoot + " (in /home, persistent; /workspace in the containers)"})
	}

	// Devices this qube cannot have.
	st.DropDevices = s.dropDevices
	if st.DropDevices == nil {
		st.DropDevices = []string{}
	}
	if len(s.dropDevices) > 0 {
		add(QubesCheck{ID: "devices", Name: "Default devices", Status: "warn",
			Detail: "config.ini maps " + strings.Join(s.dropDevices, ", ") + ", which this qube does not have (no GPU without a dom0 passthrough): every run warns that it skips it",
			Fix:    "rfswift host qubes setup removes it from the defaults"})
	}

	opts := s.defaultOptions()
	st.Plan = s.plan(opts)
	if st.Plan == nil {
		st.Plan = []string{}
	}
	st.SetMode = opts.PodmanSocket && s.mode != "rootful"
	st.CanSetup = true
	st.Ready = len(st.Plan) == 0
	for _, c := range st.Checks {
		if c.Status == "fail" {
			st.Ready = false
		}
	}
	if st.Ready {
		st.Detail = "RF Swift is set up for this qube"
	} else {
		st.Detail = fmt.Sprintf("%d step(s) left: rfswift host qubes setup", len(st.Plan))
	}
	return st
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func orAuto(mode string) string {
	if mode == "" {
		return "auto"
	}
	return mode
}

func shortList(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return append(append([]string{}, items[:n]...), fmt.Sprintf("and %d more", len(items)-n))
}

// QubesSetupOptions selects what the setup does.
type QubesSetupOptions struct {
	User         string   `json:"user"`         // account to grant (InvokingUser)
	Group        string   `json:"group"`        // group owning the rootful Podman socket (user's primary group)
	PodmanSocket bool     `json:"podmanSocket"` // make the rootful Podman socket usable by Group
	DockerGroup  bool     `json:"dockerGroup"`  // give User the Docker socket: docker group + ACL drop-in (at every boot in an AppVM)
	DockerStart  bool     `json:"dockerStart"`  // start docker.socket (at every boot in an AppVM; enabled in a TemplateVM/StandaloneVM)
	PersistStore bool     `json:"persistStore"` // bind-dirs for the rootful engines' storage
	VHCI         bool     `json:"vhci"`         // load vhci-hcd at boot so /dev/bus/usb always exists
	FixOwnership []string `json:"fixOwnership"` // workspace directories to give back to User
	PodmanFound  bool     `json:"podmanFound"`  // podman is installed
	DockerFound  bool     `json:"dockerFound"`  // docker is installed
	NixFound     bool     `json:"nixFound"`     // a Nix install is present (/nix)
	NixDaemon    bool     `json:"nixDaemon"`    // repair nix-daemon.service when its binary is missing
	RTLBlacklist bool     `json:"rtlBlacklist"` // blacklist the DVB-T driver that grabs RTL-SDR dongles
}

// DefaultQubesSetupOptions is what `rfswift host qubes setup` does without
// flags: everything that applies to this qube. The rootful Podman socket is
// only granted where root is passwordless already.
func DefaultQubesSetupOptions(in QubesStatusInput) QubesSetupOptions {
	return gatherQubesState(in).defaultOptions()
}

// validateQubesOptions refuses names the scripts could not hold safely.
func validateQubesOptions(o QubesSetupOptions) error {
	if !ValidUserName(o.User) || o.User == "root" {
		return fmt.Errorf("invalid user %q: run the setup as (or with sudo from) the desktop user", o.User)
	}
	if o.PodmanSocket && !ValidGroupName(o.Group) {
		return fmt.Errorf("invalid group name %q for the Podman socket", o.Group)
	}
	for _, p := range o.FixOwnership {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return fmt.Errorf("workspace path %q is not a clean absolute path", p)
		}
	}
	return nil
}

// QubesPodmanDropInContent is the podman.socket drop-in: the socket owned by
// the group, and /run/podman searchable by it (tmpfiles.d creates it 0700 at
// boot, before the socket is bound).
func QubesPodmanDropInContent(group string) string {
	return qubesSetupMarker + ": the rootful Podman socket usable by group " + group + ".\n" +
		"[Socket]\n" +
		"SocketMode=0660\n" +
		"SocketGroup=" + group + "\n" +
		"ExecStartPost=/bin/sh -c 'chgrp " + group + " /run/podman && chmod 0710 /run/podman'\n"
}

// qubesRuntimeScript is the part of the setup that lives outside the
// persistent volume of an AppVM: re-applied at every boot, and run once
// right away. Idempotent. No set -e: one missing piece (docker removed from
// the template) must not stop the rest.
func qubesRuntimeScript(o QubesSetupOptions) string {
	var b strings.Builder
	if o.VHCI {
		b.WriteString("# Virtual USB host controller sys-usb attaches devices through: loaded\n")
		b.WriteString("# up front, /dev/bus/usb exists before the first device is attached.\n")
		b.WriteString("modprobe vhci-hcd 2>/dev/null || true\n")
	}
	if o.PodmanSocket {
		b.WriteString("# Rootful Podman for the desktop user (podman_mode = rootful).\n")
		b.WriteString("if command -v podman >/dev/null 2>&1; then\n")
		fmt.Fprintf(&b, "  mkdir -p %s\n", filepath.Dir(qubesPodmanDropIn))
		fmt.Fprintf(&b, "  cat > %s <<'RFSWIFT_EOF'\n%sRFSWIFT_EOF\n", qubesPodmanDropIn, QubesPodmanDropInContent(o.Group))
		b.WriteString("  systemctl daemon-reload\n")
		b.WriteString("  systemctl restart podman.socket\n")
		b.WriteString("fi\n")
	}
	if o.RTLBlacklist {
		b.WriteString("# RTL-SDR dongles: keep the DVB-T driver off them so librtlsdr can claim\n")
		b.WriteString("# them (nixpkgs' rtl-sdr does not detach kernel drivers by itself).\n")
		fmt.Fprintf(&b, "printf 'blacklist dvb_usb_rtl28xxu\\n' > %s\n", qubesRTLBlacklist)
		b.WriteString("modprobe -r dvb_usb_rtl28xxu 2>/dev/null || true\n")
	}
	if o.NixDaemon {
		b.WriteString(nixDaemonBootScript())
	}
	if o.DockerGroup || o.DockerStart {
		b.WriteString(dockerScript(o))
	}
	return b.String()
}

// QubesDockerDropInContent makes docker.socket re-apply the user's ACL each
// time the socket is created: membership of the docker group only counts
// from the next login, and the plain ACL set once would be lost with the
// socket when the daemon restarts.
func QubesDockerDropInContent(user string) string {
	return qubesSetupMarker + ": " + DockerSocket + " usable by " + user + " whenever the socket is created.\n" +
		"[Socket]\n" +
		"ExecStartPost=/bin/sh -c 'command -v setfacl >/dev/null 2>&1 && setfacl -m u:" + user + ":rw " + DockerSocket + " || true'\n"
}

// dockerScript starts Docker's socket (never restarts it: running containers
// would stop) and gives the user access to it. Guarded so a template without
// Docker any more is fine.
func dockerScript(o QubesSetupOptions) string {
	q := ShellQuote(o.User)
	var b strings.Builder
	b.WriteString("# Docker: start its socket (the daemon starts on first use) and give the\n")
	b.WriteString("# desktop user access to it. In an AppVM /etc and the group database are\n")
	b.WriteString("# reset at every start.\n")
	b.WriteString("if command -v dockerd >/dev/null 2>&1; then\n")
	if o.DockerGroup {
		fmt.Fprintf(&b, "  mkdir -p %s\n", filepath.Dir(qubesDockerDropIn))
		fmt.Fprintf(&b, "  cat > %s <<'RFSWIFT_EOF'\n%sRFSWIFT_EOF\n", qubesDockerDropIn, QubesDockerDropInContent(o.User))
		b.WriteString("  systemctl daemon-reload\n")
		fmt.Fprintf(&b, "  if getent group %s >/dev/null 2>&1; then usermod -aG %s %s; fi\n", DockerGroup, DockerGroup, q)
	}
	if o.DockerStart {
		b.WriteString("  systemctl start docker.socket 2>/dev/null || systemctl start docker.service\n")
	}
	if o.DockerGroup {
		fmt.Fprintf(&b, "  if command -v setfacl >/dev/null 2>&1 && [ -S %s ]; then setfacl -m u:%s:rw %s; fi\n", ShellQuote(DockerSocket), q, ShellQuote(DockerSocket))
	}
	b.WriteString("fi\n")
	return b.String()
}

// QubesBootScriptContent is the /rw/config/rc.local.d script of an AppVM.
func QubesBootScriptContent(o QubesSetupOptions) string {
	return "#!/bin/sh\n" + qubesSetupMarker + ".\n" +
		"# Re-applies RF Swift's Qubes OS integration at every start of this qube\n" +
		"# (only /home, /rw and /usr/local persist in an AppVM). Delete this file\n" +
		"# to undo it from the next restart.\n" + qubesRuntimeScript(o)
}

// QubesSetupScript builds the root script for this qube. The persistence
// part depends on the qube: bind-dirs and a boot script in an AppVM, /etc
// directly in a TemplateVM or StandaloneVM, this session only in a DispVM
// (or with custom-persist). "" when there is nothing to do as root.
func QubesSetupScript(in QubesStatusInput, o QubesSetupOptions) (string, error) {
	s := gatherQubesState(in)
	if !s.info.IsQubes {
		return "", errors.New("not a Qubes OS qube")
	}
	if err := validateQubesOptions(o); err != nil {
		return "", err
	}
	return s.script(o), nil
}

func (s qubesState) script(o QubesSetupOptions) string {
	var b strings.Builder
	runtimePart := qubesRuntimeScript(o)
	switch {
	case s.info.TemplateBased():
		if missing := s.missingBinds(o); len(missing) > 0 {
			// The file is rewritten whole: keep the entries it already has.
			entries := append(s.ownBinds(), missing...)
			sort.Strings(entries)
			b.WriteString("mkdir -p " + filepath.Dir(QubesBindDirsFile) + "\n")
			fmt.Fprintf(&b, "cat > %s <<'RFSWIFT_EOF'\n%s: keep the rootful engines' storage and the Nix store across restarts.\n", QubesBindDirsFile, qubesSetupMarker)
			seen := map[string]bool{}
			for _, p := range entries {
				if !seen[p] {
					seen[p] = true
					fmt.Fprintf(&b, "binds+=( '%s' )\n", p)
				}
			}
			b.WriteString("RFSWIFT_EOF\n")
		}
		b.WriteString(liveBindScript(s.liveStores(o)))
		if runtimePart != "" {
			if s.needsBootScript(o) {
				b.WriteString("mkdir -p " + filepath.Dir(qubesBootScriptPath) + "\n")
				fmt.Fprintf(&b, "cat > %s <<'RFSWIFT_BOOT_EOF'\n%sRFSWIFT_BOOT_EOF\n", qubesBootScriptPath, QubesBootScriptContent(o))
				fmt.Fprintf(&b, "chmod 0755 %s\n", qubesBootScriptPath)
			}
			if len(s.plan(o)) > 0 {
				fmt.Fprintf(&b, "%s\n", qubesBootScriptPath)
			}
		}
	case s.info.Persistent():
		if o.VHCI && !s.modulesLoad {
			fmt.Fprintf(&b, "mkdir -p %s\nprintf 'vhci-hcd\\n' > %s\n", filepath.Dir(qubesModulesLoad), qubesModulesLoad)
		}
		b.WriteString(runtimePart)
		if o.PodmanSocket {
			b.WriteString("if command -v podman >/dev/null 2>&1; then systemctl enable podman.socket; fi\n")
		}
		if o.DockerStart {
			b.WriteString("if command -v dockerd >/dev/null 2>&1; then systemctl enable docker.socket; fi\n")
		}
	default:
		b.WriteString(runtimePart)
	}
	if len(o.FixOwnership) > 0 {
		owner := ShellQuote(o.User)
		if g := primaryGroup(o.User); ValidGroupName(g) {
			owner = ShellQuote(o.User + ":" + g)
		}
		for _, p := range o.FixOwnership {
			fmt.Fprintf(&b, "chown %s %s\n", owner, ShellQuote(p))
		}
	}
	return b.String()
}

// QubesSetupReport is what RunQubesSetup did.
type QubesSetupReport struct {
	Script  string      `json:"script"`  // the root script that ran ("" when none was needed)
	Status  QubesStatus `json:"status"`  // state after the change
	Restart bool        `json:"restart"` // part of it (bind-dirs) applies from the next restart
	Notes   []string    `json:"notes"`
	Detail  string      `json:"detail"`
}

// RunQubesSetup runs the root part of the setup. The caller handles the
// user's own config.ini (podman_mode, default devices) and passes the same
// input it gave GetQubesStatus.
func RunQubesSetup(in QubesStatusInput, o QubesSetupOptions) (QubesSetupReport, error) {
	var report QubesSetupReport
	s := gatherQubesState(in)
	if !s.info.IsQubes {
		return report, errors.New("not a Qubes OS qube")
	}
	if err := validateQubesOptions(o); err != nil {
		return report, err
	}
	live := s.liveStores(o)
	report.Script = s.script(o)
	if strings.TrimSpace(report.Script) != "" {
		if err := RunPrivileged(report.Script); err != nil {
			return report, fmt.Errorf("the Qubes setup script failed: %w", err)
		}
	}
	// The script switches each store now unless its engine runs containers
	// or /rw lacks room: report the ones still waiting for a restart.
	for _, l := range live {
		if !isMountPoint(l.path) {
			report.Restart = true
			report.Notes = append(report.Notes, l.path+" switches to its persistent copy at the next restart (containers were running, or /rw lacked room): what is stored there until then is discarded")
		}
	}
	if !s.info.Persistent() && !s.info.TemplateBased() {
		report.Notes = append(report.Notes, "applied for this session only: a "+s.info.Type+" keeps nothing from the inside")
	}
	report.Status = GetQubesStatus(in)
	report.Detail = "Qubes OS integration applied"
	return report, nil
}
