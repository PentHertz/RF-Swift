package nix

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	common "penthertz/rfswift/common"
)

type UpdateOptions struct {
	Check bool
	Input string
}

// localFlakePath resolves a writable filesystem flake reference. Updating a
// remote reference would not have a lock file RF Swift can persist.
func localFlakePath(ref string) (string, bool) {
	ref = strings.TrimSpace(strings.TrimPrefix(ref, "path:"))
	if ref == "" || looksLikeFlakeURL(ref) {
		return "", false
	}
	abs, err := filepath.Abs(ref)
	if err != nil || !hasFlake(abs) {
		return "", false
	}
	return abs, true
}

// EnvironmentFlakeInputs returns selectable top-level input names from the
// environment's actual lock graph. It supports writable local flakes and remote
// flakes through `nix flake metadata --json`.
func EnvironmentFlakeInputs(name string) ([]string, error) {
	env, err := GetEnvironment(name)
	if err != nil {
		return nil, err
	}
	var data []byte
	if dir, ok := localFlakePath(env.FlakeRef); ok {
		data, err = os.ReadFile(filepath.Join(dir, "flake.lock"))
	} else {
		args := append(experimentalArgs(), "flake", "metadata", "--json", env.FlakeRef)
		cmd := exec.Command(NixBinary(), args...)
		data, err = cmd.Output()
	}
	if err != nil {
		return nil, fmt.Errorf("read flake inputs: %w", err)
	}
	var doc struct {
		Root  string `json:"root"`
		Nodes map[string]struct {
			Inputs map[string]any `json:"inputs"`
		} `json:"nodes"`
		Locks *struct {
			Root  string `json:"root"`
			Nodes map[string]struct {
				Inputs map[string]any `json:"inputs"`
			} `json:"nodes"`
		} `json:"locks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse flake inputs: %w", err)
	}
	root, nodes := doc.Root, doc.Nodes
	if doc.Locks != nil {
		root, nodes = doc.Locks.Root, doc.Locks.Nodes
	}
	if root == "" {
		root = "root"
	}
	inputs := make([]string, 0, len(nodes[root].Inputs))
	for input := range nodes[root].Inputs {
		inputs = append(inputs, input)
	}
	sort.Strings(inputs)
	return inputs, nil
}

func EnvironmentUsesLocalFlake(name string) bool {
	env, err := GetEnvironment(name)
	if err != nil {
		return false
	}
	_, ok := localFlakePath(env.FlakeRef)
	return ok
}

func runNixStreaming(dir string, args ...string) error {
	cmd := exec.Command(NixBinary(), append(experimentalArgs(), args...)...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// CheckEnvironmentUpdate asks Nix for lock changes without modifying the lock.
func CheckEnvironmentUpdate(name, input string) error {
	env, err := GetEnvironment(name)
	if err != nil {
		return err
	}
	dir, ok := localFlakePath(env.FlakeRef)
	if !ok {
		if input != "" {
			return fmt.Errorf("--input requires a writable local flake; environment %q uses %s", name, env.FlakeRef)
		}
		common.PrintInfoMessage("Remote flake: checking refreshed metadata (no local lock file is modified).")
		return runNixStreaming("", "flake", "metadata", "--refresh", env.FlakeRef)
	}
	args := []string{"flake", "update", "--dry-run", "--flake", dir}
	if input != "" {
		args = append(args, input)
	}
	return runNixStreaming(dir, args...)
}

// UpdateEnvironment updates the writable flake lock (optionally one input) and
// transactionally rebuilds the named environment. A failed build keeps the
// current profile and manifest untouched.
func UpdateEnvironment(name string, opts UpdateOptions) error {
	if opts.Check {
		return CheckEnvironmentUpdate(name, opts.Input)
	}
	env, err := GetEnvironment(name)
	if err != nil {
		return err
	}
	if env.Lazy || env.ProfilePath == "" {
		return fmt.Errorf("environment %q is %s; update rollback requires an eager realised environment (recreate it without --lazy/--pure)", name, map[bool]string{true: "on-demand", false: "pure"}[env.Lazy])
	}
	dir, ok := localFlakePath(env.FlakeRef)
	var lockPath string
	var oldLock []byte
	lockExisted := false
	if !ok {
		if opts.Input != "" {
			return fmt.Errorf("--input requires a writable local flake; environment %q uses %s", name, env.FlakeRef)
		}
		common.PrintInfoMessage("Refreshing the remote flake reference before rebuilding.")
		if err := runNixStreaming("", "flake", "metadata", "--refresh", env.FlakeRef); err != nil {
			return err
		}
	} else {
		lockPath = filepath.Join(dir, "flake.lock")
		oldLock, err = os.ReadFile(lockPath)
		if err == nil {
			lockExisted = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("read flake lock: %w", err)
		}
		args := []string{"flake", "update", "--flake", dir}
		if opts.Input != "" {
			args = append(args, opts.Input)
		}
		if err := runNixStreaming(dir, args...); err != nil {
			return fmt.Errorf("flake lock update failed: %w", err)
		}
	}
	if err := rebuildEnvironment(env, opts.Input); err != nil {
		// A source update that does not build must not leave the project pinned to
		// a broken lock. Restore the exact prior bytes (or remove a newly-created
		// lock) while the active profile remains untouched.
		if lockPath != "" {
			if lockExisted {
				_ = os.WriteFile(lockPath, oldLock, 0o644)
			} else {
				_ = os.Remove(lockPath)
			}
		}
		return fmt.Errorf("updated environment did not build; active generation kept and flake.lock restored: %w", err)
	}
	return nil
}

// RebuildEnvironment rebuilds against the currently pinned flake without
// changing flake.lock.
func RebuildEnvironment(name string) error {
	env, err := GetEnvironment(name)
	if err != nil {
		return err
	}
	if env.Lazy || env.ProfilePath == "" {
		return fmt.Errorf("environment %q has no eager profile to rebuild", name)
	}
	return rebuildEnvironment(env, "")
}

func rebuildEnvironment(env *Environment, input string) error {
	if err := buildPrerequisites(env.FlakeRef, env.Image, env.Prerequisites); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(EnvDir(env.Name), ".update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	candidate := filepath.Join(tmpDir, "profile")
	common.PrintInfoMessage(fmt.Sprintf("Building updated environment %q without replacing the active generation...", env.Name))
	if err := buildProfile(env.FlakeRef, env.Image, candidate); err != nil {
		return err
	}
	storePath, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return fmt.Errorf("resolve updated profile: %w", err)
	}
	if active, activeErr := filepath.EvalSymlinks(profileLink(env.Name)); activeErr == nil && active == storePath {
		env.Updated = time.Now()
		env.LastUpdateInput = input
		if err := writeManifest(env); err != nil {
			return err
		}
		common.PrintSuccessMessage(fmt.Sprintf("Environment %q is already at the requested generation.", env.Name))
		return nil
	}
	if err := archiveCurrentProfile(env.Name); err != nil {
		return err
	}
	if err := switchProfile(profileLink(env.Name), storePath); err != nil {
		return err
	}
	env.ProfilePath = profileLink(env.Name)
	env.Updated = time.Now()
	env.LastUpdateInput = input
	if err := writeManifest(env); err != nil {
		return err
	}
	markAuditStale(env.Name)
	common.PrintSuccessMessage(fmt.Sprintf("Environment %q rebuilt; the previous closure is available with 'rfswift env rollback %s'.", env.Name, env.Name))
	return nil
}

func switchProfile(link, storePath string) error {
	tmp := link + ".new"
	_ = os.Remove(tmp)
	if err := os.Symlink(storePath, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func archiveCurrentProfile(name string) error {
	profile := profileLink(name)
	storePath, err := filepath.EvalSymlinks(profile)
	if err != nil {
		return fmt.Errorf("active profile cannot be archived: %w", err)
	}
	dir := generationsDir(name)
	if err := ensureDir(dir); err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return os.Symlink(storePath, filepath.Join(dir, stamp))
}

func ListGenerations(name string) ([]Generation, error) {
	if _, err := GetEnvironment(name); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(generationsDir(name))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	gens := make([]Generation, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(generationsDir(name), entry.Name())
		store, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		created, _ := time.Parse("20060102T150405.000000000Z", entry.Name())
		gens = append(gens, Generation{Name: entry.Name(), StorePath: store, Created: created})
	}
	sort.Slice(gens, func(i, j int) bool { return gens[i].Name > gens[j].Name })
	return gens, nil
}

// RollbackEnvironment switches to a saved generation. Empty generation means
// the newest saved one. The displaced current closure is itself preserved.
func RollbackEnvironment(name, generation string) error {
	env, err := GetEnvironment(name)
	if err != nil {
		return err
	}
	gens, err := ListGenerations(name)
	if err != nil {
		return err
	}
	if len(gens) == 0 {
		return fmt.Errorf("environment %q has no previous generations", name)
	}
	target := gens[0]
	if generation != "" {
		found := false
		for _, g := range gens {
			if g.Name == generation {
				target, found = g, true
				break
			}
		}
		if !found {
			return fmt.Errorf("generation %q not found; list them with: rfswift env generations %s", generation, name)
		}
	}
	if err := archiveCurrentProfile(name); err != nil {
		return err
	}
	if err := switchProfile(profileLink(name), target.StorePath); err != nil {
		return err
	}
	env.Updated = time.Now()
	env.LastUpdateInput = "rollback:" + target.Name
	if err := writeManifest(env); err != nil {
		return err
	}
	markAuditStale(name)
	common.PrintSuccessMessage(fmt.Sprintf("Environment %q rolled back to %s.", name, target.Name))
	return nil
}

func markAuditStale(name string) {
	src := EnvReportDir(name)
	if _, err := os.Stat(src); err != nil {
		return
	}
	dst := filepath.Join(EnvDir(name), "security-report.stale-"+time.Now().UTC().Format("20060102T150405Z"))
	_ = os.Rename(src, dst)
}

// MarshalGenerations is useful to GUI/agent callers without duplicating the
// stable JSON representation.
func MarshalGenerations(g []Generation) ([]byte, error) { return json.Marshal(g) }
