//go:build linux

package dock

import (
	"golang.org/x/sys/unix"
)

// dockerUlimitResources maps Docker/Podman --ulimit names to Linux rlimits.
var dockerUlimitResources = map[string]int{
	"core":       unix.RLIMIT_CORE,
	"cpu":        unix.RLIMIT_CPU,
	"data":       unix.RLIMIT_DATA,
	"fsize":      unix.RLIMIT_FSIZE,
	"locks":      unix.RLIMIT_LOCKS,
	"memlock":    unix.RLIMIT_MEMLOCK,
	"msgqueue":   unix.RLIMIT_MSGQUEUE,
	"nice":       unix.RLIMIT_NICE,
	"nofile":     unix.RLIMIT_NOFILE,
	"nproc":      unix.RLIMIT_NPROC,
	"rss":        unix.RLIMIT_RSS,
	"rtprio":     unix.RLIMIT_RTPRIO,
	"rttime":     unix.RLIMIT_RTTIME,
	"sigpending": unix.RLIMIT_SIGPENDING,
	"stack":      unix.RLIMIT_STACK,
	"as":         unix.RLIMIT_AS,
}

// hostHardLimit returns this process's hard limit for a Docker ulimit name,
// with -1 standing for unlimited. ok is false for unknown names.
//
//	in(1): string name Docker ulimit name (rtprio, memlock, nofile, ...)
//	out: int64 hard limit, -1 when unlimited
//	out: bool  false when the name has no Linux rlimit equivalent
func hostHardLimit(name string) (int64, bool) {
	resource, ok := dockerUlimitResources[name]
	if !ok {
		return 0, false
	}
	var limit unix.Rlimit
	if err := unix.Getrlimit(resource, &limit); err != nil {
		return 0, false
	}
	if limit.Max == unix.RLIM_INFINITY {
		return -1, true
	}
	return int64(limit.Max), true
}
