/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - optional lightweight isolation ("jail") for an environment
*  shell, via bubblewrap (Linux user namespaces).
*
*  The Nix engine normally runs tools natively as the user with full access to
*  the host (see types.go / the --engine nix docs): great for driving real RF
*  hardware, but no containment. `--isolate` wraps the environment shell in a
*  bubblewrap sandbox that hides the real $HOME and the rest of the host
*  filesystem, gives the shell its own PID/IPC/UTS namespaces and a private
*  /tmp, while DELIBERATELY keeping what RF tools need: the /nix store, USB and
*  serial devices, sysfs/udev for enumeration, the X11/Wayland display, and the
*  network. It is a usability-preserving jail, not a maximum-security sandbox.
*
*  Linux only: bubblewrap relies on Linux namespaces. On macOS the caller is
*  told isolation is unavailable rather than running unisolated silently.
 */

package nix

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// IsolateSupported reports whether --isolate can be honoured on this OS.
func IsolateSupported() bool { return runtime.GOOS == "linux" }

// jailHomeDir is the per-environment private HOME the jail exposes in place of
// the real one. It persists (so a tool's config/state survive re-entry) but is
// invisible to the host home.
func jailHomeDir(name string) string {
	return filepath.Join(EnvDir(name), "jail-home")
}

// isSetuid reports whether p exists and has the setuid bit set. A setuid-root
// bwrap sandboxes without needing unprivileged user namespaces, so it works
// where those are restricted (e.g. Ubuntu 24.04+/Debian AppArmor defaults).
func isSetuid(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode()&os.ModeSetuid != 0
}

