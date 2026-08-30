package nix

/*
*  Per-tool maintenance for environments.
*
*  Tools reach an environment two ways: on-demand shims (lazy environments:
*  `nix run <flake>#<attr>` on first call) and the extras profile (packages the
*  user installed with `rfswift env install`). Neither is rebuilt by a whole
*  environment update on a lazy environment — there is no eager profile to
*  switch — so this file offers the per-tool operations the GUI installer
*  exposes: list what an environment has, and refresh a single tool.
 */

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// EnvironmentTool is one tool known to an environment.
type EnvironmentTool struct {
	// Name is the command (shim) or profile element name shown to the user.
	Name string `json:"name"`
	// Attr is the flake attribute behind it (shims), or the attrPath recorded
	// by nix profile (installed extras).
	Attr string `json:"attr"`
	// Kind is "on-demand" (lazy shim) or "installed" (extras profile).
	Kind string `json:"kind"`
	// StorePath is the currently realised output, when known.
	StorePath string `json:"storePath,omitempty"`
}

// ListInstalledExtras returns the packages in an environment's extras profile
// (what `rfswift env install --env NAME` added), sorted by name.
func ListInstalledExtras(envName string) ([]EnvironmentTool, error) {
	profile := EnvExtrasProfile(envName)
	if !pathExists(profile) {
		return nil, nil
	}
	args := append(experimentalArgs(), "profile", "list", "--json", "--profile", profile)
	out, err := exec.Command(NixBinary(), args...).Output()
	if err != nil {
		return nil, fmt.Errorf("nix profile list: %w", err)
	}
	// nix >= 2.20 keys elements by name; older versions return an array.
	var keyed struct {
		Elements map[string]struct {
			AttrPath   string   `json:"attrPath"`
			StorePaths []string `json:"storePaths"`
		} `json:"elements"`
	}
	var tools []EnvironmentTool
	if err := json.Unmarshal(out, &keyed); err == nil && keyed.Elements != nil {
		for name, el := range keyed.Elements {
			t := EnvironmentTool{Name: name, Attr: el.AttrPath, Kind: "installed"}
			if len(el.StorePaths) > 0 {
				t.StorePath = el.StorePaths[0]
			}
			tools = append(tools, t)
		}
	} else {
		var listed struct {
			Elements []struct {
				AttrPath   string   `json:"attrPath"`
				StorePaths []string `json:"storePaths"`
			} `json:"elements"`
		}
		if err := json.Unmarshal(out, &listed); err != nil {
			return nil, fmt.Errorf("parse nix profile list: %w", err)
		}
		for _, el := range listed.Elements {
			name := el.AttrPath
			if i := strings.LastIndex(name, "."); i >= 0 {
				name = name[i+1:]
			}
			t := EnvironmentTool{Name: name, Attr: el.AttrPath, Kind: "installed"}
			if len(el.StorePaths) > 0 {
				t.StorePath = el.StorePaths[0]
			}
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

// ListEnvironmentTools returns every tool an environment provides that can be
// refreshed individually: the on-demand shims of a lazy environment, and the
// packages installed into its extras profile.
func ListEnvironmentTools(envName string) ([]EnvironmentTool, error) {
	env, err := GetEnvironment(envName)
	if err != nil {
		return nil, err
	}
	var tools []EnvironmentTool
	if env.Lazy {
		for command, attr := range env.Commands {
			tools = append(tools, EnvironmentTool{Name: command, Attr: attr, Kind: "on-demand"})
		}
		sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	}
	extras, err := ListInstalledExtras(envName)
	if err != nil {
		return nil, err
	}
	return append(tools, extras...), nil
}

// UpdateEnvironmentTool refreshes one tool of an environment:
//   - an installed extra is upgraded in place (`nix profile upgrade`), which
//     re-resolves it from its flake (a local flake's current source, or the
//     refreshed remote);
//   - an on-demand tool is rebuilt from the environment's flake right now
//     (`nix build --refresh`), so its next call runs the new version instantly
//     instead of building at that moment.
//
// name may be a command/shim name, a profile element name, or a flake attr.
func UpdateEnvironmentTool(envName, name string) error {
	if !IsAvailable() {
		return fmt.Errorf("nix is not installed or not on PATH")
	}
	env, err := GetEnvironment(envName)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	extras, err := ListInstalledExtras(envName)
	if err != nil {
		return err
	}
	for _, t := range extras {
		if t.Name == name || t.Attr == name || strings.HasSuffix(t.Attr, "."+name) {
			return runNixStreaming("", "profile", "upgrade", "--profile", EnvExtrasProfile(envName), t.Name)
		}
	}
	attr := ""
	if a, ok := env.Commands[name]; ok {
		attr = a
	} else {
		for _, a := range env.Commands {
			if a == name {
				attr = a
				break
			}
		}
	}
	if attr == "" {
		return fmt.Errorf("%q is neither installed into nor provided on demand by environment %q", name, envName)
	}
	if !env.Lazy {
		return fmt.Errorf("%q is part of the eager profile of %q; update the whole environment instead", name, envName)
	}
	args := []string{"build", "--no-link"}
	if _, local := localFlakePath(env.FlakeRef); !local {
		// A remote flake is cached by Nix; make the build see its current head.
		args = append(args, "--refresh")
	}
	args = append(args, fmt.Sprintf("%s#%s", env.FlakeRef, attr))
	return runNixStreaming("", args...)
}
