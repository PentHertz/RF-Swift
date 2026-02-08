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

	"github.com/docker/docker/client"
	common "penthertz/rfswift/common"
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
// active engine. Recommended replacement for direct
// client.NewClientWithOpts(client.FromEnv, ...) calls.
func NewEngineClient() (*client.Client, error) {
	engine := GetEngine()
	if engine == nil {
		return nil, fmt.Errorf("no container engine available: install Docker or Podman")
	}
	return engine.GetClient()
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

// EngineRestartService replaces the old RestartDockerService()
func EngineRestartService() error {
	return GetEngine().RestartService()
}

// EngineGetHostConfigPath replaces the old GetHostConfigPath()
func EngineGetHostConfigPath(containerID string) (string, error) {
	return GetEngine().GetHostConfigPath(containerID)
}

// EngineSupportsDirectConfigEdit checks whether direct config file editing
// is possible (Docker: yes, Podman: no — requires container recreation).
func EngineSupportsDirectConfigEdit() bool {
	return GetEngine().SupportsDirectConfigEdit()
}

// ---------------------------------------------------------------------------
// Engine info display
// ---------------------------------------------------------------------------

// PrintEngineInfo displays the active engine status for the "engine" CLI command
func PrintEngineInfo() {
	engine := GetEngine()

	cyan := "\033[36m"
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	reset := "\033[0m"

	fmt.Printf("\n%s🔧 Container Engine%s\n", cyan, reset)
	fmt.Printf("%s──────────────────────────────────────%s\n", cyan, reset)
	fmt.Printf("  Engine:         %s%s%s\n", yellow, engine.Name(), reset)
	fmt.Printf("  Type:           %s\n", engine.Type())
	fmt.Printf("  Socket:         %s\n", engine.GetSocketPath())

	if engine.IsAvailable() {
		fmt.Printf("  Status:         %s● available%s\n", green, reset)
	} else {
		fmt.Printf("  Status:         %s● not available%s\n", red, reset)
	}

	if engine.IsServiceRunning() {
		fmt.Printf("  Service:        %s● running%s\n", green, reset)
	} else {
		fmt.Printf("  Service:        %s● stopped%s\n", red, reset)
	}

	fmt.Printf("  Direct config:  %v\n", engine.SupportsDirectConfigEdit())
	fmt.Printf("  Storage root:   %s\n", engine.GetStorageRoot())
	fmt.Println()

	// Show alternative engine status
	var other ContainerEngine
	if engine.Type() == EngineDocker {
		other = &PodmanEngine{}
	} else {
		other = &DockerEngine{}
	}
	if other.IsAvailable() {
		fmt.Printf("  %sAlternative:%s    %s (available, use --engine %s)\n",
			cyan, reset, other.Name(), other.Type())
	}
	fmt.Println()
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