// resolveBwrap finds the bubblewrap binary. A setuid-root bwrap is preferred
// (it does not need unprivileged user namespaces), then any bwrap on PATH
// (including the NixOS wrapper), then a build from nixpkgs.
func resolveBwrap() (string, error) {
	candidates := []string{"/run/wrappers/bin/bwrap"} // NixOS setuid wrapper
	if p, err := exec.LookPath("bwrap"); err == nil {
		candidates = append(candidates, p)
	}
	// Prefer a setuid one; remember the first that merely exists as a fallback.
	fallback := ""
	for _, c := range candidates {
		if isSetuid(c) {
			return c, nil
		}
		if fallback == "" && pathExists(c) {
			fallback = c
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	args := append(experimentalArgs(), "build", "--no-link", "--print-out-paths", "nixpkgs#bubblewrap")
	out, err := exec.Command(NixBinary(), args...).Output()
	if err != nil {
		return "", fmt.Errorf("bubblewrap not found on PATH and building it from nixpkgs failed: %w", err)
	}
	store := strings.TrimSpace(string(out))
	if i := strings.IndexByte(store, '\n'); i >= 0 {
		store = strings.TrimSpace(store[:i])
	}
	if store == "" {
		return "", fmt.Errorf("could not resolve bubblewrap")
	}
	cand := filepath.Join(store, "bin", "bwrap")
	if !pathExists(cand) {
		return "", fmt.Errorf("bubblewrap build produced no bwrap at %s", cand)
	}
	return cand, nil
}

// bwrapPreflight runs a trivial sandbox to check bubblewrap can actually create
// namespaces on this host, turning the cryptic runtime failure into actionable
// guidance (the common cause is restricted unprivileged user namespaces).
func bwrapPreflight(bwrap string) error {
	out, err := exec.Command(bwrap, "--ro-bind", "/", "/", "--proc", "/proc", "--", "true").CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	low := strings.ToLower(msg)
	if strings.Contains(low, "namespace") || strings.Contains(low, "uid map") ||
		strings.Contains(low, "user namespace") || strings.Contains(low, "permission denied") ||
		strings.Contains(low, "operation not permitted") || strings.Contains(low, "setuid") {
		return fmt.Errorf("bubblewrap cannot create a sandbox on this host, so --isolate is unavailable:\n"+
			"  %s\n"+
			"  This is almost always restricted unprivileged user namespaces (default on Ubuntu 24.04+/Debian with AppArmor).\n"+
			"  Fix it as root, then retry, e.g.:\n"+
			"    sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0\n"+
			"    sudo sysctl -w kernel.unprivileged_userns_clone=1        # older kernels\n"+
			"  Persist by adding those lines under /etc/sysctl.d/. Alternatively install a setuid bubblewrap\n"+
			"  (so it needs no unprivileged namespaces), or run without --isolate.", msg)
	}
	return fmt.Errorf("bubblewrap preflight failed, --isolate is unavailable: %s", msg)
}

// Fixed in-jail mount points, kept OUTSIDE the private home so the home stays
// clean (binding anything under $HOME makes bubblewrap materialise its parent
// dirs there, which looks like leaked files). RF Swift's state dir (BaseDir,
// ~/.rfswift/nix) is remapped to jailBase and the working directory to
// jailWorkdir; PATH, the --rcfile path and the cwd are rewritten to match.
const (
	jailEnv     = "/rfswift/env"    // this environment's state (profile, shims, bashrc)
	jailShared  = "/rfswift/shared" // tools installed with --shared, common to all envs
	jailWorkdir = "/workspace"      // this environment's workspace
)

// jailMount is one host directory exposed in the jail at a clean path.
type jailMount struct {
	host string
	jail string
	rw   bool
}

// jailMounts returns exactly the host directories the jail exposes for env, and
// where. It is deliberately narrow: only THIS environment's state, the
// shared-extras profile, and this environment's workspace - never the sibling
// environments under ~/.rfswift/nix/environments, nor any other workspace. So
// an isolated session cannot read or tamper with another workspace's files or
// its captured evidence.
func jailMounts(env *Environment, workdir string) []jailMount {
	mounts := []jailMount{{host: EnvDir(env.Name), jail: jailEnv}}
	if sh := SharedExtrasProfile(); pathExists(sh) {
		mounts = append(mounts, jailMount{host: sh, jail: jailShared})
	}
	if workdir != "" && workdir != homeDir() && pathExists(workdir) {
		mounts = append(mounts, jailMount{host: workdir, jail: jailWorkdir, rw: true})
	}
	return mounts
}

// isolateArgs builds the bubblewrap argument vector for env's jail, up to the
// "--" separator (the inner command is appended by the caller). workdir is the
// directory the shell should start in.
func isolateArgs(env *Environment, workdir string) []string {
	home := homeDir()
	jail := jailHomeDir(env.Name)
	_ = os.MkdirAll(jail, 0o700)

	args := []string{
		"--die-with-parent",
		"--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
		"--proc", "/proc",
		"--dev", "/dev",

		// Read-only host runtime the tools and the shell resolve against. /nix
		// carries the whole environment closure (and the profile gcroots).
		"--ro-bind", "/nix", "/nix",
		"--ro-bind-try", "/bin", "/bin",
		"--ro-bind-try", "/usr", "/usr",
		"--ro-bind-try", "/lib", "/lib",
		"--ro-bind-try", "/lib64", "/lib64",
		"--ro-bind-try", "/etc", "/etc",
		"--ro-bind-try", "/run/current-system", "/run/current-system",
		"--ro-bind-try", "/run/opengl-driver", "/run/opengl-driver",
		"--ro-bind-try", "/run/opengl-driver-32", "/run/opengl-driver-32",

		// Hardware access: USB (libusb - HackRF, Proxmark, RTL-SDR, bladeRF, ...)
		// and sysfs/udev so libusb and tools can enumerate devices.
		"--dev-bind-try", "/dev/bus/usb", "/dev/bus/usb",
		"--ro-bind-try", "/sys", "/sys",
		"--ro-bind-try", "/run/udev", "/run/udev",
	}

	// Serial device nodes are created dynamically and bubblewrap cannot glob, so
	// bind a common range best-effort (missing ones are silently skipped).
	for _, b := range []string{"/dev/ttyUSB", "/dev/ttyACM", "/dev/ttyS"} {
		for i := 0; i < 8; i++ {
			node := b + strconv.Itoa(i)
			args = append(args, "--dev-bind-try", node, node)
		}
	}

	// Private, clean HOME (persistent per environment): nothing else is bound
	// under it, so `ls $HOME` shows only what the tools create. A private /tmp
	// with just the display sockets bound back in.
	args = append(args,
		"--bind", jail, home,
		"--setenv", "HOME", home,
		"--tmpfs", "/tmp",
		"--ro-bind-try", "/tmp/.X11-unix", "/tmp/.X11-unix",
	)
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		for _, s := range []string{"wayland-0", "wayland-1", "pulse/native", "pipewire-0"} {
			p := filepath.Join(xdg, s)
			args = append(args, "--ro-bind-try", p, p)
		}
	}

	// Expose ONLY this environment's state, the shared-extras profile and the
	// workspace, each at a clean path outside the home. Sibling environments and
	// other workspaces are never mounted, so an isolated session cannot touch
	// another workspace's files or its captured evidence. The workspace is the
	// working directory; otherwise start in the (empty) home.
	chdir := home
	for _, m := range jailMounts(env, workdir) {
		if m.rw {
			args = append(args, "--bind-try", m.host, m.jail)
			chdir = m.jail
		} else {
			args = append(args, "--ro-bind-try", m.host, m.jail)
		}
	}
	args = append(args, "--chdir", chdir)

	return args
}

// jailRemap rewrites a host path that lives under one of the jail mounts to its
// in-jail location, so PATH, the --rcfile argument and the cwd resolve inside
// the sandbox. The first matching mount wins.
func jailRemap(s string, mounts []jailMount) string {
	for _, m := range mounts {
		if strings.HasPrefix(s, m.host) {
			return m.jail + strings.TrimPrefix(s, m.host)
		}
	}
	return s
}

// jailRemapList rewrites every ':'-separated element (e.g. PATH) through
// jailRemap.
func jailRemapList(v string, mounts []jailMount) string {
	parts := strings.Split(v, ":")
	for i, p := range parts {
		parts[i] = jailRemap(p, mounts)
	}
	return strings.Join(parts, ":")
}

// IsolateCommand wraps inner in a bubblewrap jail for env. It returns a new
// *exec.Cmd that runs the same program with the same environment and stdio
// inside the sandbox, with paths under RF Swift's state dir and the working
// directory remapped to their clean in-jail locations.
func IsolateCommand(inner *exec.Cmd, env *Environment, workdir string) (*exec.Cmd, error) {
	if !IsolateSupported() {
		return nil, fmt.Errorf("--isolate is only available on Linux (bubblewrap); on macOS use the container engine for isolation, or drop --isolate")
	}
	bwrap, err := resolveBwrap()
	if err != nil {
		return nil, err
	}
	if err := bwrapPreflight(bwrap); err != nil {
		return nil, err
	}
	mounts := jailMounts(env, workdir)
	args := isolateArgs(env, workdir)
	args = append(args, "--")

	// Remap the program arguments (e.g. the shell's --rcfile path under the
	// environment's state dir). The program itself (args[0]) and a user "-c"
	// command are left as-is except for an exact mount prefix, which is safe.
	for _, a := range inner.Args {
		args = append(args, jailRemap(a, mounts))
	}

	cmd := exec.Command(bwrap, args...)

	// Remap path-bearing environment values (PATH and any var pointing into a
	// mounted directory) so tools resolve inside the jail.
	env2 := make([]string, 0, len(inner.Env))
	for _, kv := range inner.Env {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key, val := kv[:i], kv[i+1:]
			env2 = append(env2, key+"="+jailRemapList(val, mounts))
		} else {
			env2 = append(env2, kv)
		}
	}
	cmd.Env = env2
	// bwrap runs from a host dir; the in-jail cwd is set with --chdir above.
	cmd.Dir = "/"
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inner.Stdin, inner.Stdout, inner.Stderr
	return cmd, nil
}
