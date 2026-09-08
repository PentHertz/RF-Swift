package cli

import (
	"strings"
	"testing"
)

func TestDeviceAccessAdviceNamesTheActualProblem(t *testing.T) {
	member := func([]string) (absent, notMember []string) { return nil, nil }
	notMember := func([]string) (absent, notMember []string) { return nil, []string{"dialout"} }

	cases := []struct {
		name   string
		info   deviceAccessInfo
		groups func([]string) (absent, notMember []string)
		want   []string
		absent []string
	}{
		{"usable device", deviceAccessInfo{Kind: "device", Accessible: true, Group: "dialout", GroupAccess: true}, member, nil, nil},
		{"missing path", deviceAccessInfo{Kind: "missing"}, member, nil, nil},
		{"stray directory", deviceAccessInfo{Kind: "directory", Group: "root", OwnerRoot: true}, member, []string{"empty directory", "rfswift host devclean", "sudo rmdir /dev/ttyACM0"}, []string{"newgrp"}},
		{"root-only node", deviceAccessInfo{Kind: "device", Group: "root", GroupAccess: false, OwnerRoot: true}, member, []string{"no udev rule", "rfswift host udev"}, []string{"newgrp root"}},
		{"group not joined", deviceAccessInfo{Kind: "device", Group: "dialout", GroupAccess: true}, notMember, []string{"usermod -aG dialout"}, []string{"newgrp"}},
		{"group not in session", deviceAccessInfo{Kind: "device", Group: "dialout", GroupAccess: true}, member, []string{"newgrp dialout", "log out and in"}, []string{"usermod"}},
	}
	for _, c := range cases {
		got := deviceAccessAdvice("/dev/ttyACM0", c.info, c.groups)
		if c.want == nil {
			if got != "" {
				t.Errorf("%s: expected no advice, got %q", c.name, got)
			}
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: advice %q lacks %q", c.name, got, w)
			}
		}
		for _, a := range c.absent {
			if strings.Contains(got, a) {
				t.Errorf("%s: advice %q must not suggest %q", c.name, got, a)
			}
		}
	}
}
