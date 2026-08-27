package workbench

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
)

// Native dialog backends do not interpret multi-extension patterns
// consistently (notably Cocoa versus GTK). Keep each extension separate and
// always offer an unrestricted fallback so valid archives never disappear.
func nixEnvironmentFilters() []wruntime.FileFilter {
	return []wruntime.FileFilter{
		{DisplayName: "RF Swift environment (.rfenv)", Pattern: "*.rfenv"},
		{DisplayName: "Encrypted RF Swift environment (.rfenv.age)", Pattern: "*.rfenv.age"},
		{DisplayName: "All files", Pattern: "*.*"},
	}
}

func containerArchiveFilters() []wruntime.FileFilter {
	return []wruntime.FileFilter{
		{DisplayName: "Compressed tar archive (.tar.gz)", Pattern: "*.tar.gz"},
		{DisplayName: "Encrypted container archive (.tar.gz.age)", Pattern: "*.tar.gz.age"},
		{DisplayName: "Compressed tar archive (.tgz)", Pattern: "*.tgz"},
		{DisplayName: "Tar archive (.tar)", Pattern: "*.tar"},
		{DisplayName: "All files", Pattern: "*.*"},
	}
}

func ensureArchiveExtension(path, extension string) string {
	if !strings.HasSuffix(strings.ToLower(path), strings.ToLower(extension)) {
		return path + extension
	}
	return path
}

func (a *App) transferProgress(operation, kind, target string, percent int, stage string, bytes, total int64) {
	wruntime.EventsEmit(a.ctx, "rfswift:transfer-progress", map[string]any{
		"operation": operation, "kind": kind, "target": target,
		"percent": percent, "stage": stage, "bytes": bytes, "total": total,
	})
}

// ExportTarget exports a selected container filesystem or a complete portable
// Nix environment. routeLocalMission is important when Docker, Podman and Lima
// are running together: it points the legacy transfer helper at the daemon
// that actually owns the selected container.
func (a *App) ExportTarget(id, engine, password string) (string, error) {
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
		extension := ".rfenv"
		if password != "" {
			extension += ".age"
		}
		path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
			Title:           "Export RF Swift Nix environment",
			DefaultFilename: id + extension,
			Filters:         nixEnvironmentFilters(),
		})
		if err != nil || path == "" {
			return "", err
		}
		path = ensureArchiveExtension(path, extension)
		output, cleanup, err := exportWorkingPath(path, password)
		if err != nil {
			return "", err
		}
		defer cleanup()
		err = rfnix.ExportEnvironmentWithProgress(id, output, func(percent int, stage string) {
			percent = percent * 75 / 100
			a.transferProgress("export", "nix", id, percent, stage, 0, 0)
		})
		if err == nil && password != "" {
			a.transferProgress("export", "nix", id, 82, "Encrypting portable environment", 0, 0)
			err = encryptArchive(output, path, password)
		}
		if err == nil {
			a.transferProgress("export", "nix", id, 90, "Exporting Workbench mission data", 0, 0)
			_, err = a.exportMissionNotes(id, path, password)
		}
		if err == nil {
			a.transferProgress("export", "nix", id, 100, "Environment and notes export complete", 0, 0)
		}
		if err != nil {
			a.transferProgress("export", "nix", id, 0, "Export failed: "+err.Error(), 0, 0)
		}
		return path, err
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
	extension := ".tar.gz"
	if password != "" {
		extension += ".age"
	}
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Export RF Swift container",
		DefaultFilename: id + "-" + time.Now().Format("20060102-150405") + extension,
		Filters:         containerArchiveFilters(),
	})
	if err != nil || path == "" {
		return "", err
	}
	path = ensureArchiveExtension(path, extension)
	output, cleanup, err := exportWorkingPath(path, password)
	if err != nil {
		return "", err
	}
	defer cleanup()
	err = rfdock.ExportContainerWithProgress(id, output, func(percent int, stage string, bytes int64) {
		percent = percent * 75 / 100
		a.transferProgress("export", "container", id, percent, stage, bytes, 0)
	})
	if err == nil && password != "" {
		a.transferProgress("export", "container", id, 82, "Encrypting container archive", 0, 0)
		err = encryptArchive(output, path, password)
	}
	if err == nil {
		a.transferProgress("export", "container", id, 90, "Exporting Workbench mission data", 0, 0)
		_, err = a.exportMissionNotes(id, path, password)
	}
	if err == nil {
		a.transferProgress("export", "container", id, 100, "Container and notes export complete", 0, 0)
	}
	if err != nil {
		a.transferProgress("export", "container", id, 0, "Export failed: "+err.Error(), 0, 0)
	}
	return path, err
}

