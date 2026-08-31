package nix

import (
	"os"
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

func TestBwrapPreflightExplainsUserns(t *testing.T) {
	dir := t.TempDir()
	fake := dir + "/bwrap"
	// A fake bwrap that fails the way a userns-restricted host does.
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'bwrap: setting up uid map: Permission denied' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := bwrapPreflight(fake)
	if err == nil {
		t.Fatal("expected preflight to fail")
	}
	msg := err.Error()
	for _, want := range []string{"user namespace", "sysctl", "apparmor_restrict_unprivileged_userns"} {
		if !strings.Contains(strings.ToLower(msg), strings.ToLower(want)) {
			t.Errorf("guidance missing %q in: %s", want, msg)
		}
	}
}
