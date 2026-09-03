/* This code is part of RF Swift by @Penthertz
 * Author(s): Sebastien Dudek (@FlUxIuS)
 */
package dock

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/moby/moby/client"
	common "penthertz/rfswift/common"
	"penthertz/rfswift/hostsetup"
	rfutils "penthertz/rfswift/rfutils"
	"penthertz/rfswift/tui"
)

// CheckResult holds the outcome of a single diagnostic check.
type CheckResult struct {
	Name    string
	Status  string // "ok", "warn", "fail", "skip"
	Message string
}

// DoctorReport aggregates all diagnostic results.
type DoctorReport struct {
	Results []CheckResult
	pass    int
	warn    int
	fail    int
}

func (r *DoctorReport) add(result CheckResult) {
	r.Results = append(r.Results, result)
	switch result.Status {
	case "ok":
		r.pass++
	case "warn":
		r.warn++
	case "fail":
		r.fail++
	}
}

// RunDoctor performs all diagnostic checks and prints a formatted report.
func RunDoctor() {
	report := &DoctorReport{}

	tui.PrintDoctorHeader()

	// Run all checks
	checkContainerEngine(report)
	checkContainerService(report)
	checkDockerPermissions(report)
	checkContainerImages(report)
	checkLimaVM(report)
	checkX11Display(report)
	checkXhost(report)
	checkAudioSystem(report)
	checkAudioServer(report)
	checkUSBDevices(report)
	checkHostUdevRules(report)
	checkNixEngine(report)
	checkConfigFile(report)
	checkKernelModules(report)

	// Print results
	printReport(report)
}

// checkNixEngine reports whether the Nix engine can run. On Linux and macOS
// that is a nix binary on PATH; on Windows the engine lives inside a WSL 2
// distribution provisioned with nix and the Linux rfswift (rfswift nix wsl).
func checkNixEngine(report *DoctorReport) {
	if runtime.GOOS != "windows" {
		if path, err := exec.LookPath("nix"); err == nil {
			report.add(CheckResult{"Nix engine", "ok", fmt.Sprintf("nix found at %s ('rfswift run --engine nix' runs tools natively)", path)})
		} else {
			report.add(CheckResult{"Nix engine", "skip", "nix not installed (optional: native environments with --engine nix, see docs/nix-engine.md)"})
		}
		return
	}
	st, err := rfutils.ResolveWSLNix(common.ConfigFileByPlatform())
	if err != nil {
		report.add(CheckResult{"Nix engine (WSL 2)", "warn", fmt.Sprintf("%v", err)})
		return
	}
	if !st.Ready() {
		report.add(CheckResult{"Nix engine (WSL 2)", "warn", fmt.Sprintf("distribution %s lacks %s - set it up with 'rfswift nix wsl setup'", st.Distro, strings.Join(st.Missing(), " and "))})
		return
	}
	extras := []string{}
	if st.Systemd {
		extras = append(extras, "systemd")
	} else {
		extras = append(extras, "no systemd (udev rules not applied automatically)")
	}
	if st.X11 && st.Audio {
		extras = append(extras, "WSLg display+audio")
	}
	if st.GPULibs {
		extras = append(extras, "WSLg GPU libs")
	}
	report.add(CheckResult{"Nix engine (WSL 2)", "ok", fmt.Sprintf("%s: %s, rfswift %s (%s)", st.Distro, st.NixVersion, st.RFSwiftVersion, strings.Join(extras, ", "))})
	// WSLg's display client: when it has stopped painting after an RDP
	// graphics error, GUI tools show only a taskbar icon.
	if display, derr := rfutils.WSLgDisplayStatus(); derr == nil {
		switch {
		case display.Degraded:
			report.add(CheckResult{"WSLg display client", "warn", fmt.Sprintf("stopped painting windows at %s (%s): GUI tools show only a taskbar icon - run 'rfswift nix wsl display-reset'", display.LastGfxError.Local().Format("15:04:05"), display.LastGfxErrorText)})
		case display.ClientRunning:
			report.add(CheckResult{"WSLg display client", "ok", "msrdc.exe connected, GUI windows are painted ('rfswift nix wsl display-reset' restarts it if one ever shows only a taskbar icon)"})
		default:
			report.add(CheckResult{"WSLg display client", "skip", "not running yet (WSL starts it with the first GUI window)"})
		}
	}
	if st.RFSwiftVersion != "unknown" && st.RFSwiftVersion != common.Version {
		report.add(CheckResult{"Nix engine (WSL 2)", "warn", fmt.Sprintf("the Linux rfswift in %s is %s while this one is %s; align them with 'rfswift nix wsl setup --update'", st.Distro, st.RFSwiftVersion, common.Version)})
	}
}

