/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package dock

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/client"
	common "penthertz/rfswift/common"
	"penthertz/rfswift/tui"
)

// EngineType represents the container engine backend
type EngineType string

const (
	EngineDocker EngineType = "docker"
	EnginePodman EngineType = "podman"
	EngineAuto   EngineType = "auto"
)

// ContainerEngine defines the interface that both Docker and Podman implement.
// Since Podman exposes a Docker-compatible API, the Docker Go SDK is used for
// both backends — the engine layer handles socket routing and platform quirks.
type ContainerEngine interface {
	// Identity
	Name() string
	Type() EngineType

	// Availability
	IsAvailable() bool
	IsServiceRunning() bool

	// Client management — returns a Docker-compatible SDK client
	GetClient() (*client.Client, error)
	GetSocketPath() string

	// Service lifecycle
	StartService() error
	RestartService() error

	// Container config paths (engine-specific storage layouts)
	GetHostConfigPath(containerID string) (string, error)
	GetConfigV2Path(containerID string) (string, error)

	// Engine capabilities
	SupportsDirectConfigEdit() bool // Docker: yes, Podman: no
	GetStorageRoot() string
}

// ---------------------------------------------------------------------------
// Global engine state
// ---------------------------------------------------------------------------

var (
	activeEngine    ContainerEngine
	activeEngineMu  sync.RWMutex
	preferredEngine EngineType = EngineAuto
)

// SetPreferredEngine sets the preferred engine type from a CLI flag or config.
// Must be called before the first GetEngine() / NewEngineClient() call
// (typically in PersistentPreRun).
//
//	in(1): string engine engine type ("docker", "podman", or "auto")
func SetPreferredEngine(engine string) {
	activeEngineMu.Lock()
	defer activeEngineMu.Unlock()

	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "docker":
		preferredEngine = EngineDocker
	case "podman":
		preferredEngine = EnginePodman
	default:
		preferredEngine = EngineAuto
	}
	// Reset cached engine so the next GetEngine() re-detects
	activeEngine = nil
}

// GetEngine returns the active container engine, performing lazy detection on
// first call. The result is cached for the process lifetime.
//
// When Podman is selected, DOCKER_HOST is set automatically so that existing
// client.FromEnv calls throughout rfdock.go use the Podman socket with zero
// code changes required.
//
//	out: ContainerEngine
func GetEngine() ContainerEngine {
	activeEngineMu.RLock()
	if activeEngine != nil {
		defer activeEngineMu.RUnlock()
		return activeEngine
	}
	activeEngineMu.RUnlock()

	activeEngineMu.Lock()
	defer activeEngineMu.Unlock()

	// Double-check after acquiring write lock
	if activeEngine != nil {
		return activeEngine
	}

	activeEngine = detectEngine()

	// Transparent Podman compatibility: set DOCKER_HOST so every existing
	// client.NewClientWithOpts(client.FromEnv, ...) call picks up the socket
	// without requiring changes in rfdock.go.
	if activeEngine.Type() == EnginePodman {
		socketPath := activeEngine.GetSocketPath()
		if socketPath != "" && os.Getenv("DOCKER_HOST") == "" {
			os.Setenv("DOCKER_HOST", socketPath)
		}
	}

	return activeEngine
}

// NewEngineClient creates a Docker-compatible SDK client routed through the
// active engine. If the engine service is not running, it attempts to start it.
// Recommended replacement for direct
// client.NewClientWithOpts(client.FromEnv, ...) calls.
//
//	out: (*client.Client, error)
func NewEngineClient() (*client.Client, error) {
	engine := GetEngine()
	if engine == nil {
		return nil, fmt.Errorf("no container engine available: install Docker or Podman")
	}

	if err := EnsureEngineRunning(engine); err != nil {
		return nil, err
	}

	return engine.GetClient()
}

// EnsureEngineRunning checks whether the engine service is reachable and
// attempts to start it if not. This handles the common case on macOS and
// Windows where Docker Desktop or a Podman machine must be running before
// any container operation.
//
//	in(1): ContainerEngine engine
//	out: error
func EnsureEngineRunning(engine ContainerEngine) error {
	if engine.IsServiceRunning() {
		return nil
	}

	common.PrintWarningMessage(fmt.Sprintf("%s service is not running. Attempting to start it...", engine.Name()))
	if err := engine.StartService(); err != nil {
		return fmt.Errorf("failed to start %s: %v", engine.Name(), err)
	}

	// Wait for the service to become reachable
	for i := 0; i < 15; i++ {
		time.Sleep(2 * time.Second)
		if engine.IsServiceRunning() {
			common.PrintSuccessMessage(fmt.Sprintf("%s is now running", engine.Name()))
			return nil
		}
	}

	return fmt.Errorf("%s was started but is not reachable after 30 seconds", engine.Name())
}

// ---------------------------------------------------------------------------
// Detection
// ---------------------------------------------------------------------------

