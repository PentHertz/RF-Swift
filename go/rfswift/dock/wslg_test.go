package dock

import "testing"

func TestWSLgPulseServerFor(t *testing.T) {
	cases := []struct {
		goos, in, want string
		wslg           bool
	}{
		{"windows", "tcp:localhost:34567", WSLgPulseServer, true},
		{"windows", "tcp:127.0.0.1:34567", WSLgPulseServer, true},
		{"windows", "", WSLgPulseServer, true},
		{"windows", WSLgPulseServer, WSLgPulseServer, true},
		{"windows", "tcp:192.168.1.10:4713", "tcp:192.168.1.10:4713", false},
		{"linux", "tcp:localhost:34567", "tcp:localhost:34567", false},
		{"darwin", "", "", false},
	}
	for _, c := range cases {
		got, ok := wslgPulseServerFor(c.goos, c.in)
		if got != c.want || ok != c.wslg {
			t.Fatalf("wslgPulseServerFor(%q,%q) = %q,%v; want %q,%v", c.goos, c.in, got, ok, c.want, c.wslg)
		}
	}
}

func TestBindTargetsAndEnvHasKey(t *testing.T) {
	binds := []string{"/run/desktop/mnt/host/wslg:/mnt/wslg", "/dev/bus/usb:/dev/bus/usb:rw", "C:\\Users\\me\\ws:/workspace:rw"}
	if !bindTargets(binds, "/mnt/wslg") || !bindTargets(binds, "/dev/bus/usb") || !bindTargets(binds, "/workspace") {
		t.Fatal("known destinations not detected")
	}
	if bindTargets(binds, "/tmp/.X11-unix") {
		t.Fatal("missing destination reported present")
	}
	env := []string{"DISPLAY=:0", "PULSE_SERVER=unix:/mnt/wslg/PulseServer"}
	if !envHasKey(env, "PULSE_SERVER") || envHasKey(env, "PULSE") || envHasKey(env, "XAUTHORITY") {
		t.Fatal("env key detection")
	}
}
