/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*  Lima VM engine - transparent Docker-in-Lima management for macOS
 */

package dock

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/moby/moby/client"
	common "penthertz/rfswift/common"
	rfutils "penthertz/rfswift/rfutils"
)

// LimaEngine implements ContainerEngine for Docker running inside a Lima VM.
// It transparently manages the Lima instance lifecycle so the user can run
// RF Swift container commands without manually dealing with Lima.
type LimaEngine struct {
	instance string // Lima instance name (default: "rfswift")
	detected bool
	socket   string
}

const defaultLimaInstance = "rfswift"

// Name returns the engine display name.
func (e *LimaEngine) Name() string {
	return fmt.Sprintf("Docker (Lima VM: %s)", e.getInstance())
}

// Type returns the engine type identifier.
func (e *LimaEngine) Type() EngineType {
	return EngineLima
}

// getInstance returns the Lima instance name, defaulting to "rfswift".
func (e *LimaEngine) getInstance() string {
	if e.instance == "" {
		if inst := os.Getenv("RFSWIFT_LIMA_INSTANCE"); inst != "" {
			e.instance = inst
		} else {
			e.instance = defaultLimaInstance
		}
	}
	return e.instance
}

// isGPU reports whether this engine targets the GPU (krunkit) VM variant,
// selected via --gpu (which sets RFSWIFT_LIMA_INSTANCE=rfswift-gpu) or any
// instance name ending in "-gpu". The GPU variant uses the krunkit/libkrun
// backend (Vulkan via Venus/MoltenVK) and cannot do USB passthrough.
func (e *LimaEngine) isGPU() bool {
	return strings.HasSuffix(e.getInstance(), "-gpu")
}

// IsAvailable checks that Lima and the backend for this variant are installed:
// QEMU for the default USB VM, or krunkit for the GPU VM.
func (e *LimaEngine) IsAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	if !rfutils.IsLimaInstalled() {
		return false
	}
	if e.isGPU() {
		// GPU variant runs on the krunkit (libkrun) backend, not QEMU.
		if !rfutils.IsKrunkitInstalled() {
			common.PrintWarningMessage("GPU Lima VM requested but krunkit is missing (install with: brew tap slp/krunkit && brew install krunkit)")
			return false
		}
		return true
	}
	if !rfutils.IsQEMUInstalled() {
		common.PrintWarningMessage("Lima is installed but QEMU is missing (install with: brew install qemu)")
		return false
	}
	return true
}

// IsServiceRunning checks if the Lima VM is running and Docker inside it is reachable.
func (e *LimaEngine) IsServiceRunning() bool {
	if !rfutils.IsLimaInstanceRunning(e.getInstance()) {
		return false
	}

	// Check if Docker is reachable inside the VM
	cli, err := e.GetClient()
	if err != nil {
		return false
	}
	defer cli.Close()
	return pingClient(cli)
}

// GetClient returns a moby API client connected to the Docker socket inside Lima.
func (e *LimaEngine) GetClient() (*client.Client, error) {
	socketPath := e.GetSocketPath()
	if socketPath == "" {
		return nil, fmt.Errorf("Lima Docker socket not found - is the '%s' instance running?", e.getInstance())
	}

	return client.New(
		client.WithHost(socketPath),
		client.WithAPIVersionNegotiation(),
	)
}

// GetSocketPath returns the Docker socket exposed by the Lima VM.
func (e *LimaEngine) GetSocketPath() string {
	if e.socket != "" {
		return e.socket
	}

	instance := e.getInstance()
	home := os.Getenv("HOME")

	// Lima exposes the Docker socket from the guest into the host
	candidates := []string{
		// Lima's default socket forwarding path
		filepath.Join(home, ".lima", instance, "sock", "docker.sock"),
		// Alternative: some Lima configurations
		filepath.Join(home, ".lima", instance, "docker.sock"),
	}

	for _, sock := range candidates {
		if _, err := os.Stat(sock); err == nil {
			e.socket = "unix://" + sock
			return e.socket
		}
	}

	// Try querying lima to find the socket
	if sock := queryLimaDockerSocket(instance); sock != "" {
		e.socket = sock
		return e.socket
	}

	return ""
}