func printReport(report *DoctorReport) {
	for _, r := range report.Results {
		tui.PrintDoctorResult(tui.DoctorResult{
			Name:    r.Name,
			Status:  r.Status,
			Message: r.Message,
		})
	}

	tui.PrintDoctorSummary(tui.DoctorSummary{
		Pass: report.pass,
		Warn: report.warn,
		Fail: report.fail,
	})
}

// ---------------------------------------------------------------------------
// Individual checks
// ---------------------------------------------------------------------------

func checkContainerEngine(report *DoctorReport) {
	engine := GetEngine()
	if engine == nil {
		report.add(CheckResult{"Container engine", "fail", "No container engine found (install Docker or Podman)"})
		return
	}

	if !engine.IsAvailable() {
		report.add(CheckResult{"Container engine", "fail",
			fmt.Sprintf("%s binary not found in PATH", engine.Name())})
		return
	}

	report.add(CheckResult{"Container engine", "ok",
		fmt.Sprintf("%s (%s)", engine.Name(), engine.Type())})

	// Check for alternative engine
	var other ContainerEngine
	if engine.Type() == EngineDocker {
		other = &PodmanEngine{}
	} else {
		other = &DockerEngine{}
	}
	if other.IsAvailable() {
		report.add(CheckResult{"Alternative engine", "ok",
			fmt.Sprintf("%s also available", other.Name())})
	}
}

