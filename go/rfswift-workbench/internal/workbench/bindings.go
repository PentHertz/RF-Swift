package workbench

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
)

// This file is the bound API surface: every exported method is callable from the
// frontend. Methods are thin - persistence lives in the Store, security/report
// logic in report.go / connection.go, and engine actions behind the Engine
// interface (local now, remote agent later).

// --- workspaces ---

func (a *App) Workspaces() ([]string, error) { return a.store.ListWorkspaces() }
func (a *App) CurrentWorkspace() string      { return a.ws }
func (a *App) OpenWorkspace(name string) error {
	if err := a.store.CreateWorkspace(name); err != nil {
		return err
	}
	a.ws = name
	return nil
}

func (a *App) CloseWorkspace() (string, error) {
	const fallback = "default"
	if err := a.store.CreateWorkspace(fallback); err != nil {
		return "", err
	}
	a.ws = fallback
	return fallback, nil
}

func (a *App) DeleteWorkspace(name string) error {
	if name == a.ws {
		return errors.New("close or switch away from the project before deleting it")
	}
	missions, err := a.store.ListMissions(name)
	if err != nil {
		return err
	}
	for _, mission := range missions {
		items, loadErr := a.store.LoadSecrets(name, mission.ID)
		if loadErr != nil {
			return loadErr
		}
		for _, item := range items {
			if deleteErr := a.secretStore.Delete(secretRef(a.store.Root, name, mission.ID, item.ID)); deleteErr != nil {
				return errors.New("cannot delete project while a secret remains in the OS credential vault: " + deleteErr.Error())
			}
		}
	}
	return a.store.DeleteWorkspace(name)
}

// --- missions ---

// Missions lists the live targets from the engine (containers across the active
// engine + Nix environments). Per-mission notes/findings/captures are stored by
// mission ID in the workspace.
func (a *App) Missions() ([]Mission, error) {
	live, err := a.eng.ListTargets()
	if err != nil {
		return nil, err
	}
	saved, _ := a.store.ListMissions(a.ws)
	byID := make(map[string]Mission, len(saved))
	for _, m := range saved {
		byID[m.ID] = m
	}
	for i := range live {
		if m, ok := byID[live[i].ID]; ok {
			if m.Title != "" {
				live[i].Title = m.Title
			}
			live[i].Notes = m.Notes
			live[i].EnvironmentAudit = m.EnvironmentAudit
			if severityTotal(live[i].EnvironmentAudit) == 0 {
				live[i].EnvironmentAudit = m.Posture // migrate legacy audit counters
			}
		}
		findings, _ := a.store.LoadFindings(a.ws, live[i].ID)
		live[i].FindingSummary = summarizeFindings(findings)
		auditPath := filepath.Join(a.store.missionDir(a.ws, live[i].ID), "environment-audits", "latest.json")
		if _, statErr := os.Stat(auditPath); errors.Is(statErr, os.ErrNotExist) {
			auditPath = filepath.Join(a.store.missionDir(a.ws, live[i].ID), "audits", "latest.json")
		}
		if _, statErr := os.Stat(auditPath); statErr == nil {
			result := parseAuditFile(auditPath)
			live[i].AuditIssues = result.Issues
			if severityTotal(live[i].EnvironmentAudit) == 0 {
				live[i].EnvironmentAudit = result.Posture
			}
		}
		live[i].Posture = Posture{}
		_ = a.store.SaveMission(a.ws, live[i])
	}
	return live, nil
}

func severityTotal(p Posture) int { return p.Crit + p.High + p.Med + p.Low }

func summarizeFindings(findings []Finding) Posture {
	var p Posture
	for _, finding := range findings {
		switch strings.ToLower(finding.Sev) {
		case "critical", "crit":
			p.Crit++
		case "high":
			p.High++
		case "medium", "med":
			p.Med++
		case "low":
			p.Low++
		}
	}
	return p
}

func (a *App) MissionStatuses() (map[string]string, error) {
	targets, err := a.eng.ListTargets()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(targets))
	for _, m := range targets {
		out[m.ID] = m.Status
	}
	return out, nil
}

func (a *App) MissionTemplates() ([]MissionTemplate, error) {
	cat, err := rfnix.LoadCatalog()
	if err != nil {
		return nil, err
	}
	out := make([]MissionTemplate, 0, len(cat.Environments))
	for _, e := range cat.Environments {
		out = append(out, MissionTemplate{Name: e.Name, Description: e.Description, Category: e.Category})
	}
	return out, nil
}

