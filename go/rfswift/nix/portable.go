/*
*  This code is part of RF Swift by @Penthertz
*  Author(s): Sebastien Dudek (@FlUxIuS)
*
*  Portable environments: export a realised Nix environment (its whole closure)
*  plus its workspace into a single compressed archive, and import it on another
*  machine. Uses `nix copy` to a file:// binary cache for the closure (robust for
*  large closures, no ARG_MAX issues) wrapped, with the workspace and a manifest,
*  in one gzipped tar.
 */
package nix

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	common "penthertz/rfswift/common"
)

const (
	maxPortableManifestSize = 1 << 20
	maxPortableArchiveFiles = 1_000_000
)

// PortableEnvironmentName reads only the manifest from an environment archive.
// It lets GUI callers associate companion metadata with the imported mission
// without having to guess from a user-renamed filename.
func PortableEnvironmentName(inFile string) (string, error) {
	f, err := os.Open(inFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("invalid portable environment compression: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := 0
	for {
		h, err := tr.Next()
		if err != nil {
			return "", fmt.Errorf("portable environment manifest not found: %w", err)
		}
		entries++
		if entries > maxPortableArchiveFiles {
			return "", errors.New("portable environment archive contains too many entries")
		}
		if filepath.Clean(h.Name) != "manifest.json" {
			continue
		}
		if h.Size < 0 || h.Size > maxPortableManifestSize {
			return "", errors.New("portable environment manifest is too large")
		}
		var m exportManifest
		if err := json.NewDecoder(tr).Decode(&m); err != nil {
			return "", fmt.Errorf("corrupt portable environment manifest: %w", err)
		}
		if err := ValidateEnvironmentName(m.Env.Name); err != nil {
			return "", fmt.Errorf("portable environment manifest has invalid environment name: %w", err)
		}
		return m.Env.Name, nil
	}
}

// exportManifest is the metadata stored inside a .rfenv archive so import can
// re-register the environment and pin the right closure.
type exportManifest struct {
	Version      int         `json:"version"`
	StorePath    string      `json:"storePath"`
	ExtrasPath   string      `json:"extrasPath,omitempty"`
	HasWorkspace bool        `json:"hasWorkspace"`
	Env          Environment `json:"env"`
}

type ExportProgress func(percent int, stage string)

// envStorePath realises the environment if needed and returns the /nix/store
// path of its buildEnv closure.
func envStorePath(env *Environment) (string, error) {
	// Already realised (eager env with a gcroot): resolve the symlink.
	if env.ProfilePath != "" && pathExists(env.ProfilePath) {
		if p, err := filepath.EvalSymlinks(env.ProfilePath); err == nil {
			return p, nil
		}
	}
	// Otherwise realise packages.<image> to a temporary out-link and resolve it.
	tmp, err := os.MkdirTemp("", "rfswift-realise-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	link := filepath.Join(tmp, "result")
	if err := buildProfile(env.FlakeRef, env.Image, link); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(link)
}

// ExportEnvironment writes the environment's closure + workspace to outFile
// (default <name>.rfenv) as a single gzipped tar.
func ExportEnvironment(name, outFile string) error {
	return ExportEnvironmentWithProgress(name, outFile, nil)
}

// ExportEnvironmentWithProgress is ExportEnvironment with coarse, truthful
// phase updates. Nix does not expose a stable byte total for a store closure,
// so progress represents completed phases rather than fabricated byte ratios.
func ExportEnvironmentWithProgress(name, outFile string, progress ExportProgress) error {
	report := func(percent int, stage string) {
		if progress != nil {
			progress(percent, stage)
		}
	}
	report(3, "Checking Nix environment")
	if useWSL() {
		return wslExportEnvironment(name, outFile, progress)
	}
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH")
	}
	env, err := GetEnvironment(name)
	if err != nil {
		return err
	}

	report(10, "Realising environment closure")
	common.PrintInfoMessage(fmt.Sprintf("Realising environment '%s' for export...", name))
	storePath, err := envStorePath(env)
	if err != nil {
		return fmt.Errorf("could not realise environment '%s' for export: %w", name, err)
	}
	report(35, "Environment closure ready")
	extrasPath := ""
	if profile := EnvExtrasProfile(name); pathExists(profile) {
		if resolved, resolveErr := filepath.EvalSymlinks(profile); resolveErr == nil {
			extrasPath = resolved
		}
	}

	staging, err := os.MkdirTemp("", "rfswift-export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	storeDir := filepath.Join(staging, "store")
	wsDir := filepath.Join(staging, "workspace")

	// 1. Copy the closure into a file:// binary cache. No inner compression - the
	//    outer tar.gz compresses everything uniformly.
	common.PrintInfoMessage("Exporting the Nix closure...")
	report(40, "Exporting Nix store closure")
	cacheURL := "file://" + storeDir + "?compression=none"
	copyPaths := []string{storePath}
	if extrasPath != "" {
		copyPaths = append(copyPaths, extrasPath)
	}
	args := append(experimentalArgs(), "copy", "--no-check-sigs", "--to", cacheURL)
	args = append(args, copyPaths...)
	cp := nixCommand(args...)
	cp.Stdout, cp.Stderr = os.Stderr, os.Stderr
	if err := cp.Run(); err != nil {
		return fmt.Errorf("nix copy (export) failed: %w", err)
	}
	report(65, "Nix store closure exported")

	// 2. Copy the workspace, if the environment has one.
	hasWorkspace := false
	if env.Workspace != "" && env.Workspace != "none" && pathExists(env.Workspace) {
		report(68, "Copying environment workspace")
		if err := ensureDir(wsDir); err != nil {
			return err
		}
		c := exec.Command("cp", "-a", env.Workspace+"/.", wsDir+"/")
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("failed to copy workspace: %w", err)
		}
		hasWorkspace = true
	}
	report(76, "Writing environment manifest")

	// 3. Manifest.
	m := exportManifest{Version: 2, StorePath: storePath, ExtrasPath: extrasPath, HasWorkspace: hasWorkspace, Env: *env}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), data, 0o644); err != nil {
		return err
	}

	// 4. Pack it all into one gzipped tar.
	if outFile == "" {
		outFile = name + ".rfenv"
	}
	if abs, err := filepath.Abs(outFile); err == nil {
		outFile = abs
	}
	common.PrintInfoMessage("Compressing...")
	report(82, "Compressing portable environment")
	t := exec.Command("tar", "-czf", outFile, "-C", staging, ".")
	t.Stderr = os.Stderr
	if err := t.Run(); err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}
	report(100, "Environment export complete")

	wsNote := ""
	if hasWorkspace {
		wsNote = " (with workspace)"
	}
	common.PrintSuccessMessage(fmt.Sprintf("Exported environment '%s'%s to %s", name, wsNote, outFile))
	return nil
}

