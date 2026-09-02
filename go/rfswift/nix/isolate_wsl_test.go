package nix

import (
	"os"
	"testing"
)

// Inside WSL 2 the display, the sound server and the virtual GPU live behind
// paths that do not exist on other hosts; the jail binds them back in with
// -try variants so the same argument vector serves every Linux.
func TestIsolateArgsKeepWSLgAndVirtualGPU(t *testing.T) {
	t.Setenv("RFSWIFT_NIX_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	args := isolateArgs(&Environment{Name: "wsljail"}, "")
	if !hasTriple(args, "--ro-bind-try", "/mnt/wslg", "/mnt/wslg") {
		t.Fatal("WSLg tree (X11 socket, PulseServer) must be reachable in the jail")
	}
	if !hasTriple(args, "--dev-bind-try", "/dev/dxg", "/dev/dxg") {
		t.Fatal("the WSL virtual GPU node must be reachable in the jail")
	}
	if !hasTriple(args, "--ro-bind-try", "/tmp/.X11-unix", "/tmp/.X11-unix") {
		t.Fatal("the X11 socket directory must be bound over the private /tmp")
	}
}

// Name resolution inside the jail: a resolv.conf that is a symlink into /run
// (systemd-resolved, resolvconf, NetworkManager) gets its target bound; a plain
// file under /etc, or no file at all, needs nothing.
func TestResolvConfBinds(t *testing.T) {
	stub := "/run/systemd/resolve/stub-resolv.conf"
	got := resolvConfBinds(func(string) (string, error) { return stub, nil })
	if len(got) != 3 || got[0] != "--ro-bind-try" || got[1] != stub || got[2] != stub {
		t.Fatalf("systemd-resolved target must be bound: %q", got)
	}
	if got := resolvConfBinds(func(string) (string, error) { return "/etc/resolv.conf", nil }); got != nil {
		t.Fatalf("a plain file under /etc needs no extra bind: %q", got)
	}
	if got := resolvConfBinds(func(string) (string, error) { return "/etc/resolvconf/run/resolv.conf", nil }); got != nil {
		t.Fatalf("a target under /etc is already carried by the /etc bind: %q", got)
	}
	if got := resolvConfBinds(func(string) (string, error) { return "", os.ErrNotExist }); got != nil {
		t.Fatalf("no resolv.conf, nothing to bind: %q", got)
	}
}