func (a *App) ContainerProfiles() []MissionProfile {
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		var out []MissionProfile
		if err := remoteEngine.call("profiles.list", map[string]any{}, &out); err == nil {
			return out
		}
		return []MissionProfile{}
	}
	// Match `rfswift profile init`: create every missing shipped profile in the
	// platform-specific user configuration directory. Existing YAML files are
	// deliberately preserved, including user-customized versions.
	rfdock.InitDefaultProfiles(false)

	// Always expose current shipped presets. User YAML profiles override presets
	// with the same name and remain available as additional custom profiles.
	profilesByName := make(map[string]rfdock.Profile)
	order := make([]string, 0)
	for _, p := range rfdock.DefaultProfiles() {
		key := strings.ToLower(p.Name)
		profilesByName[key] = p
		order = append(order, key)
	}
	for _, p := range rfdock.LoadProfiles() {
		key := strings.ToLower(p.Name)
		if _, exists := profilesByName[key]; !exists {
			order = append(order, key)
		}
		// Repair the exact RFID layout shipped by older RF-Swift versions.
		if key == "rfid" && p.Devices == "" && strings.Contains(p.Bindings, "/dev/ttyACM0:/dev/ttyACM0") {
			p.Devices = "/dev/ttyACM0:/dev/ttyACM0"
			p.Bindings = removeProfileCSVItem(p.Bindings, "/dev/ttyACM0:/dev/ttyACM0")
			if p.Cgroups == "" {
				p.Cgroups = "c 189:* rwm"
			}
		}
		profilesByName[key] = p
	}
	profiles := make([]rfdock.Profile, 0, len(order))
	for _, key := range order {
		profiles = append(profiles, profilesByName[key])
	}
	out := make([]MissionProfile, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, MissionProfile{Name: p.Name, Description: p.Description, Image: p.Image, Network: p.Network, ExposedPorts: p.ExposedPorts, PortBindings: p.PortBindings, Bindings: p.Bindings, Devices: p.Devices, Caps: p.Caps, Cgroups: p.Cgroups, GPUs: p.GPUs, Privileged: p.Privileged, Realtime: p.Realtime, Desktop: p.Desktop, DesktopSSL: p.DesktopSSL, NoX11: p.NoX11})
	}
	return out
}

func (a *App) ContainerDefaults() (ContainerDefaults, error) {
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		var out ContainerDefaults
		err := remoteEngine.call("profiles.defaults", map[string]any{}, &out)
		return out, err
	}
	d, err := rfdock.LoadCreationDefaults()
	if err != nil {
		return ContainerDefaults{}, err
	}
	return ContainerDefaults{Path: d.Path, Image: d.Image, Shell: d.Shell, Bindings: d.Bindings,
		Network: d.Network, ExposedPorts: d.ExposedPorts, PortBindings: d.PortBindings,
		ExtraHosts: d.ExtraHosts, Environment: d.Environment, Devices: d.Devices,
		Privileged: d.Privileged, Caps: d.Caps, Seccomp: d.Seccomp, CgroupRules: d.CgroupRules,
		DesktopProto: d.DesktopProto, DesktopHost: d.DesktopHost, DesktopPort: d.DesktopPort,
		DesktopPass: d.DesktopPass, DesktopSSL: d.DesktopSSL}, nil
}

func removeProfileCSVItem(value, unwanted string) string {
	items := strings.Split(value, ",")
	out := items[:0]
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" && item != unwanted {
			out = append(out, item)
		}
	}
	return strings.Join(out, ",")
}

func (a *App) SelectWorkspaceDirectory() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Select mission workspace"})
}

func (a *App) SelectRecordingDirectory() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Select terminal recording destination"})
}

func (a *App) SelectHostPath(directory bool) (string, error) {
	if directory {
		return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Select host directory"})
	}
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Select host device or file"})
}

func (a *App) OpenExternalURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "vnc" && u.Scheme != "vncs") || u.Host == "" {
		return errors.New("invalid desktop URL")
	}
	wruntime.BrowserOpenURL(a.ctx, raw)
	return nil
}

