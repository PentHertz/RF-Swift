package workbench

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	common "penthertz/rfswift/common"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
	rfutils "penthertz/rfswift/rfutils"
)

// HostOS reports the platform the Workbench runs on (darwin/linux/windows), so
// the GUI can hide capabilities the OS cannot provide.
func (a *App) HostOS() string { return runtime.GOOS }

// NixSupported reports whether the Nix engine can be offered on this host. It
// is true everywhere: natively on Linux and macOS, and on Windows inside a
// WSL 2 distribution that the engine drives for the GUI (see NixEngineStatus
// for whether that distribution is provisioned, and NixWSLSetup to do it).
func (a *App) NixSupported() bool { return true }

// NixEngineStatus describes how the Nix engine is served on this host, for the
// engine doctor.
type NixEngineStatus struct {
	Host           string   `json:"host"`  // "native" (Linux/macOS) or "wsl" (Windows: a WSL 2 distribution)
	Ready          bool     `json:"ready"` // environments can be created and entered
	NixVersion     string   `json:"nixVersion"`
	Distro         string   `json:"distro,omitempty"`         // WSL 2 distribution hosting the engine
	RFSwiftVersion string   `json:"rfswiftVersion,omitempty"` // the Linux rfswift inside it
	Missing        []string `json:"missing"`                  // what keeps it from being ready
	Detail         string   `json:"detail"`                   // one-line summary for the UI
	WorkspaceRoot  string   `json:"workspaceRoot,omitempty"`  // where default workspaces are, as a host path
	CanSetup       bool     `json:"canSetup"`                 // NixWSLSetup can provision it from here
}

// NixEngineStatus reports the Nix engine's availability: nix on PATH on Linux
// and macOS; on Windows the WSL 2 distribution and what it offers.
func (a *App) NixEngineStatus() NixEngineStatus {
	st := NixEngineStatus{Host: "native", Missing: []string{}}
	if _, ok := a.eng.(*LocalEngine); !ok {
		st.Detail = "engine status is only available for the local connection"
		return st
	}
	if runtime.GOOS != "windows" {
		if rfnix.IsAvailable() {
			st.Ready = true
			if v, err := rfnix.Version(); err == nil {
				st.NixVersion = v
			}
			st.Detail = st.NixVersion
			if st.Detail == "" {
				st.Detail = "nix available"
			}
		} else {
			st.Missing = []string{"nix"}
			st.Detail = "nix is not installed (https://nixos.org/download)"
		}
		return st
	}
	st.Host = "wsl"
	w, err := rfnix.WSLBackend()
	if err != nil {
		st.Missing = []string{"a WSL 2 Linux distribution"}
		st.Detail = err.Error()
		if errors.Is(err, rfutils.ErrNoWSLNixDistro) {
			st.Detail = "no WSL 2 Linux distribution: run 'wsl --install -d Ubuntu' in a terminal, then set up Nix from here"
		}
		return st
	}
	st.Distro, st.NixVersion, st.RFSwiftVersion = w.Distro, w.NixVersion, w.RFSwiftVersion
	st.Ready = w.Ready()
	st.Missing = w.Missing()
	if st.Missing == nil {
		st.Missing = []string{}
	}
	st.CanSetup = !st.Ready
	st.WorkspaceRoot = rfnix.WSLWorkspaceRoot()
	switch {
	case st.Ready:
		st.Detail = fmt.Sprintf("%s: %s, rfswift %s", w.Distro, w.NixVersion, w.RFSwiftVersion)
		if w.RFSwiftVersion != "unknown" && w.RFSwiftVersion != common.Version {
			st.Detail += fmt.Sprintf(" (Workbench is %s: 'rfswift nix wsl setup --update' aligns them)", common.Version)
		}
	default:
		st.Detail = fmt.Sprintf("%s is missing %s", w.Distro, strings.Join(st.Missing, " and "))
	}
	return st
}

// NixWSLSetup provisions the WSL 2 distribution for the Nix engine from the
// GUI (Windows): systemd, Nix with flakes and the Linux rfswift CLI, all as
// root inside the distribution, so no password prompt is involved. Output
// lines stream as "rfswift:nix-wsl-setup" events. A distribution must exist
// already: installing one runs an interactive first boot that needs a console
// ('wsl --install -d Ubuntu' in a terminal).
func (a *App) NixWSLSetup() (NixEngineStatus, error) {
	if _, err := a.requireLocal(); err != nil {
		return NixEngineStatus{}, err
	}
	if runtime.GOOS != "windows" {
		return a.NixEngineStatus(), errors.New("the Nix engine runs natively here; nothing to set up in WSL")
	}
	w := &setupEventWriter{a: a}
	_, err := rfnix.SetupWSL(rfnix.WSLSetupOptions{Yes: true, Output: w, Log: w.emit})
	w.flush()
	rfnix.ResetWSLBackend()
	return a.NixEngineStatus(), err
}

var setupANSI = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

// setupEventWriter turns provisioning output into per-line GUI events.
type setupEventWriter struct {
	a   *App
	mu  sync.Mutex
	buf []byte
}

func (w *setupEventWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		w.emit(line)
	}
	return len(p), nil
}

// flush emits a trailing line without a newline.
func (w *setupEventWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = nil
	}
}

func (w *setupEventWriter) emit(line string) {
	line = strings.TrimRight(setupANSI.ReplaceAllString(line, ""), " \t")
	if strings.TrimSpace(line) == "" || w.a.ctx == nil {
		return
	}
	wruntime.EventsEmit(w.a.ctx, "rfswift:nix-wsl-setup", map[string]any{"line": line})
}

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
