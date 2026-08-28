package common

import "testing"

func TestCheckUSBAccess(t *testing.T) {
	cases := []struct {
		name       string
		devices    []string
		bindings   []string
		rules      []string
		privileged bool
		level      string
		tree, rule bool
	}{
		{name: "windows defaults: device tree + rules", devices: []string{"/dev/bus/usb:/dev/bus/usb", "/dev/snd:/dev/snd"}, rules: []string{"c 189:* rwm", "c 166:* rwm"}, level: "ok", tree: true, rule: true},
		{name: "device tree alone infers the rule", devices: []string{"/dev/bus/usb:/dev/bus/usb"}, level: "ok", tree: true, rule: true},
		{name: "bare device path", devices: []string{"/dev/bus/usb"}, level: "ok", tree: true, rule: true},
		{name: "workbench hotplug defaults: bind + rule", bindings: []string{"/dev/bus/usb:/dev/bus/usb:rw"}, rules: []string{"c 189:* rwm"}, level: "ok", tree: true, rule: true},
		{name: "bind mount without rule opens nothing", bindings: []string{"/dev/bus/usb:/dev/bus/usb:rw"}, level: "warn", tree: true, rule: false},
		{name: "wildcard rule counts", bindings: []string{"/dev/bus/usb:/dev/bus/usb"}, rules: []string{"a *:* rwm"}, level: "ok", tree: true, rule: true},
		{name: "cgroup v1 catch-all", bindings: []string{"/dev/bus/usb:/dev/bus/usb"}, rules: []string{"a"}, level: "ok", tree: true, rule: true},
		{name: "read-only rule is not enough", bindings: []string{"/dev/bus/usb:/dev/bus/usb"}, rules: []string{"c 189:* r"}, level: "warn", tree: true, rule: false},
		{name: "other major does not help", bindings: []string{"/dev/bus/usb:/dev/bus/usb"}, rules: []string{"c 166:* rwm"}, level: "warn", tree: true, rule: false},
		{name: "single node only", devices: []string{"/dev/bus/usb/001/002:/dev/bus/usb/001/002"}, level: "warn", tree: false, rule: false},
		{name: "nothing", devices: []string{"/dev/ttyACM0:/dev/ttyACM0"}, bindings: []string{"/tmp/.X11-unix:/tmp/.X11-unix"}, rules: []string{"c 166:* rwm"}, level: "none"},
		{name: "empty", level: "none"},
		{name: "privileged always ok", privileged: true, level: "ok", rule: true},
	}
	for _, c := range cases {
		got := CheckUSBAccess(c.devices, c.bindings, c.rules, c.privileged)
		if got.Level != c.level || got.Tree != c.tree || got.Rule != c.rule {
			t.Fatalf("%s: got level=%s tree=%v rule=%v, want level=%s tree=%v rule=%v (%+v)", c.name, got.Level, got.Tree, got.Rule, c.level, c.tree, c.rule, got)
		}
		if got.Summary == "" {
			t.Fatalf("%s: empty summary", c.name)
		}
		if got.Level != "ok" && got.Advice == "" {
			t.Fatalf("%s: non-ok result needs advice", c.name)
		}
		if got.Privileged != c.privileged {
			t.Fatalf("%s: privileged flag lost", c.name)
		}
	}
	if got := CheckUSBAccess(nil, nil, nil, true); got.Advice == "" {
		t.Fatal("privileged result must explain that privileged mode is not required")
	}
	if got := CheckUSBAccess([]string{"/dev/bus/usb/001/002"}, nil, nil, false); !got.Nodes {
		t.Fatal("node mapping must be reported")
	}
}

func TestSplitMappingSpec(t *testing.T) {
	cases := []struct{ in, host, target string }{
		{"/dev/bus/usb:/dev/bus/usb:rw", "/dev/bus/usb", "/dev/bus/usb"},
		{"/dev/bus/usb/", "/dev/bus/usb", "/dev/bus/usb"},
		{" /dev/snd : /dev/snd ", "/dev/snd", "/dev/snd"},
		{"C:\\data:/data", "C", "\\data"},
		{"", "", ""},
	}
	for _, c := range cases {
		host, target := splitMappingSpec(c.in)
		if host != c.host || target != c.target {
			t.Fatalf("splitMappingSpec(%q) = %q, %q; want %q, %q", c.in, host, target, c.host, c.target)
		}
	}
}
