package workbench

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
	rfutils "penthertz/rfswift/rfutils"
)

// This file backs the Engine doctor's management actions: nix store GC, per-
// engine space reclaim (images / build cache / volumes / unused networks), and
// Lima VM lifecycle + sizing. All are local-only; the GUI hides them for remote
// connections.

func (a *App) requireLocal() (*LocalEngine, error) {
	local, ok := a.engine().(*LocalEngine)
	if !ok {
		return nil, fmt.Errorf("engine management is only available for the local connection")
	}
	return local, nil
}

// --- Nix ---

// NixGarbageCollect runs `nix store gc` where the engine is: on this machine
// (or inside the WSL 2 distribution on Windows), or on the agent host while a
// remote connection is active. Returns nix's own "N store paths deleted, M
// freed" summary.
func (a *App) NixGarbageCollect() (string, error) {
	if remoteEngine, ok := a.engine().(*RemoteEngine); ok {
		return remoteEngine.NixGC()
	}
	if _, err := a.requireLocal(); err != nil {
		return "", err
	}
	out, err := rfnix.StoreGC()
	if err != nil {
		return "", err
	}
	if note := wslDiskNote(); note != "" {
		out += "\n" + note
	}
	return out, nil
}

// --- Container engines: reclaim space ---

// PruneSummary reports what a reclaim pass freed.
type PruneSummary = rfdock.PruneSummary

// PruneEngine reclaims space on one container engine, on this machine or on
// the agent host while a remote connection is active (rfdock.PruneEngine
// documents the targets).
func (a *App) PruneEngine(name string, images, unusedImages, buildCache, volumes, networks bool) (PruneSummary, error) {
	opts := rfdock.PruneOptions{Images: images, UnusedImages: unusedImages, BuildCache: buildCache, Volumes: volumes, Networks: networks}
	if remoteEngine, ok := a.engine().(*RemoteEngine); ok {
		return remoteEngine.Prune(name, opts)
	}
	if _, err := a.requireLocal(); err != nil {
		return PruneSummary{}, err
	}
	eng := engineByType(rfdock.EngineType(strings.ToLower(strings.TrimSpace(name))))
	if eng == nil {
		return PruneSummary{}, fmt.Errorf("unknown engine %q", name)
	}
	resetEngineEnv()
	return rfdock.PruneEngine(eng, opts)
}

// --- Lima VM lifecycle & sizing ---

func (a *App) limaManageable() error {
	if _, err := a.requireLocal(); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" || !rfutils.IsLimaInstalled() {
		return fmt.Errorf("Lima VM management needs macOS with Lima installed")
	}
	return nil
}

// LimaResetVM deletes the Lima instance and recreates it from the template — a
// clean VM. Everything inside the old VM (containers, images) is destroyed;
// bind-mounted host workspaces are not.
func (a *App) LimaResetVM() (string, error) {
	if err := a.limaManageable(); err != nil {
		return "", err
	}
	lima := &rfdock.LimaEngine{}
	tmpl := lima.FindTemplate()
	if tmpl == "" {
		return "", fmt.Errorf("no Lima template found (expected ~/.config/rfswift/lima.yaml or a bundled rfswift.yaml)")
	}
	if err := lima.ResetInstance(tmpl); err != nil {
		return "", err
	}
	return "Lima VM recreated from " + tmpl, nil
}

// LimaSpecs is the VM's top-level sizing from its config.
type LimaSpecs struct {
	CPUs         int    `json:"cpus"`
	Memory       string `json:"memory"`
	Disk         string `json:"disk"`
	VMType       string `json:"vmType"`
	Source       string `json:"source"` // the file the values were read from
	TemplatePath string `json:"templatePath"`
}

