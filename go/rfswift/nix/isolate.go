/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
*
*  Nix engine - optional lightweight isolation ("jail") for an environment
*  shell: bubblewrap on Linux, the Seatbelt sandbox (sandbox-exec) on macOS.
*
*  The Nix engine normally runs tools natively as the user with full access to
*  the host (see types.go / the --engine nix docs): great for driving real RF
*  hardware, but no containment. `--isolate` wraps the environment shell in a
*  sandbox that hides the real $HOME and the rest of the host filesystem, while
*  DELIBERATELY keeping what RF tools need: the /nix store, USB (and serial
*  devices on Linux), the display, and the network. It is a usability-preserving
*  jail, not a maximum-security sandbox.
*
*  Linux: bubblewrap bind-mounts a private $HOME and the environment's state,
*  gives the shell its own PID/IPC/UTS namespaces and a private /tmp, and binds
*  USB/serial/sysfs back in.
*
*  macOS: bubblewrap and mount namespaces do not exist, so the jail is a
*  Seatbelt policy applied with sandbox-exec. It cannot mount an empty $HOME, so
*  it denies the real one (and every other user home - personal files, sibling
*  environments, other workspaces) and points HOME at a private per-environment
*  directory; USB (IOKit), the display and the network stay reachable. See the
*  macOS section at the bottom of this file.
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

// IsolateSupported reports whether --isolate can be honoured on this OS. Linux
// uses bubblewrap; macOS uses the Seatbelt sandbox (sandbox-exec).
func IsolateSupported() bool {
	switch runtime.GOOS {
	case "linux":
		return true
	case "darwin":
		return sandboxExecPath() != ""
	case "windows":
		// The environment runs inside the WSL 2 distribution, where bubblewrap
		// (built from nixpkgs on first use) jails it like on any Linux host.
		st, err := wslState()
		return err == nil && st.Ready()
	default:
		return false
	}
}

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

// Where the distribution's bubblewrap package installs the binary. Ubuntu
// 24.04+ restricts unprivileged user namespaces with AppArmor: only a bwrap
// covered by an AppArmor profile may create one, and that profile (shipped
// active on 26.04, optional in apparmor-profiles on 24.04) is attached to this
// path. A bwrap from a Nix profile or a local build is unconfined and fails
// with "setting up uid map: Permission denied" there, so the distribution's
// binary is preferred over any other found on PATH. A variable so tests can
// point it elsewhere.
var distroBwrap = "/usr/bin/bwrap"

// The kernel knobs that decide whether an unprivileged, non-setuid bwrap may
// create its user namespace. The first is Ubuntu's (24.04+, AppArmor); the
// second is carried by Debian kernels, on by default.
const (
	sysctlAppArmorUserns = "/proc/sys/kernel/apparmor_restrict_unprivileged_userns"
	sysctlUsernsClone    = "/proc/sys/kernel/unprivileged_userns_clone"
)

// readProcSysctl returns the trimmed content of a /proc/sys entry, or "" when
// the kernel has no such knob. A variable so tests can simulate hosts.
var readProcSysctl = func(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// resolveBwrap finds the bubblewrap binary. A setuid-root bwrap is preferred
// (it does not need unprivileged user namespaces), then the distribution's
// /usr/bin/bwrap (the one an AppArmor profile covers where user namespaces are
// restricted), then any other bwrap on PATH, then a build from nixpkgs.
func resolveBwrap() (string, error) {
	candidates := []string{"/run/wrappers/bin/bwrap", distroBwrap} // NixOS setuid wrapper, distro package
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
	out, err := nixCommand(args...).Output()
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
		return fmt.Errorf("%s", usernsGuidance(bwrap, msg))
	}
	return fmt.Errorf("bubblewrap preflight failed, --isolate is unavailable: %s", msg)
}

