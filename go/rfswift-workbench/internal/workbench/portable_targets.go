package workbench

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
)

// ExportTarget exports a selected container filesystem or a complete portable
// Nix environment. routeLocalMission is important when Docker, Podman and Lima
// are running together: it points the legacy transfer helper at the daemon
// that actually owns the selected container.
func (a *App) ExportTarget(id, engine string) (string, error) {
	if _, local := a.eng.(*LocalEngine); !local {
		return "", errors.New("target export is currently available for local targets only")
	}
	id = strings.TrimSpace(id)
	if !validWorkspaceName(id) {
		return "", errors.New("invalid target name")
	}
	if err := a.requireMission(id); err != nil {
		return "", err
	}
	if engine == "nix" {
		path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
			Title:           "Export RF Swift Nix environment",
			DefaultFilename: id + ".rfenv",
			Filters:         []wruntime.FileFilter{{DisplayName: "RF Swift environment", Pattern: "*.rfenv"}},
		})
		if err != nil || path == "" {
			return "", err
		}
		if filepath.Ext(path) == "" {
			path += ".rfenv"
		}
		return path, rfnix.ExportEnvironment(id, path)
	}
	if engine != "docker" && engine != "podman" && engine != "lima" {
		return "", errors.New("unsupported container engine")
	}
	a.routeLocalMission(id)
	active := rfdock.GetEngine()
	if string(active.Type()) != engine {
		return "", errors.New("could not route the selected container to its engine")
	}
	if !active.IsServiceRunning() {
		return "", errors.New("the container engine is stopped")
	}
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Export RF Swift container",
		DefaultFilename: id + "-" + time.Now().Format("20060102-150405") + ".tar.gz",
		Filters:         []wruntime.FileFilter{{DisplayName: "Compressed container archive", Pattern: "*.tar.gz"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(path), ".tar.gz") {
		path += ".tar.gz"
	}
	return path, rfdock.ExportContainer(id, path)
}

// ImportContainerArchive imports a filesystem archive into the explicitly
// selected Docker-compatible daemon, including Docker inside a Lima VM. It
// never starts that daemon: the GUI obtains consent first.
func (a *App) ImportContainerArchive(engine, imageName string) (string, error) {
	if _, local := a.eng.(*LocalEngine); !local {
		return "", errors.New("container import is currently available for local targets only")
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	imageName = strings.TrimSpace(imageName)
	if imageName == "" || strings.ContainsAny(imageName, "\r\n\t ") {
		return "", errors.New("a valid image name and tag are required")
	}
	eng := engineByType(rfdock.EngineType(engine))
	if eng == nil || (engine != "docker" && engine != "podman" && engine != "lima") {
		return "", errors.New("select Docker, Podman, or Lima")
	}
	if !eng.IsAvailable() {
		return "", errors.New(engine + " is not installed")
	}
	if !eng.IsServiceRunning() {
		return "", errors.New(engine + " is stopped")
	}
	resetEngineEnv()
	rfdock.SetPreferredEngine(engine)
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   "Import RF Swift container archive",
		Filters: []wruntime.FileFilter{{DisplayName: "Container archive", Pattern: "*.tar.gz;*.tgz;*.tar"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, rfdock.ImportContainer(path, imageName)
}

func (a *App) ImportNixEnvironment(name string) (string, error) {
	if _, local := a.eng.(*LocalEngine); !local {
		return "", errors.New("Nix environment import is currently available for local targets only")
	}
	name = strings.TrimSpace(name)
	if name != "" && !validWorkspaceName(name) {
		return "", errors.New("invalid environment name")
	}
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   "Import RF Swift Nix environment",
		Filters: []wruntime.FileFilter{{DisplayName: "RF Swift environment", Pattern: "*.rfenv"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, rfnix.ImportEnvironment(path, name, "")
}