// GetLimaSpecs reads the VM's current sizing — from the live instance config if
// it exists, else the user template, else the bundled template.
func (a *App) GetLimaSpecs() (LimaSpecs, error) {
	var s LimaSpecs
	if err := a.limaManageable(); err != nil {
		return s, err
	}
	lima := &rfdock.LimaEngine{}
	s.TemplatePath = lima.UserTemplatePath()
	candidates := []string{
		rfutils.GetLimaInstanceConfigPath(limaInstanceName()),
		lima.UserTemplatePath(),
		lima.FindTemplate(),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s.Source = p
		for _, line := range strings.Split(string(data), "\n") {
			switch {
			case strings.HasPrefix(line, "cpus:"):
				s.CPUs, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "cpus:")))
			case strings.HasPrefix(line, "memory:"):
				s.Memory = unquoteYAML(strings.TrimPrefix(line, "memory:"))
			case strings.HasPrefix(line, "disk:"):
				s.Disk = unquoteYAML(strings.TrimPrefix(line, "disk:"))
			case strings.HasPrefix(line, "vmType:"):
				s.VMType = unquoteYAML(strings.TrimPrefix(line, "vmType:"))
			}
		}
		return s, nil
	}
	return s, fmt.Errorf("no Lima config or template found to read")
}

// SetLimaSpecs writes CPU/memory/disk/vmType into the user template (so the
// change persists and takes precedence over the bundled one) and applies it.
// disk and vmType changes cannot be applied in place, so they force a
// destructive recreate; recreate is also used when the caller sets it.
func (a *App) SetLimaSpecs(cpus int, memory, disk, vmType string, recreate bool) (string, error) {
	if err := a.limaManageable(); err != nil {
		return "", err
	}
	memory, disk, vmType = strings.TrimSpace(memory), strings.TrimSpace(disk), strings.TrimSpace(vmType)
	if memory != "" && !rfutils.IsValidLimaSize(memory) {
		return "", fmt.Errorf("invalid memory size %q (use e.g. 8GiB)", memory)
	}
	if disk != "" && !rfutils.IsValidLimaSize(disk) {
		return "", fmt.Errorf("invalid disk size %q (use e.g. 100GiB)", disk)
	}
	if vmType != "" && vmType != "qemu" && vmType != "vz" {
		return "", fmt.Errorf("vmType must be qemu or vz (got %q)", vmType)
	}
	lima := &rfdock.LimaEngine{}
	tmpl := lima.UserTemplatePath()
	// Seed the user template from the effective template on first edit so we
	// carry over provisioning/mounts, not just the few sizing keys.
	if _, err := os.Stat(tmpl); err != nil {
		src := lima.FindTemplate()
		if src == "" {
			return "", fmt.Errorf("no base Lima template to seed settings from")
		}
		if err := os.MkdirAll(filepath.Dir(tmpl), 0o755); err != nil {
			return "", err
		}
		if err := rfutils.CopyFile(src, tmpl); err != nil {
			return "", err
		}
	}
	changes, err := rfutils.SetLimaResources(tmpl, cpus, memory, disk)
	if err != nil {
		return "", err
	}
	if vmType != "" {
		if vmChange, err := setTemplateVMType(tmpl, vmType); err != nil {
			return "", err
		} else if vmChange != "" {
			changes = append(changes, vmChange)
		}
	}
	if len(changes) == 0 {
		return "No changes to apply.", nil
	}
	// disk/vmType require recreate; cpus/memory can be applied in place.
	force := recreate || disk != "" || vmType != ""
	if err := lima.ReconfigureInstance(tmpl, force); err != nil {
		return "", err
	}
	verb := "applied in place"
	if force {
		verb = "applied (VM recreated)"
	}
	return "Settings " + verb + ": " + strings.Join(changes, ", "), nil
}

// setTemplateVMType rewrites the top-level vmType key in a Lima YAML template.
func setTemplateVMType(path, vmType string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	set := false
	for i, line := range lines {
		if strings.HasPrefix(line, "vmType:") {
			if unquoteYAML(strings.TrimPrefix(line, "vmType:")) == vmType {
				return "", nil // already this value
			}
			lines[i] = "vmType: " + vmType
			set = true
			break
		}
	}
	if !set {
		lines = append(lines, "vmType: "+vmType)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", err
	}
	return "vmType -> " + vmType, nil
}

// --- helpers ---

func unquoteYAML(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return strings.TrimSpace(s)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
