/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
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
)

// PodmanEngine implements ContainerEngine for Podman.
// It uses Podman's Docker-compatible API so the Docker Go SDK works as-is.
type PodmanEngine struct {
	// Cached after first detection
	cachedSocket   string
	cachedRootless bool
	detected       bool
}

func (e *PodmanEngine) Name() string {
	e.ensureDetected()
	mode := "rootful"
	if e.cachedRootless {
		mode = "rootless"
	}
	return fmt.Sprintf("Podman (%s)", mode)
}

func (e *PodmanEngine) Type() EngineType {
	return EnginePodman
}

// IsAvailable checks if the podman binary exists and the API socket is reachable.
func (e *PodmanEngine) IsAvailable() bool {
	if !binaryExists("podman") {
		return false
	}

	socketPath := e.GetSocketPath()
	if socketPath == "" {
		return false
	}

	// Check socket file exists (Linux / macOS)
	if runtime.GOOS != "windows" {
		if socketFileExists(socketPath) {
			return true
		}
		// Socket might not exist yet — attempt activation
		if e.tryActivateSocket() {
			time.Sleep(500 * time.Millisecond)
			return socketFileExists(socketPath)
		}
		return false
	}

	// Windows: try a client ping
	cli, err := e.GetClient()
	if err != nil {
		return false
	}
	defer cli.Close()
	return pingClient(cli)
}

// IsServiceRunning pings the Podman API through the Docker-compatible endpoint.
func (e *PodmanEngine) IsServiceRunning() bool {
	cli, err := e.GetClient()
	if err != nil {
		return false
	}
	defer cli.Close()
	return pingClient(cli)
}

// GetClient returns a Docker SDK client pointed at the Podman socket.
func (e *PodmanEngine) GetClient() (*client.Client, error) {
	socketPath := e.GetSocketPath()
	if socketPath == "" {
		return nil, fmt.Errorf("podman socket not found — enable with: systemctl --user enable --now podman.socket")
	}

	return client.NewClientWithOpts(
		client.WithHost(socketPath),
		client.WithAPIVersionNegotiation(),
	)
}

// GetSocketPath returns the Podman API socket for the current platform.
func (e *PodmanEngine) GetSocketPath() string {
	e.ensureDetected()
	return e.cachedSocket
}