func (a *App) BeginMissionCreation(name string) error {
	if !validWorkspaceName(name) {
		return errors.New("mission name must be a single safe path component")
	}
	if _, err := os.Stat(a.store.missionDir(a.ws, name)); err == nil {
		var existing Mission
		if readErr := readJSON(filepath.Join(a.store.missionDir(a.ws, name), "mission.json"), &existing); readErr != nil || existing.Engine != "nix" {
			return errors.New("a mission with this name already exists in the current project; choose another name or remove the preserved mission data first")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not check the mission name: %w", err)
	}
	a.createMu.Lock()
	defer a.createMu.Unlock()
	if a.createCancel != nil {
		return errors.New("another mission creation is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.createCancel = cancel
	a.creationContext = ctx
	a.createName = name
	return nil
}

func (a *App) CancelMissionCreation(name string) bool {
	a.createMu.Lock()
	defer a.createMu.Unlock()
	if a.createCancel == nil || a.createName != name {
		return false
	}
	a.createCancel()
	return true
}

func (a *App) FinishMissionCreation(name string) {
	a.createMu.Lock()
	defer a.createMu.Unlock()
	if a.createName != name {
		return
	}
	if a.createCancel != nil {
		a.createCancel()
	}
	a.createCancel = nil
	a.creationContext = nil
	a.createName = ""
}

func (a *App) CreateMission(req MissionCreate) (Mission, error) {
	if !validWorkspaceName(req.Name) {
		return Mission{}, errors.New("mission name must be a single safe path component")
	}
	if _, err := os.Stat(a.store.missionDir(a.ws, req.Name)); err == nil {
		var existing Mission
		if readErr := readJSON(filepath.Join(a.store.missionDir(a.ws, req.Name), "mission.json"), &existing); readErr != nil || existing.Engine != "nix" || req.Engine != "nix" {
			return Mission{}, errors.New("a mission with this name already exists in the current project")
		}
	} else if !os.IsNotExist(err) {
		return Mission{}, fmt.Errorf("could not check the mission name: %w", err)
	}
	a.createMu.Lock()
	ctx := context.Context(nil)
	if a.createName == req.Name && a.createCancel != nil {
		ctx = a.creationContext
	}
	a.createMu.Unlock()
	if ctx != nil {
		req.Context = ctx
	}
	a.emitOperationProgress("mission-create", req.Name, 55, "Creating target")
	m, err := a.eng.Create(req)
	if err != nil {
		return Mission{}, err
	}
	if err := a.store.SaveMission(a.ws, m); err != nil {
		return Mission{}, err
	}
	a.emitOperationProgress("mission-create", req.Name, 100, "Mission ready")
	return m, nil
}

func (a *App) PullMissionImage(engine, image string) (string, error) {
	return a.pullMissionImage(context.Background(), engine, image)
}

func (a *App) PullMissionImageForMission(mission, engine, image string) (string, error) {
	a.createMu.Lock()
	ctx := context.Context(nil)
	if a.createName == mission && a.createCancel != nil {
		ctx = a.creationContext
	}
	a.createMu.Unlock()
	if ctx == nil {
		return "", errors.New("mission creation is not active")
	}
	return a.pullMissionImage(ctx, engine, image)
}

func (a *App) pullMissionImage(ctx context.Context, engine, image string) (string, error) {
	if engine != "docker" && engine != "podman" && engine != "lima" {
		return "", errors.New("select Docker, Podman, or Lima before pulling an image")
	}
	if strings.TrimSpace(image) == "" {
		return "", errors.New("image name is required")
	}
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		var resolved string
		if err := remoteEngine.callContext(ctx, "images.pull", map[string]string{"engine": engine, "image": image}, &resolved); err != nil {
			return "", err
		}
		return resolved, nil
	}
	resetEngineEnv()
	resolved, err := rfdock.PullImageContext(ctx, engine, image, func(p rfdock.PullProgress) {
		wruntime.EventsEmit(a.ctx, "rfswift:image-pull", map[string]any{
			"image": p.Image, "layer": p.Layer, "status": p.Status,
			"current": p.Current, "total": p.Total,
		})
	})
	if err != nil {
		return "", err
	}
	// Keep official short names short in the creation form; the engine retains
	// responsibility for resolving the configured repository internally.
	if !strings.Contains(image, "/") && !strings.Contains(image, ":") {
		return strings.TrimSpace(image), nil
	}
	return resolved, nil
}

func (a *App) CheckMissionImage(engine, image string) (rfdock.ImageAvailability, error) {
	if engine != "docker" && engine != "podman" && engine != "lima" {
		return rfdock.ImageAvailability{}, errors.New("select Docker, Podman, or Lima")
	}
	if strings.TrimSpace(image) == "" {
		return rfdock.ImageAvailability{}, errors.New("image name is required")
	}
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		var availability rfdock.ImageAvailability
		err := remoteEngine.call("images.check", map[string]string{"engine": engine, "image": image}, &availability)
		return availability, err
	}
	resetEngineEnv()
	return rfdock.CheckImage(engine, image)
}

// InspectMission returns a mission's full configuration and network.
func (a *App) InspectMission(id string) (Mission, error) {
	if err := a.requireMission(id); err != nil {
		return Mission{}, err
	}
	return a.eng.Inspect(id)
}

func (a *App) SaveMission(m Mission) error {
	if !validWorkspaceName(strings.TrimSpace(m.ID)) {
		return errors.New("mission id must be a single non-empty path component")
	}
	return a.store.SaveMission(a.ws, m)
}

// StartMission / StopMission start or stop the underlying container. Nix
// environments are native tool closures and therefore have no daemon lifecycle.
func (a *App) StartMission(id string) error {
	if err := a.requireMission(id); err != nil {
		return err
	}
	return a.eng.Start(id)
}
func (a *App) StopMission(id string) error {
	if err := a.requireMission(id); err != nil {
		return err
	}
	return a.eng.Stop(id)
}

func (a *App) ConfigureContainer(id string, change ContainerChange) error {
	if err := a.requireMission(id); err != nil {
		return err
	}
	value := strings.TrimSpace(change.Value)
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		return remoteEngine.Configure(id, change)
	}
	a.routeLocalMission(id)
	switch change.Kind {
	case "volume", "device":
		return rfdock.UpdateBinding(id, change.Kind, change.Source, change.Target, change.Add)
	case "device-bind":
		return rfdock.UpdateBinding(id, "volume", change.Source, change.Target, change.Add)
	case "capability":
		if value == "" {
			return errors.New("capability is required")
		}
		return rfdock.UpdateCapability(id, value, change.Add)
	case "cgroup":
		if value == "" {
			return errors.New("cgroup rule is required")
		}
		return rfdock.UpdateCgroupRule(id, value, change.Add)
	case "gpu":
		if value == "" {
			return errors.New("GPU selection is required")
		}
		return rfdock.UpdateGPUs(id, value, change.Add)
	case "exposed-port":
		if value == "" {
			return errors.New("port is required")
		}
		return rfdock.UpdateExposedPort(id, value, change.Add)
	case "published-port":
		if value == "" {
			return errors.New("port binding is required")
		}
		return rfdock.UpdatePortBinding(id, value, change.Add)
	default:
		return fmt.Errorf("unsupported container setting %q", change.Kind)
	}
}

func (a *App) DeleteContainer(id string) error {
	if err := a.requireMission(id); err != nil {
		return err
	}
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		return remoteEngine.Delete(id, false, false)
	}
	a.routeLocalMission(id)
	return rfdock.RemoveContainer(id)
}

// routeLocalMission points rfdock's global engine at the engine hosting this
// mission before helpers that operate on the global engine (configure,
// install scripts, removal) — the container may live inside the Lima VM.
func (a *App) routeLocalMission(id string) {
	if local, ok := a.eng.(*LocalEngine); ok {
		local.RouteMission(id)
	}
}