// usernsGuidance explains why bwrap at path could not create its user
// namespace (bwrap's own message is msg) and how to fix it, from the most
// targeted change to the bluntest. The host's sysctls decide which case
// applies: Ubuntu 24.04+ restricts unprivileged user namespaces with AppArmor
// and only a profiled bwrap (/usr/bin/bwrap) may create one; a Debian kernel
// with kernel.unprivileged_userns_clone=0 forbids them for every program.
// Stock Debian does not restrict them.
func usernsGuidance(path, msg string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bubblewrap cannot create a sandbox on this host, so --isolate is unavailable:\n  %s\n", msg)
	switch {
	case readProcSysctl(sysctlAppArmorUserns) == "1":
		b.WriteString("  This host restricts unprivileged user namespaces with AppArmor (Ubuntu 24.04+ default):\n")
		fmt.Fprintf(&b, "  only a bwrap covered by an AppArmor profile may create one, and that profile is attached to %s.\n", distroBwrap)
		if path != distroBwrap {
			fmt.Fprintf(&b, "  The bwrap in use is %s, which no profile covers. Install the distribution's package\n", path)
			fmt.Fprintf(&b, "  (sudo apt install bubblewrap); RF Swift then prefers %s.\n", distroBwrap)
		}
		b.WriteString("  On Ubuntu 24.04 that profile is optional; enable it (it keeps the restriction for everything else):\n")
		b.WriteString("    sudo apt install apparmor-profiles\n")
		b.WriteString("    sudo cp /usr/share/apparmor/extra-profiles/bwrap-userns-restrict /etc/apparmor.d/\n")
		b.WriteString("    sudo apparmor_parser -r /etc/apparmor.d/bwrap-userns-restrict\n")
		b.WriteString("  get_rfswift.sh offers both steps. Last resort, it weakens the host's hardening for every program:\n")
		b.WriteString("    sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0   # persist under /etc/sysctl.d/\n")
	case readProcSysctl(sysctlUsernsClone) == "0":
		b.WriteString("  Unprivileged user namespaces are disabled on this kernel (kernel.unprivileged_userns_clone=0). Enable them as root:\n")
		b.WriteString("    sudo sysctl -w kernel.unprivileged_userns_clone=1   # persist under /etc/sysctl.d/\n")
	default:
		b.WriteString("  This usually means unprivileged user namespaces are restricted: check as root\n")
		b.WriteString("    sysctl kernel.apparmor_restrict_unprivileged_userns   # Ubuntu 24.04+: 1 = only AppArmor-profiled programs\n")
		b.WriteString("    sysctl kernel.unprivileged_userns_clone               # Debian kernels: 0 = disabled\n")
		b.WriteString("  or your kernel's hardening (seccomp, a container runtime).\n")
	}
	b.WriteString("  Alternatively install a setuid bubblewrap, which needs no unprivileged namespaces, or run without --isolate.")
	return b.String()
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
// workdir marks the workspace, where the shell starts.
type jailMount struct {
	host    string
	jail    string
	rw      bool
	workdir bool
}

// jailMounts returns exactly the host directories the jail exposes for env, and
// where. It is deliberately narrow: only THIS environment's state, the
// shared-extras profile, and this environment's workspace - never the sibling
// environments under ~/.rfswift/nix/environments, nor any other workspace. So
// an isolated session cannot read or tamper with another workspace's files or
// its captured evidence.
//
// The state dir is read-only - a jailed tool must not rewrite the manifest
// (its workspace path decides what the next session mounts) or the shell rc
// files - except tools/, where a lazy environment's shims pin what they build
// (`nix build --out-link tools/<attr>`). Its own private HOME (under the state
// dir) is bound over HOME separately.
func jailMounts(env *Environment, workdir string) []jailMount {
	mounts := []jailMount{{host: EnvDir(env.Name), jail: jailEnv}}
	if env.Lazy {
		tools := toolsDir(env.Name)
		_ = ensureDir(tools)
		mounts = append(mounts, jailMount{host: tools, jail: jailEnv + "/tools", rw: true})
	}
	if sh := SharedExtrasProfile(); pathExists(sh) {
		mounts = append(mounts, jailMount{host: sh, jail: jailShared})
	}
	if workdir != "" && workdir != homeDir() && pathExists(workdir) {
		mounts = append(mounts, jailMount{host: workdir, jail: jailWorkdir, rw: true, workdir: true})
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
	linkJailWorkspace(jail, jailWorkdir, workdir)

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

		// WSL 2 (absent elsewhere, so these are no-ops there): WSLg serves the
		// display and PulseAudio through /mnt/wslg (/tmp/.X11-unix and
		// PULSE_SERVER point into it), and the host GPU is reached through
		// /dev/dxg by Mesa's d3d12 driver with the libraries under /usr/lib/wsl.
		"--ro-bind-try", "/mnt/wslg", "/mnt/wslg",
		"--dev-bind-try", "/dev/dxg", "/dev/dxg",
	}

	// Serial device nodes are created dynamically and bubblewrap cannot glob, so
	// bind a common range best-effort (missing ones are silently skipped).
	for _, b := range []string{"/dev/ttyUSB", "/dev/ttyACM", "/dev/ttyS"} {
		for i := 0; i < 8; i++ {
			node := b + strconv.Itoa(i)
			args = append(args, "--dev-bind-try", node, node)
		}
	}

	// The network is kept, so name resolution must work too: on systemd-resolved
	// hosts (Ubuntu desktops, WSL 2 with systemd) /etc/resolv.conf is a symlink
	// into /run, which the read-only /etc bind alone does not carry. Bind the
	// resolved target at its own path so the symlink resolves inside the jail.
	args = append(args, resolvConfBinds(filepath.EvalSymlinks)...)

	// Private, clean HOME (persistent per environment): nothing else is bound
	// under it, so `ls $HOME` shows only what the tools create, plus the
	// `workspace` link to /workspace. A private /tmp with just the display
	// sockets bound back in.
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
		} else {
			args = append(args, "--ro-bind-try", m.host, m.jail)
		}
		if m.workdir {
			chdir = m.jail
		}
	}
	args = append(args, "--chdir", chdir)

	return args
}

