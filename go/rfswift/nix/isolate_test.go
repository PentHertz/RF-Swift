package nix

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hasPair reports whether args contains a b as consecutive elements.
func hasPair(args []string, a, b string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return true
		}
	}
	return false
}

func hasTriple(args []string, a, b, c string) bool {
	for i := 0; i+2 < len(args); i++ {
		if args[i] == a && args[i+1] == b && args[i+2] == c {
			return true
		}
	}
	return false
}

func TestIsolateArgsKeepsDevicesAndStoreButHidesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	t.Setenv("SUDO_USER", "") // ensure homeDir() uses HOME

	env := &Environment{Name: "jailtest"}
	args := isolateArgs(env, "")

	// The /nix store must be bound read-only (tools + profile live there).
	if !hasTriple(args, "--ro-bind", "/nix", "/nix") {
		t.Errorf("expected read-only bind of /nix; got: %v", args)
	}
	// USB and sysfs must be passed through for hardware access.
	if !hasTriple(args, "--dev-bind-try", "/dev/bus/usb", "/dev/bus/usb") {
		t.Errorf("expected /dev/bus/usb dev-bind for USB devices")
	}
	if !hasTriple(args, "--ro-bind-try", "/sys", "/sys") {
		t.Errorf("expected /sys bind for device enumeration")
	}
	// A serial node range should be bound (HydraBus/Flipper/NanoVNA CDC-ACM).
	if !hasTriple(args, "--dev-bind-try", "/dev/ttyACM0", "/dev/ttyACM0") {
		t.Errorf("expected serial nodes to be dev-bound")
	}
	// The real HOME must be shadowed by the private per-env jail home, never
	// bound straight through, and nothing must be bound under HOME (which would
	// clutter it / look like a leak).
	if !hasTriple(args, "--bind", jailHomeDir("jailtest"), home) {
		t.Errorf("expected jail home %q bound over HOME %q", jailHomeDir("jailtest"), home)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--bind "+home+" "+home) {
		t.Errorf("HOME must not be bound read-write straight through: %s", joined)
	}
	// Only THIS environment's state dir is mounted (at jailEnv), never the whole
	// ~/.rfswift/nix tree, so sibling environments are not exposed.
	if !hasTriple(args, "--ro-bind-try", EnvDir("jailtest"), jailEnv) {
		t.Errorf("expected env dir %q mounted at %q", EnvDir("jailtest"), jailEnv)
	}
	if strings.Contains(strings.Join(args, " "), EnvironmentsDir()+" ") {
		t.Errorf("the shared environments dir must not be mounted (would expose sibling envs)")
	}
	// Nothing is bound under HOME.
	for i := 0; i+2 < len(args); i++ {
		if (args[i] == "--ro-bind-try" || args[i] == "--bind-try") && strings.HasPrefix(args[i+2], home+"/") {
			t.Errorf("nothing should be bound under HOME, found target %q", args[i+2])
		}
	}
	// Private PID/IPC and tmpfs /tmp.
	if !contains(args, "--unshare-pid") || !contains(args, "--unshare-ipc") {
		t.Errorf("expected private PID and IPC namespaces")
	}
	if !hasPair(args, "--tmpfs", "/tmp") {
		t.Errorf("expected a private /tmp")
	}
}

func TestIsolateSupportedMatchesPlatform(t *testing.T) {
	// Linux (bubblewrap) always supports it; macOS supports it when sandbox-exec
	// is present; nothing else does.
	var want bool
	switch runtime.GOOS {
	case "linux":
		want = true
	case "darwin":
		want = sandboxExecPath() != ""
	}
	if got := IsolateSupported(); got != want {
		t.Errorf("IsolateSupported()=%v, want %v on %s", got, want, runtime.GOOS)
	}
}