func (a *App) DeleteNixEnvironment(id string, cleanStore bool) error {
	if err := a.requireMission(id); err != nil {
		return err
	}
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		return remoteEngine.Delete(id, true, cleanStore)
	}
	if err := rfnix.RemoveEnvironment(id); err != nil {
		return err
	}
	if cleanStore {
		return rfnix.GarbageCollect(rfnix.GCOptions{})
	}
	return nil
}

// DeleteMissionCompletely removes the live target and all Workbench-owned
// mission data. Host bind mounts, named volumes and images remain outside the
// Workbench data root and are intentionally preserved.
func (a *App) DeleteMissionCompletely(id, engine string) error {
	if err := a.requireMission(id); err != nil {
		return err
	}
	if engine == "nix" {
		if err := a.DeleteNixEnvironment(id, false); err != nil {
			return err
		}
	} else if engine == "docker" || engine == "podman" || engine == "lima" {
		if err := a.DeleteContainer(id); err != nil {
			return err
		}
	} else {
		return errors.New("unsupported mission engine")
	}
	return a.store.DeleteMission(a.ws, id)
}

// Exec runs a command inside a mission's container and returns its output.
func (a *App) Exec(missionID, cmd string) (string, error) {
	if err := a.requireMission(missionID); err != nil {
		return "", err
	}
	out, err := a.eng.Exec(missionID, cmd)
	if err != nil && strings.TrimSpace(out) != "" {
		// Wails rejects the Promise when err is non-nil and otherwise discards the
		// Go return value. Preserve stderr/stdout in the rejection so the console
		// can show the command's useful diagnostic rather than only "exit status 1".
		return "", fmt.Errorf("%s\n%w", strings.TrimRight(out, "\r\n"), err)
	}
	return out, err
}

func (a *App) SearchMissionTools(mission, query string, allNixpkgs bool) ([]ToolCandidate, error) {
	if err := a.requireMission(mission); err != nil {
		return nil, err
	}
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		var out []ToolCandidate
		err := remoteEngine.call("tools.search", map[string]any{"mission": mission, "query": query, "allNixpkgs": allNixpkgs}, &out)
		return out, err
	}
	m, err := a.eng.Inspect(mission)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	var out []ToolCandidate
	if m.Engine != "nix" {
		a.routeLocalMission(mission)
		for _, name := range rfdock.ListInstallFunctions(mission) {
			if query == "" || strings.Contains(strings.ToLower(name), query) {
				out = append(out, ToolCandidate{Name: name, Detail: strings.TrimSuffix(name, "_install"), Source: "container script"})
			}
		}
		return out, nil
	}
	env, err := rfnix.GetEnvironment(mission)
	if err != nil {
		return nil, err
	}
	if allNixpkgs {
		hits, err := rfnix.SearchNixpkgs(env.FlakeRef, query)
		if err != nil {
			return nil, err
		}
		for name, detail := range hits {
			out = append(out, ToolCandidate{Name: name, Detail: detail, Source: "nixpkgs"})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out, nil
	}
	for _, hit := range rfnix.SearchPackages(query) {
		out = append(out, ToolCandidate{Name: hit.Name, Detail: strings.Join(hit.Envs, ", "), Source: "RF Swift"})
	}
	return out, nil
}

func (a *App) InstallMissionTool(mission, name string) error {
	if err := a.requireMission(mission); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("tool name is required")
	}
	if remoteEngine, ok := a.eng.(*RemoteEngine); ok {
		a.emitOperationProgress("package-install", mission, 10, "Starting remote installation")
		err := remoteEngine.call("tools.install", map[string]string{"mission": mission, "name": name}, nil)
		if err == nil {
			a.emitOperationProgress("package-install", mission, 100, "Package installed")
		}
		return err
	}
	if env, err := rfnix.GetEnvironment(mission); err == nil {
		return rfnix.InstallPackagesWithProgress(env.FlakeRef, []string{name}, mission, func(percent int, stage string) {
			a.emitOperationProgress("package-install", mission, percent, stage)
		})
	}
	a.routeLocalMission(mission)
	a.emitOperationProgress("package-install", mission, 20, "Running container installer")
	err := rfdock.ContainerInstallScript(mission, "entrypoint.sh", name)
	if err == nil {
		a.emitOperationProgress("package-install", mission, 100, "Tool installed")
	}
	return err
}

func (a *App) emitOperationProgress(operation, target string, percent int, stage string) {
	if a.ctx == nil {
		return
	}
	wruntime.EventsEmit(a.ctx, "rfswift:operation-progress", map[string]any{
		"operation": operation, "target": target, "percent": percent, "stage": stage,
	})
}

// --- notebook ---

func (a *App) GetNote(mission, name string) (string, error) {
	return a.store.GetNote(a.ws, mission, name)
}
func (a *App) SaveNote(mission, name, body string) error {
	if err := a.store.SaveNote(a.ws, mission, name, body); err != nil {
		return err
	}
	// A managed recording embedded in an AI-readable mission note is evidence,
	// not merely a visual pointer. Register completed recordings immediately;
	// active/incomplete recordings remain unavailable until a later save/read.
	a.approveReferencedTerminalRecordings(mission, body)
	return nil
}

var terminalRecordingDirective = regexp.MustCompile(`:::terminal-recording\s+(.+?)\s*:::`)