func checkContainerService(report *DoctorReport) {
	engine := GetEngine()
	if engine == nil || !engine.IsAvailable() {
		report.add(CheckResult{"Engine service", "skip", "No engine available"})
		return
	}

	if !engine.IsServiceRunning() {
		report.add(CheckResult{"Engine service", "fail",
			fmt.Sprintf("%s is not running (try: sudo systemctl start %s)", engine.Name(), engine.Type())})
		return
	}

	report.add(CheckResult{"Engine service", "ok", "Running and reachable"})

	// Check server version
	cli, err := engine.GetClient()
	if err != nil {
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ver, err := cli.ServerVersion(ctx, client.ServerVersionOptions{})
	if err == nil {
		report.add(CheckResult{"Engine version", "ok",
			fmt.Sprintf("%s (API %s)", ver.Version, ver.APIVersion)})
	}
}

func checkDockerPermissions(report *DoctorReport) {
	if runtime.GOOS != "linux" {
		report.add(CheckResult{"Docker permissions", "skip", "Not applicable on " + runtime.GOOS})
		return
	}

	currentUser, err := user.Current()
	if err != nil {
		report.add(CheckResult{"Docker permissions", "warn", "Could not determine current user"})
		return
	}

	// If running as root, no permission issues
	if currentUser.Uid == "0" {
		report.add(CheckResult{"Docker permissions", "ok", "Running as root"})
		return
	}

	// Check if user is in docker group
	groups, err := currentUser.GroupIds()
	if err != nil {
		report.add(CheckResult{"Docker permissions", "warn", "Could not read user groups"})
		return
	}

	dockerGroup, err := user.LookupGroup("docker")
	if err != nil {
		// docker group doesn't exist — check if using podman (rootless)
		engine := GetEngine()
		if engine != nil && engine.Type() == EnginePodman {
			report.add(CheckResult{"Docker permissions", "ok", "Using rootless Podman"})
		} else {
			report.add(CheckResult{"Docker permissions", "warn", "docker group not found (may need: sudo groupadd docker)"})
		}
		return
	}

	access := hostsetup.GetDockerAccess()
	for _, gid := range groups {
		if gid == dockerGroup.Gid {
			if access.SocketFound && !access.Accessible {
				// Joined after this session started: the kernel still has the
				// old group list. rfswift host docker-access adds a socket ACL.
				report.add(CheckResult{"Docker permissions", "warn",
					fmt.Sprintf("User '%s' is in the docker group but this session predates it: 'rfswift host docker-access' makes it work now (or 'newgrp docker' / log in again)", currentUser.Username)})
				return
			}
			report.add(CheckResult{"Docker permissions", "ok",
				fmt.Sprintf("User '%s' is in docker group", currentUser.Username)})
			return
		}
	}

	if access.Accessible {
		report.add(CheckResult{"Docker permissions", "warn",
			fmt.Sprintf("User '%s' can use the socket in this session only (ACL); 'rfswift host docker-access' makes it permanent", currentUser.Username)})
		return
	}
	report.add(CheckResult{"Docker permissions", "warn",
		fmt.Sprintf("User '%s' not in docker group: 'rfswift host docker-access' fixes it without logging out (or sudo usermod -aG docker %s)", currentUser.Username, currentUser.Username)})
}

// checkHostUdevRules reports whether RF Swift's udev rules (SDR/RF hardware
// without root for rootless Podman and Nix environments) are installed on a
// Linux host. Docker needs none, so a missing file is only a warning.
func checkHostUdevRules(report *DoctorReport) {
	if runtime.GOOS != "linux" {
		return
	}
	st := hostsetup.GetUdevStatus()
	switch {
	case st.Ready:
		report.add(CheckResult{"udev rules", "ok", st.Path + " installed, group " + strings.Join(st.Groups, ", ") + " in place"})
	case st.State == hostsetup.UdevInstalled:
		report.add(CheckResult{"udev rules", "warn", st.Detail + " - fix with 'rfswift host udev'"})
	case st.State == hostsetup.UdevForeign:
		report.add(CheckResult{"udev rules", "warn", st.Detail})
	default:
		report.add(CheckResult{"udev rules", "warn", st.Detail + " - install with 'rfswift host udev' (or 'rfswift host setup')"})
	}
}

func checkContainerImages(report *DoctorReport) {
	engine := GetEngine()
	if engine == nil || !engine.IsAvailable() || !engine.IsServiceRunning() {
		report.add(CheckResult{"RF Swift images", "skip", "Engine not available"})
		return
	}

	cli, err := engine.GetClient()
	if err != nil {
		report.add(CheckResult{"RF Swift images", "skip", "Could not connect to engine"})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	imagesRes, err := cli.ImageList(ctx, client.ImageListOptions{All: true})
	if err != nil {
		report.add(CheckResult{"RF Swift images", "warn", "Could not list images"})
		return
	}

	rfswiftCount := 0
	for _, img := range imagesRes.Items {
		for _, tag := range img.RepoTags {
			if strings.Contains(tag, "rfswift") || strings.Contains(tag, "myrfswift") {
				rfswiftCount++
			}
		}
	}

	if rfswiftCount == 0 {
		report.add(CheckResult{"RF Swift images", "warn",
			"No RF Swift images found (run: rfswift images pull)"})
	} else {
		report.add(CheckResult{"RF Swift images", "ok",
			fmt.Sprintf("%d RF Swift image(s) available", rfswiftCount)})
	}
}

// wslgProbe caches the WSLg query for the checks that need it (one wsl.exe
// round-trip per doctor run).
var wslgProbe struct {
	done   bool
	status rfutils.WSLgStatus
	err    error
}

func wslgStatus() (rfutils.WSLgStatus, error) {
	if !wslgProbe.done {
		wslgProbe.status, wslgProbe.err = rfutils.CheckWSLg()
		wslgProbe.done = true
	}
	return wslgProbe.status, wslgProbe.err
}

func checkX11Display(report *DoctorReport) {
	if runtime.GOOS == "windows" {
		// The sockets live inside the WSL 2 VM, so ask WSL rather than the
		// Windows filesystem.
		status, err := wslgStatus()
		switch {
		case err != nil:
			report.add(CheckResult{"X11 display", "warn", fmt.Sprintf("WSLg not reachable: %v", err)})
		case status.X11:
			report.add(CheckResult{"X11 display", "ok", "WSLg X11 socket (/mnt/wslg/.X11-unix, DISPLAY=:0) mounted into containers"})
		default:
			report.add(CheckResult{"X11 display", "warn", "WSLg X11 socket not found in WSL (wsl --update, then wsl --shutdown; or use --desktop)"})
		}
		return
	}

	display := os.Getenv("DISPLAY")
	if display == "" {
		report.add(CheckResult{"X11 display", "warn", "DISPLAY not set (use --desktop for headless GUI or export DISPLAY)"})
		return
	}

	// Check X11 socket
	if _, err := os.Stat("/tmp/.X11-unix"); err != nil {
		report.add(CheckResult{"X11 display", "warn",
			fmt.Sprintf("DISPLAY=%s but /tmp/.X11-unix not found", display)})
		return
	}

	report.add(CheckResult{"X11 display", "ok",
		fmt.Sprintf("DISPLAY=%s, X11 socket present", display)})
}

func checkXhost(report *DoctorReport) {
	if runtime.GOOS == "windows" {
		report.add(CheckResult{"xhost", "skip", "Not applicable on Windows/WSL"})
		return
	}

	if _, err := exec.LookPath("xhost"); err != nil {
		report.add(CheckResult{"xhost", "warn", "xhost not installed (needed for X11 forwarding)"})
		return
	}

	report.add(CheckResult{"xhost", "ok", "Installed"})
}

func checkAudioSystem(report *DoctorReport) {
	if runtime.GOOS == "windows" {
		status, err := wslgStatus()
		switch {
		case err != nil:
			report.add(CheckResult{"Audio system", "warn", fmt.Sprintf("WSLg not reachable: %v", err)})
		case status.Audio:
			report.add(CheckResult{"Audio system", "ok", "WSLg PulseAudio (containers use PULSE_SERVER=" + WSLgPulseServer + ")"})
		default:
			report.add(CheckResult{"Audio system", "warn", "WSLg PulseAudio socket not found in WSL (wsl --update, then wsl --shutdown)"})
		}
		return
	}
	status := rfutils.GetAudioSystemStatus()

	if strings.Contains(status, "No audio") {
		report.add(CheckResult{"Audio system", "warn", "No audio system detected (PulseAudio or PipeWire)"})
	} else if strings.Contains(status, "PipeWire") {
		report.add(CheckResult{"Audio system", "ok", "PipeWire"})
	} else if strings.Contains(status, "PulseAudio") {
		report.add(CheckResult{"Audio system", "ok", "PulseAudio"})
	} else {
		report.add(CheckResult{"Audio system", "ok", status})
	}
}

func checkAudioServer(report *DoctorReport) {
	if UsesWSLgAudio(containerCfg.pulseServer) {
		report.add(CheckResult{"Audio TCP server", "skip", "Not needed on Windows: audio uses the WSLg socket instead of a TCP module"})
		return
	}
	parts := strings.Split(containerCfg.pulseServer, ":")
	if len(parts) != 3 {
		report.add(CheckResult{"Audio TCP server", "warn",
			fmt.Sprintf("Invalid pulse server config: %s", containerCfg.pulseServer)})
		return
	}

	address := parts[1]
	port := parts[2]
	endpoint := net.JoinHostPort(address, port)

	conn, err := net.DialTimeout("tcp", endpoint, 3*time.Second)
	if err != nil {
		report.add(CheckResult{"Audio TCP server", "warn",
			fmt.Sprintf("Not reachable at %s (run: rfswift host audio enable)", endpoint)})
		return
	}
	conn.Close()

	report.add(CheckResult{"Audio TCP server", "ok",
		fmt.Sprintf("Listening on %s", endpoint)})
}

func checkLimaVM(report *DoctorReport) {
	if runtime.GOOS != "darwin" {
		return // Lima checks only relevant on macOS
	}

	if !rfutils.IsLimaInstalled() {
		report.add(CheckResult{"Lima VM", "warn",
			"Lima not installed (optional — enables USB passthrough: brew install lima qemu)"})
		return
	}
	report.add(CheckResult{"Lima VM", "ok", "Lima installed"})

	if !rfutils.IsQEMUInstalled() {
		report.add(CheckResult{"QEMU", "fail",
			"QEMU not installed — required by Lima for USB passthrough (brew install qemu)"})
		return
	}
	report.add(CheckResult{"QEMU", "ok", "QEMU installed"})

	instance := os.Getenv("RFSWIFT_LIMA_INSTANCE")
	if instance == "" {
		instance = "rfswift"
	}

	if !rfutils.IsLimaInstanceRunning(instance) {
		if limaInstanceExists(instance) {
			report.add(CheckResult{"Lima instance", "warn",
				fmt.Sprintf("Instance '%s' exists but is not running (rfswift will auto-start it)", instance)})
		} else {
			report.add(CheckResult{"Lima instance", "warn",
				fmt.Sprintf("Instance '%s' not found (rfswift will auto-create it on first run)", instance)})
		}
		return
	}
	report.add(CheckResult{"Lima instance", "ok",
		fmt.Sprintf("Instance '%s' is running", instance)})

	// Check QMP socket for USB passthrough
	sockPath, err := rfutils.FindLimaQMPSocket(instance)
	if err != nil {
		report.add(CheckResult{"Lima USB passthrough", "warn",
			"QMP socket not found — USB passthrough requires vmType: qemu"})
	} else {
		report.add(CheckResult{"Lima USB passthrough", "ok",
			fmt.Sprintf("QMP socket: %s", sockPath)})
	}
}

func checkUSBDevices(report *DoctorReport) {
	if runtime.GOOS == "windows" {
		// Containers live in the WSL 2 VM; usbipd-win forwards host devices into
		// it. Sharing needs administrator rights once per device, attach/detach
		// never do - so the doctor only checks the tooling and reports counts.
		if !rfutils.IsUsbipdInstalled() {
			report.add(CheckResult{"USB devices", "warn", "usbipd-win not installed (winget install usbipd) - needed to forward USB devices into WSL 2 containers"})
			return
		}
		version, _ := rfutils.UsbipdVersion()
		devices, err := rfutils.ListUSBDevices()
		if err != nil {
			report.add(CheckResult{"USB devices", "warn", fmt.Sprintf("usbipd %s installed but not usable: %v", version, err)})
			return
		}
		var connected, shared, attached int
		for _, d := range devices {
			if d.Connected {
				connected++
			}
			if d.Shared {
				shared++
			}
			if d.Attached {
				attached++
			}
		}
		report.add(CheckResult{"USB devices", "ok",
			fmt.Sprintf("usbipd %s: %d connected, %d shared, %d attached to WSL 2 (use 'rfswift usb attach')", version, connected, shared, attached)})
		if wsl, err := rfutils.WSLDistributions(); err != nil {
			report.add(CheckResult{"WSL 2", "warn", fmt.Sprintf("%v", err)})
		} else if !wsl.HasWSL2Distribution() {
			report.add(CheckResult{"WSL 2", "warn", "no WSL 2 distribution found; usbipd attaches devices to the default one (wsl --install -d Ubuntu)"})
		} else {
			name := wsl.DefaultDistro
			if name == "" {
				name = "(no default set: wsl --set-default <name>)"
			}
			report.add(CheckResult{"WSL 2", "ok", "default distribution: " + name})
		}
		return
	}

	if runtime.GOOS == "darwin" {
		// On macOS, USB devices are managed via macusb commands through Lima QMP
		devices, err := rfutils.ListMacUSBDevices()
		if err != nil {
			report.add(CheckResult{"USB devices", "warn",
				fmt.Sprintf("Could not list USB devices: %v", err)})
			return
		}
		report.add(CheckResult{"USB devices", "ok",
			fmt.Sprintf("%d USB device(s) on host (use 'rfswift macusb attach' to forward to VM)", len(devices))})
		return
	}

	if _, err := os.Stat("/dev/bus/usb"); err != nil {
		report.add(CheckResult{"USB devices", "warn", "/dev/bus/usb not found"})
		return
	}

	// Count USB devices
	entries, err := os.ReadDir("/dev/bus/usb")
	if err != nil {
		report.add(CheckResult{"USB devices", "ok", "/dev/bus/usb present"})
		return
	}

	busCount := 0
	for _, e := range entries {
		if e.IsDir() {
			busCount++
		}
	}

	report.add(CheckResult{"USB devices", "ok",
		fmt.Sprintf("/dev/bus/usb present (%d bus(es))", busCount)})
}

func checkConfigFile(report *DoctorReport) {
	configPath := common.ConfigFileByPlatform()

	info, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			report.add(CheckResult{"Config file", "warn",
				fmt.Sprintf("Not found at %s (will be created on first run)", configPath)})
		} else {
			report.add(CheckResult{"Config file", "warn",
				fmt.Sprintf("Cannot access %s: %v", configPath, err)})
		}
		return
	}

	// Check permissions on Linux/macOS
	if runtime.GOOS != "windows" {
		mode := info.Mode().Perm()
		if mode&0o077 != 0 {
			report.add(CheckResult{"Config file", "warn",
				fmt.Sprintf("%s is world/group-readable (chmod 600 recommended)", configPath)})
			return
		}
	}

	report.add(CheckResult{"Config file", "ok", configPath})
}