func TestDarwinSandboxProfileHidesHomeKeepsEnv(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only sandbox profile")
	}
	env := &Environment{Name: "rfid"}
	jail := jailHomeDir(env.Name)
	profile := darwinSandboxProfile(env, "", jail)
	// The real user home is denied...
	if !strings.Contains(profile, "(deny file-read* file-write* (subpath \"/Users\"))") {
		t.Errorf("profile does not deny /Users:\n%s", profile)
	}
	// ...but the private jail HOME and the env state are allowed back, with the
	// writable jail rule after the read-only env rule (last match wins).
	envIdx := strings.Index(profile, sbplString(EnvDir(env.Name)))
	jailIdx := strings.LastIndex(profile, sbplString(jail))
	if envIdx < 0 || jailIdx < 0 {
		t.Fatalf("profile missing env or jail allow rule:\n%s", profile)
	}
	if jailIdx < envIdx {
		t.Errorf("writable jail rule must come after the env rule (last match wins):\n%s", profile)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// fakeSysctls makes readProcSysctl answer from a map for the test's duration.
func fakeSysctls(t *testing.T, values map[string]string) {
	t.Helper()
	orig := readProcSysctl
	readProcSysctl = func(path string) string { return values[path] }
	t.Cleanup(func() { readProcSysctl = orig })
}

func TestBwrapPreflightExplainsUserns(t *testing.T) {
	dir := t.TempDir()
	fake := dir + "/bwrap"
	// A fake bwrap that fails the way a userns-restricted host does.
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'bwrap: setting up uid map: Permission denied' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		sysctls map[string]string
		want    []string
		absent  []string
	}{
		{
			// Ubuntu 24.04+: AppArmor lets only the profiled /usr/bin/bwrap create
			// user namespaces; a Nix-built bwrap is unconfined and blocked. Point
			// at the distro package and the profile first, the sysctl last.
			name:    "apparmor restriction, non-distro bwrap",
			sysctls: map[string]string{sysctlAppArmorUserns: "1", sysctlUsernsClone: "1"},
			want: []string{"user namespace", "AppArmor", "profile", "/usr/bin/bwrap", fake,
				"apt install bubblewrap", "apparmor-profiles", "bwrap-userns-restrict", "apparmor_parser",
				"sysctl -w kernel.apparmor_restrict_unprivileged_userns=0", "last resort"},
		},
		{
			// Debian kernel with the knob switched off: nothing to do with AppArmor.
			name:    "userns_clone disabled",
			sysctls: map[string]string{sysctlUsernsClone: "0"},
			want:    []string{"user namespace", "sysctl -w kernel.unprivileged_userns_clone=1"},
			absent:  []string{"apparmor_restrict_unprivileged_userns=0", "apparmor-profiles"},
		},
		{
			// Neither knob explains it (stock Debian has no AppArmor restriction):
			// name both knobs to check, do not tell the user to flip either.
			name:    "unknown cause",
			sysctls: map[string]string{},
			want:    []string{"user namespace", "sysctl kernel.apparmor_restrict_unprivileged_userns", "sysctl kernel.unprivileged_userns_clone", "without --isolate"},
			absent:  []string{"sysctl -w"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeSysctls(t, tc.sysctls)
			err := bwrapPreflight(fake)
			if err == nil {
				t.Fatal("expected preflight to fail")
			}
			msg := err.Error()
			if !strings.Contains(msg, "setting up uid map: Permission denied") {
				t.Errorf("bwrap's own message dropped from: %s", msg)
			}
			for _, want := range tc.want {
				if !strings.Contains(strings.ToLower(msg), strings.ToLower(want)) {
					t.Errorf("guidance missing %q in: %s", want, msg)
				}
			}
			for _, no := range tc.absent {
				if strings.Contains(strings.ToLower(msg), strings.ToLower(no)) {
					t.Errorf("guidance wrongly contains %q in: %s", no, msg)
				}
			}
		})
	}
}

// The distribution's bwrap wins over another one earlier on PATH: on Ubuntu
// 24.04+ it is the only bwrap the AppArmor userns profile covers, so a Nix
// profile's bwrap ahead of /usr/bin must not be picked.
func TestResolveBwrapPrefersDistroBinaryOverPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("bubblewrap is Linux-only")
	}
	dir := t.TempDir()
	distro := dir + "/distro/bwrap"
	onPath := dir + "/nixprofile/bwrap"
	for _, p := range []string{distro, onPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	orig := distroBwrap
	distroBwrap = distro
	t.Cleanup(func() { distroBwrap = orig })
	t.Setenv("PATH", filepath.Dir(onPath))

	got, err := resolveBwrap()
	if err != nil {
		t.Fatal(err)
	}
	if got != distro {
		t.Errorf("resolveBwrap() = %s, want the distribution's %s over the PATH one", got, distro)
	}

	// Without the distribution package, the PATH one is still used.
	distroBwrap = dir + "/missing/bwrap"
	got, err = resolveBwrap()
	if err != nil {
		t.Fatal(err)
	}
	if got != onPath {
		t.Errorf("resolveBwrap() = %s, want the PATH bwrap %s", got, onPath)
	}
}