// resolvConfBinds returns the bubblewrap arguments that make /etc/resolv.conf
// usable in the jail when it is a symlink out of /etc (systemd-resolved's
// /run/systemd/resolve/stub-resolv.conf, resolvconf's /run/resolvconf/...,
// NetworkManager's /run/NetworkManager/...). A plain file under /etc needs
// nothing: the /etc bind carries it.
func resolvConfBinds(evalSymlinks func(string) (string, error)) []string {
	target, err := evalSymlinks("/etc/resolv.conf")
	if err != nil || target == "" || target == "/etc/resolv.conf" || strings.HasPrefix(target, "/etc/") {
		return nil
	}
	return []string{"--ro-bind-try", target, target}
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

// linkJailWorkspace keeps a `workspace` symlink in the jail's private HOME
// pointing at where the workspace is seen from inside (target): /workspace in
// a Linux jail, the host path in the macOS sandbox. So `ls ~` in a jail shows
// where the shared directory is instead of an empty home. Without a usable
// workspace (workdir "" or the home itself) a stale link is removed.
func linkJailWorkspace(jail, target, workdir string) {
	link := filepath.Join(jail, "workspace")
	if workdir == "" || workdir == homeDir() || !pathExists(workdir) {
		if fi, err := os.Lstat(link); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(link)
		}
		return
	}
	if cur, err := os.Readlink(link); err == nil && cur == target {
		return
	}
	if fi, err := os.Lstat(link); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(link)
	}
	_ = os.Symlink(target, link)
}

// WorkspaceInShell reports where env's workspace appears from inside its
// shell. A Linux jail (bubblewrap, natively or in the WSL 2 distribution)
// binds it at /workspace and shows nothing of the host path; the macOS
// sandbox and unjailed shells keep the host path. "" without a workspace.
func WorkspaceInShell(env *Environment) string {
	ws := env.Workspace
	if ws == "" || ws == "none" {
		return ""
	}
	if env.Isolate && (runtime.GOOS == "linux" || useWSL()) {
		return jailWorkdir
	}
	return ws
}

// IsolateCommand wraps inner in a jail for env. It returns a new *exec.Cmd that
// runs the same program with the same environment and stdio inside a sandbox
// that hides the real $HOME and the rest of the host filesystem while keeping
// what RF tools need. Linux uses bubblewrap (mount namespaces); macOS uses the
// Seatbelt sandbox (sandbox-exec).
func IsolateCommand(inner *exec.Cmd, env *Environment, workdir string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "linux":
		return isolateCommandLinux(inner, env, workdir)
	case "darwin":
		return isolateCommandDarwin(inner, env, workdir)
	default:
		return nil, fmt.Errorf("--isolate is not supported on %s; use the container engine for isolation, or drop --isolate", runtime.GOOS)
	}
}

// isolateCommandLinux wraps inner in a bubblewrap jail, remapping paths under RF
// Swift's state dir and the working directory to their clean in-jail locations.
func isolateCommandLinux(inner *exec.Cmd, env *Environment, workdir string) (*exec.Cmd, error) {
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

// ---------------------------------------------------------------------------
// macOS isolation (Seatbelt / sandbox-exec)
//
// macOS has no bubblewrap and no mount namespaces, so the jail is expressed as a
// Seatbelt policy applied with sandbox-exec instead. It cannot swap an empty
// HOME in by mounting; it denies the real one (and every other user home) and
// HOME is pointed at a private per-environment directory. The security goal is
// the same as the Linux jail: an isolated tool cannot read the user's personal
// files, sibling environments or other workspaces, while USB (IOKit), the
// display, the network and the /nix closure stay reachable.
// ---------------------------------------------------------------------------

// sandboxExecPath returns the path to macOS's sandbox-exec, or "" when absent.
func sandboxExecPath() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	const p = "/usr/bin/sandbox-exec"
	if pathExists(p) {
		return p
	}
	if q, err := exec.LookPath("sandbox-exec"); err == nil {
		return q
	}
	return ""
}

// sbplString quotes s as a Seatbelt (SBPL) string literal. SBPL uses
// double-quoted, backslash-escaped strings, which Go's quoted form matches
// closely enough for filesystem paths.
func sbplString(s string) string { return strconv.Quote(s) }