// StartService ensures the Lima VM is running and Docker is available inside it.
// This is the core of transparent VM management.
func (e *LimaEngine) StartService() error {
	instance := e.getInstance()

	// Step 0: Without limactl every step below fails confusingly — a failing
	// `limactl list` makes an existing instance look absent, so we would try
	// (and fail) to create one. GUI apps launched from Finder are the common
	// case: they inherit a minimal PATH without Homebrew or /usr/local.
	if !rfutils.IsLimaInstalled() {
		return fmt.Errorf("limactl not found (looked in PATH, /opt/homebrew/bin, /usr/local/bin) — install Lima with: brew install lima qemu")
	}

	// Step 1: Check if Lima instance exists
	if !limaInstanceExists(instance) {
		common.PrintInfoMessage(fmt.Sprintf("Lima instance '%s' not found. Creating it...", instance))
		if err := e.createInstance(); err != nil {
			return err
		}
		common.PrintSuccessMessage(fmt.Sprintf("Lima instance '%s' created and started", instance))
		// Wait for Docker to be ready inside the VM
		return e.waitForDocker()
	}

	// Step 2: Start the instance if not running
	if !rfutils.IsLimaInstanceRunning(instance) {
		common.PrintInfoMessage(fmt.Sprintf("Starting Lima instance '%s'...", instance))
		if err := rfutils.StartLimaInstance(instance); err != nil {
			return fmt.Errorf("failed to start Lima instance: %w", err)
		}
		common.PrintSuccessMessage(fmt.Sprintf("Lima instance '%s' started", instance))
		return e.waitForDocker()
	}

	return nil
}

// RestartService restarts the Docker daemon inside the Lima VM.
// This is faster and less disruptive than restarting the entire VM.
func (e *LimaEngine) RestartService() error {
	instance := e.getInstance()

	common.PrintInfoMessage(fmt.Sprintf("Restarting Docker inside Lima instance '%s'...", instance))
	if err := exec.Command(rfutils.LimaCtl(), "shell", instance, "sudo", "systemctl", "restart", "docker").Run(); err != nil {
		// Fall back to full VM restart if in-VM restart fails
		common.PrintWarningMessage("In-VM Docker restart failed, falling back to full VM restart...")
		_ = exec.Command(rfutils.LimaCtl(), "stop", instance).Run()
		time.Sleep(2 * time.Second)
		if err := rfutils.StartLimaInstance(instance); err != nil {
			return fmt.Errorf("failed to restart Lima instance: %w", err)
		}
	}

	return e.waitForDocker()
}

// GetHostConfigPath returns the Docker container config path inside the Lima VM.
func (e *LimaEngine) GetHostConfigPath(containerID string) (string, error) {
	return fmt.Sprintf("/var/lib/docker/containers/%s/hostconfig.json", containerID), nil
}

// GetConfigV2Path returns the Docker config.v2.json path inside the Lima VM.
func (e *LimaEngine) GetConfigV2Path(containerID string) (string, error) {
	return fmt.Sprintf("/var/lib/docker/containers/%s/config.v2.json", containerID), nil
}

// SupportsDirectConfigEdit returns true - files are accessed via limactl shell.
func (e *LimaEngine) SupportsDirectConfigEdit() bool {
	return true
}

// GetStorageRoot returns the Docker storage root (inside the VM).
func (e *LimaEngine) GetStorageRoot() string {
	return "/var/lib/docker (inside Lima VM)"
}

// ReadFile reads a file from inside the Lima VM via limactl shell.
//
//	in(1): string path - absolute path inside the VM
//	out: ([]byte, error)
func (e *LimaEngine) ReadFile(path string) ([]byte, error) {
	out, err := exec.Command(rfutils.LimaCtl(), "shell", e.getInstance(), "sudo", "cat", path).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read %s inside Lima VM: %w", path, err)
	}
	return out, nil
}