// StartService starts the Podman API socket.
func (e *PodmanEngine) StartService() error {
	e.ensureDetected()

	switch runtime.GOOS {
	case "linux":
		if e.cachedRootless {
			err := exec.Command("systemctl", "--user", "start", "podman.socket").Run()
			if err != nil {
				// Fallback: start the API service directly (foreground=false)
				common.PrintInfoMessage("Starting Podman API service directly...")
				cmd := exec.Command("podman", "system", "service", "--time=0")
				return cmd.Start() // Start in background
			}
			return nil
		}
		return exec.Command("sudo", "systemctl", "start", "podman.socket").Run()

	case "darwin", "windows":
		return exec.Command("podman", "machine", "start").Run()

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// RestartService restarts the Podman API socket.
func (e *PodmanEngine) RestartService() error {
	e.ensureDetected()

	switch runtime.GOOS {
	case "linux":
		if e.cachedRootless {
			err := exec.Command("systemctl", "--user", "restart", "podman.socket").Run()
			if err != nil {
				return exec.Command("systemctl", "--user", "restart", "podman.service").Run()
			}
			return nil
		}
		return exec.Command("sudo", "systemctl", "restart", "podman.socket").Run()

	case "darwin", "windows":
		// Podman machine: stop then start
		_ = exec.Command("podman", "machine", "stop").Run()
		time.Sleep(1 * time.Second)
		return exec.Command("podman", "machine", "start").Run()

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// GetHostConfigPath returns the Podman container config path.
// Podman stores container metadata differently from Docker:
//
//	<storage_root>/overlay-containers/<id>/userdata/config.json
//
// Note: direct editing is NOT supported. Use container recreation instead.
func (e *PodmanEngine) GetHostConfigPath(containerID string) (string, error) {
	storageRoot := e.GetStorageRoot()
	configPath := filepath.Join(storageRoot, "overlay-containers", containerID, "userdata", "config.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "", fmt.Errorf(
			"podman container config not found: %s\n"+
				"  Note: Podman does not support direct config editing.\n"+
				"  Use container recreation (--recreate) for config changes.", configPath)
	} else if err != nil {
		return "", fmt.Errorf("error checking file: %v", err)
	}

	return configPath, nil
}

// GetConfigV2Path returns the Podman equivalent of Docker's config.v2.json.
// Podman uses a different layout; the closest equivalent is the OCI spec file.
func (e *PodmanEngine) GetConfigV2Path(containerID string) (string, error) {
	storageRoot := e.GetStorageRoot()

	// Try the OCI spec file
	specPath := filepath.Join(storageRoot, "overlay-containers", containerID, "userdata", "spec")
	if _, err := os.Stat(specPath); err == nil {
		return specPath, nil
	}

	// Fallback to config.json
	return e.GetHostConfigPath(containerID)
}

// SupportsDirectConfigEdit returns false — Podman does not support editing
// container config files on disk. Configuration changes require container
// recreation, which recreateContainerWithProperties() already handles.
func (e *PodmanEngine) SupportsDirectConfigEdit() bool {
	return false
}

// GetStorageRoot returns the Podman storage root directory.
func (e *PodmanEngine) GetStorageRoot() string {
	e.ensureDetected()

	// Try to get the real path from podman info
	if root := e.queryStorageRoot(); root != "" {
		return root
	}

	// Fallback to convention
	if e.cachedRootless {
		home := os.Getenv("HOME")
		return filepath.Join(home, ".local", "share", "containers", "storage")
	}
	return "/var/lib/containers/storage"
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// ensureDetected runs socket detection once and caches the result
func (e *PodmanEngine) ensureDetected() {
	if e.detected {
		return
	}
	e.cachedSocket, e.cachedRootless = detectPodmanSocket()
	e.detected = true
}

// queryStorageRoot asks podman for the GraphRoot
func (e *PodmanEngine) queryStorageRoot() string {
	out, err := exec.Command("podman", "info", "--format", "{{.Store.GraphRoot}}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// tryActivateSocket attempts to start the Podman API socket service
func (e *PodmanEngine) tryActivateSocket() bool {
	e.ensureDetected()

	switch runtime.GOOS {
	case "linux":
		if e.cachedRootless {
			err := exec.Command("systemctl", "--user", "start", "podman.socket").Run()
			return err == nil
		}
		err := exec.Command("sudo", "systemctl", "start", "podman.socket").Run()
		return err == nil

	case "darwin", "windows":
		if !isPodmanMachineRunning() {
			common.PrintInfoMessage("Starting Podman machine...")
			err := exec.Command("podman", "machine", "start").Run()
			if err == nil {
				time.Sleep(2 * time.Second)
				// Re-detect socket after machine start
				e.cachedSocket, e.cachedRootless = detectPodmanSocket()
				return true
			}
		}
		return false

	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Socket detection
// ---------------------------------------------------------------------------

// detectPodmanSocket finds the Podman API socket.
// Returns (socket_uri, is_rootless).
func detectPodmanSocket() (string, bool) {
	// 1. CONTAINER_HOST — Podman's own env var
	if host := os.Getenv("CONTAINER_HOST"); host != "" {
		rootless := os.Getuid() != 0
		return host, rootless
	}

	// 2. DOCKER_HOST pointing at a Podman socket
	if host := os.Getenv("DOCKER_HOST"); host != "" && strings.Contains(host, "podman") {
		rootless := os.Getuid() != 0
		return host, rootless
	}

	switch runtime.GOOS {
	case "linux":
		return detectPodmanSocketLinux()
	case "darwin":
		return detectPodmanSocketDarwin()
	case "windows":
		return detectPodmanSocketWindows()
	default:
		return "", false
	}
}

func detectPodmanSocketLinux() (string, bool) {
	uid := os.Getuid()

	if uid != 0 {
		// Rootless mode
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			runtimeDir = fmt.Sprintf("/run/user/%d", uid)
		}

		candidates := []string{
			filepath.Join(runtimeDir, "podman", "podman.sock"),
			filepath.Join(runtimeDir, "podman.sock"),
		}

		for _, sock := range candidates {
			if _, err := os.Stat(sock); err == nil {
				return "unix://" + sock, true
			}
		}

		// Return default (socket activation may create it on first connection)
		return "unix://" + filepath.Join(runtimeDir, "podman", "podman.sock"), true
	}

	// Rootful mode
	candidates := []string{
		"/run/podman/podman.sock",
		"/var/run/podman/podman.sock",
	}

	for _, sock := range candidates {
		if _, err := os.Stat(sock); err == nil {
			return "unix://" + sock, false
		}
	}

	return "unix:///run/podman/podman.sock", false
}

func detectPodmanSocketDarwin() (string, bool) {
	// Podman machine socket via `podman machine inspect`
	if sock := queryPodmanMachineSocket(); sock != "" {
		return "unix://" + sock, true
	}

	home := os.Getenv("HOME")
	candidates := []string{
		// Podman 4.x+
		filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock"),
		// Podman machine default
		filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman-machine-default", "podman.sock"),
		filepath.Join(home, ".local", "share", "containers", "podman", "machine", "qemu", "podman.sock"),
		// /tmp fallback
		fmt.Sprintf("/tmp/podman-run-%d/podman/podman.sock", os.Getuid()),
	}

	for _, sock := range candidates {
		if _, err := os.Stat(sock); err == nil {
			return "unix://" + sock, true
		}
	}

	return "unix://" + filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock"), true
}

func detectPodmanSocketWindows() (string, bool) {
	// Podman machine on Windows exposes a named pipe
	return "npipe:////./pipe/podman-machine-default", true
}

// queryPodmanMachineSocket gets the socket path from `podman machine inspect`
func queryPodmanMachineSocket() string {
	out, err := exec.Command("podman", "machine", "inspect").Output()
	if err != nil {
		return ""
	}

	// Try array format first (Podman 4.x+)
	var machines []map[string]interface{}
	if err := json.Unmarshal(out, &machines); err != nil {
		// Try single-object format
		var machine map[string]interface{}
		if err := json.Unmarshal(out, &machine); err != nil {
			return ""
		}
		machines = []map[string]interface{}{machine}
	}

	for _, m := range machines {
		if connInfo, ok := m["ConnectionInfo"].(map[string]interface{}); ok {
			if podmanSock, ok := connInfo["PodmanSocket"].(map[string]interface{}); ok {
				if path, ok := podmanSock["Path"].(string); ok && path != "" {
					return path
				}
			}
		}
	}

	return ""
}

// isPodmanMachineRunning checks if any Podman machine is running (macOS/Windows)
func isPodmanMachineRunning() bool {
	out, err := exec.Command("podman", "machine", "list", "--format", "{{.Running}}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "true" {
			return true
		}
	}
	return false
}