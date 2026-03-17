/* This code is part of RF Switch by @Penthertz
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

	"github.com/docker/docker/client"
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

// IsAvailable checks if Lima is installed and the rfswift instance exists or
// can be created.
func (e *LimaEngine) IsAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	return rfutils.IsLimaInstalled()
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

// GetClient returns a Docker SDK client connected to the Docker socket inside Lima.
func (e *LimaEngine) GetClient() (*client.Client, error) {
	socketPath := e.GetSocketPath()
	if socketPath == "" {
		return nil, fmt.Errorf("Lima Docker socket not found — is the '%s' instance running?", e.getInstance())
	}

	return client.NewClientWithOpts(
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

	// Step 1: Check if Lima instance exists
	if !limaInstanceExists(instance) {
		common.PrintInfoMessage(fmt.Sprintf("Lima instance '%s' not found. Creating it...", instance))
		if err := e.createInstance(); err != nil {
			return fmt.Errorf("failed to create Lima instance: %w", err)
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

// RestartService stops and restarts the Lima VM.
func (e *LimaEngine) RestartService() error {
	instance := e.getInstance()

	common.PrintInfoMessage(fmt.Sprintf("Restarting Lima instance '%s'...", instance))
	_ = exec.Command("limactl", "stop", instance).Run()
	time.Sleep(2 * time.Second)

	if err := rfutils.StartLimaInstance(instance); err != nil {
		return fmt.Errorf("failed to restart Lima instance: %w", err)
	}

	return e.waitForDocker()
}

// GetHostConfigPath returns the Docker container config path inside Lima.
// Direct config editing is not supported through Lima.
func (e *LimaEngine) GetHostConfigPath(containerID string) (string, error) {
	return "", fmt.Errorf("direct config editing not supported through Lima — use container recreation")
}

// GetConfigV2Path returns the Docker config.v2.json path inside Lima.
func (e *LimaEngine) GetConfigV2Path(containerID string) (string, error) {
	return "", fmt.Errorf("direct config editing not supported through Lima — use container recreation")
}

// SupportsDirectConfigEdit returns false — files are inside the VM.
func (e *LimaEngine) SupportsDirectConfigEdit() bool {
	return false
}

// GetStorageRoot returns the Docker storage root (inside the VM).
func (e *LimaEngine) GetStorageRoot() string {
	return "/var/lib/docker (inside Lima VM)"
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// createInstance creates the Lima VM from the embedded template or a bundled YAML.
func (e *LimaEngine) createInstance() error {
	instance := e.getInstance()

	// Look for the Lima template in common locations
	templatePath := findLimaTemplate()
	if templatePath == "" {
		// Use a minimal inline template via stdin
		return createLimaInstanceInline(instance)
	}

	return rfutils.CreateLimaInstance(templatePath, instance)
}

// findLimaTemplate looks for rfswift.yaml in known locations.
func findLimaTemplate() string {
	// Check relative to the binary
	execPath, _ := os.Executable()
	if execPath != "" {
		candidates := []string{
			filepath.Join(filepath.Dir(execPath), "lima", "rfswift.yaml"),
			filepath.Join(filepath.Dir(execPath), "..", "lima", "rfswift.yaml"),
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	// Check in home directory
	home := os.Getenv("HOME")
	candidates := []string{
		filepath.Join(home, ".config", "rfswift", "lima.yaml"),
		filepath.Join(home, ".rfswift", "lima.yaml"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// createLimaInstanceInline creates a Lima instance with an inline YAML config
// passed via a temporary file.
func createLimaInstanceInline(instance string) error {
	template := `# RF Swift Lima VM - auto-generated
vmType: qemu
cpus: 4
memory: "8GiB"
disk: "100GiB"

images:
  - location: "https://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img"
    arch: "x86_64"
  - location: "https://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-arm64.img"
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
        usermod -aG docker "${LIMA_CIDATA_USER}"
      fi
      apt-get update -qq
      apt-get install -y -qq usbutils libusb-1.0-0-dev libhidapi-libusb0 libhidapi-hidraw0 libftdi1-dev udev
      # Load USB serial modules
      for mod in cdc_acm cp210x ftdi_sio ch341 pl2303; do modprobe "$mod" 2>/dev/null || true; done
      # Udev rules for common SDR devices (HackRF, RTL-SDR, USRP, BladeRF, Airspy, PlutoSDR)
      echo 'SUBSYSTEMS=="usb", ATTRS{idVendor}=="1d50", MODE="0666"' > /etc/udev/rules.d/99-rf.rules
      echo 'SUBSYSTEMS=="usb", ATTRS{idVendor}=="0bda", MODE="0666"' >> /etc/udev/rules.d/99-rf.rules
      echo 'SUBSYSTEMS=="usb", ATTRS{idVendor}=="2500", MODE="0666"' >> /etc/udev/rules.d/99-rf.rules
      echo 'SUBSYSTEMS=="usb", ATTRS{idVendor}=="2cf0", MODE="0666"' >> /etc/udev/rules.d/99-rf.rules
      echo 'SUBSYSTEMS=="usb", ATTRS{idVendor}=="03eb", MODE="0666"' >> /etc/udev/rules.d/99-rf.rules
      echo 'SUBSYSTEMS=="usb", ATTRS{idVendor}=="0456", MODE="0666"' >> /etc/udev/rules.d/99-rf.rules
      echo 'SUBSYSTEMS=="usb", ATTRS{idVendor}=="0403", MODE="0666"' >> /etc/udev/rules.d/99-rf.rules
      echo 'SUBSYSTEMS=="usb", ATTRS{idVendor}=="fffe", MODE="0666"' >> /etc/udev/rules.d/99-rf.rules
      echo 'SUBSYSTEMS=="usb", ATTRS{idVendor}=="3923", MODE="0666"' >> /etc/udev/rules.d/99-rf.rules
      udevadm control --reload-rules && udevadm trigger
      [ -d /dev/bus/usb ] && chmod -R a+rw /dev/bus/usb || true

portForwards:
  - guestSocket: "/var/run/docker.sock"
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
	out, err := exec.Command("limactl", "list", "--json").Output()
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

// IsLimaEngineCandidate returns true if Lima should be considered as an engine
// on the current platform. Used by detectEngine().
func IsLimaEngineCandidate() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	return rfutils.IsLimaInstalled()
}
