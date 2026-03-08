/* This code is part of RF Switch by @Penthertz
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

	"github.com/docker/docker/api/types/image"
	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
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

	cyan := "\033[36m"
	reset := "\033[0m"

	fmt.Printf("\n%s🩺 RF Swift Doctor%s\n", cyan, reset)
	fmt.Printf("%s══════════════════════════════════════════════════════════%s\n\n", cyan, reset)

	// Run all checks
	checkContainerEngine(report)
	checkContainerService(report)
	checkDockerPermissions(report)
	checkContainerImages(report)
	checkX11Display(report)
	checkXhost(report)
	checkAudioSystem(report)
	checkAudioServer(report)
	checkUSBDevices(report)
	checkConfigFile(report)
	checkKernelModules(report)

	// Print results
	printReport(report)
}

func statusIcon(status string) string {
	switch status {
	case "ok":
		return "\033[32m✓\033[0m" // green check
	case "warn":
		return "\033[33m!\033[0m" // yellow warning
	case "fail":
		return "\033[31m✗\033[0m" // red cross
	case "skip":
		return "\033[90m-\033[0m" // gray dash
	default:
		return "?"
	}
}

func printReport(report *DoctorReport) {
	for _, r := range report.Results {
		fmt.Printf("  %s  %-30s %s\n", statusIcon(r.Status), r.Name, r.Message)
	}

	cyan := "\033[36m"
	green := "\033[32m"
	yellow := "\033[33m"
	red := "\033[31m"
	reset := "\033[0m"

	fmt.Printf("\n%s──────────────────────────────────────────────────────────%s\n", cyan, reset)
	fmt.Printf("  %s%d passed%s", green, report.pass, reset)
	if report.warn > 0 {
		fmt.Printf("  %s%d warnings%s", yellow, report.warn, reset)
	}
	if report.fail > 0 {
		fmt.Printf("  %s%d failed%s", red, report.fail, reset)
	}
	fmt.Printf("\n\n")
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
	ver, err := cli.ServerVersion(ctx)
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

	for _, gid := range groups {
		if gid == dockerGroup.Gid {
			report.add(CheckResult{"Docker permissions", "ok",
				fmt.Sprintf("User '%s' is in docker group", currentUser.Username)})
			return
		}
	}

	report.add(CheckResult{"Docker permissions", "warn",
		fmt.Sprintf("User '%s' not in docker group (sudo usermod -aG docker %s)", currentUser.Username, currentUser.Username)})
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

	images, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		report.add(CheckResult{"RF Swift images", "warn", "Could not list images"})
		return
	}

	rfswiftCount := 0
	for _, img := range images {
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

func checkX11Display(report *DoctorReport) {
	if runtime.GOOS == "windows" {
		// Check for WSLg
		if _, err := os.Stat("/run/desktop/mnt/host/wslg/.X11-unix"); err == nil {
			report.add(CheckResult{"X11 display", "ok", "WSLg X11 socket found"})
		} else {
			report.add(CheckResult{"X11 display", "warn", "WSLg X11 socket not found (install WSLg or use --desktop)"})
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

func checkUSBDevices(report *DoctorReport) {
	if runtime.GOOS == "windows" {
		// Check usbipd availability
		if _, err := exec.LookPath("usbipd.exe"); err != nil {
			report.add(CheckResult{"USB devices", "warn", "usbipd not installed (needed for USB passthrough on Windows)"})
		} else {
			report.add(CheckResult{"USB devices", "ok", "usbipd available"})
		}
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