// ImportEnvironment restores an environment from a .rfenv archive: imports its
// closure into the local store, restores its workspace, and registers it under
// newName (or the archived name) so it can be entered directly.
func ImportEnvironment(inFile, newName, newWorkspace string) error {
	return ImportEnvironmentWithProgress(inFile, newName, newWorkspace, nil)
}

type ImportProgress func(percent int, stage string)

func ImportEnvironmentWithProgress(inFile, newName, newWorkspace string, progress ImportProgress) error {
	report := func(percent int, stage string) {
		if progress != nil {
			progress(percent, stage)
		}
	}
	report(3, "Checking portable environment")
	if useWSL() {
		return wslImportEnvironment(inFile, newName, newWorkspace, progress)
	}
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH")
	}
	if !pathExists(inFile) {
		return fmt.Errorf("archive not found: %s", inFile)
	}

	staging, err := os.MkdirTemp("", "rfswift-import-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	common.PrintInfoMessage("Extracting archive...")
	report(10, "Extracting portable environment")
	if err := extractPortableArchive(inFile, staging); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(staging, "manifest.json"))
	if err != nil {
		return fmt.Errorf("archive is missing manifest.json (not an rfswift environment export?): %w", err)
	}
	var m exportManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("corrupt manifest in archive: %w", err)
	}
	report(25, "Environment manifest validated")

	name := newName
	if name == "" {
		name = m.Env.Name
	}
	if name == "" {
		return fmt.Errorf("could not determine environment name from the archive; pass --name")
	}
	if err := ValidateEnvironmentName(name); err != nil {
		return err
	}
	if m.Version < 1 || m.Version > 2 {
		return fmt.Errorf("unsupported portable environment format version %d", m.Version)
	}
	if err := validateImportedStorePath(m.StorePath); err != nil {
		return fmt.Errorf("invalid imported store path: %w", err)
	}
	if m.ExtrasPath != "" {
		if err := validateImportedStorePath(m.ExtrasPath); err != nil {
			return fmt.Errorf("invalid imported extras path: %w", err)
		}
	}
	if pathExists(EnvDir(name)) {
		return fmt.Errorf("environment '%s' already exists; pass --name <other> or remove it first (rfswift nix remove %s)", name, name)
	}

	// 1. Import the closure into the local store.
	common.PrintInfoMessage("Importing the Nix closure into the store...")
	report(30, "Importing Nix store closure")
	cacheURL := "file://" + filepath.Join(staging, "store") + "?compression=none"
	copyPaths := []string{m.StorePath}
	if m.ExtrasPath != "" {
		copyPaths = append(copyPaths, m.ExtrasPath)
	}
	args := append(experimentalArgs(), "copy", "--no-check-sigs", "--from", cacheURL)
	args = append(args, copyPaths...)
	cp := nixCommand(args...)
	cp.Stdout, cp.Stderr = os.Stderr, os.Stderr
	if err := cp.Run(); err != nil {
		return fmt.Errorf("nix copy (import) failed: %w", err)
	}
	report(60, "Nix store closure imported")

	// 2. Pin the imported closure with a gcroot profile symlink so it survives GC.
	if err := ensureDir(EnvDir(name)); err != nil {
		return err
	}
	link := profileLink(name)
	bargs := append(experimentalArgs(), "build", m.StorePath, "--out-link", link)
	b := nixCommand(bargs...)
	b.Stderr = os.Stderr
	if err := b.Run(); err != nil {
		return fmt.Errorf("failed to pin the imported closure: %w", err)
	}
	report(72, "Environment closure pinned")
	if m.ExtrasPath != "" {
		report(74, "Restoring installed environment tools")
		extras := EnvExtrasProfile(name)
		eargs := append(experimentalArgs(), "build", m.ExtrasPath, "--out-link", extras)
		e := nixCommand(eargs...)
		e.Stderr = os.Stderr
		if err := e.Run(); err != nil {
			return fmt.Errorf("failed to restore installed environment tools: %w", err)
		}
	}

	// 3. Restore the workspace.
	ws := ""
	if m.HasWorkspace {
		report(76, "Restoring environment workspace")
		ws = newWorkspace
		if ws == "" {
			ws = filepath.Join(homeDir(), "rfswift-workspace", name)
		}
		if abs, err := filepath.Abs(ws); err == nil {
			ws = abs
		}
		if err := ensureDir(ws); err != nil {
			return err
		}
		c := exec.Command("cp", "-a", filepath.Join(staging, "workspace")+"/.", ws+"/")
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("failed to restore workspace: %w", err)
		}
	}
	report(90, "Registering environment")

	// 4. Register the environment (now realised, non-lazy).
	env := m.Env
	env.Name = name
	env.ProfilePath = link
	env.Workspace = ws
	env.Lazy = false
	env.Commands = nil
	if err := writeManifest(&env); err != nil {
		return err
	}
	report(100, "Environment import complete")

	wsNote := ""
	if ws != "" {
		wsNote = fmt.Sprintf(" (workspace restored to %s)", ws)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Imported environment '%s'%s. Enter it with: rfswift exec %s", name, wsNote, name))
	return nil
}