// subsystemCheck defines a kernel subsystem to verify, checking both
// /proc/modules (loadable modules) and sysfs paths (built-in support).
type subsystemCheck struct {
	desc       string
	moduleName string   // grep pattern in /proc/modules
	sysPaths   []string // sysfs paths that prove built-in support
}

func checkKernelModules(report *DoctorReport) {
	if runtime.GOOS != "linux" {
		report.add(CheckResult{"Kernel modules", "skip", "Not applicable on " + runtime.GOOS})
		return
	}

	modulesData, _ := os.ReadFile("/proc/modules")
	modules := string(modulesData)

	checks := []subsystemCheck{
		{"USB support", "usbcore", []string{"/sys/bus/usb", "/dev/bus/usb"}},
		{"Sound/ALSA", "snd", []string{"/sys/class/sound"}},
		{"Bluetooth", "bluetooth", []string{"/sys/class/bluetooth"}},
		{"Wi-Fi/802.11", "mac80211", []string{"/sys/class/ieee80211"}},
	}

	var found []string
	var missing []string

	for _, chk := range checks {
		if strings.Contains(modules, chk.moduleName) {
			found = append(found, chk.desc)
			continue
		}
		// Module not in /proc/modules — check if built into the kernel via sysfs
		builtIn := false
		for _, p := range chk.sysPaths {
			if _, err := os.Stat(p); err == nil {
				builtIn = true
				break
			}
		}
		if builtIn {
			found = append(found, chk.desc+" (built-in)")
		} else {
			missing = append(missing, chk.desc)
		}
	}

	if len(found) > 0 {
		report.add(CheckResult{"Kernel modules", "ok",
			fmt.Sprintf("Loaded: %s", strings.Join(found, ", "))})
	}

	if len(missing) > 0 {
		report.add(CheckResult{"Kernel modules (optional)", "warn",
			fmt.Sprintf("Not loaded: %s", strings.Join(missing, ", "))})
	}
}
