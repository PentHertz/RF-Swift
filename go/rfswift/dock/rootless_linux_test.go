//go:build linux

package dock

import "testing"

func TestHostHardLimitReadsProcessLimits(t *testing.T) {
	if _, ok := hostHardLimit("nofile"); !ok {
		t.Fatal("nofile must resolve to RLIMIT_NOFILE")
	}
	if _, ok := hostHardLimit("no-such-limit"); ok {
		t.Fatal("unknown names must be reported as unknown so the ulimit is kept")
	}
}