func (a *App) approveReferencedTerminalRecordings(mission, body string) {
	seen := map[string]bool{}
	for _, match := range terminalRecordingDirective.FindAllStringSubmatch(body, -1) {
		if len(match) != 2 {
			continue
		}
		name := filepath.Base(strings.TrimSpace(match[1]))
		if seen[name] || !strings.EqualFold(filepath.Ext(name), ".cast") {
			continue
		}
		seen[name] = true
		_, _ = a.RegisterTerminalRecordingEvidence(mission, name)
	}
}
func (a *App) ListNotes(mission string) ([]string, error) { return a.store.ListNotes(a.ws, mission) }

type NoteImage struct {
	Path    string `json:"path"`
	DataURL string `json:"dataURL"`
}

func (a *App) saveNoteImage(mission, name string, data []byte) (NoteImage, error) {
	if err := a.requireMission(mission); err != nil {
		return NoteImage{}, err
	}
	if len(data) == 0 || len(data) > 20<<20 {
		return NoteImage{}, errors.New("note image must be between 1 byte and 20 MiB")
	}
	mime := http.DetectContentType(data)
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}[mime]
	if ext == "" {
		return NoteImage{}, errors.New("clipboard content is not a supported image")
	}
	base := strings.TrimSuffix(safeName(name), filepath.Ext(name))
	if base == "" || base == "." {
		base = "image"
	}
	fileName := fmt.Sprintf("%s-%s%s", base, time.Now().Format("20060102-150405.000"), ext)
	rel := filepath.ToSlash(filepath.Join("assets", fileName))
	path := filepath.Join(a.store.notesDir(a.ws, mission), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return NoteImage{}, err
	}
	if err := writePrivateFile(path, data); err != nil {
		return NoteImage{}, err
	}
	return NoteImage{Path: rel, DataURL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)}, nil
}

func (a *App) SaveNoteImage(mission, name, dataURL string) (NoteImage, error) {
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 || !strings.Contains(dataURL[:comma], ";base64") {
		return NoteImage{}, errors.New("invalid clipboard image data")
	}
	data, err := base64.StdEncoding.DecodeString(dataURL[comma+1:])
	if err != nil {
		return NoteImage{}, errors.New("invalid clipboard image encoding")
	}
	return a.saveNoteImage(mission, name, data)
}

func (a *App) ImportNoteImage(mission, rawPath string) (NoteImage, error) {
	rawPath = strings.TrimSpace(rawPath)
	if strings.HasPrefix(rawPath, "file://") {
		encoded := strings.TrimPrefix(rawPath, "file://")
		decoded, err := url.PathUnescape(encoded)
		if err != nil {
			return NoteImage{}, errors.New("invalid pasted image URI")
		}
		rawPath = decoded
		if strings.HasPrefix(rawPath, "localhost/") {
			rawPath = "/" + strings.TrimPrefix(rawPath, "localhost/")
		}
		if runtime.GOOS == "windows" {
			if strings.HasPrefix(rawPath, "/") && len(rawPath) > 2 && rawPath[2] == ':' {
				rawPath = rawPath[1:]
			} else if !strings.HasPrefix(rawPath, "/") && !filepath.IsAbs(rawPath) {
				rawPath = `\\` + filepath.FromSlash(rawPath)
			}
		}
	}
	info, err := os.Stat(rawPath)
	if err != nil && runtime.GOOS == "linux" {
		if resolved := resolvePortalImage(filepath.Base(rawPath)); resolved != "" {
			rawPath = resolved
			info, err = os.Stat(rawPath)
		}
	}
	if err != nil || info.IsDir() {
		return NoteImage{}, errors.New("pasted image path is not a readable file")
	}
	if info.Size() > 20<<20 {
		return NoteImage{}, errors.New("note image exceeds 20 MiB")
	}
	data, err := os.ReadFile(rawPath)
	if err != nil {
		return NoteImage{}, err
	}
	return a.saveNoteImage(mission, filepath.Base(rawPath), data)
}