// WriteFile writes data to a file inside the Lima VM via limactl shell.
//
//	in(1): string path - absolute path inside the VM
//	in(2): []byte data - content to write
//	out: error
func (e *LimaEngine) WriteFile(path string, data []byte) error {
	cmd := exec.Command(rfutils.LimaCtl(), "shell", e.getInstance(), "sudo", "sh", "-c",
		fmt.Sprintf("cat > %s", path))
	cmd.Stdin = strings.NewReader(string(data))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write %s inside Lima VM: %w", path, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// createInstance creates the Lima VM from the embedded template or a bundled YAML.
func (e *LimaEngine) createInstance() error {
	instance := e.getInstance()
	gpu := e.isGPU()

	// Look for the Lima template in common locations
	templatePath := findLimaTemplate(gpu)
	if templatePath == "" {
		if gpu {
			return fmt.Errorf("GPU Lima template not found (expected rfswift-gpu.yaml next to the rfswift binary, or ~/.config/rfswift/lima-gpu.yaml)")
		}
		// Use a minimal inline template via stdin
		return createLimaInstanceInline(instance)
	}

	return rfutils.CreateLimaInstance(templatePath, instance)
}

// findLimaTemplate looks for the Lima template in known locations. User config
// directories are checked first so that an updated template (e.g., from the
// install script) takes priority over one bundled next to the binary. When gpu
// is true it looks for the krunkit GPU template instead of the default USB one.
func findLimaTemplate(gpu bool) string {
	home := os.Getenv("HOME")

	userName, bundledName := "lima.yaml", "rfswift.yaml"
	if gpu {
		userName, bundledName = "lima-gpu.yaml", "rfswift-gpu.yaml"
	}

	// 1. Check user config directories first (these are updated by install scripts)
	userCandidates := []string{
		filepath.Join(home, ".config", "rfswift", userName),
		filepath.Join(home, ".rfswift", userName),
	}
	for _, p := range userCandidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 2. Check relative to the binary (bundled with the release archive)
	execPath, _ := os.Executable()
	if execPath != "" {
		bundledCandidates := []string{
			filepath.Join(filepath.Dir(execPath), "lima", bundledName),
			filepath.Join(filepath.Dir(execPath), "..", "lima", bundledName),
		}
		for _, p := range bundledCandidates {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	return ""
}

// createLimaInstanceInline creates a Lima instance with an inline YAML config
// passed via a temporary file.
func createLimaInstanceInline(instance string) error {
	template := `# RF Swift Lima VM - auto-generated
vmType: qemu
# Non-"none" display makes upstream Lima add qemu-xhci,id=usb-bus for USB
# passthrough; "vnc" is headless with no macOS QEMU perf penalty.
video:
  display: "vnc"
cpus: 4
memory: "8GiB"
disk: "100GiB"

images:
  - location: "https://cloud-images.ubuntu.com/releases/26.04/release/ubuntu-26.04-server-cloudimg-amd64.img"
    arch: "x86_64"
  - location: "https://cloud-images.ubuntu.com/releases/26.04/release/ubuntu-26.04-server-cloudimg-arm64.img"
    arch: "aarch64"

mounts:
  - location: "~"
    writable: true
  - location: "/tmp/lima"
    writable: true

ssh:
  forwardAgent: true

provision:
  - mode: system
    script: |
      #!/bin/bash
      set -eux -o pipefail
      if ! command -v docker &> /dev/null; then
        curl -fsSL https://get.docker.com | sh
      fi
      # Ensure Lima user is in docker group (UID >= 500 for macOS-mirrored users)
      LIMA_USER=$(awk -F: '$3 >= 500 && $3 < 65534 && $6 ~ /^\/home\// { print $1; exit }' /etc/passwd)
      [ -n "$LIMA_USER" ] && usermod -aG docker "$LIMA_USER"
      # Make Docker socket accessible without VM restart for group reload
      mkdir -p /etc/systemd/system/docker.service.d
      cat > /etc/systemd/system/docker.service.d/socket-permissions.conf << 'DROPIN'
      [Service]
      ExecStartPost=/bin/chmod 666 /var/run/docker.sock
      DROPIN
      systemctl daemon-reload
      systemctl restart docker
      while ! docker info >/dev/null 2>&1; do sleep 1; done
      apt-get update -qq
      apt-get install -y -qq usbutils libusb-1.0-0-dev libhidapi-libusb0 libhidapi-hidraw0 libftdi1-dev udev bluez bluetooth
      # ── Kernel modules for RF devices ──────────────────────────
      apt-get install -y -qq linux-firmware linux-modules-extra-$(uname -r)
      # Load USB serial and Bluetooth modules
      for mod in usbcore usb_storage cdc_acm cp210x ftdi_sio ch341 pl2303 bluetooth btusb rfcomm vhci-hcd; do modprobe "$mod" 2>/dev/null || true; done
      cat > /etc/modules-load.d/rfswift.conf << 'MODULES'
      cdc_acm
      cp210x
      ftdi_sio
      ch341
      pl2303
      bluetooth
      btusb
      rfcomm
      vhci-hcd
      MODULES
      [ -e /dev/vhci ] && chmod 0666 /dev/vhci || true
      # Udev rules - permissive vendor-ID matching for broad device support
      cat > /etc/udev/rules.d/99-rfswift.rules << 'UDEV'
      # HackRF, Great Scott Gadgets, BladeRF, Airspy, LimeSDR, Ubertooth
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="1d50", MODE="0666"
      # RTL-SDR, Realtek
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="0bda", MODE="0666"
      # Ettus USRP
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="2500", MODE="0666"
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="3923", MODE="0666"
      # Nuand BladeRF
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="2cf0", MODE="0666"
      # Airspy HF+
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="03eb", MODE="0666"
      # ADALM-Pluto / PlutoSDR
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="0456", MODE="0666"
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="2fa2", MODE="0666"
      # FTDI (LimeSDR, JTAG, serial)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="0403", MODE="0666"
      # USRP1
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="fffe", MODE="0666"
      # STM32 (VNA, ST-Link)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="0483", MODE="0666"
      # FUNcube Dongle
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="04d8", MODE="0666"
      # VNA (SiLabs)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="1209", MODE="0666"
      # Cypress FX3 (BladeRF recovery)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="04b4", MODE="0666"
      # RFNM
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="15a2", MODE="0666"
      # Saleae Logic
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="0925", MODE="0666"
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="21a9", MODE="0666"
      # DSLogic / DreamSourceLab
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="2a0e", MODE="0666"
      # Segger J-Link
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="1366", MODE="0666"
      # CSR Bluetooth
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="0a12", MODE="0666"
      # Broadcom Bluetooth
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="0a5c", MODE="0666"
      # Intel Bluetooth
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="8087", MODE="0666"
      # Qualcomm/Atheros Bluetooth
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="0cf3", MODE="0666"
      # Nordic Semiconductor (nRF Sniffer)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="1915", MODE="0666"
      # TI CC2540/CC2531 (BLE sniffer)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="0451", MODE="0666"
      # NXP (HackRF bootloader)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="1fc9", MODE="0666"
      # Proxmark3
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="9ac4", MODE="0666"
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="2d2d", MODE="0666"
      # ACS NFC readers (ACR122U)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="072f", MODE="0666"
      # SCM Microsystems NFC
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="04e6", MODE="0666"
      # Maple/STM32 (ChameleonMini)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="1eaf", MODE="0666"
      # Teensy/VUSB
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="16c0", MODE="0666"
      # Silicon Labs CP210x
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="10c4", MODE="0666"
      # QinHeng CH340/CH341
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="1a86", MODE="0666"
      # Espressif ESP32
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="303a", MODE="0666"
      # Adafruit (nRF52, RP2040)
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="239a", MODE="0666"
      # Arduino
      SUBSYSTEMS=="usb", ATTRS{idVendor}=="2341", MODE="0666"
      # HID raw devices (GPSDO, calibration)
      KERNEL=="hidraw*", SUBSYSTEM=="hidraw", MODE="0660", GROUP="plugdev"
      UDEV
      udevadm control --reload-rules && udevadm trigger
      [ -d /dev/bus/usb ] && chmod -R a+rw /dev/bus/usb || true

portForwards:
  - guestSocket: "/run/docker.sock"
    hostSocket: "{{.Dir}}/sock/docker.sock"
  - guestPort: 6080
    hostPort: 6080
  - guestPort: 34567
    hostPort: 34567
`

	tmpFile, err := os.CreateTemp("", "rfswift-lima-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(template); err != nil {
		return fmt.Errorf("failed to write template: %w", err)
	}
	tmpFile.Close()

	return rfutils.CreateLimaInstance(tmpFile.Name(), instance)
}

// waitForDocker waits for Docker to become reachable inside the Lima VM.
func (e *LimaEngine) waitForDocker() error {
	common.PrintInfoMessage("Waiting for Docker inside Lima VM...")

	// Re-detect socket after VM start
	e.socket = ""

	for i := 0; i < 30; i++ {
		time.Sleep(2 * time.Second)

		// Try to get a fresh socket path
		if e.GetSocketPath() == "" {
			continue
		}

		cli, err := e.GetClient()
		if err != nil {
			continue
		}
		if pingClient(cli) {
			cli.Close()
			common.PrintSuccessMessage("Docker is ready inside Lima VM")
			return nil
		}
		cli.Close()
	}

	return fmt.Errorf("Docker did not become reachable inside Lima VM after 60 seconds")
}

// limaInstanceExists checks if a Lima instance has been created (regardless of state).
func limaInstanceExists(instance string) bool {
	out, err := exec.Command(rfutils.LimaCtl(), "list", "--json").Output()
	if err != nil {
		return false
	}

	// limactl list --json outputs one JSON object per line (JSONL)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var info map[string]interface{}
		if err := json.Unmarshal([]byte(line), &info); err != nil {
			continue
		}
		if name, ok := info["name"].(string); ok && name == instance {
			return true
		}
	}

	return false
}

// queryLimaDockerSocket tries to find the Docker socket forwarded by Lima.
func queryLimaDockerSocket(instance string) string {
	// Check if Lima forwards the Docker socket via its config
	home := os.Getenv("HOME")
	limaDir := filepath.Join(home, ".lima", instance)

	// Read the lima.yaml to check for guestSocket forwarding
	yamlPath := filepath.Join(limaDir, "lima.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return ""
	}

	// Simple check: if the config has docker.sock forwarding
	content := string(data)
	if strings.Contains(content, "docker.sock") {
		sockPath := filepath.Join(limaDir, "sock", "docker.sock")
		if _, err := os.Stat(sockPath); err == nil {
			return "unix://" + sockPath
		}
	}

	return ""
}

// ReconfigureInstance applies an updated YAML template to the Lima instance.
// When force is false, it stops the instance, overwrites its config, and restarts.
// When force is true, it deletes the instance entirely and recreates it from the template.
//
//	in(1): string templatePath path to the new YAML template
//	in(2): bool force if true, delete and recreate (destructive)
//	out: error
func (e *LimaEngine) ReconfigureInstance(templatePath string, force bool) error {
	instance := e.getInstance()

	if !limaInstanceExists(instance) {
		return fmt.Errorf("Lima instance '%s' does not exist - run a container command first to create it, or use 'reset' to create from scratch", instance)
	}

	wasRunning := rfutils.IsLimaInstanceRunning(instance)

	if force {
		// Destructive path: delete + recreate
		if wasRunning {
			common.PrintInfoMessage(fmt.Sprintf("Stopping Lima instance '%s'...", instance))
			if err := rfutils.StopLimaInstance(instance); err != nil {
				return fmt.Errorf("failed to stop instance: %w", err)
			}
			common.PrintSuccessMessage("Instance stopped.")
		}

		common.PrintInfoMessage(fmt.Sprintf("Deleting Lima instance '%s'...", instance))
		if err := rfutils.DeleteLimaInstance(instance); err != nil {
			return fmt.Errorf("failed to delete instance: %w", err)
		}
		common.PrintSuccessMessage("Instance deleted.")

		common.PrintInfoMessage(fmt.Sprintf("Creating Lima instance '%s' from %s...", instance, templatePath))
		if err := rfutils.CreateLimaInstance(templatePath, instance); err != nil {
			return fmt.Errorf("failed to create instance: %w", err)
		}
	} else {
		// Non-destructive: stop, overwrite config, start
		if wasRunning {
			common.PrintInfoMessage(fmt.Sprintf("Stopping Lima instance '%s'...", instance))
			if err := rfutils.StopLimaInstance(instance); err != nil {
				return fmt.Errorf("failed to stop instance: %w", err)
			}
			common.PrintSuccessMessage("Instance stopped.")
		}

		destPath := rfutils.GetLimaInstanceConfigPath(instance)
		common.PrintInfoMessage(fmt.Sprintf("Applying new configuration to %s...", destPath))
		data, err := os.ReadFile(templatePath)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", templatePath, err)
		}
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write config to %s: %w", destPath, err)
		}
		common.PrintSuccessMessage("Configuration updated.")

		common.PrintInfoMessage(fmt.Sprintf("Starting Lima instance '%s'...", instance))
		if err := rfutils.StartLimaInstance(instance); err != nil {
			return fmt.Errorf("failed to start instance: %w", err)
		}
	}

	// Reset cached socket so it's re-detected
	e.socket = ""

	common.PrintInfoMessage("Waiting for Docker inside Lima VM...")
	if err := e.waitForDocker(); err != nil {
		return err
	}

	common.PrintSuccessMessage(fmt.Sprintf("Lima instance '%s' reconfigured successfully.", instance))
	if !force {
		common.PrintInfoMessage("Note: disk size and base image changes require --force (destructive recreate)")
	}
	return nil
}