func detectEngine() ContainerEngine {
	// Environment variable override (lower priority than CLI flag)
	if envEngine := os.Getenv("RFSWIFT_ENGINE"); envEngine != "" && preferredEngine == EngineAuto {
		switch strings.ToLower(envEngine) {
		case "docker":
			preferredEngine = EngineDocker
		case "podman":
			preferredEngine = EnginePodman
		default:
			common.PrintWarningMessage(fmt.Sprintf("Unknown RFSWIFT_ENGINE value '%s', falling back to auto", envEngine))
		}
	}

	docker := &DockerEngine{}
	podman := &PodmanEngine{}

	switch preferredEngine {
	case EngineDocker:
		if docker.IsAvailable() {
			common.PrintInfoMessage("Container engine: Docker (explicit)")
			return docker
		}
		common.PrintWarningMessage("Docker requested but not available, trying Podman...")
		if podman.IsAvailable() {
			common.PrintInfoMessage("Container engine: Podman (fallback)")
			return podman
		}
		common.PrintWarningMessage("No container engine available")
		return docker // will fail with descriptive errors on actual operations

	case EnginePodman:
		if podman.IsAvailable() {
			common.PrintInfoMessage("Container engine: Podman (explicit)")
			return podman
		}
		common.PrintWarningMessage("Podman requested but not available, trying Docker...")
		if docker.IsAvailable() {
			common.PrintInfoMessage("Container engine: Docker (fallback)")
			return docker
		}
		common.PrintWarningMessage("No container engine available")
		return podman

	default: // EngineAuto
		if docker.IsAvailable() {
			common.PrintInfoMessage("Container engine: Docker (auto-detected)")
			return docker
		}
		if podman.IsAvailable() {
			common.PrintInfoMessage("Container engine: Podman (auto-detected)")
			return podman
		}
		common.PrintWarningMessage("No container engine detected — defaulting to Docker")
		return docker
	}
}

// ---------------------------------------------------------------------------
// Backward-compatible wrappers
// These replace the old top-level functions in dockerutils.go so callers
// don't need changes.
// ---------------------------------------------------------------------------

// EngineRestartService replaces the old RestartDockerService().
//
//	out: error
func EngineRestartService() error {
	return GetEngine().RestartService()
}

// EngineGetHostConfigPath replaces the old GetHostConfigPath().
//
//	in(1): string containerID container identifier
//	out: (string, error)
func EngineGetHostConfigPath(containerID string) (string, error) {
	return GetEngine().GetHostConfigPath(containerID)
}

// EngineSupportsDirectConfigEdit checks whether direct config file editing
// is possible (Docker: yes, Podman: no — requires container recreation).
//
//	out: bool
func EngineSupportsDirectConfigEdit() bool {
	return GetEngine().SupportsDirectConfigEdit()
}

// ---------------------------------------------------------------------------
// Engine info display
// ---------------------------------------------------------------------------

// PrintEngineInfo displays the active engine status for the "engine" CLI command
func PrintEngineInfo() {
	engine := GetEngine()

	statusAvail := lipgloss.NewStyle().Foreground(tui.ColorSuccess).Render("● available")
	statusUnavail := lipgloss.NewStyle().Foreground(tui.ColorDanger).Render("● not available")
	statusRunning := lipgloss.NewStyle().Foreground(tui.ColorSuccess).Render("● running")
	statusStopped := lipgloss.NewStyle().Foreground(tui.ColorDanger).Render("● stopped")

	avail := statusUnavail
	if engine.IsAvailable() {
		avail = statusAvail
	}
	svc := statusStopped
	if engine.IsServiceRunning() {
		svc = statusRunning
	}

	items := []tui.PropertyItem{
		{Key: "Engine", Value: engine.Name(), ValueColor: tui.ColorWarning},
		{Key: "Type", Value: string(engine.Type())},
		{Key: "Socket", Value: engine.GetSocketPath()},
		{Key: "Status", Value: avail},
		{Key: "Service", Value: svc},
		{Key: "Direct config", Value: fmt.Sprintf("%v", engine.SupportsDirectConfigEdit())},
		{Key: "Storage root", Value: engine.GetStorageRoot()},
	}

	// Show alternative engine
	var other ContainerEngine
	if engine.Type() == EngineDocker {
		other = &PodmanEngine{}
	} else {
		other = &DockerEngine{}
	}
	if other.IsAvailable() {
		items = append(items, tui.PropertyItem{
			Key:        "Alternative",
			Value:      fmt.Sprintf("%s (use --engine %s)", other.Name(), other.Type()),
			ValueColor: tui.ColorCyan,
		})
	}

	tui.RenderPropertySheet("🔧 Container Engine", tui.ColorPrimary, items)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func binaryExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func socketFileExists(path string) bool {
	cleanPath := strings.TrimPrefix(path, "unix://")
	cleanPath = strings.TrimPrefix(cleanPath, "npipe://")
	info, err := os.Stat(cleanPath)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

func pingClient(cli *client.Client) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := cli.Ping(ctx)
	return err == nil
}

// engineIsServiceRunning is a shared implementation for IsServiceRunning:
// get a client, ping, return bool.
func engineIsServiceRunning(e ContainerEngine) bool {
	cli, err := e.GetClient()
	if err != nil {
		return false
	}
	defer cli.Close()
	return pingClient(cli)
}