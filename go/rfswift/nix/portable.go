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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	common "penthertz/rfswift/common"
)

// exportManifest is the metadata stored inside a .rfenv archive so import can
// re-register the environment and pin the right closure.
type exportManifest struct {
	Version      int         `json:"version"`
	StorePath    string      `json:"storePath"`
	HasWorkspace bool        `json:"hasWorkspace"`
	Env          Environment `json:"env"`
}

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
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH")
	}
	env, err := GetEnvironment(name)
	if err != nil {
		return err
	}

	common.PrintInfoMessage(fmt.Sprintf("Realising environment '%s' for export...", name))
	storePath, err := envStorePath(env)
	if err != nil {
		return fmt.Errorf("could not realise environment '%s' for export: %w", name, err)
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
	cacheURL := "file://" + storeDir + "?compression=none"
	args := append(experimentalArgs(), "copy", "--no-check-sigs", "--to", cacheURL, storePath)
	cp := exec.Command(NixBinary(), args...)
	cp.Stdout, cp.Stderr = os.Stderr, os.Stderr
	if err := cp.Run(); err != nil {
		return fmt.Errorf("nix copy (export) failed: %w", err)
	}

	// 2. Copy the workspace, if the environment has one.
	hasWorkspace := false
	if env.Workspace != "" && env.Workspace != "none" && pathExists(env.Workspace) {
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

	// 3. Manifest.
	m := exportManifest{Version: 1, StorePath: storePath, HasWorkspace: hasWorkspace, Env: *env}
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
	t := exec.Command("tar", "-czf", outFile, "-C", staging, ".")
	t.Stderr = os.Stderr
	if err := t.Run(); err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

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
	x := exec.Command("tar", "-xzf", inFile, "-C", staging)
	x.Stderr = os.Stderr
	if err := x.Run(); err != nil {
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

	name := newName
	if name == "" {
		name = m.Env.Name
	}
	if name == "" {
		return fmt.Errorf("could not determine environment name from the archive; pass --name")
	}
	if pathExists(EnvDir(name)) {
		return fmt.Errorf("environment '%s' already exists; pass --name <other> or remove it first (rfswift nix remove %s)", name, name)
	}

	// 1. Import the closure into the local store.
	common.PrintInfoMessage("Importing the Nix closure into the store...")
	cacheURL := "file://" + filepath.Join(staging, "store") + "?compression=none"
	args := append(experimentalArgs(), "copy", "--no-check-sigs", "--from", cacheURL, m.StorePath)
	cp := exec.Command(NixBinary(), args...)
	cp.Stdout, cp.Stderr = os.Stderr, os.Stderr
	if err := cp.Run(); err != nil {
		return fmt.Errorf("nix copy (import) failed: %w", err)
	}

	// 2. Pin the imported closure with a gcroot profile symlink so it survives GC.
	if err := ensureDir(EnvDir(name)); err != nil {
		return err
	}
	link := profileLink(name)
	bargs := append(experimentalArgs(), "build", m.StorePath, "--out-link", link)
	b := exec.Command(NixBinary(), bargs...)
	b.Stderr = os.Stderr
	if err := b.Run(); err != nil {
		return fmt.Errorf("failed to pin the imported closure: %w", err)
	}

	// 3. Restore the workspace.
	ws := ""
	if m.HasWorkspace {
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

	wsNote := ""
	if ws != "" {
		wsNote = fmt.Sprintf(" (workspace restored to %s)", ws)
	}
	common.PrintSuccessMessage(fmt.Sprintf("Imported environment '%s'%s. Enter it with: rfswift exec %s", name, wsNote, name))
	return nil
}
