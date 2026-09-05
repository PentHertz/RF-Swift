package nix

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"penthertz/rfswift/hostsetup"
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

// isolationHost points hostsetup's host paths (bubblewrap locations, sysctl
// tree, AppArmor directories) at a temporary tree with the given sysctl knobs,
// so the preflight's diagnosis does not depend on the machine running the
// tests. Returns the fake distribution bwrap path (not created).
func isolationHost(t *testing.T, knobs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	saved := []string{hostsetup.DistroBwrap, hostsetup.NixOSBwrapWrapper, hostsetup.ProcSys, hostsetup.AppArmorDir, hostsetup.AppArmorExtraDir}
	t.Cleanup(func() {
		hostsetup.DistroBwrap, hostsetup.NixOSBwrapWrapper, hostsetup.ProcSys, hostsetup.AppArmorDir, hostsetup.AppArmorExtraDir = saved[0], saved[1], saved[2], saved[3], saved[4]
	})
	hostsetup.DistroBwrap = filepath.Join(dir, "usr/bin/bwrap")
	hostsetup.NixOSBwrapWrapper = filepath.Join(dir, "run/wrappers/bin/bwrap")
	hostsetup.ProcSys = filepath.Join(dir, "proc/sys")
	hostsetup.AppArmorDir = filepath.Join(dir, "etc/apparmor.d")
	hostsetup.AppArmorExtraDir = filepath.Join(dir, "usr/share/apparmor/extra-profiles")
	for name, value := range knobs {
		p := filepath.Join(hostsetup.ProcSys, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return hostsetup.DistroBwrap
}

func writeScript(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// A fake bwrap that fails the way a userns-restricted host does.
const deniedBwrap = "#!/bin/sh\necho 'bwrap: setting up uid map: Permission denied' >&2\nexit 1\n"

func TestBwrapPreflightExplainsUserns(t *testing.T) {
	cases := []struct {
		name   string
		knobs  map[string]string
		extra  bool // the packaged AppArmor profile is available
		want   []string
		absent []string
	}{
		{
			// Ubuntu 24.04: AppArmor lets only the profiled /usr/bin/bwrap create
			// user namespaces and the profile is not enabled. The fix is the one
			// command, with the steps it runs; the global sysctl is not suggested.
			name:  "apparmor restriction, profile not enabled",
			knobs: map[string]string{"kernel/apparmor_restrict_unprivileged_userns": "1", "kernel/unprivileged_userns_clone": "1"},
			extra: true,
			want: []string{"Cause: AppArmor restricts unprivileged user namespaces", "bwrap-userns-restrict",
				"Fix:   rfswift host isolate", "It will:", "enable the packaged AppArmor profile", "apparmor_parser", "Engine doctor", "without --isolate"},
			absent: []string{"sysctl -w", "none RF Swift can apply"},
		},
		{
			// Debian kernel with the knob switched off: nothing to do with AppArmor.
			name:   "userns_clone disabled",
			knobs:  map[string]string{"kernel/unprivileged_userns_clone": "0"},
			want:   []string{"Cause: unprivileged user namespaces are disabled", "Fix:   rfswift host isolate", "kernel.unprivileged_userns_clone=1"},
			absent: []string{"AppArmor", "apparmor-profiles"},
		},
		{
			// Neither knob explains it (stock Debian has no AppArmor restriction):
			// say so, point at --status, do not tell the user to flip anything.
			name:   "unknown cause",
			knobs:  map[string]string{},
			want:   []string{"neither sysctl explains it", "none RF Swift can apply automatically", "rfswift host isolate --status", "without --isolate"},
			absent: []string{"sysctl -w", "It will:"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := isolationHost(t, tc.knobs)
			writeScript(t, fake, deniedBwrap)
			if tc.extra {
				writeScript(t, filepath.Join(hostsetup.AppArmorExtraDir, hostsetup.BwrapProfile), "# profile\n")
			}
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
	distro := isolationHost(t, nil)
	onPath := filepath.Join(t.TempDir(), "nixprofile", "bwrap")
	writeScript(t, distro, "#!/bin/sh\nexit 0\n")
	writeScript(t, onPath, "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", filepath.Dir(onPath))

	got, err := resolveBwrap()
	if err != nil {
		t.Fatal(err)
	}
	if got != distro {
		t.Errorf("resolveBwrap() = %s, want the distribution's %s over the PATH one", got, distro)
	}

	// Without the distribution package, the PATH one is still used.
	if err := os.Remove(distro); err != nil {
		t.Fatal(err)
	}
	got, err = resolveBwrap()
	if err != nil {
		t.Fatal(err)
	}
	if got != onPath {
		t.Errorf("resolveBwrap() = %s, want the PATH bwrap %s", got, onPath)
	}
}
