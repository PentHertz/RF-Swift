package workbench

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/moby/moby/client"

	rfdock "penthertz/rfswift/dock"
	rfnix "penthertz/rfswift/nix"
	rfutils "penthertz/rfswift/rfutils"
)

// This file backs the Engine doctor's management actions: nix store GC, per-
// engine space reclaim (images / build cache / volumes / unused networks), and
// Lima VM lifecycle + sizing. All are local-only; the GUI hides them for remote
// connections.

func (a *App) requireLocal() (*LocalEngine, error) {
	local, ok := a.eng.(*LocalEngine)
	if !ok {
		return nil, fmt.Errorf("engine management is only available for the local connection")
	}
	return local, nil
}

// --- Nix ---

// NixGarbageCollect runs `nix store gc`, freeing store paths not reachable from
// a gcroot (RF Swift environments keep a gcroot, so they survive), and returns
// nix's own "N store paths deleted, M freed" summary.
func (a *App) NixGarbageCollect() (string, error) {
	if _, err := a.requireLocal(); err != nil {
		return "", err
	}
	if !rfnix.IsAvailable() {
		return "", fmt.Errorf("nix is not installed or not on PATH")
	}
	// NixCommand runs nix where the engine is: locally, or inside the WSL 2
	// distribution on Windows.
	cmd := rfnix.NixCommand("--extra-experimental-features", "nix-command", "store", "gc")
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if err != nil {
		if out != "" {
			return "", fmt.Errorf("%s", lastLine(out))
		}
		return "", err
	}
	if out == "" {
		out = "Nothing to collect; the store is already minimal."
	}
	if note := wslDiskNote(); note != "" {
		out += "\n" + note
	}
	return out, nil
}

// --- Container engines: reclaim space ---

// PruneSummary reports what a reclaim pass freed.
type PruneSummary struct {
	Reclaimed    uint64 `json:"reclaimed"` // bytes
	ReclaimedStr string `json:"reclaimedStr"`
	Images       int    `json:"images"`
	CacheEntries int    `json:"cacheEntries"`
	Volumes      int    `json:"volumes"`
	Networks     int    `json:"networks"`
	Detail       string `json:"detail"`
}

// PruneEngine reclaims space on one container engine (docker/podman, or the
// Docker daemon inside the Lima VM). `images` prunes dangling-only (untagged
// layers — always safe); `unusedImages` prunes every image no container
// references (reclaims tagged images left by deleted containers, so they must be
// pulled again later). Build cache is pruned fully, volumes and networks only
// when unused. Each target is best-effort — a daemon that does not support one
// (e.g. Podman has no build-cache endpoint) does not fail the whole pass.
func (a *App) PruneEngine(name string, images, unusedImages, buildCache, volumes, networks bool) (PruneSummary, error) {
	var s PruneSummary
	if _, err := a.requireLocal(); err != nil {
		return s, err
	}
	eng := engineByType(rfdock.EngineType(strings.ToLower(strings.TrimSpace(name))))
	if eng == nil {
		return s, fmt.Errorf("unknown engine %q", name)
	}
	resetEngineEnv()
	if !eng.IsServiceRunning() {
		return s, fmt.Errorf("%s is not running", name)
	}
	cli, err := eng.GetClient()
	if err != nil {
		return s, err
	}
	defer cli.Close()
	ctx := context.Background()
	var notes []string

	if unusedImages || images {
		opts := client.ImagePruneOptions{}
		if unusedImages {
			// dangling=false widens the prune to every image not used by a
			// container (includes the dangling set), reclaiming tagged images
			// left behind by deleted containers.
			f := make(client.Filters)
			f.Add("dangling", "false")
			opts.Filters = f
		}
		if r, e := cli.ImagePrune(ctx, opts); e == nil {
			s.Reclaimed += r.Report.SpaceReclaimed
			s.Images += len(r.Report.ImagesDeleted)
		} else {
			notes = append(notes, "images: "+cleanPruneErr(e))
		}
	}
	if buildCache {
		if r, e := cli.BuildCachePrune(ctx, client.BuildCachePruneOptions{All: true}); e == nil {
			s.Reclaimed += r.Report.SpaceReclaimed
			s.CacheEntries += len(r.Report.CachesDeleted)
		} else {
			notes = append(notes, "build cache: "+cleanPruneErr(e))
		}
	}
	if volumes {
		if r, e := cli.VolumePrune(ctx, client.VolumePruneOptions{}); e == nil {
			s.Reclaimed += r.Report.SpaceReclaimed
			s.Volumes += len(r.Report.VolumesDeleted)
		} else {
			notes = append(notes, "volumes: "+cleanPruneErr(e))
		}
	}
	if networks {
		if r, e := cli.NetworkPrune(ctx, client.NetworkPruneOptions{}); e == nil {
			s.Networks += len(r.Report.NetworksDeleted)
		} else {
			notes = append(notes, "networks: "+cleanPruneErr(e))
		}
	}
	s.ReclaimedStr = humanBytes(s.Reclaimed)
	parts := []string{fmt.Sprintf("reclaimed %s", s.ReclaimedStr)}
	if s.Images > 0 {
		parts = append(parts, fmt.Sprintf("%d image(s)", s.Images))
	}
	if s.CacheEntries > 0 {
		parts = append(parts, fmt.Sprintf("%d cache entr(ies)", s.CacheEntries))
	}
	if s.Volumes > 0 {
		parts = append(parts, fmt.Sprintf("%d volume(s)", s.Volumes))
	}
	if s.Networks > 0 {
		parts = append(parts, fmt.Sprintf("%d network(s)", s.Networks))
	}
	s.Detail = strings.Join(parts, ", ")
	if len(notes) > 0 {
		s.Detail += " (skipped: " + strings.Join(notes, "; ") + ")"
	}
	return s, nil
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

// cleanPruneErr turns "not supported" daemon errors (e.g. Podman has no
// build-cache prune endpoint → "Not Found") into a readable note.
func cleanPruneErr(e error) string {
	msg := e.Error()
	low := strings.ToLower(msg)
	if strings.Contains(low, "not found") || strings.Contains(low, "not implemented") || strings.Contains(low, "404") {
		return "not supported by this engine"
	}
	return msg
}

func unquoteYAML(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return strings.TrimSpace(s)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