// ResetInstance deletes and recreates the Lima instance from a YAML template.
// If the instance does not exist, it creates it directly.
//
//	in(1): string templatePath path to the YAML template
//	out: error
func (e *LimaEngine) ResetInstance(templatePath string) error {
	instance := e.getInstance()

	if limaInstanceExists(instance) {
		if rfutils.IsLimaInstanceRunning(instance) {
			common.PrintInfoMessage(fmt.Sprintf("Stopping Lima instance '%s'...", instance))
			if err := rfutils.StopLimaInstance(instance); err != nil {
				return fmt.Errorf("failed to stop instance: %w", err)
			}
		}
		common.PrintInfoMessage(fmt.Sprintf("Deleting Lima instance '%s'...", instance))
		if err := rfutils.DeleteLimaInstance(instance); err != nil {
			return fmt.Errorf("failed to delete instance: %w", err)
		}
		common.PrintSuccessMessage("Old instance deleted.")
	}

	common.PrintInfoMessage(fmt.Sprintf("Creating Lima instance '%s' from %s...", instance, templatePath))
	if err := rfutils.CreateLimaInstance(templatePath, instance); err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	e.socket = ""
	if err := e.waitForDocker(); err != nil {
		return err
	}

	common.PrintSuccessMessage(fmt.Sprintf("Lima instance '%s' created and ready.", instance))
	return nil
}

// FindTemplate locates the Lima YAML template using the standard search paths.
// Returns the path if found, or empty string.
func (e *LimaEngine) FindTemplate() string {
	return findLimaTemplate(e.isGPU())
}

// UserTemplatePath returns the preferred user-config path for this engine's
// template variant: ~/.config/rfswift/lima.yaml, or lima-gpu.yaml for the GPU VM.
// This is where `rfswift engine lima set` writes so changes persist and take
// precedence over the bundled template.
func (e *LimaEngine) UserTemplatePath() string {
	name := "lima.yaml"
	if e.isGPU() {
		name = "lima-gpu.yaml"
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "rfswift", name)
}

// IsLimaEngineCandidate returns true if Lima should be considered as an engine
// on the current platform. Used by detectEngine().
func IsLimaEngineCandidate() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	return rfutils.IsLimaInstalled()
}
