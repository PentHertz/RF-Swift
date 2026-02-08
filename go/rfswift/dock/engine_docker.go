/* This code is part of RF Switch by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package dock

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/docker/docker/client"
)

// DockerEngine implements ContainerEngine for Docker Desktop / Docker CE
type DockerEngine struct{}

func (e *DockerEngine) Name() string {
	return "Docker"
}

func (e *DockerEngine) Type() EngineType {
	return EngineDocker
}

// IsAvailable returns true when both the docker binary and a reachable
// daemon socket are present.
func (e *DockerEngine) IsAvailable() bool {
	if !binaryExists("docker") {
		return false
	}

	// Socket file check (Linux/macOS)
	socketPath := e.GetSocketPath()
	if socketPath != "" && socketFileExists(socketPath) {
		return true
	}

	// DOCKER_HOST explicitly set → trust the user
	if os.Getenv("DOCKER_HOST") != "" {
		return true
	}

	// Docker Desktop on macOS / Windows may not expose a visible socket file
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		cli, err := e.GetClient()
		if err != nil {
			return false
		}
		defer cli.Close()
		return pingClient(cli)
	}

	return false
}

// IsServiceRunning pings the Docker daemon API.
func (e *DockerEngine) IsServiceRunning() bool {
	cli, err := e.GetClient()
	if err != nil {
		return false
	}
	defer cli.Close()
	return pingClient(cli)
}

// GetClient returns a standard Docker SDK client.
func (e *DockerEngine) GetClient() (*client.Client, error) {
	return client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
}

// GetSocketPath returns the Docker daemon socket path for the current platform.
func (e *DockerEngine) GetSocketPath() string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		return host
	}

	switch runtime.GOOS {
	case "linux", "darwin":
		// Check common locations
		candidates := []string{
			"/var/run/docker.sock",
			fmt.Sprintf("%s/.docker/run/docker.sock", os.Getenv("HOME")),
			// Colima on macOS
			fmt.Sprintf("%s/.colima/default/docker.sock", os.Getenv("HOME")),
		}
		for _, sock := range candidates {
			if socketFileExists("unix://" + sock) {
				return "unix://" + sock
			}
		}
		return "unix:///var/run/docker.sock"

	case "windows":
		return "npipe:////./pipe/docker_engine"

	default:
		return "unix:///var/run/docker.sock"
	}
}

// StartService starts the Docker daemon.
func (e *DockerEngine) StartService() error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("sudo", "systemctl", "start", "docker").Run()
	case "darwin":
		return exec.Command("open", "-a", "Docker").Run()
	case "windows":
		return exec.Command("powershell", "Start-Service", "Docker").Run()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// RestartService restarts the Docker daemon.
func (e *DockerEngine) RestartService() error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("sudo", "systemctl", "restart", "docker").Run()
	case "darwin":
		return exec.Command("osascript", "-e",
			`do shell script "brew services restart docker" with administrator privileges`).Run()
	case "windows":
		return exec.Command("powershell", "Restart-Service", "Docker").Run()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// GetHostConfigPath returns the Docker-internal hostconfig.json path.
func (e *DockerEngine) GetHostConfigPath(containerID string) (string, error) {
	var configPath string

	switch runtime.GOOS {
	case "linux", "darwin":
		configPath = fmt.Sprintf("/var/lib/docker/containers/%s/hostconfig.json", containerID)
	case "windows":
		configPath = fmt.Sprintf("C:\\ProgramData\\docker\\containers\\%s\\hostconfig.json", containerID)
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", configPath)
	} else if err != nil {
		return "", fmt.Errorf("error checking file: %v", err)
	}

	return configPath, nil
}

// GetConfigV2Path returns the Docker config.v2.json path.
func (e *DockerEngine) GetConfigV2Path(containerID string) (string, error) {
	hostPath, err := e.GetHostConfigPath(containerID)
	if err != nil {
		return "", err
	}
	return strings.Replace(hostPath, "hostconfig.json", "config.v2.json", 1), nil
}

// SupportsDirectConfigEdit returns true — Docker allows editing hostconfig.json
// and config.v2.json on disk (requires service restart to take effect).
func (e *DockerEngine) SupportsDirectConfigEdit() bool {
	return true
}

// GetStorageRoot returns the Docker storage root directory.
func (e *DockerEngine) GetStorageRoot() string {
	switch runtime.GOOS {
	case "linux", "darwin":
		return "/var/lib/docker"
	case "windows":
		return "C:\\ProgramData\\docker"
	default:
		return "/var/lib/docker"
	}
}