func resolvePortalImage(name string) string {
	if name == "" || filepath.Base(name) != name {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, root := range []string{filepath.Join(home, "Pictures"), filepath.Join(home, "Desktop"), filepath.Join(home, "Downloads")} {
		var found string
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || found != "" {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil && strings.Count(filepath.ToSlash(rel), "/") > 4 && entry.IsDir() {
				return filepath.SkipDir
			}
			if !entry.IsDir() && entry.Name() == name {
				found = path
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	return ""
}

func (a *App) ReadNoteImage(mission, rel string) (string, error) {
	if err := a.requireMission(mission); err != nil {
		return "", err
	}
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(rel)))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.Dir(clean) != "assets" {
		return "", errors.New("image is outside the mission note assets")
	}
	assetsRoot, err := filepath.EvalSymlinks(filepath.Join(a.store.notesDir(a.ws, mission), "assets"))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(a.store.notesDir(a.ws, mission), clean))
	if err != nil {
		return "", err
	}
	within, err := filepath.Rel(assetsRoot, resolved)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", errors.New("image resolves outside the mission note assets")
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return "", errors.New("note asset is not an image")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (a *App) ReadTerminalRecording(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ToLower(filepath.Ext(path)) != ".cast" {
		return "", errors.New("a .cast terminal recording is required")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(a.store.Root)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("terminal recording is outside the managed Workbench store")
	}
	if !strings.Contains(filepath.ToSlash(rel), "/recordings/") && !strings.Contains(filepath.ToSlash(rel), "/captures/") {
		return "", errors.New("terminal recording is not managed mission evidence")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.Size() > 50<<20 {
		return "", errors.New("terminal recording exceeds the 50 MiB note playback limit")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("terminal recording is not a regular file")
	}
	b, err := os.ReadFile(resolved)
	return string(b), err
}

// RegisteredTerminalRecordingPath resolves an approved capture to its managed
// .cast copy for the built-in player. It never accepts an arbitrary path from
// the frontend.
func (a *App) RegisteredTerminalRecordingPath(mission, name string) (string, error) {
	if err := a.requireMission(mission); err != nil {
		return "", err
	}
	name = safeName(name)
	if !strings.EqualFold(filepath.Ext(name), ".cast") {
		return "", errors.New("registered capture is not a terminal recording")
	}
	captures, err := a.store.ListCaptures(a.ws, mission)
	if err != nil {
		return "", err
	}
	for _, capture := range captures {
		if capture.Name == name {
			path := filepath.Join(a.store.capturesDir(a.ws, mission), name)
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
				return "", errors.New("registered terminal recording file is unavailable")
			}
			return path, nil
		}
	}
	return "", errors.New("registered terminal recording not found")
}

type TerminalRecordingInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

// RegisterTerminalRecordingEvidence copies a completed managed recording into
// the mission capture store and explicitly approves its text for the
// mission-scoped AI/MCP bridge. The original recording remains available for
// playback and project export.
func (a *App) RegisterTerminalRecordingEvidence(mission, name string) (Capture, error) {
	if err := a.requireMission(mission); err != nil {
		return Capture{}, err
	}
	name = strings.TrimSpace(name)
	if filepath.Base(name) != name || strings.ToLower(filepath.Ext(name)) != ".cast" {
		return Capture{}, errors.New("invalid terminal recording name")
	}
	source := filepath.Join(a.store.missionDir(a.ws, mission), "recordings", name)
	info, err := os.Stat(source)
	if err != nil {
		return Capture{}, err
	}
	if !info.Mode().IsRegular() {
		return Capture{}, errors.New("terminal recording is not a regular file")
	}
	a.termMu.Lock()
	for _, session := range a.terminals {
		if session.record != nil && session.recordPath == source {
			a.termMu.Unlock()
			return Capture{}, errors.New("stop the recording before using it as evidence")
		}
	}
	a.termMu.Unlock()
	data, err := os.ReadFile(source)
	if err != nil {
		return Capture{}, err
	}
	if _, _, err := readArtifactAIContent(source); err != nil {
		return Capture{}, fmt.Errorf("recording cannot be exposed to AI: %w", err)
	}
	sum := sha256.Sum256(data)
	c := Capture{
		Mission: mission,
		Name:    name,
		Path:    source,
		Type:    "terminal",
		Tool:    "Workbench terminal recorder",
		Note:    "Completed terminal recording explicitly approved as mission AI evidence",
		Meta: map[string]string{
			"Recording path":    filepath.Join("recordings", name),
			"Format":            "asciinema v2",
			"Size":              strconv.FormatInt(info.Size(), 10),
			"SHA-256":           hex.EncodeToString(sum[:]),
			"AI content access": "approved",
		},
	}
	if err := a.store.ImportCapture(a.ws, mission, &c); err != nil {
		if os.IsExist(err) {
			if accessErr := a.SetArtifactAIContentAccess(mission, name, true); accessErr != nil {
				return Capture{}, errors.New("recording evidence is already registered")
			}
			return c, nil
		}
		return Capture{}, err
	}
	return c, nil
}

// ListTerminalRecordings lists evidence kept in this mission's managed
// recordings directory. Custom external destinations remain outside this view.
func (a *App) ListTerminalRecordings(mission string) ([]TerminalRecordingInfo, error) {
	if err := a.requireMission(mission); err != nil {
		return nil, err
	}
	dir := filepath.Join(a.store.missionDir(a.ws, mission), "recordings")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []TerminalRecordingInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]TerminalRecordingInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".cast" {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		out = append(out, TerminalRecordingInfo{Name: entry.Name(), Path: filepath.Join(dir, entry.Name()), Size: info.Size(), Modified: info.ModTime().Unix()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out, nil
}

// RenameTerminalRecording renames only a managed .cast file and updates note
// directives which embed it, keeping evidence links valid.
func (a *App) RenameTerminalRecording(mission, oldName, newName string) (TerminalRecordingInfo, error) {
	if err := a.requireMission(mission); err != nil {
		return TerminalRecordingInfo{}, err
	}
	oldName, newName = strings.TrimSpace(oldName), strings.TrimSpace(newName)
	if filepath.Base(oldName) != oldName || strings.ToLower(filepath.Ext(oldName)) != ".cast" {
		return TerminalRecordingInfo{}, errors.New("invalid recording name")
	}
	if filepath.Ext(newName) == "" {
		newName += ".cast"
	}
	if filepath.Base(newName) != newName || strings.ToLower(filepath.Ext(newName)) != ".cast" || newName == ".cast" {
		return TerminalRecordingInfo{}, errors.New("recording name must be a single .cast filename")
	}
	dir := filepath.Join(a.store.missionDir(a.ws, mission), "recordings")
	oldPath, newPath := filepath.Join(dir, oldName), filepath.Join(dir, newName)
	a.termMu.Lock()
	for _, session := range a.terminals {
		if session.record != nil && session.recordPath == oldPath {
			a.termMu.Unlock()
			return TerminalRecordingInfo{}, errors.New("stop this recording before renaming it")
		}
	}
	a.termMu.Unlock()
	if _, err := os.Stat(newPath); err == nil {
		return TerminalRecordingInfo{}, errors.New("a recording with that name already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return TerminalRecordingInfo{}, err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return TerminalRecordingInfo{}, err
	}
	notes, _ := a.store.ListNotes(a.ws, mission)
	for _, name := range notes {
		body, err := a.store.GetNote(a.ws, mission, name)
		if err == nil && strings.Contains(body, oldPath) {
			_ = a.store.SaveNote(a.ws, mission, name, strings.ReplaceAll(body, oldPath, newPath))
		}
	}
	info, err := os.Stat(newPath)
	if err != nil {
		return TerminalRecordingInfo{}, err
	}
	return TerminalRecordingInfo{Name: newName, Path: newPath, Size: info.Size(), Modified: info.ModTime().Unix()}, nil
}

// --- findings (pwndoc-style) ---

func (a *App) ListFindings(mission string) ([]Finding, error) {
	return a.store.LoadFindings(a.ws, mission)
}

// SaveFinding upserts a finding (by ID) and returns it with an ID assigned.
func (a *App) SaveFinding(mission string, f Finding) (Finding, error) {
	fs, err := a.store.LoadFindings(a.ws, mission)
	if err != nil {
		return f, err
	}
	if f.ID == "" {
		f.ID = "F-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		fs = append(fs, f)
	} else {
		found := false
		for i := range fs {
			if fs[i].ID == f.ID {
				fs[i] = f
				found = true
				break
			}
		}
		if !found {
			fs = append(fs, f)
		}
	}
	return f, a.store.SaveFindings(a.ws, mission, fs)
}

func (a *App) DeleteFinding(mission, id string) error {
	fs, err := a.store.LoadFindings(a.ws, mission)
	if err != nil {
		return err
	}
	out := fs[:0]
	for _, f := range fs {
		if f.ID != id {
			out = append(out, f)
		}
	}
	return a.store.SaveFindings(a.ws, mission, out)
}

// --- captures ---

func (a *App) ListCaptures(mission string) ([]Capture, error) {
	return a.store.ListCaptures(a.ws, mission)
}

func (a *App) AddCapture(mission string, c Capture) error {
	if c.Type == "" {
		c.Type = ClassifyCapture(c.Name)
	}
	c.Mission = mission
	return a.store.AddCapture(a.ws, mission, c)
}

// ImportCaptureDialog selects a real artifact, copies it into the current
// workspace and writes its metadata sidecar. An empty capture means cancel.
func (a *App) ImportCaptureDialog(mission string) (Capture, error) {
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Import assessment capture",
	})
	if err != nil || path == "" {
		return Capture{}, err
	}
	c := Capture{
		Mission: mission,
		Name:    filepath.Base(path),
		Path:    path,
		Type:    ClassifyCapture(path),
		Tool:    "import",
	}
	if err := a.store.ImportCapture(a.ws, mission, &c); err != nil {
		return Capture{}, err
	}
	return c, nil
}

func (a *App) ClassifyCapture(filename string) string { return ClassifyCapture(filename) }

func (a *App) BuiltinCaptureTypes() []CaptureType { return BuiltinCaptureTypes() }
func (a *App) CustomCaptureTypes() []CaptureType  { return a.store.LoadCustomCaptureTypes(a.ws) }
func (a *App) SaveCaptureType(c CaptureType) error {
	return a.store.SaveCustomCaptureType(a.ws, c)
}

// --- audit ---

// Audit returns only the execution-environment audit summary. Assessment
// findings are stored and counted independently in findings.json.
func (a *App) Audit(mission string) (Posture, error) {
	p, err := a.eng.Audit(mission)
	if err == nil {
		if ms, listErr := a.store.ListMissions(a.ws); listErr == nil {
			for _, m := range ms {
				if m.ID == mission {
					m.EnvironmentAudit = p
					m.Posture = Posture{}
					_ = a.store.SaveMission(a.ws, m)
					break
				}
			}
		}
		return p, nil
	}
	if errors.Is(err, ErrNotWired) {
		ms, _ := a.store.ListMissions(a.ws)
		for _, m := range ms {
			if m.ID == mission {
				return m.EnvironmentAudit, nil
			}
		}
		return Posture{}, nil
	}
	return Posture{}, err
}

func (a *App) AuditDetailed(mission string) (AuditResult, error) {
	if !validWorkspaceName(mission) {
		return AuditResult{}, errors.New("invalid mission")
	}
	emit := func(percent int, stage string) {
		wruntime.EventsEmit(a.ctx, "rfswift:audit-progress", map[string]any{"mission": mission, "percent": percent, "stage": stage})
	}
	emit(5, "Preparing audit")
	local, localOK := a.eng.(*LocalEngine)
	remoteEngine, remoteOK := a.eng.(*RemoteEngine)
	if !localOK && !remoteOK {
		return AuditResult{}, errors.New("detailed audit is unavailable for this engine")
	}
	emit(20, "Scanning packages and configuration")
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		percent := 20
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if percent < 85 {
					percent += 5
					emit(percent, "Scanning packages and configuration")
				}
			}
		}
	}()
	var result AuditResult
	var err error
	if localOK {
		result, err = local.AuditDetailed(mission)
	} else {
		result, err = remoteEngine.AuditDetailed(mission)
	}
	close(done)
	if err != nil {
		emit(100, "Audit failed")
		return AuditResult{}, err
	}
	emit(90, "Normalizing environment vulnerabilities")
	dir := filepath.Join(a.store.missionDir(a.ws, mission), "environment-audits")
	if err := os.MkdirAll(dir, 0o700); err == nil && len(result.Raw) > 0 {
		_ = writePrivateFile(filepath.Join(dir, "latest.json"), result.Raw)
	}
	if missions, loadErr := a.store.ListMissions(a.ws); loadErr == nil {
		for _, m := range missions {
			if m.ID == mission {
				m.EnvironmentAudit = result.Posture
				m.Posture = Posture{}
				_ = a.store.SaveMission(a.ws, m)
				break
			}
		}
	}
	emit(100, "Audit complete")
	return result, nil
}