// darwinSandboxProfile builds the Seatbelt policy for env's jail. It starts from
// a working system (allow default) and takes away access to personal data,
// rather than deny-all and re-allow the whole of macOS's runtime - the same
// usability-preserving trade-off the Linux jail makes. Every user home is
// denied, then only this environment's state and shared profile (read-only) and
// its workspace and private HOME (read-write) are allowed back. Rules are
// emitted in order of increasing specificity because SBPL is last-match-wins,
// and the private HOME lives under the state dir so it must come last to stay
// writable.
func darwinSandboxProfile(env *Environment, workdir, jail string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(allow default)\n")
	// Personal files, keys, browser data, sibling RF Swift environments and
	// other workspaces all live under a user home; hide them all, plus root's.
	b.WriteString("(deny file-read* file-write* (subpath \"/Users\"))\n")
	b.WriteString("(deny file-read* file-write* (subpath \"/private/var/root\"))\n")

	// Every allowed directory sits under a denied home, and path resolution
	// needs to stat each component on the way there: SQLite (nix's fetcher
	// and eval caches under $HOME/.cache) lstat()s the full path component by
	// component and fails with "unable to open database file" when /Users is
	// opaque; `mkdir -p` and realpath() trip the same way. Allow metadata
	// reads on the ancestors only - as literals, so their contents (the rest
	// of the home) stay unreadable and unlistable.
	seen := map[string]bool{}
	writeAllow := func(mode, p string) {
		if p == "" {
			return
		}
		for _, dir := range ancestorDirs(p) {
			if !seen[dir] {
				seen[dir] = true
				fmt.Fprintf(&b, "(allow file-read-metadata (literal %s))\n", sbplString(dir))
			}
		}
		fmt.Fprintf(&b, "(allow %s (subpath %s))\n", mode, sbplString(p))
	}
	writeAllow("file-read*", EnvDir(env.Name))
	if env.Lazy {
		// Where the shims pin what they build; the rest of the state dir
		// (manifest, rc files) stays read-only, as in the Linux jail.
		_ = ensureDir(toolsDir(env.Name))
		writeAllow("file-read* file-write*", toolsDir(env.Name))
	}
	if sh := SharedExtrasProfile(); pathExists(sh) {
		writeAllow("file-read*", sh)
	}
	if workdir != "" && workdir != homeDir() && pathExists(workdir) {
		writeAllow("file-read* file-write*", workdir)
	}
	writeAllow("file-read* file-write*", jail)
	return b.String()
}

// ancestorDirs lists the directories above p, nearest last, excluding the
// root and p itself: "/a/b/c" -> ["/a", "/a/b"].
func ancestorDirs(p string) []string {
	var out []string
	for dir := filepath.Dir(filepath.Clean(p)); dir != "/" && dir != "." && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		out = append([]string{dir}, out...)
	}
	return out
}

// isolateCommandDarwin wraps inner in a Seatbelt sandbox. Unlike the Linux jail
// there is no path remapping: the sandbox restricts access to the real host
// paths in place, so PATH, the rcfile argument and the workspace keep their
// on-host locations. HOME is redirected to the private per-environment jail.
func isolateCommandDarwin(inner *exec.Cmd, env *Environment, workdir string) (*exec.Cmd, error) {
	sb := sandboxExecPath()
	if sb == "" {
		return nil, fmt.Errorf("--isolate needs macOS's sandbox-exec, which was not found at /usr/bin/sandbox-exec")
	}
	jail := jailHomeDir(env.Name)
	if err := os.MkdirAll(jail, 0o700); err != nil {
		return nil, fmt.Errorf("could not create the isolated HOME %q: %w", jail, err)
	}
	linkJailWorkspace(jail, workdir, workdir)
	profile := darwinSandboxProfile(env, workdir, jail)

	// sandbox-exec -p <profile> <program> [args...]. The profile is a single
	// argument (exec runs no shell), so its newlines and quotes are literal.
	args := append([]string{"-p", profile}, inner.Args...)
	cmd := exec.Command(sb, args...)

	// Point HOME at the private jail (the real one is denied), leaving every
	// other inherited variable - PATH into the /nix profile, DISPLAY, the GL
	// vars - untouched: macOS keeps the on-host paths.
	cmd.Env = replaceEnv(inner.Env, "HOME", jail)

	// Start in the workspace if there is one, else in the private home, so the
	// shell never opens in a directory the sandbox denies.
	chdir := jail
	if workdir != "" && workdir != homeDir() && pathExists(workdir) {
		chdir = workdir
	}
	cmd.Dir = chdir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inner.Stdin, inner.Stdout, inner.Stderr
	return cmd, nil
}

// replaceEnv returns env with key set to val, replacing any existing entry.
func replaceEnv(env []string, key, val string) []string {
	out := make([]string, 0, len(env)+1)
	prefix := key + "="
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			out = append(out, prefix+val)
			found = true
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, prefix+val)
	}
	return out
}
