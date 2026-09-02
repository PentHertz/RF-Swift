package nix

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InteractiveCommand prepares, but does not start, an interactive shell for a
// persisted RF Swift Nix environment. GUI/TUI PTY frontends use this so shell
// completion, resize, full-screen programs and job control behave normally.
func InteractiveCommand(name, requestedShell string) (*exec.Cmd, error) {
	if useWSL() {
		return wslInteractiveCommand(name)
	}
	if !IsAvailable() {
		return nil, fmt.Errorf("nix is not installed or not on PATH")
	}
	env, err := GetEnvironment(name)
	if err != nil {
		return nil, err
	}
	if env.Lazy && !pathExists(shimsDir(name)) {
		if env.Commands == nil {
			env.Commands = resolveCommands(env.FlakeRef, env.Packages)
		}
		if err := writeShims(env); err != nil {
			return nil, err
		}
	}
	if !env.Lazy && env.ProfilePath != "" && !pathExists(env.ProfilePath) {
		if err := buildProfile(env.FlakeRef, env.Image, env.ProfilePath); err != nil {
			return nil, err
		}
	}
	workdir := env.Workspace
	if workdir == "" || !pathExists(workdir) {
		workdir, _ = os.Getwd()
	}
	shell := requestedShell
	if shell == "" || !pathExists(shell) {
		shell = userShell()
	}
	// Same host-side setup as the CLI shell: X server access for local GUI
	// tools, and the OpenGL runtime on hosts that are not NixOS (gl.go).
	setupX11()
	gl := GLEnvironment(env, false)
	pure := env.ProfilePath == "" && !env.Lazy
	if pure {
		args := append(experimentalArgs(), "develop", fmt.Sprintf("%s#%s", env.FlakeRef, env.Image), "--ignore-environment")
		for _, key := range glEnvKeys(gl) {
			args = append(args, "--keep", key)
		}
		args = append(args, "--command", shell)
		cmd := nixCommand(args...)
		cmd.Env = withEnv(os.Environ(), gl)
		cmd.Dir = workdir
		if env.Isolate {
			return IsolateCommand(cmd, env, workdir)
		}
		return cmd, nil
	}
	binDir := filepath.Join(env.ProfilePath, "bin")
	if env.Lazy {
		binDir = shimsDir(env.Name)
	}
	pathParts := []string{binDir}
	if p := filepath.Join(EnvExtrasProfile(env.Name), "bin"); pathExists(p) {
		pathParts = append(pathParts, p)
	}
	if p := filepath.Join(SharedExtrasProfile(), "bin"); pathExists(p) {
		pathParts = append(pathParts, p)
	}
	pathParts = append(pathParts, os.Getenv("PATH"))
	var cmd *exec.Cmd
	if filepath.Base(shell) == "bash" {
		if rc, err := writeBashRC(env, binDir); err == nil {
			cmd = exec.Command(shell, "--rcfile", rc, "-i")
		} else {
			cmd = exec.Command(shell, "-i")
		}
	} else {
		cmd = exec.Command(shell, "-i")
	}
	vars := map[string]string{"PATH": strings.Join(pathParts, string(os.PathListSeparator)), "RFSWIFT_NIX_ENV": env.Name, "RFSWIFT_ENGINE": "nix", "TERM": "xterm-256color", "COLORTERM": "truecolor"}
	for k, v := range pluginPathEnv(env) {
		vars[k] = v
	}
	for k, v := range gl {
		vars[k] = v
	}
	cmd.Env = withEnv(os.Environ(), vars)
	cmd.Dir = workdir
	if env.Isolate {
		return IsolateCommand(cmd, env, workdir)
	}
	return cmd, nil
}