// --- report + pwndoc ---

func (a *App) gather(mission string) (Mission, []Finding, string, []Capture) {
	m, err := a.eng.Inspect(mission)
	if err != nil || m.ID == "" {
		m = Mission{ID: mission}
	}
	fs, _ := a.store.LoadFindings(a.ws, mission)
	note, _ := a.store.GetNote(a.ws, mission, "note.md")
	caps, _ := a.store.ListCaptures(a.ws, mission)
	return m, fs, note, caps
}

// ReportMarkdown builds the per-mission markdown report (branded).
func (a *App) ReportMarkdown(mission string) (string, error) {
	m, fs, note, caps := a.gather(mission)
	return BuildReportMarkdown(m, fs, note, caps), nil
}

// SaveReportToWorkspace writes report-<mission>.md into the mission's reports/
// and returns the path.
func (a *App) SaveReportToWorkspace(mission string) (string, error) {
	md, _ := a.ReportMarkdown(mission)
	return a.store.SaveReport(a.ws, mission, "report-"+mission+".md", []byte(md))
}

// SaveFileDialog offers content to the user through the native save dialog and
// writes it to the chosen path (returns "" if the user cancels).
func (a *App) SaveFileDialog(defaultName, content string) (string, error) {
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{DefaultFilename: defaultName})
	if err != nil || path == "" {
		return "", err
	}
	if err := writePrivateFile(path, []byte(content)); err != nil {
		return "", err
	}
	return path, nil
}