// ImportContainerArchive imports a filesystem archive into the explicitly
// selected Docker-compatible daemon, including Docker inside a Lima VM. It
// never starts that daemon: the GUI obtains consent first.
func (a *App) ImportContainerArchive(engine, imageName, password string) (string, error) {
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
		Filters: containerArchiveFilters(),
	})
	if err != nil || path == "" {
		return "", err
	}
	input, cleanup, err := importWorkingPath(path, password)
	if err != nil {
		return path, err
	}
	defer cleanup()
	err = rfdock.ImportContainerWithProgress(input, imageName, func(percent int, stage string, bytes, total int64) {
		a.transferProgress("import", "container", imageName, percent, stage, bytes, total)
	})
	if err != nil {
		a.transferProgress("import", "container", imageName, 0, "Import failed: "+err.Error(), 0, 0)
	}
	return path, err
}

type PortableImportResult struct {
	Path           string `json:"path"`
	Target         string `json:"target"`
	NotesAvailable bool   `json:"notesAvailable"`
}

func (a *App) ImportNixEnvironment(name, password string) (PortableImportResult, error) {
	if _, local := a.eng.(*LocalEngine); !local {
		return PortableImportResult{}, errors.New("Nix environment import is currently available for local targets only")
	}
	name = strings.TrimSpace(name)
	if name != "" && !validWorkspaceName(name) {
		return PortableImportResult{}, errors.New("invalid environment name")
	}
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   "Import RF Swift Nix environment",
		Filters: nixEnvironmentFilters(),
	})
	if err != nil || path == "" {
		return PortableImportResult{}, err
	}
	input, cleanup, err := importWorkingPath(path, password)
	if err != nil {
		return PortableImportResult{Path: path}, err
	}
	defer cleanup()
	target := name
	if target == "" {
		target, err = rfnix.PortableEnvironmentName(input)
		if err != nil {
			return PortableImportResult{Path: path}, err
		}
	}
	if err := a.ensureNixImportNameAvailable(target); err != nil {
		return PortableImportResult{Path: path, Target: target}, err
	}
	err = rfnix.ImportEnvironmentWithProgress(input, name, "", func(percent int, stage string) {
		a.transferProgress("import", "nix", target, percent, stage, 0, 0)
	})
	if err != nil {
		a.transferProgress("import", "nix", target, 0, "Import failed: "+err.Error(), 0, 0)
	}
	return PortableImportResult{Path: path, Target: target, NotesAvailable: notesCompanionPath(path) != ""}, err
}

// ensureNixImportNameAvailable prevents a Nix environment and a container from
// sharing the project-wide mission ID used by notes, findings and recordings.
// Preserved metadata from a previously removed Nix environment is allowed so
// importing its archive can intentionally restore that mission.
func (a *App) ensureNixImportNameAvailable(target string) error {
	live, err := a.eng.ListTargets()
	if err != nil {
		return fmt.Errorf("could not check existing mission names: %w", err)
	}
	for _, mission := range live {
		if mission.ID == target {
			return fmt.Errorf("mission name %q is already used by a live %s target", target, mission.Engine)
		}
	}
	saved, err := a.store.ListMissions(a.ws)
	if err != nil {
		return fmt.Errorf("could not check saved mission names: %w", err)
	}
	for _, mission := range saved {
		if mission.ID == target && mission.Engine != "nix" {
			return fmt.Errorf("mission name %q belongs to a preserved %s mission; choose another import name", target, mission.Engine)
		}
	}
	return nil
}

func exportWorkingPath(destination, password string) (string, func(), error) {
	if password == "" {
		return destination, func() {}, nil
	}
	f, err := os.CreateTemp("", "rfswift-export-*")
	if err != nil {
		return "", func() {}, err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", func() {}, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}

func importWorkingPath(source, password string) (string, func(), error) {
	encrypted, err := archiveIsEncrypted(source)
	if err != nil || !encrypted {
		return source, func() {}, err
	}
	f, err := os.CreateTemp("", "rfswift-import-*")
	if err != nil {
		return "", func() {}, err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", func() {}, err
	}
	_ = os.Remove(path) // decryptArchive creates the private destination exclusively.
	if err := decryptArchive(source, path, password); err != nil {
		_ = os.Remove(path)
		return "", func() {}, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}
