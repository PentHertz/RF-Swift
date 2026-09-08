package dock

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	common "penthertz/rfswift/common"
)

func TestLoadCreationDefaultsUsesPlatformConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("HOME-isolated path assertion is Linux-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	path := common.ConfigFileByPlatform()
	if path != filepath.Join(home, ".config", "rfswift", "config.ini") {
		t.Fatalf("platform config path = %q", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `[general]
imagename = test/image:latest
repotag = test/repo
[container]
shell = /bin/zsh
bindings = /data:/data
network = host
exposedports =
portbindings =
x11forward = /tmp/.X11-unix:/tmp/.X11-unix
xdisplay = DISPLAY=:0
extrahost = lab.local:192.0.2.10
extraenv = TEST=1
devices = /dev/bus/usb:/dev/bus/usb,/dev/ttyACM0:/dev/ttyACM0
privileged = false
caps = NET_RAW
seccomp =
cgroups = c 189:* rwm,c 166:* rwm
[audio]
pulse_server =
[desktop]
proto =
host = 127.0.0.1
password =
port = 6080
ssl =
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := LoadCreationDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if d.Path != path || d.Shell != "/bin/zsh" || d.Network != "host" || len(d.Devices) != 2 || len(d.CgroupRules) != 2 || len(d.Bindings) != 1 {
		t.Fatalf("unexpected defaults: %#v", d)
	}
}
