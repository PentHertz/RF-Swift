package dock

import (
	"os"
	"runtime"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestParsePodmanMode(t *testing.T) {
	for in, want := range map[string]PodmanMode{"": PodmanModeAuto, "AUTO": PodmanModeAuto, "rootless": PodmanModeRootless, "user": PodmanModeRootless, " Rootful ": PodmanModeRootful, "system": PodmanModeRootful} {
		if got, ok := ParsePodmanMode(in); !ok || got != want {
			t.Errorf("ParsePodmanMode(%q) = %q, %v", in, got, ok)
		}
	}
	if _, ok := ParsePodmanMode("sometimes"); ok {
		t.Error("unknown modes must be rejected")
	}
}

func TestRootfulPodmanURI(t *testing.T) {
	for uri, want := range map[string]bool{
		"unix:///run/podman/podman.sock":           true,
		"unix:///var/run/podman/podman.sock":       true,
		"unix:///run/user/1000/podman/podman.sock": false,
		"ssh://core@host/run/podman/podman.sock":   false,
	} {
		if got := isRootfulPodmanURI(uri); got != want {
			t.Errorf("isRootfulPodmanURI(%q) = %v", uri, got)
		}
	}
	if os.Getuid() != 0 && podmanURIRootless("unix:///run/podman/podman.sock") {
		t.Error("the system socket is never the rootless service")
	}
}

// A user configured with podman_mode = rootful gets the system socket,
// reported as rootful (so the rootless restrictions are not applied).
func TestDetectPodmanSocketRootfulMode(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("rootful mode for a user is a Linux, non-root path")
	}
	t.Setenv("CONTAINER_HOST", "")
	os.Unsetenv("CONTAINER_HOST")
	t.Setenv("DOCKER_HOST", "")
	os.Unsetenv("DOCKER_HOST")
	podmanModeOverride = PodmanModeRootful
	defer func() { podmanModeOverride = ""; withdrawContainerHost() }()

	sock, rootless := detectPodmanSocket()
	if rootless || !isRootfulPodmanURI(sock) {
		t.Fatalf("rootful mode: got %q rootless=%v", sock, rootless)
	}
	e := &PodmanEngine{}
	if e.IsRootless() {
		t.Fatal("engine must report rootful")
	}
	if got := os.Getenv("CONTAINER_HOST"); got != e.GetSocketPath() {
		t.Fatalf("the podman CLI must be routed to the same service, CONTAINER_HOST=%q", got)
	}

	podmanModeOverride = PodmanModeRootless
	sock, rootless = detectPodmanSocket()
	if !rootless || isRootfulPodmanURI(sock) {
		t.Fatalf("rootless mode after rootful: got %q rootless=%v (exported CONTAINER_HOST must not stick)", sock, rootless)
	}
	(&PodmanEngine{}).ensureDetected()
	if os.Getenv("CONTAINER_HOST") != "" {
		t.Fatal("switching back to rootless must withdraw the CONTAINER_HOST RF Swift exported")
	}
}

func TestUserContainerHostWins(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("Linux, non-root")
	}
	t.Setenv("CONTAINER_HOST", "unix:///tmp/my-own-podman.sock")
	podmanModeOverride = PodmanModeRootful
	defer func() { podmanModeOverride = "" }()
	exportRootfulContainerHost("unix:///run/podman/podman.sock")
	if got := os.Getenv("CONTAINER_HOST"); got != "unix:///tmp/my-own-podman.sock" {
		t.Fatalf("the user's CONTAINER_HOST was overwritten: %q", got)
	}
	if sock, rootless := detectPodmanSocket(); sock != "unix:///tmp/my-own-podman.sock" || !rootless {
		t.Fatalf("got %q rootless=%v", sock, rootless)
	}
}

func TestSELinuxLabelApplies(t *testing.T) {
	plain := func() *container.HostConfig { return &container.HostConfig{} }
	cases := []struct {
		name      string
		engine    EngineType
		enforcing bool
		policy    string
		hc        *container.HostConfig
		want      bool
	}{
		{"podman enforcing", EnginePodman, true, "auto", plain(), true},
		{"docker enforcing", EngineDocker, true, "auto", plain(), true},
		{"permissive", EnginePodman, false, "auto", plain(), false},
		{"opted out", EnginePodman, true, "keep", plain(), false},
		{"privileged", EnginePodman, true, "auto", &container.HostConfig{Privileged: true}, false},
		{"lima VM", EngineLima, true, "auto", plain(), false},
		{"explicit label", EnginePodman, true, "auto", &container.HostConfig{SecurityOpt: []string{"label=type:my_t"}}, false},
		{"seccomp only", EnginePodman, true, "auto", &container.HostConfig{SecurityOpt: []string{"seccomp=unconfined"}}, true},
	}
	for _, c := range cases {
		if got := selinuxLabelApplies(c.engine, c.enforcing, c.policy, c.hc); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestPromoteDeviceTrees(t *testing.T) {
	dirs := map[string]bool{"/dev/bus/usb": true, "/dev/snd": true}
	isDir := func(p string) bool { return dirs[p] }
	binds, devices := promoteDeviceTrees(
		[]string{"/tmp/.X11-unix:/tmp/.X11-unix", "/dev/snd:/dev/snd:rw"},
		"/dev/bus/usb:/dev/bus/usb,/dev/snd:/dev/snd,/dev/ttyACM0:/dev/ttyACM0, /dev/vhci:/dev/vhci", isDir)
	if devices != "/dev/ttyACM0:/dev/ttyACM0,/dev/vhci:/dev/vhci" {
		t.Errorf("device nodes must stay devices, trees must go: %q", devices)
	}
	want := []string{"/tmp/.X11-unix:/tmp/.X11-unix", "/dev/snd:/dev/snd:rw", "/dev/bus/usb:/dev/bus/usb"}
	if len(binds) != len(want) {
		t.Fatalf("binds = %v, want %v", binds, want)
	}
	for i := range want {
		if binds[i] != want[i] {
			t.Errorf("binds[%d] = %q, want %q", i, binds[i], want[i])
		}
	}
}

func TestPulseServerFromEnv(t *testing.T) {
	if got := pulseServerFromEnv([]string{"DISPLAY=:0", "PULSE_SERVER=tcp:localhost:34567", "HOME=/root"}); got != "tcp:localhost:34567" {
		t.Fatalf("got %q", got)
	}
	if got := pulseServerFromEnv([]string{"DISPLAY=:0"}); got != "" {
		t.Fatalf("no PULSE_SERVER: got %q", got)
	}
}
