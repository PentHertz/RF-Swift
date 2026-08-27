package workbench

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	rfdock "penthertz/rfswift/dock"
	rfutils "penthertz/rfswift/rfutils"
)

// HostOS reports the platform the Workbench runs on (darwin/linux/windows), so
// the GUI can hide capabilities the OS cannot provide.
func (a *App) HostOS() string { return runtime.GOOS }

// NixSupported reports whether the Nix engine can run on this host. Nix does not
// run natively on Windows, so the GUI hides Nix environments and the Nix engine
// option there (including in mission creation).
func (a *App) NixSupported() bool { return runtime.GOOS != "windows" }

// EngineStatus describes one container engine for the GUI's engine doctor.
type EngineStatus struct {
	Name      string `json:"name"`  // docker|podman|lima
	Label     string `json:"label"` // display name, e.g. "Docker (Lima VM: rfswift)"
	Available bool   `json:"available"`
	Running   bool   `json:"running"`
	State     string `json:"state"`              // running|starting|stopped|not installed
	Active    bool   `json:"active"`             // rfdock's current default engine
	Socket    string `json:"socket"`             //
	Instance  string `json:"instance,omitempty"` // Lima VM instance name
	VM        bool   `json:"vm"`                 // lifecycle is GUI-controllable (Lima)
}

// ContainerEngines reports every container engine relevant on this host with
// availability and service state, so the GUI can show where missions can run
// and expose Lima VM lifecycle controls. Engines other people's daemons could
// alias (DOCKER_HOST into the Lima socket) are still all reported here — the
// list view dedupes, the status strip should not hide them.
func (a *App) ContainerEngines() []EngineStatus {
	if _, ok := a.eng.(*LocalEngine); !ok {
		return nil
	}
	resetEngineEnv()
	activeType := rfdock.GetEngine().Type()
	candidates := []rfdock.ContainerEngine{&rfdock.DockerEngine{}, &rfdock.PodmanEngine{}}
	if rfdock.IsLimaEngineCandidate() || strings.EqualFold(string(activeType), "lima") {
		candidates = append(candidates, &rfdock.LimaEngine{})
	}
	var out []EngineStatus
	for _, eng := range candidates {
		status := EngineStatus{
			Name:      string(eng.Type()),
			Label:     eng.Name(),
			Available: eng.IsAvailable(),
			Active:    eng.Type() == activeType,
			VM:        eng.Type() == rfdock.EngineLima,
		}
		if status.Available {
			status.Running = eng.IsServiceRunning()
			status.Socket = eng.GetSocketPath()
		}
		if eng.Type() == rfdock.EngineLima {
			status.Instance = limaInstanceName()
		}
		switch {
		case !status.Available:
			status.State = "not installed"
		case status.Running:
			status.State = "running"
		case status.VM && rfutils.IsLimaInstanceRunning(status.Instance):
			// The VM is up but Docker inside is not reachable yet (booting,
			// or the guest daemon is down) — "stopped" would be misleading.
			status.State = "starting"
		default:
			status.State = "stopped"
		}
		// Hide engines that are simply not installed, except the Lima entry on
		// macOS — showing it stopped/unavailable is how users discover the VM.
		if !status.Available && !status.VM {
			continue
		}
		out = append(out, status)
	}
	return out
}

// StartContainerEngine starts an engine's service: the Lima VM (creating it on
// first use), Docker Desktop, or the Podman machine. Blocks until the engine
// is reachable, so the GUI should call it asynchronously and show progress.
func (a *App) StartContainerEngine(name string) error {
	if _, ok := a.eng.(*LocalEngine); !ok {
		return fmt.Errorf("engine control is only available for the local connection")
	}
	eng := engineByType(rfdock.EngineType(strings.ToLower(strings.TrimSpace(name))))
	if eng == nil {
		return fmt.Errorf("unknown engine %q", name)
	}
	resetEngineEnv()
	return rfdock.EnsureEngineRunning(eng)
}

// StopContainerEngine stops the Lima VM (and with it the containers inside).
// Docker and Podman daemons are left to their own lifecycle managers.
func (a *App) StopContainerEngine(name string) error {
	if _, ok := a.eng.(*LocalEngine); !ok {
		return fmt.Errorf("engine control is only available for the local connection")
	}
	if !strings.EqualFold(strings.TrimSpace(name), "lima") {
		return fmt.Errorf("stopping %s from the Workbench is not supported", name)
	}
	return rfutils.StopLimaInstance(limaInstanceName())
}

func limaInstanceName() string {
	if v := os.Getenv("RFSWIFT_LIMA_INSTANCE"); v != "" {
		return v
	}
	return "rfswift"
}