// ExportPwndoc returns the mission's findings as pwndoc-compatible JSON.
func (a *App) ExportPwndoc(mission string) (string, error) {
	m, fs, _, _ := a.gather(mission)
	b, err := BuildPwndocJSON(m, fs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ImportPwndoc parses a pwndoc export and appends its findings to the mission.
func (a *App) ImportPwndoc(mission, data string) (int, error) {
	imported, err := ParsePwndoc([]byte(data), mission)
	if err != nil {
		return 0, err
	}
	fs, _ := a.store.LoadFindings(a.ws, mission)
	for _, f := range imported {
		f.ID = "F-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		fs = append(fs, f)
	}
	if err := a.store.SaveFindings(a.ws, mission, fs); err != nil {
		return 0, err
	}
	return len(imported), nil
}

// --- optional external coding-agent / MCP bridge ---

func (a *App) AgentClients() []AgentClient    { return AgentClients() }
func (a *App) GetAgentCfg() AgentCfg          { return a.store.LoadAgentCfg() }
func (a *App) SetAgentCfg(cfg AgentCfg) error { return a.store.SaveAgentCfg(cfg) }
func (a *App) MCPCommand(mission string) (string, error) {
	if !validWorkspaceName(mission) {
		return "", errors.New("invalid mission scope")
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	cfg := a.store.LoadAgentCfg()
	if !cfg.Enabled {
		return "", errors.New("enable the mission-scoped MCP bridge first")
	}
	args := []string{"--mcp", "--workspace", a.ws, "--mission", mission}
	if cfg.AllowWrite {
		args = append(args, "--mcp-write")
	}
	if cfg.AllowExec {
		args = append(args, "--mcp-exec")
	}
	parts := []string{shellQuote(executable)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " "), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// --- connections + connection security audit ---

// Connections lists the agents the Workbench can attach to. The scaffold ships a
// local connection and one sample remote; real ones will come from config.
func (a *App) Connections() []Connection {
	return []Connection{
		{ID: "local", Name: "This machine", Host: "in-process", Kind: "local", Auth: []string{"local OS user"}, CertDays: -1, Bind: "loopback", RateLimit: true, Version: "up-to-date"},
	}
}

// SelectConnection is explicit even while only the in-process engine exists,
// so a future remote Engine can implement the same binding without UI changes.
func (a *App) SelectConnection(id string) error {
	if id != "local" {
		return fmt.Errorf("connect with the agent credentials before selecting %q", id)
	}
	a.eng = NewLocalEngine()
	return nil
}

// AuditConn runs the connection security audit for a connection id, including
// the optional MCP bridge permissions.
func (a *App) AuditConn(id string) (ConnAudit, error) {
	var conn Connection
	for _, c := range a.Connections() {
		if c.ID == id {
			conn = c
		}
	}
	if conn.ID == "" {
		return ConnAudit{}, errors.New("unknown connection")
	}
	res := AuditConnection(conn)
	res.Checks = append(res.Checks, agentPosture(a.store.LoadAgentCfg())...)
	res.Posture = worstOf(res.Checks)
	return res, nil
}