type portableLink struct {
	path, target string
	typeflag     byte
}

// extractPortableArchive extracts a user-supplied .rfenv without allowing tar
// traversal, device nodes, or an early symlink to redirect later file writes.
func extractPortableArchive(source, destination string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	root, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	withinRoot := func(name string) (string, error) {
		clean := filepath.Clean(filepath.FromSlash(name))
		for clean == "." || strings.HasPrefix(clean, "."+string(filepath.Separator)) {
			if clean == "." {
				return root, nil
			}
			clean = strings.TrimPrefix(clean, "."+string(filepath.Separator))
		}
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe archive path %q", name)
		}
		path := filepath.Join(root, clean)
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe archive path %q", name)
		}
		return path, nil
	}
	tr := tar.NewReader(gz)
	links := make([]portableLink, 0)
	entries := 0
	for {
		h, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nextErr
		}
		entries++
		if entries > maxPortableArchiveFiles {
			return errors.New("portable environment archive contains too many entries")
		}
		if filepath.Clean(h.Name) == "manifest.json" && (h.Size < 0 || h.Size > maxPortableManifestSize) {
			return errors.New("portable environment manifest is too large")
		}
		path, pathErr := withinRoot(h.Name)
		if pathErr != nil {
			return pathErr
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}
			out, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if openErr != nil {
				return openErr
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink, tar.TypeLink:
			links = append(links, portableLink{path: path, target: h.Linkname, typeflag: h.Typeflag})
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue
		default:
			return fmt.Errorf("unsupported archive entry type %d for %q", h.Typeflag, h.Name)
		}
	}
	// Create links only after regular files, so no archive entry can write
	// through a link planted by an earlier header.
	for _, link := range links {
		if err := os.MkdirAll(filepath.Dir(link.path), 0o700); err != nil {
			return err
		}
		if link.typeflag == tar.TypeLink {
			target, targetErr := withinRoot(link.target)
			if targetErr != nil {
				return targetErr
			}
			if err := os.Link(target, link.path); err != nil {
				return err
			}
			continue
		}
		if filepath.IsAbs(link.target) {
			return fmt.Errorf("absolute symlink target is not allowed: %q", link.target)
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(link.path), filepath.FromSlash(link.target)))
		rel, relErr := filepath.Rel(root, resolved)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("symlink escapes archive root: %q", link.target)
		}
		if err := os.Symlink(link.target, link.path); err != nil {
			return err
		}
	}
	return nil
}

func validateImportedStorePath(path string) error {
	clean := filepath.Clean(path)
	if clean != path || !strings.HasPrefix(clean, "/nix/store/") || strings.Contains(strings.TrimPrefix(clean, "/nix/store/"), string(filepath.Separator)) {
		return fmt.Errorf("expected a direct /nix/store path, got %q", path)
	}
	base := strings.TrimPrefix(clean, "/nix/store/")
	if len(base) < 34 || base[32] != '-' {
		return fmt.Errorf("malformed Nix store path %q", path)
	}
	for _, c := range base[:32] {
		if !strings.ContainsRune("0123456789abcdfghijklmnpqrsvwxyz", c) {
			return fmt.Errorf("malformed Nix store hash in %q", path)
		}
	}
	return nil
}
